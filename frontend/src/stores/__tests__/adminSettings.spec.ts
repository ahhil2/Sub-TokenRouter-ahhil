import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminSettingsStore } from '../adminSettings'

/**
 * adminSettings.spec.ts verifies administrator settings request sharing, retry, and credential-scoped publication.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

const api = vi.hoisted(() => ({ settings: vi.fn(), payment: vi.fn() }))
vi.mock('@/api', () => ({
  adminAPI: { settings: { getSettings: api.settings }, payment: { getConfig: api.payment } },
}))
vi.mock('@/i18n', () => ({ getLocale: () => 'zh' }))

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  vi.clearAllMocks()
  api.settings.mockResolvedValue({ ops_monitoring_enabled: true, custom_menu_items: [] })
  api.payment.mockResolvedValue({ data: { enabled: true } })
})
afterEach(() => {
  useAdminSettingsStore().$dispose()
  vi.restoreAllMocks()
})

describe('管理员设置共享状态', () => {
  // Two consumers fetch administrator navigation settings before the first response
  // completes. Both must await the same request and observe jointly published settings and
  // payment availability.
  it('多个调用方等待同一请求完成', async () => {
    let resolve!: (value: unknown) => void
    api.settings.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes
      }),
    )
    const store = useAdminSettingsStore()
    const first = store.fetch()
    let secondFinished = false
    const second = store.fetch().then(() => {
      secondFinished = true
    })
    await Promise.resolve()
    expect(secondFinished).toBe(false)
    expect(api.settings).toHaveBeenCalledOnce()
    resolve({ ops_monitoring_enabled: false })
    await Promise.all([first, second])
    expect(store.loaded).toBe(true)
    expect(store.opsMonitoringEnabled).toBe(false)
    expect(store.paymentEnabled).toBe(true)
  })

  // The settings endpoint initially fails and then recovers before a forced refresh.
  // Readiness must permit retry, and force must reach the API cache layer.
  it('请求失败后重试，强制刷新越过 API 缓存', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.settings.mockRejectedValueOnce(new Error('offline'))
    const store = useAdminSettingsStore()
    await store.fetch()
    expect(store.loaded).toBe(false)
    await store.fetch()
    expect(store.loaded).toBe(true)
    await store.fetch(true)
    expect(api.settings).toHaveBeenLastCalledWith(true)
    expect(api.settings).toHaveBeenCalledTimes(3)
  })

  // Credentials change while an earlier administrator settings response remains pending. The
  // newer account response must retain ownership when the old account response finally
  // completes.
  it('切换账号后旧响应不能覆盖当前功能开关', async () => {
    let resolve!: (value: unknown) => void
    api.settings.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes
      }),
    )
    localStorage.setItem('auth_token', 'user-a')
    const store = useAdminSettingsStore()
    const old = store.fetch()
    localStorage.setItem('auth_token', 'user-b')
    api.settings.mockResolvedValueOnce({ ops_monitoring_enabled: false })
    await store.fetch()
    resolve({ ops_monitoring_enabled: true })
    await old
    expect(store.opsMonitoringEnabled).toBe(false)
    expect(api.settings).toHaveBeenCalledTimes(2)
  })
})
