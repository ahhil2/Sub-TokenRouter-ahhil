package routing_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	catalogtest "github.com/TokenFlux/TokenRouter/internal/modelcatalog/testkit"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// countedCataloguePolicies 统计每次目录解析取得计费来源的次数。
type countedCataloguePolicies struct {
	*routing.PricingConfigService
	pricingReads int
}

// TestCatalogueReadsGroupOnce 大目录和限制阶段均使用本次查询的分组快照。
func TestCatalogueReadsGroupOnce(t *testing.T) {
	for _, stage := range []string{routing.BillingModelSourceRequested, routing.BillingModelSourceGroupMapped, routing.BillingModelSourceUpstream} {
		t.Run(stage, func(t *testing.T) {
			ids := []string{"allowed"}
			for i := range 12000 {
				ids = append(ids, fmt.Sprintf("model-%d", i))
			}
			reads := 0
			policies := &countedCataloguePolicies{PricingConfigService: routing.NewPricingConfigService(nil, nil, routing.PricingConfigOptions{ReadGroup: func(context.Context, int64) (*routing.Group, error) {
				reads++
				return &routing.Group{ID: 1, AllowedProtocols: []protocol.ProtocolID{protocol.ProtocolOpenAIResponses}, RoutingPolicy: routing.GroupRoutingPolicy{Enabled: true, RestrictModels: true, RestrictionModelSource: stage, AllowedModels: []string{"allowed"}}}, nil
			}})}
			resolver := routing.RequestableResolver{GroupPolicies: policies, Defaults: gatewayprovider.CatalogueDefaults(catalogtest.New(ids...))}
			records := []provider.Record{{Platform: "openai", Type: "apikey", Credentials: map[string]any{"model_whitelist": []string{"allowed"}}}}
			group := int64(1)
			result := resolver.ResolveWithProviders(context.Background(), &group, "", nil, gatewayprovider.CatalogueProviders(records))
			require.Equal(t, []string{"allowed"}, routing.RequestableModelIDs(result.Models))
			require.Equal(t, "allowed", result.Models[0].PricingModel)
			require.Equal(t, 1, reads)
			require.Equal(t, 1, policies.pricingReads)
		})
	}
}

// GetPricingConfigForGroup 记录计费来源的读取次数。
func (p *countedCataloguePolicies) GetPricingConfigForGroup(context.Context, int64) (*routing.PricingConfig, error) {
	p.pricingReads++
	return &routing.PricingConfig{BillingModelSource: routing.BillingModelSourceUpstream}, nil
}
