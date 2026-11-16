package googleforward

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// gemini_stream.go applies existing provider policy to failed Gemini streams.
// A successful HTTP status does not establish a successful generation: the
// validated reader can discover an embedded provider error, an incomplete body,
// or an idle timeout. Those failures retain their status and partial accounting
// while selecting the same configured retry and error rules as ordinary HTTP
// failures.
//
// Presentation is supplied by the selected client protocol. Failover is returned
// only while the output sink reports no committed output; otherwise the
// same failure ends the existing response. Cancellation and downstream write
// errors bypass provider policy, preserving their original classification and
// avoiding a second response to a client that has already disconnected.

// streamFailure reconciles a classified stream failure with provider policy and output.
// The callback presents one final protocol error after retries are no longer
// eligible. Unclassified errors and cancellation return unchanged for their existing owner.
//
// A live sink supplies commitment without accessing mutable response headers.
// Unknown sinks conservatively prohibit retry. Provider policy consumes copied
// headers, leaving the successful transport metadata and executor observations
// available to completion accounting.
// Final presentation retains the stream error in its returned error chain;
// eligible failover returns the existing retry error type to orchestration.
func (s *Gemini) streamFailure(ctx context.Context, c *attempt, provider *gatewayprovider.ExecutionProvider, result upstream.AttemptResult, failure error, present func(int, []byte) error) error {
	var streamError *upstream.GeminiStreamError
	if !errors.As(failure, &streamError) || ctx.Err() != nil || result.ClientDisconnect {
		return failure
	}
	body := streamError.Body
	if len(body) == 0 {
		body, _ = json.Marshal(map[string]any{"error": map[string]any{"code": streamError.Status, "message": streamError.Message}})
	}
	decision := s.applyGeminiUpstreamErrorPolicy(ctx, provider, streamError.Status, result.UpstreamHeaders.Clone(), body, result.UpstreamModel)
	skipped := decision.Policy == providercore.ErrorPolicyCustomSkipped || decision.Policy == providercore.ErrorPolicyPoolBypassed
	defaultFailover := s.shouldFailoverGeminiUpstreamError(streamError.Status)
	message := strings.ToLower(strings.TrimSpace(upstream.ExtractErrorMessage(body)))
	googleConfigError := streamError.Status == http.StatusBadRequest && upstream.IsGoogleProjectConfigError(message)
	shouldFailover := decision.ShouldFailover(gatewayprovider.ExecutionErrorPolicy(provider), streamError.Status, defaultFailover || googleConfigError)
	retryable := decision.RetryableOnSameProvider(gatewayprovider.ExecutionErrorPolicy(provider), streamError.Status)
	if skipped {
		// Skipped policies retain ordinary status failover, and same-provider replay
		// follows the pool settings already used by ordinary Gemini HTTP failures.
		shouldFailover = defaultFailover
		retryable = provider.View().IsPoolMode() && provider.View().IsPoolModeRetryableStatus(streamError.Status)
	}
	committed := true
	if state, ok := c.Sink().(interface {
		// OutputState reports current delivery without taking response-header ownership.
		OutputState() upstream.OutputHead
	}); ok {
		committed = state.OutputState().Committed
	}
	if shouldFailover && !committed {
		// Reuse configured retry policy before the response writer commits bytes.
		// Writers with unknown delivery state conservatively keep the current attempt.
		c.Observe(ops.OpsUpstreamErrorEvent{
			Platform: provider.Record.Platform, ProviderID: provider.Record.ID, ProviderName: provider.Record.Name,
			UpstreamStatusCode: streamError.Status, UpstreamRequestID: result.RequestID,
			Kind: "failover", Message: logredact.SanitizeUpstreamQueries(streamError.Message),
		})
		return &forwardcore.UpstreamFailoverError{StatusCode: streamError.Status, ResponseBody: body, RetryableOnSameProvider: retryable}
	}
	if decision.Policy == providercore.ErrorPolicyCustomSkipped {
		presented := c.GeminiCustomCodeSkippedError(provider, streamError.Status, result.RequestID, body, func() {
			fallback, _ := json.Marshal(map[string]any{"error": map[string]any{"code": http.StatusInternalServerError, "message": geminiCustomCodeSkippedClientMessage}})
			_ = present(http.StatusInternalServerError, fallback)
		})
		return errors.Join(failure, presented)
	}
	return errors.Join(failure, present(streamError.Status, body))
}
