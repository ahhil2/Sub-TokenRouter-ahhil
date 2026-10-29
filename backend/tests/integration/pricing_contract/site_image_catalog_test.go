package pricingcontract

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	billingtestkit "github.com/TokenFlux/TokenRouter/internal/billing/testkit"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	catalogprovider "github.com/TokenFlux/TokenRouter/internal/modelcatalog/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	routingtestkit "github.com/TokenFlux/TokenRouter/internal/routing/testkit"
)

// site_image_catalog_test.go checks the site's independent image model across
// catalog loading, provider configuration, route admission, and configured unit
// pricing. The fixture loads the deployment supplement directly so missing
// capability declarations prevent image admission during regression checks.

// TestSiteSupplementAdmitsIndependentImageModel loads the deployment supplement
// through the production catalog service. The image route keeps its complete
// model identity, and unspecified pricing and input capabilities remain absent.
func TestSiteSupplementAdmitsIndependentImageModel(t *testing.T) {
	baseline := catalogprovider.NewService(catalogprovider.Options{DataDir: t.TempDir()}, nil)
	require.NoError(t, baseline.Initialize())
	t.Cleanup(baseline.Stop)

	catalog := catalogprovider.NewService(catalogprovider.Options{
		DataDir:      t.TempDir(),
		FallbackFile: filepath.Join("..", "..", "..", "..", "deploy", "superahhil", "model_supplements.json"),
	}, nil)
	require.NoError(t, catalog.Initialize())
	t.Cleanup(catalog.Stop)

	groupID := *billingtestkit.GroupID()
	group := &routing.Group{ID: groupID, AllowedProtocols: []capability.ProtocolID{capability.ProtocolImagesGenerations}}
	policies := routingtestkit.NewPricingConfigService(routingtestkit.ConfigRows{}, nil, routing.PricingConfigOptions{
		ReadGroup: func(context.Context, int64) (*routing.Group, error) { return group, nil },
	})
	record := provider.Record{Platform: "openai", Type: "apikey", Credentials: map[string]any{
		"model_whitelist": []string{"gpt-image-2.5"},
	}}
	providers := gatewayprovider.CatalogueProviders([]provider.Record{record})
	attributes := &routing.ModelAttributeService{Catalog: routing.ModelAttributeCatalog{View: baseline.CapabilityView}}
	catalogue := &routing.RequestableCatalogue{
		Read:     func(context.Context, *int64) ([]routing.CatalogueProvider, error) { return providers, nil },
		Resolver: routing.RequestableResolver{GroupPolicies: policies, Attributes: attributes},
	}
	resolved := catalogue.ResolveRequestableModels(context.Background(), &groupID, "")
	require.NoError(t, resolved.Err)
	require.Empty(t, routing.RequestableModelIDs(resolved.Models))

	attributes.Catalog.View = catalog.CapabilityView
	resolved = catalogue.ResolveRequestableModels(context.Background(), &groupID, "")
	require.NoError(t, resolved.Err)
	require.Equal(t, []string{"gpt-image-2.5"}, routing.RequestableModelIDs(resolved.Models))
	attrs := catalog.ModelAttributes("gpt-image-2.5")
	require.NoError(t, routing.RequireImageOutput(attrs))
	require.Nil(t, attrs.InputModalities)
	require.Nil(t, catalog.GetModelPricing("gpt-image-2.5"))
	calculator := newCalculator(catalog)
	input := billing.PricingInput{Model: "gpt-image-2.5", GroupID: &groupID}
	withoutCards := billingtestkit.ResolverWithCards(t, calculator, nil)
	_, err := withoutCards.ResolveImageQuote(context.Background(), input, "1K")
	require.ErrorIs(t, err, pricing.ErrModelPricingUnavailable)

	resolver := billingtestkit.ResolverWithCards(t, calculator, []routing.ModelPricingEntry{{
		Models: []string{"gpt-image-2.5"}, BillingMode: routing.BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0.065), IndependentPricing: true,
	}})
	quote, err := resolver.ResolveImageQuote(context.Background(), input, "1K")
	require.NoError(t, err)
	// USD amounts tolerate binary floating-point representation at this comparison.
	require.InDelta(t, 0.065, quote.UnitPrice, 1e-12)
	require.True(t, quote.IndependentPricing)
}
