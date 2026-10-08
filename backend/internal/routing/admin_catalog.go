package routing

import (
	"sort"
)

const (
	CatalogClaude      AdminCatalogKind = "claude"
	CatalogOpenAI      AdminCatalogKind = "openai"
	CatalogGemini      AdminCatalogKind = "gemini"
	CatalogGoogleOne   AdminCatalogKind = "google_one"
	CatalogAntigravity AdminCatalogKind = "antigravity"
	CatalogQoder       AdminCatalogKind = "qoder"
	CatalogGrok        AdminCatalogKind = "grok"
)

// AdminCatalogKind 标识管理目录的数据来源。
type AdminCatalogKind string

// AdminCatalogModel 保留各目录的值，HTTP 按原 wire 变体输出。
type AdminCatalogModel struct {
	ID, Object, Type, OwnedBy, DisplayName, CreatedAt string
	Created                                           int64
}
type AdminCatalogInput struct {
	Platform, Site                string
	OAuth, GoogleOne, Passthrough bool
	Accept                        func(string) bool
}
type AdminCatalogResult struct {
	Kind   AdminCatalogKind
	Models []AdminCatalogModel
}
type AdminCatalogOptions struct {
	Defaults func(AdminCatalogKind, string) ([]AdminCatalogModel, error)
}

// AdminCatalog 不安装缓存；动态目录每次按原顺序读取一次。
type AdminCatalog struct{ options AdminCatalogOptions }

func NewAdminCatalog(options AdminCatalogOptions) *AdminCatalog { return &AdminCatalog{options} }
func (s *AdminCatalog) Available(input AdminCatalogInput, configured func() []string) (AdminCatalogResult, error) {
	kind := CatalogClaude
	switch input.Platform {
	case "openai":
		kind = CatalogOpenAI
	case "gemini":
		kind = CatalogGemini
		if input.GoogleOne && input.OAuth {
			kind = CatalogGoogleOne
		}
	case "antigravity":
		kind = CatalogAntigravity
	case "qoder":
		kind = CatalogQoder
	case "grok":
		kind = CatalogGrok
	}
	requested := configured()
	defaults, err := s.options.Defaults(kind, input.Site)
	if err != nil {
		return AdminCatalogResult{}, err
	}
	for _, model := range defaults {
		requested = append(requested, model.ID)
	}
	sort.Strings(requested)
	byID := make(map[string]AdminCatalogModel, len(defaults))
	for _, m := range defaults {
		if _, ok := byID[m.ID]; ok && kind != CatalogGrok {
			continue
		}
		byID[m.ID] = m
	}
	var models []AdminCatalogModel
	seen := make(map[string]bool)
	for _, id := range requested {
		if seen[id] || (input.Accept != nil && !input.Accept(id)) {
			continue
		}
		seen[id] = true
		if m, ok := byID[id]; ok {
			models = append(models, m)
			continue
		}
		m := AdminCatalogModel{ID: id, Type: "model", DisplayName: id}
		if kind == CatalogOpenAI {
			m.Object = "model"
		}
		if kind == CatalogGrok {
			m.Object = "model"
			m.Type = ""
			m.OwnedBy = "xai"
		}
		models = append(models, m)
	}
	return AdminCatalogResult{Kind: kind, Models: models}, nil
}
