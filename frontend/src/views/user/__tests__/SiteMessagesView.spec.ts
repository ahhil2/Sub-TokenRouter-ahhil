import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import SiteMessagesView from '../SiteMessagesView.vue'
import MessageComposer from '@/components/messages/MessageComposer.vue'
import type { MessageThread } from '@/api/siteMessages'

const mocks = vi.hoisted(() => ({
  auth: { user: { id: 2 }, isAdmin: false },
  route: { query: {} as Record<string, string> },
  replace: vi.fn(), refresh: vi.fn(), showSuccess: vi.fn(),
  api: { conversations: vi.fn(), thread: vi.fn(), markRead: vi.fn(), send: vi.fn() },
}))
vi.mock('@/api/siteMessages', () => ({ siteMessagesAPI: mocks.api }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot name="page-heading-actions" /><slot /></div>' } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess }) }))
vi.mock('@/stores/siteMessages', () => ({ useSiteMessagesStore: () => ({ refresh: mocks.refresh }) }))
vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh-Hans' } }) }))

function thread(userID = 2, body = 'Hello 😊'): MessageThread {
  return {
    contact: { id: userID, username: `User ${userID}`, email: `user${userID}@example.invalid` },
    items: [{ id: userID * 10, user_id: userID, sender_id: 1, is_admin: true, body, created_at: '2026-10-08T12:00:00Z' }],
    has_more: false, read_through_id: 0, peer_read_through_id: 0,
  }
}

function mountView() {
  return shallowMount(SiteMessagesView, { global: { stubs: { AppLayout: { template: '<div><slot name="page-heading-actions" /><slot /></div>' }, Icon: true } } })
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.auth.isAdmin = false
  mocks.route.query = {}
  vi.stubGlobal('crypto', { randomUUID: () => '019bb611-0872-7000-8000-000000000001' })
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  mocks.api.conversations.mockResolvedValue({ items: [], has_more: false })
  mocks.api.thread.mockResolvedValue(thread())
  mocks.api.markRead.mockResolvedValue(undefined)
  mocks.api.send.mockResolvedValue({ recipient_count: 1 })
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('SiteMessagesView', () => {
  it('ignores another user in the URL and renders message content as text', async () => {
    mocks.route.query = { user: '99' }
    mocks.api.thread.mockResolvedValue(thread(2, '<img src=x onerror=alert(1)> 😊'))
    const wrapper = mountView()
    await flushPromises()
    expect(mocks.api.thread).toHaveBeenCalledWith(2, 0)
    expect(mocks.api.conversations).not.toHaveBeenCalled()
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)> 😊')
    expect(mocks.api.markRead).toHaveBeenCalledWith(2, 20)
    wrapper.unmount()
  })

  it('does not mark messages read in a hidden tab', async () => {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Hello 😊')
    expect(mocks.api.markRead).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('retries a reply with the same idempotency key and no recipient override', async () => {
    mocks.api.send.mockRejectedValueOnce(new Error('network timeout'))
    const wrapper = mountView()
    await flushPromises()
    wrapper.getComponent(MessageComposer).vm.$emit('update:modelValue', 'Thanks 👍')
    await flushPromises()
    wrapper.getComponent(MessageComposer).vm.$emit('submit')
    await flushPromises()
    expect(wrapper.getComponent(MessageComposer).props('modelValue')).toBe('Thanks 👍')
    wrapper.getComponent(MessageComposer).vm.$emit('submit')
    await flushPromises()
    expect(mocks.api.send).toHaveBeenCalledTimes(2)
    expect(mocks.api.send.mock.calls[0][0]).toEqual(mocks.api.send.mock.calls[1][0])
    expect(mocks.api.send.mock.calls[0][0]).not.toHaveProperty('recipient_ids')
    expect(wrapper.getComponent(MessageComposer).props('modelValue')).toBe('')
    wrapper.unmount()
  })

  it('discards a stale conversation response after the administrator changes users', async () => {
    mocks.auth.isAdmin = true
    mocks.route.query = { user: '2' }
    let resolveFirst!: (value: MessageThread) => void
    const first = new Promise<MessageThread>(resolve => { resolveFirst = resolve })
    mocks.api.thread.mockImplementation((userID: number) => userID === 2 ? first : Promise.resolve(thread(3, 'Only user three')))
    mocks.api.conversations.mockResolvedValue({ items: [{ user_id: 3, username: 'User three', email: 'three@example.invalid', body: 'Preview', unread_count: 1 }], has_more: false })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('User three'))!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Only user three')
    resolveFirst(thread(2, 'Stale private message'))
    await flushPromises()
    expect(wrapper.text()).not.toContain('Stale private message')
    expect(mocks.api.markRead).toHaveBeenCalledWith(3, 30)
    expect(mocks.api.markRead).not.toHaveBeenCalledWith(2, 20)
    wrapper.unmount()
  })
})
