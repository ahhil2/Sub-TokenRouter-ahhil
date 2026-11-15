package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	settingscore "github.com/TokenFlux/TokenRouter/internal/settings"
)

// runtime_settings_external_test.go exercises registration interval reads through
// the public runtime reader. Missing settings represent upgraded installations,
// while corrupt stored values and unavailable storage must reject admission.
// The shared store fixture exposes the same missing-key distinction as production.

// TestRegistrationIPIntervalSettings covers missing, partial, and malformed saved
// components. It protects duration conversion and failure propagation so corrupt
// configuration cannot silently disable the registration guard.
func TestRegistrationIPIntervalSettings(t *testing.T) {
	for _, tc := range []struct {
		name    string            // configuration case label
		values  map[string]string // stored interval component strings
		hours   int               // expected total hours
		invalid bool              // expected read rejection
	}{
		{name: "missing"},
		{name: "disabled", values: map[string]string{"registration_ip_interval_days": "0", "registration_ip_interval_hours": "0"}},
		{name: "six hours", values: map[string]string{"registration_ip_interval_hours": "6"}, hours: 6},
		{name: "thirty days", values: map[string]string{"registration_ip_interval_days": "30"}, hours: 720},
		{name: "maximum", values: map[string]string{"registration_ip_interval_days": "3650", "registration_ip_interval_hours": "23"}, hours: 87623},
		{name: "negative days", values: map[string]string{"registration_ip_interval_days": "-1"}, invalid: true},
		{name: "excessive days", values: map[string]string{"registration_ip_interval_days": "3651"}, invalid: true},
		{name: "negative hours", values: map[string]string{"registration_ip_interval_hours": "-1"}, invalid: true},
		{name: "excessive hours", values: map[string]string{"registration_ip_interval_hours": "24"}, invalid: true},
		{name: "fraction", values: map[string]string{"registration_ip_interval_hours": "0.5"}, invalid: true},
		{name: "empty saved value", values: map[string]string{"registration_ip_interval_days": ""}, invalid: true},
		{name: "corrupt", values: map[string]string{"registration_ip_interval_hours": "bad"}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := identity.NewRuntimeSettings(&settingRepoStub{values: tc.values}, settingscore.ErrSettingNotFound)
			hours, err := reader.GetRegistrationIPInterval(context.Background())
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.hours, hours)
		})
	}
	failure := errors.New("settings unavailable")
	reader := identity.NewRuntimeSettings(&settingRepoStub{err: failure}, settingscore.ErrSettingNotFound)
	_, err := reader.GetRegistrationIPInterval(context.Background())
	require.ErrorIs(t, err, failure)
}
