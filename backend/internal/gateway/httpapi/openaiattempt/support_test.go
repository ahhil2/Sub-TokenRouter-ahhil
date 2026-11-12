package openaiattempt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// support_test.go verifies shared attempt diagnostics and cancellation across
// compatible text entries that dispatch through Chat or Responses independently
// of their inbound URL. The fixtures check result/context/default endpoint
// precedence, request cancellation and explicit post-write recovery permission,
// and proxy observations that expose selected connection metadata without
// credential fields.

// TestOtherFailureCanceledSkipsProviderFeedback ends requests before or after
// downstream commitment. Each text protocol returns before invoking any bound
// provider feedback, warning, or fallback dependency, preserving the selected
// wire status and emitting one informational cancellation observation.
func TestOtherFailureCanceledSkipsProviderFeedback(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, committed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", protocol, committed), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
				if committed {
					c.Status(http.StatusOK)
					_, err := c.Writer.WriteString(": connected\n\n")
					require.NoError(t, err)
				}
				core, logs := observer.New(zap.InfoLevel)
				base := responsesAttemptBridge{
					c: c, reqLog: zap.New(core), streamStarted: &committed,
					provider: &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 31}},
				}
				cancel()
				// Unbound execution callbacks fail if cancellation reaches provider
				// feedback or output, so successful return proves those side effects were bypassed.
				require.NotPanics(t, func() {
					err := fmt.Errorf("stream read error: %w", context.Canceled)
					switch protocol {
					case "chat":
						bridge := openAIChatAttemptBridge{responsesAttemptBridge: base}
						bridge.OtherFailure(err)
					case "responses":
						base.OtherFailure(err)
						base.Failed()
					case "messages":
						bridge := openAIMessageAttemptBridge{responsesAttemptBridge: base}
						bridge.OtherFailure(err)
					}
				})
				wantStatus := gatewayhttp.StatusClientClosedRequest
				if committed {
					wantStatus = http.StatusOK
				}
				assert.Equal(t, wantStatus, c.Writer.Status())
				assert.NotContains(t, recorder.Body.String(), "error")
				require.Len(t, logs.All(), 1)
				assert.Equal(t, zap.InfoLevel, logs.All()[0].Level)
				assert.Contains(t, logs.All()[0].Message, "client_disconnected")
			})
		}
	}
}

// TestResolveOpenAIUpstreamEndpointPrefersForwardResult varies inbound,
// recorded, and returned endpoint evidence across providers. It checks the
// result wins, then attempt context, then the selected platform's default for
// compatible execution.
func TestResolveOpenAIUpstreamEndpointPrefersForwardResult(t *testing.T) {
	tests := []struct {
		name            string                             // endpoint precedence scenario label
		provider        *gatewayprovider.ExecutionProvider // selected provider platform fixture
		result          *forwardcore.OpenAIResult          // returned execution endpoint evidence
		inboundEndpoint string                             // incoming client endpoint path
		runtimeEndpoint string                             // recorded actual attempt endpoint
		want            string                             // expected resolved upstream endpoint
	}{
		{
			name:            "grok raw chat result overrides stale context",
			provider:        &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformGrok, Type: capability.ProviderTypeOAuth}},
			result:          &forwardcore.OpenAIResult{UpstreamEndpoint: gatewayhttp.EndpointChatCompletions},
			runtimeEndpoint: gatewayhttp.EndpointResponses,
			want:            gatewayhttp.EndpointChatCompletions,
		},
		{
			name:     "grok chat bridged to responses",
			provider: &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformGrok, Type: capability.ProviderTypeOAuth}},
			result:   &forwardcore.OpenAIResult{UpstreamEndpoint: gatewayhttp.EndpointResponses},
			want:     gatewayhttp.EndpointResponses,
		},
		{
			name:     "grok empty result keeps responses default",
			provider: &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformGrok, Type: capability.ProviderTypeOAuth}},
			result:   &forwardcore.OpenAIResult{},
			want:     gatewayhttp.EndpointResponses,
		},
		{
			name:            "grok raw error uses runtime endpoint",
			provider:        &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformGrok, Type: capability.ProviderTypeOAuth}},
			runtimeEndpoint: gatewayhttp.EndpointChatCompletions,
			want:            gatewayhttp.EndpointChatCompletions,
		},
		{
			name:     "openai behavior remains responses",
			provider: &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth}},
			result:   &forwardcore.OpenAIResult{},
			want:     gatewayhttp.EndpointResponses,
		},
		{
			name:            "openai api key chat attempt records runtime endpoint",
			provider:        &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey}},
			result:          &forwardcore.OpenAIResult{},
			runtimeEndpoint: gatewayhttp.EndpointChatCompletions,
			want:            gatewayhttp.EndpointChatCompletions,
		},
		{
			name: "openai api key responses attempt records runtime endpoint",
			provider: &gatewayprovider.ExecutionProvider{
				Record: providercore.Record{
					LoadLocation: time.LoadLocation, Platform: capability.PlatformOpenAI,
					Type:  capability.ProviderTypeAPIKey,
					Extra: map[string]any{"openai_text_route_mode": "force_responses"},
				},
			},
			result:          &forwardcore.OpenAIResult{},
			runtimeEndpoint: gatewayhttp.EndpointResponses,
			want:            gatewayhttp.EndpointResponses,
		},
		{
			name:            "responses fallback records runtime chat endpoint",
			provider:        &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey}},
			result:          &forwardcore.OpenAIResult{},
			inboundEndpoint: gatewayhttp.EndpointResponses,
			runtimeEndpoint: gatewayhttp.EndpointChatCompletions,
			want:            gatewayhttp.EndpointChatCompletions,
		},
		{
			name:            "messages native path records runtime responses endpoint",
			provider:        &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey}},
			result:          &forwardcore.OpenAIResult{},
			inboundEndpoint: gatewayhttp.EndpointMessages,
			runtimeEndpoint: gatewayhttp.EndpointResponses,
			want:            gatewayhttp.EndpointResponses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			inboundEndpoint := tt.inboundEndpoint
			if inboundEndpoint == "" {
				inboundEndpoint = gatewayhttp.EndpointChatCompletions
			}
			c.Request = httptest.NewRequest(http.MethodPost, inboundEndpoint, nil)
			c.Set("_gateway_inbound_endpoint", inboundEndpoint)
			gatewayhttp.SetActualOpenAIUpstreamEndpoint(c, tt.runtimeEndpoint)
			require.Equal(t, tt.want, ResolveOpenAIUpstreamEndpoint(c, tt.provider, tt.result))
		})
	}
}

// TestOpenAIForwardMayFailoverOnlyAfterNonSemanticWrite writes and flushes an
// SSE comment before checking recovery. It verifies the explicit
// SafeToFailoverAfterWrite flag permits replay after changed output size, while
// its absence rejects that path.
func TestOpenAIForwardMayFailoverOnlyAfterNonSemanticWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	before := gatewayhttp.OpenAICompactKeepaliveAdjustedWrittenSize(c)

	_, err := fmt.Fprint(c.Writer, ":\n\n")
	require.NoError(t, err)
	c.Writer.Flush()

	require.True(t, OpenAIForwardMayFailover(c, before, &forwardcore.UpstreamFailoverError{
		SafeToFailoverAfterWrite: true,
	}))
	require.False(t, OpenAIForwardMayFailover(c, before, &forwardcore.UpstreamFailoverError{}))
}

// TestOpenAIRequestAllowsFailoverReplayStopsCanceledClient supplies missing and
// active request contexts, then cancels the active one. It checks missing input
// rejects replay and cancellation changes the active request from allowed to
// denied.
func TestOpenAIRequestAllowsFailoverReplayStopsCanceledClient(t *testing.T) {
	require.False(t, OpenAIRequestAllowsFailoverReplay(nil))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestCtx)

	require.True(t, OpenAIRequestAllowsFailoverReplay(c))
	// End the request before another replay check, isolating cancellation
	// from output-size evidence so no disconnected client can authorize an
	// additional upstream attempt.
	cancel()
	require.False(t, OpenAIRequestAllowsFailoverReplay(c))
}

// TestAppendOpenAIProviderProxyLogFields logs a provider with a loaded
// credentialed proxy. It checks selected ID/name/host/port are present and
// username, password, and URL fields are absent from the captured observation.
func TestAppendOpenAIProviderProxyLogFields(t *testing.T) {
	proxyID := int64(17)
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation, ProxyID: &proxyID,
			Proxy: &egress.Proxy{
				ID:       proxyID,
				Name:     "openai-egress",
				Host:     "proxy.example.com",
				Port:     8443,
				Username: "proxy-user-secret",
				Password: "proxy-password-secret",
			},
		},
	}
	core, logs := observer.New(zap.WarnLevel)
	log := zap.New(core)

	log.Warn("openai.websocket_proxy_failed", AppendOpenAIProviderProxyLogFields(nil, provider)...)

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.EqualValues(t, proxyID, fields["proxy_id"])
	require.Equal(t, "openai-egress", fields["proxy_name"])
	require.Equal(t, "proxy.example.com", fields["proxy_host"])
	require.EqualValues(t, 8443, fields["proxy_port"])
	require.NotContains(t, fields, "proxy_username")
	require.NotContains(t, fields, "proxy_password")
	require.NotContains(t, fields, "proxy_url")
}

// TestAppendOpenAIProviderProxyLogFields_FallsBackToProxyID supplies a provider
// with only its proxy reference loaded. It checks the log contains that
// identifier as the sole proxy field for the partial record.
func TestAppendOpenAIProviderProxyLogFields_FallsBackToProxyID(t *testing.T) {
	proxyID := int64(23)
	core, logs := observer.New(zap.WarnLevel)
	log := zap.New(core)

	log.Warn("openai.websocket_proxy_failed", AppendOpenAIProviderProxyLogFields(nil, &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ProxyID: &proxyID}})...)

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.EqualValues(t, proxyID, fields["proxy_id"])
	require.Len(t, fields, 1)
}
