package httpx

import (
	"context"
	"errors"
	"maps"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// error.go maps application errors and HTTP status codes to response
// representations.
// Wrapped application errors carry the numeric response code, reason, and
// message selected by their owner. Mapping extracts that same entity through
// wrappers and copies its string metadata for the response, while unrecognized
// errors become the generic internal-error body supplied by apperror. Requests
// canceled during admission or body reading terminate with client-closed status
// before those failures can be presented as internal errors.

// ToHTTP returns the numeric code and serializable application-error status.
// Nil input or a nil ApplicationError found through wrappers returns HTTP 200.
// Unrecognized errors use the generic 500 message; recognized codes pass through
// unchanged, and the returned metadata map can be edited independently.
// @project-doc docs/interfaces/http_api.md#response_errors
func ToHTTP(err error) (statusCode int, body apperror.Status) {
	if err == nil {
		return http.StatusOK, apperror.Status{Code: int32(http.StatusOK)}
	}

	appErr := apperror.FromError(err)
	if appErr == nil {
		return http.StatusOK, apperror.Status{Code: int32(http.StatusOK)}
	}

	body = apperror.Status{
		Code:    appErr.Code,
		Reason:  appErr.Reason,
		Message: appErr.Message,
	}
	if appErr.Metadata != nil {
		body.Metadata = make(map[string]string, len(appErr.Metadata))
		maps.Copy(body.Metadata, appErr.Metadata)
	}
	return int(appErr.Code), body
}

// ErrorCode returns the numeric application category through error wrappers.
// Nil interfaces yield 200, unknown errors yield 500, and custom codes pass
// through. Callers provide a true nil interface for no error; CategoryOf
// dereferences a typed nil ApplicationError found in a nonnil error chain.
func ErrorCode(err error) int {
	return int(apperror.CategoryOf(err))
}

// HTTPStatusToGoogleStatus maps named HTTP failures to their Google status names.
// Unlisted statuses at or above 500 use INTERNAL, including custom server codes;
// other unlisted statuses use UNKNOWN for the caller's Google error body.
func HTTPStatusToGoogleStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	default:
		// A custom code such as 529 still selects the server-error category here,
		// keeping its returned status consistent with other failures in that numeric range.
		if status >= 500 {
			return "INTERNAL"
		}
		return "UNKNOWN"
	}
}

// AbortCanceledRequest ends an already canceled HTTP request without an error body.
// A canceled context or wrapped context.Canceled error selects 499 while output
// is uncommitted. Existing response status survives; unrelated errors on live
// requests remain available to their caller's normal error handling.
func AbortCanceledRequest(c *gin.Context, err error) bool {
	if c == nil || c.Request == nil || (c.Request.Context().Err() == nil && !errors.Is(err, context.Canceled)) {
		return false
	}
	// Cancellation removes the response consumer. Retain committed framing and
	// record client closure instead of manufacturing a server or input-validation failure.
	if !c.Writer.Written() {
		c.Status(499)
	}
	c.Abort()
	return true
}
