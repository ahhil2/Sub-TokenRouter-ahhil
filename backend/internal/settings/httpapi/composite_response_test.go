package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/settings/composite"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

// composite_response_test.go checks administrator response fields and audit visibility.
// Response mapping exposes saved controls for editing, while versioned logo URLs
// keep inline image payloads out of the administrator JSON returned to browsers.

// TestSystemSettingsResponseUsesLogoReference maps a settings snapshot containing inline
// image bytes. It verifies the administrative response contains the versioned URL and
// omits the encoded image payload.
func TestSystemSettingsResponseUsesLogoReference(t *testing.T) {
	settings := &composite.Snapshot{}
	settings.SiteLogo = "data:image/png;base64,aW1hZ2U="
	response := buildSystemSettingsResponse(settings, &payment.PaymentConfig{}, nil, false)
	require.Equal(t, site.LogoURL(settings.SiteLogo), response.SiteLogo)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "data:image")
}

// TestSystemSettingsPromptRouteResponseAndAudit exposes the saved prompt route to administrators.
// It checks JSON field names and confirms both route changes appear in the
// settings audit labels used after a successful update.
func TestSystemSettingsPromptRouteResponseAndAudit(t *testing.T) {
	before := &composite.Snapshot{}
	after := &composite.Snapshot{CreativePromptGroupID: 17, CreativePromptModel: "vision-model"}
	response := buildSystemSettingsResponse(after, &payment.PaymentConfig{}, nil, false)
	require.Equal(t, int64(17), response.CreativePromptGroupID)
	require.Equal(t, "vision-model", response.CreativePromptModel)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"creative_prompt_group_id":17`)
	require.Contains(t, string(encoded), `"creative_prompt_model":"vision-model"`)
	changed := DiffSettings(before, after, nil, nil, UpdateSettingsRequest{})
	require.Contains(t, changed, "creative_prompt_group_id")
	require.Contains(t, changed, "creative_prompt_model")
}
