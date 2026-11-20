import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsTokenStatsCard from '../OpsTokenStatsCard.vue'

// OpsTokenStatsCard.spec.ts checks token-statistics filters, ranking and
// pagination, and retrieval presentation with mocked API results. Selector
// doubles emit the public model events while the real component builds requests,
// updates pages, and renders empty or failed responses. Remounting verifies that
// page-size selections reach browser storage and govern later requests.

// mockGetTokenStats captures mode and filter parameters while each scenario
// controls response rows, pagination totals, or retrieval failures.
const mockGetTokenStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getTokenStats: (...args: any[]) => mockGetTokenStats(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (key === 'admin.ops.tokenStats.pageInfo' && params) {
          return `第 ${params.page}/${params.total} 页`
        }
        return key
      },
    }),
  }
})

// SelectStub exposes model events and options so tests drive the real card query
// controls without dropdown interactions.
const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    }, // selected control fixture value
    options: {
      type: Array,
      default: () => [],
    }, // selectable control fixture entries
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

// EmptyStateStub renders its supplied labels so tests can observe the card
// decision to display no matching statistics.
const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' }, // displayed stub heading text
    description: { type: String, default: '' }, // displayed stub explanatory text
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

// sampleResponse supplies one measured model and enough total rows to exercise
// pagination independently of the visible fixture length.
const sampleResponse = {
  time_range: '30d' as const,
  start_time: '2026-01-01T00:00:00Z',
  end_time: '2026-01-31T00:00:00Z',
  platform: 'anthropic',
  group_id: 7,
  items: [
    {
      model: 'claude-sonnet-4',
      request_count: 12,
      avg_tokens_per_sec: 22.5,
      avg_ttft_ms: 123.45,
      total_output_tokens: 1234,
      avg_duration_ms: 321,
      requests_with_first_token: 10,
    },
  ],
  total: 40,
  page: 1,
  page_size: 20,
  top_n: null,
}

describe('OpsTokenStatsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    delete window.__APP_CONFIG__
  })

  afterEach(() => {
    localStorage.clear()
    delete window.__APP_CONFIG__
  })

  // Another table saves two hundred rows, exceeding this statistics endpoint's
  // limit. Pagination must request one hundred and exclude unsupported choices
  // while retaining the saved preference until the user selects another size.
  it('bounds an oversized saved preference to the statistics API limit', async () => {
    window.__APP_CONFIG__ = { table_page_size_options: [10, 20, 50, 100, 200] } as any
    localStorage.setItem('table-page-size', '200')
    mockGetTokenStats.mockResolvedValue(sampleResponse)
    const wrapper = mount(OpsTokenStatsCard, {
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    await flushPromises()
    wrapper.findAllComponents(SelectStub)[2].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()

    const sizeSelect = wrapper.findAllComponents(SelectStub)[3]
    expect(sizeSelect.props('modelValue')).toBe(100)
    expect(sizeSelect.props('options')).toEqual(
      [10, 20, 30, 50, 100].map((size) => ({ value: size, label: String(size) })),
    )
    expect(mockGetTokenStats).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 100 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(localStorage.getItem('table-page-size')).toBe('200')
    sizeSelect.vm.$emit('update:modelValue', 200)
    await flushPromises()
    expect(localStorage.getItem('table-page-size')).toBe('100')
    expect(sizeSelect.props('modelValue')).toBe(100)
    wrapper.unmount()
  })

  // The custom page-size control selects thirty rows under an older configured
  // option list. Remounting must request that saved size and continue offering
  // every standard choice.
  it('restores the selected page size after remounting', async () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 20,
      table_page_size_options: [10, 20, 50],
    } as any
    mockGetTokenStats.mockResolvedValue(sampleResponse)
    const mountOptions = {
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    }
    const wrapper = mount(OpsTokenStatsCard, mountOptions)
    await flushPromises()
    wrapper.findAllComponents(SelectStub)[2].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()
    const sizeSelect = wrapper.findAllComponents(SelectStub)[3]
    expect(sizeSelect.props('options')).toEqual(
      [10, 20, 30, 50, 100].map((size) => ({ value: size, label: String(size) })),
    )
    sizeSelect.vm.$emit('update:modelValue', 30)
    await flushPromises()
    expect(mockGetTokenStats).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 30 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    wrapper.unmount()

    const reopened = mount(OpsTokenStatsCard, mountOptions)
    await flushPromises()
    reopened.findAllComponents(SelectStub)[2].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()
    expect(reopened.findAllComponents(SelectStub)[3].props('modelValue')).toBe(30)
    expect(mockGetTokenStats).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 30 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    reopened.unmount()
  })

  // The card starts with platform and group constraints and then changes its
  // statistics window. Both initial and subsequent API calls must retain the
  // parent filters.
  it('默认加载并透传 platform/group 过滤，支持时间窗口切换', async () => {
    mockGetTokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsTokenStatsCard, {
      props: {
        platformFilter: 'anthropic',
        groupIdFilter: 7,
        groups: [{ id: 7, name: 'Anthropic 主分组', platform: 'anthropic' }],
      },
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()
    expect(mockGetTokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '30d',
        platform: 'anthropic',
        group_id: 7,
        top_n: 20,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', '1h')
    await flushPromises()

    expect(mockGetTokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '1h',
        platform: 'anthropic',
        group_id: 7,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  // The view changes from ranking to pagination and back before selecting a
  // larger top-N limit. Requests must use the active mode parameters and advance
  // the requested page.
  it('支持分页与 TopN 模式切换并按参数请求', async () => {
    mockGetTokenStats.mockImplementation(async (params: Record<string, any>) => ({
      ...sampleResponse,
      time_range: params.time_range ?? '30d',
      page: params.page ?? 1,
      page_size: params.page_size ?? 20,
      top_n: params.top_n ?? null,
      total: 40,
    }))

    const wrapper = mount(OpsTokenStatsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()

    let selects = wrapper.findAllComponents(SelectStub)
    await selects[2].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()

    expect(mockGetTokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 1,
        page_size: 20,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )

    const buttons = wrapper.findAll('button')
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    await buttons[1].trigger('click')
    await flushPromises()

    expect(mockGetTokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 2,
        page_size: 20,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )

    selects = wrapper.findAllComponents(SelectStub)
    await selects[2].vm.$emit('update:modelValue', 'topn')
    await flushPromises()
    selects = wrapper.findAllComponents(SelectStub)
    await selects[3].vm.$emit('update:modelValue', 50)
    await flushPromises()

    expect(mockGetTokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        top_n: 50,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  // The parent supplies a selected group and the user chooses another. The card
  // must emit that group ID and query it after the parent updates its applied
  // prop.
  it('分组选择器与父级 groupId 双向同步', async () => {
    mockGetTokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsTokenStatsCard, {
      props: {
        platformFilter: 'anthropic',
        groupIdFilter: 7,
        groups: [
          { id: 7, name: 'Anthropic 主分组', platform: 'anthropic' },
          { id: 8, name: 'Anthropic 备用分组', platform: 'anthropic' },
          { id: 9, name: '其他平台', platform: 'gemini' },
        ],
      },
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()

    const groupSelect = wrapper.findAllComponents(SelectStub)[0]
    expect(groupSelect.props('modelValue')).toBe(7)
    expect(groupSelect.props('options')).toEqual([
      { value: null, label: 'common.all' },
      { value: 7, label: 'Anthropic 主分组' },
      { value: 8, label: 'Anthropic 备用分组' },
      { value: 9, label: '其他平台' },
    ])

    await groupSelect.vm.$emit('update:modelValue', 8)
    expect(wrapper.emitted('update:group')).toEqual([[8]])

    await wrapper.setProps({ groupIdFilter: 8 })
    await flushPromises()
    expect(mockGetTokenStats).toHaveBeenLastCalledWith(
      expect.objectContaining({ group_id: 8 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  // The statistics endpoint returns no items and zero matches. After loading
  // settles, the card must render its empty-state component for the selected
  // query.
  it('接口返回空数据时显示空态', async () => {
    mockGetTokenStats.mockResolvedValue({
      ...sampleResponse,
      items: [],
      total: 0,
    })

    const wrapper = mount(OpsTokenStatsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
  })

  // The statistics response includes a model row with valid measurements. The
  // mounted table must retain a capped scrolling container so additional rows
  // cannot grow the panel indefinitely.
  it('数据表使用固定高度滚动容器，避免纵向无限增长', async () => {
    mockGetTokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsTokenStatsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.max-h-panel').exists()).toBe(true)
  })

  // The statistics API rejects its request with a message. After loading
  // settles, the visible card must expose that message so the user can identify
  // the retrieval failure.
  it('接口异常时显示错误提示', async () => {
    mockGetTokenStats.mockRejectedValue(new Error('加载失败'))

    const wrapper = mount(OpsTokenStatsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.find('.empty-state').exists()).toBe(false)
  })

  // The card completes its initial request and the user clicks its refresh
  // control. The API call count must increase by one after that explicit action.
  it('只在用户点击刷新按钮时重新请求统计数据', async () => {
    mockGetTokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsTokenStatsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })
    await flushPromises()
    expect(mockGetTokenStats).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-test="token-stats-refresh"]').trigger('click')
    await flushPromises()

    expect(mockGetTokenStats).toHaveBeenCalledTimes(2)
  })

  // A successful response is followed by a failed refresh with identical
  // controls. Keep its model visible and mark the retained data as stale so
  // users can distinguish it from a fresh observation.
  it('retains the last successful statistics when refreshing fails', async () => {
    mockGetTokenStats.mockResolvedValueOnce(sampleResponse)
    const wrapper = mount(OpsTokenStatsCard, {
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    await flushPromises()
    mockGetTokenStats.mockRejectedValueOnce(new Error('请求超时'))
    await wrapper.get('[data-test="token-stats-refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'admin.ops.tokenStats.refreshFailedStale',
    )
    expect(wrapper.get('[role="alert"]').text()).toContain('请求超时')

    // A retry still displays the previous observation until it succeeds, so its
    // stale warning stays visible throughout the pending request.
    let resolveRetry!: (value: typeof sampleResponse) => void
    mockGetTokenStats.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRetry = resolve
        }),
    )
    await wrapper.get('[data-test="token-stats-refresh"]').trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'admin.ops.tokenStats.refreshFailedStale',
    )
    expect(wrapper.text()).toContain('claude-sonnet-4')
    resolveRetry(sampleResponse)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // A new group fails after the previous group loaded successfully. Its failure
  // must not leave the previous group's model visible under the new controls.
  it('clears previous statistics when a changed filter fails', async () => {
    mockGetTokenStats.mockResolvedValueOnce(sampleResponse)
    const wrapper = mount(OpsTokenStatsCard, {
      props: { groupIdFilter: 7 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    await flushPromises()
    mockGetTokenStats.mockRejectedValueOnce(new Error('请求超时'))
    await wrapper.setProps({ groupIdFilter: 8 })
    await flushPromises()

    expect(wrapper.text()).not.toContain('claude-sonnet-4')
    expect(wrapper.text()).not.toContain('admin.ops.tokenStats.refreshFailedStale')
    expect(wrapper.get('[role="alert"]').text()).toContain('请求超时')
    wrapper.unmount()
  })

  // The first request resolves after a different filter has already completed.
  // Abort the superseded request and ignore its late result, even if the adapter
  // does not honor cancellation.
  it('ignores late results from superseded filters', async () => {
    let resolveFirst!: (value: typeof sampleResponse) => void
    mockGetTokenStats.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve
        }),
    )
    const wrapper = mount(OpsTokenStatsCard, {
      props: { groupIdFilter: 7 },
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    const firstSignal = mockGetTokenStats.mock.calls[0][1].signal as AbortSignal
    mockGetTokenStats.mockResolvedValueOnce({
      ...sampleResponse,
      items: [{ ...sampleResponse.items[0], model: 'current-model' }],
    })
    await wrapper.setProps({ groupIdFilter: 8 })
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    expect(wrapper.text()).toContain('current-model')
    resolveFirst(sampleResponse)
    await flushPromises()

    expect(wrapper.text()).toContain('current-model')
    expect(wrapper.text()).not.toContain('claude-sonnet-4')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // Removing the card interrupts an outstanding request. Cancellation must
  // reach its AbortSignal and a late rejection must not emit a retrieval error.
  it('cancels the active request when the card unmounts', async () => {
    let rejectRequest!: (reason: Error) => void
    mockGetTokenStats.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectRequest = reject
        }),
    )
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = mount(OpsTokenStatsCard, {
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    const signal = mockGetTokenStats.mock.calls[0][1].signal as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    rejectRequest(new Error('cancelled after unmount'))
    await flushPromises()

    expect(errorLog).not.toHaveBeenCalled()
    errorLog.mockRestore()
  })

  // While page two is selected, the result count shrinks to one page. The card
  // requests the corrected page exactly once and displays its returned rows.
  it('requests the last valid page once when the total shrinks', async () => {
    mockGetTokenStats.mockResolvedValue(sampleResponse)
    const wrapper = mount(OpsTokenStatsCard, {
      global: { stubs: { Select: SelectStub, EmptyState: EmptyStateStub } },
    })
    await flushPromises()
    wrapper.findAllComponents(SelectStub)[2].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(mockGetTokenStats.mock.calls.at(-1)?.[0].page).toBe(2)

    mockGetTokenStats.mockResolvedValue({ ...sampleResponse, total: 10 })
    mockGetTokenStats.mockClear()
    await wrapper.get('[data-test="token-stats-refresh"]').trigger('click')
    await flushPromises()

    expect(mockGetTokenStats.mock.calls.map(([params]) => params.page)).toEqual([2, 1])
    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('第 1/1 页')
    wrapper.unmount()
  })
})
