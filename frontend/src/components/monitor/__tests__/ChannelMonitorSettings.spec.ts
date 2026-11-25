import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChannelMonitorSettings from '../ChannelMonitorSettings.vue'
import { getChannelMonitorConfig, saveChannelMonitorConfig } from '@/api/channelMonitor'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'

/**
 * ChannelMonitorSettings.spec.ts exercises loading and saving public monitoring
 * configuration through mocked endpoints. The cases cover cleared selections, failed
 * initial loads, retries, and retained draft changes after save errors. Ordering
 * cases exercise selected neighbors, endpoint controls, and the submitted sequence.
 */

vi.mock('@/api/channelMonitor', () => ({
  getChannelMonitorConfig: vi.fn(),
  saveChannelMonitorConfig: vi.fn(),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, values: Record<string, unknown> = {}) => {
      const message = key
        .replace(/^channelMonitor\./, '')
        .split('.')
        .reduce<unknown>((value, part) => (value as Record<string, unknown>)[part], channelMonitor)
      return String(message).replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ''))
    },
  }),
}))

// render mounts the editor with status and icon components stubbed.
function render() {
  return mount(ChannelMonitorSettings, { global: { stubs: { Select: true, Icon: true } } })
}

describe('ChannelMonitorSettings', () => {
  beforeEach(() => {
    vi.mocked(getChannelMonitorConfig)
      .mockReset()
      .mockResolvedValue([
        {
          group_id: 1,
          name: 'GPT',
          brand: 'OpenAI',
          enabled: true,
          status: 'auto',
          eligible: true,
        },
        {
          group_id: 2,
          name: 'Claude',
          brand: 'Anthropic',
          enabled: false,
          status: 'auto',
          eligible: false,
        },
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
    expect(saveChannelMonitorConfig).toHaveBeenCalledWith([
      { group_id: 2, enabled: true, status: 'maintenance' },
    ])
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

  describe('selected group ordering', () => {
    beforeEach(() => {
      vi.mocked(getChannelMonitorConfig).mockResolvedValue([
        {
          group_id: 1,
          name: 'GPT',
          brand: 'OpenAI',
          enabled: true,
          status: 'auto',
          eligible: true,
        },
        {
          group_id: 2,
          name: 'Claude',
          brand: 'Anthropic',
          enabled: false,
          status: 'auto',
          eligible: false,
        },
        {
          group_id: 3,
          name: 'Gemini',
          brand: 'Google',
          enabled: true,
          status: 'maintenance',
          eligible: true,
        },
      ])
    })

    // Selected groups surround an unchecked row and carry different status choices.
    // Moving in either direction changes their submitted order while retaining each
    // group's status, and clears a success message from an earlier save.
    it('moves selected neighbors in both directions and saves their order and statuses', async () => {
      const wrapper = render()
      await flushPromises()
      await wrapper.get('button[aria-label="下移 GPT"]').trigger('click')
      expect(
        wrapper.findAll('input[type="checkbox"]').map((input) => input.attributes('data-group-id')),
      ).toEqual(['3', '2', '1'])
      expect(saveChannelMonitorConfig).not.toHaveBeenCalled()
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(saveChannelMonitorConfig).toHaveBeenLastCalledWith([
        { group_id: 3, enabled: true, status: 'maintenance' },
        { group_id: 1, enabled: true, status: 'auto' },
      ])
      expect(wrapper.text()).toContain('配置已保存')
      await wrapper.get('button[aria-label="上移 GPT"]').trigger('click')
      expect(wrapper.text()).not.toContain('配置已保存')
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(saveChannelMonitorConfig).toHaveBeenLastCalledWith([
        { group_id: 1, enabled: true, status: 'auto' },
        { group_id: 3, enabled: true, status: 'maintenance' },
      ])
      wrapper.unmount()
    })

    // Endpoint and unchecked rows cannot move outside the selected sequence.
    // Changing selection updates those controls, including the single-selected-group
    // case where neither direction has an available neighbor.
    it('disables unavailable moves as selection changes', async () => {
      const wrapper = render()
      await flushPromises()
      expect(wrapper.get('button[aria-label="上移 GPT"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('button[aria-label="下移 Gemini"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('button[aria-label="上移 Claude"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('button[aria-label="下移 Claude"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-group-id="3"]').setValue(false)
      expect(wrapper.get('button[aria-label="下移 GPT"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-group-id="2"]').setValue(true)
      expect(wrapper.get('button[aria-label="上移 Claude"]').attributes('disabled')).toBeUndefined()
      await wrapper.get('button[aria-label="上移 Claude"]').trigger('click')
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(saveChannelMonitorConfig).toHaveBeenLastCalledWith([
        { group_id: 2, enabled: true, status: 'auto' },
        { group_id: 1, enabled: true, status: 'auto' },
      ])
      wrapper.unmount()
    })

    // A pending save rejects while an administrator has reordered the draft.
    // Moves remain disabled during persistence, and retrying submits the same
    // selected order and statuses after the failure.
    it('locks ordering during saving and retains the order after a failed save', async () => {
      let rejectSave: (error: Error) => void = () => {}
      vi.mocked(saveChannelMonitorConfig).mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectSave = reject
          }),
      )
      const wrapper = render()
      await flushPromises()
      await wrapper.get('button[aria-label="上移 Gemini"]').trigger('click')
      await wrapper.get('form').trigger('submit')
      expect(wrapper.get('button[aria-label="下移 Gemini"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('button[aria-label="上移 GPT"]').attributes('disabled')).toBeDefined()
      rejectSave(new Error('offline'))
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('配置保存失败')
      expect(wrapper.get('button[aria-label="下移 Gemini"]').attributes('disabled')).toBeUndefined()
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(saveChannelMonitorConfig).toHaveBeenNthCalledWith(2, [
        { group_id: 3, enabled: true, status: 'maintenance' },
        { group_id: 1, enabled: true, status: 'auto' },
      ])
      expect(wrapper.emitted('saved')).toHaveLength(1)
      wrapper.unmount()
    })
  })
})
