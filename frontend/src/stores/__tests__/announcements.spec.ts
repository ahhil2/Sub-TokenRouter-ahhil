import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAnnouncementStore } from '@/stores/announcements'

// announcements.spec.ts verifies announcement fetch admission and retention of the full
// visible list. The API double records ordinary and forced request arguments. A list
// larger than the popup window protects the complete visible catalog used by other
// announcement surfaces.

// mockList records the announcement API's visibility and force arguments while each test
// controls the returned visible list.
const mockList = vi.fn()
const mockRead = vi.fn()

vi.mock('@/api/announcements', () => ({
  default: {
    list: (...args: unknown[]) => mockList(...args),
    markRead: (...args: unknown[]) => mockRead(...args),
  },
}))

describe('useAnnouncementStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockList.mockResolvedValue([])
  })

  // An old account's fetch completes while the replacement account still waits.
  // Its response must leave the new list empty and preserve the new loading owner.
  it('discards a previous session list without clearing the new request loading state', async () => {
    const store = useAnnouncementStore()
    const oldList = Promise.withResolvers<unknown[]>()
    const newList = Promise.withResolvers<unknown[]>()
    mockList.mockReturnValueOnce(oldList.promise).mockReturnValueOnce(newList.promise)
    const first = store.fetchAnnouncements()
    store.reset()
    const second = store.fetchAnnouncements()
    oldList.resolve([{ id: 1, notify_mode: 'popup' }])
    await first
    expect(store.announcements).toEqual([])
    expect(store.currentPopup).toBeNull()
    expect(store.loading).toBe(true)
    newList.resolve([])
    await second
    expect(store.loading).toBe(false)
  })

  // Six unread announcements include one failed acknowledgment and five successful writes.
  // Only four requests start together, and successful local timestamps survive the failure.
  it('bounds read requests and keeps only successful acknowledgments', async () => {
    const store = useAnnouncementStore()
    mockList.mockResolvedValue(Array.from({ length: 6 }, (_, index) => ({ id: index + 1 })))
    await store.fetchAnnouncements()
    const gate = Promise.withResolvers<void>()
    mockRead.mockImplementation((id: number) =>
      id === 2 ? Promise.reject(Error('offline')) : gate.promise,
    )
    const batch = store.markAllAsRead()
    expect(mockRead).toHaveBeenCalledTimes(4)
    gate.resolve()
    await expect(batch).rejects.toThrow('offline')
    expect(mockRead).toHaveBeenCalledTimes(6)
    expect(store.announcements.filter((item) => !item.read_at).map((item) => item.id)).toEqual([2])
  })

  // Logout resets the store while its first acknowledgment group remains pending.
  // Completing that group must neither dispatch queued reads nor alter the replacement list.
  it('stops dispatching queued reads after a session reset', async () => {
    const store = useAnnouncementStore()
    mockList.mockResolvedValue(Array.from({ length: 6 }, (_, index) => ({ id: index + 1 })))
    await store.fetchAnnouncements()
    const gate = Promise.withResolvers<void>()
    mockRead.mockReturnValue(gate.promise)
    const batch = store.markAllAsRead()
    store.reset()
    await store.fetchAnnouncements()
    gate.resolve()
    await batch
    expect(mockRead).toHaveBeenCalledTimes(4)
    expect(store.unreadCount).toBe(6)
  })

  // An ordinary announcement fetch is followed immediately by a forced fetch. The second
  // request bypasses admission throttling and forwards force to the API cache controls.
  it('强制刷新时同时绕过请求节流和浏览器缓存', async () => {
    const store = useAnnouncementStore()

    await store.fetchAnnouncements()
    await store.fetchAnnouncements(true)

    expect(mockList).toHaveBeenNthCalledWith(1, false, false)
    expect(mockList).toHaveBeenNthCalledWith(2, false, true)
  })

  // The announcement endpoint returns more records than the popup candidate limit. The
  // store retains the complete visible list so consumers can sort and count all
  // announcements.
  it('保留完整可见公告列表供不同界面独立排序', async () => {
    const store = useAnnouncementStore()
    mockList.mockResolvedValue(
      Array.from({ length: 25 }, (_, index) => ({
        id: index + 1,
        title: `公告 ${index + 1}`,
        content: `内容 ${index + 1}`,
        notify_mode: 'silent',
        created_at: '2026-08-01T12:00:00Z',
        updated_at: '2026-08-01T12:00:00Z',
      })),
    )

    await store.fetchAnnouncements()

    expect(store.announcements).toHaveLength(25)
  })
})
