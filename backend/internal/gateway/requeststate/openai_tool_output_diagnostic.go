package requeststate

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// openai_tool_output_diagnostic.go describes the structure of rejected OpenAI
// tool histories using classifications, counts, and input positions. A missing
// call identifier can describe an ordinary invalid result or an unconverted
// Codex bootstrap item, so diagnostics include the existing bootstrap checks
// alongside field states. They inspect the body at tool validation time; these
// observations do not reconstruct earlier transformations or upstream events.
// Bootstrap metadata and output format are classified independently so an
// unsupported wrapper can be distinguished from unrecognized content.
//
// Only failure logging calls this collector. Missing-ID samples also retain
// bounded names, namespaces, and top-level field types so operators can identify
// unsupported events. User text, tool arguments, output, call identifiers, and
// nested metadata values remain excluded from the observations. Samples
// prioritize outputs missing call IDs, while aggregate counts include every
// inspected item. Duplicate members disable decoded bootstrap observations
// because map decoding and the validator's raw scan could otherwise describe
// different identities. Returned JSON-style containers are detached metadata
// and remain immutable after publication to asynchronous logging sinks.

const (
	// openAIToolDiagnosticItemLimit bounds serialized samples while aggregate
	// counts retain evidence about tool histories longer than the sample budget.
	openAIToolDiagnosticItemLimit = 32

	// Structural limits bound retained labels and field descriptions for each
	// missing-ID sample, including JSON escaping in the serialized size budget.
	openAIToolDiagnosticFieldLimit = 32
	openAIToolDiagnosticLabelBytes = 256
	openAIToolDiagnosticShapeBytes = 4 * 1024
)

// DescribeOpenAIToolOutputFailure collects safe metadata from the rejected body.
// Call it after missing-call-ID validation fails and only when the diagnostic
// will be logged. The result contains JSON-style maps and arrays with
// classifications, counts, and boolean observations. Missing-ID samples include
// bounded name and namespace strings plus top-level field names and types;
// callers must treat these strings as client-supplied diagnostic data.
//
// Bootstrap checks reuse the normalizer's predicates on this body and report
// their first blocker. A successful check describes what normalization would
// do now, not what happened earlier. Samples contain at most thirty-two tool
// items, prioritizing client results without IDs before other history; their
// original input indices remain available when that priority changes order.
// Alternative identifier fields report presence only; their existence does
// not establish a valid call association or authorize identifier recovery.
func DescribeOpenAIToolOutputFailure(body []byte) map[string]any {
	root := gjson.ParseBytes(body)
	bootstrap, decodedInput := inspectOpenAIDiagnosticBootstrap(body)
	diagnostic := map[string]any{
		"body_stage":                 "tool_output_validation",
		"previous_response_id_state": openAIToolDiagnosticIDState(root.Get("previous_response_id")),
		"bootstrap":                  bootstrap,
	}
	missingItems := make([]any, 0, openAIToolDiagnosticItemLimit)
	otherItems := make([]any, 0, openAIToolDiagnosticItemLimit)
	inputCount, toolCount, missingCount, hostedCount := 0, 0, 0, 0
	input := root.Get("input")
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			index := inputCount
			inputCount++
			if !item.IsObject() {
				return true
			}
			typ := openAIToolDiagnosticType(item.Get("type"))
			if typ == "" {
				return true
			}
			toolCount++
			rawType := item.Get("type").String()
			execution := openAIToolDiagnosticExecution(item.Get("execution"))
			hosted := (typ == "tool_search_call" || typ == "tool_search_output") && execution == "server"
			if hosted {
				hostedCount++
			}
			requiresID := openai.IsCodexToolCallOutputItemType(rawType) && !hosted
			missingID := requiresID && strings.TrimSpace(item.Get("call_id").String()) == ""
			if missingID {
				missingCount++
			}

			// Keep separate bounded samples so late missing-ID results can displace
			// earlier ordinary history without allocating one record for every input item.
			if (missingID && len(missingItems) == openAIToolDiagnosticItemLimit) ||
				(!missingID && len(otherItems) == openAIToolDiagnosticItemLimit) {
				return true
			}
			candidate := "not_checked"
			var decodedItem any
			if index < len(decodedInput) {
				decodedItem = decodedInput[index]
				candidate = openAIToolDiagnosticBootstrapCandidate(decodedItem)
			}
			sample := map[string]any{
				"index":                   index,
				"type":                    typ,
				"execution":               execution,
				"call_id_state":           openAIToolDiagnosticIDState(item.Get("call_id")),
				"id_state":                openAIToolDiagnosticIDState(item.Get("id")),
				"tool_call_id_state":      openAIToolDiagnosticIDState(item.Get("tool_call_id")),
				"bootstrap_candidate":     candidate,
				"bootstrap_shape":         openAIToolDiagnosticBootstrapShape(decodedItem),
				"requires_client_call_id": requiresID,
				"missing_call_id":         missingID,
			}
			if missingID {
				sample["item_shape"] = openAIToolDiagnosticItemShape(decodedItem)
				missingItems = append(missingItems, sample)
			} else {
				otherItems = append(otherItems, sample)
			}
			return true
		})
	}
	remaining := openAIToolDiagnosticItemLimit - len(missingItems)
	if len(otherItems) > remaining {
		otherItems = otherItems[:remaining]
	}
	items := append(missingItems, otherItems...)
	diagnostic["input_item_count"] = inputCount
	diagnostic["tool_item_count"] = toolCount
	diagnostic["missing_call_id_count"] = missingCount
	diagnostic["hosted_search_count"] = hostedCount
	diagnostic["items_omitted"] = toolCount - len(items)
	diagnostic["items"] = items
	return diagnostic
}

// openAIToolDiagnosticItemShape describes one failed item's names and fields.
// Callers supply an item decoded from a request with unique JSON members;
// unavailable decoding returns a not_checked marker. String names are retained
// with byte limits, while other field values contribute their JSON types only.
//
// Fields are sorted for repeatable observations and selected within count and
// serialized-byte budgets. Omitted counts describe the remaining fields, even
// when the budget is reached before the count limit. Nested objects and arrays
// remain type labels so diagnostic logging cannot traverse their payloads.
func openAIToolDiagnosticItemShape(raw any) map[string]any {
	item, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{"state": "not_checked"}
	}
	shape := map[string]any{
		"field_count": len(item), "fields_omitted": len(item), "fields": []any{},
	}
	for _, key := range []string{"name", "namespace"} {
		if value, ok := item[key].(string); ok {
			openAIToolDiagnosticSetLabel(shape, key, value)
		}
	}
	fields := make([]any, 0, min(len(item), openAIToolDiagnosticFieldLimit))
	for _, key := range slices.Sorted(maps.Keys(item)) {
		if len(fields) == openAIToolDiagnosticFieldLimit {
			break
		}
		field := map[string]any{"type": openAIToolDiagnosticValueType(item[key], true)}
		openAIToolDiagnosticSetLabel(field, "name", key)
		shape["fields"] = append(fields, field)
		shape["fields_omitted"] = len(item) - len(fields) - 1
		encoded, err := json.Marshal(shape)
		// Escaped client labels can exceed their source length. Keep the previous
		// complete field list when the serialized budget cannot hold the next field.
		if err != nil || len(encoded) > openAIToolDiagnosticShapeBytes {
			break
		}
		fields = append(fields, field)
	}
	shape["fields"] = fields
	shape["fields_omitted"] = len(item) - len(fields)
	return shape
}

// openAIToolDiagnosticSetLabel copies a bounded client label into diagnostic data.
// UTF-8 code points remain intact, and control characters become replacement
// characters so names can be stored in PostgreSQL JSON and viewed in logs.
// Truncation and sanitization flags distinguish modified labels from exact
// values. The destination contains freshly built strings suitable for queued
// logging and retains no reference to the source request's storage.
func openAIToolDiagnosticSetLabel(destination map[string]any, key, value string) {
	label := make([]byte, 0, min(len(value), openAIToolDiagnosticLabelBytes))
	for _, char := range value {
		// Replace controls before applying the byte limit so PostgreSQL can store
		// NUL-bearing labels and log viewers receive printable text.
		if unicode.IsControl(char) {
			char = unicode.ReplacementChar
			destination[key+"_sanitized"] = true
		}
		// Limit by encoded bytes while keeping each retained code point complete.
		if len(label)+utf8.RuneLen(char) > openAIToolDiagnosticLabelBytes {
			destination[key+"_truncated"] = true
			break
		}
		label = utf8.AppendRune(label, char)
	}
	destination[key] = string(label)
}

// inspectOpenAIDiagnosticBootstrap prepares shared bootstrap checks for one body.
// Ambiguous JSON or invalid UTF-8 disables candidate observations so decoded
// map identities cannot contradict the validator's raw-field interpretation. The
// normalizer's unique-member check and number-preserving decoding are reused
// here on the failure path. Successfully decoded items support candidate
// recognition alongside raw field-state scanning because unique members give
// both views the same field identities. The decoded input is transient and
// must never be published as logging metadata; callers extract classifications
// and bounded name and field descriptions from individual failed items.
func inspectOpenAIDiagnosticBootstrap(body []byte) (map[string]any, []any) {
	if !utf8.Valid(body) || !HasUniqueJSONMembers(body) {
		return unavailableOpenAIDiagnosticBootstrap("not_unique_or_invalid", "json_members_not_unique_or_invalid"), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil || request == nil {
		return unavailableOpenAIDiagnosticBootstrap("unique", "request_not_object"), nil
	}
	input, _ := request["input"].([]any)
	return map[string]any{
		"json_members": "unique",
		"automation":   inspectOpenAIDiagnosticBootstrapKind(request, IsCodexAutomationCandidate, false),
		"delegation":   inspectOpenAIDiagnosticBootstrapKind(request, IsCodexDelegationCandidate, true),
	}, input
}

// unavailableOpenAIDiagnosticBootstrap marks checks that lack an unambiguous
// request object. Negative counts and indices mean no candidate or blocker
// determination was made; they do not assert that candidates were absent.
func unavailableOpenAIDiagnosticBootstrap(members, reason string) map[string]any {
	return map[string]any{
		"json_members": members,
		"automation": map[string]any{
			"result": reason, "blocked_index": -1, "candidate_count": -1,
		},
		"delegation": map[string]any{
			"result": reason, "blocked_index": -1, "candidate_count": -1,
		},
	}
}

// inspectOpenAIDiagnosticBootstrapKind reports the normalizer's first blocker
// and counts recognized candidates independently of that blocker. Callers
// supply a request with unique members and the same stable candidate predicate
// used for normalization. Root-level blockers can precede candidate inspection,
// so a separate count distinguishes absent candidates from candidates rejected
// by surrounding context. A ready preflight becomes would_normalize when that
// count is positive; otherwise it becomes no_candidate. These observations
// describe the supplied body state without rewriting it or claiming prior execution.
func inspectOpenAIDiagnosticBootstrapKind(request map[string]any, isCandidate func(map[string]any) bool, allowHistoricalContext bool) map[string]any {
	_, reason, index := inspectCodexBootstrapContext(request, isCandidate, allowHistoricalContext)
	candidateCount := 0
	input, _ := request["input"].([]any)
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if ok && isCandidate(item) {
			candidateCount++
		}
	}
	if reason == "ready" {
		reason = "no_candidate"
		if candidateCount > 0 {
			reason = "would_normalize"
		}
	}
	return map[string]any{
		"result": reason, "blocked_index": index, "candidate_count": candidateCount,
	}
}

// openAIToolDiagnosticBootstrapCandidate classifies recognized bootstrap shapes
// with the normalizer's predicates. It returns fixed labels even when names,
// namespaces, or output text contain arbitrary client-supplied strings.
func openAIToolDiagnosticBootstrapCandidate(raw any) string {
	item, ok := raw.(map[string]any)
	if !ok {
		return "none"
	}
	if IsCodexDelegationCandidate(item) {
		return "delegation"
	}
	if IsCodexAutomationCandidate(item) {
		return "automation"
	}
	return "none"
}

// openAIToolDiagnosticBootstrapShape separates metadata recognition from output
// format for one sampled input item. Callers supply an item decoded from a
// unique-member request; unavailable decoding produces not_checked labels.
//
// Bootstrap normalization requires supported tool metadata and supported text,
// so a candidate=none result cannot identify the mismatched part. These
// independent classifications retain that distinction using fixed values.
// Recognized content alone neither proves its origin nor permits conversion;
// the existing candidate and whole-request checks still govern normalization.
func openAIToolDiagnosticBootstrapShape(raw any) map[string]any {
	item, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{
			"namespace": "not_checked", "name": "not_checked",
			"output_type": "not_checked", "content_format": "not_checked",
		}
	}
	output, exists := item["output"]
	return map[string]any{
		"namespace":      openAIToolDiagnosticKnownLabel(item, "namespace", "codex_app", "codex_tui", "agent"),
		"name":           openAIToolDiagnosticKnownLabel(item, "name", "create_thread", "send_message_to_thread", "automation_update", "goal_objective", "codex_apps_open_page", "codex_apps_selected_page_part_1_of_1"),
		"output_type":    openAIToolDiagnosticValueType(output, exists),
		"content_format": openAIToolDiagnosticBootstrapContent(output),
	}
}

// openAIToolDiagnosticKnownLabel classifies one metadata field against fixed
// allowed spellings. Unknown strings become other, while absent, null, empty,
// blank, and nonstring fields retain separate states for compatibility diagnosis.
func openAIToolDiagnosticKnownLabel(item map[string]any, key string, allowed ...string) string {
	value, exists := item[key]
	if !exists {
		return "missing"
	}
	if value == nil {
		return "null"
	}
	label, ok := value.(string)
	if !ok {
		return "invalid_type"
	}
	if slices.Contains(allowed, label) {
		// Log a detached allowlisted label so queued metadata cannot retain text
		// from the rejected request, even if its decoder changes string allocation.
		return strings.Clone(label)
	}
	if label == "" {
		return "empty"
	}
	if strings.TrimSpace(label) == "" {
		return "blank"
	}
	return "other"
}

// openAIToolDiagnosticValueType reports a decoded JSON value's structural type.
// The presence flag distinguishes an omitted member from explicit null; content
// and container members are excluded from the returned fixed category.
func openAIToolDiagnosticValueType(value any, exists bool) string {
	if !exists {
		return "missing"
	}
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	default:
		return "other"
	}
}

// openAIToolDiagnosticBootstrapContent recognizes supported text independently
// of the item's tool metadata. Nonstring output is not_text; other means the
// string failed all existing format validators, not that it is ordinary tool
// output or safe to convert into a user message.
func openAIToolDiagnosticBootstrapContent(output any) string {
	text, ok := output.(string)
	if !ok {
		return "not_text"
	}
	if ValidCodexDelegationEnvelope(text) {
		return "delegation"
	}
	if ValidCodexAutomationBootstrap(text) {
		return "automation"
	}
	if ValidCodexAutomationHeartbeat(text) {
		return "heartbeat"
	}
	return "other"
}

// openAIToolDiagnosticIDState labels identifier presence and string shape.
// It never returns identifier text, length, or a hash that could correlate
// user-provided identities beyond the request's existing diagnostic context.
func openAIToolDiagnosticIDState(value gjson.Result) string {
	if !value.Exists() {
		return "missing"
	}
	if value.Type == gjson.Null {
		return "null"
	}
	if value.Type != gjson.String {
		return "invalid_type"
	}
	if value.Str == "" {
		return "empty"
	}
	if strings.TrimSpace(value.Str) == "" {
		return "blank"
	}
	return "nonempty"
}

// openAIToolDiagnosticExecution keeps exact supported execution labels for logs.
// Other strings become a fixed category, including whitespace-padded labels
// that the protocol's exact server-execution comparison does not accept.
func openAIToolDiagnosticExecution(value gjson.Result) string {
	if !value.Exists() {
		return "missing"
	}
	if value.Type == gjson.Null {
		return "null"
	}
	if value.Type != gjson.String {
		return "invalid_type"
	}
	switch value.Str {
	case "server":
		return "server"
	case "client":
		return "client"
	default:
		return "other"
	}
}

// openAIToolDiagnosticType selects tool-related items and replaces unknown
// tool labels with fixed categories. Non-tool items are counted at the input
// level but do not consume sample slots or expose arbitrary type strings.
func openAIToolDiagnosticType(value gjson.Result) string {
	if value.Type != gjson.String {
		return ""
	}
	typ := strings.TrimSpace(value.Str)
	switch typ {
	case "tool_call", "function_call", "local_shell_call", "tool_search_call",
		"custom_tool_call", "mcp_tool_call", "computer_call", "shell_call", "apply_patch_call",
		"function_call_output", "tool_search_output", "custom_tool_call_output", "mcp_tool_call_output",
		"local_shell_call_output", "computer_call_output", "shell_call_output", "apply_patch_call_output",
		"item_reference":
		// JSON strings can reference the whole body buffer. Detach the allowlisted
		// label so queued diagnostic metadata cannot retain the rejected request text.
		return strings.Clone(typ)
	}
	if strings.HasSuffix(typ, "_call_output") {
		return "other_tool_output"
	}
	if strings.HasSuffix(typ, "_call") {
		return "other_tool_call"
	}
	return ""
}
