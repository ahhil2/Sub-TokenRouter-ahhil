package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// openai_text_notifications_scenario_test.go exercises notification admission in
// openai_text_responses.go and upstream preparation in openai_responses_execution.go.
// Authenticated actor policy controls whether call-less client notifications
// become assistant history messages. Captured API-key and OAuth requests verify
// that complete event records survive namespace filtering and both compaction
// protocols while genuine tool pairs and local validation remain intact.

const (
	// These complete client event records cover both supported notification kinds,
	// including heartbeat metadata rejected by the older automation bootstrap parser.
	codexAutomationNoticeFixture = `{"type":"function_call_output","id":"fco_00000000000000000000000000000001","namespace":"codex_app","name":"automation_update","output":"<heartbeat><automation_id>diagnostic</automation_id><current_time_iso>2026-11-21T10:00:00+08:00</current_time_iso><instructions>Receipt HB-test 中文</instructions></heartbeat>"}`
	codexGoalNoticeFixture       = `{"type":"function_call_output","id":"fco_00000000000000000000000000000002","namespace":"agent","name":"goal_objective","output":"Continue the existing objective; preserve \"quoted\" text and 中文."}`
)

// TestOpenAITextClientNotificationActorScope submits both event kinds with
// different authenticated actors, payers, and key owners. Only positive
// selected API-key principals receive conversion, regardless of public context
// changes or client-supplied identity claims.
func TestOpenAITextClientNotificationActorScope(t *testing.T) {
	for _, notice := range []struct {
		name  string // readable notification kind
		event string // complete client event
	}{
		{name: "automation", event: codexAutomationNoticeFixture},
		{name: "goal", event: codexGoalNoticeFixture},
	} {
		t.Run(notice.name, func(t *testing.T) {
			for _, test := range []struct {
				name             string  // policy scenario label
				allowlist        []int64 // configured actor identities
				allUsers         bool    // site-wide conversion switch
				actor            int64   // authenticated acting user
				payer            int64   // resolved paying user
				owner            int64   // authenticated key owner
				missingPrincipal bool    // omitted private authentication
				// publicAfter rewrites the compatibility identity after recording
				// authentication when nonzero; zero keeps the original public payer.
				publicAfter int64
				accepted    bool // expected execution admission
				// credentialKind overrides the API-key fixture default when another
				// authentication mechanism must be rejected despite a matching user.
				credentialKind string
			}{
				{name: "default disabled", actor: 7, payer: 7, owner: 7},
				{name: "all users without list", allUsers: true, actor: 123, payer: 123, owner: 123, accepted: true},
				{name: "all users outside list", allUsers: true, allowlist: []int64{40}, actor: 7, payer: 40, owner: 40, accepted: true},
				{name: "all users requires principal", allUsers: true, actor: 7, payer: 7, owner: 7, missingPrincipal: true},
				{name: "all users requires API key", allUsers: true, actor: 7, payer: 7, owner: 7, credentialKind: "jwt"},
				{name: "other actor and spoofed identity", allowlist: []int64{40}, actor: 7, payer: 7, owner: 7},
				{name: "key owner alone cannot enable", allowlist: []int64{40}, actor: 7, payer: 7, owner: 40},
				{name: "payer alone cannot enable", allowlist: []int64{40}, actor: 7, payer: 40, owner: 40},
				{name: "enabled owner", allowlist: []int64{7}, actor: 7, payer: 7, owner: 7, accepted: true},
				{name: "enabled actor with unlisted payer", allowlist: []int64{7}, actor: 7, payer: 40, owner: 40, accepted: true},
				{name: "zero actor", allowlist: []int64{0}, actor: 0, payer: 7, owner: 7},
				{name: "negative actor", allowlist: []int64{-1}, actor: -1, payer: 7, owner: 7},
				{name: "missing private principal", allowlist: []int64{7}, actor: 7, payer: 7, owner: 7, missingPrincipal: true},
				{name: "JWT principal", allowlist: []int64{7}, actor: 7, payer: 7, owner: 7, credentialKind: "jwt"},
				{name: "later public identity cannot enable", allowlist: []int64{40}, actor: 7, payer: 7, owner: 7, publicAfter: 40},
				{name: "later public identity cannot disable", allowlist: []int64{7}, actor: 7, payer: 7, owner: 7, publicAfter: 40, accepted: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					body := `{"model":"gpt-5","user_id":40,"metadata":{"user_id":40},"input":[` + notice.event + `]}`
					p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
					p.key.UserID = test.owner
					// Authentication records the caller after resolving the payer.
					// Later compatibility-field changes must not replace that verified actor.
					c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: test.payer})
					if !test.missingPrincipal {
						kind := test.credentialKind
						if kind == "" {
							kind = "api_key"
						}
						authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: test.actor, CredentialKind: kind})
					}
					if test.publicAfter != 0 {
						c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: test.publicAfter})
					}
					c.Request.Header.Set("X-User-ID", "40")
					c.Request.Header.Set("X-Authenticated-User-ID", "40")
					h := NewOpenAITextHandler(OpenAITextOptions{
						CodexClientNotifications: test.allUsers, CodexClientNotificationUserIDs: test.allowlist,
					}, p, p, p)

					h.Responses(c)

					if !test.accepted {
						require.Equal(t, http.StatusBadRequest, response.Code)
						require.Contains(t, response.Body.String(), "Client tool outputs require a non-empty call_id")
						require.Nil(t, p.call)
						return
					}
					require.Equal(t, http.StatusOK, response.Code)
					require.NotNil(t, p.call)
					require.JSONEq(t, body, string(p.call.SessionHashBody))
					assertCodexNotificationMessage(t, gjson.GetBytes(p.call.Body, "input.0"), notice.event)
				})
			}
		})
	}
}

// TestOpenAITextClientNotificationMixedAPIKeyForward admits two interleaved tool
// pairs alongside both notification kinds. Actual API-key preparation must keep
// each association and complete event through upstream namespace cleanup.
func TestOpenAITextClientNotificationMixedAPIKeyForward(t *testing.T) {
	const call = `{"type":"function_call","namespace":"diagnostics","name":"ping","call_id":"call_diagnostic","arguments":"{}"}`
	const result = `{"type":"function_call_output","call_id":"call_diagnostic","output":"pong"}`
	const secondCall = `{"type":"function_call","namespace":"diagnostics","name":"ping","call_id":"call_second","arguments":"{}"}`
	const secondResult = `{"type":"function_call_output","call_id":"call_second","output":"second pong"}`
	goal := strings.TrimSuffix(codexGoalNoticeFixture, "}") + `,"internal_chat_message_metadata_passthrough":{"retained":true,"sequence":9007199254740993}}`
	body := `{"model":"gpt-5","stream":false,"instructions":"Keep all event data.","tools":[{"type":"namespace","name":"diagnostics","tools":[{"type":"function","name":"ping","parameters":{"type":"object"}}]}],"input":[{"role":"user","content":"Continue"},` + call + `,` + secondCall + `,` + secondResult + `,` + codexAutomationNoticeFixture + `,` + result + `,` + goal + `]}`
	for _, test := range []struct {
		name        string // forwarding scenario label
		enabled     bool   // actor compatibility policy
		passthrough bool   // provider passthrough option
	}{
		{name: "disabled mixed history"},
		{name: "enabled standard API key", enabled: true},
		{name: "enabled passthrough API key", enabled: true, passthrough: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
			authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
			options := OpenAITextOptions{}
			if test.enabled {
				options.CodexClientNotifications = true
			}
			NewOpenAITextHandler(options, p, p, p).Responses(c)
			require.Equal(t, http.StatusOK, response.Code)
			require.NotNil(t, p.call)
			require.JSONEq(t, body, string(p.call.SessionHashBody))

			// Forward the exact admitted bytes through the production API-key path.
			// A recording transport exposes namespace cleanup without contacting an external service.
			upstream := &auxiliaryHTTPRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_notifications","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
				LoadLocation: time.LoadLocation,
				ID:           5108,
				Platform:     capability.PlatformOpenAI,
				Type:         capability.ProviderTypeAPIKey,
				Concurrency:  1,
				Credentials:  map[string]any{"api_key": "sk-test", "base_url": "https://compat.example"},
				Extra: map[string]any{
					"openai_passthrough":               test.passthrough,
					providercore.ExtraKeyTextRouteMode: string(providercore.TextRouteModePreserveClientProtocol),
				},
			}}
			forwarded, err := newResponsesFixture(responsesFixtureInputs{transport: upstream}).Forward(context.Background(), c, provider, p.call.Body)
			require.NoError(t, err)
			require.NotNil(t, forwarded)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
			input := gjson.GetBytes(upstream.lastBody, "input").Array()
			require.Len(t, input, 7)
			require.JSONEq(t, call, input[1].Raw)
			require.JSONEq(t, secondCall, input[2].Raw)
			require.JSONEq(t, secondResult, input[3].Raw)
			require.JSONEq(t, result, input[5].Raw)
			if test.enabled {
				assertCodexNotificationMessage(t, input[4], codexAutomationNoticeFixture)
				assertCodexNotificationMessage(t, input[6], goal)
				require.Contains(t, input[6].Get("content").String(), `"sequence":9007199254740993`)
			} else {
				require.JSONEq(t, body, string(p.call.Body))
				require.Equal(t, "function_call_output", input[4].Get("type").String())
				require.Equal(t, "function_call_output", input[6].Get("type").String())
				require.False(t, input[4].Get("namespace").Exists())
				require.False(t, input[6].Get("namespace").Exists())
			}
		})
	}
}

// TestOpenAITextClientNotificationsSurviveCompactionForwarding runs legacy
// Compact, body-promoted Compact, and native streaming compaction through
// admission and real outbound preparation. Both client event records retain
// their complete JSON beside opaque compaction state and a trigger, for API-key
// and OAuth providers with managed or passthrough forwarding. The affinity
// input remains the client event representation captured before conversion.
func TestOpenAITextClientNotificationsSurviveCompactionForwarding(t *testing.T) {
	const compactState = `{"type":"compaction","id":"cmp_history","encrypted_content":"opaque-history-ciphertext","opaque":{"sequence":9007199254740993,"payload":"keep \\ and 中文"}}`
	const trigger = `{"type":"compaction_trigger"}`
	const compactResult = `{"type":"compaction","id":"cmp_result","encrypted_content":"opaque-result-ciphertext","opaque":{"future_field":true}}`
	goal := strings.TrimSuffix(codexGoalNoticeFixture, "}") + `,"internal_chat_message_metadata_passthrough":{"opaque":{"sequence":9007199254740993}}}`
	for _, route := range []struct {
		name   string // admission and upstream protocol scenario
		path   string // original client endpoint
		native bool   // streaming native compaction instead of unary Compact
	}{
		{name: "explicit compact", path: "/v1/responses/compact"},
		{name: "body promoted compact", path: "/v1/responses"},
		{name: "remote compaction v2", path: "/v1/responses", native: true},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, providerType := range []string{capability.ProviderTypeAPIKey, capability.ProviderTypeOAuth} {
				for _, passthrough := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/passthrough=%t", providerType, passthrough), func(t *testing.T) {
						body := fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"instructions":"Retain the full task history.","prompt_cache_key":"notice-compaction-session","input":[%s,%s,%s,%s]}`, route.native, compactState, codexAutomationNoticeFixture, goal, trigger)
						p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
						c.Request.URL.Path = route.path
						authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
						NewOpenAITextHandler(OpenAITextOptions{CodexClientNotifications: true}, p, p, p).Responses(c)
						require.Equal(t, http.StatusOK, response.Code)
						require.NotNil(t, p.call)
						require.Equal(t, route.native, p.call.NativeCompactionV2)
						require.Equal(t, !route.native, p.call.LegacyCompact)
						admitted := gjson.GetBytes(p.call.Body, "input").Array()
						require.Len(t, admitted, 4)
						assertCodexNotificationMessage(t, admitted[1], codexAutomationNoticeFixture)
						assertCodexNotificationMessage(t, admitted[2], goal)
						require.JSONEq(t, codexAutomationNoticeFixture, gjson.GetBytes(p.call.SessionHashBody, "input.1").Raw)
						require.JSONEq(t, goal, gjson.GetBytes(p.call.SessionHashBody, "input.2").Raw)

						responseBody := `{"id":"resp_compaction_notice","status":"completed","output":[` + compactResult + `],"usage":{"input_tokens":1,"output_tokens":1}}`
						contentType := "application/json"
						if route.native {
							contentType = "text/event-stream"
							responseBody = "data: {\"type\":\"response.output_item.done\",\"item\":" + compactResult + "}\n\ndata: {\"type\":\"response.completed\",\"response\":" + responseBody + "}\n\ndata: [DONE]\n\n"
						}
						upstream := &auxiliaryHTTPRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{contentType}},
							Body:       io.NopCloser(strings.NewReader(responseBody)),
						}}
						provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
							LoadLocation: time.LoadLocation, ID: 5109, Platform: capability.PlatformOpenAI,
							Type: providerType, Concurrency: 1,
							Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://compat.example/v1"},
							Extra: map[string]any{
								"openai_passthrough":               passthrough,
								providercore.ExtraKeyTextRouteMode: string(providercore.TextRouteModePreserveClientProtocol),
							},
						}}
						expectedURL := "https://compat.example/v1/responses"
						if providerType == capability.ProviderTypeOAuth {
							provider.Record.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-fixture"}
							expectedURL = ChatgptCodexURL
						}
						if !route.native {
							expectedURL += "/compact"
						}
						SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
						forwarded, err := newResponsesFixture(responsesFixtureInputs{transport: upstream}).Forward(context.Background(), c, provider, p.call.Body)
						require.NoError(t, err)
						require.NotNil(t, forwarded)
						require.Len(t, upstream.bodies, 1)
						require.Equal(t, expectedURL, upstream.lastReq.URL.String())
						input := gjson.GetBytes(upstream.lastBody, "input").Array()
						require.Len(t, input, 4)
						expectedState := compactState
						if providerType == capability.ProviderTypeOAuth && !passthrough {
							// Managed OAuth already drops historical item IDs while
							// forwarding the encrypted state and opaque extension data.
							expectedState = strings.Replace(expectedState, `"id":"cmp_history",`, "", 1)
						}
						require.JSONEq(t, expectedState, input[0].Raw)
						require.Equal(t, "9007199254740993", input[0].Get("opaque.sequence").Raw)
						assertCodexNotificationMessage(t, input[1], codexAutomationNoticeFixture)
						assertCodexNotificationMessage(t, input[2], goal)
						require.Contains(t, input[2].Get("content").String(), `"sequence":9007199254740993`)
						require.JSONEq(t, trigger, input[3].Raw)
						if route.native {
							require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
							require.Equal(t, "remote_compaction_v2", upstream.lastReq.Header.Get("x-codex-beta-features"))
						} else {
							require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Exists())
							require.Equal(t, "notice-compaction-session", c.GetString(OpenAICompactSessionSeedKey))
						}
						require.Contains(t, response.Body.String(), "opaque-result-ciphertext")
					})
				}
			}
		})
	}
}

// TestOpenAITextClientNotificationRejectsInvalidToolResults enables compatibility
// while submitting ordinary and fabricated tool results. Their original local
// validation errors must remain visible before any execution attempt occurs.
func TestOpenAITextClientNotificationRejectsInvalidToolResults(t *testing.T) {
	for _, test := range []struct {
		name    string // invalid result scenario
		event   string // submitted tool result
		message string // expected validation message
	}{
		{name: "ordinary missing call", event: `{"type":"function_call_output","id":"fco_ordinary","output":"ordinary tool result"}`, message: "Client tool outputs require a non-empty call_id"},
		{name: "fabricated call", event: strings.TrimSuffix(codexAutomationNoticeFixture, "}") + `,"call_id":"call_fake"}`, message: "Client tool outputs require tool-call context"},
		{name: "null call", event: strings.TrimSuffix(codexGoalNoticeFixture, "}") + `,"call_id":null}`, message: "Client tool outputs require a non-empty call_id"},
		{name: "empty call", event: strings.TrimSuffix(codexGoalNoticeFixture, "}") + `,"call_id":""}`, message: "Client tool outputs require a non-empty call_id"},
		{name: "unknown event", event: strings.Replace(codexGoalNoticeFixture, "goal_objective", "unknown_event", 1), message: "Client tool outputs require a non-empty call_id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, _, c, response, _ := newOpenAITextEntryProbe(t, `{"model":"gpt-5","input":[`+test.event+`]}`)
			authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
			NewOpenAITextHandler(OpenAITextOptions{CodexClientNotificationUserIDs: []int64{7}}, p, p, p).Responses(c)
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), test.message)
			require.Nil(t, p.call)
		})
	}
}

// TestOpenAITextClientNotificationsDoNotReenterLegacyBootstrap combines a valid
// notification with heartbeat records outside the supported format. Converting
// the first record must leave the other record subject to tool-result validation.
func TestOpenAITextClientNotificationsDoNotReenterLegacyBootstrap(t *testing.T) {
	for _, extra := range []string{``, `,"call_id":""`, `,"future":true`} {
		t.Run(extra, func(t *testing.T) {
			invalid := `{"type":"function_call_output","id":"bad","namespace":"codex_app","name":"automation_update","output":"<heartbeat><automation_id>demo</automation_id></heartbeat>"` + extra + `}`
			body := `{"model":"gpt-5","input":[` + codexGoalNoticeFixture + `,` + invalid + `]}`
			p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
			authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
			NewOpenAITextHandler(OpenAITextOptions{CodexClientNotifications: true}, p, p, p).Responses(c)
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), "Client tool outputs require a non-empty call_id")
			require.Nil(t, p.call)
		})
	}
}

// TestOpenAITextClientNotificationRemainingOutputsRequireContext converts the
// only function-call output into assistant history data. Remaining client output
// kinds must still require call identifiers and complete matching references.
func TestOpenAITextClientNotificationRemainingOutputsRequireContext(t *testing.T) {
	for _, itemType := range []string{"custom_tool_call_output", "mcp_tool_call_output", "tool_search_output"} {
		t.Run(itemType, func(t *testing.T) {
			payload := `"output":"tool result"`
			if itemType == "tool_search_output" {
				payload = `"execution":"client","tools":[]`
			}
			for _, test := range []struct {
				name      string // output association scenario
				callField string // optional identifier member
				reference string // optional reference item
				message   string // expected rejection substring
			}{
				{name: "missing call identifier", message: "Client tool outputs require a non-empty call_id"},
				{name: "identified without context", callField: `,"call_id":"call_pending"`, message: "Client tool outputs require tool-call context"},
				{name: "unrelated item reference", callField: `,"call_id":"call_pending"`, reference: `,{"type":"item_reference","id":"call_other"}`, message: "Client tool outputs require tool-call context"},
				{name: "matching item reference", callField: `,"call_id":"call_pending"`, reference: `,{"type":"item_reference","id":"call_pending"}`},
			} {
				t.Run(test.name, func(t *testing.T) {
					event := `{"type":"` + itemType + `",` + payload + test.callField + `}`
					body := `{"model":"gpt-5","input":[` + codexGoalNoticeFixture + `,` + event + test.reference + `]}`
					p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
					authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
					NewOpenAITextHandler(OpenAITextOptions{CodexClientNotificationUserIDs: []int64{7}}, p, p, p).Responses(c)
					if test.message != "" {
						require.Equal(t, http.StatusBadRequest, response.Code)
						require.Contains(t, response.Body.String(), test.message)
						require.Nil(t, p.call)
						return
					}
					require.Equal(t, http.StatusOK, response.Code)
					require.NotNil(t, p.call)
					input := gjson.GetBytes(p.call.Body, "input").Array()
					require.Len(t, input, 3)
					assertCodexNotificationMessage(t, input[0], codexGoalNoticeFixture)
					require.JSONEq(t, event, input[1].Raw)
					require.JSONEq(t, strings.TrimPrefix(test.reference, ","), input[2].Raw)
				})
			}
		})
	}
}

// TestOpenAITextClientNotificationAllowsHostedSearch converts a goal notice
// beside server-executed search history. The hosted output must remain intact
// and admitted without a client call identifier or item reference.
func TestOpenAITextClientNotificationAllowsHostedSearch(t *testing.T) {
	for _, event := range []string{
		`{"type":"tool_search_output","execution":"server","tools":[]}`,
		`{"type":"tool_search_output","execution":"server","call_id":null,"tools":[]}`,
	} {
		t.Run(event, func(t *testing.T) {
			body := `{"model":"gpt-5","input":[` + codexGoalNoticeFixture + `,` + event + `]}`
			p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
			authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
			NewOpenAITextHandler(OpenAITextOptions{CodexClientNotificationUserIDs: []int64{7}}, p, p, p).Responses(c)
			require.Equal(t, http.StatusOK, response.Code)
			require.NotNil(t, p.call)
			input := gjson.GetBytes(p.call.Body, "input").Array()
			require.Len(t, input, 2)
			assertCodexNotificationMessage(t, input[0], codexGoalNoticeFixture)
			require.JSONEq(t, event, input[1].Raw)
		})
	}
}

// TestOpenAITextClientNotificationExpansionLimit accepts the original request
// length but rejects its larger enclosed representation. Notification conversion
// must enforce the configured body limit before moderation or execution begins.
func TestOpenAITextClientNotificationExpansionLimit(t *testing.T) {
	body := `{"model":"gpt-5","input":[` + codexAutomationNoticeFixture + `]}`
	p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
	authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
	options := OpenAITextOptions{MaxBodyBytes: int64(len(body)), CodexClientNotificationUserIDs: []int64{7}}
	NewOpenAITextHandler(options, p, p, p).Responses(c)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Contains(t, response.Body.String(), BodyTooLargeMessage(int64(len(body))))
	require.Nil(t, p.call)
	require.NotContains(t, p.events, "moderate")
}

// TestOpenAITextClientNotificationDelegationExpansionLimit combines a notice
// containing literal HTML characters with a valid delegation bootstrap. The
// second conversion can exceed the first conversion's budget; HTTP admission
// must reject that body before moderation or execution while accepting its exact size.
func TestOpenAITextClientNotificationDelegationExpansionLimit(t *testing.T) {
	notice := `{"type":"function_call_output","id":"fco_00000000000000000000000000000002","namespace":"agent","name":"goal_objective","output":"` + strings.Repeat("<", 1024) + `","internal_chat_message_metadata_passthrough":{"sequence":9007199254740993}}`
	delegation := `{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"<codex_delegation><source_thread_id>thread_fixture</source_thread_id><input>continue</input></codex_delegation>"}`
	body := `{"model":"gpt-5","input":[` + notice + `,` + delegation + `]}`
	// A small explicit limit admits the raw and notification-wrapped input,
	// then exposes HTML escaping performed by the existing delegation conversion.
	const conversionLimit int64 = 4096
	normalized, count, err := requeststate.NormalizeCodexClientNotifications([]byte(body), conversionLimit)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Less(t, int64(len(normalized)), conversionLimit)
	expanded, changed := requeststate.NormalizeCodexDelegationBootstrap(normalized)
	require.True(t, changed)
	require.Greater(t, int64(len(expanded)), conversionLimit)

	for _, test := range []struct {
		name     string // final body budget scenario
		limit    int64  // configured request byte limit
		rejected bool   // final body exceeds limit
	}{
		{name: "delegation exceeds limit", limit: conversionLimit, rejected: true},
		{name: "exact final limit", limit: int64(len(expanded))},
		{name: "default limit", limit: 0},
		{name: "negative selects default", limit: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, _, c, response, _ := newOpenAITextEntryProbe(t, body)
			authctx.SetAuthenticatedPrincipal(c, identity.Principal{UserID: 7, CredentialKind: "api_key"})
			NewOpenAITextHandler(OpenAITextOptions{MaxBodyBytes: test.limit, CodexClientNotifications: true}, p, p, p).Responses(c)
			if test.rejected {
				require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
				require.Contains(t, response.Body.String(), BodyTooLargeMessage(test.limit))
				require.Nil(t, p.call)
				require.NotContains(t, p.events, "moderate")
				require.NotContains(t, p.events, "execution")
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			require.NotNil(t, p.call)
			require.Equal(t, expanded, p.call.Body)
			assertCodexNotificationMessage(t, gjson.GetBytes(p.call.Body, "input.0"), notice)
			require.Contains(t, gjson.GetBytes(p.call.Body, "input.0.content").String(), `"sequence":9007199254740993`)
			require.Equal(t, "user", gjson.GetBytes(p.call.Body, "input.1.role").String())
		})
	}
}

// assertCodexNotificationMessage requires an assistant history message enclosing the
// complete original event. It compares parsed event JSON so harmless whitespace
// changes cannot obscure dropped fields, changed values, or altered notification identity.
func assertCodexNotificationMessage(t *testing.T, item gjson.Result, event string) {
	t.Helper()
	require.Equal(t, "message", item.Get("type").String())
	require.Equal(t, "assistant", item.Get("role").String())
	content := item.Get("content")
	require.Equal(t, gjson.String, content.Type)
	text := content.String()
	require.Contains(t, text, "Client-provided Codex event record.")
	require.Contains(t, text, "not a new user instruction, tool invocation, or approval.")
	_, enclosed, found := strings.Cut(text, "Original event as JSON:\n")
	require.True(t, found)
	require.JSONEq(t, event, enclosed)
}
