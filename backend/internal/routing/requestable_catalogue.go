package routing

import "context"

// RequestableCatalogue 组合模型列表缓存、提供商查询和可请求模型解析器。
type RequestableCatalogue struct {
	Models   *ModelList
	Read     func(context.Context, *int64) ([]CatalogueProvider, error)
	Resolver RequestableResolver
	Warn     func(string, ...any)
}

func (c *RequestableCatalogue) Prefetch(ctx context.Context) ([]CatalogueProvider, bool, error) {
	if c == nil || c.Read == nil {
		return nil, false, nil
	}
	values, err := c.Read(ctx, nil)
	return values, err == nil, err
}

// ResolveRequestableModels 合并目录与配置，并用当前提供商复核候选。
func (c *RequestableCatalogue) ResolveRequestableModels(ctx context.Context, groupID *int64, platform string) RequestableModelsResult {
	if c == nil || c.Read == nil {
		return RequestableModelsResult{}
	}
	models := c.Models
	if models == nil {
		models = &ModelList{Read: c.Read}
	}
	base := models.Available(ctx, groupID, platform)
	providers, err := c.Read(ctx, groupID)
	if err != nil {
		if c.Warn != nil {
			var id int64
			if groupID != nil {
				id = *groupID
			}
			c.Warn("failed to load providers for requestable model resolution", "group_id", id, "platform", platform, "error", err)
		}
		return RequestableModelsResult{Restricted: true, HadExplicitProviderModels: len(base) > 0}
	}
	return c.Resolver.ResolveWithProviders(ctx, groupID, platform, base, providers)
}
