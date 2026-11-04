package site

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogoURLAndDecode supplies supported, malformed, external, and oversized logo
// representations. It verifies stable version references, independent decoded bytes,
// accepted media types, and pass-through behavior for unsupported values.
func TestLogoURLAndDecode(t *testing.T) {
	raw := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("image bytes"))
	reference := LogoURL(raw)
	require.True(t, IsLogoReference(reference))
	require.Equal(t, reference, LogoURL(raw))
	require.NotEqual(t, reference, LogoURL("data:image/png;base64,b3RoZXI="))
	bytes, media, ok := DecodeLogo(raw)
	require.True(t, ok)
	require.Equal(t, "image/png", media)
	require.Equal(t, "image bytes", string(bytes))
	bytes, media, ok = DecodeLogo("data:image/svg+xml,%3Csvg%3E%3C%2Fsvg%3E")
	require.True(t, ok)
	require.Equal(t, "image/svg+xml", media)
	require.Equal(t, "<svg></svg>", string(bytes))
	for _, value := range []string{"", "https://example.com/logo.png", "data:text/html;base64,eA==", "data:image/png;base64,???", "data:image/png;base64,", "data:image/png,%ZZ", "data:image/png," + strings.Repeat("x", (8<<20)+1)} {
		_, _, valid := DecodeLogo(value)
		require.False(t, valid)
		require.Equal(t, value, LogoURL(value))
	}
	require.False(t, IsLogoReference(logoURLPrefix+"invalid"))
}
