package clientmeta

import "strings"

// IsCodexRoutingClient 按入站 UA 前缀或 originator 精确值划分请求来源。
// 请求头由客户端提供，分类结果用于调度，身份认证由 API Key 负责。
func IsCodexRoutingClient(userAgent, originator string) bool {
	userAgent = strings.ToLower(strings.TrimSpace(userAgent))
	for _, prefix := range []string{"codex desktop/", "codex-tui/", "codex_vscode/", "codex_cli_rs/", "codex_sdk_ts/", "codex_exec/"} {
		if strings.HasPrefix(userAgent, prefix) {
			return true
		}
	}
	switch strings.ToLower(strings.TrimSpace(originator)) {
	case "codex desktop", "codex_work_desktop", "codex-tui", "codex_vscode", "codex_cli_rs":
		return true
	default:
		return false
	}
}
