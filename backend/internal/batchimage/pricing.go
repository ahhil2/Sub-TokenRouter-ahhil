package batchimage

import (
	"context"
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// pricing.go resolves batch-image unit prices through the shared billing price
// resolver.
// Batch estimates need a per-image price for the selected billing model and
// image size. Pricing forwards that lookup to the shared billing resolver and
// wraps lookup failures with the missing-task-price category used by
// submission and settlement.

// ImagePrices resolves a per-image unit price using billing rules and output
// size.
type ImagePrices interface {
	// ResolveImageQuote returns the selected image unit price and independent-price
	// flag for a billing model and size. Configured zero prices remain valid;
	// unavailable pricing returns an error to admission and settlement callers.
	ResolveImageQuote(context.Context, billing.PricingInput, string) (billing.ImageQuote, error)
}

// Pricing delegates batch unit-price lookup to the shared image pricing
// resolver.
type Pricing struct {
	Resolver  ImagePrices // shared image price resolver
	GroupRepo GroupReader // unused group reader dependency
}

// BatchImageUnitPrice resolves Model and ImageSize within GroupID through the
// shared resolver. The optional Group view is not forwarded. A missing resolver
// or lookup error returns the batch pricing category; a successful configured
// zero price is returned unchanged.
func (r *Pricing) BatchImageUnitPrice(ctx context.Context, input BatchImagePriceInput) (float64, error) {
	quote, err := r.BatchImageQuote(ctx, input)
	return quote.UnitPrice, err
}

// BatchImageQuote resolves Model, GroupID, and ImageSize into a price snapshot
// input, including the independent-price flag. Missing resolvers and lookup
// failures return the batch missing-price category; configured zero prices succeed.
func (r *Pricing) BatchImageQuote(ctx context.Context, input BatchImagePriceInput) (billing.ImageQuote, error) {
	if r == nil || r.Resolver == nil {
		return billing.ImageQuote{}, ErrBatchImageSettlementPricingMissing
	}
	price, err := r.Resolver.ResolveImageQuote(ctx, billing.PricingInput{Model: input.Model, GroupID: input.GroupID}, input.ImageSize)
	if err != nil {
		// Admission and settlement inspect the batch missing-price category. Retain
		// the resolver's diagnostic text while exposing that category to those callers.
		return billing.ImageQuote{}, fmt.Errorf("%w: %v", ErrBatchImageSettlementPricingMissing, err)
	}
	return price, nil
}
