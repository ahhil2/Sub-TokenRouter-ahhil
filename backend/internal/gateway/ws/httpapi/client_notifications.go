package httpapi

import (
	"errors"
	"slices"
	"strings"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// client_notifications.go applies Codex notification compatibility to admitted
// WebSocket request frames. The authenticated API-key actor selects the site
// switch or rollout list. Notification names remain format markers, and the
// shared converter labels their content as client-submitted event data.
//
// First-frame admission, pooled normalization, HTTP bridging, and dedicated
// sessions use this operation before interpreting notification items as tool
// results. Expansion obeys both the configured HTTP body limit and the client
// frame limit captured when the connection was accepted. Runtime increases
// cannot enlarge that captured limit. Unrelated frame types retain their
// existing processing, and conversion failures stop forwarding with a typed
// client close instruction.

// responsesWSReadLimitContextKey keeps the accepted connection limit available
// to per-turn adapters after runtime parameters publish a different value.
const responsesWSReadLimitContextKey = "responses_ws_client_read_limit_bytes"

// NormalizeClientNotifications converts supported notification records for
// authenticated API-key actors selected by startup configuration. readLimit is
// the current adapter's client-frame budget; a smaller captured connection
// limit or positive MaxBodyBytes wins when notification text expands.
//
// The converter accepts response.create and the omitted type already supported
// by initial and pooled normalization. Other frame types keep their bytes.
// The private authenticated principal supplies rollout identity, so billing
// ownership and client headers cannot select a different user. Oversized
// converted payloads return status 1009, and other conversion failures return
// status 1008 before any upstream exchange uses the frame.
func (s *OpenAIWebSocketExecutor) NormalizeClientNotifications(c *gin.Context, body []byte, readLimit int64) ([]byte, error) {
	if s == nil || c == nil || (!s.CodexClientNotifications && len(s.CodexClientNotificationUserIDs) == 0) {
		return body, nil
	}
	actor, authenticated := authctx.GetPrincipal(c)
	if !authenticated || actor.CredentialKind != "api_key" || actor.UserID <= 0 ||
		(!s.CodexClientNotifications && !slices.Contains(s.CodexClientNotificationUserIDs, actor.UserID)) {
		return body, nil
	}
	frameType := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	if frameType != "" && frameType != "response.create" {
		return body, nil
	}

	// Retain the handshake's smaller limit because read contexts can outlive the
	// runtime snapshot that supplied it, while conversion happens after socket reads.
	if captured, exists := c.Get(responsesWSReadLimitContextKey); exists {
		if limit, ok := captured.(int64); ok && limit > 0 && (readLimit <= 0 || limit < readLimit) {
			readLimit = limit
		}
	}
	if readLimit <= 0 {
		readLimit = ResolveOpenAIWSClientReadLimitBytes(s.Options)
	}
	if s.MaxBodyBytes > 0 && s.MaxBodyBytes < readLimit {
		readLimit = s.MaxBodyBytes
	}
	updated, _, err := requeststate.NormalizeCodexClientNotifications(body, readLimit)
	if err != nil {
		if _, ok := errors.AsType[*protocolopenai.BodyLimitError](err); ok {
			return nil, gatewayhttp.NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "notification-expanded request exceeds the configured size limit", err)
		}
		return nil, gatewayhttp.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket notification payload", err)
	}
	return updated, nil
}
