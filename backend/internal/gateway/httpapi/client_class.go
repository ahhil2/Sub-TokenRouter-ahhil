package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/gateway/clientmeta"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
)

// CaptureClientClass 在认证、协议转换和上游 Header 改写前保存请求来源。
func CaptureClientClass(context *gin.Context) {
	hints := requeststate.ExecutionHintsFromContext(context.Request.Context())
	hints.CodexClient = requeststate.Hint[bool]{
		Value: clientmeta.IsCodexRoutingClient(context.GetHeader("User-Agent"), context.GetHeader("Originator")),
		Set:   true,
	}
	context.Request = context.Request.WithContext(requeststate.WithExecutionHints(context.Request.Context(), hints))
	context.Next()
}
