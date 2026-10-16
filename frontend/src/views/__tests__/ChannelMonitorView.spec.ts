import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ChannelMonitorView from '../ChannelMonitorView.vue'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'
import { getChannelMonitor } from '@/api/channelMonitor'

const auth = vi.hoisted(() => ({ isAuthenticated: false, isAdmin: false }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div data-test="app-layout"><aside data-test="sidebar" /><slot /></div>' } }))
vi.mock('@/api/channelMonitor', () => ({ getChannelMonitor: vi.fn() }))
vi.mock('@/components/layout/AppHeader.vue', () => ({ default: { template: '<header />' } }))
vi.mock('vue-i18n', async () => {
  const { ref } = await import('vue')
  return { createI18n: () => ({ install() {} }), useI18n: () => ({ locale: ref('zh'), t: (key: string, values: Record<string, unknown> = {}) => {
    const message = key.replace(/^channelMonitor\./, '').split('.').reduce<unknown>((value, part) => (value as Record<string, unknown>)[part], channelMonitor)
    return String(message).replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ''))
  } }) }
})

const response = { generated_at: '2026-10-13T00:00:00Z', window_minutes: 90, slot_seconds: 270, refresh_seconds: 30, monitoring_enabled: true, channels: [] }

function render() {
  return mount(ChannelMonitorView, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { channelMonitor } } })], stubs: { ProviderIcon: true, ChannelMonitorCard: true, ChannelMonitorSettings: true } } })
}

describe('ChannelMonitorView', () => {
  beforeEach(() => {
    auth.isAuthenticated = false
    auth.isAdmin = false
    vi.useFakeTimers()
    vi.mocked(getChannelMonitor).mockReset().mockResolvedValue(response)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

  it('refreshes every thirty seconds and cleans up on unmount', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('暂无可展示的公开渠道')
    await vi.advanceTimersByTimeAsync(30000)
    expect(getChannelMonitor).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60000)
    expect(getChannelMonitor).toHaveBeenCalledTimes(2)
  })

  it('keeps the previous snapshot with a stale notice after errors', async () => {
    const wrapper = render()
    await flushPromises()
    vi.mocked(getChannelMonitor).mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(30000)
    expect(wrapper.get('[role="alert"]').text()).toContain('上次获取的数据')
    expect(wrapper.text()).toContain('更新于')
    wrapper.unmount()
  })

  it('pauses polling in hidden tabs and refreshes when visible', async () => {
    const wrapper = render()
    await flushPromises()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await vi.advanceTimersByTimeAsync(60000)
    expect(getChannelMonitor).toHaveBeenCalledTimes(1)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getChannelMonitor).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows monitoring disabled instead of a healthy empty page', async () => {
    vi.mocked(getChannelMonitor).mockResolvedValueOnce({ ...response, monitoring_enabled: false })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('渠道监控暂未开启')
    expect(wrapper.find('.monitor-legend').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the app sidebar for signed-in users and hides configuration', async () => {
    auth.isAuthenticated = true
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="sidebar"]').exists()).toBe(true)
    expect(wrapper.find('header').exists()).toBe(false)
    expect(wrapper.find('[aria-controls="channel-monitor-settings"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('lets administrators configure monitoring within the app layout', async () => {
    auth.isAuthenticated = true
    auth.isAdmin = true
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="sidebar"]').exists()).toBe(true)
    await wrapper.get('[aria-controls="channel-monitor-settings"]').trigger('click')
    expect(wrapper.findComponent({ name: 'ChannelMonitorSettings' }).exists()).toBe(true)
    wrapper.findComponent({ name: 'ChannelMonitorSettings' }).vm.$emit('saved')
    await flushPromises()
    expect(getChannelMonitor).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps guests on the public layout without configuration', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('header').exists()).toBe(true)
    expect(wrapper.find('[data-test="sidebar"]').exists()).toBe(false)
    expect(wrapper.find('[aria-controls="channel-monitor-settings"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('refreshes again when settings are saved during an in-flight poll', async () => {
    auth.isAuthenticated = true
    auth.isAdmin = true
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[aria-controls="channel-monitor-settings"]').trigger('click')
    let finish: (value: typeof response) => void = () => {}
    vi.mocked(getChannelMonitor).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await vi.advanceTimersByTimeAsync(30000)
    wrapper.findComponent({ name: 'ChannelMonitorSettings' }).vm.$emit('saved')
    finish(response)
    await flushPromises()
    expect(getChannelMonitor).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })
})
