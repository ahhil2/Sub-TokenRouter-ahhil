package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemanticErrorPolicy(test *testing.T) {
	for _, scenario := range []struct {
		name  string
		raw   string
		valid bool
	}{
		{"missing", `{}`, true},
		{"cleared", `{"semantic_error":null}`, true},
		{"disabled", `{"semantic_error":{"enabled":false}}`, true},
		{"valid", `{"semantic_error":{"enabled":true,"texts":["Upstream unavailable"],"cooldown_minutes":5}}`, true},
		{"no texts", `{"semantic_error":{"enabled":true,"texts":[],"cooldown_minutes":5}}`, false},
		{"short", `{"semantic_error":{"enabled":true,"texts":["error"],"cooldown_minutes":5}}`, false},
		{"wrong type", `{"semantic_error":{"enabled":"true"}}`, false},
		{"fraction", `{"semantic_error":{"enabled":true,"texts":["Upstream unavailable"],"cooldown_minutes":1.5}}`, false},
		{"zero", `{"semantic_error":{"enabled":true,"texts":["Upstream unavailable"],"cooldown_minutes":0}}`, false},
		{"excess", `{"semantic_error":{"enabled":true,"texts":["Upstream unavailable"],"cooldown_minutes":1441}}`, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			var extra map[string]any
			require.NoError(test, json.Unmarshal([]byte(scenario.raw), &extra))
			err := ValidateSemanticErrorExtra(extra)
			if scenario.valid {
				require.NoError(test, err)
			} else {
				require.Error(test, err)
				require.False(test, (&Record{Extra: extra}).SemanticErrorPolicy().Enabled)
				_, err = BuildProviderForCreate(&CreateProviderInput{}, extra, CreationOptions{})
				require.Error(test, err)
				admin := &Admin{}
				_, err = admin.UpdateProvider(context.Background(), 7, &UpdateProviderInput{Extra: extra})
				require.Error(test, err)
				require.Error(test, admin.UpdateProviderExtra(context.Background(), 7, extra))
				_, err = admin.BulkUpdateProviders(context.Background(), &BulkUpdateProvidersInput{Extra: extra})
				require.Error(test, err)
			}
		})
	}
	policy := SemanticErrorPolicy{Enabled: true, Texts: []string{strings.Repeat("x", 513)}, CooldownMinutes: 1}
	require.Error(test, ValidateSemanticErrorExtra(map[string]any{SemanticErrorExtraKey: policy}))
	policy.Texts = make([]string, 11)
	for index := range policy.Texts {
		policy.Texts[index] = "Upstream unavailable"
	}
	require.Error(test, ValidateSemanticErrorExtra(map[string]any{SemanticErrorExtraKey: policy}))
	policy.Texts = []string{" Upstream  unavailable \n please retry "}
	record := &Record{Extra: map[string]any{SemanticErrorExtraKey: policy}}
	require.Equal(test, []string{"Upstream unavailable please retry"}, record.SemanticErrorPolicy().Texts)
}
