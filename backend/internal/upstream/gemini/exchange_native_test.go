package gemini

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// exchange_native_test.go verifies cancellation at request construction, transport, and
// backoff for the Native exchange. The shared fixtures also keep independent
// upstream deadlines retryable, covering the distinction between request
// termination and a provider that timed out while its client remains active.

// TestExchangeNativeCancellation covers canceled requests before and during
// dispatch or backoff. It checks error identity, stopped attempts, response
// cleanup, and retry eligibility for independent upstream deadlines.
func TestExchangeNativeCancellation(t *testing.T) {
	testExchangeCancellation(t, ExchangeNative)
}

// TestExchangeNativeCancellationSkipsCountFallback cancels a token-count request
// from transport with its estimate path enabled. It checks that cancellation
// remains an error and does not become a successful local estimate.
func TestExchangeNativeCancellationSkipsCountFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options := cancellationExchangeOptions()
	options.MaxRetries = 1
	options.CountFallback = true
	options.Do = func(*http.Request) (*http.Response, error) {
		cancel()
		return nil, context.Canceled
	}
	options.EstimateCount = func() int {
		t.Error("canceled request reached token estimate")
		return 1
	}
	result, err := ExchangeNative(ctx, options)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result.EstimatedTokens)
}

// TestExchangeNativeCancellationWhileReadingSkipsCountFallback cancels while
// reading the last retryable HTTP response. It checks that an interrupted error
// body cannot be replaced with a successful token estimate.
func TestExchangeNativeCancellationWhileReadingSkipsCountFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options := cancellationExchangeOptions()
	options.MaxRetries = 1
	options.CountFallback = true
	options.Do = func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture"))}, nil
	}
	options.ReadError = func(*http.Response) []byte {
		cancel()
		return nil
	}
	options.EstimateCount = func() int {
		t.Error("canceled error read reached token estimate")
		return 1
	}
	result, err := ExchangeNative(ctx, options)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result.EstimatedTokens)
}
