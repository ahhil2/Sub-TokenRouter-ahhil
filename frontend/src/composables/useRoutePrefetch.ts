import { ref, readonly } from 'vue'
import { hasPendingRequests } from '@/api/client'
import type { RouteLocationNormalized, Router } from 'vue-router'

// useRoutePrefetch.ts schedules adjacent route component imports during browser idle time.
// Its adjacency table contains route paths, and import functions are resolved from the
// supplied router when work actually runs. A one-second delay lets page requests
// register before idle admission, which waits again while API requests are pending.
// Hidden pages and restricted connections skip speculative work. A new trigger
// replaces pending scheduling while dispatched imports continue independently.
//
// Completed import batches mark the originating route as processed, including batches
// whose individual import failures were caught. That set suppresses later scheduling for
// the same origin until reset. Reset cancels pending callbacks and clears the set but does
// not cancel active imports, which can repopulate it when they finish. Callers own
// lifecycle cancellation.

// ComponentImportFn represents a lazy route component import returning a promise.
type ComponentImportFn = () => Promise<unknown>

// PREFETCH_ADJACENCY maps current route paths to neighboring route paths whose lazy
// components should be prefetched.
const PREFETCH_ADJACENCY: Record<string, string[]> = {
  '/admin/dashboard': ['/admin/providers', '/admin/users'],
  '/admin/providers': ['/admin/dashboard', '/admin/users'],
  '/admin/users': ['/admin/groups', '/admin/dashboard'],
  '/admin/groups': ['/admin/subscriptions', '/admin/users'],
  '/admin/subscriptions': ['/admin/groups', '/admin/redeem'],

  '/dashboard': ['/keys', '/usage'],
  '/keys': ['/dashboard', '/usage'],
  '/usage': ['/keys', '/redeem'],
  '/redeem': ['/usage', '/profile'],
  '/profile': ['/dashboard', '/keys'],
}

/**
 * scheduleIdleCallback returns cancellation for an idle callback or delayed timer.
 * Browsers without idle scheduling wait one second; the owner must invoke the returned
 * function when queued work becomes obsolete.
 */
function scheduleIdleCallback(callback: () => void): () => void {
  if (typeof window.requestIdleCallback === 'function') {
    const handle = window.requestIdleCallback(callback)
    return () => window.cancelIdleCallback(handle)
  }
  const handle = setTimeout(callback, 1000)
  return () => clearTimeout(handle)
}

// useRoutePrefetch returns a route-prefetch scheduler using the optional router's lazy
// components. Missing routers or non-lazy route components produce no imports. Callers
// cancel scheduled work when its owner is disposed.
//
// Each trigger replaces only the pending idle callback. In-progress import batches finish
// independently and mark their originating path processed even when individual imports
// fail. Reset clears completed origins but cannot stop active imports from recording
// completion later, so its effects concern currently retained scheduler state.
export function useRoutePrefetch(router?: Router) {
  // The cancellation handle follows the currently scheduled delay or idle callback.
  // Replacing navigation cancels that phase before admitting another prefetch request.
  let cancelScheduledPrefetch: (() => void) | undefined

  const prefetchedRoutes = ref<Set<string>>(new Set())

  // getComponentImporter finds the default lazy component function for an exact router
  // path. It returns null when the router, route, default component, or function
  // representation is unavailable.
  const getComponentImporter = (path: string): ComponentImportFn | null => {
    if (!router) return null

    const routes = router.getRoutes()
    const route = routes.find((r) => r.path === path)

    if (route && route.components?.default) {
      const component = route.components.default

      if (typeof component === 'function') {
        return component as ComponentImportFn
      }
    }
    return null
  }

  // getPrefetchPaths returns configured adjacent paths, or an empty list for an unknown
  // route. Configured arrays belong to the shared adjacency table and callers should treat
  // them as read-only.
  const getPrefetchPaths = (route: RouteLocationNormalized): string[] => {
    return PREFETCH_ADJACENCY[route.path] || []
  }

  // prefetchComponent awaits one lazy import and absorbs its rejection. Failures produce a
  // development-only debug message; the returned promise resolves so neighboring imports
  // can complete their batch bookkeeping.
  const prefetchComponent = async (importFn: ComponentImportFn): Promise<void> => {
    try {
      await importFn()
      // Prefetch failure should not reject the batch or interrupt navigation; development
      // logging retains the error for inspection.
    } catch (error) {
      if (import.meta.env.DEV) {
        console.debug('[Prefetch] Failed to prefetch component:', error)
      }
    }
  }

  // cancelPendingPrefetch removes the currently scheduled idle callback and clears its
  // handle. It is safe when nothing is scheduled and has no effect on imports already
  // dispatched.
  const cancelPendingPrefetch = (): void => {
    cancelScheduledPrefetch?.()
    cancelScheduledPrefetch = undefined
  }

  // triggerPrefetch replaces pending idle work with imports adjacent to the supplied
  // route. Origins with no configured neighbors schedule nothing. Once idle work begins,
  // previously completed origins are skipped and current router import functions are
  // collected.
  //
  // A nonempty batch runs concurrently and records its originating path after every import
  // wrapper resolves. Individual failures are swallowed by that wrapper, so the recorded
  // set represents completed attempts. Cancellation or reset after dispatch does not stop
  // that completion callback.
  const triggerPrefetch = (route: RouteLocationNormalized): void => {
    // Replace the scheduled callback before choosing neighbors so a route with no
    // neighbors also cancels obsolete queued work.
    cancelPendingPrefetch()

    const prefetchPaths = getPrefetchPaths(route)
    if (prefetchPaths.length === 0) return

    const connection = (
      navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }
    ).connection
    if (connection?.saveData || ['slow-2g', '2g', '3g'].includes(connection?.effectiveType ?? ''))
      return
    // schedule admits this route's imports after an idle check. Hidden pages and
    // changed routes stop the attempt; active API requests defer another admission.
    // Completed import wrappers record the origin even when an individual import
    // fails, preventing repeated speculative attempts until scheduler reset.
    //
    // The cancellation handle follows the current timer or idle callback. Imports
    // already dispatched remain outside that cancellation scope and may finish
    // after reset, so their completion can record the originating route again.
    const schedule = () => {
      cancelScheduledPrefetch = scheduleIdleCallback(() => {
        cancelScheduledPrefetch = undefined
        if (
          document.visibilityState === 'hidden' ||
          (router && router.currentRoute.value.path !== route.path)
        )
          return
        if (hasPendingRequests()) {
          delaySchedule()
          return
        }
        if (prefetchedRoutes.value.has(route.path)) return

        const importFns = prefetchPaths
          .map(getComponentImporter)
          .filter((fn): fn is ComponentImportFn => fn !== null)
        if (importFns.length > 0) {
          void Promise.all(importFns.map(prefetchComponent)).then(() => {
            prefetchedRoutes.value.add(route.path)
          })
        }
      })
    }
    // delaySchedule waits one second before requesting idle admission. It replaces
    // the cancellation handle with the new timer so navigation or disposal can stop
    // work before any component import begins.
    const delaySchedule = () => {
      const handle = setTimeout(schedule, 1000)
      cancelScheduledPrefetch = () => clearTimeout(handle)
    }
    // Page queries begin after successful navigation, so delay admission until mounting
    // can register those requests before the idle callback checks network activity.
    delaySchedule()
  }

  // resetPrefetchState cancels scheduled idle work and clears recorded origin paths.
  // Already dispatched imports retain their promises and can add their origin again when
  // their batch completes.
  const resetPrefetchState = (): void => {
    cancelPendingPrefetch()
    prefetchedRoutes.value.clear()
  }

  // isAdminRoute reports whether a path begins with the administrator prefix.
  const isAdminRoute = (path: string): boolean => {
    return path.startsWith('/admin')
  }

  // getPrefetchConfig returns available lazy import functions for the route's configured
  // neighbors. It skips missing or eager components and resolves functions on demand
  // without executing any imports.
  const getPrefetchConfig = (route: RouteLocationNormalized): ComponentImportFn[] => {
    const paths = getPrefetchPaths(route)
    const importFns: ComponentImportFn[] = []
    for (const path of paths) {
      const importFn = getComponentImporter(path)
      if (importFn) importFns.push(importFn)
    }
    return importFns
  }

  return {
    prefetchedRoutes: readonly(prefetchedRoutes),
    triggerPrefetch,
    cancelPendingPrefetch,
    resetPrefetchState,
    _getPrefetchConfig: getPrefetchConfig,
    _isAdminRoute: isAdminRoute,
  }
}

// _adminPrefetchMap and _userPrefetchMap expose the shared adjacency object for
// compatibility with existing route-prefetch tests.
export const _adminPrefetchMap = PREFETCH_ADJACENCY
export const _userPrefetchMap = PREFETCH_ADJACENCY
