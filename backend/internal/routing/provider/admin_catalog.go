package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/modelcatalog"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// AdminCatalogOptions 在查询时读取统一目录，响应格式由 HTTP 适配器决定。
func AdminCatalogOptions(catalog modelcatalog.Reader) routing.AdminCatalogOptions {
	return routing.AdminCatalogOptions{Defaults: func(kind routing.AdminCatalogKind, _ string) ([]routing.AdminCatalogModel, error) {
		out := []routing.AdminCatalogModel{}
		if catalog == nil {
			return out, nil
		}
		for _, id := range catalog.ModelIDs() {
			entry := catalog.ModelEntry(id)
			name := id
			if entry.Attributes.DisplayName != nil && *entry.Attributes.DisplayName != "" {
				name = *entry.Attributes.DisplayName
			}
			model := routing.AdminCatalogModel{ID: id, Type: "model", DisplayName: name}
			if kind == routing.CatalogOpenAI || kind == routing.CatalogGrok {
				model.Object = "model"
				model.OwnedBy = entry.Provider
			}
			out = append(out, model)
		}
		return out, nil
	}}
}
