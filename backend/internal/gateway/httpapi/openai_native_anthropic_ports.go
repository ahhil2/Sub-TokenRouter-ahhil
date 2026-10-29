package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	protocolforward "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/modelidentity"
	forward "github.com/TokenFlux/TokenRouter/internal/gateway/provider/openaiforward"
	"github.com/TokenFlux/TokenRouter/internal/gateway/searchtools"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	protocolanthropic "github.com/TokenFlux/TokenRouter/internal/protocol/anthropic"
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/anthropic"
)

// openai_native_anthropic_ports.go binds native Anthropic execution to
// client-specific conversions and HTTP output.
// Native Anthropic execution uses the selected provider's protocol key, model
// policy and proxy while the client may speak Messages, Chat or Responses. The
// adapter's kind selects client errors and output conversion, and shared
// attempt state supplies credentials, request observations and synchronous
// output until the runner closes the upstream response.

// openAINativeAnthropicAdapter binds native Anthropic exchange to shared text
// attempt services.
type openAINativeAnthropicAdapter struct {
	// openAIMessagesExecutionAdapter shares this attempt's provider, services and
	// HTTP state; the native runner borrows them until response handling ends.
	*openAIMessagesExecutionAdapter
	// kind selects the client error and conversion format. The native entry
	// constructor supplies Messages, Chat or Responses for the current call.
	kind forward.NativeAnthropicKind
}

func (adapter *openAINativeAnthropicAdapter) errorWriter() func(*gin.Context, int, string, string) {
	switch adapter.kind {
	case forward.NativeResponses:
		return writeTextResponsesError
	case forward.NativeChat:
		return WriteForwardChatError
	default:
		return WriteForwardAnthropicError
	}
}

func (adapter *openAINativeAnthropicAdapter) Profile() forward.MessagesProfile {
	return forward.MessagesProfile{Profile: openAIForwardProfile(adapter.provider), ID: adapter.provider.Record.ID}
}

func (adapter *openAINativeAnthropicAdapter) Error(status int, kind, message string) {
	adapter.errorWriter()(adapter.c, status, kind, message)
}

// PrepareModel delegates attribute-sensitive body preparation for the
// Anthropic Messages protocol. The selected model and provider guide the
// shared attribute service; preparation errors return before the native runner
// constructs or sends its request.
func (adapter *openAINativeAnthropicAdapter) PrepareModel(ctx context.Context, body []byte, model string) ([]byte, error) {
	return gatewayprovider.PrepareModelAttributeParameters(ctx, adapter.provider, model, body, adapter.s.modelAttributes(), "anthropic_messages")
}

func (adapter *openAINativeAnthropicAdapter) ThinkingFallback(effort *string, body []byte, model string) *string {
	return gatewayprovider.ApplyThinkingEnabledFallback(effort, body, model)
}

func (adapter *openAINativeAnthropicAdapter) StripEmpty(body []byte) []byte {
	return protocolanthropic.StripEmptyTextBlocks(body)
}

func (adapter *openAINativeAnthropicAdapter) FilterSearch(body []byte, model string) []byte {
	return searchtools.FilterWebSearchHistoryBlocks(body, modelidentity.ResolveThinkingProtocol(model) == modelidentity.ThinkingProtocolPassbackRequired)
}

func (adapter *openAINativeAnthropicAdapter) CacheLimit(body []byte) []byte {
	return anthropic.EnforceCacheControlLimit(body)
}

func (adapter *openAINativeAnthropicAdapter) Log(format string, args ...any) {
	logging.LegacyPrintf("service.gateway", format, args...)
}

func (adapter *openAINativeAnthropicAdapter) ProtocolAPIKey() string {
	return adapter.provider.View().GetOpenAIProtocolAPIKey()
}

func (adapter *openAINativeAnthropicAdapter) TargetURL() (string, error) {
	return adapter.s.Requests.AnthropicURL(adapter.provider)
}

func (adapter *openAINativeAnthropicAdapter) StreamContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	return gatewayprovider.DetachStreamUpstreamContext(ctx, stream)
}

func (adapter *openAINativeAnthropicAdapter) BuildNative(ctx context.Context, body []byte, key, url string) (*http.Request, error) {
	r, _, err := adapter.s.Requests.BuildAnthropic(ctx, adapter.c, adapter.provider, body, key, url)
	return r, err
}

func (adapter *openAINativeAnthropicAdapter) SendNative(r *http.Request) (*http.Response, error) {
	resetResponseModel(adapter.c)
	return adapter.s.Requests.Transport.Do(r, adapter.proxyURL, adapter.provider.Record.ID, adapter.provider.Record.Concurrency)
}

func (adapter *openAINativeAnthropicAdapter) TransportErrorNative(ctx context.Context, err error) error {
	return adapter.s.Requests.Failure.Handle(ctx, adapter.c, adapter.provider, err, true)
}

func (adapter *openAINativeAnthropicAdapter) InspectResponse(ctx context.Context, response *http.Response) error {
	return gatewayprovider.CheckAnthropicSemanticError(ctx, adapter.s.Output.Observer, adapter.provider, response)
}

func (adapter *openAINativeAnthropicAdapter) DirectOptions() forward.NativeAnthropicOptions {
	return adapter.s.Output.AnthropicDirectOptions(adapter.c, adapter.provider)
}

func (adapter *openAINativeAnthropicAdapter) AdaptResponsesTools(body []byte) ([]byte, bridge.ResponsesClientToolMapping, error) {
	return protocolforward.AdaptResponsesClientToolsForAnthropic(body)
}

func (adapter *openAINativeAnthropicAdapter) ResponsesToAnthropic(r *protocolopenai.ResponsesRequest) (*protocolanthropic.AnthropicRequest, error) {
	return bridge.ResponsesToAnthropicRequest(r)
}

func (adapter *openAINativeAnthropicAdapter) ChatToResponses(r *protocolopenai.ChatCompletionsRequest) (*protocolopenai.ResponsesRequest, error) {
	return bridge.ChatCompletionsToResponses(r, protocolforward.ConversionOptionsForModel(r.Model))
}

func (adapter *openAINativeAnthropicAdapter) ResponsesEffort(body []byte, models ...string) *string {
	return protocolforward.ExtractEffort(body, false, capability.NormalizeRecordedOpenAIEffortForModel, models...)
}

func (adapter *openAINativeAnthropicAdapter) ChatEffort(body []byte, models ...string) *string {
	return protocolforward.ExtractEffort(body, true, capability.NormalizeRecordedOpenAIEffortForModel, models...)
}

func (adapter *openAINativeAnthropicAdapter) MapStatus(status int) int {
	return protocolforward.MapStatus(status)
}

func (adapter *openAINativeAnthropicAdapter) OutputOptions() forward.AnthropicOutputOptions {
	return adapter.s.Output.AnthropicOptions(adapter.c, adapter.errorWriter())
}

// writeTextResponsesError writes a Responses conversion error and marks
// terminal output committed. It delegates formatting to
// ForwardConversionOutput with Responses enabled, so outer attempt handling
// sees the dedicated protocol rejection before further fallback decisions.
func writeTextResponsesError(c *gin.Context, status int, code, message string) {
	output := ForwardConversionOutput{Context: c, Responses: true, Commit: func() { MarkResponseCommitted(c) }}
	output.Error(status, code, message)
}
