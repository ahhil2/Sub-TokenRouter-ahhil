package identity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// admin_settings_test.go checks registration interval validation and serialized
// settings readback. Accepted components must survive persistence as whole day
// and hour values, and rejected input must produce no prepared settings batch.

// TestPrepareAdminSettingsRegistrationInterval exercises disabled, combined, and
// invalid durations. Accepted writes round-trip through the administrative reader,
// while invalid components reject the complete prepared batch.
func TestPrepareAdminSettingsRegistrationInterval(t *testing.T) {
	for _, tc := range []struct {
		days  int  // submitted whole days
		hours int  // submitted additional hours
		valid bool // expected preparation acceptance
	}{{0, 0, true}, {0, 6, true}, {30, 0, true}, {3650, 23, true}, {-1, 0, false}, {3651, 0, false}, {0, -1, false}, {0, 24, false}} {
		input := &AdminSettings{RegistrationIPIntervalDays: tc.days, RegistrationIPIntervalHours: tc.hours}
		values, err := PrepareAdminSettings(input)
		if !tc.valid {
			require.ErrorContains(t, err, "registration interval")
			require.Nil(t, values)
			continue
		}
		require.NoError(t, err)
		reader := NewOAuthSettings(nil, nil, nil)
		saved := reader.ReadAdminSettings(values, func() int { return 1 })
		require.Equal(t, tc.days, saved.RegistrationIPIntervalDays)
		require.Equal(t, tc.hours, saved.RegistrationIPIntervalHours)
	}
}
