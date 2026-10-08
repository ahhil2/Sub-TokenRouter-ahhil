package provider

import "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"

const (
	OpenAIClientClassExtraKey = "openai_client_class"
	ClientClassAny            = "any"
	ClientClassCodex          = "codex"
	ClientClassNonCodex       = "non-codex"
)

// ValidateClientClassExtra 校验提供商请求来源设置，空值表示不限。
func ValidateClientClassExtra(extra map[string]any) error {
	value, exists := extra[OpenAIClientClassExtraKey]
	if !exists || value == nil {
		return nil
	}
	class, valid := value.(string)
	if valid && (class == "" || class == ClientClassAny || class == ClientClassCodex || class == ClientClassNonCodex) {
		return nil
	}
	return apperror.BadRequest("INVALID_OPENAI_CLIENT_CLASS", "OpenAI client class must be any, codex, or non-codex")
}

// AllowsClientClass 按入站客户端分类筛选 OpenAI 提供商，其他平台保持可用。
func (record *Record) AllowsClientClass(codex bool) bool {
	if record == nil {
		return false
	}
	if record.Platform != PlatformOpenAI {
		return true
	}
	value := record.Extra[OpenAIClientClassExtraKey]
	if value == nil {
		return true
	}
	class, valid := value.(string)
	if !valid {
		return false
	}
	switch class {
	case "", ClientClassAny:
		return true
	case ClientClassCodex:
		return codex
	case ClientClassNonCodex:
		return !codex
	default:
		return false
	}
}
