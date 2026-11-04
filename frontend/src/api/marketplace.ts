import { apiClient } from './client'
import { createCachedRequest } from './readCache'
import type { MarketplaceGroup, MarketplaceStats } from '@/api/marketplace.types'

/**
 * marketplace.ts retrieves public model-marketplace groups and summary
 * statistics. The model query disables capacity aggregation because the
 * marketplace display uses prices and availability, and both responses are
 * returned through the shared browser API client.
 */

/**
 * getMarketplaceModels retrieves marketplace models and returns the server
 * response data.
 */
export async function getMarketplaceModels(): Promise<MarketplaceGroup[]> {
  /**
   * The marketplace needs prices and availability for display; skipping capacity
   * aggregation reduces the server work needed for this request.
   */
  const { data } = await apiClient.get<MarketplaceGroup[]>('/marketplace/models', {
    params: { include_capacity: false },
  })
  return data
}

/**
 * MarketplaceModelOptions contains group labels and model identifiers for administrative
 * selectors.
 */
export interface MarketplaceModelOptions {
  id: number // Group numeric identifier.
  name: string // Localized group name.
  models: Array<{ id: string /* Requestable model identifier. */ }> // Available model choices.
}

/**
 * getMarketplaceModelOptions returns cached selector data with capacity observation
 * disabled. The shared reader separates credentials and languages, deduplicates pending
 * requests, and returns an independently editable copy to each caller.
 */
export const getMarketplaceModelOptions = createCachedRequest(
  async (): Promise<MarketplaceModelOptions[]> => {
    const { data } = await apiClient.get<MarketplaceModelOptions[]>('/marketplace/models', {
      params: { view: 'options', include_capacity: false },
    })
    return data
  },
)

/**
 * getMarketplaceStats retrieves marketplace stats and returns the server
 * response data.
 */
export async function getMarketplaceStats(): Promise<MarketplaceStats> {
  const { data } = await apiClient.get<MarketplaceStats>('/marketplace/stats')
  return data
}

/**
 * marketplaceAPI groups public model-marketplace groups and statistics for
 * callers using the module's named or default API object.
 */
export const marketplaceAPI = {
  getMarketplaceModels,
  getMarketplaceModelOptions,
  getMarketplaceStats,
}

export default marketplaceAPI
