package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// public_quote_test.go compares public media quotes with their resolved unit
// prices and settlement amounts. Fixtures distinguish explicit image dimensions,
// video defaults, unavailable units, and independent prices so display metadata
// cannot silently change the user charge.

// TestPublicQuotePreservesExactImagePrices quotes explicit catalog image
// prices, including zero, with a user multiplier. It checks quote, unit
// price, and calculated charge agreement, and keeps an unknown image
// model distinguishable as unpriced.
func TestPublicQuotePreservesExactImagePrices(t *testing.T) {
	calculator := NewCalculator(defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"grok-imagine-image-quality": {CatalogRules: pricing.CatalogRules{ImagePrices: map[string]float64{"1K": 0.05}}, TokenPricingAbsent: true},
		"custom-image":               {OutputCostPerImage: 0.1, ImagePricePresent: true, TokenPricingAbsent: true, Mode: "image_generation"},
		"free-image":                 {ImagePricePresent: true, TokenPricingAbsent: true, Mode: "image_generation"},
	}}, CalculatorOptions{})
	resolver := NewPriceResolver(nil, calculator, nil, nil)
	for _, tc := range []struct {
		model string  // quoted image model
		price float64 // expected USD unit price
	}{
		{"grok-imagine-image-quality", 0.05},
		{"custom-image", 0.1},
		{"free-image", 0},
	} {
		input := PricingInput{Model: tc.model}
		quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 2})
		require.Equal(t, "priced", quote.PriceStatus, tc.model)
		require.Equal(t, "image", quote.PricingMode, tc.model)
		// Compare multiplied display amounts with a small tolerance for binary
		// floating-point noise while requiring the unit price and calculated
		// charge to retain their separately asserted values.
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

// TestPricingManagementFreeImageOverridesCatalog applies a free configured
// image card over a catalog with positive size prices. It checks that
// user unit prices become zero while the catalog prices remain available
// for each displayed size.
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
		name         string
		card         *ModelPricingEntry
		rate         float64
		expected     pricing.MediaPrices
		defaultPrice *float64
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
			rate: 0.18, defaultPrice: requestTestPrice(0.0216),
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
			rate: 1, defaultPrice: requestTestPrice(0),
		},
		{
			name: "zero group multiplier",
			card: &ModelPricingEntry{BillingMode: pricing.BillingModeVideo, PerRequestPrice: requestTestPrice(0.06)},
			rate: 0, defaultPrice: requestTestPrice(0),
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
			if testCase.defaultPrice != nil {
				require.NotNil(t, quote.VideoPricePerSecond)
				require.InDelta(t, *testCase.defaultPrice, *quote.VideoPricePerSecond, 1e-12)
				cost, err := calculator.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: input.Model, GroupID: input.GroupID, UsageUnits: 5, SizeTier: "1440p", RateMultiplier: testCase.rate, Resolver: resolver})
				require.NoError(t, err)
				require.InDelta(t, *quote.VideoPricePerSecond*5, cost.ActualCost, 1e-12)
			} else {
				require.Nil(t, quote.VideoPricePerSecond)
			}
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

// TestPublicQuoteImageCardMultiplierKeepsGroupMultiplier combines a declared
// image tier with a per-request default and a group rate. The displayed image
// amount must match the charge calculated from that same card.
func TestPublicQuoteImageCardMultiplierKeepsGroupMultiplier(t *testing.T) {
	groupID := int64(1)
	calculator := NewCalculator(nil, CalculatorOptions{})
	resolver := NewPriceResolver(mediaPriceCards{card: &ModelPricingEntry{
		BillingMode: pricing.BillingModePerRequest, PerRequestPrice: requestTestPrice(0.065), PriceMultiplier: requestTestPrice(1),
		Intervals: []pricing.PricingInterval{{TierLabel: "1K", PerRequestPrice: requestTestPrice(0.065)}},
	}}, calculator, nil, nil)
	input := PricingInput{Model: "grok-imagine-image-2.0", GroupID: &groupID}
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 0.18})
	require.InDelta(t, 0.0117, quote.ImagePrice1K, 1e-12)
	cost, err := calculator.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: input.Model, GroupID: input.GroupID, RequestCount: 1, RateMultiplier: 0.18, Resolver: resolver})
	require.NoError(t, err)
	require.InDelta(t, 0.065, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.0117, cost.ActualCost, 1e-12)
}

// TestPublicQuoteIndependentPricingMatchesSettlement checks free and paid media
// cards against zero, discounted, and increased group rates. Explicit image tiers
// and video defaults establish display units while independent pricing fixes
// the effective user multiplier at one.
func TestPublicQuoteIndependentPricingMatchesSettlement(t *testing.T) {
	groupID := int64(1)
	for _, mode := range []pricing.BillingMode{pricing.BillingModePerRequest, pricing.BillingModeImage, pricing.BillingModeVideo} {
		for _, rate := range []float64{0, 0.18, 3} {
			for _, unit := range []float64{0, 0.065} {
				calculator := NewCalculator(nil, CalculatorOptions{})
				card := &ModelPricingEntry{BillingMode: mode, PerRequestPrice: requestTestPrice(unit), PriceMultiplier: requestTestPrice(2), IndependentPricing: true}
				if mode == pricing.BillingModePerRequest {
					card.Intervals = []pricing.PricingInterval{{TierLabel: "1K", PerRequestPrice: requestTestPrice(unit)}}
				}
				resolver := NewPriceResolver(mediaPriceCards{card: card}, calculator, nil, nil)
				input := PricingInput{Model: "custom-image", GroupID: &groupID}
				quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: rate})
				require.True(t, quote.IndependentPricing)
				require.Equal(t, "priced", quote.PriceStatus, "mode=%s rate=%g unit=%g", mode, rate, unit)
				size := "1K"
				if mode == pricing.BillingModeVideo {
					size = "720p"
					require.NotNil(t, quote.VideoPricePerSecond)
					require.InDelta(t, unit*2, *quote.VideoPricePerSecond, 1e-12)
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

// TestPublicQuoteIndependentRequestRequiresDisplayDimensions checks unlabeled
// independent per-request prices. The public quote does not infer image units
// from a model name, while settlement still charges the configured independent
// amount for every request, including an explicitly free default.
func TestPublicQuoteIndependentRequestRequiresDisplayDimensions(t *testing.T) {
	groupID := int64(1)
	for _, rate := range []float64{0, 0.18, 3} {
		for _, unit := range []float64{0, 0.065} {
			calculator := NewCalculator(nil, CalculatorOptions{})
			card := &ModelPricingEntry{
				BillingMode: pricing.BillingModePerRequest, PerRequestPrice: requestTestPrice(unit),
				PriceMultiplier: requestTestPrice(2), IndependentPricing: true,
			}
			resolver := NewPriceResolver(mediaPriceCards{card: card}, calculator, nil, nil)
			input := PricingInput{Model: "custom-image", GroupID: &groupID}
			quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: rate})
			require.True(t, quote.IndependentPricing)
			require.Equal(t, "unpriced", quote.PriceStatus)
			require.Empty(t, quote.ImagePriceSizes)
			cost, err := calculator.CalculateCostUnified(CostInput{
				Ctx: context.Background(), Model: input.Model, GroupID: &groupID,
				RequestCount: 3, RateMultiplier: rate, Resolver: resolver,
			})
			require.NoError(t, err)
			require.True(t, cost.IndependentPricing)
			require.InDelta(t, unit*2*3, cost.TotalCost, 1e-12)
			require.InDelta(t, cost.TotalCost, cost.ActualCost, 1e-12)
		}
	}
}

// TestPublicVideoQuoteMatchesCatalogCharges compares custom catalog rates with
// long-video charges. Free dimensions remain present, unknown dimensions remain
// unpriced, and the administrator catalog exposes every declared resolution.
func TestPublicVideoQuoteMatchesCatalogCharges(t *testing.T) {
	calculator := NewCalculator(defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"custom-video": {CatalogRules: pricing.CatalogRules{VideoPrices: pricing.MediaPrices{"768P": 0.12, "2K": 0}}, TokenPricingAbsent: true},
	}}, CalculatorOptions{})
	resolver := NewPriceResolver(nil, calculator, nil, nil)
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: PricingInput{Model: "custom-video"}, RateMultiplier: 1.5})
	require.Equal(t, "video", quote.PricingMode)
	require.Equal(t, "priced", quote.PriceStatus)
	require.Len(t, quote.VideoPrices, 2)
	require.Nil(t, quote.VideoPricePerSecond)
	for _, resolution := range []string{"768p", "2k"} {
		cost, err := calculator.CalculateVideoCost("custom-video", resolution, 2, 60, 1.5)
		require.NoError(t, err)
		require.InDelta(t, quote.VideoPrices[resolution]*120, cost.ActualCost, 1e-12)
	}
	_, err := calculator.CalculateVideoCost("custom-video", "480p", 1, 30, 1)
	require.ErrorIs(t, err, pricing.ErrModelPricingUnavailable)
	defaults := calculator.DefaultModelPrice("custom-video", "", "video")
	require.Len(t, defaults.Prices, 2)
	require.Equal(t, "2k", defaults.Prices[0].Key)
	require.Zero(t, *defaults.Prices[0].Value)
}

// TestConfiguredVideoQuoteKeepsMissingDimensionsUnavailable selects a partial
// video card over a catalog containing other resolutions. Unified video dispatch
// rejects a missing tier while free and paid controls settle at their quoted
// rates, proving that the missing-price assertion exercises the video path.
func TestConfiguredVideoQuoteKeepsMissingDimensionsUnavailable(t *testing.T) {
	groupID := int64(1)
	zero := 0.0
	paid := 0.25
	calculator := NewCalculator(defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		"custom-video": {CatalogRules: pricing.CatalogRules{VideoPrices: pricing.MediaPrices{"480p": 0.1, "768p": 0.2}}, TokenPricingAbsent: true},
	}}, CalculatorOptions{})
	resolver := NewPriceResolver(mediaPriceCards{card: &ModelPricingEntry{
		BillingMode: pricing.BillingModeVideo,
		Intervals: []pricing.PricingInterval{
			{TierLabel: "2K", PerRequestPrice: &zero},
			{TierLabel: "1080p", PerRequestPrice: &paid},
		},
	}}, calculator, nil, nil)
	input := PricingInput{Model: "custom-video", GroupID: &groupID}
	quote := resolver.PublicQuote(context.Background(), PublicQuoteInput{PricingInput: input, RateMultiplier: 1.5})
	require.Equal(t, map[string]float64{"2k": 0, "1080p": 0.375}, quote.VideoPrices)
	require.Nil(t, quote.VideoPricePerSecond)
	resolved := resolver.Resolve(context.Background(), input)
	costInput := CostInput{
		Ctx:            context.Background(),
		Model:          input.Model,
		GroupID:        &groupID,
		Resolver:       resolver,
		Resolved:       resolved,
		UsageUnits:     60,
		SizeTier:       "768p",
		RateMultiplier: 1.5,
	}
	_, err := calculator.CalculateCostUnified(costInput)
	require.ErrorIs(t, err, pricing.ErrModelPricingUnavailable)
	for resolution, unitPrice := range map[string]float64{"2k": zero, "1080p": paid} {
		t.Run(resolution, func(t *testing.T) {
			costInput.SizeTier = resolution
			cost, err := calculator.CalculateCostUnified(costInput)
			require.NoError(t, err)
			require.Equal(t, string(pricing.BillingModeVideo), cost.BillingMode)
			// Allow binary rounding when comparing the same per-second amounts
			// through base-cost calculation and group-scaled public quotation.
			require.InDelta(t, unitPrice*60, cost.TotalCost, 1e-12)
			require.InDelta(t, quote.VideoPrices[resolution]*60, cost.ActualCost, 1e-12)
		})
	}
}
