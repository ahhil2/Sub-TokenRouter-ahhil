package bridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAdaptOpenAIResponsesClientToolsPairsHistoryAfterToolReplacement replays a
// completed custom call after the active declarations change. Calls and results
// must use matching kinds, while retired tools stay absent from future mappings.
func TestAdaptOpenAIResponsesClientToolsPairsHistoryAfterToolReplacement(t *testing.T) {
	for _, tc := range []struct {
		name    string // declaration replacement scenario
		tools   string // optional JSON declarations
		lowered bool   // history uses function representation
	}{
		{"same custom", `[{"type":"custom","name":"exec"}]`, true},
		{"different custom", `[{"type":"custom","name":"apply_patch"}]`, true},
		{"ordinary function", `[{"type":"function","name":"lookup"}]`, true},
		{"discovery", `[{"type":"tool_search"}]`, true},
		{"empty", `[]`, false},
		{"null", `null`, false},
		{"omitted", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{
				"model": "gpt-5.5",
				"input": []any{
					map[string]any{"type": "custom_tool_call", "id": "ctc_old", "call_id": "call_old", "name": "exec", "input": "echo \"hello\"\n"},
					map[string]any{"type": "custom_tool_call_output", "call_id": "call_old", "output": "hello\n"},
				},
			}
			if tc.tools != "" {
				request["tools"] = json.RawMessage(tc.tools)
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			adapted, mapping, err := AdaptOpenAIResponsesClientTools(body)
			require.NoError(t, err)
			call := gjson.GetBytes(adapted, "input.0")
			output := gjson.GetBytes(adapted, "input.1")
			require.Equal(t, "call_old", call.Get("call_id").String())
			require.Equal(t, call.Get("call_id").String(), output.Get("call_id").String())
			require.Equal(t, "hello\n", output.Get("output").String())
			if tc.lowered {
				require.Equal(t, "function_call", call.Get("type").String())
				require.Equal(t, "function_call_output", output.Get("type").String())
				require.Equal(t, "fc_old", call.Get("id").String())
				require.JSONEq(t, `{"input":"echo \"hello\"\n"}`, call.Get("arguments").String())
			} else {
				require.Equal(t, body, adapted)
			}
			if tc.name != "same custom" {
				require.False(t, mapping.CustomTools["exec"])
				for _, tool := range gjson.GetBytes(adapted, "tools").Array() {
					require.NotEqual(t, "exec", tool.Get("name").String())
				}
			}
		})
	}
}

// TestAdaptOpenAIResponsesClientToolsKeepsNestedResultData replays a function
// result containing JSON that resembles a retired custom call. Those fields
// are tool data and must keep their JSON values and types during conversion.
func TestAdaptOpenAIResponsesClientToolsKeepsNestedResultData(t *testing.T) {
	body := []byte(`{"tools":[{"type":"custom","name":"other"}],"input":[{"type":"function_call_output","call_id":"call_lookup","output":{"type":"custom_tool_call","id":"ctc_data","call_id":"data","name":"exec","input":"sample"}}]}`)
	adapted, _, err := AdaptOpenAIResponsesClientTools(body)
	require.NoError(t, err)
	require.JSONEq(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(adapted, "input").Raw)
}

// TestAdaptOpenAIResponsesClientToolsLeavesNamespaceOnlyBodyUnchanged screens
// a request containing a namespace with an ordinary function. Its bytes remain
// equal, and all returned client-tool mapping collections are empty.
func TestAdaptOpenAIResponsesClientToolsLeavesNamespaceOnlyBodyUnchanged(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"tools": [{"type": "namespace", "name": "code_tools", "tools": [{"type": "function", "name": "run"}]}],
		"tool_choice": "auto"
	}`)

	adapted, mapping, err := AdaptOpenAIResponsesClientTools(body)

	require.NoError(t, err)
	require.Equal(t, body, adapted)
	require.Empty(t, mapping.CustomTools)
	require.Empty(t, mapping.NamespaceTools)
	require.False(t, mapping.ToolSearch)
}

// TestAdaptOpenAIResponsesClientToolsRejectsTrailingData appends garbage or a
// second document to a custom-tool request. Both cases return a trailing-input
// error, source bytes, and an empty mapping.
func TestAdaptOpenAIResponsesClientToolsRejectsTrailingData(t *testing.T) {
	tests := map[string][]byte{
		"trailing garbage":     append(openAIClientToolsRequest(false), []byte(` garbage`)...),
		"second JSON document": append(openAIClientToolsRequest(false), []byte(` {"model":"other"}`)...),
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			adapted, mapping, err := AdaptOpenAIResponsesClientTools(body)

			require.ErrorContains(t, err, "decode OpenAI Responses client tools trailing data")
			require.Equal(t, body, adapted)
			require.Empty(t, mapping)
		})
	}
}

// TestResponsesFunctionUpstreamsLowerToolSearchDiscoveryOutput runs OpenAI and
// Grok adaptation on client discovery output. Both produce function output
// containing the discovered tool list and remove discovery-only fields.
func TestResponsesFunctionUpstreamsLowerToolSearchDiscoveryOutput(t *testing.T) {
	body := []byte(`{"tools":[{"type":"tool_search"}],"input":[{"type":"tool_search_output","call_id":"search_1","tools":[{"type":"namespace","name":"github"}],"status":"completed","execution":"client"}]}`)
	adapters := map[string]func([]byte) ([]byte, ResponsesClientToolMapping, error){
		"OpenAI API-key": AdaptOpenAIResponsesClientTools,
		"Grok": func(body []byte) ([]byte, ResponsesClientToolMapping, error) {
			return AdaptResponsesClientToolsJSON(body, "Grok")
		},
	}
	for name, adapt := range adapters {
		t.Run(name, func(t *testing.T) {
			adapted, mapping, err := adapt(body)
			require.NoError(t, err)
			require.True(t, mapping.ToolSearch)
			require.Equal(t, "function_call_output", gjson.GetBytes(adapted, "input.0.type").String())
			require.JSONEq(t, `[{"name":"github","type":"namespace"}]`, gjson.GetBytes(adapted, "input.0.output").String())
			require.False(t, gjson.GetBytes(adapted, "input.0.tools").Exists())
			require.False(t, gjson.GetBytes(adapted, "input.0.status").Exists())
			require.False(t, gjson.GetBytes(adapted, "input.0.execution").Exists())
		})
	}
}

// openAIClientToolsRequest builds a Responses fixture with two custom tools
// and the supplied streaming flag.
func openAIClientToolsRequest(stream bool) []byte {
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	return []byte(`{"model":"gpt-5.4","input":"fix it","stream":` + streamValue + `,"tools":[{"type":"custom","name":"exec"},{"type":"custom","name":"apply_patch"}]}`)
}
