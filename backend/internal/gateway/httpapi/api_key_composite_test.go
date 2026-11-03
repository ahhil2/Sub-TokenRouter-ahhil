package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
)

// SystemOne composite-prefix failures use the decision endpoint's model error
// envelope, including its machine-readable code. Endpoint matching accepts a
// trailing slash while keeping unrelated SystemOne-like paths out of that response choice.

// TestSystemOneCompositeErrorUsesDecisionEndpoint exercises the decision path
// with and without a trailing slash. Both requests must abort with a model
// parameter error and the composite-prefix-required code, while other paths
// retain their own endpoint classification.
func TestSystemOneCompositeErrorUsesDecisionEndpoint(t *testing.T) {
	for _, path := range []string{EndpointSystemOne, EndpointSystemOne + "/"} {
		require.True(t, IsSystemOneEndpoint(path))
		require.False(t, IsOpenAICompositeEndpoint(path))
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		AbortCompositeKeyError(c, apikey.ErrCompositeKeyPrefixRequired)
		require.True(t, c.IsAborted())
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), `"code":"COMPOSITE_KEY_MODEL_PREFIX_REQUIRED"`)
		require.Contains(t, rec.Body.String(), `"param":"model"`)
	}
	require.False(t, IsSystemOneEndpoint("/v1/chat/completions"))
	require.False(t, IsSystemOneEndpoint("/other/systemone"))
	require.True(t, IsOpenAICompositeEndpoint("/v1/chat/completions"))
}

// TestCompositeCanceledBodyStopsWithoutInternalError reproduces cancellation during model extraction.
// Canceled input must abort the handler with client-closed status and an empty body, before any provider is involved.
func TestCompositeCanceledBodyStopsWithoutInternalError(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", io.NopCloser(iotest.ErrReader(context.Canceled))).WithContext(ctx)
	_, err := CompositeModelFromRequest(c.Request)
	require.ErrorIs(t, err, context.Canceled)
	AbortCompositeKeyError(c, err)
	require.Equal(t, StatusClientClosedRequest, c.Writer.Status())
	require.Empty(t, rec.Body.String())
	require.True(t, c.IsAborted())
	t.Logf("canceled request produced status=%d body=%s", rec.Code, rec.Body.String())
}

// TestLocalToolConversionFailureReturnsRequestError rejects conflicting tool definitions through each transport adapter.
// The HTTP fallback retains the local explanation and uses a request-error status before any provider exchange can happen.
func TestLocalToolConversionFailureReturnsRequestError(t *testing.T) {
	body := []byte(`{"model":"test","tools":[{"type":"custom","name":"wait"},{"type":"namespace","name":"functions","tools":[{"type":"function","name":"wait"}]}]}`)
	adapters := map[string]func() error{
		"openai": func() error {
			_, _, err := bridge.AdaptOpenAIResponsesClientTools(body)
			return err
		},
		"json": func() error {
			_, _, err := bridge.AdaptResponsesClientToolsJSON(body, "test")
			return err
		},
		"chat": func() error {
			var request bridge.ResponsesRequest
			require.NoError(t, json.Unmarshal(body, &request))
			_, _, err := bridge.ResponsesToChatCompletionsRequestWithMapping(&request, nil)
			return err
		},
	}
	for name, adapt := range adapters {
		t.Run(name, func(t *testing.T) {
			err := adapt()
			require.Error(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			require.True(t, DefaultOpenAIErrorOutput().EnsureResponse(c, false, err))
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "conflict")
			require.Contains(t, rec.Body.String(), "invalid_request_error")
		})
	}
}
