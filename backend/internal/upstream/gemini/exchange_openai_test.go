package gemini

import "testing"

// exchange_openai_test.go verifies cancellation at request construction, transport, and
// backoff for the OpenAI exchange. The shared fixtures also keep independent
// upstream deadlines retryable, covering the distinction between request
// termination and a provider that timed out while its client remains active.

// TestExchangeOpenAICancellation covers canceled requests before and during
// dispatch or backoff. It checks error identity, stopped attempts, response
// cleanup, and retry eligibility for independent upstream deadlines.
func TestExchangeOpenAICancellation(t *testing.T) {
	testExchangeCancellation(t, ExchangeOpenAI)
}
