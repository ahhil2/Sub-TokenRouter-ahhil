import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { OpsDashboardOverview } from '@/api/admin/ops'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

// OpsDashboardHeader.spec.ts checks realtime traffic ownership as filters change
// and the dashboard header unmounts. Deferred API responses expose cancellation
// and publication ordering through rendered QPS and TPS, while failed observations
// remain distinct from measured zero traffic.

// Realtime request and monitoring doubles expose late responses that must not
// overwrite the selected query or alter shared settings after component teardown.
const mocks = vi.hoisted(() => ({
  getRealtimeTrafficSummary: vi.fn(),
  setOpsRealtimeMonitoringEnabledLocal: vi.fn(),
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getRealtimeTrafficSummary: mocks.getRealtimeTrafficSummary },
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsRealtimeMonitoringEnabled: true,
    setOpsRealtimeMonitoringEnabledLocal: mocks.setOpsRealtimeMonitoringEnabledLocal,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

// mountHeader renders realtime measurements with a historical overview and
// lightweight control substitutes. Tests own the returned wrapper and unmount it
// after observing current measurements or the cancellation signal.
function mountHeader() {
  return mount(OpsDashboardHeader, {
    props: {
      overview: {} as OpsDashboardOverview,
      platform: 'anthropic',
      groupId: 1,
      timeRange: '1h',
      loading: false,
      lastUpdated: null,
    },
    global: {
      stubs: {
        Icon: true,
        Select: true,
        FilterDropdown: true,
        FilterField: true,
        HelpTooltip: true,
        BaseDialog: true,
      },
    },
  })
}

describe('OpsDashboardHeader realtime requests', () => {
  beforeEach(() => {
    mocks.getRealtimeTrafficSummary.mockReset()
    mocks.setOpsRealtimeMonitoringEnabledLocal.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // A group change arrives while the previous group's realtime request is pending.
  // The latest query starts immediately, cancels the old transport, and keeps its
  // measurements even when the earlier response arrives afterward.
  it('loads the latest group while busy and rejects the prior group response', async () => {
    let resolveOld!: (value: unknown) => void
    let resolveLatest!: (value: unknown) => void
    mocks.getRealtimeTrafficSummary
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveLatest = resolve
          }),
      )
    const wrapper = mountHeader()
    const oldSignal = mocks.getRealtimeTrafficSummary.mock.calls[0][3].signal as AbortSignal
    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('-')

    await wrapper.setProps({ groupId: 2 })

    expect(oldSignal.aborted).toBe(true)
    expect(mocks.getRealtimeTrafficSummary).toHaveBeenCalledTimes(2)
    expect(mocks.getRealtimeTrafficSummary).toHaveBeenLastCalledWith('1min', 'anthropic', 2, {
      signal: expect.any(AbortSignal),
    })
    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('-')
    resolveLatest({ enabled: true, summary: { qps: { current: 77 }, tps: { current: 99 } } })
    await flushPromises()
    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('77.0')

    resolveOld({ enabled: true, summary: { qps: { current: 11 }, tps: { current: 22 } } })
    await flushPromises()

    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('77.0')
    expect(wrapper.get('[data-test="realtime-tps"]').text()).toBe('99.0')
    wrapper.unmount()
  })

  // A pending realtime response arrives after the header has been removed.
  // Unmounting cancels its transport and prevents the late response from changing
  // shared monitoring settings or reporting an error.
  it('cancels on unmount and ignores late monitoring updates', async () => {
    let resolveRequest!: (value: unknown) => void
    mocks.getRealtimeTrafficSummary.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRequest = resolve
        }),
    )
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const wrapper = mountHeader()
    const signal = mocks.getRealtimeTrafficSummary.mock.calls[0][3].signal as AbortSignal

    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolveRequest({ enabled: false, summary: null })
    await flushPromises()

    expect(mocks.setOpsRealtimeMonitoringEnabledLocal).not.toHaveBeenCalled()
    expect(errorLog).not.toHaveBeenCalled()
    expect(mocks.getRealtimeTrafficSummary).toHaveBeenCalledTimes(1)
  })

  // A same-query refresh fails after measured traffic has been displayed.
  // Current QPS and TPS become unavailable, so a transport failure cannot look
  // like a successful observation of zero requests or tokens.
  it('shows unavailable rates after a realtime refresh fails', async () => {
    mocks.getRealtimeTrafficSummary.mockResolvedValueOnce({
      enabled: true,
      summary: { qps: { current: 8 }, tps: { current: 16 } },
    })
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const wrapper = mountHeader()
    await flushPromises()
    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('8.0')
    mocks.getRealtimeTrafficSummary.mockRejectedValueOnce({ code: 'REQUEST_TIMEOUT' })

    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="realtime-qps"]').text()).toBe('-')
    expect(wrapper.get('[data-test="realtime-tps"]').text()).toBe('-')
    wrapper.unmount()
  })
})
