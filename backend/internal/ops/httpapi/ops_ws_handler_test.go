package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// ops_ws_handler_test.go checks trusted proxy parsing and administrator
// WebSocket handshakes against a local server. Protocol fixtures protect current
// branding preference, legacy compatibility, and the exclusion of authentication
// tokens from the negotiated protocol.

// TestOpsWSHelpers mixes one valid network prefix with invalid text, strips a
// normal host port, and checks matching addresses. It protects those supplied
// helper cases without asserting complete proxy or Origin policy behavior.
func TestOpsWSHelpers(t *testing.T) {
	prefixes, invalid := parseTrustedProxyList("10.0.0.0/8,invalid")
	require.Len(t, prefixes, 1)
	require.Len(t, invalid, 1)

	host := hostWithoutPort("example.com:443")
	require.Equal(t, "example.com", host)

	addr := netip.MustParseAddr("10.0.0.1")
	require.True(t, isAddrInTrustedProxies(addr, prefixes))
	require.False(t, isAddrInTrustedProxies(netip.MustParseAddr("192.168.0.1"), prefixes))
}

// TestOpsWSBrandNegotiation offers legacy, current, and mixed administrator
// subprotocol names to a local upgrade server. It checks legacy compatibility
// and current-name preference in either client order. Unrecognized protocols and
// jwt.fixture are not echoed; this fixture performs no JWT validation.
func TestOpsWSBrandNegotiation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeOpsWS(w, r, nil)
		if err != nil {
			return
		}
		// Release the server side immediately after handshake; these cases inspect
		// protocol selection and require no traffic-push loop.
		_ = conn.Close()
	}))
	// Stop accepting local handshakes after the cases. The server handler closes
	// each upgraded socket separately, and client cases close their returned
	// connections.
	defer server.Close()
	for _, tc := range []struct {
		name      string   // handshake case label
		protocols []string // offered client protocols
		want      string   // expected negotiated protocol
	}{
		{"legacy", []string{"sub2api-admin", "jwt.fixture"}, "sub2api-admin"},
		{"current", []string{"tokenrouter-admin", "jwt.fixture"}, "tokenrouter-admin"},
		{"legacy first", []string{"sub2api-admin", "tokenrouter-admin", "jwt.fixture"}, "tokenrouter-admin"},
		{"current first", []string{"tokenrouter-admin", "sub2api-admin", "jwt.fixture"}, "tokenrouter-admin"},
		{"token first", []string{"jwt.fixture", "tokenrouter-admin"}, "tokenrouter-admin"},
		{"token only", []string{"jwt.fixture"}, ""},
		{"unsupported", []string{"other-admin", "jwt.fixture"}, ""},
		{"no protocols", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialer := websocket.Dialer{Subprotocols: tc.protocols}
			// Open each case through a real local handshake to observe the selected
			// response subprotocol, then close the resulting connection.
			conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, conn.Subprotocol())
			require.NoError(t, conn.Close())
		})
	}
}
