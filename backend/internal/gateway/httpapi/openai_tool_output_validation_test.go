package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// TestValidateOpenAIFunctionCallOutput_HostedSearch replays hosted search beside
// client tool results. It checks HTTP admission, rejection details, continuation
// shortcuts, and the request bytes supplied to subsequent execution.
func TestValidateOpenAIFunctionCallOutput_HostedSearch(t *testing.T) {
	cases := []struct {
		name    string // request scenario label
		body    string // complete JSON request
		accept  bool   // expected admission result
		message string // expected rejection message
	}{
		{
			name:   "hosted search with null ID and referenced function result",
			body:   `{"input":[{"type":"tool_search_output","execution":"server","call_id":null,"tools":[]},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"item_reference","id":"call_1"}]}`,
			accept: true,
		},
		{
			name:   "hosted search with omitted ID after referenced function result",
			body:   `{"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"item_reference","id":"call_1"},{"type":"tool_search_output","execution":"server","tools":[]}]}`,
			accept: true,
		},
		{
			name:   "hosted search without function result",
			body:   `{"input":[{"type":"tool_search_output","execution":"server","call_id":null,"tools":[]}]}`,
			accept: true,
		},
		{
			name:    "hosted search does not supply missing function ID",
			body:    `{"input":[{"type":"tool_search_output","execution":"server","call_id":null},{"type":"function_call_output","output":"ok"}]}`,
			message: "Client tool outputs require a non-empty call_id",
		},
		{
			name:    "client search still requires an ID",
			body:    `{"input":[{"type":"tool_search_output","execution":"client","call_id":null},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"item_reference","id":"call_1"}]}`,
			message: "Client tool outputs require a non-empty call_id",
		},
		{
			name:    "unspecified search execution still requires an ID",
			body:    `{"input":[{"type":"tool_search_output","call_id":null},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"item_reference","id":"call_1"}]}`,
			message: "Client tool outputs require a non-empty call_id",
		},
		{
			name:    "hosted search does not supply missing function context",
			body:    `{"input":[{"type":"tool_search_output","execution":"server","call_id":null},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`,
			message: "Client tool outputs require tool-call context, item_reference entries matching each call_id, or a previous_response_id",
		},
		{
			name:    "mismatched reference is rejected",
			body:    `{"input":[{"type":"tool_search_output","execution":"server","call_id":null},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"item_reference","id":"call_other"}]}`,
			message: "Client tool outputs require tool-call context, item_reference entries matching each call_id, or a previous_response_id",
		},
		{
			name:    "partial reference coverage is rejected",
			body:    `{"input":[{"type":"tool_search_output","execution":"server","call_id":null},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"function_call_output","call_id":"call_2","output":"ok"},{"type":"item_reference","id":"call_1"}]}`,
			message: "Client tool outputs require tool-call context, item_reference entries matching each call_id, or a previous_response_id",
		},
		{
			name:   "previous response admits continuation for upstream validation",
			body:   `{"previous_response_id":"resp_1","input":[{"type":"function_call_output","output":"ok"}]}`,
			accept: true,
		},
		{
			name:   "inline context admits history for upstream validation",
			body:   `{"input":[{"type":"function_call_output","output":"ok"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]}`,
			accept: true,
		},
		{
			name:    "blank previous response does not admit missing ID",
			body:    `{"previous_response_id":" ","input":[{"type":"function_call_output","output":"ok"}]}`,
			message: "Client tool outputs require a non-empty call_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			original := bytes.Clone(body)
			c, recorder := newGinContextForEndpoint(t, "/v1/responses")
			accepted := ValidateOpenAIFunctionCallOutput(c, body, zap.NewNop())
			require.Equal(t, tc.accept, accepted)
			require.Equal(t, original, body)
			if tc.accept {
				require.False(t, c.Writer.Written())
				require.Empty(t, recorder.Body.String())
				return
			}
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
			require.Equal(t, tc.message, gjson.Get(recorder.Body.String(), "error.message").String())
		})
	}
}

// TestValidateOpenAIFunctionCallOutput_DiagnosticStates submits distinct missing
// identifier shapes through the HTTP gate. Serialized warnings must preserve
// those distinctions while rejection messages and caller-owned bytes stay intact.
func TestValidateOpenAIFunctionCallOutput_DiagnosticStates(t *testing.T) {
	cases := []struct {
		name          string // identifier shape scenario
		rootFields    string // preceding request JSON fields
		itemFields    string // preceding output JSON fields
		previousState string // expected continuation field classification
		callIDState   string // expected call identifier classification
	}{
		{name: "missing fields", previousState: "missing", callIDState: "missing"},
		{name: "null fields", rootFields: `"previous_response_id":null,`, itemFields: `"call_id":null,`, previousState: "null", callIDState: "null"},
		{name: "empty fields", rootFields: `"previous_response_id":"",`, itemFields: `"call_id":"",`, previousState: "empty", callIDState: "empty"},
		{name: "blank fields", rootFields: `"previous_response_id":" \t",`, itemFields: `"call_id":" \t",`, previousState: "blank", callIDState: "blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{` + tc.rootFields + `"input":[{` + tc.itemFields + `"type":"function_call_output","output":"private result"}]}`)
			original := bytes.Clone(body)
			logger, logs := newObservedLogger(t)
			logger = logger.With(zap.String("request_id", "request-diagnostic-test"))
			c, recorder := newGinContextForEndpoint(t, "/v1/responses")

			require.False(t, ValidateOpenAIFunctionCallOutput(c, body, logger))
			require.Equal(t, original, body)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
			require.Equal(t, "Client tool outputs require a non-empty call_id", gjson.Get(recorder.Body.String(), "error.message").String())
			require.NotContains(t, recorder.Body.String(), "tool_output_diagnostic")

			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, "openai.request_validation_failed", entries[0].Message)
			encodedFields, err := json.Marshal(entries[0].ContextMap())
			require.NoError(t, err)
			encoded := string(encodedFields)
			require.True(t, json.Valid(encodedFields))
			require.Equal(t, "request-diagnostic-test", gjson.Get(encoded, "request_id").String())
			require.Equal(t, "function_call_output_missing_call_id", gjson.Get(encoded, "reason").String())
			diagnostic := gjson.Get(encoded, "tool_output_diagnostic")
			require.True(t, diagnostic.IsObject())
			require.Equal(t, "tool_output_validation", diagnostic.Get("body_stage").String())
			require.Equal(t, tc.previousState, diagnostic.Get("previous_response_id_state").String())
			require.Equal(t, int64(1), diagnostic.Get("input_item_count").Int())
			require.Equal(t, int64(1), diagnostic.Get("tool_item_count").Int())
			require.Equal(t, int64(1), diagnostic.Get("missing_call_id_count").Int())
			require.Equal(t, int64(0), diagnostic.Get("hosted_search_count").Int())
			require.Equal(t, int64(0), diagnostic.Get("items_omitted").Int())
			require.Len(t, diagnostic.Get("items").Array(), 1)
			require.Equal(t, int64(0), diagnostic.Get("items.0.index").Int())
			require.Equal(t, "function_call_output", diagnostic.Get("items.0.type").String())
			require.Equal(t, tc.callIDState, diagnostic.Get("items.0.call_id_state").String())
			require.Equal(t, "none", diagnostic.Get("items.0.bootstrap_candidate").String())
			require.True(t, diagnostic.Get("items.0.requires_client_call_id").Bool())
			require.True(t, diagnostic.Get("items.0.missing_call_id").Bool())
			require.NotContains(t, encoded, "private result")
		})
	}
}

// TestValidateOpenAIFunctionCallOutput_DiagnosticDelegation reproduces delegation
// bootstrap outputs with absent and null continuation fields. The warning must
// identify the candidate and normalization blocker without logging its envelope.
func TestValidateOpenAIFunctionCallOutput_DiagnosticDelegation(t *testing.T) {
	cases := []struct {
		name       string // bootstrap continuation scenario
		rootFields string // preceding request JSON fields
		result     string // expected delegation inspection result
	}{
		{name: "missing previous response", result: "would_normalize"},
		{name: "null previous response", rootFields: `"previous_response_id":null,`, result: "previous_response_id_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{` + tc.rootFields + `"input":[{"type":"function_call_output","namespace":"codex_tui","name":"create_thread","output":"<codex_delegation><source_thread_id>private-thread-id</source_thread_id><input>private-delegation-input</input></codex_delegation>"}]}`)
			original := bytes.Clone(body)
			logger, logs := newObservedLogger(t)
			c, recorder := newGinContextForEndpoint(t, "/v1/responses")

			require.False(t, ValidateOpenAIFunctionCallOutput(c, body, logger))
			require.Equal(t, original, body)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, "openai.request_validation_failed", entries[0].Message)
			encodedFields, err := json.Marshal(entries[0].ContextMap())
			require.NoError(t, err)
			encoded := string(encodedFields)
			require.True(t, json.Valid(encodedFields))
			diagnostic := gjson.Get(encoded, "tool_output_diagnostic")
			require.Equal(t, "delegation", diagnostic.Get("items.0.bootstrap_candidate").String())
			require.Equal(t, "unique", diagnostic.Get("bootstrap.json_members").String())
			require.Equal(t, int64(1), diagnostic.Get("bootstrap.delegation.candidate_count").Int())
			require.Equal(t, tc.result, diagnostic.Get("bootstrap.delegation.result").String())
			require.Equal(t, int64(-1), diagnostic.Get("bootstrap.delegation.blocked_index").Int())
			require.Equal(t, int64(0), diagnostic.Get("bootstrap.automation.candidate_count").Int())
			require.NotContains(t, encoded, "private-thread-id")
			require.NotContains(t, encoded, "private-delegation-input")
			require.NotContains(t, encoded, "codex_delegation")
		})
	}
}

// TestValidateOpenAIFunctionCallOutput_DiagnosticRedactsRequestValues places
// secrets in output content, parameters, identifiers, and extensible metadata.
// Serialized warning fields must contain only fixed classifications for those
// values, including unknown tool types and execution values.
func TestValidateOpenAIFunctionCallOutput_DiagnosticRedactsRequestValues(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call_output","execution":"PRIVATE_EXECUTION","name":"PRIVATE_NAME","namespace":"PRIVATE_NAMESPACE","arguments":"PRIVATE_ARGUMENTS","output":"PRIVATE_OUTPUT"},{"type":"tool_search_output","execution":"server","call_id":null,"tools":[{"type":"function","name":"PRIVATE_TOOL_NAME","parameters":{"description":"PRIVATE_PARAMETERS"}}]},{"type":"PRIVATE_TYPE_call_output","execution":"PRIVATE_OTHER_EXECUTION","call_id":"PRIVATE_CALL_ID","output":{"text":"PRIVATE_NESTED_OUTPUT"}}]}`)
	original := bytes.Clone(body)
	logger, logs := newObservedLogger(t)
	c, recorder := newGinContextForEndpoint(t, "/v1/responses")

	require.False(t, ValidateOpenAIFunctionCallOutput(c, body, logger))
	require.Equal(t, original, body)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "openai.request_validation_failed", entries[0].Message)
	encodedFields, err := json.Marshal(entries[0].ContextMap())
	require.NoError(t, err)
	encoded := string(encodedFields)
	require.True(t, json.Valid(encodedFields))
	require.NotContains(t, encoded, "PRIVATE_")
	diagnostic := gjson.Get(encoded, "tool_output_diagnostic")
	require.Equal(t, int64(3), diagnostic.Get("input_item_count").Int())
	require.Equal(t, int64(1), diagnostic.Get("hosted_search_count").Int())
	require.Equal(t, "other", diagnostic.Get("items.0.execution").String())
	require.Equal(t, "server", diagnostic.Get("items.1.execution").String())
	require.False(t, diagnostic.Get("items.1.requires_client_call_id").Bool())
	require.False(t, diagnostic.Get("items.1.missing_call_id").Bool())
	require.Equal(t, "other_tool_output", diagnostic.Get("items.2.type").String())
	require.Equal(t, "other", diagnostic.Get("items.2.execution").String())
}

// TestValidateOpenAIFunctionCallOutput_DiagnosticScope exercises successful
// admission, missing context, and disabled warnings. Detailed structure must
// appear only for enabled missing-call-ID warnings, without changing HTTP results.
func TestValidateOpenAIFunctionCallOutput_DiagnosticScope(t *testing.T) {
	cases := []struct {
		name             string // gate branch scenario
		body             string // complete JSON request
		warningsDisabled bool   // suppress warning log events
		accept           bool   // expected admission result
		reason           string // expected existing warning reason
	}{
		{name: "ordinary input", body: `{"input":"private input"}`, accept: true},
		{name: "referenced result", body: `{"input":[{"type":"function_call_output","call_id":"call-1","output":"ok"},{"type":"item_reference","id":"call-1"}]}`, accept: true},
		{name: "previous response", body: `{"previous_response_id":"resp-1","input":[{"type":"function_call_output","output":"ok"}]}`, accept: true},
		{name: "inline context", body: `{"input":[{"type":"function_call","call_id":"call-1"},{"type":"function_call_output","output":"ok"}]}`, accept: true},
		{name: "missing context", body: `{"input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`, reason: "function_call_output_missing_item_reference"},
		{name: "warnings disabled", body: `{"input":[{"type":"function_call_output","output":"ok"}]}`, warningsDisabled: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			original := bytes.Clone(body)
			logger, logs := newObservedLogger(t)
			if tc.warningsDisabled {
				logger = logger.WithOptions(zap.IncreaseLevel(zap.ErrorLevel))
			}
			c, recorder := newGinContextForEndpoint(t, "/v1/responses")

			require.Equal(t, tc.accept, ValidateOpenAIFunctionCallOutput(c, body, logger))
			require.Equal(t, original, body)
			if tc.accept {
				require.False(t, c.Writer.Written())
				require.Empty(t, recorder.Body.String())
			} else {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			}
			entries := logs.All()
			if tc.reason == "" {
				require.Empty(t, entries)
			} else {
				require.Len(t, entries, 1)
				require.Equal(t, "openai.request_validation_failed", entries[0].Message)
				encodedFields, err := json.Marshal(entries[0].ContextMap())
				require.NoError(t, err)
				require.True(t, json.Valid(encodedFields))
				require.Equal(t, tc.reason, gjson.GetBytes(encodedFields, "reason").String())
				require.False(t, gjson.GetBytes(encodedFields, "tool_output_diagnostic").Exists())
			}
		})
	}
}
