import type { ProviderBrandKey } from '@/utils/providerBrand'
import type { ProviderPlatform } from '@/api/admin/providers.types'
import { resolveProviderBrandKey } from '@/utils/providerBrand'

/**
 * keyGroupProviders.ts classifies routing groups into the provider filters offered
 * during API-key creation. Explicit display brands take precedence over names,
 * recognized domestic brands share a category, and mixed provider families are placed
 * in the other category.
 */

export type KeyGroupProvider = 'anthropic' | 'openai' | 'domestic' | 'other'

export const KEY_GROUP_PROVIDERS: KeyGroupProvider[] = ['anthropic', 'openai', 'domestic', 'other']

export const KEY_GROUP_PROVIDER_ICONS: Record<KeyGroupProvider, ProviderPlatform[]> = {
  anthropic: ['anthropic'],
  openai: ['openai'],
  domestic: ['deepseek', 'kimi'],
  other: ['gemini', 'grok'],
}

const DOMESTIC_PROVIDER_BRANDS = new Set<ProviderBrandKey>([
  'deepseek',
  'moonshot',
  'zhipu',
  'alibaba',
  'baidu',
  'iflytek',
  'tencent',
  'ai360',
  'zeroone',
  'doubao',
  'xiaomi',
  'minimax',
])

export function getKeyGroupProvider(group: {
  display_brand?: string | null
  name: string
}): KeyGroupProvider {
  const brand = group.display_brand?.trim() || group.name
  const brandKey = resolveProviderBrandKey(brand)
  const normalizedBrand = brand.toLocaleLowerCase().replace(/[\s_-]+/g, '')
  const brandParts = brand.split(/[|,/+:，、]+|\b(?:and|with)\b|与|和|混合/iu)
  const partProviders = new Set(
    brandParts
      .map((part) => resolveProviderBrandKey(part))
      .filter((key): key is ProviderBrandKey => key !== null)
      .map(toKeyGroupProvider),
  )

  if (partProviders.size > 1) return 'other'
  if (partProviders.size === 1) return [...partProviders][0]!
  if (normalizedBrand.includes('零一') || normalizedBrand.includes('360')) return 'domestic'
  return toKeyGroupProvider(brandKey)
}

function toKeyGroupProvider(brandKey: ProviderBrandKey | null): KeyGroupProvider {
  if (brandKey === 'anthropic' || brandKey === 'openai') return brandKey
  if (brandKey && DOMESTIC_PROVIDER_BRANDS.has(brandKey)) return 'domestic'
  return 'other'
}
