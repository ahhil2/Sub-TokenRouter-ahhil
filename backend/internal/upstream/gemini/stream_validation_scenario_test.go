package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// stream_validation_scenario_test.go covers execute.go and its buffered exchanges
// in exchange_native.go, exchange_messages.go, and exchange_openai.go when the
// provider returns SSE. An incomplete generation keeps its observed accounting
// and closes the transport, while protocol conversion waits for validated
// completion before publishing a successful JSON response.

// TestExecuteBufferedGeminiRejectsIncompleteSSE truncates an upstream response
// after content and sparse usage. Every buffered protocol returns partial usage
// with the read failure and leaves its downstream body empty.
func TestExecuteBufferedGeminiRejectsIncompleteSSE(t *testing.T) {
	for _, kind := range []string{"native", "messages", "chat", "responses"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":2}}\n\n")
				_, _ = io.WriteString(w, "data: {\"usageMetadata\":{\"candidatesTokenCount\":4}}\n\n")
			}))
			defer server.Close()
			mode := MessagesResponse
			switch kind {
			case "native":
				mode = NativeResponse
			case "chat", "responses":
				mode = OpenAIResponse
			}
			var closes atomic.Int32
			target := localTarget(server, mode, false, &closes)
			target.UpstreamStream = true
			if kind == "responses" {
				target.OpenAIProtocol = OpenAICompatResponses
			}
			sink := &executeSink{}
			result, err := (Executor{}).Execute(context.Background(), nativeInput(target, false), sink)
			var failure *upstream.GeminiStreamError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "stream_incomplete", failure.Kind)
			require.True(t, result.HasUsage)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Empty(t, sink.body.String())
			require.EqualValues(t, 1, closes.Load())
		})
	}
}
