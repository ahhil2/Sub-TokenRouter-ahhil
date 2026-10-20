import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

vi.mock('../DateTimePicker.vue', () => ({
  default: {
    props: ['modelValue', 'id'],
    emits: ['update:modelValue'],
    template: '<input type="datetime-local" :id="id" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last15Minutes': 'Last 15 Minutes',
  'dates.last30Minutes': 'Last 30 Minutes',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range',
  'dates.selectEndDate': 'Select an end date',
  'dates.previousMonth': 'Previous month',
  'dates.nextMonth': 'Next month',
  'common.cancel': 'Cancel'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const waitForTransition = () => new Promise((resolve) => window.setTimeout(resolve, 250))

describe('DateRangePicker', () => {
  it('applies yesterday and rolling 24 hours directly from the visible shortcuts', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    try {
      const now = new Date(2026, 9, 20, 10, 35)
      vi.setSystemTime(now)
      const wrapper = mount(DateRangePicker, {
        props: { startDate: '2026-10-20', endDate: '2026-10-20', enableTime: true, quickPresets: ['yesterday', 'last24Hours'] },
        global: { stubs: { Icon: true } },
      })
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      await wrapper.get('[data-quick-preset="yesterday"]').trigger('click')
      expect(wrapper.emitted('change')?.[0]).toEqual([{ startDate: '2026-10-19', endDate: '2026-10-19', preset: 'yesterday' }])
      await wrapper.get('[data-quick-preset="last24Hours"]').trigger('click')
      expect(wrapper.emitted('change')?.[1]).toEqual([{
        startDate: new Date(now.getTime() - 86400000).toISOString(),
        endDate: now.toISOString(), preset: 'last24Hours',
      }])
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('applies an 80-minute local range as timestamps and displays both times', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('.input-trigger').trigger('click')
    expect((wrapper.get('[data-test="range-end-time"]').element as HTMLInputElement).value).toBe('2026-10-20T00:00')
    await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
    await wrapper.get('[data-test="range-end-time"]').setValue('2026-10-19T21:40')
    expect(wrapper.emitted('change')).toBeUndefined()
    await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([{
      startDate: new Date('2026-10-19T20:20').toISOString(),
      endDate: new Date('2026-10-19T21:40').toISOString(), preset: null,
    }])
    expect(wrapper.get('.input-trigger').text()).toContain('20:20')
    expect(wrapper.get('.input-trigger').text()).toContain('21:40')
    wrapper.unmount()
  })

  it.each(['2026-10-19T20:20', '2026-10-19T19:00', ''])('rejects an invalid end time %s', async (end) => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('.input-trigger').trigger('click')
    await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
    await wrapper.get('[data-test="range-end-time"]').setValue(end)
    expect(wrapper.get('.date-picker-actions .btn-primary').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')).toBeUndefined()
    wrapper.unmount()
  })

  it('discards minute edits on cancel and keeps day-only applications unchanged', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
      global: { stubs: { Icon: true } },
    })
    const original = wrapper.get('.input-trigger').text()
    await wrapper.get('.input-trigger').trigger('click')
    await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
    await wrapper.get('.date-picker-actions .btn-secondary').trigger('click')
    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.get('.input-trigger').text()).toBe(original)
    await wrapper.get('.input-trigger').trigger('click')
    await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({ startDate: '2026-10-19', endDate: '2026-10-19' })
    wrapper.unmount()
  })

  it('keeps time controls and visible shortcuts opt-in', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19' },
      global: { stubs: { Icon: true } },
    })
    await wrapper.get('.input-trigger').trigger('click')
    expect(wrapper.find('input[type="datetime-local"]').exists()).toBe(false)
    expect(wrapper.find('[data-quick-preset]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not identify two calendar dates as a rolling day near midnight', () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    try {
      vi.setSystemTime(new Date(2026, 9, 20, 0, 0, 30))
      const wrapper = mount(DateRangePicker, {
        props: { startDate: '2026-10-19', endDate: '2026-10-20' },
        global: { stubs: { Icon: true } },
      })
      expect(wrapper.get('.input-trigger').text()).not.toContain('Last 24 Hours')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })
  it('recognizes a same-day range as Today', () => {
    const today = formatLocalDate(new Date())
    const wrapper = mount(DateRangePicker, {
      props: { startDate: today, endDate: today },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.get('.input-trigger').text()).toContain('Today')
    expect(wrapper.get('.input-trigger').text()).not.toContain('Last 24 Hours')
    wrapper.unmount()
  })

  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: yesterday.toISOString(),
        endDate: now.toISOString()
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-actions .btn-primary').trigger('click')

    const expectedStart = wrapper.emitted('update:startDate')![0]![0] as string
    const expectedEnd = wrapper.emitted('update:endDate')![0]![0] as string
    expect(expectedStart).toMatch(/T.*Z$/)
    expect(new Date(expectedEnd).getTime() - new Date(expectedStart).getTime()).toBe(86400000)
    expect(Math.abs(Date.now() - new Date(expectedEnd).getTime())).toBeLessThan(2000)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })

  it('can apply a preset immediately when applyOnPreset is enabled', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today,
        applyOnPreset: true
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last Month')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')

    expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({
      preset: 'lastMonth'
    })
    await waitForTransition()
    expect(wrapper.find('.date-picker-dropdown').exists()).toBe(false)
  })

  it('selects a custom range from the calendar and swaps reversed clicks', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const dayButton = (day: number) =>
      wrapper.findAll('.date-picker-day').find((node) => node.text() === String(day))!

    // 先点 20 日再点 5 日，结束日期早于开始日期时自动对调。
    await dayButton(20).trigger('click')
    expect(wrapper.find('.date-picker-actions .btn-primary').attributes('disabled')).toBeDefined()
    await dayButton(5).trigger('click')
    expect(wrapper.findAll('.date-picker-day-in-range')).toHaveLength(16)

    await wrapper.find('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: '2026-01-05',
        endDate: '2026-01-20',
        preset: null
      }
    ])
  })

  it('discards unapplied changes when cancelled', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    const triggerText = wrapper.find('.input-trigger').text()
    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 7 Days')
    )
    await presetButton!.trigger('click')
    // 未应用前触发器仍显示原范围。
    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
    expect(wrapper.find('.date-picker-preset-active').text()).toContain('Last 7 Days')

    const cancelButton = wrapper.findAll('.date-picker-actions .btn').find((node) =>
      node.text().includes('Cancel')
    )
    await cancelButton!.trigger('click')

    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
  })
})
