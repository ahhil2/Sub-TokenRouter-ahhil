import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

import UsageFilters from '../UsageFilters.vue'

// 仅保留 UsageFilters 当前测试需要的 i18n 文案。
const messages: Record<string, string> = {
  'admin.usage.userDeletedBadge': 'deleted',
  'admin.usage.userFilter': 'User',
  'admin.usage.teamFilter': 'Team',
  'admin.usage.allTeams': 'All Teams',
  'admin.usage.searchUserPlaceholder': 'Search user...',
  'usage.apiKeyFilter': 'API Key',
  'admin.usage.searchApiKeyPlaceholder': 'Search API key...',
  'usage.model': 'Model',
  'admin.usage.allModels': 'All Models',
  'admin.usage.provider': 'Provider',
  'admin.usage.searchProviderPlaceholder': 'Search provider...',
  'usage.type': 'Type',
  'admin.usage.allTypes': 'All Types',
  'usage.ws': 'WS',
  'usage.stream': 'Stream',
  'usage.sync': 'Sync',
  'usage.cyber': 'Cyber',
  'admin.usage.billingType': 'Billing Type',
  'admin.usage.allBillingTypes': 'All Billing Types',
  'admin.usage.billingTypeBalance': 'Balance',
  'admin.usage.billingTypeSubscription': 'Subscription',
  'admin.usage.billingMode': 'Billing Mode',
  'admin.usage.allBillingModes': 'All Billing Modes',
  'admin.usage.billingModeToken': 'Token',
  'admin.usage.billingModePerRequest': 'Per Request',
  'admin.usage.billingModeImage': 'Image',
  'admin.usage.group': 'Group',
  'admin.usage.allGroups': 'All Groups',
  'common.refresh': 'Refresh',
  'common.reset': 'Reset',
  'admin.usage.cleanup.button': 'Cleanup',
  'usage.exportExcel': 'Export',
}

// 固定 i18n 输出，便于直接断言下拉文案。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

// 管理端 API mock 由各用例按需控制返回值。
const mockSearchUsers = vi.fn()
const mockSearchApiKeys = vi.fn().mockResolvedValue([])
const mockGroupsList = vi.fn().mockResolvedValue({ items: [] })
const mockTeamsList = vi.fn().mockResolvedValue([])
const mockGetModelStats = vi.fn().mockResolvedValue({ models: [] })
const mockProvidersList = vi.fn().mockResolvedValue({ items: [] })

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      searchUsers: (...args: any[]) => mockSearchUsers(...args),
      searchApiKeys: (...args: any[]) => mockSearchApiKeys(...args),
    },
    groups: { list: (...args: any[]) => mockGroupsList(...args) },
    teams: { list: (...args: any[]) => mockTeamsList(...args) },
    dashboard: { getModelStats: (...args: any[]) => mockGetModelStats(...args) },
    providers: { list: (...args: any[]) => mockProvidersList(...args) },
  },
}))

// 每个用例使用独立的默认筛选参数。
const defaultFilters = () => ({
  user_id: undefined,
  api_key_id: undefined,
  provider_id: undefined,
  model: null,
  request_type: null,
  billing_type: null,
  billing_mode: null,
  group_id: null,
  team_id: null,
  start_date: '',
  end_date: '',
})

function mountFilters(filters = defaultFilters()) {
  return mount(UsageFilters, {
    props: {
      modelValue: filters,
      exporting: false,
      startDate: '2026-05-01',
      endDate: '2026-05-28',
      showActions: false,
      modelOptions: [],
    },
    global: {
      stubs: {
        Icon: true,
        Select: true,
        Teleport: true,
      },
    },
  })
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })

  return { promise, resolve, reject }
}

describe('UsageFilters quick filters', () => {
  it.each(['usage', 'errors', 'ranking'] as const)('resets all filters from the toolbar while preserving dates in %s mode', async mode => {
    const filters = {
      ...defaultFilters(), model: 'claude-test', user_id: 7, provider_id: 8,
      api_key_id: 9, group_id: 3, team_id: 4, request_id: 'request-test',
      error_phase: 'upstream', error_category: 'rate_limit', status_code: 429,
    }
    const wrapper = mountFilters(filters as unknown as ReturnType<typeof defaultFilters>)
    await wrapper.setProps({ showActions: true, mode })
    await flushPromises()

    const reset = wrapper.get('[data-testid="usage-toolbar-reset"]')
    expect(reset.isVisible()).toBe(true)
    expect(reset.text()).toBe('Reset')
    expect(reset.element.closest('[role="dialog"]')).toBeNull()
    await reset.trigger('click')

    for (const [key, value] of Object.entries(filters)) {
      if (key !== 'start_date' && key !== 'end_date') expect(value).toBeUndefined()
    }
    expect(filters.start_date).toBe('2026-05-01')
    expect(filters.end_date).toBe('2026-05-28')
    expect(wrapper.emitted('change')).toHaveLength(1)
    expect(wrapper.emitted('cleanup')).toBeUndefined()
    wrapper.unmount()
  }, 60_000)

  it('clears unselected search text when resetting from the toolbar', async () => {
    const wrapper = mountFilters()
    await wrapper.setProps({ showActions: true })
    await flushPromises()
    const inputs = wrapper.findAll('input').filter(input =>
      ['Search user...', 'Search provider...', 'Search API key...'].includes(input.attributes('placeholder'))
    )
    expect(inputs).toHaveLength(3)
    for (const input of inputs) await input.setValue('unfinished search')
    await wrapper.get('[data-testid="usage-toolbar-reset"]').trigger('click')
    for (const input of inputs) expect(input.element.value).toBe('')
    expect(wrapper.emitted('change')).toHaveLength(1)
    wrapper.unmount()
  }, 60_000)

  it.each(['usage', 'errors', 'ranking'] as const)('keeps four ordered filters outside the panel in %s mode', async mode => {
    const wrapper = mountFilters()
    await wrapper.setProps({ mode })
    await flushPromises()

    const fields = wrapper.findAll('[data-usage-quick-filter]')
    expect(fields.map(field => field.attributes('data-usage-quick-filter'))).toEqual(['model', 'user', 'provider', 'group'])
    for (const field of fields) {
      expect(field.isVisible()).toBe(true)
      expect(field.element.closest('[role="dialog"]')).toBeNull()
    }
    expect(wrapper.get('[role="dialog"]').findAll('[data-usage-quick-filter]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="usage-filter-toolbar"]').classes()).toContain('flex-wrap')
    wrapper.unmount()
  }, 60_000)

  it('updates quick selections and resets them with the panel filters', async () => {
    const filters = { ...defaultFilters(), user_id: 7, provider_id: 8, start_date: '2026-10-17', end_date: '2026-10-17' }
    const wrapper = mountFilters(filters as unknown as ReturnType<typeof defaultFilters>)
    await wrapper.setProps({ startDate: '2026-10-17', endDate: '2026-10-17' })
    await flushPromises()

    const model = wrapper.get('[data-usage-quick-filter="model"]').findComponent({ name: 'Select' })
    model.vm.$emit('update:modelValue', 'claude-test')
    model.vm.$emit('change')
    const group = wrapper.get('[data-usage-quick-filter="group"]').findComponent({ name: 'Select' })
    group.vm.$emit('update:modelValue', 3)
    group.vm.$emit('change')
    await flushPromises()

    expect(filters.model).toBe('claude-test')
    expect(filters.group_id).toBe(3)
    expect(wrapper.emitted('change')).toHaveLength(2)
    await wrapper.get('[data-testid="filter-reset"]').trigger('click')
    for (const field of ['model', 'user_id', 'provider_id', 'group_id'] as const) {
      expect(filters[field]).toBeUndefined()
    }
    expect(filters.start_date).toBe('2026-10-17')
    expect(filters.end_date).toBe('2026-10-17')
    wrapper.unmount()
  }, 60_000)

  it('searches, selects and clears a provider from the toolbar', async () => {
    vi.useFakeTimers()
    mockProvidersList.mockClear()
    mockProvidersList.mockResolvedValueOnce({ items: [{ id: 7, name: 'Part' }] })
    const filters = defaultFilters()
    const wrapper = mountFilters(filters)
    try {
      await flushPromises()
      const field = wrapper.get('[data-usage-quick-filter="provider"]')
      const input = field.get('input')
      await input.trigger('focus')
      await input.setValue('part')
      await vi.advanceTimersByTimeAsync(300)
      await flushPromises()
      expect(mockProvidersList).toHaveBeenCalledWith(1, 20, { search: 'part' })
      await field.get('button.dropdown-item').trigger('click')
      expect(filters.provider_id).toBe(7)
      expect((input.element as HTMLInputElement).value).toBe('Part')
      await field.get('[aria-label="Clear provider filter"]').trigger('click')
      expect(filters.provider_id).toBeUndefined()
      expect((input.element as HTMLInputElement).value).toBe('')
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  }, 60_000)
})

describe('UsageFilters — user search dropdown', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockSearchUsers.mockReset()
    mockSearchApiKeys.mockResolvedValue([])
    mockGetModelStats.mockClear()
    mockGroupsList.mockClear()
    mockTeamsList.mockClear()
    mockProvidersList.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('(a) labels deleted users with the i18n badge and (b) sorts active users before deleted ones, (c) selection sets user_id', async () => {
    // 故意让接口先返回已删用户，验证组件会把活跃用户排在前面。
    mockSearchUsers.mockResolvedValue([
      { id: 2, email: 'gone@test.com', deleted: true },
      { id: 1, email: 'active@test.com', deleted: false },
    ])

    const wrapper = mountFilters()

    // 聚焦并输入关键词以触发防抖搜索。
    const input = wrapper.find('input[placeholder="Search user..."]')
    await input.trigger('focus')
    await input.setValue('test')
    await input.trigger('input')

    // 推进防抖定时器后等待搜索 Promise 完成。
    vi.advanceTimersByTime(300)
    await flushPromises()

    // 通过实际 DOM 顺序验证活跃用户排在已删用户前面。
    const buttons = wrapper.findAll('.usage-filter-dropdown button[type="button"]')
    const emailTexts = buttons.map((b) => b.text())

    const activeIdx = emailTexts.findIndex((t) => t.includes('active@test.com'))
    const deletedIdx = emailTexts.findIndex((t) => t.includes('gone@test.com'))
    expect(activeIdx).toBeGreaterThanOrEqual(0)
    expect(deletedIdx).toBeGreaterThanOrEqual(0)
    expect(activeIdx).toBeLessThan(deletedIdx)

    const deletedButton = buttons[deletedIdx]
    expect(deletedButton.text()).toContain('deleted')

    const activeButton = buttons[activeIdx]
    expect(activeButton.text()).not.toContain('deleted')

    await activeButton.trigger('click')
    await flushPromises()

    const changeEmits = wrapper.emitted('change')
    expect(changeEmits).toBeTruthy()
    expect(changeEmits!.length).toBeGreaterThan(0)

    // 组件通过 toRef 修改 modelValue，再发出 change 事件。
    expect(wrapper.props('modelValue').user_id).toBe(1)
  })

  it('keeps results from the latest user search when responses arrive out of order', async () => {
    const firstSearch = deferred<Array<{ id: number; email: string; deleted: boolean }>>()
    const secondSearch = deferred<Array<{ id: number; email: string; deleted: boolean }>>()
    mockSearchUsers
      .mockImplementationOnce(() => firstSearch.promise)
      .mockImplementationOnce(() => secondSearch.promise)

    const wrapper = mountFilters()
    const input = wrapper.find('input[placeholder="Search user..."]')
    await input.trigger('focus')

    await input.setValue('a')
    vi.advanceTimersByTime(300)
    await flushPromises()

    await input.setValue('ab')
    vi.advanceTimersByTime(300)
    await flushPromises()

    secondSearch.resolve([{ id: 2, email: 'ab@test.com', deleted: false }])
    await flushPromises()
    expect(wrapper.text()).toContain('ab@test.com')

    firstSearch.resolve([{ id: 1, email: 'a@test.com', deleted: false }])
    await flushPromises()
    expect(wrapper.text()).toContain('ab@test.com')
    expect(wrapper.text()).not.toContain('a@test.com')
  })

  it('does not restore stale user results after the search is cleared', async () => {
    const pendingSearch = deferred<Array<{ id: number; email: string; deleted: boolean }>>()
    mockSearchUsers.mockImplementationOnce(() => pendingSearch.promise)

    const wrapper = mountFilters()
    const input = wrapper.find('input[placeholder="Search user..."]')
    await input.trigger('focus')

    await input.setValue('stale')
    vi.advanceTimersByTime(300)
    await flushPromises()

    await input.setValue('')
    vi.advanceTimersByTime(300)
    await flushPromises()

    pendingSearch.resolve([{ id: 3, email: 'stale@test.com', deleted: false }])
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale@test.com')
  })
})

describe('UsageFilters — model options come from prop (no dup request)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockGetModelStats.mockClear()
    mockGroupsList.mockClear()
    mockTeamsList.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not call dashboard.getModelStats on mount and renders model options from prop', async () => {
    const wrapper = mount(UsageFilters, {
      props: {
        modelValue: defaultFilters(),
        exporting: false,
        startDate: '2026-05-01',
        endDate: '2026-05-28',
        showActions: false,
        modelOptions: ['claude-3', 'gpt-4o'],
      },
      global: {
        stubs: {
          Select: true,
          Teleport: true,
        },
      },
    })
    await flushPromises()

    expect(mockGetModelStats).not.toHaveBeenCalled()

    const opts = (wrapper.vm as any).modelOptions as Array<{ value: string | null; label: string }>
    expect(opts.map((o) => o.value)).toEqual([null, 'claude-3', 'gpt-4o'])
  })
})

describe('UsageFilters — team options', () => {
  it('loads admin teams into the project Select options', async () => {
    mockTeamsList.mockResolvedValueOnce([
      { id: 9, name: 'Platform Team' },
    ])

    const wrapper = mountFilters()
    await flushPromises()

    const options = (wrapper.vm as any).teamOptions as Array<{ value: number | null; label: string }>
    expect(options).toEqual([
      { value: null, label: 'All Teams' },
      { value: 9, label: 'Platform Team' },
    ])
  })
})

describe('UsageFilters — 未设置的条件', () => {
  it('undefined 条件按 null 传给下拉框，显示“全部”选项；选择后写回筛选对象', async () => {
    const filters: Record<string, any> = { start_date: '', end_date: '' }
    const wrapper = mountFilters(filters as ReturnType<typeof defaultFilters>)
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    expect(selects.length).toBeGreaterThan(0)
    for (const select of selects) {
      expect(select.props('modelValue')).toBeNull()
    }

    const typeSelect = selects.find((select) => select.props('options')?.[0]?.label === 'All Types')
    typeSelect!.vm.$emit('update:modelValue', 'sync')
    expect(filters.request_type).toBe('sync')
  })
})
