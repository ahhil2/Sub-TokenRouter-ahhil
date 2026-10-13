import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ChannelMonitorView from '../ChannelMonitorView.vue'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'
import { getChannelMonitor } from '@/api/channelMonitor'

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
  return mount(ChannelMonitorView, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { channelMonitor } } })], stubs: { ProviderIcon: true, ChannelMonitorCard: true } } })
}

describe('ChannelMonitorView', () => {
  beforeEach(() => {
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
})
