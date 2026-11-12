package creative

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// settings_participant.go prepares partial creative administrator settings
// updates while retaining request-field presence.
// Creative administrator requests can update one field while keeping the
// other stored settings. Encoding a partial Fields map into AdminSettings gives
// omitted fields their Go zero values, and preparation expands that typed
// record into a complete map of persisted setting strings. Prompt route fields
// first inherit their saved counterparts so partial edits validate a complete
// group/model pair. Presence information
// therefore has to survive alongside the typed conversion.
//
// The participant validates and normalizes the typed values, then removes
// persisted keys whose request fields were absent. A supplied false flag,
// defaulted worker count, or empty model allowlist can still be written when
// its field was present. This separates a deliberate administrator update from
// an omitted field before the settings owner persists the batch.

// These persisted keys store the feature switch, worker pool target, and exact
// group/model allowlist and prompt route prepared by the creative settings participant.
const (
	// runtime feature enable switch.
	SettingKeyCreativeEnabled = "creative_enabled"
	// desired worker pool size.
	SettingKeyCreativeWorkerCount = "creative_worker_count"
	// configured group model allowlist.
	SettingKeyCreativeModelSettings = "creative_model_settings"
	// selected prompt routing group.
	SettingKeyCreativePromptGroupID = "creative_prompt_group_id"
	// exact prompt assistance model.
	SettingKeyCreativePromptModel = "creative_prompt_model"
)

// AdminSettings contains Creative access, worker, image model, and prompt routing values.
type AdminSettings struct {
	// requested feature enable switch
	CreativeEnabled bool `json:"creative_enabled"`
	// requested task worker count
	CreativeWorkerCount int `json:"creative_worker_count"`
	// requested model allowlist entries
	CreativeModelSettings []CreativeModelSetting `json:"creative_model_settings"`
	// CreativePromptGroupID and CreativePromptModel identify one prompt route.
	// A zero group together with an empty model disables prompt assistance;
	// enabled routes require a positive group and an exact model name.
	CreativePromptGroupID int64  `json:"creative_prompt_group_id"` // prompt routing group identifier
	CreativePromptModel   string `json:"creative_prompt_model"`    // prompt assistance model name
}

// PrepareAdminSettings returns normalized creative values and strings ready
// for settings persistence. Nonpositive worker counts use
// DefaultCreativeWorkerCount, while invalid model entries return
// INVALID_CREATIVE_MODEL_SETTINGS before any write occurs. Prompt routing requires
// a positive group and exact model of at most 200 Unicode characters, or zero
// and empty values to disable it.
func PrepareAdminSettings(value AdminSettings) (AdminSettings, map[string]string, error) {
	value.CreativePromptModel = strings.TrimSpace(value.CreativePromptModel)
	if value.CreativePromptGroupID < 0 ||
		(value.CreativePromptGroupID == 0) != (value.CreativePromptModel == "") ||
		utf8.RuneCountInString(value.CreativePromptModel) > 200 ||
		strings.ContainsAny(value.CreativePromptModel, "*?") {
		return value, nil, apperror.BadRequest("INVALID_CREATIVE_PROMPT_SETTINGS", "Prompt assistance requires a positive group ID and an exact model name of at most 200 characters, or a zero group ID and empty model to disable it")
	}
	values := map[string]string{}
	values[SettingKeyCreativePromptGroupID] = strconv.FormatInt(value.CreativePromptGroupID, 10)
	values[SettingKeyCreativePromptModel] = value.CreativePromptModel
	values[SettingKeyCreativeEnabled] = strconv.FormatBool(value.CreativeEnabled)
	raw, normalized, err := MarshalCreativeModelSettings(value.CreativeModelSettings)
	if err != nil {
		return value, nil, apperror.BadRequest("INVALID_CREATIVE_MODEL_SETTINGS", err.Error())
	}
	value.CreativeModelSettings = normalized
	if value.CreativeWorkerCount <= 0 {
		value.CreativeWorkerCount = DefaultCreativeWorkerCount
	}
	values[SettingKeyCreativeModelSettings] = raw
	values[SettingKeyCreativeWorkerCount] = strconv.Itoa(value.CreativeWorkerCount)

	return value, values, nil
}

// SettingsParticipant returns the creative request fields, persisted keys,
// and a preparation callback for partial settings updates. Empty input returns
// an empty change; encoding, decoding, or model validation errors stop
// preparation before the settings owner writes values.
//
// The callback converts supplied fields to AdminSettings, then retains the
// original field map to distinguish omission from typed zero values. Prompt
// fields inherit saved values before validation, so a partial edit uses the
// matching stored group or model. After
// normalization it removes persisted keys for absent request fields, so an
// omitted switch or allowlist cannot overwrite a stored value merely because
// its typed representation is false or empty.
func SettingsParticipant() settings.Participant {
	fields := []string{"creative_enabled", "creative_worker_count", "creative_model_settings", "creative_prompt_group_id", "creative_prompt_model"}
	keys := []string{SettingKeyCreativeEnabled, SettingKeyCreativeWorkerCount, SettingKeyCreativeModelSettings, SettingKeyCreativePromptGroupID, SettingKeyCreativePromptModel}
	return settings.Participant{Module: "creative", Fields: fields, Keys: keys, Prepare: func(_ context.Context, input settings.Fields, current map[string]string) (settings.PreparedChange, error) {
		if len(input) == 0 {
			return settings.PreparedChange{}, nil
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return settings.PreparedChange{}, err
		}
		// Merge the saved prompt pair before decoding partial fields so a model-only
		// edit validates against its existing routing group within the settings update session.
		prior := ReadAdminSettings(current)
		value := AdminSettings{
			CreativePromptGroupID: prior.CreativePromptGroupID,
			CreativePromptModel:   prior.CreativePromptModel,
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return settings.PreparedChange{}, err
		}
		_, values, err := PrepareAdminSettings(value)
		if err != nil {
			return settings.PreparedChange{}, err
		}
		// Preparing a typed record gives omitted fields zero values. Remove their
		// corresponding keys so a partial administrator request keeps
		// stored values
		// owned by fields that were absent from the request.
		for i, field := range fields {
			if _, ok := input[field]; !ok {
				delete(values, keys[i])
			}
		}
		return settings.PreparedChange{Values: values}, nil
	}}
}
