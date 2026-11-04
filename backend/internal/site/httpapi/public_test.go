package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

// calendarPublicSource supplies empty settings so the test exercises the injected calendar.
type calendarPublicSource struct{}

// logoPublicSource supplies a mutable inline logo for public asset endpoint tests.
type logoPublicSource struct {
	logo string // Current inline image fixture.
}

// TestPublicCalendarMatchesAPIAndInjection supplies calendars with distinct zone names and display labels.
// It verifies that API and HTML injection return the configured label and the
// current UTC offset derived from the same injected calendar.
func TestPublicCalendarMatchesAPIAndInjection(t *testing.T) {
	for _, item := range []struct {
		location, name string // IANA calendar location and displayed timezone label
	}{
		{"Asia/Shanghai", "Asia/Shanghai"},
		{"America/New_York", "America/New_York"},
		{"America/New_York", "Local"},
	} {
		t.Run(item.name, func(t *testing.T) {
			location, err := time.LoadLocation(item.location)
			require.NoError(t, err)
			calendar := timezone.NewCalendar(location)
			service := site.NewPublicService(calendarPublicSource{}, calendar, item.name)
			handler := NewPublicHandler(service, "calendar-version")
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/settings/public", nil)
			handler.GetPublicSettings(ctx)
			require.Equal(t, 200, response.Code)
			var envelope struct {
				Data map[string]any `json:"data"` // public settings response
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			injection, err := service.GetPublicSettingsForInjection(context.Background())
			require.NoError(t, err)
			encoded, err := json.Marshal(injection)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(encoded, &fields))
			for _, values := range []map[string]any{envelope.Data, fields} {
				require.Equal(t, item.name, values["server_timezone"])
				require.Equal(t, calendar.UTCOffset(time.Now()), values["server_utc_offset"])
			}
		})
	}
}

// LoadSitePublicInputs supplies empty display, auth, and usage inputs for calendar tests.
// Each call allocates a fresh values map, keeping fixture data independent of
// the response service's language and navigation transformations.
func (calendarPublicSource) LoadSitePublicInputs(context.Context) (site.PublicInputs, error) {
	return site.PublicInputs{Values: map[string]string{}}, nil
}

// PublicVersion returns the fixed source build label used by injection fixtures.
func (calendarPublicSource) PublicVersion() string { return "calendar-contract" }

// TestPublicLogoUsesVersionedAsset exposes an inline SVG through settings, injection, and
// the image route. It checks cache validation, content policy, and rejection of stale
// versions after the stored image changes.
func TestPublicLogoUsesVersionedAsset(t *testing.T) {
	source := &logoPublicSource{logo: "data:image/svg+xml,%3Csvg%20xmlns=%22http://www.w3.org/2000/svg%22%3E%3C/svg%3E"}
	service := site.NewPublicService(source, timezone.NewCalendar(time.UTC), "UTC")
	router := gin.New()
	RegisterPublicSettingsRoutes(router.Group("/api/v1/settings"), NewPublicHandler(service, "test"))
	reference := site.LogoURL(source.logo)
	call := func(path, etag string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", etag)
		router.ServeHTTP(rec, req)
		return rec
	}
	settings := call("/api/v1/settings/public", "")
	require.Equal(t, http.StatusOK, settings.Code)
	require.Contains(t, settings.Body.String(), reference)
	require.NotContains(t, settings.Body.String(), "data:image")
	injection, err := service.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	encoded, err := json.Marshal(injection)
	require.NoError(t, err)
	require.Contains(t, string(encoded), reference)
	require.NotContains(t, string(encoded), "data:image")

	asset := call(reference, "")
	require.Equal(t, http.StatusOK, asset.Code)
	require.Equal(t, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`, asset.Body.String())
	require.Equal(t, "image/svg+xml", asset.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", asset.Header().Get("Cache-Control"))
	require.Contains(t, asset.Header().Get("Content-Security-Policy"), "sandbox")
	require.Equal(t, "nosniff", asset.Header().Get("X-Content-Type-Options"))
	cached := call(reference, asset.Header().Get("ETag"))
	require.Equal(t, http.StatusNotModified, cached.Code)
	require.Empty(t, cached.Body.String())
	require.Equal(t, http.StatusNotFound, call(reference+"x", "").Code)
	source.logo = "data:image/png;base64,bmV3"
	require.Equal(t, http.StatusNotFound, call(reference, "").Code)
	require.Equal(t, http.StatusOK, call(site.LogoURL(source.logo), "").Code)
}

// LoadSitePublicInputs returns the fixture logo through the site settings source.
func (s *logoPublicSource) LoadSitePublicInputs(context.Context) (site.PublicInputs, error) {
	return site.PublicInputs{Values: map[string]string{site.SettingKeySiteLogo: s.logo}}, nil
}

// PublicVersion provides the stable build label for logo endpoint fixtures.
func (*logoPublicSource) PublicVersion() string { return "logo-test" }
