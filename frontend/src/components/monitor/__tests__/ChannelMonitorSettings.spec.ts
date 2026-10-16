import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChannelMonitorSettings from '../ChannelMonitorSettings.vue'
import { getChannelMonitorConfig, saveChannelMonitorConfig } from '@/api/channelMonitor'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'

vi.mock('@/api/channelMonitor', () => ({ getChannelMonitorConfig: vi.fn(), saveChannelMonitorConfig: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => {
  const message = key.replace(/^channelMonitor\./, '').split('.').reduce<unknown>((value, part) => (value as Record<string, unknown>)[part], channelMonitor)
  return String(message)
} }) }))

function render() {
  return mount(ChannelMonitorSettings, { global: { stubs: { Select: true } } })
}

describe('ChannelMonitorSettings', () => {
  beforeEach(() => {
    vi.mocked(getChannelMonitorConfig).mockReset().mockResolvedValue([
      { group_id: 1, name: 'GPT', brand: 'OpenAI', enabled: true, status: 'auto', eligible: true },
      { group_id: 2, name: 'Claude', brand: 'Anthropic', enabled: false, status: 'auto', eligible: false }
    ])
    vi.mocked(saveChannelMonitorConfig).mockReset().mockResolvedValue(undefined)
  })

  it('loads persisted selections and saves only selected groups with their statuses', async () => {
    const wrapper = render()
    await flushPromises()
    expect((wrapper.get('[data-group-id="1"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-group-id="2"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.text()).toContain('当前不对用户展示')
    await wrapper.get('[data-group-id="1"]').setValue(false)
    await wrapper.get('[data-group-id="2"]').setValue(true)
    wrapper.findAllComponents({ name: 'Select' })[1].vm.$emit('update:modelValue', 'maintenance')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(saveChannelMonitorConfig).toHaveBeenCalledWith([{ group_id: 2, enabled: true, status: 'maintenance' }])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.text()).toContain('配置已保存')
    wrapper.unmount()
  })

  it('allows clearing every selection', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-group-id="1"]').setValue(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(saveChannelMonitorConfig).toHaveBeenCalledWith([])
    wrapper.unmount()
  })

  it('blocks saving after a failed load and allows retrying', async () => {
    vi.mocked(getChannelMonitorConfig).mockRejectedValueOnce(new Error('offline'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('配置加载失败')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(saveChannelMonitorConfig).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
    wrapper.unmount()
  })

  it('retains draft changes after save errors', async () => {
    vi.mocked(saveChannelMonitorConfig).mockRejectedValueOnce(new Error('offline'))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-group-id="2"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('配置保存失败')
    expect((wrapper.get('[data-group-id="2"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.emitted('saved')).toBeUndefined()
    wrapper.unmount()
  })
})
