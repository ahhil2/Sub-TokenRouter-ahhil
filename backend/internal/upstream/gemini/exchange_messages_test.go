package gemini

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// exchange_messages_test.go verifies cancellation at request construction, transport, and
// backoff for the Messages exchange. The shared fixtures also keep independent
// upstream deadlines retryable, covering the distinction between request
// termination and a provider that timed out while its client remains active.

// TestExchangeMessagesCancellation covers canceled requests before and during
// dispatch or backoff. It checks error identity, stopped attempts, response
// cleanup, and retry eligibility for independent upstream deadlines.
func TestExchangeMessagesCancellation(t *testing.T) {
	testExchangeCancellation(t, ExchangeMessages)
}

// TestExchangeMessagesCancelsSignatureRetry interrupts the delay after a
// signature-related response triggers history conversion. It checks that the
// response closes and the replacement payload receives no additional attempt.
func TestExchangeMessagesCancelsSignatureRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var closes atomic.Int32
		calls := 0
		options := cancellationExchangeOptions()
		options.Do = func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: &executeBody{ReadCloser: io.NopCloser(strings.NewReader(`{"error":{"message":"invalid thought_signature"}}`)), closes: &closes}}, nil
		}
		options.FilterThinking = func() []byte { return []byte(`{"messages":[]}`) }
		options.ConvertRetry = func(body []byte) ([]byte, error) { return body, nil }
		options.ReplaceBody = func([]byte) {}
		// Interrupt the shared backoff after signature recovery has rebuilt the
		// body, before another request can consume a slot in the attempt budget.
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()
		started := time.Now()
		_, err := ExchangeMessages(ctx, options)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 100*time.Millisecond, time.Since(started))
		require.Equal(t, 1, calls)
		require.EqualValues(t, 1, closes.Load())
	})
}
