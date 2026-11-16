package googleforward_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/googleforward"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// gemini_stream_scenario_test.go exercises failed HTTP-200 generations
// through gemini_attempts.go, gemini_response.go, gemini_openai.go, and the real
// HTTP error output. Each client protocol
// must receive one framed failure while completion retains the usage received
// before truncation, malformed data, or an embedded provider error.

// TestGeminiFailedStreamRetainsUsageAndProtocolError supplies sparse usage and
// content before three failure forms. All client protocols retain partial usage,
// avoid successful completion, and reject failover after content reaches the client.
func TestGeminiFailedStreamRetainsUsageAndProtocolError(t *testing.T) {
	for _, protocol := range []string{"native", "messages", "chat", "responses"} {
		for _, failure := range []string{"eof", "malformed", "provider"} {
			t.Run(protocol+"/"+failure, func(t *testing.T) {
				payload := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"visible\"}]}}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":2}}\n\n" +
					"data: {\"usageMetadata\":{\"candidatesTokenCount\":4}}\n\n"
				switch failure {
				case "malformed":
					payload += "data: {broken\n\n"
				case "provider":
					payload += "data: {\"error\":{\"code\":503,\"message\":\"provider unavailable\",\"status\":\"UNAVAILABLE\"}}\n\n"
				}
				transport := &geminiCompatHTTPUpstreamStub{response: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(payload)),
				}}
				svc := newGeminiFixture(geminiDependencies{httpUpstream: transport, cfg: &googleforward.Options{}})
				provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
					LoadLocation: time.LoadLocation, ID: 415, Platform: capability.PlatformGemini,
					Type: capability.ProviderTypeAPIKey, Concurrency: 1,
					Credentials: map[string]any{"api_key": "fixture-key"},
				}}
				body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hello"}],"max_tokens":64,"stream":true}`)
				path := "/v1/messages"
				switch protocol {
				case "responses":
					body = []byte(`{"model":"gemini-2.5-flash","input":"hello","stream":true}`)
					path = "/v1/responses"
				case "native":
					body = []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
					path = "/v1beta/models/gemini-2.5-flash:streamGenerateContent"
				case "chat":
					path = "/v1/chat/completions"
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				output := gatewayhttp.NewGoogleBoundary(c, svc.Options, false)
				var result *forwardcore.MessagesResult
				var err error
				switch protocol {
				case "native":
					result, err = svc.ForwardNative(context.Background(), output, provider, "gemini-2.5-flash", "streamGenerateContent", true, body)
				case "messages":
					result, err = svc.Forward(context.Background(), output, provider, body)
				case "chat":
					result, err = svc.ForwardAsChatCompletions(context.Background(), output, provider, body)
				case "responses":
					result, err = svc.ForwardAsResponses(context.Background(), output, provider, body, nil)
				}
				var streamError *upstream.GeminiStreamError
				require.ErrorAs(t, err, &streamError)
				var failover *forwardcore.UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.NotNil(t, result)
				require.Equal(t, 7, result.Usage.InputTokens)
				require.Equal(t, 4, result.Usage.OutputTokens)
				require.False(t, result.ClientDisconnect)
				require.Equal(t, http.StatusOK, recorder.Code)
				wire := recorder.Body.String()
				require.Contains(t, wire, "visible")
				require.NotContains(t, wire, "event: message_stop")
				require.NotContains(t, wire, "event: response.completed")
				require.NotContains(t, wire, "data: [DONE]")
				errorsSeen := 0
				for line := range strings.SplitSeq(wire, "\n") {
					if value, found := strings.CutPrefix(line, "data: "); found {
						require.True(t, gjson.Valid(value), value)
						if gjson.Get(value, "error").IsObject() || gjson.Get(value, "response.error").IsObject() {
							errorsSeen++
						}
					}
				}
				require.Equal(t, 1, errorsSeen, wire)
				before := recorder.Body.String()
				// Outer error handling can encounter the same failure after forwarding.
				// Its presentation callback must leave the already-delivered terminal alone.
				_ = output.GeminiOutput.GoogleError(http.StatusBadGateway, "duplicate error")
				require.Equal(t, before, recorder.Body.String())
			})
		}
	}
}
