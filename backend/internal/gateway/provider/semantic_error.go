package provider

import (
	"context"
	"net/http"

	"github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/anthropic"
)

func CheckAnthropicSemanticError(ctx context.Context, health *provideradapter.UpstreamHealth, target *ExecutionProvider, response *http.Response) error {
	if target == nil {
		return nil
	}
	policy := target.Record.SemanticErrorPolicy()
	if !policy.Enabled {
		return nil
	}
	matched, err := anthropic.DetectSemanticError(ctx, response, policy.Texts)
	if err != nil {
		return err
	}
	if !matched {
		return nil
	}
	body := []byte(`{"type":"error","error":{"type":"upstream_semantic_error","message":"Upstream returned a configured error response"}}`)
	if health != nil && health.Core != nil {
		health.Core.TriggerTempUnschedulable(ctx, ExecutionRecord(target), providercore.TempUnschedulableRule{DurationMinutes: policy.CooldownMinutes}, -1, response.StatusCode, "upstream_semantic_error", body)
	}
	return &forward.UpstreamFailoverError{
		StatusCode: http.StatusServiceUnavailable, ResponseBody: body, ResponseHeaders: response.Header.Clone(),
		Reason: "upstream_semantic_error", Scope: forward.GatewayFailureScopeProvider,
		NextProviderAction: forward.NextProviderRetry,
	}
}
