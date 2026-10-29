import { apiClient } from './client'

/**
 * pelican.ts exposes scheduled model drawing tasks and their published results.
 * Public browsing and administrator inspection use separate endpoints, while
 * shared response shapes let their screens display task status, execution timing,
 * and generated content consistently.
 */

export interface PelicanTask {
  id: number
  group_id: number
  group_name: string
  model: string
  enabled: boolean
  interval_minutes: number
  public_consent: boolean
  manual_requested: boolean
  next_run_at: string
  lease_until: string | null
  last_status: string
  last_run_at: string | null
}

export interface PelicanResult {
  id: number
  task_id: number
  group_id: number
  group_name: string
  model: string
  status: 'running' | 'success' | 'failed' | 'interrupted'
  published: boolean
  latency_ms: number
  started_at: string
  finished_at: string | null
  provider_id?: number
  execution_model?: string
  raw_response?: string
  html?: string
  error_message?: string
}

export interface PelicanFilter {
  page?: number
  group_id?: number
  task_id?: number
  model?: string
  date?: string
}

export interface PelicanPage {
  items: PelicanResult[]
  has_more: boolean
  prompt: string
}
export interface PelicanCatalogItem {
  group_id: number
  group_name: string
  model: string
}
export type PelicanTaskInput = Pick<
  PelicanTask,
  'group_id' | 'model' | 'enabled' | 'interval_minutes' | 'public_consent'
>

export const pelicanApi = {
  async catalog() {
    return (await apiClient.get<PelicanCatalogItem[]>('/pelican-showcase/catalog')).data
  },
  async results(filter: PelicanFilter, admin = false, signal?: AbortSignal) {
    return (
      await apiClient.get<PelicanPage>(
        admin ? '/admin/pelican-tests/results' : '/pelican-showcase',
        { params: filter, signal },
      )
    ).data
  },
  async result(id: number, admin = false, signal?: AbortSignal) {
    return (
      await apiClient.get<PelicanResult>(
        admin ? `/admin/pelican-tests/results/${id}` : `/pelican-showcase/items/${id}`,
        { signal },
      )
    ).data
  },
  async tasks() {
    return (await apiClient.get<PelicanTask[]>('/admin/pelican-tests')).data
  },
  async save(input: PelicanTaskInput, id?: number) {
    return id
      ? (await apiClient.put<PelicanTask>(`/admin/pelican-tests/${id}`, input)).data
      : (await apiClient.post<PelicanTask>('/admin/pelican-tests', input)).data
  },
  async run(id: number) {
    await apiClient.post(`/admin/pelican-tests/${id}/run`)
  },
  async removeTask(id: number) {
    await apiClient.delete(`/admin/pelican-tests/${id}`)
  },
  async publish(id: number, published: boolean) {
    await apiClient.patch(`/admin/pelican-tests/results/${id}`, { published })
  },
  async removeResult(id: number) {
    await apiClient.delete(`/admin/pelican-tests/results/${id}`)
  },
}
