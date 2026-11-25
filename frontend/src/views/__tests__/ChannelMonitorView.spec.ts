import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ChannelMonitorView from '../ChannelMonitorView.vue'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'
import { getChannelMonitor } from '@/api/channelMonitor'

/**
 * ChannelMonitorView.spec.ts exercises monitoring refresh, stale data, document
 * visibility, and role-dependent configuration. Mocked snapshots and timers expose
 * refresh behavior and unmount cleanup, while layout assertions distinguish anonymous
 * visitors, signed-in users, and administrators.
 */

const auth = vi.hoisted(() => ({ isAuthenticated: false, isAdmin: false }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div data-test="app-layout"><aside data-test="sidebar" /><slot /></div>' },
}))
vi.mock('@/api/channelMonitor', () => ({ getChannelMonitor: vi.fn() }))
vi.mock('@/components/layout/AppHeader.vue', () => ({ default: { template: '<header />' } }))
vi.mock('vue-i18n', async () => {
  const { ref } = await import('vue')
  return {
    createI18n: () => ({ install() {} }),
    useI18n: () => ({
      locale: ref('zh'),
      t: (key: string, values: Record<string, unknown> = {}) => {
        const message = key
          .replace(/^channelMonitor\./, '')
          .split('.')
          .reduce<unknown>(
            (value, part) => (value as Record<string, unknown>)[part],
            channelMonitor,
          )
        return String(message).replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ''))
      },
    }),
  }
})

const response = {
  generated_at: '2026-10-13T00:00:00Z',
  window_minutes: 90,
  slot_seconds: 270,
  refresh_seconds: 30,
  monitoring_enabled: true,
  channels: [],
}

function render() {
  return mount(ChannelMonitorView, {
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { channelMonitor } } })],
      stubs: { ProviderIcon: true, ChannelMonitorCard: true, ChannelMonitorSettings: true },
    },
  })
}

describe('ChannelMonitorView', () => {
  beforeEach(() => {
    auth.isAuthenticated = false
    auth.isAdmin = false
    vi.useFakeTimers()
    vi.mocked(getChannelMonitor).mockReset().mockResolvedValue(response)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

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
    vi.mocked(getChannelMonitor).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    await vi.advanceTimersByTimeAsync(30000)
    wrapper.findComponent({ name: 'ChannelMonitorSettings' }).vm.$emit('saved')
    finish(response)
    await flushPromises()
    expect(getChannelMonitor).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  // Saved order can interleave brands before the page groups their cards.
  // Refreshing after a settings save updates brand sections by first occurrence
  // and keeps the returned order of channels within each section.
  it('uses saved channel order for brand sections and cards after refreshing', async () => {
    auth.isAuthenticated = true
    auth.isAdmin = true
    const channels = [
      { id: 3, name: 'Gemini Pro', brand: 'Google' },
      { id: 1, name: 'GPT', brand: 'OpenAI' },
      { id: 4, name: 'Gemini Flash', brand: 'Google' },
    ].map((channel) => ({
      ...channel,
      rate_multiplier: 1,
      requests: 0,
      successes: 0,
      availability: null,
      cache_rate: null,
      ttft_p50_ms: null,
      slots: [],
    }))
    vi.mocked(getChannelMonitor).mockResolvedValueOnce({ ...response, channels })
    const wrapper = render()
    await flushPromises()
    let sections = wrapper.findAll('.monitor-brand-group')
    expect(sections.map((section) => section.attributes('aria-label'))).toEqual([
      'Google',
      'OpenAI',
    ])
    expect(
      sections[0]
        .findAllComponents({ name: 'ChannelMonitorCard' })
        .map((card) => card.props('channel').id),
    ).toEqual([3, 4])
    vi.mocked(getChannelMonitor).mockResolvedValueOnce({
      ...response,
      channels: [channels[1], channels[2], channels[0]],
    })
    await wrapper.get('[aria-controls="channel-monitor-settings"]').trigger('click')
    wrapper.findComponent({ name: 'ChannelMonitorSettings' }).vm.$emit('saved')
    await flushPromises()
    sections = wrapper.findAll('.monitor-brand-group')
    expect(sections.map((section) => section.attributes('aria-label'))).toEqual([
      'OpenAI',
      'Google',
    ])
    expect(
      sections[1]
        .findAllComponents({ name: 'ChannelMonitorCard' })
        .map((card) => card.props('channel').id),
    ).toEqual([4, 3])
    wrapper.unmount()
  })
})
