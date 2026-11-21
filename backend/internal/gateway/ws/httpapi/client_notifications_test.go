package httpapi

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
)

// client_notifications_test.go checks WebSocket notification authorization,
// frame boundaries, expansion budgets, and affinity preservation. The tests
// use the private middleware principal and production session-hash resolver;
// public payer fields and request headers cannot enable a rollout.

// wsNotificationEvent supplies a complete goal record with opaque metadata for
// transport admission, byte-budget, and authenticated actor selection fixtures.
const wsNotificationEvent = `{"type":"function_call_output","id":"fco_00000000000000000000000000000000","namespace":"agent","name":"goal_objective","output":"Receipt GOAL-fixture","internal_chat_message_metadata_passthrough":{"opaque":"keep"}}`

// notificationWSContext constructs a request context with an optional verified
// principal. A distinct payer and spoofable headers deliberately identify a
// rollout member, while the actor alone controls conversion eligibility.
func notificationWSContext(actor *identity.Principal) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.0.0")
	c.Request.Header.Set("X-User-Id", "1751")
	c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 1751, Concurrency: 1})
	if actor != nil {
		authctx.SetAuthenticatedPrincipal(c, *actor)
	}
	return c
}

// TestClientNotifications_AuthenticatedActor compares site-wide and rollout
// policies against private caller identities. Wrong credential kinds and
// payer-only identities preserve the exact original request bytes.
func TestClientNotifications_AuthenticatedActor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		global    bool
		ids       []int64
		actor     *identity.Principal
		converted bool
	}{
		{"global actor", true, nil, &identity.Principal{UserID: 8, CredentialKind: "api_key"}, true},
		{"rollout actor", false, []int64{1751}, &identity.Principal{UserID: 1751, CredentialKind: "api_key"}, true},
		{"disabled", false, nil, &identity.Principal{UserID: 1751, CredentialKind: "api_key"}, false},
		{"payer cannot select", false, []int64{1751}, &identity.Principal{UserID: 8, CredentialKind: "api_key"}, false},
		{"missing principal", true, nil, nil, false},
		{"jwt cannot select", true, nil, &identity.Principal{UserID: 1751, CredentialKind: "jwt"}, false},
		{"nonpositive actor", true, nil, &identity.Principal{UserID: 0, CredentialKind: "api_key"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotifications: tc.global, CodexClientNotificationUserIDs: tc.ids})
			body := []byte(`{"type":"response.create","model":"gpt-5.5","input":[` + wsNotificationEvent + `]}`)
			original := bytes.Clone(body)
			got, err := svc.NormalizeClientNotifications(notificationWSContext(tc.actor), body, 0)
			require.NoError(t, err)
			require.Equal(t, original, body)
			if tc.converted {
				require.Equal(t, "assistant", gjson.GetBytes(got, "input.0.role").String())
				require.Contains(t, gjson.GetBytes(got, "input.0.content").String(), wsNotificationEvent)
			} else {
				require.Equal(t, original, got)
			}
		})
	}
}

// TestClientNotifications_FrameScope leaves non-request control frames intact
// and verifies that an omitted initial type follows the same notification
// rules as response.create. Real tool results remain paired by call_id.
func TestClientNotifications_FrameScope(t *testing.T) {
	svc := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotifications: true})
	c := notificationWSContext(&identity.Principal{UserID: 8, CredentialKind: "api_key"})
	for _, frameType := range []string{"session.update", "response.cancel", "extension.unknown"} {
		body := []byte(`{"type":"` + frameType + `","input":[` + wsNotificationEvent + `]}`)
		got, err := svc.NormalizeClientNotifications(c, body, 0)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
	body := []byte(`{"input":[{"type":"function_call_output","call_id":"call_real","output":"tool-result"},` + wsNotificationEvent + `]}`)
	got, err := svc.NormalizeClientNotifications(c, body, 0)
	require.NoError(t, err)
	require.Equal(t, gjson.GetBytes(body, "input.0").Raw, gjson.GetBytes(got, "input.0").Raw)
	require.Equal(t, "assistant", gjson.GetBytes(got, "input.1.role").String())
}

// TestClientNotifications_ExpansionLimits verifies that the smallest active
// budget governs converted payload size, including the immutable limit of an
// already accepted socket after a runtime increase. Rejection uses close 1009
// and does not return a partially rewritten request.
func TestClientNotifications_ExpansionLimits(t *testing.T) {
	body := []byte(`{"type":"response.create","input":[` + wsNotificationEvent + `]}`)
	original := bytes.Clone(body)
	actor := &identity.Principal{UserID: 8, CredentialKind: "api_key"}
	baseline := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotifications: true})
	expanded, err := baseline.NormalizeClientNotifications(notificationWSContext(actor), body, 0)
	require.NoError(t, err)
	require.Greater(t, len(expanded), len(body))
	small := int64(len(expanded) - 1)
	large := int64(len(expanded) + 10)
	for _, tc := range []struct {
		name       string
		frameLimit int64
		bodyLimit  int64
		captured   int64
	}{
		{"frame", small, large, 0},
		{"HTTP body", large, small, 0},
		{"captured before runtime increase", large, large, small},
		{"captured with default adapter", 0, large, small},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := notificationWSContext(actor)
			if tc.captured > 0 {
				c.Set(responsesWSReadLimitContextKey, tc.captured)
			}
			svc := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotifications: true, MaxBodyBytes: tc.bodyLimit})
			got, err := svc.NormalizeClientNotifications(c, body, tc.frameLimit)
			require.Nil(t, got)
			closeErr, ok := errors.AsType[*gatewayhttp.OpenAIWSClientCloseError](err)
			require.True(t, ok)
			require.Equal(t, coderws.StatusMessageTooBig, closeErr.StatusCode())
			require.Equal(t, original, body)
		})
	}
	got, err := baseline.NormalizeClientNotifications(notificationWSContext(actor), body, int64(len(expanded)))
	require.NoError(t, err)
	require.Equal(t, expanded, got)
}

// TestClientNotifications_AffinityPreserved compares the actual session hash
// before and after conversion. Both explicit session identifiers and content
// fallback ignore these independent notification records, including a history
// without any user message.
func TestClientNotifications_AffinityPreserved(t *testing.T) {
	svc := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotifications: true})
	actor := &identity.Principal{UserID: 8, CredentialKind: "api_key"}
	for _, history := range []string{"", `{"role":"system","content":"system"},{"role":"user","content":"start task"},`} {
		for _, sessionID := range []string{"", "existing-session"} {
			body := []byte(`{"type":"response.create","model":"gpt-5.5","input":[` + history + wsNotificationEvent + `]}`)
			got, err := svc.NormalizeClientNotifications(notificationWSContext(actor), body, 0)
			require.NoError(t, err)
			before, after := notificationWSContext(actor), notificationWSContext(actor)
			before.Request.Header.Set("session_id", sessionID)
			after.Request.Header.Set("session_id", sessionID)
			require.Equal(t, gatewayhttp.GenerateOpenAISessionHashWithFallback(before, body, "user-key-seed"), gatewayhttp.GenerateOpenAISessionHashWithFallback(after, got, "user-key-seed"))
		}
	}
}

// TestClientNotifications_ConstructorOwnsRollout confirms that caller mutation
// after construction cannot silently move a startup rollout to another actor.
func TestClientNotifications_ConstructorOwnsRollout(t *testing.T) {
	ids := []int64{1751}
	svc := NewOpenAIWebSocketExecutor(OpenAIWSDependencies{CodexClientNotificationUserIDs: ids})
	ids[0] = 8
	require.Equal(t, []int64{1751}, svc.CodexClientNotificationUserIDs)
}
