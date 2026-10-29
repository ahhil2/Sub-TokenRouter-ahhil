import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ChannelMonitorCard from '../ChannelMonitorCard.vue'
import channelMonitor from '@/i18n/locales/zh/channelMonitor'
import type { MonitoredChannel } from '@/api/channelMonitor'

/**
 * ChannelMonitorCard.spec.ts verifies channel statistics, history slots, and their
 * accessible hover and focus details. Fixtures distinguish missing traffic from zero
 * availability and confirm that manual status labels leave measured metrics and
 * history unchanged.
 */

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

const metrics = {
  requests: 10,
  successes: 10,
  availability: 100,
  cache_rate: 85.2,
  ttft_p50_ms: 7660,
}
const channel: MonitoredChannel = {
  id: 1,
  name: 'ChatGPT-Plus【特惠】',
  brand: 'OpenAI',
  rate_multiplier: 0.05,
  ...metrics,
  slots: Array.from({ length: 20 }, (_, index) => ({
    ...metrics,
    start: new Date(Date.UTC(2026, 9, 13, 0, 0, index * 270)).toISOString(),
    end: new Date(Date.UTC(2026, 9, 13, 0, 0, (index + 1) * 270)).toISOString(),
  })),
}

function render(input = channel) {
  return mount(ChannelMonitorCard, {
    props: { channel: input, countdown: 28, windowMinutes: 90, slotSeconds: 270 },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { channelMonitor } } })],
      stubs: { ProviderIcon: true },
    },
  })
}

describe('ChannelMonitorCard', () => {
  it('labels manual status without changing statistics or history blocks', () => {
    const wrapper = render({ ...channel, status_override: 'maintenance' })
    expect(wrapper.get('.monitor-status').text()).toContain('维护中')
    expect(wrapper.get('.monitor-status').text()).toContain('手动')
    expect(wrapper.text()).toContain('100.0%')
    expect(wrapper.findAll('.monitor-block[data-status="operational"]')).toHaveLength(20)
  })
  it('shows real metrics and twenty blocks without intelligence testing', () => {
    const wrapper = render()
    expect(wrapper.findAll('.monitor-block')).toHaveLength(20)
    expect(wrapper.text()).toContain('85.2%')
    expect(wrapper.text()).toContain('7.66s')
    expect(wrapper.text()).toContain('28s 后刷新')
    expect(wrapper.text()).not.toMatch(/智商|降智检测/)
  })

  it('shows availability and cache rate on hover and keyboard focus', async () => {
    const wrapper = render()
    const block = wrapper.findAll('.monitor-block')[19]
    await block.trigger('mouseenter')
    expect(wrapper.get('[role="tooltip"]').text()).toContain('100.0%')
    expect(wrapper.get('[role="tooltip"]').text()).toContain('85.2%')
    await wrapper.get('.monitor-history').trigger('mouseleave')
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    await block.trigger('focus')
    expect(block.attributes('aria-describedby')).toBe('channel-1-slot-tooltip')
    await block.trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
  })

  it('distinguishes no traffic from zero availability', () => {
    const empty = {
      requests: 0,
      successes: 0,
      availability: null,
      cache_rate: null,
      ttft_p50_ms: null,
    }
    const wrapper = render({
      ...channel,
      ...empty,
      slots: channel.slots.map((slot) => ({ ...slot, ...empty })),
    })
    expect(wrapper.get('.monitor-status').text()).toBe('暂无数据')
    expect(wrapper.text()).not.toContain('100.0%')
    expect(wrapper.findAll('[data-status="unknown"]')).toHaveLength(21)
  })
})
