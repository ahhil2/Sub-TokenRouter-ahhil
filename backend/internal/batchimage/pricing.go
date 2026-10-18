package batchimage

import (
	"context"
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// ImagePrices 根据价卡和模型返回图片单价。
type ImagePrices interface {
	ResolveImageQuote(context.Context, billing.PricingInput, string) (billing.ImageQuote, error)
}

// Pricing 保留任务报价的按需分组读取顺序，价格算法仍由 billing 唯一实现。
type Pricing struct {
	Resolver  ImagePrices
	GroupRepo GroupReader
}

func (r *Pricing) BatchImageUnitPrice(ctx context.Context, input BatchImagePriceInput) (float64, error) {
	quote, err := r.BatchImageQuote(ctx, input)
	return quote.UnitPrice, err
}

func (r *Pricing) BatchImageQuote(ctx context.Context, input BatchImagePriceInput) (billing.ImageQuote, error) {
	if r == nil || r.Resolver == nil {
		return billing.ImageQuote{}, ErrBatchImageSettlementPricingMissing
	}
	price, err := r.Resolver.ResolveImageQuote(ctx, billing.PricingInput{Model: input.Model, GroupID: input.GroupID}, input.ImageSize)
	if err != nil {
		return billing.ImageQuote{}, fmt.Errorf("%w: %v", ErrBatchImageSettlementPricingMissing, err)
	}
	return price, nil
}
