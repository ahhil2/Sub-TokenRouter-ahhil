import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

// OpsDashboard.spec.ts checks dashboard filter propagation and independent
// histogram refreshes through mounted child controls. Request failures exercise
// stale overview presentation, filter changes, cancellation, and compatibility
// routing, while browser storage verifies that accepted histogram preferences
// survive another page instance.

// API and navigation doubles expose panel request counts, URL replacements, and
// settings initialization during dashboard filter scenarios.
const mocks = vi.hoisted(() => ({
  getDashboardSnapshotV2: vi.fn(),
  getDashboardOverview: vi.fn(),
  getErrorTrend: vi.fn(),
  getThroughputTrend: vi.fn(),
  getLatencyHistogram: vi.fn(),
  getErrorDistribution: vi.fn(),
  getAdvancedSettings: vi.fn(),
  getMetricThresholds: vi.fn(),
  getGroups: vi.fn(),
  routerReplace: vi.fn(),
  settingsFetch: vi.fn(),
  showError: vi.fn(),
}))

// route supplies a reactive query object so each scenario controls the dashboard
// initial deep-link and histogram state.
const route = reactive<{
  query: Record<string, string> // reactive dashboard query parameters
}>({ query: {} })

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace: mocks.routerReplace }),
}))

vi.mock('@vueuse/core', () => ({
  useDebounceFn: (fn: (...args: unknown[]) => unknown) => fn,
  useIntervalFn: () => ({ pause: vi.fn(), resume: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
    fetch: mocks.settingsFetch,
  }),
  useAppStore: () => ({ showError: mocks.showError }),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    groups: { getOptions: mocks.getGroups },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getDashboardSnapshotV2: mocks.getDashboardSnapshotV2,
    getDashboardOverview: mocks.getDashboardOverview,
    getErrorTrend: mocks.getErrorTrend,
    getThroughputTrend: mocks.getThroughputTrend,
    getLatencyHistogram: mocks.getLatencyHistogram,
    getErrorDistribution: mocks.getErrorDistribution,
    getAdvancedSettings: mocks.getAdvancedSettings,
    getMetricThresholds: mocks.getMetricThresholds,
  },
}))

import OpsDashboard from '../OpsDashboard.vue'
import OpsDashboardHeader from '../components/OpsDashboardHeader.vue'
import OpsRequestDetailsModal from '../components/OpsRequestDetailsModal.vue'
import { LATENCY_BUCKET_STORAGE_KEY } from '../latencyBuckets'

// Dialog and layout doubles render slots, while inactive panel doubles keep the
// fixture focused on dashboard request coordination.
const SlotStub = defineComponent({
  template: '<div><slot /></div>',
})

const EmptyStub = defineComponent({
  template: '<div />',
})

// LatencyChartStub emits a valid five-boundary edit through the same public
// event used by the real histogram editor.
const LatencyChartStub = defineComponent({
  name: 'OpsLatencyChart',
  emits: ['update:bucketBoundaries'],
  template: `
    <button
      data-test="apply-custom-latency-bounds"
      @click="$emit('update:bucketBoundaries', [1000, 2000, 5000, 10000, 20000])"
    />
  `,
})

// mountDashboard mounts the dashboard with request doubles and lightweight panel
// stubs. Passing realHeader exercises visible overview recovery controls. The
// returned wrapper owns lifecycle cleanup, so each test unmounts it after
// observing requests and query updates.
function mountDashboard(realHeader = false) {
  return mount(OpsDashboard, {
    global: {
      stubs: {
        AppLayout: SlotStub,
        BaseDialog: SlotStub,
        OpsDashboardSkeleton: EmptyStub,
        OpsDashboardHeader: realHeader ? false : true,
        Icon: true,
        Select: true,
        FilterDropdown: SlotStub,
        FilterField: SlotStub,
        HelpTooltip: true,
        OpsConcurrencyCard: EmptyStub,
        OpsSwitchRateTrendChart: EmptyStub,
        OpsThroughputTrendChart: EmptyStub,
        OpsLatencyChart: LatencyChartStub,
        OpsErrorDistributionChart: EmptyStub,
        OpsErrorTrendChart: EmptyStub,
        OpsTokenStatsCard: EmptyStub,
        OpsAlertEventsCard: EmptyStub,
        OpsSystemLogTable: EmptyStub,
        OpsSettingsDialog: EmptyStub,
        OpsAlertRulesCard: EmptyStub,
        OpsErrorDetailsModal: EmptyStub,
        OpsErrorDetailModal: EmptyStub,
        OpsRequestDetailsModal: EmptyStub,
      },
    },
  })
}

describe('OpsDashboard requests', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    route.query = {}
    localStorage.removeItem(LATENCY_BUCKET_STORAGE_KEY)
    mocks.settingsFetch.mockResolvedValue(undefined)
    mocks.getGroups.mockResolvedValue([])
    mocks.getMetricThresholds.mockResolvedValue(null)
    mocks.getAdvancedSettings.mockResolvedValue({
      display_alert_events: true,
      display_openai_token_stats: false,
      auto_refresh_enabled: false,
      auto_refresh_interval_seconds: 30,
    })
    mocks.getDashboardSnapshotV2.mockResolvedValue({
      overview: {},
      throughput_trend: { points: [] },
      error_trend: { points: [] },
    })
    mocks.getThroughputTrend.mockResolvedValue({ points: [] })
    mocks.getDashboardOverview.mockResolvedValue({ request_count_total: 42 })
    mocks.getErrorTrend.mockResolvedValue({ points: [] })
    mocks.getErrorDistribution.mockResolvedValue({ total: 0, items: [] })
    mocks.getLatencyHistogram.mockImplementation(
      async (params: {
        bucket_boundaries_ms?: number[] // requested millisecond histogram cutoffs
      }) => ({
        start_time: '',
        end_time: '',
        platform: '',
        bucket_boundaries_ms: params.bucket_boundaries_ms ?? [100, 200, 500, 1000, 2000],
        total_requests: 0,
        buckets: [],
      }),
    )
    mocks.routerReplace.mockResolvedValue(undefined)
  })

  // A loaded overview encounters a temporary server failure under unchanged filters.
  // Its measurements and success timestamp remain visible with a stale notice, and
  // the inline retry replaces them after the server recovers.
  it('retains the last overview on refresh failure and retries from its stale notice', async () => {
    mocks.getDashboardSnapshotV2.mockResolvedValueOnce({
      overview: { request_count_total: 42, health_score: 96 },
      throughput_trend: { points: [] },
      error_trend: { points: [] },
    })
    const wrapper = mountDashboard(true)
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)
    const lastUpdated = header.props('lastUpdated')

    mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ status: 500 })
    header.vm.$emit('refresh')
    await flushPromises()

    expect(header.props('overview')?.request_count_total).toBe(42)
    expect(header.props('overviewError')).toBe(true)
    expect(header.props('lastUpdated')).toBe(lastUpdated)
    expect(wrapper.get('[data-test="overview-status"]').text()).toContain(
      'admin.ops.overviewRefreshFailedStale',
    )
    expect(mocks.getDashboardOverview).not.toHaveBeenCalled()
    expect(mocks.getErrorTrend).not.toHaveBeenCalled()

    mocks.getDashboardSnapshotV2.mockResolvedValueOnce({
      overview: { request_count_total: 84, health_score: 98 },
      throughput_trend: { points: [] },
      error_trend: { points: [] },
    })
    await wrapper.get('[data-test="retry-overview"]').trigger('click')
    await flushPromises()

    expect(header.props('overview')?.request_count_total).toBe(84)
    expect(header.props('overviewError')).toBe(false)
    expect(wrapper.find('[data-test="overview-status"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // The first overview request fails before any measurements have been collected.
  // A visible error and retry action replace the metric area without implying that
  // missing health observations represent an idle or healthy system.
  it('shows a retryable placeholder after the first overview request fails', async () => {
    mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ status: 500 })
    const wrapper = mountDashboard(true)
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)

    expect(header.props('overview')).toBeNull()
    expect(header.props('lastUpdated')).toBeNull()
    expect(wrapper.get('[data-test="overview-status"]').attributes('role')).toBe('alert')
    expect(wrapper.get('[data-test="overview-status"]').text()).toContain(
      'admin.ops.failedToLoadOverview',
    )
    expect(header.text()).not.toContain('admin.ops.idleStatus')
    expect(header.text()).not.toContain('admin.ops.healthCondition')
    expect(wrapper.get('[data-test="retry-overview"]').exists()).toBe(true)

    await wrapper.get('[data-test="retry-overview"]').trigger('click')
    await flushPromises()
    expect(header.props('overviewError')).toBe(false)
    expect(header.props('overview')).not.toBeNull()
    wrapper.unmount()
  })

  // An unavailable or overloaded server rejects the snapshot route without indicating
  // that the endpoint is unsupported. The dashboard must report the failure without
  // amplifying it through the three legacy core requests.
  it.each([500, 429, 401, 403, 0])(
    'does not fall back after snapshot status %s',
    async (status) => {
      mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ status })
      const wrapper = mountDashboard()
      await flushPromises()

      expect(wrapper.getComponent(OpsDashboardHeader).props('overviewError')).toBe(true)
      expect(mocks.getDashboardOverview).not.toHaveBeenCalled()
      expect(mocks.getErrorTrend).not.toHaveBeenCalled()
      expect(mocks.getThroughputTrend).toHaveBeenCalledTimes(1)
      wrapper.unmount()
    },
  )

  // A server explicitly lacks support for the snapshot route while legacy queries
  // remain available. The dashboard loads those measurements and publishes the
  // successful overview with a valid freshness timestamp.
  it.each([404, 405, 501])('uses legacy core queries for unsupported status %s', async (status) => {
    mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ status })
    const wrapper = mountDashboard()
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)

    expect(mocks.getDashboardOverview).toHaveBeenCalledTimes(1)
    expect(mocks.getErrorTrend).toHaveBeenCalledTimes(1)
    expect(mocks.getThroughputTrend).toHaveBeenCalledTimes(2)
    expect(header.props('overview')?.request_count_total).toBe(42)
    expect(header.props('overviewError')).toBe(false)
    expect(header.props('lastUpdated')).toBeInstanceOf(Date)
    wrapper.unmount()
  })

  // An older server accepts legacy queries but its overview fails during a refresh.
  // The previous overview remains available with its original successful timestamp
  // while the header identifies that the new observation could not be loaded.
  it('retains the overview when the legacy overview request fails', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)
    const previousOverview = header.props('overview')
    const lastUpdated = header.props('lastUpdated')
    mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ status: 404 })
    mocks.getDashboardOverview.mockRejectedValueOnce({ status: 500 })

    header.vm.$emit('refresh')
    await flushPromises()

    expect(header.props('overview')).toBe(previousOverview)
    expect(header.props('lastUpdated')).toBe(lastUpdated)
    expect(header.props('overviewError')).toBe(true)
    wrapper.unmount()
  })

  // A user changes the group while an earlier group's measurements are displayed.
  // Those values disappear before the new request settles, and a failure for the
  // new group cannot revive the previous group's overview.
  it('clears old-filter measurements before a new group request completes', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)
    let rejectSnapshot!: (error: unknown) => void
    mocks.getDashboardSnapshotV2.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectSnapshot = reject
        }),
    )

    header.vm.$emit('update:group', 2)
    await flushPromises()

    expect(header.props('overview')).toBeNull()
    expect(header.props('lastUpdated')).toBeNull()
    expect(header.props('overviewError')).toBe(false)
    rejectSnapshot({ status: 500 })
    await flushPromises()

    expect(header.props('overview')).toBeNull()
    expect(header.props('overviewError')).toBe(true)
    expect(mocks.getDashboardSnapshotV2).toHaveBeenLastCalledWith(
      expect.objectContaining({ group_id: 2 }),
      expect.any(Object),
    )
    wrapper.unmount()
  })

  // A request cancellation interrupts a refresh after a successful overview load.
  // Cancellation preserves the last measurements and does not produce error feedback
  // or compatibility requests for an endpoint that was never rejected.
  it('does not report canceled snapshot requests as failures', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const header = wrapper.getComponent(OpsDashboardHeader)
    const previousOverview = header.props('overview')
    mocks.getDashboardSnapshotV2.mockRejectedValueOnce({ code: 'ERR_CANCELED' })

    header.vm.$emit('refresh')
    await flushPromises()

    expect(header.props('overview')).toBe(previousOverview)
    expect(header.props('overviewError')).toBe(false)
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.getDashboardOverview).not.toHaveBeenCalled()
    expect(mocks.getErrorTrend).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // Two group selections overlap while the first request ignores cancellation.
  // A late success or failure from that superseded group must leave the latest
  // overview, its success timestamp, and its error state unchanged.
  it.each(['success', 'failure'])(
    'ignores a superseded snapshot %s after a newer group has loaded',
    async (outcome) => {
      const wrapper = mountDashboard()
      await flushPromises()
      const header = wrapper.getComponent(OpsDashboardHeader)
      let resolveOld!: (value: unknown) => void
      let rejectOld!: (error: unknown) => void
      mocks.getDashboardSnapshotV2.mockImplementationOnce(
        () =>
          new Promise((resolve, reject) => {
            resolveOld = resolve
            rejectOld = reject
          }),
      )
      header.vm.$emit('update:group', 2)
      await flushPromises()

      mocks.getDashboardSnapshotV2.mockResolvedValueOnce({
        overview: { request_count_total: 99 },
        throughput_trend: { points: [] },
        error_trend: { points: [] },
      })
      header.vm.$emit('update:group', 3)
      await flushPromises()
      const lastUpdated = header.props('lastUpdated')

      if (outcome === 'success') {
        resolveOld({ overview: { request_count_total: 7 } })
      } else {
        rejectOld({ status: 500 })
      }
      await flushPromises()

      expect(header.props('overview')?.request_count_total).toBe(99)
      expect(header.props('overviewError')).toBe(false)
      expect(header.props('lastUpdated')).toBe(lastUpdated)
      expect(mocks.showError).not.toHaveBeenCalled()
      expect(mocks.getDashboardOverview).not.toHaveBeenCalled()
      wrapper.unmount()
    },
  )

  // A bucket edit arrives from the histogram control after the dashboard has
  // loaded. It refreshes histogram data and stores the URL preference while
  // other panel request counts stay unchanged.
  it('应用新边界时只刷新时长图并同步 URL', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    expect(mocks.getDashboardSnapshotV2).toHaveBeenCalledTimes(1)
    expect(mocks.getThroughputTrend).toHaveBeenCalledTimes(1)
    expect(mocks.getLatencyHistogram).toHaveBeenCalledTimes(1)
    expect(mocks.getErrorDistribution).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-test="apply-custom-latency-bounds"]').trigger('click')
    await flushPromises()

    expect(mocks.getLatencyHistogram).toHaveBeenCalledTimes(2)
    expect(mocks.getLatencyHistogram).toHaveBeenLastCalledWith(
      expect.objectContaining({ bucket_boundaries_ms: [1000, 2000, 5000, 10000, 20000] }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(mocks.getDashboardSnapshotV2).toHaveBeenCalledTimes(1)
    expect(mocks.getThroughputTrend).toHaveBeenCalledTimes(1)
    expect(mocks.getErrorDistribution).toHaveBeenCalledTimes(1)
    expect(mocks.routerReplace).toHaveBeenCalledWith({
      query: { latency_bounds: '1000,2000,5000,10000,20000' },
    })
    expect(localStorage.getItem(LATENCY_BUCKET_STORAGE_KEY)).toBe('1000,2000,5000,10000,20000')

    wrapper.unmount()
  })

  // The URL omits histogram boundaries while browser storage contains a valid
  // preference. The first histogram request must use that stored sequence of
  // millisecond cutoffs.
  it('没有 URL 参数时恢复本地保存的分桶偏好', async () => {
    localStorage.setItem(LATENCY_BUCKET_STORAGE_KEY, '1000,2000,5000,10000,20000')

    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    expect(mocks.getLatencyHistogram).toHaveBeenCalledWith(
      expect.objectContaining({ bucket_boundaries_ms: [1000, 2000, 5000, 10000, 20000] }),
      expect.any(Object),
    )

    wrapper.unmount()
  })

  // The header selects a custom window and then changes its bounds again. Both
  // request-detail props and overview queries must follow each timestamp
  // selection while the mode remains custom.
  it('自定义时间边界传给请求明细，重复修改范围时概览同步刷新', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const header = wrapper.findComponent(OpsDashboardHeader)
    header.vm.$emit('update:customTimeRange', '2026-08-25T00:00:00Z', '2026-08-28T00:00:00Z')
    header.vm.$emit('update:timeRange', 'custom')
    await flushPromises()
    const details = wrapper.findComponent(OpsRequestDetailsModal)
    expect(details.attributes('custom-start-time')).toBe('2026-08-25T00:00:00Z')
    expect(details.attributes('custom-end-time')).toBe('2026-08-28T00:00:00Z')
    expect(mocks.getDashboardSnapshotV2).toHaveBeenLastCalledWith(
      expect.objectContaining({
        start_time: '2026-08-25T00:00:00Z',
        end_time: '2026-08-28T00:00:00Z',
      }),
      expect.any(Object),
    )
    const calls = mocks.getDashboardSnapshotV2.mock.calls.length
    header.vm.$emit('update:customTimeRange', '2026-08-26T00:00:00Z', '2026-08-27T00:00:00Z')
    header.vm.$emit('update:timeRange', 'custom')
    await flushPromises()
    expect(mocks.getDashboardSnapshotV2).toHaveBeenCalledTimes(calls + 1)
    expect(mocks.getDashboardSnapshotV2).toHaveBeenLastCalledWith(
      expect.objectContaining({
        start_time: '2026-08-26T00:00:00Z',
        end_time: '2026-08-27T00:00:00Z',
      }),
      expect.any(Object),
    )
    expect(details.attributes('custom-start-time')).toBe('2026-08-26T00:00:00Z')
    wrapper.unmount()
  })

  // The URL contains too few histogram boundaries and storage has no preference.
  // The dashboard must request standard boundaries and replace the invalid query
  // parameter with a cleaned route.
  it('非法 URL 边界回退默认值并清理参数', async () => {
    route.query = { latency_bounds: '100,200' }
    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    expect(mocks.getLatencyHistogram).toHaveBeenCalledWith(
      expect.objectContaining({ bucket_boundaries_ms: [100, 200, 500, 1000, 2000] }),
      expect.any(Object),
    )
    expect(mocks.routerReplace).toHaveBeenCalledWith({ query: {} })

    wrapper.unmount()
  })

  // A histogram edit occurs while fullscreen is enabled by the route. The
  // rewritten query must include both the custom boundaries and the active
  // fullscreen flag.
  it('全屏模式应用新边界时保留全屏参数', async () => {
    route.query = { fullscreen: '1' }
    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-test="apply-custom-latency-bounds"]').trigger('click')
    await flushPromises()

    expect(mocks.routerReplace).toHaveBeenCalledWith({
      query: {
        latency_bounds: '1000,2000,5000,10000,20000',
        fullscreen: '1',
      },
    })

    wrapper.unmount()
  })
})
