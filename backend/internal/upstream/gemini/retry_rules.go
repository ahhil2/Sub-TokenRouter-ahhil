package gemini

import (
	"context"
	"errors"
	mathrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	googlewire "github.com/TokenFlux/TokenRouter/internal/protocol/google"
)

// retry_rules.go classifies Gemini recovery signals, applies retry backoff,
// and estimates tokens from prompt text.
// Gemini exchanges use retry classification and local token estimates when an
// upstream operation needs recovery. The retry delay adds jitter to the capped
// exponential base, while text estimation reads ordinary prompt text as an
// approximation for countTokens responses when OAuth scopes prevent that
// action.
//
// Request cancellation interrupts waiting and retains its error identity before
// protocol-specific error mapping. Parent deadlines use the request context
// because the transport can time out independently while the client remains
// active. The timer path rechecks cancellation when both signals are ready,
// and exchange callers check again before constructing another request.

const (
	// geminiRetryBaseDelay and geminiRetryMaxDelay bound the exponential delay
	// before per-attempt jitter is applied by SleepGeminiBackoff.
	geminiRetryBaseDelay = time.Second
	geminiRetryMaxDelay  = 16 * time.Second
)

// IsGeminiSignatureRelatedError checks extracted platform error text for a
// signature marker. If extraction yields blank text, it searches the full body
// instead; matching is case-insensitive and includes the broad signature
// substring.
func IsGeminiSignatureRelatedError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(googlewire.ExtractPlatformMessage(respBody)))
	// Use the raw body when structured platform text is missing so legacy or
	// malformed error envelopes can still trigger the signature recovery
	// classifier.
	if msg == "" {
		msg = strings.ToLower(string(respBody))
	}
	return strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature")
}

// SleepGeminiBackoff waits for a positive one-based attempt using the capped
// exponential base and twenty-percent jitter. Cancellation returns ctx.Err
// promptly, including when the timer and cancellation become ready together;
// a completed delay returns nil and permits another attempt.
//
// Exchange callers check the context again before dispatch because cancellation
// can arrive after this function returns. The timer is stopped on every exit,
// and the existing capped jitter calculation determines successful wait length.
func SleepGeminiBackoff(ctx context.Context, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := min(geminiRetryBaseDelay*time.Duration(1<<uint(attempt-1)), geminiRetryMaxDelay)

	// Jitter spreads simultaneous retries around the exponential base. The
	// sixteen-second cap applies before jitter, so the final wait can exceed that
	// base cap.
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(float64(delay) * 0.2 * (r.Float64()*2 - 1))
	sleepFor := max(delay+jitter, 0)
	timer := time.NewTimer(sleepFor)
	defer timer.Stop()
	// Both signals can become ready together. Recheck the context after timer
	// selection so a completed delay cannot authorize work for a canceled request.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

// geminiCancellationError returns request cancellation or a wrapped transport
// cancellation while leaving independent upstream deadlines retryable. A parent
// deadline is identified through ctx.Err because a transport deadline can expire
// while the client request remains active. Wrapped cancellation errors keep their
// identity so the executor can classify them after this exchange returns.
// Independent timeout errors return nil here and reach the ordinary transport
// retry budget and final upstream error presenter.
func geminiCancellationError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// IsGeminiInsufficientScope recognizes OAuth scope rejection in the challenge
// header or response body. Header and body matches are case-insensitive;
// callers supply the HTTP status when deciding whether to substitute a local
// estimate.
func IsGeminiInsufficientScope(headers http.Header, body []byte) bool {
	if strings.Contains(strings.ToLower(headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "insufficient authentication scopes") || strings.Contains(lower, "access_token_scope_insufficient")
}

// EstimateGeminiCountTokens sums approximate counts for camelCase
// systemInstruction and contents text parts. Blank and nontext parts
// contribute zero, and malformed or alternate field shapes can yield partial
// or empty observations; this value is separate from upstream usage.
func EstimateGeminiCountTokens(reqBody []byte) int {
	total := 0

	// Count system text separately from conversation parts because both are
	// prompt inputs for the local scope-rejection response, while image and tool
	// data have no estimate here.
	gjson.GetBytes(reqBody, "systemInstruction.parts").ForEach(func(_, part gjson.Result) bool {
		if t := strings.TrimSpace(part.Get("text").String()); t != "" {
			total += EstimateTokensForText(t)
		}
		return true
	})

	gjson.GetBytes(reqBody, "contents").ForEach(func(_, content gjson.Result) bool {
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			if t := strings.TrimSpace(part.Get("text").String()); t != "" {
				total += EstimateTokensForText(t)
			}
			return true
		})
		return true
	})

	if total < 0 {
		return 0
	}
	return total
}

// EstimateTokensForText returns an approximate token count after trimming the
// text. At least eighty percent ASCII runes selects the rounded-up
// four-runes-per-token estimate; other text uses one token per rune, and blank
// text returns zero.
func EstimateTokensForText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}
	asciiRatio := float64(ascii) / float64(len(runes))
	if asciiRatio >= 0.8 {
		// Use the ASCII-dominant estimate and round upward so a nonempty tail
		// shorter than four runes still contributes one token.
		return (len(runes) + 3) / 4
	}
	// Count each rune when the text crosses the non-ASCII threshold because the
	// four-rune estimate is reserved for ASCII-dominant prompt text.
	return len(runes)
}
