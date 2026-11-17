package timing

import (
	"context"
	"io"
	"net/http/httptrace"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// delayedStageBody represents a network body whose reads consume controlled time.
type delayedStageBody struct {
	io.Reader /* caller-owned input bytes */
}

// ingressStepBody supplies one delayed read result, including data with errors.
// Its return values exercise Reader's permission to deliver bytes and termination
// together, while Close remains owned by the caller of the timing wrapper.
type ingressStepBody struct {
	data string // original encoded input
	err  error  // configured terminal error
}

// Read models a slow network read without changing returned bytes.
func (r delayedStageBody) Read(p []byte) (int, error) {
	time.Sleep(time.Second)
	return r.Reader.Read(p)
}

// Close leaves the test reader available for fixture inspection.
func (r delayedStageBody) Close() error { return nil }

// TestStagesAdmissionSeparatesReadingWaitingAndRetries exercises original reads,
// preparation, two queues, and another attempt. Only original admission contributes
// to the four-way split and later provider waiting remains separately measurable.
func TestStagesAdmissionSeparatesReadingWaitingAndRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, s := WithStages(context.Background(), time.Now(), nil)
		body := s.ReadBody(delayedStageBody{strings.NewReader("body")})
		_, err := io.ReadAll(body)
		require.NoError(t, err)
		time.Sleep(3 * time.Second)
		time.Sleep(2 * time.Second)
		s.Wait("user", 2*time.Second)
		time.Sleep(4 * time.Second)
		s.Wait("provider", 4*time.Second)
		s.Admit()
		initial := s.Snapshot()
		require.Equal(t, int64(2000), initial["request_body_read_ms"])
		require.Equal(t, int64(3000), initial["preprocess_ms"])
		require.Equal(t, int64(11000), initial["provider_slot_acquired_ms"])
		time.Sleep(7 * time.Second)
		s.Wait("provider", 7*time.Second)
		s.Admit()
		s.Finish()
		got := s.Snapshot()
		for _, key := range []string{"request_body_read_ms", "preprocess_ms", "user_slot_wait_ms", "provider_slot_wait_ms", "provider_slot_acquired_ms"} {
			require.Equal(t, initial[key], got[key], key)
		}
		require.Equal(t, int64(7000), got["retry_provider_slot_wait_ms"])
	})
}

// TestStagesFailureAndFirstObservation preserves failure-only admission and
// measured zero. Repeated stream events and completion cannot rewrite earlier
// milestones or fabricate an acquired provider after the request has ended.
func TestStagesFailureAndFirstObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, s := WithStages(context.Background(), time.Now(), nil)
		s.SetStream(true)
		s.Mark("upstream_first_event_ms")
		time.Sleep(time.Second)
		s.Mark("upstream_first_event_ms")
		s.Finish()
		s.Admit()
		got := s.Snapshot()
		require.Equal(t, int64(0), got["upstream_first_event_ms"])
		require.Equal(t, int64(0), got["timing_admitted"])
		require.Equal(t, int64(1000), got["preprocess_ms"])
		require.NotContains(t, got, "provider_slot_acquired_ms")
		require.Equal(t, int64(0), got["user_slot_wait_ms"])
	})
}

// TestStagesAndHTTPTraceShareIngress installs tracing on either side of stage
// collection. Transport milestones must retain the request ingress reference,
// rather than silently subtracting time spent before trace installation.
func TestStagesAndHTTPTraceShareIngress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		time.Sleep(time.Second)
		ctx, stages := WithStages(t.Context(), start, nil)
		ctx = BeginHTTPTrace(ctx)
		time.Sleep(time.Second)
		httptrace.ContextClientTrace(ctx).GetConn("upstream.test")
		snapshot := stages.Snapshot()
		require.Equal(t, int64(2000), snapshot["upstream_get_conn_ms"])
		require.Equal(t, int64(1000), snapshot["upstream_started_ms"])
	})
}

// Read returns the configured bytes and terminal result after simulated input delay.
func (b ingressStepBody) Read(p []byte) (int, error) {
	time.Sleep(5 * time.Second)
	return copy(p, b.data), b.err
}

// Close releases no resources in this single-read input fixture.
func (b ingressStepBody) Close() error { return nil }

// TestStagesIngressPartialReadDiagnostics reproduces delayed, empty, and truncated uploads.
// Received bytes survive a simultaneous error, and EOF is the sole successful
// terminal result; failure snapshots retain each observed ingress milestone.
func TestStagesIngressPartialReadDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name string // scenario display name
		data string // returned input bytes
		err  error  // terminal read result
	}{
		{"complete", "body", io.EOF},
		{"truncated", "part", io.ErrUnexpectedEOF},
		{"canceled", "", context.Canceled},
		{"empty", "", io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, stages := WithStages(t.Context(), time.Now(), nil)
				stages.SetContentLength(100)
				time.Sleep(2 * time.Second)
				body := stages.ReadBody(ingressStepBody{tc.data, tc.err})
				buffer := make([]byte, 20)
				n, err := body.Read(buffer)
				require.ErrorIs(t, err, tc.err)
				require.Equal(t, tc.data, string(buffer[:n]))
				stages.Finish()
				got := stages.FailureSnapshot()
				require.Equal(t, int64(100), got["request_content_length"])
				require.Equal(t, int64(len(tc.data)), got["request_body_received_bytes"])
				if n > 0 || tc.err == io.EOF {
					require.Equal(t, int64(len(tc.data)), got["request_raw_bytes"])
				} else {
					require.NotContains(t, got, "request_raw_bytes")
				}
				require.Equal(t, int64(2000), got["request_body_read_started_ms"])
				require.Equal(t, int64(7000), got["request_body_read_finished_ms"])
				require.Equal(t, int64(5000), got["request_body_read_ms"])
				require.Equal(t, tc.err != io.EOF, got["request_body_read_failed"] == 1)
				if n > 0 {
					require.Equal(t, int64(7000), got["request_body_first_byte_ms"])
					require.Equal(t, int64(7000), got["request_body_last_byte_ms"])
				} else {
					require.NotContains(t, got, "request_body_first_byte_ms")
				}
				require.NotContains(t, got, "moderation_started_ms")
				require.NotContains(t, got, "provider_slot_acquired_ms")
			})
		})
	}
}

// TestStagesIngressBlockedReadRemainsObservable stalls after the first upload chunk.
// Snapshots remain available during a blocked read, preserve the observed byte
// count, and distinguish unfinished input from a later cancellation failure.
func TestStagesIngressBlockedReadRemainsObservable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, stages := WithStages(t.Context(), time.Now(), nil)
		reader, writer := io.Pipe()
		t.Cleanup(func() {
			require.NoError(t, reader.Close())
			require.NoError(t, writer.Close())
		})
		body := stages.ReadBody(reader)
		done := make(chan error, 1)
		go func() {
			_, err := io.ReadAll(body)
			done <- err
		}()
		synctest.Wait()
		require.Equal(t, int64(0), stages.Snapshot()["request_body_read_started_ms"])
		time.Sleep(time.Second)
		_, err := writer.Write([]byte("first"))
		require.NoError(t, err)
		synctest.Wait()
		time.Sleep(time.Minute)
		got := stages.Snapshot()
		require.Equal(t, int64(5), got["request_body_received_bytes"])
		require.Equal(t, int64(1000), got["request_body_first_byte_ms"])
		require.NotContains(t, got, "request_body_read_finished_ms")
		require.NoError(t, writer.CloseWithError(context.Canceled))
		require.ErrorIs(t, <-done, context.Canceled)
		got = stages.FailureSnapshot()
		require.Equal(t, int64(61000), got["request_body_read_finished_ms"])
		require.Equal(t, int64(1), got["request_body_read_failed"])
	})
}

// TestStagesSubstageTelemetryAndAttemptRequestBytes exercises wire byte reads,
// body-decode subtraction, pre-admission substages, and per-attempt preparation
// across two exchanges. Initial admission totals remain exact while retry
// attempts isolate their own preparation duration and request byte size.
func TestStagesSubstageTelemetryAndAttemptRequestBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, s := WithStages(context.Background(), time.Now(), nil)
		body := s.ReadBody(delayedStageBody{strings.NewReader("payload")})
		readStart := time.Now()
		raw, err := io.ReadAll(body)
		require.NoError(t, err)
		time.Sleep(150 * time.Millisecond)
		ObserveBodyDecode(ctx, int64(len(raw)*2), true, time.Since(readStart))
		// A second replay normalization over already-decompressed bytes must keep
		// request_compressed=1 and accumulate decode time without re-subtracting wire wait.
		replayStart := time.Now()
		time.Sleep(20 * time.Millisecond)
		ObserveBodyDecode(ctx, int64(len(raw)*2), false, time.Since(replayStart))

		ObserveSubstage(ctx, "ingress_validate_ms", 25*time.Millisecond)
		ObserveSubstage(ctx, "bootstrap_ms", 5*time.Millisecond)
		ObserveSubstage(ctx, "moderation_extract_ms", 10*time.Millisecond)
		ObserveSubstage(ctx, "moderation_external_ms", 320*time.Millisecond)
		ObserveSubstage(ctx, "session_ms", 8*time.Millisecond)
		ObserveSubstage(ctx, "scheduler_select_ms", 12*time.Millisecond)
		time.Sleep(360 * time.Millisecond)

		s.Admit()
		// Post-admission substage calls during retries must not alter frozen admission metrics.
		ObserveSubstage(ctx, "scheduler_select_ms", 999*time.Millisecond)

		ObserveProviderPrepare(ctx, 40*time.Millisecond)
		time.Sleep(40 * time.Millisecond)
		attempt1 := s.BeginAttempt()
		s.SetAttemptRequestBytes(attempt1, 16384)
		attempt1Fail := s.AttemptFailureSnapshot()
		require.Equal(t, int64(40), attempt1Fail["provider_prepare_ms"])
		require.Equal(t, int64(16384), attempt1Fail["upstream_request_bytes"])

		ObserveProviderPrepare(ctx, 15*time.Millisecond)
		time.Sleep(15 * time.Millisecond)
		attempt2 := s.BeginAttempt()
		s.SetAttemptRequestBytes(attempt2, 8192)
		s.FinishAttempt(attempt2)

		snap := s.Snapshot()
		require.Equal(t, int64(7), snap["request_raw_bytes"])
		require.Equal(t, int64(14), snap["request_decoded_bytes"])
		require.Equal(t, int64(1), snap["request_compressed"])
		require.Equal(t, int64(170), snap["request_body_decode_ms"])
		require.Equal(t, int64(25), snap["ingress_validate_ms"])
		require.Equal(t, int64(5), snap["bootstrap_ms"])
		require.Equal(t, int64(10), snap["moderation_extract_ms"])
		require.Equal(t, int64(320), snap["moderation_external_ms"])
		require.Equal(t, int64(8), snap["session_ms"])
		require.Equal(t, int64(12), snap["scheduler_select_ms"])
		require.Equal(t, int64(15), snap["provider_prepare_ms"])
		require.Equal(t, int64(8192), snap["upstream_request_bytes"])
		require.Equal(t, snap["provider_slot_acquired_ms"], snap["request_body_read_ms"]+snap["preprocess_ms"]+snap["user_slot_wait_ms"]+snap["provider_slot_wait_ms"])

		failSnap := s.FailureSnapshot()
		require.Equal(t, int64(170), failSnap["request_body_decode_ms"])
		require.Equal(t, int64(15), failSnap["provider_prepare_ms"])
		require.Equal(t, int64(8192), failSnap["upstream_request_bytes"])
	})
}
