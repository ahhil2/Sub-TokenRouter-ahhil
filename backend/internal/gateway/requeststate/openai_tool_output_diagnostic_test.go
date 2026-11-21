package requeststate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// openai_tool_output_diagnostic_test.go verifies that rejected tool histories
// produce bounded structural observations without exposing their values.
// Fixtures distinguish recognized content from supported metadata, keep
// ambiguous JSON unclassified, and protect the caller's original request bytes.

// TestDescribeOpenAIToolOutputFailure_BoundedSamples puts missing IDs after a
// long valid history. Diagnostics must retain late failures, report complete
// counts, and remain small while leaving the supplied request bytes unchanged.
func TestDescribeOpenAIToolOutputFailure_BoundedSamples(t *testing.T) {
	for _, missingCount := range []int{1, 45} {
		t.Run(fmt.Sprintf("missing_%d", missingCount), func(t *testing.T) {
			input := make([]any, 0, 100)
			for range 5 {
				input = append(input, map[string]any{"type": "message", "content": "PRIVATE_MESSAGE"})
			}
			for range 41 {
				input = append(input, map[string]any{"type": "function_call_output", "call_id": "PRIVATE_ID", "output": "PRIVATE_OUTPUT"})
			}
			for range missingCount {
				input = append(input, map[string]any{
					"type": "function_call_output", "call_id": nil,
					"id": "PRIVATE_ID", "tool_call_id": map[string]any{"PRIVATE_KEY": "PRIVATE_VALUE"},
					"namespace": "codex_app", "name": "send_message_to_thread",
					"output": `<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>`,
				})
			}
			for range 3 {
				input = append(input, map[string]any{"type": "tool_search_output", "execution": "server", "call_id": nil})
			}
			body, err := json.Marshal(map[string]any{"input": input})
			require.NoError(t, err)
			original := bytes.Clone(body)
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
			require.NoError(t, err)
			require.Equal(t, original, body)
			require.EqualValues(t, 49+missingCount, gjson.GetBytes(encoded, "input_item_count").Int())
			require.EqualValues(t, 44+missingCount, gjson.GetBytes(encoded, "tool_item_count").Int())
			require.EqualValues(t, missingCount, gjson.GetBytes(encoded, "missing_call_id_count").Int())
			require.EqualValues(t, 3, gjson.GetBytes(encoded, "hosted_search_count").Int())
			require.EqualValues(t, 12+missingCount, gjson.GetBytes(encoded, "items_omitted").Int())
			samples := gjson.GetBytes(encoded, "items").Array()
			require.Len(t, samples, 32)
			require.EqualValues(t, 46, samples[0].Get("index").Int())
			for i := range min(missingCount, len(samples)) {
				require.True(t, samples[i].Get("missing_call_id").Bool())
				require.EqualValues(t, 46+i, samples[i].Get("index").Int())
				require.Equal(t, "nonempty", samples[i].Get("id_state").String())
				require.Equal(t, "invalid_type", samples[i].Get("tool_call_id_state").String())
				require.Equal(t, "delegation", samples[i].Get("bootstrap_shape.content_format").String())
			}
			// The fixed schema should fit ordinary log records regardless of history
			// length; this ceiling catches accidental publication of raw request values.
			require.Less(t, len(encoded), 16*1024)
			t.Logf("serialized diagnostic: %d bytes for %d samples", len(encoded), len(samples))
			require.NotContains(t, string(encoded), "PRIVATE_")
		})
	}
}

// TestDescribeOpenAIToolOutputFailure_AmbiguousJSON prevents a decoder from
// inventing bootstrap evidence by selecting one duplicate value. Invalid or
// non-object requests also produce fixed unavailable labels without content.
func TestDescribeOpenAIToolOutputFailure_AmbiguousJSON(t *testing.T) {
	cases := []struct {
		name   string // input ambiguity label
		body   string // synthetic request representation
		reason string // unavailable check category
	}{
		{"duplicate call ID", `{"input":[{"type":"function_call_output","call_id":null,"call_id":"PRIVATE_ID","output":"PRIVATE_OUTPUT"}]}`, "json_members_not_unique_or_invalid"},
		{"escaped duplicate", `{"input":[{"type":"function_call_output","call_id":null,"call_\u0069d":"PRIVATE_ID"}]}`, "json_members_not_unique_or_invalid"},
		{"duplicate namespace", `{"input":[{"type":"function_call_output","namespace":"codex_app","namespace":"PRIVATE_NAMESPACE","name":"create_thread","output":"<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>"}]}`, "json_members_not_unique_or_invalid"},
		{"duplicate name", `{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","name":"PRIVATE_NAME","output":"<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>"}]}`, "json_members_not_unique_or_invalid"},
		{"duplicate output", `{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"PRIVATE_OUTPUT","out\u0070ut":"<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>"}]}`, "json_members_not_unique_or_invalid"},
		{"truncated", `{"input":[{"type":"function_call_output","output":"PRIVATE_OUTPUT"}`, "json_members_not_unique_or_invalid"},
		{"array root", `[{"output":"PRIVATE_OUTPUT"}]`, "request_not_object"},
		{"null root", `null`, "request_not_object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			original := bytes.Clone(body)
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
			require.NoError(t, err)
			require.Equal(t, original, body)
			require.Equal(t, tc.reason, gjson.GetBytes(encoded, "bootstrap.delegation.result").String())
			require.EqualValues(t, -1, gjson.GetBytes(encoded, "bootstrap.delegation.candidate_count").Int())
			require.EqualValues(t, -1, gjson.GetBytes(encoded, "bootstrap.automation.blocked_index").Int())
			items := gjson.GetBytes(encoded, "items").Array()
			if tc.reason == "json_members_not_unique_or_invalid" && tc.name != "truncated" {
				require.Len(t, items, 1)
			}
			for _, item := range items {
				require.Equal(t, "not_checked", item.Get("bootstrap_candidate").String())
				for _, field := range []string{"namespace", "name", "output_type", "content_format"} {
					require.Equal(t, "not_checked", item.Get("bootstrap_shape."+field).String())
				}
			}
			require.NotContains(t, string(encoded), "PRIVATE_")
		})
	}
}

// TestDescribeOpenAIToolOutputFailure_BootstrapShapeHistory compares ordinary
// output, a delegation envelope with unsupported metadata, and a recognized
// delegation item. Content recognition must explain the mismatch without
// changing candidate counts, call association, or the supplied request.
func TestDescribeOpenAIToolOutputFailure_BootstrapShapeHistory(t *testing.T) {
	delegation := `<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>`
	body, err := json.Marshal(map[string]any{
		"input": []any{
			map[string]any{
				"type": "function_call_output", "output": "PRIVATE_ORDINARY_OUTPUT",
				"id": "PRIVATE_ITEM_ID", "tool_call_id": "PRIVATE_TOOL_CALL_ID",
			},
			map[string]any{
				"type": "function_call_output", "namespace": "PRIVATE_NAMESPACE",
				"name": "PRIVATE_TOOL", "output": delegation,
			},
			map[string]any{
				"type": "function_call_output", "namespace": "codex_tui",
				"name": "send_message_to_thread", "output": delegation,
			},
		},
	})
	require.NoError(t, err)
	original := bytes.Clone(body)
	encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
	require.NoError(t, err)
	require.Equal(t, original, body)
	require.NotContains(t, string(encoded), "PRIVATE_")
	require.EqualValues(t, 3, gjson.GetBytes(encoded, "input_item_count").Int())
	require.EqualValues(t, 3, gjson.GetBytes(encoded, "missing_call_id_count").Int())
	require.EqualValues(t, 1, gjson.GetBytes(encoded, "bootstrap.delegation.candidate_count").Int())
	require.Equal(t, "tool_history_disallowed", gjson.GetBytes(encoded, "bootstrap.delegation.result").String())
	require.EqualValues(t, 0, gjson.GetBytes(encoded, "bootstrap.delegation.blocked_index").Int())
	items := gjson.GetBytes(encoded, "items").Array()
	require.Len(t, items, 3)
	for index, expected := range []struct {
		candidate string // complete bootstrap classification
		namespace string // supported namespace classification
		name      string // supported tool classification
		content   string // independent output format
	}{
		{"none", "missing", "missing", "other"},
		{"none", "other", "other", "delegation"},
		{"delegation", "codex_tui", "send_message_to_thread", "delegation"},
	} {
		require.EqualValues(t, index, items[index].Get("index").Int())
		require.Equal(t, expected.candidate, items[index].Get("bootstrap_candidate").String())
		require.Equal(t, expected.namespace, items[index].Get("bootstrap_shape.namespace").String())
		require.Equal(t, expected.name, items[index].Get("bootstrap_shape.name").String())
		require.Equal(t, expected.content, items[index].Get("bootstrap_shape.content_format").String())
		require.Equal(t, "string", items[index].Get("bootstrap_shape.output_type").String())
		require.Equal(t, "missing", items[index].Get("call_id_state").String())
		require.True(t, items[index].Get("missing_call_id").Bool())
	}
	require.Equal(t, "nonempty", items[0].Get("id_state").String())
	require.Equal(t, "nonempty", items[0].Get("tool_call_id_state").String())
}

// TestDescribeOpenAIToolOutputFailure_MetadataStates varies tool metadata while
// retaining a valid delegation envelope. Exact known labels and malformed
// wrappers stay distinguishable, and unsupported labels never become log values
// or gain bootstrap eligibility through whitespace normalization.
func TestDescribeOpenAIToolOutputFailure_MetadataStates(t *testing.T) {
	for _, field := range []string{"namespace", "name"} {
		t.Run(field, func(t *testing.T) {
			known := "codex_app"
			if field == "name" {
				known = "create_thread"
			}
			for _, tc := range []struct {
				name    string // metadata shape scenario
				value   any    // private fixture value
				present bool   // include the tested member
				state   string // expected fixed classification
			}{
				{"missing", nil, false, "missing"},
				{"null", nil, true, "null"},
				{"empty", "", true, "empty"},
				{"blank", " \t", true, "blank"},
				{"padded", " " + known + " ", true, "other"},
				{"unknown", "PRIVATE_METADATA", true, "other"},
				{"boolean", true, true, "invalid_type"},
				{"number", 42, true, "invalid_type"},
				{"array", []any{"PRIVATE_METADATA"}, true, "invalid_type"},
				{"object", map[string]any{"PRIVATE_KEY": "PRIVATE_VALUE"}, true, "invalid_type"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					item := map[string]any{
						"type": "function_call_output", "namespace": "codex_app", "name": "create_thread",
						"output": `<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>`,
					}
					delete(item, field)
					if tc.present {
						item[field] = tc.value
					}
					body, err := json.Marshal(map[string]any{"input": []any{item}})
					require.NoError(t, err)
					original := bytes.Clone(body)
					encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
					require.NoError(t, err)
					require.Equal(t, original, body)
					require.NotContains(t, string(encoded), "PRIVATE_")
					require.Equal(t, tc.state, gjson.GetBytes(encoded, "items.0.bootstrap_shape."+field).String())
					require.Equal(t, "delegation", gjson.GetBytes(encoded, "items.0.bootstrap_shape.content_format").String())
					require.Equal(t, "none", gjson.GetBytes(encoded, "items.0.bootstrap_candidate").String())
					require.Zero(t, gjson.GetBytes(encoded, "bootstrap.delegation.candidate_count").Int())
				})
			}
		})
	}
}

// TestDescribeOpenAIToolOutputFailure_OutputShapes exercises JSON output types
// and the supported text formats through serialized diagnostics. Structural
// labels never expose nested content, and valid text remains distinguishable
// from malformed envelopes and ordinary tool results.
func TestDescribeOpenAIToolOutputFailure_OutputShapes(t *testing.T) {
	for _, tc := range []struct {
		name      string // output structure scenario
		tool      string // recognized tool metadata
		output    any    // private synthetic output
		present   bool   // include the output member
		valueType string // expected JSON value category
		format    string // expected content format
		candidate string // complete bootstrap classification
	}{
		{"missing", "create_thread", nil, false, "missing", "not_text", "none"},
		{"null", "create_thread", nil, true, "null", "not_text", "none"},
		{"boolean", "create_thread", true, true, "boolean", "not_text", "none"},
		{"number", "create_thread", 42, true, "number", "not_text", "none"},
		{"array", "create_thread", []any{map[string]any{"text": "PRIVATE_OUTPUT"}}, true, "array", "not_text", "none"},
		{"object", "create_thread", map[string]any{"PRIVATE_KEY": "PRIVATE_OUTPUT"}, true, "object", "not_text", "none"},
		{"ordinary", "create_thread", "PRIVATE_OUTPUT", true, "string", "other", "none"},
		{"invalid delegation", "create_thread", `<codex_delegation><input>PRIVATE_TASK</input></codex_delegation>`, true, "string", "other", "none"},
		{"delegation", "create_thread", `<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>`, true, "string", "delegation", "delegation"},
		{"automation", "automation_update", "Automation: PRIVATE_TITLE\nAutomation ID: PRIVATE_AUTOMATION\nAutomation memory: $CODEX_HOME/automations/PRIVATE_AUTOMATION/memory.md\nLast run: never\n\nPRIVATE_TASK", true, "string", "automation", "automation"},
		{"heartbeat", "automation_update", `<heartbeat><automation_id>PRIVATE_AUTOMATION</automation_id></heartbeat>`, true, "string", "heartbeat", "automation"},
		{"invalid heartbeat", "automation_update", `<heartbeat><automation_id>../PRIVATE_AUTOMATION</automation_id></heartbeat>`, true, "string", "other", "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{"type": "function_call_output", "namespace": "codex_app", "name": tc.tool}
			if tc.present {
				item["output"] = tc.output
			}
			body, err := json.Marshal(map[string]any{"input": []any{item}})
			require.NoError(t, err)
			original := bytes.Clone(body)
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
			require.NoError(t, err)
			require.Equal(t, original, body)
			require.NotContains(t, string(encoded), "PRIVATE_")
			require.Equal(t, "codex_app", gjson.GetBytes(encoded, "items.0.bootstrap_shape.namespace").String())
			require.Equal(t, tc.tool, gjson.GetBytes(encoded, "items.0.bootstrap_shape.name").String())
			require.Equal(t, tc.valueType, gjson.GetBytes(encoded, "items.0.bootstrap_shape.output_type").String())
			require.Equal(t, tc.format, gjson.GetBytes(encoded, "items.0.bootstrap_shape.content_format").String())
			require.Equal(t, tc.candidate, gjson.GetBytes(encoded, "items.0.bootstrap_candidate").String())
		})
	}
}

// TestDescribeOpenAIToolOutputFailure_AlternativeIdentifierStates supplies
// alternative identifier fields without a valid call_id. Their shape remains
// observable without publishing values, repairing the association, or reducing
// the missing-call-ID count.
func TestDescribeOpenAIToolOutputFailure_AlternativeIdentifierStates(t *testing.T) {
	for _, tc := range []struct {
		name    string // identifier shape scenario
		value   any    // private identifier fixture
		present bool   // include alternative identifier members
		state   string // expected fixed field state
	}{
		{"missing", nil, false, "missing"},
		{"null", nil, true, "null"},
		{"empty", "", true, "empty"},
		{"blank", " \t", true, "blank"},
		{"nonempty", "PRIVATE_IDENTIFIER", true, "nonempty"},
		{"number", 42, true, "invalid_type"},
		{"boolean", false, true, "invalid_type"},
		{"array", []any{"PRIVATE_IDENTIFIER"}, true, "invalid_type"},
		{"object", map[string]any{"PRIVATE_KEY": "PRIVATE_IDENTIFIER"}, true, "invalid_type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{"type": "function_call_output", "output": "PRIVATE_OUTPUT"}
			if tc.present {
				item["id"] = tc.value
				item["tool_call_id"] = tc.value
			}
			body, err := json.Marshal(map[string]any{"input": []any{item}})
			require.NoError(t, err)
			original := bytes.Clone(body)
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
			require.NoError(t, err)
			require.Equal(t, original, body)
			require.NotContains(t, string(encoded), "PRIVATE_")
			require.Equal(t, tc.state, gjson.GetBytes(encoded, "items.0.id_state").String())
			require.Equal(t, tc.state, gjson.GetBytes(encoded, "items.0.tool_call_id_state").String())
			require.Equal(t, "missing", gjson.GetBytes(encoded, "items.0.call_id_state").String())
			require.True(t, gjson.GetBytes(encoded, "items.0.missing_call_id").Bool())
			require.EqualValues(t, 1, gjson.GetBytes(encoded, "missing_call_id_count").Int())
		})
	}
}

// TestDescribeOpenAIToolOutputFailure_BootstrapBlockers distinguishes the known
// nullable-field and hosted-history cases. The diagnostic rechecks the supplied
// failure body while exposing candidate presence independently of its blocker.
func TestDescribeOpenAIToolOutputFailure_BootstrapBlockers(t *testing.T) {
	cases := []struct {
		name      string // synthetic failure condition
		callNull  bool   // candidate carries null
		prevNull  bool   // previous response null
		hosted    bool   // hosted history included
		reason    string // first preflight blocker
		index     int    // blocker input position
		candidate string // sampled candidate classification
	}{
		{"hosted search", false, false, true, "tool_history_disallowed", 1, "delegation"},
		{"candidate null", true, false, false, "candidate_call_id_type", 0, "delegation"},
		{"previous null", false, true, false, "previous_response_id_type", -1, "delegation"},
		{"root blocker first", true, true, true, "previous_response_id_type", -1, "delegation"},
		{"ready candidate", false, false, false, "would_normalize", -1, "delegation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := map[string]any{
				"type": "function_call_output", "namespace": "codex_app", "name": "create_thread",
				"output": `<codex_delegation><source_thread_id>PRIVATE_THREAD</source_thread_id><input>PRIVATE_TASK</input></codex_delegation>`,
			}
			if tc.callNull {
				candidate["call_id"] = nil
			}
			input := []any{candidate}
			if tc.hosted {
				input = append(input, map[string]any{"type": "tool_search_output", "execution": "server", "call_id": nil})
			}
			request := map[string]any{"input": input}
			if tc.prevNull {
				request["previous_response_id"] = nil
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure(body))
			require.NoError(t, err)
			require.Equal(t, tc.reason, gjson.GetBytes(encoded, "bootstrap.delegation.result").String())
			require.EqualValues(t, tc.index, gjson.GetBytes(encoded, "bootstrap.delegation.blocked_index").Int())
			require.EqualValues(t, 1, gjson.GetBytes(encoded, "bootstrap.delegation.candidate_count").Int())
			require.Equal(t, tc.candidate, gjson.GetBytes(encoded, "items.0.bootstrap_candidate").String())
			require.NotContains(t, string(encoded), "PRIVATE_")
		})
	}
}
