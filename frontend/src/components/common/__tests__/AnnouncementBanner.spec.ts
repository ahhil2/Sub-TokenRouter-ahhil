import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AnnouncementBanner from '../AnnouncementBanner.vue'
import { useAnnouncementStore } from '@/stores/announcements'

/**
 * AnnouncementBanner.spec.ts checks which announcements appear in the persistent
 * banner region. Store fixtures distinguish delivery modes and read state, while
 * component assertions verify that an empty banner collection leaves no unused
 * announcement region.
 */

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api', () => ({ announcementsAPI: { list: vi.fn(), markRead: vi.fn() } }))
enableAutoUnmount(afterEach)

describe('AnnouncementBanner', () => {
  it('shows only banners and keeps read banners visible', () => {
    setActivePinia(createPinia())
    const store = useAnnouncementStore()
    store.announcements = [
      {
        id: 1,
        title: '顶部公告',
        content: '**余额** 123',
        notify_mode: 'banner',
        read_at: '2026-10-19',
        created_at: '',
        updated_at: '',
      },
      {
        id: 2,
        title: '弹窗',
        content: 'popup',
        notify_mode: 'popup',
        created_at: '',
        updated_at: '',
      },
    ]
    const wrapper = mount(AnnouncementBanner)
    expect(wrapper.get('strong').text()).toBe('余额')
    expect(wrapper.text()).not.toContain('popup')
  })

  it('leaves no empty announcement region', () => {
    setActivePinia(createPinia())
    expect(mount(AnnouncementBanner).find('aside').exists()).toBe(false)
  })
})
