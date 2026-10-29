import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import PelicanShowcaseView from '../PelicanShowcaseView.vue'
import PelicanTestsView from '../admin/PelicanTestsView.vue'

/**
 * PelicanShowcaseView.spec.ts checks published drawing presentation and the active-
 * preview budget. Server fixtures supply the displayed prompt and results, including
 * failure cases, while shared administrator rendering is checked against the same
 * server-provided prompt.
 */

enableAutoUnmount(afterEach)

const { results, catalog, result, tasks } = vi.hoisted(() => ({
  results: vi.fn(),
  catalog: vi.fn(),
  result: vi.fn(),
  tasks: vi.fn(),
}))

vi.mock('@/api/pelican', () => ({ pelicanApi: { results, catalog, result, tasks } }))
vi.mock('@/api/admin/groups', () => ({
  default: {},
  getAll: vi.fn().mockResolvedValue([]),
  getModelsListCandidates: vi.fn().mockResolvedValue([]),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))
vi.mock('@/components/layout/AppHeader.vue', () => ({ default: { template: '<header />' } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const items = Array.from({ length: 8 }, (_, index) => ({
  id: index + 1,
  group_id: 1,
  group_name: '演示分组',
  model: `model-${index + 1}`,
  status: 'success',
  published: true,
  started_at: '2026-10-19T00:00:00Z',
  latency_ms: 1000,
}))

const global = {
  stubs: {
    AppLayout: { template: '<div><slot /></div>' },
    AppHeader: true,
    Select: true,
    RouterLink: true,
    ConfirmDialog: true,
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
    PelicanCard: {
      name: 'PelicanCard',
      props: ['item', 'active'],
      emits: ['visible', 'open'],
      template: '<article />',
    },
    PelicanPreview: true,
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  results.mockResolvedValue({
    items,
    has_more: false,
    prompt: '来自服务端的提示词\n每次独立选择场景。',
  })
  catalog.mockResolvedValue([])
  tasks.mockResolvedValue([])
  result.mockImplementation(async (id: number) => ({
    ...items[id - 1],
    html: '<html><svg></svg></html>',
  }))
})

describe('pelican gallery', () => {
  it('shows the server prompt in a collapsed section and uses a four-column desktop grid', async () => {
    const wrapper = mount(PelicanShowcaseView, { global })
    await flushPromises()
    expect(wrapper.get('details').text()).toContain('来自服务端的提示词')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.get('details p').classes()).toContain('whitespace-pre-wrap')
    const grid = wrapper.get('section > div')
    expect(grid.classes()).toEqual(
      expect.arrayContaining(['grid-cols-1', 'sm:grid-cols-2', 'xl:grid-cols-4']),
    )
    expect(grid.findAll('article')).toHaveLength(8)
  })

  it('keeps at most six active animations including an expanded preview', async () => {
    const wrapper = mount(PelicanShowcaseView, { global })
    await flushPromises()
    const cards = wrapper.findAllComponents({ name: 'PelicanCard' })
    for (const card of cards) card.vm.$emit('visible', card.props('item').id, true)
    await flushPromises()
    expect(cards.filter((card) => card.props('active'))).toHaveLength(6)
    cards[0].vm.$emit('open', items[0])
    await flushPromises()
    expect(cards.filter((card) => card.props('active'))).toHaveLength(5)
    expect(wrapper.findComponent({ name: 'PelicanPreview' }).exists()).toBe(true)
    cards[0].vm.$emit('visible', 1, false)
    await flushPromises()
    expect(cards[0].props('active')).toBe(false)
  })

  it('does not display a stale hardcoded prompt when results fail', async () => {
    results.mockRejectedValue(new Error('offline'))
    const wrapper = mount(PelicanShowcaseView, { global })
    await flushPromises()
    expect(wrapper.find('details').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toBe('pelican.loadFailed')
  })

  it('shows the same server prompt in administration', async () => {
    const wrapper = mount(PelicanTestsView, { global })
    await flushPromises()
    expect(results).toHaveBeenCalledWith({ page: 1, task_id: 0 }, true)
    expect(wrapper.get('details').text()).toContain('来自服务端的提示词')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
  })
})
