package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/gateway/session/testkit"
	gatewayws "github.com/TokenFlux/TokenRouter/internal/gateway/ws"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// client_notifications_scenario_test.go drives real downstream WebSockets
// through pooled, dedicated, and HTTP bridge execution. Scripted local
// upstreams capture both initial notifications and subsequent response.create
// frames; no external provider or model invocation is used.

// TestClientNotifications_ConsecutiveTransportTurns verifies both supported
// event classes before upstream serialization in every WS execution mode.
// A second frame omits model and continues the first response, while ordinary
// paired tool history and complete notification metadata remain available.
func TestClientNotifications_ConsecutiveTransportTurns(t *testing.T) {
	for _, scenario := range []string{"pooled", "per_session", "http_bridge", "pooled_oauth", "per_session_oauth"} {
		t.Run(scenario, func(t *testing.T) {
			mode := strings.TrimSuffix(scenario, "_oauth")
			options := passthroughLifecycleConfig()
			options.WS.ReadTimeoutSeconds = 3
			options.WS.IngressInterTurnIdleTimeoutSeconds = 3
			options.Output.OpenAIFirstOutputTimeoutSeconds = 3
			options.Pool.MaxConnsPerProvider = 1
			options.Pool.MaxIdlePerProvider = 1
			options.Pool.QueueLimitPerConn = 8
			provider := passthroughLifecycleProvider()
			provider.Record.Extra = map[string]any{"responses_websockets_v2_enabled": true}
			if strings.HasSuffix(scenario, "_oauth") {
				provider.Record.Type = capability.ProviderTypeOAuth
				provider.Record.Credentials = map[string]any{"access_token": "oauth-token"}
			}
			completion := []string{
				`{"type":"response.completed","response":{"id":"resp_notification_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
				`{"type":"response.completed","response":{"id":"resp_notification_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
			}
			upstreamHTTP := &auxiliaryHTTPRecorder{}
			upstreamWS := &openAIWSCaptureConn{events: [][]byte{[]byte(completion[0]), []byte(completion[1])}}
			dedicated := newStagedPassthroughConn()
			inputs := wsFixtureInputs{options: options, transport: upstreamHTTP, cache: &testkit.StickyCache{}, corrector: openai.NewCodexToolCorrector()}
			switch mode {
			case "pooled":
				pool := newOpenAIWSConnPool(options)
				pool.SetClientDialerForTest(&openAIWSCaptureDialer{conn: upstreamWS})
				inputs.pool = pool
			case "per_session":
				provider.Record.Extra = map[string]any{"responses_ws_connection_mode": "per_session"}
				inputs.dialer = &stagedPassthroughDialer{conn: dedicated}
			case "http_bridge":
				provider.Record.Extra = map[string]any{"openai_apikey_responses_websockets_v2_mode": "http_bridge"}
				for _, event := range completion {
					upstreamHTTP.responses = append(upstreamHTTP.responses, &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader("data: " + event + "\n\ndata: [DONE]\n\n")),
					})
				}
			}
			svc := newWSFixture(inputs)
			svc.CodexClientNotifications = true
			controlCtx, cancelControl := context.WithCancel(context.Background())
			defer cancelControl()
			server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, provider, func(c *gin.Context) *gatewayws.OpenAIIngressHooks {
				authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 8, CredentialKind: "api_key"})
				return nil
			})
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			cancel()
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()

			automation := strings.ReplaceAll(strings.ReplaceAll(wsNotificationEvent, `"agent"`, `"codex_app"`), `"goal_objective"`, `"automation_update"`)
			frames := []string{
				`{"type":"response.create","model":"gpt-5.1","input":[{"role":"user","content":"Continue the task"},` + automation + `,{"type":"function_call","id":"fc_real","call_id":"call_real","name":"fixture_tool","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"tool-result"}]}`,
				`{"type":"response.create","previous_response_id":"resp_notification_1","input":[` + wsNotificationEvent + `]}`,
			}
			var captured [][]byte
			for turn, frame := range frames {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err = client.Write(ctx, coderws.MessageText, []byte(frame))
				cancel()
				require.NoError(t, err)
				if mode == "per_session" {
					captured = append(captured, requirePassthroughUpstreamWrite(t, dedicated, 3*time.Second))
					dedicated.Send(completion[turn])
				}
				event, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, readErr)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			}
			_ = client.Close(coderws.StatusNormalClosure, "done")
			select {
			case err := <-serverErr:
				if err != nil {
					require.Contains(t, err.Error(), "StatusNormalClosure")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("notification scenario session did not exit")
			}

			// Inspect shared captures after the handler exits to avoid racing the
			// scripted transport's request-recording writes.
			switch mode {
			case "pooled":
				for _, request := range upstreamWS.writes {
					body, marshalErr := json.Marshal(request)
					require.NoError(t, marshalErr)
					captured = append(captured, body)
				}
			case "http_bridge":
				captured = upstreamHTTP.bodies
			}
			require.Len(t, captured, 2)
			require.Equal(t, "assistant", gjson.GetBytes(captured[0], "input.1.role").String())
			require.Contains(t, gjson.GetBytes(captured[0], "input.1.content").String(), automation)
			require.Equal(t, "call_real", gjson.GetBytes(captured[0], "input.3.call_id").String())
			require.Equal(t, "tool-result", gjson.GetBytes(captured[0], "input.3.output").String())
			secondInput := gjson.GetBytes(captured[1], "input").Array()
			require.NotEmpty(t, secondInput)
			lastItem := secondInput[len(secondInput)-1]
			require.Equal(t, "assistant", lastItem.Get("role").String())
			require.Contains(t, lastItem.Get("content").String(), wsNotificationEvent)
			if mode == "http_bridge" {
				// The existing HTTP bridge reconstructs full history and removes
				// the WS continuation anchor before issuing its stateless request.
				require.Len(t, secondInput, 5)
				require.False(t, gjson.GetBytes(captured[1], "previous_response_id").Exists())
			} else {
				require.Equal(t, "resp_notification_1", gjson.GetBytes(captured[1], "previous_response_id").String())
			}
			require.Equal(t, "gpt-5.1", gjson.GetBytes(captured[1], "model").String())
		})
	}
}

// TestClientNotifications_DedicatedRuntimeLimit reduces the frame budget after
// a successful dedicated turn. A later notification fits the raw frame but
// exceeds the new budget after conversion, closes with 1009, and never reaches
// the upstream transport. The socket's initially larger limit cannot override
// the current runtime limit for conversion.
func TestClientNotifications_DedicatedRuntimeLimit(t *testing.T) {
	options := passthroughLifecycleConfig()
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(options, upstream)
	svc.CodexClientNotifications = true
	parameters := gatewayws.DefaultParameters()
	parameters.ClientReadLimitBytes = 4096
	parameters.ReadTimeoutSeconds = 3
	parameters.WriteTimeoutSeconds = 3
	svc.Runtime = gatewayws.NewRuntime(parameters, nil, nil)
	controlCtx, cancelControl := context.WithCancel(context.Background())
	defer cancelControl()
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleProvider(), func(c *gin.Context) *gatewayws.OpenAIIngressHooks {
		authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 8, CredentialKind: "api_key"})
		c.Set(responsesWSReadLimitContextKey, int64(4096))
		return nil
	})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_limit_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	frame := []byte(`{"type":"response.create","previous_response_id":"resp_limit_1","input":[` + wsNotificationEvent + `]}`)
	converted, err := svc.NormalizeClientNotifications(notificationWSContext(&identity.Principal{UserID: 8, CredentialKind: "api_key"}), frame, 4096)
	require.NoError(t, err)
	require.Greater(t, len(converted), len(frame))
	require.NoError(t, svc.Runtime.Publish(fmt.Sprintf(`{"client_read_limit_bytes":%d}`, len(converted)-1)))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = client.Write(ctx, coderws.MessageText, frame)
	cancel()
	require.NoError(t, err)
	_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.Equal(t, coderws.StatusMessageTooBig, coderws.CloseStatus(err))
	select {
	case err := <-serverErr:
		closeErr, ok := errors.AsType[*gatewayhttp.OpenAIWSClientCloseError](err)
		require.True(t, ok)
		require.Equal(t, coderws.StatusMessageTooBig, closeErr.StatusCode())
	case <-time.After(3 * time.Second):
		t.Fatal("notification size rejection did not end the session")
	}
	require.Empty(t, upstream.writes)
}
