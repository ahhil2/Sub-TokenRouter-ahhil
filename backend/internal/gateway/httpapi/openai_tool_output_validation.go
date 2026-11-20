package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// openai_tool_output_validation.go validates HTTP tool-output context before
// Responses execution. Tool results need an association with their originating
// calls, and the protocol helper reports which supporting input is present.
// Requests containing no function-call output pass through this check, leaving
// other request validation to the entry handler.
//
// A nonblank previous response ID or existing tool-call context admits the
// request before standalone-output checks. Server-executed tool search results
// carry hosted history; the protocol helper excludes them from client call-ID
// requirements. Remaining outputs require call IDs first, then item references
// covering every call ID. Missing IDs and missing references produce distinct
// diagnostic reasons and HTTP 400 messages, so callers can identify which
// association information the request lacks.
// Missing-ID warnings include bounded structural observations and a bootstrap
// recheck of the rejected body. Diagnostic values use fixed labels and counts
// so request text and identifiers remain outside the log. Validation leaves
// the supplied body intact for subsequent execution.

// ValidateOpenAIFunctionCallOutput reports whether HTTP tool outputs pass its
// call-context checks. Requests without function_call_output pass immediately.
// A nonblank previous_response_id or explicit tool-call context also passes;
// remaining client outputs need call IDs and matching item_reference entries.
// Hosted tool search results do not require a client reply identifier, while
// invalid client outputs are logged and written as HTTP 400. Structural
// diagnostics are built only for missing-ID warnings accepted by the logger.
//
// @project-doc docs/interfaces/openai_upstream.md#openai_tool_identity
func ValidateOpenAIFunctionCallOutput(c *gin.Context, body []byte, reqLog *zap.Logger) bool {
	if !gjson.GetBytes(body, `input.#(type=="function_call_output")`).Exists() {
		return true
	}

	validation := openai.ValidateFunctionCallOutputContextBytes(body)
	if !validation.HasFunctionCallOutput {
		return true
	}

	previousResponseID := gjson.GetBytes(body, "previous_response_id").String()
	// Continuation or inline tool-call context short-circuits standalone
	// output checks. Keep the supplied input intact for subsequent upstream
	// handling while skipping the later call-ID and item-reference guards.
	if strings.TrimSpace(previousResponseID) != "" || validation.HasToolCallContext {
		return true
	}

	if validation.HasFunctionCallOutputMissingCallID {
		// Inspect structure only for an emitted failure warning; successful requests
		// and disabled warnings do not incur the diagnostic parsing or bootstrap checks.
		if entry := reqLog.Check(zap.WarnLevel, "openai.request_validation_failed"); entry != nil {
			entry.Write(
				zap.String("reason", "function_call_output_missing_call_id"),
				zap.Any("tool_output_diagnostic", requeststate.DescribeOpenAIToolOutputFailure(body)),
			)
		}
		DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", "Client tool outputs require a non-empty call_id")
		return false
	}
	if validation.HasItemReferenceForAllCallIDs {
		return true
	}

	reqLog.Warn("openai.request_validation_failed",
		zap.String("reason", "function_call_output_missing_item_reference"),
	)
	DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", "Client tool outputs require tool-call context, item_reference entries matching each call_id, or a previous_response_id")
	return false
}
