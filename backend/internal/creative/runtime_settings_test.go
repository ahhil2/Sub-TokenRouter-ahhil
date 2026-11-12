package creative

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// runtime_settings_test.go verifies model and prompt settings admission from stored values.
// Persisted model settings fail closed after decoding or validation failure,
// while an absent feature switch defaults to enabled. Map-backed readers
// isolate these parsing results from external settings storage and allow an
// existing model value to change between reads.

// creativeRuntimeSettingsFixture returns stored values or ErrSettingNotFound
// for missing keys.
type creativeRuntimeSettingsFixture struct {
	values   map[string]string // mutable stored setting strings
	batchErr error             // injected batch read failure
}

// TestSettingService_IsCreativeEnabled reads a stored false feature value and
// then an absent value. The first disables creative and the second returns the
// enabled default through the map reader.
func TestSettingService_IsCreativeEnabled(t *testing.T) {
	repo := &creativeRuntimeSettingsFixture{values: map[string]string{SettingKeyCreativeEnabled: "false"}}
	svc := NewRuntimeSettings(repo, settings.ErrSettingNotFound)
	require.False(t, svc.IsCreativeEnabled(context.Background()))

	// A missing key defaults to enabled.
	repo = &creativeRuntimeSettingsFixture{values: map[string]string{}}
	svc = NewRuntimeSettings(repo, settings.ErrSettingNotFound)
	require.True(t, svc.IsCreativeEnabled(context.Background()))
}

// TestSettingServiceGetCreativeModelSettings reads one valid model JSON entry,
// then replaces it with malformed JSON. It checks the normalized entry first
// and the empty allowlist on the second read.
func TestSettingServiceGetCreativeModelSettings(t *testing.T) {
	repo := &creativeRuntimeSettingsFixture{values: map[string]string{
		SettingKeyCreativeModelSettings: `[{"group_id":12,"model":"gpt-image-2","operations":["generate"]}]`,
	}}
	svc := NewRuntimeSettings(repo, settings.ErrSettingNotFound)
	require.Equal(t, []CreativeModelSetting{{
		GroupID:    12,
		Model:      "gpt-image-2",
		Operations: []string{CreativeOperationGenerate},
	}}, svc.GetCreativeModelSettings(context.Background()))

	repo.values[SettingKeyCreativeModelSettings] = "not-json"
	require.Empty(t, svc.GetCreativeModelSettings(context.Background()))
}

// GetValue returns the fixture stored string for key or ErrSettingNotFound.
// The map remains shared with the test so replacing a value affects the next
// read.
func (r *creativeRuntimeSettingsFixture) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", settings.ErrSettingNotFound
}

// TestParseCreativeModelSettingsFailsClosed supplies broken JSON, an
// empty-operation entry, and valid unordered operations. Invalid cases yield
// empty settings, while the valid case returns generate before edit.
func TestParseCreativeModelSettingsFailsClosed(t *testing.T) {
	require.Empty(t, ParseCreativeModelSettings("{broken"))
	require.Empty(t, ParseCreativeModelSettings(`[{"group_id":1,"model":"image","operations":[]}]`))

	parsed := ParseCreativeModelSettings(`[{
		"group_id": 7,
		"model": "gpt-image-2",
		"operations": ["edit", "generate"]
	}]`)
	require.Equal(t, []CreativeModelSetting{{
		GroupID:    7,
		Model:      "gpt-image-2",
		Operations: []string{"generate", "edit"},
	}}, parsed)
}

// GetMultiple returns the selected values from one fixture snapshot. A configured
// batch error prevents any values from being returned, matching failed storage
// reads used by prompt admission tests.
func (r *creativeRuntimeSettingsFixture) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.batchErr != nil {
		return nil, r.batchErr
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, exists := r.values[key]; exists {
			values[key] = value
		}
	}
	return values, nil
}

// TestCreativePromptRuntimeSettings checks configured routes and currency conversion defaults.
// Invalid routes and failed batch reads disable assistance, while invalid rates
// retain the configured route with the seven RMB conversion default.
func TestCreativePromptRuntimeSettings(t *testing.T) {
	repo := &creativeRuntimeSettingsFixture{values: map[string]string{
		SettingKeyCreativePromptGroupID:   "12",
		SettingKeyCreativePromptModel:     " vision-model ",
		billing.SettingKeyUSDExchangeRate: "7.25",
	}}
	runtime := NewRuntimeSettings(repo, settings.ErrSettingNotFound)
	require.Equal(t, PromptConfig{GroupID: 12, Model: "vision-model", USDExchangeRate: 7.25}, runtime.GetCreativePromptConfig(t.Context()))

	for _, rate := range []string{"", "0", "-2", "NaN", "+Inf", "garbage"} {
		repo.values[billing.SettingKeyUSDExchangeRate] = rate
		require.Equal(t, PromptConfig{GroupID: 12, Model: "vision-model", USDExchangeRate: 7}, runtime.GetCreativePromptConfig(t.Context()), rate)
	}
	for _, group := range []string{"", "0", "-1", "garbage", "9223372036854775808"} {
		repo.values[SettingKeyCreativePromptGroupID] = group
		require.Zero(t, runtime.GetCreativePromptConfig(t.Context()), group)
	}
	repo.values[SettingKeyCreativePromptGroupID] = "12"
	for _, model := range []string{"", " ", "vision-*", "vision-?", strings.Repeat("m", 201)} {
		repo.values[SettingKeyCreativePromptModel] = model
		require.Zero(t, runtime.GetCreativePromptConfig(t.Context()), model)
	}
	repo.values[SettingKeyCreativePromptModel] = "vision-model"
	repo.batchErr = errors.New("storage unavailable")
	require.Zero(t, runtime.GetCreativePromptConfig(t.Context()))
	require.Zero(t, NewRuntimeSettings(nil, settings.ErrSettingNotFound).GetCreativePromptConfig(t.Context()))
}

// TestCreativePromptSettingsParticipantPartialUpdates changes one field of an existing route.
// The participant validates the complete saved pair while persisting supplied
// fields, rejects incomplete disabling, and supports clearing both fields together.
func TestCreativePromptSettingsParticipantPartialUpdates(t *testing.T) {
	participant := SettingsParticipant()
	current := map[string]string{
		SettingKeyCreativePromptGroupID: "12",
		SettingKeyCreativePromptModel:   "vision-model",
	}
	change, err := participant.Prepare(t.Context(), settings.Fields{
		SettingKeyCreativePromptModel: json.RawMessage(`" updated-model "`),
	}, current)
	require.NoError(t, err)
	require.Equal(t, map[string]string{SettingKeyCreativePromptModel: "updated-model"}, change.Values)

	change, err = participant.Prepare(t.Context(), settings.Fields{
		SettingKeyCreativeEnabled: json.RawMessage(`false`),
	}, current)
	require.NoError(t, err)
	require.Equal(t, map[string]string{SettingKeyCreativeEnabled: "false"}, change.Values)

	_, err = participant.Prepare(t.Context(), settings.Fields{
		SettingKeyCreativePromptGroupID: json.RawMessage(`0`),
	}, current)
	require.ErrorContains(t, err, "Prompt assistance requires")

	change, err = participant.Prepare(t.Context(), settings.Fields{
		SettingKeyCreativePromptGroupID: json.RawMessage(`0`),
		SettingKeyCreativePromptModel:   json.RawMessage(`""`),
	}, current)
	require.NoError(t, err)
	require.Equal(t, map[string]string{SettingKeyCreativePromptGroupID: "0", SettingKeyCreativePromptModel: ""}, change.Values)
}

// TestPrepareCreativePromptSettingsRejectsInvalidRoutes validates administrator input before persistence.
// Negative groups, incomplete pairs, wildcards, and oversized names fail; a complete
// route is trimmed and an empty pair remains available for disabling assistance.
func TestPrepareCreativePromptSettingsRejectsInvalidRoutes(t *testing.T) {
	for _, input := range []AdminSettings{
		{CreativePromptGroupID: -1, CreativePromptModel: "vision"},
		{CreativePromptGroupID: 1},
		{CreativePromptModel: "vision"},
		{CreativePromptGroupID: 1, CreativePromptModel: "vision*"},
		{CreativePromptGroupID: 1, CreativePromptModel: "vision?"},
		{CreativePromptGroupID: 1, CreativePromptModel: strings.Repeat("界", 201)},
	} {
		_, values, err := PrepareAdminSettings(input)
		require.Error(t, err)
		require.Nil(t, values)
	}
	prepared, values, err := PrepareAdminSettings(AdminSettings{CreativePromptGroupID: 3, CreativePromptModel: " vision "})
	require.NoError(t, err)
	require.Equal(t, "vision", prepared.CreativePromptModel)
	require.Equal(t, "3", values[SettingKeyCreativePromptGroupID])
	require.Equal(t, "vision", values[SettingKeyCreativePromptModel])
	_, _, err = PrepareAdminSettings(AdminSettings{})
	require.NoError(t, err)
}
