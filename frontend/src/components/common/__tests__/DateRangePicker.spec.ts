import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

/**
 * DateRangePicker.spec.ts checks date shortcuts and optional minute-level range
 * editing. Controlled date inputs cover ordering, cancellation, rolling-day
 * identification, and emitted timestamp values so usage screens can distinguish
 * calendar ranges from elapsed-time windows.
 */

vi.mock('../DateTimePicker.vue', () => ({
  default: {
    props: ['modelValue', 'id'],
    emits: ['update:modelValue'],
    template:
      '<input type="datetime-local" :id="id" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
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
  'common.cancel': 'Cancel',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en'),
  }),
}))

// formatLocalDate formats fixture dates using browser-local year, month, and day.
const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

// waitForTransition waits for the test panel leave delay to elapse. It uses a fixed
// timer so assertions can inspect the removed panel after its CSS transition window.
const waitForTransition = () => new Promise((resolve) => window.setTimeout(resolve, 250))

describe('DateRangePicker', () => {
  // A picker inside a page stacking context renders its calendar outside that page.
  // Calendar clicks retain the draft, and an outside click closes the reopened panel.
  it('keeps body-mounted calendar interactions inside the picker ownership', async () => {
    const host = document.createElement('div')
    host.style.position = 'relative'
    /* check-ui-allow: This local fixture reproduces AppLayout's content stacking
       context below the fixed header so portal ownership can be checked outside it. */
    host.style.zIndex = '10'
    document.body.append(host)
    const wrapper = mount(DateRangePicker, {
      attachTo: host,
      props: { startDate: '2026-10-20', endDate: '2026-10-20' },
      global: { stubs: { Icon: true } },
    })
    try {
      await wrapper.get('button[aria-haspopup="dialog"]').trigger('click')
      await nextTick()
      const panel = document.body.querySelector<HTMLElement>('.date-picker-dropdown')
      expect(panel).not.toBeNull()
      expect(host.contains(panel)).toBe(false)
      const yesterday = Array.from(panel!.querySelectorAll('button')).find(
        (button) => button.textContent?.trim() === 'Yesterday',
      )!
      yesterday.click()
      await nextTick()
      expect(document.body.querySelector('[role="dialog"]')).not.toBeNull()
      expect(wrapper.emitted('change')).toBeUndefined()
      const apply = Array.from(panel!.querySelectorAll('button')).find(
        (button) => button.textContent?.trim() === 'Apply',
      )!
      apply.click()
      await nextTick()
      expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({ preset: 'yesterday' })
      await waitForTransition()
      await wrapper.get('button[aria-haspopup="dialog"]').trigger('click')
      document.body.click()
      await nextTick()
      await waitForTransition()
      expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    } finally {
      // Unmount before removing the host so document listeners and teleported nodes
      // are released even when an assertion interrupts the interactive fixture.
      wrapper.unmount()
      host.remove()
    }
  })

  // Visible shortcuts are enabled with minute editing at a fixed local time. Each
  // click publishes its complete range immediately and leaves the calendar closed.
  it('applies yesterday and rolling 24 hours directly from the visible shortcuts', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    try {
      const now = new Date(2026, 9, 20, 10, 35)
      vi.setSystemTime(now)
      const wrapper = mount(DateRangePicker, {
        props: {
          startDate: '2026-10-20',
          endDate: '2026-10-20',
          enableTime: true,
          quickPresets: ['yesterday', 'last24Hours'],
        },
        global: { stubs: { Icon: true, Teleport: true } },
      })
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      await wrapper.get('[data-quick-preset="yesterday"]').trigger('click')
      expect(wrapper.emitted('change')?.[0]).toEqual([
        { startDate: '2026-10-19', endDate: '2026-10-19', preset: 'yesterday' },
      ])
      await wrapper.get('[data-quick-preset="last24Hours"]').trigger('click')
      expect(wrapper.emitted('change')?.[1]).toEqual([
        {
          startDate: new Date(now.getTime() - 86400000).toISOString(),
          endDate: now.toISOString(),
          preset: 'last24Hours',
        },
      ])
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  // A day-only range opens in minute-editing mode and receives two local times.
  // Application emits ISO timestamps and displays both selected times on the trigger.
  it('applies an 80-minute local range as timestamps and displays both times', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
      global: { stubs: { Icon: true, Teleport: true } },
    })
    await wrapper.get('.input-trigger').trigger('click')
    expect((wrapper.get('[data-test="range-end-time"]').element as HTMLInputElement).value).toBe(
      '2026-10-20T00:00',
    )
    await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
    await wrapper.get('[data-test="range-end-time"]').setValue('2026-10-19T21:40')
    expect(wrapper.emitted('change')).toBeUndefined()
    await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: new Date('2026-10-19T20:20').toISOString(),
        endDate: new Date('2026-10-19T21:40').toISOString(),
        preset: null,
      },
    ])
    expect(wrapper.get('.input-trigger').text()).toContain('20:20')
    expect(wrapper.get('.input-trigger').text()).toContain('21:40')
    wrapper.unmount()
  })

  // An end time equals the start, precedes it, or is empty. The picker reports
  // invalid input, disables application, and publishes no range change.
  it.each(['2026-10-19T20:20', '2026-10-19T19:00', ''])(
    'rejects an invalid end time %s',
    async (end) => {
      const wrapper = mount(DateRangePicker, {
        props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
        global: { stubs: { Icon: true, Teleport: true } },
      })
      await wrapper.get('.input-trigger').trigger('click')
      await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
      await wrapper.get('[data-test="range-end-time"]').setValue(end)
      expect(wrapper.get('.date-picker-actions .btn-primary').attributes('disabled')).toBeDefined()
      expect(wrapper.find('[role="alert"]').exists()).toBe(true)
      await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
      expect(wrapper.emitted('change')).toBeUndefined()
      wrapper.unmount()
    },
  )

  // A minute edit is cancelled before the picker opens again. Cancellation keeps
  // the committed label, and applying the restored draft emits date-only endpoints.
  it('discards minute edits on cancel and keeps day-only applications unchanged', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19', enableTime: true },
      global: { stubs: { Icon: true, Teleport: true } },
    })
    const original = wrapper.get('.input-trigger').text()
    await wrapper.get('.input-trigger').trigger('click')
    await wrapper.get('[data-test="range-start-time"]').setValue('2026-10-19T20:20')
    await wrapper.get('.date-picker-actions .btn-secondary').trigger('click')
    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.get('.input-trigger').text()).toBe(original)
    await wrapper.get('.input-trigger').trigger('click')
    await wrapper.get('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({
      startDate: '2026-10-19',
      endDate: '2026-10-19',
    })
    wrapper.unmount()
  })

  // A caller supplies only the required date endpoints. Opening the default picker
  // exposes neither minute inputs nor the optional buttons beside the trigger.
  it('keeps time controls and visible shortcuts opt-in', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-10-19', endDate: '2026-10-19' },
      global: { stubs: { Icon: true, Teleport: true } },
    })
    await wrapper.get('.input-trigger').trigger('click')
    expect(wrapper.find('input[type="datetime-local"]').exists()).toBe(false)
    expect(wrapper.find('[data-quick-preset]').exists()).toBe(false)
    wrapper.unmount()
  })

  // Two date-only endpoints straddle midnight when the current day has just begun.
  // Preset recognition must keep this calendar range distinct from a rolling day.
  it('does not identify two calendar dates as a rolling day near midnight', () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    try {
      vi.setSystemTime(new Date(2026, 9, 20, 0, 0, 30))
      const wrapper = mount(DateRangePicker, {
        props: { startDate: '2026-10-19', endDate: '2026-10-20' },
        global: { stubs: { Icon: true, Teleport: true } },
      })
      expect(wrapper.get('.input-trigger').text()).not.toContain('Last 24 Hours')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })
  // Both supplied endpoints name the current local calendar date. The trigger
  // identifies Today and does not assign the rolling twenty-four-hour preset.
  it('recognizes a same-day range as Today', () => {
    const today = formatLocalDate(new Date())
    const wrapper = mount(DateRangePicker, {
      props: { startDate: today, endDate: today },
      global: { stubs: { Icon: true, Teleport: true } },
    })

    expect(wrapper.get('.input-trigger').text()).toContain('Today')
    expect(wrapper.get('.input-trigger').text()).not.toContain('Last 24 Hours')
    wrapper.unmount()
  })

  // Supplied timestamps span exactly twenty-four hours ending at the current time.
  // Initial rendering identifies the elapsed-time preset before the user opens the panel.
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: yesterday.toISOString(),
        endDate: now.toISOString(),
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  // The user selects the last-24-hours shortcut and then applies the draft. The
  // component must publish both date endpoints and an aggregate change carrying the
  // matching preset identifier.
  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today,
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true,
        },
      },
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper
      .findAll('.date-picker-preset')
      .find((node) => node.text().includes('Last 24 Hours'))
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
        preset: 'last24Hours',
      },
    ])
  })

  // The picker enables immediate application and the user selects the previous-month
  // shortcut. The change event must identify that preset and the panel must disappear
  // after its leave transition.
  it('can apply a preset immediately when applyOnPreset is enabled', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today,
        applyOnPreset: true,
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true,
        },
      },
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper
      .findAll('.date-picker-preset')
      .find((node) => node.text().includes('Last Month'))
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')

    expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({
      preset: 'lastMonth',
    })
    await waitForTransition()
    expect(wrapper.find('.date-picker-dropdown').exists()).toBe(false)
  })

  // The user selects day twenty followed by day five within the same displayed month.
  // Application remains disabled after the first click, then emits ordered endpoints
  // with a custom preset marker.
  it('selects a custom range from the calendar and swaps reversed clicks', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12',
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true,
        },
      },
    })

    await wrapper.find('.input-trigger').trigger('click')
    // dayButton finds the rendered calendar button for the requested day.
    const dayButton = (day: number) =>
      wrapper.findAll('.date-picker-day').find((node) => node.text() === String(day))!

    await dayButton(20).trigger('click')
    expect(wrapper.find('.date-picker-actions .btn-primary').attributes('disabled')).toBeDefined()
    await dayButton(5).trigger('click')
    expect(wrapper.findAll('.date-picker-day-in-range')).toHaveLength(16)

    await wrapper.find('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: '2026-01-05',
        endDate: '2026-01-20',
        preset: null,
      },
    ])
  })

  // The user selects a different preset while the picker holds an opening snapshot,
  // then cancels. The trigger keeps its applied label and cancellation publishes no
  // aggregate range change.
  it('discards unapplied changes when cancelled', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12',
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true,
        },
      },
    })

    const triggerText = wrapper.find('.input-trigger').text()
    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper
      .findAll('.date-picker-preset')
      .find((node) => node.text().includes('Last 7 Days'))
    await presetButton!.trigger('click')

    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
    expect(wrapper.find('.date-picker-preset-active').text()).toContain('Last 7 Days')

    const cancelButton = wrapper
      .findAll('.date-picker-actions .btn')
      .find((node) => node.text().includes('Cancel'))
    await cancelButton!.trigger('click')

    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
  })
})
