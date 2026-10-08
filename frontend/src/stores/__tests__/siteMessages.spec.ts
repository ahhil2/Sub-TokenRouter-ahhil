import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useSiteMessagesStore } from '../siteMessages'

const mocks = vi.hoisted(() => ({ auth: { user: null as { id: number } | null }, summary: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/api/siteMessages', () => ({ siteMessagesAPI: { summary: mocks.summary } }))

beforeEach(() => {
  setActivePinia(createPinia())
  mocks.auth = reactive({ user: { id: 2 } as { id: number } | null })
  mocks.summary.mockReset()
})

describe('site message unread state', () => {
  it('coalesces parallel requests and clears the badge on logout', async () => {
    let resolve!: (value: { unread_count: number }) => void
    mocks.summary.mockReturnValue(new Promise(done => { resolve = done }))
    const store = useSiteMessagesStore()
    const first = store.refresh()
    const second = store.refresh()
    expect(mocks.summary).toHaveBeenCalledTimes(1)
    resolve({ unread_count: 7 })
    await Promise.all([first, second])
    expect(store.unreadCount).toBe(7)
    mocks.auth.user = null
    expect(store.unreadCount).toBe(0)
    await store.refresh()
    expect(mocks.summary).toHaveBeenCalledTimes(1)
  })

  it('ignores a previous account response after switching accounts', async () => {
    let resolveOld!: (value: { unread_count: number }) => void
    mocks.summary.mockReturnValueOnce(new Promise(done => { resolveOld = done }))
    const store = useSiteMessagesStore()
    const oldRequest = store.refresh()
    mocks.auth.user = { id: 3 }
    mocks.summary.mockResolvedValueOnce({ unread_count: 2 })
    await store.refresh()
    resolveOld({ unread_count: 99 })
    await oldRequest
    expect(store.unreadCount).toBe(2)
  })
})
