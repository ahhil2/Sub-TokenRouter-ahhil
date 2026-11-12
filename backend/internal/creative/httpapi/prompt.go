package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/creative"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// prompt.go handles authenticated fixed-price prompt expansion requests.
// Requests carry the same browser workspace identity as creative media tasks,
// while a required idempotency key lets interrupted requests recover a temporary
// result through the service's shared billing and cache coordination.

// BindPromptExpansion attaches prompt operations before the handler serves requests.
// Callers finish this binding before publishing the handler; a nil expander
// leaves prompt capabilities disabled and expansion requests unavailable.
func (h *CreativeHandler) BindPromptExpansion(expander *creative.PromptExpander) { h.prompt = expander }

// ExpandPrompt parses bounded JSON and delegates authenticated prompt expansion.
// A valid workspace and idempotency key identify the recoverable operation;
// error responses use the common envelope, and returned text is never cacheable.
func (h *CreativeHandler) ExpandPrompt(c *gin.Context) {
	if h.enter != nil {
		done, err := h.enter()
		if err != nil {
			response.ErrorFrom(c, creative.ErrPromptUnavailable)
			return
		}
		defer done()
	}
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	scope, err := creativeRunScopeFromRequest(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.prompt == nil {
		response.ErrorFrom(c, creative.ErrPromptUnavailable)
		return
	}
	// Twelve bytes per code point admit escaped surrogate pairs in JSON.
	// The additional envelope budget bounds the object body before prompt
	// validation counts its decoded Unicode code points.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(h.prompt.MaxPromptChars())*12+1024)
	var input creative.PromptExpansionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, creative.ErrCreativeInvalidParams)
		return
	}
	result, err := h.prompt.Expand(c.Request.Context(), scope, input, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, result)
}
