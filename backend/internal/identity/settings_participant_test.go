package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// settings_participant_test.go verifies field presence and validation when
// registration policy joins a combined settings update. Unsubmitted interval
// components must remain outside the persistence batch, while explicit zero
// values must reach storage so administrators can disable the guard.

// TestSettingsParticipantRegistrationInterval covers partial requests from old
// and current pages. It protects presence filtering, explicit disabling, and
// validation before invalid values can enter the persistence coordinator.
func TestSettingsParticipantRegistrationInterval(t *testing.T) {
	participant := SettingsParticipant(NewGrantSettings(nil, GrantSettingsOptions{}))
	for _, tc := range []struct {
		input    string            // submitted JSON field object
		expected map[string]string // expected prepared persistence values
		invalid  bool              // expected preparation rejection
	}{
		{input: `{"registration_enabled":true}`, expected: map[string]string{SettingKeyRegistrationEnabled: "true"}},
		{input: `{"registration_ip_interval_hours":6}`, expected: map[string]string{SettingKeyRegistrationIPIntervalHours: "6"}},
		{input: `{"registration_ip_interval_days":30,"registration_ip_interval_hours":0}`, expected: map[string]string{SettingKeyRegistrationIPIntervalDays: "30", SettingKeyRegistrationIPIntervalHours: "0"}},
		{input: `{"registration_ip_interval_days":0,"registration_ip_interval_hours":0}`, expected: map[string]string{SettingKeyRegistrationIPIntervalDays: "0", SettingKeyRegistrationIPIntervalHours: "0"}},
		{input: `{"registration_ip_interval_days":-1}`, invalid: true},
		{input: `{"registration_ip_interval_hours":24}`, invalid: true},
		{input: `{"registration_ip_interval_days":0.5}`, invalid: true},
		{input: `{"registration_ip_interval_hours":"6"}`, invalid: true},
	} {
		var fields settings.Fields
		require.NoError(t, json.Unmarshal([]byte(tc.input), &fields))
		result, err := participant.Prepare(context.Background(), fields, nil)
		if tc.invalid {
			require.Error(t, err)
			require.Empty(t, result.Values)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tc.expected, result.Values)
	}
}
