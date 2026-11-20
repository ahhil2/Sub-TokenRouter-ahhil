package requeststate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

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
				input = append(input, map[string]any{"type": "function_call_output", "call_id": nil, "output": "PRIVATE_OUTPUT"})
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
			}
			// The fixed schema should fit ordinary log records regardless of history
			// length; this ceiling catches accidental publication of raw request values.
			require.Less(t, len(encoded), 16*1024)
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
		{"truncated", `{"input":[{"type":"function_call_output","output":"PRIVATE_OUTPUT"}`, "json_members_not_unique_or_invalid"},
		{"array root", `[{"output":"PRIVATE_OUTPUT"}]`, "request_not_object"},
		{"null root", `null`, "request_not_object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(DescribeOpenAIToolOutputFailure([]byte(tc.body)))
			require.NoError(t, err)
			require.Equal(t, tc.reason, gjson.GetBytes(encoded, "bootstrap.delegation.result").String())
			require.EqualValues(t, -1, gjson.GetBytes(encoded, "bootstrap.delegation.candidate_count").Int())
			require.EqualValues(t, -1, gjson.GetBytes(encoded, "bootstrap.automation.blocked_index").Int())
			for _, item := range gjson.GetBytes(encoded, "items").Array() {
				require.Equal(t, "not_checked", item.Get("bootstrap_candidate").String())
			}
			require.NotContains(t, string(encoded), "PRIVATE_")
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
