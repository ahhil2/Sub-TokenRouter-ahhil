package site

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPrepareAdminSettingsPreservesReferencedLogo submits a version reference while
// changing another setting. It verifies the stored data URL survives, while new image
// values and explicit clearing still update storage.
func TestPrepareAdminSettingsPreservesReferencedLogo(t *testing.T) {
	stored := "data:image/png;base64,aW1hZ2U="
	values := map[string]string{SettingKeySiteLogo: stored}
	updates, err := PrepareAdminSettings(&AdminSettings{SiteName: "updated", SiteLogo: LogoURL(stored)})
	require.NoError(t, err)
	require.NotContains(t, updates, SettingKeySiteLogo)
	maps.Copy(values, updates)
	require.Equal(t, stored, values[SettingKeySiteLogo])
	require.Equal(t, "updated", values[SettingKeySiteName])
	for _, value := range []string{"", "https://example.com/new.png", "data:image/png;base64,bmV3"} {
		updates, err = PrepareAdminSettings(&AdminSettings{SiteLogo: value})
		require.NoError(t, err)
		require.Contains(t, updates, SettingKeySiteLogo)
		require.Equal(t, value, updates[SettingKeySiteLogo])
	}
}
