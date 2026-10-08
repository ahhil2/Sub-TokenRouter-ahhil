package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	catalogprovider "github.com/TokenFlux/TokenRouter/internal/modelcatalog/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// catalogueReader 查询指定分组或全部可调度提供商，由网关适配器转换模型数据。
func catalogueReader(store *providerpostgres.ProviderStore) func(context.Context, *int64) ([]routing.CatalogueProvider, error) {
	return func(ctx context.Context, id *int64) ([]routing.CatalogueProvider, error) {
		var values []provider.Record
		var err error
		if id != nil {
			values, err = store.ListSchedulableByGroupID(ctx, *id)
		} else {
			values, err = store.ListSchedulable(ctx)
		}
		if err != nil {
			return nil, err
		}
		return gatewayprovider.CatalogueProviders(values), nil
	}
}

// provideRequestableCatalogue 与市场、模型列表共用缓存和提供商存储。
func provideRequestableCatalogue(catalog *catalogprovider.Service, models *routing.ModelList, store *providerpostgres.ProviderStore, modelConfigs *routing.PricingConfigService) *routing.RequestableCatalogue {
	return &routing.RequestableCatalogue{Models: models, Read: catalogueReader(store), Resolver: routing.RequestableResolver{GroupPolicies: modelConfigs, Defaults: gatewayprovider.CatalogueDefaults(catalog), Warn: slog.Warn}, Warn: slog.Warn}
}

// provideRoutingModelList 为模型列表缓存配置 TTL，默认十五秒。
func provideRoutingModelList(catalog *catalogprovider.Service, repo *providerpostgres.ProviderStore, cfg *config.Config) *routing.ModelList {
	result := routing.NewModelList(catalogueReader(repo), resolveModelsListCacheTTL(cfg))
	result.Version = catalog.ModelVersion
	return result
}

// resolveModelsListCacheTTL 返回配置的正数 TTL，其他情况使用十五秒。
func resolveModelsListCacheTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Gateway.ModelsListCacheTTLSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(cfg.Gateway.ModelsListCacheTTLSeconds) * time.Second
}
