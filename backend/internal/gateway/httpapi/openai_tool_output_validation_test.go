package httpapi

import (
	"bytes"
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
