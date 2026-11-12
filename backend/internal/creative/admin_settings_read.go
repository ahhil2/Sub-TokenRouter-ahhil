package creative

import (
	"strconv"
	"strings"
)

// admin_settings_read.go decodes stored creative settings for administrator
// responses.
// Administrator reads interpret the feature flag independently from model
// admission. Missing worker counts use the pool default, while missing or
// malformed model settings yield an empty allowlist that closes the creative
// directory.

// AdminReadSettings contains the creative feature switch, model settings, and
// worker count, together with the saved prompt assistance route.
type AdminReadSettings struct {
	CreativeEnabled       bool                   // feature switch read result
	CreativeModelSettings []CreativeModelSetting // normalized model allowlist entries
	CreativeWorkerCount   int                    // configured task worker count
	CreativePromptGroupID int64                  // prompt routing group identifier
	CreativePromptModel   string                 // prompt assistance model name
}

// ReadAdminSettings parses creative values for the administration response.
// Only the exact string false disables the feature; model parsing fails closed
// and invalid worker counts use DefaultCreativeWorkerCount. Missing or malformed
// prompt group identifiers become zero, and prompt model names are trimmed.
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}
	result.CreativeModelSettings = ParseCreativeModelSettings(settings[SettingKeyCreativeModelSettings])
	result.CreativeWorkerCount = ParseCreativeWorkerCount(settings[SettingKeyCreativeWorkerCount])
	result.CreativeEnabled = settings[SettingKeyCreativeEnabled] != "false"
	// Malformed group identifiers become zero for administrative repair; runtime
	// admission separately rejects incomplete or invalid pairs before routing any request.
	if groupID, err := strconv.ParseInt(strings.TrimSpace(settings[SettingKeyCreativePromptGroupID]), 10, 64); err == nil {
		result.CreativePromptGroupID = groupID
	}
	result.CreativePromptModel = strings.TrimSpace(settings[SettingKeyCreativePromptModel])

	return result
}
