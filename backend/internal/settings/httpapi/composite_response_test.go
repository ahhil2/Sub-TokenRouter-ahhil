package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/settings/composite"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

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
