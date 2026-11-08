package usage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/timing"
)

// Usage records can carry an explicit request category beside older stream
// and WebSocket flags. These cases exercise transport normalization, legacy
// recovery and nil receivers, checking the effective category or updated
// booleans in each selected fixture.

// TestUsageLogSyncRequestTypeAndLegacyFields supplies WS v2 with false legacy
// flags. It requires the stored category to stay WS v2 and both flags to
// become true.
func TestUsageLogSyncRequestTypeAndLegacyFields(t *testing.T) {
	t.Parallel()

	log := &UsageLog{RequestType: RequestTypeWSV2, Stream: false, OpenAIWSMode: false}
	log.SyncRequestTypeAndLegacyFields()

	require.Equal(t, RequestTypeWSV2, log.RequestType)
	require.True(t, log.Stream)
	require.True(t, log.OpenAIWSMode)
}

// TestUsageLogEffectiveRequestTypeFallback supplies an unknown category with
// both legacy transport flags set. It requires the effective category to
// resolve to WS v2.
func TestUsageLogEffectiveRequestTypeFallback(t *testing.T) {
	t.Parallel()

	log := &UsageLog{RequestType: RequestTypeUnknown, Stream: true, OpenAIWSMode: true}
	require.Equal(t, RequestTypeWSV2, log.EffectiveRequestType())
}

// TestUsageLogEffectiveRequestTypeNilReceiver reads a nil UsageLog pointer.
// It requires the returned category to be unknown so callers can inspect
// absent records.
func TestUsageLogEffectiveRequestTypeNilReceiver(t *testing.T) {
	t.Parallel()

	var log *UsageLog
	require.Equal(t, RequestTypeUnknown, log.EffectiveRequestType())
}

// TestUsageLogSyncRequestTypeAndLegacyFieldsNilReceiver invokes
// synchronization on a nil UsageLog pointer. Returning without panic is the
// sole observation protected by this absent-record smoke case.
func TestUsageLogSyncRequestTypeAndLegacyFieldsNilReceiver(t *testing.T) {
	t.Parallel()

	var log *UsageLog
	log.SyncRequestTypeAndLegacyFields()
}

// TestUsageLogTPSSelectsTextOutput distinguishes text generation from generated
// media and Live sessions. Image input and supported text transports remain
// eligible, while media output and missing performance observations remain absent.
func TestUsageLogTPSSelectsTextOutput(t *testing.T) {
	image, video, token := "image", "video", "token"
	for _, tc := range []struct {
		name     string          // usage eligibility scenario label
		change   func(*UsageLog) // category or observation change
		eligible bool            // expected average output rate availability
	}{
		{"sync", func(u *UsageLog) { u.RequestType = RequestTypeSync }, true},
		{"stream", func(u *UsageLog) { u.RequestType = RequestTypeStream }, true},
		{"websocket", func(u *UsageLog) { u.RequestType = RequestTypeWSV2 }, true},
		{"cyber_completed", func(u *UsageLog) { u.RequestType = RequestTypeCyberBlocked }, true},
		{"legacy", func(u *UsageLog) { u.Stream = true }, true},
		{"invalid_type", func(u *UsageLog) { u.RequestType = 99 }, true},
		{"live", func(u *UsageLog) { u.RequestType = RequestTypeLive }, false},
		{"image_count", func(u *UsageLog) { u.ImageCount = 1 }, false},
		{"image_tokens", func(u *UsageLog) { u.ImageOutputTokens = 1 }, false},
		{"image_billing", func(u *UsageLog) { u.BillingMode = &image }, false},
		{"image_media", func(u *UsageLog) { u.MediaType = &image }, false},
		{"video_count", func(u *UsageLog) { u.VideoCount = 1 }, false},
		{"video_billing", func(u *UsageLog) { u.BillingMode = &video }, false},
		{"video_media", func(u *UsageLog) { u.MediaType = &video }, false},
		{"image_input", func(u *UsageLog) {
			u.ImageInputTokens = 100
			u.BillingMode = &token
		}, true},
		{"historical", func(u *UsageLog) { u.Timings = nil }, false},
		{"incomplete_usage", func(u *UsageLog) { u.Timings.UsageComplete = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := timing.NewPerformance(timing.FirstOutput, 200)
			p.Complete, p.UsageComplete = true, true
			p.Values["upstream_finished_ms"] = 1200
			row := &UsageLog{Timings: p, OutputTokens: 17}
			tc.change(row)
			if tc.eligible {
				require.NotNil(t, row.TPS())
				require.Equal(t, 17.0, *row.TPS())
			} else {
				require.Nil(t, row.TPS())
			}
		})
	}
	var missing *UsageLog
	require.Nil(t, missing.TPS())
}
