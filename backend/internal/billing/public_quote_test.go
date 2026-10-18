package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// TestPublicQuotePreservesExactImagePrices 验证仅有按张报价的型号仍显示价格，零价不等于缺价。
func TestPublicQuotePreservesExactImagePrices(t *testing.T) {
	calculator := NewCalculator(defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"grok-imagine-image-quality": {CatalogRules: pricing.CatalogRules{ImagePrices: map[string]float64{"1K": 0.05}}, TokenPricingAbsent: true},
		"custom-image":               {OutputCostPerImage: 0.1, ImagePricePresent: true, TokenPricingAbsent: true, Mode: "image_generation"},
		"free-image":                 {ImagePricePresent: true, TokenPricingAbsent: true, Mode: "image_generation"},
	}}, CalculatorOptions{})
	resolver := NewPriceResolver(nil, calculator, nil, nil)
	for _, tc := range []struct {
		model string
		price float64
	}{
		{"grok-imagine-image-quality", 0.05},
		{"custom-image", 0.1},
		{"free-image", 0},
	} {
		input := PricingInput{Model: tc.model}
		quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 2})
		require.Equal(t, "priced", quote.PriceStatus, tc.model)
		require.Equal(t, "image", quote.PricingMode, tc.model)
		require.InDelta(t, tc.price*2, quote.ImagePrice1K, 1e-12)
		unit, err := resolver.ResolveImageUnitPrice(context.Background(), input, "1K")
		require.NoError(t, err)
		require.Equal(t, tc.price, unit)
		cost, err := calculator.CalculateImageCost(tc.model, "1K", 1, 2)
		require.NoError(t, err)
		require.Equal(t, quote.ImagePrice1K, cost.ActualCost)
	}
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: PricingInput{Model: "unknown-image"}, RateMultiplier: 1})
	require.Equal(t, "unpriced", quote.PriceStatus)
}

// TestPricingManagementFreeImageOverridesCatalog 检查用户零价生效时，目录成本仍有独立的尺寸价格。
func TestPricingManagementFreeImageOverridesCatalog(t *testing.T) {
	zero := 0.0
	groupID := int64(1)
	catalog := defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"gemini-3-pro-image": {CatalogRules: pricing.CatalogRules{ImagePrices: pricing.MediaPrices{"1K": 0.134, "2K": 0.134, "4K": 0.24}}, TokenPricingAbsent: true},
	}}
	calculator := NewCalculator(catalog, CalculatorOptions{})
	resolver := NewPriceResolver(mediaPriceCards{card: &ModelPricingEntry{BillingMode: pricing.BillingModeImage, PerRequestPrice: &zero}}, calculator, nil, nil)
	input := PricingInput{Model: "gemini-3-pro-image", GroupID: &groupID}
	shown := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 2})
	require.Equal(t, "priced", shown.PriceStatus)
	require.ElementsMatch(t, []string{"1K", "2K", "4K"}, shown.ImagePriceSizes)
	for _, size := range shown.ImagePriceSizes {
		unit, err := resolver.ResolveImageUnitPrice(context.Background(), input, size)
		require.NoError(t, err)
		require.Zero(t, unit)
		official, err := calculator.DefaultImagePrice(input.Model, size)
		require.NoError(t, err)
		require.Positive(t, official, "用户售价不能覆盖目录及提供商成本的基础数据")
	}
}

func TestPublicQuoteVideoPrices(t *testing.T) {
	groupID := int64(1)
	for _, testCase := range []struct {
		name     string
		card     *ModelPricingEntry
		rate     float64
		expected pricing.MediaPrices
	}{
		{
			name: "configured resolutions",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, PriceMultiplier: requestTestPrice(1), Intervals: []pricing.PricingInterval{
				{TierLabel: "480p", PerRequestPrice: requestTestPrice(0.06)},
				{TierLabel: "720p", PerRequestPrice: requestTestPrice(0.09)},
				{TierLabel: "1080p", PerRequestPrice: requestTestPrice(0.18)},
			}},
			rate: 0.18, expected: pricing.MediaPrices{"480p": 0.0108, "720p": 0.0162, "1080p": 0.0324},
		},
		{
			name: "card and group multipliers",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, PriceMultiplier: requestTestPrice(2), PerRequestPrice: requestTestPrice(0.06)},
			rate: 0.18, expected: pricing.MediaPrices{"480p": 0.0216, "720p": 0.0216, "1080p": 0.0216},
		},
		{
			name: "partial card preserves free tier and omits missing tiers",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, Intervals: []pricing.PricingInterval{
				{TierLabel: "480p", PerRequestPrice: requestTestPrice(0)},
				{TierLabel: "720p", PerRequestPrice: requestTestPrice(0.09)},
			}},
			rate: 0.18, expected: pricing.MediaPrices{"480p": 0, "720p": 0.0162},
		},
		{
			name: "explicit free default",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, PerRequestPrice: requestTestPrice(0)},
			rate: 1, expected: pricing.MediaPrices{"480p": 0, "720p": 0, "1080p": 0},
		},
		{
			name: "zero group multiplier",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, PerRequestPrice: requestTestPrice(0.06)},
			rate: 0, expected: pricing.MediaPrices{"480p": 0, "720p": 0, "1080p": 0},
		},
		{
			name: "catalog video prices",
			rate: 0.18, expected: pricing.MediaPrices{"480p": 0.0144, "1080p": 0},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			catalog := defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
				"grok-imagine-video-1.5": {CatalogRules: pricing.CatalogRules{VideoPrices: pricing.MediaPrices{"480p": 0.08, "1080p": 0}}, TokenPricingAbsent: true},
			}}
			calculator := NewCalculator(catalog, CalculatorOptions{})
			resolver := NewPriceResolver(mediaPriceCards{card: testCase.card}, calculator, nil, nil)
			input := PricingInput{Model: "grok-imagine-video-1.5", GroupID: &groupID}
			quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: testCase.rate})
			require.Equal(t, "video", quote.PricingMode)
			require.Equal(t, "priced", quote.PriceStatus)
			require.Len(t, quote.VideoPrices, len(testCase.expected))
			for resolution, expected := range testCase.expected {
				require.Contains(t, quote.VideoPrices, resolution)
				require.InDelta(t, expected, quote.VideoPrices[resolution], 1e-12)
				var cost *CostBreakdown
				var err error
				if testCase.card != nil {
					cost, err = calculator.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: input.Model, GroupID: input.GroupID, UsageUnits: 5, SizeTier: resolution, RateMultiplier: testCase.rate, Resolver: resolver})
				} else {
					cost, err = calculator.CalculateVideoCost(input.Model, resolution, 1, 5, testCase.rate)
				}
				require.NoError(t, err)
				require.InDelta(t, quote.VideoPrices[resolution]*5, cost.ActualCost, 1e-12)
			}
		})
	}
}

func TestPublicQuoteMissingVideoPrice(t *testing.T) {
	calculator := NewCalculator(defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"grok-imagine-video": {CatalogRules: pricing.CatalogRules{VideoPrices: pricing.MediaPrices{"480p": 0.08}}, TokenPricingAbsent: true},
	}}, CalculatorOptions{})
	resolver := NewPriceResolver(nil, calculator, nil, nil)
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: PricingInput{Model: "grok-imagine-video-1.5"}, RateMultiplier: 1})
	require.Equal(t, "unpriced", quote.PriceStatus)
	require.Empty(t, quote.VideoPrices)
}

func TestPublicQuoteImageCardMultiplierKeepsGroupMultiplier(t *testing.T) {
	groupID := int64(1)
	calculator := NewCalculator(nil, CalculatorOptions{})
	resolver := NewPriceResolver(mediaPriceCards{card: &ModelPricingEntry{
		BillingMode: pricing.BillingModePerRequest, PerRequestPrice: requestTestPrice(0.065), PriceMultiplier: requestTestPrice(1),
	}}, calculator, nil, nil)
	input := PricingInput{Model: "grok-imagine-image-2.0", GroupID: &groupID}
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 0.18})
	require.InDelta(t, 0.0117, quote.ImagePrice1K, 1e-12)
	cost, err := calculator.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: input.Model, GroupID: input.GroupID, RequestCount: 1, RateMultiplier: 0.18, Resolver: resolver})
	require.NoError(t, err)
	require.InDelta(t, 0.065, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.0117, cost.ActualCost, 1e-12)
}

func TestPublicQuoteIndependentPricingMatchesSettlement(t *testing.T) {
	groupID := int64(1)
	for _, mode := range []pricing.BillingMode{pricing.BillingModePerRequest, pricing.BillingModeImage, pricing.BillingModeVideo} {
		for _, rate := range []float64{0, 0.18, 3} {
			for _, unit := range []float64{0, 0.065} {
				calculator := NewCalculator(nil, CalculatorOptions{})
				card := &ModelPricingEntry{BillingMode: mode, PerRequestPrice: requestTestPrice(unit), PriceMultiplier: requestTestPrice(2), IndependentPricing: true}
				resolver := NewPriceResolver(mediaPriceCards{card: card}, calculator, nil, nil)
				input := PricingInput{Model: "custom-image", GroupID: &groupID}
				quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: rate})
				require.True(t, quote.IndependentPricing)
				require.Equal(t, "priced", quote.PriceStatus)
				size := "1K"
				if mode == pricing.BillingModeVideo {
					size = "720p"
					require.InDelta(t, unit*2, quote.VideoPrices[size], 1e-12)
				} else {
					require.InDelta(t, unit*2, quote.ImagePrice1K, 1e-12)
					image, err := resolver.ResolveImageQuote(context.Background(), input, size)
					require.NoError(t, err)
					require.True(t, image.IndependentPricing)
					require.InDelta(t, unit*2, image.UnitPrice, 1e-12)
				}
				cost, err := calculator.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: input.Model, GroupID: &groupID, RequestCount: 1, UsageUnits: 1, SizeTier: size, RateMultiplier: rate, Resolver: resolver})
				require.NoError(t, err)
				require.True(t, cost.IndependentPricing)
				require.InDelta(t, unit*2, cost.TotalCost, 1e-12)
				require.InDelta(t, cost.TotalCost, cost.ActualCost, 1e-12)
			}
		}
	}
}
