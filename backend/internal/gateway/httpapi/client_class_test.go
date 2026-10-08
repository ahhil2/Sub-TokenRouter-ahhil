package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
)

func TestCaptureClientClassBeforeHeaderRewrite(t *testing.T) {
	for _, original := range []struct {
		name, userAgent, originator string
		codex                       bool
	}{
		{"missing", "", "", false},
		{"third party", "curl/8.0", "", false},
		{"codex user agent", "codex_cli_rs/1.0", "", true},
		{"desktop originator", "Mozilla/5.0", "codex_work_desktop", true},
	} {
		t.Run(original.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CaptureClientClass)
			router.GET("/responses", func(context *gin.Context) {
				context.Request.Header.Set("User-Agent", "codex_cli_rs/rewritten")
				context.Request.Header.Set("Originator", "codex-tui")
				class := requeststate.ExecutionHintsFromContext(context.Request.Context()).CodexClient
				assert.True(t, class.Set)
				assert.Equal(t, original.codex, class.Value)
				context.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/responses", nil)
			request.Header.Set("User-Agent", original.userAgent)
			request.Header.Set("Originator", original.originator)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusNoContent, response.Code)
		})
	}
}
