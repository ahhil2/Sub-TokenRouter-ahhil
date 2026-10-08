package clientmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCodexRoutingClient(t *testing.T) {
	for _, userAgent := range []string{"Codex Desktop/1.0", "codex-tui/1.0", "codex_vscode/1.0", "codex_cli_rs/1.0", "codex_sdk_ts/1.0", "codex_exec/1.0", " CODEX_CLI_RS/1.0 "} {
		assert.True(t, IsCodexRoutingClient(userAgent, ""), userAgent)
	}
	for _, originator := range []string{"Codex Desktop", "codex_work_desktop", "codex-tui", "codex_vscode", "codex_cli_rs", " CODEX_CLI_RS "} {
		assert.True(t, IsCodexRoutingClient("Mozilla/5.0", originator), originator)
	}
	for _, headers := range [][2]string{
		{"", ""}, {"curl/8.0", ""}, {"python-openai/1.0", ""},
		{"Claude-Code/1.0", "claude_code"}, {"Mozilla/5.0 codex_cli_rs/1.0", ""},
		{"evil-codex_cli_rs/1.0", ""}, {"codex_cli_rs", ""},
		{"curl/8.0", "codex_cli_rs_evil"}, {"curl/8.0", "codex-tui,third-party"},
	} {
		assert.False(t, IsCodexRoutingClient(headers[0], headers[1]), headers)
	}
}
