import { describe, expect, it } from 'vitest'
import { pricingKind, videoPricingTiers } from '../marketplacePricing'
import type { MarketplaceModelPricing } from '@/types'

describe('video marketplace pricing', () => {
  it('orders resolutions and preserves zero without inventing missing prices', () => {
    const pricing: MarketplaceModelPricing = {
      pricing_mode: 'video', price_status: 'priced', video_prices: { '1080p': 0.0324, '480p': 0 },
    }
    expect(pricingKind(pricing)).toBe('video')
    expect(videoPricingTiers(pricing)).toEqual([
      { resolution: '480p', price: 0 },
      { resolution: '1080p', price: 0.0324 },
    ])
  })

  it.each([undefined, -1, NaN, Infinity])('treats invalid or missing video price %s as unpriced', (price) => {
    const pricing: MarketplaceModelPricing = {
      pricing_mode: 'video', price_status: 'priced', video_prices: { '480p': price },
    }
    expect(pricingKind(pricing)).toBe('unpriced')
    expect(videoPricingTiers(pricing)).toEqual([])
  })

  it('respects the backend unpriced status', () => {
    expect(pricingKind({ pricing_mode: 'video', price_status: 'unpriced', video_prices: { '480p': 0.06 } })).toBe('unpriced')
  })
})
