package site

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
)

// logo.go converts supported inline logos into stable public asset references.
// The database retains the data URL, while public settings and editable responses
// carry a hash-addressed URL. Decoding admits an explicit image media list and
// bounded payloads; unsupported values remain available through their original URL.

// logoURLPrefix identifies references that preserve the stored logo during settings updates.
const logoURLPrefix = "/api/v1/settings/logo/"

// LogoURL returns a SHA-256-addressed public URL for a supported inline image. External
// URLs and unsupported data URLs pass through unchanged; identical encoded strings share
// a reference.
func LogoURL(value string) string {
	if _, _, ok := DecodeLogo(value); !ok {
		return value
	}
	hash := sha256.Sum256([]byte(value))
	return logoURLPrefix + hex.EncodeToString(hash[:])
}

// IsLogoReference recognizes a syntactically valid content-versioned logo reference.
// Settings writers use it to preserve the stored image; the public endpoint separately
// verifies that the reference matches the current image.
func IsLogoReference(value string) bool {
	if !strings.HasPrefix(value, logoURLPrefix) {
		return false
	}
	hash, err := hex.DecodeString(strings.TrimPrefix(value, logoURLPrefix))
	return err == nil && len(hash) == sha256.Size
}

// DecodeLogo decodes supported image data URLs into independent bytes and their media
// type. Invalid encodings, unsupported types, empty payloads, or images exceeding eight
// MiB return false. The encoded input is capped before allocation; validation checks
// representation and size rather than parsing image contents.
func DecodeLogo(value string) ([]byte, string, bool) {
	// Bound encoded input before decoding to limit allocation from malformed settings;
	// the separate decoded limit also covers percent-encoded and unencoded image data.
	if !strings.HasPrefix(value, "data:") || len(value) > 12<<20 {
		return nil, "", false
	}
	header, encoded, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok {
		return nil, "", false
	}
	media, _, _ := strings.Cut(header, ";")
	switch media {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/svg+xml", "image/x-icon", "image/vnd.microsoft.icon":
	default:
		return nil, "", false
	}
	var data []byte
	var err error
	if strings.HasSuffix(header, ";base64") {
		data, err = base64.StdEncoding.DecodeString(encoded)
	} else {
		var decoded string
		decoded, err = url.PathUnescape(encoded)
		data = []byte(decoded)
	}
	if err != nil || len(data) == 0 || len(data) > 8<<20 {
		return nil, "", false
	}
	return data, media, true
}
