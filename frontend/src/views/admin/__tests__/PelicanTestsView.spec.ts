import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { PelicanTask } from '@/api/pelican'
import PelicanTestsView from '../PelicanTestsView.vue'

// PelicanTestsView.spec.ts checks administrator editing of generation time limits.
// Controlled API fixtures exercise the mounted task form, its native numeric
// input, and submitted settings. Existing budgets must load into the editor,
// while new tasks begin with the default limit.

enableAutoUnmount(afterEach)

// API doubles provide task data and capture saved drafts while preventing
// component polling and model selection from contacting an application server.
const { tasks, results, save } = vi.hoisted(() => ({
  tasks: vi.fn(),
  results: vi.fn(),
  save: vi.fn(),
}))

vi.mock('@/api/pelican', () => ({ pelicanApi: { tasks, results, save } }))
vi.mock('@/api/admin/groups', () => ({
  getOptions: vi.fn().mockResolvedValue([{ id: 1, name: 'Drawing group' }]),
  getModelsListCandidates: vi.fn().mockResolvedValue(['drawing-model']),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, values?: { seconds?: number }) =>
      values?.seconds === undefined ? key : `${key} ${values.seconds}`,
  }),
}))

// The saved task uses a limit above five minutes to expose any accidental
// replacement with the default when opening or saving an existing task.
const task: PelicanTask = {
  id: 7,
  group_id: 1,
  group_name: 'Drawing group',
  model: 'drawing-model',
  enabled: false,
  interval_minutes: 360,
  timeout_seconds: 900,
  public_consent: true,
  manual_requested: false,
  next_run_at: '2026-11-17T00:00:00Z',
  lease_until: null,
  last_status: '',
  last_run_at: null,
}

// Dialog and layout stubs retain real form controls while removing unrelated
// overlays, preview rendering, and navigation from the editor assertions below.
const global = {
  stubs: {
    RouterLink: true,
    ConfirmDialog: true,
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
    Select: true,
    Toggle: true,
    PelicanPreview: true,
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  tasks.mockResolvedValue([{ ...task }])
  results.mockResolvedValue({ items: [], has_more: false, prompt: '' })
  save.mockResolvedValue({ ...task })
})

describe('Pelican generation time limits', () => {
  // An existing task has a custom budget that must be visible before editing.
  // Saving the form submits the changed value with the selected task identifier.
  it('loads, edits, and saves a custom generation limit', async () => {
    const wrapper = mount(PelicanTestsView, { global })
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('pelican.timeoutSummary 900')
    await wrapper.get('tbody button').trigger('click')
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('input[name="timeout_seconds"]')
    expect(input.element.value).toBe('900')
    await input.setValue('1200')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({ timeout_seconds: 1200, group_id: 1, model: 'drawing-model' }),
      7,
    )
    expect(wrapper.find('form').exists()).toBe(false)
  })

  // A new task starts with a five-minute generation budget and integer bounds.
  // Native validity rejects empty, fractional, negative, and excessive values.
  it('defaults new tasks to 300 seconds and bounds the numeric input', async () => {
    tasks.mockResolvedValue([])
    const wrapper = mount(PelicanTestsView, { global })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('input[name="timeout_seconds"]')
    expect(input.element.value).toBe('300')
    for (const value of ['', '0', '-1', '1.5', '3601']) {
      await input.setValue(value)
      expect(input.element.checkValidity()).toBe(false)
    }
    for (const value of ['1', '600', '3600']) {
      await input.setValue(value)
      expect(input.element.checkValidity()).toBe(true)
    }
  })
})
