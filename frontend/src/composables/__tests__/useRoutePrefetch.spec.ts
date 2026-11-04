import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteLocationNormalized, Router } from 'vue-router'
import { useRoutePrefetch } from '../useRoutePrefetch'

/**
 * useRoutePrefetch.spec.ts verifies idle route prefetch admission under navigation, network, and visibility changes.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */
const hasPendingRequests = vi.hoisted(() => vi.fn(() => false))
vi.mock('@/api/client', () => ({ hasPendingRequests }))
/**
 * route creates the minimal destination used by the prefetch scheduler fixture.
 */
const route = (path: string) => ({ path }) as RouteLocationNormalized
/**
 * fixture supplies a router with observable lazy imports and mutable current navigation.
 */
function fixture() {
  const load = vi.fn().mockResolvedValue({ default: {} })
  const currentRoute = ref(route('/admin/dashboard'))
  const router = {
    currentRoute,
    getRoutes: () =>
      ['/admin/providers', '/admin/users', '/admin/groups', '/admin/dashboard'].map((path) => ({
        path,
        components: { default: load },
      })),
  } as unknown as Router
  return { ...useRoutePrefetch(router), currentRoute, load }
}

beforeEach(() => {
  vi.useFakeTimers()
  hasPendingRequests.mockReturnValue(false)
  vi.stubGlobal(
    'requestIdleCallback',
    vi.fn((callback: () => void) => setTimeout(callback, 0)),
  )
  vi.stubGlobal(
    'cancelIdleCallback',
    vi.fn((handle: number) => clearTimeout(handle)),
  )
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('路由预取', () => {
  // Page requests remain active after navigation has mounted its destination. Prefetch must
  // wait for their completion and idle admission, then reuse the completed-origin record.
  it('等待页面挂载和业务请求完成，再使用空闲回调', async () => {
    const state = fixture()
    state.triggerPrefetch(state.currentRoute.value)
    await vi.advanceTimersByTimeAsync(999)
    expect(state.load).not.toHaveBeenCalled()
    hasPendingRequests.mockReturnValue(true)
    await vi.advanceTimersByTimeAsync(1001)
    expect(state.load).not.toHaveBeenCalled()
    hasPendingRequests.mockReturnValue(false)
    await vi.advanceTimersByTimeAsync(1001)
    expect(state.load).toHaveBeenCalledTimes(2)
    expect(window.requestIdleCallback).toHaveBeenCalledWith(expect.any(Function))
    state.triggerPrefetch(state.currentRoute.value)
    await vi.advanceTimersByTimeAsync(1001)
    expect(state.load).toHaveBeenCalledTimes(2)
  })

  // The browser reports data-saving mode or a slow effective connection before scheduling.
  // Adjacent route imports must remain unstarted after timers have fully advanced.
  it.each([{ saveData: true }, { effectiveType: '3g' }, { effectiveType: '2g' }])(
    '受限网络 %j 跳过预取',
    async (connection) => {
      vi.stubGlobal('navigator', { connection })
      const state = fixture()
      state.triggerPrefetch(state.currentRoute.value)
      await vi.advanceTimersByTimeAsync(5000)
      expect(state.load).not.toHaveBeenCalled()
    },
  )
  // Queued prefetch is cancelled, its document becomes hidden, or its route changes. The
  // delayed callback must avoid importing neighbors for the obsolete destination.
  it.each(['cancel', 'hidden', 'navigation'])('%s 会跳过等待中的预取', async (reason) => {
    const state = fixture()
    state.triggerPrefetch(state.currentRoute.value)
    if (reason === 'cancel') state.cancelPendingPrefetch()
    if (reason === 'hidden') vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    if (reason === 'navigation') state.currentRoute.value = route('/admin/users')
    await vi.advanceTimersByTimeAsync(1001)
    expect(state.load).not.toHaveBeenCalled()
  })

  // A new navigation replaces a queued origin before imports begin. Completion must record
  // the new origin, and reset must clear that recorded state.
  it('导航切换取消旧任务，重置后允许再次预取', async () => {
    const state = fixture()
    state.triggerPrefetch(state.currentRoute.value)
    state.currentRoute.value = route('/admin/users')
    state.triggerPrefetch(state.currentRoute.value)
    await vi.advanceTimersByTimeAsync(1001)
    expect([...state.prefetchedRoutes.value]).toEqual(['/admin/users'])
    state.resetPrefetchState()
    expect(state.prefetchedRoutes.value.size).toBe(0)
  })

  // The browser has no idle callback implementation and uses the delayed scheduler.
  // Cancellation must still prevent imports before the replacement timeout is allowed to
  // run.
  it('没有空闲回调时仍可取消延时预取', async () => {
    vi.stubGlobal('requestIdleCallback', undefined)
    const state = fixture()
    state.triggerPrefetch(state.currentRoute.value)
    await vi.advanceTimersByTimeAsync(1001)
    state.cancelPendingPrefetch()
    await vi.advanceTimersByTimeAsync(1001)
    expect(state.load).not.toHaveBeenCalled()
  })
})
