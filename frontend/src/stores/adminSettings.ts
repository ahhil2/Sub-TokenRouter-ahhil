import { defineStore } from 'pinia'
import { onScopeDispose, ref } from 'vue'
import { getLocale } from '@/i18n'
import type { CustomMenuItem } from '@/api/settings.types'

/**
 * adminSettings.ts shares administrator navigation settings and payment availability.
 * Browser-cached switches initialize the navigation while requests load. Settings
 * and payment responses publish together, and successful reads record the credential
 * and language that own the result. Failed reads remain retryable and retain the
 * last published switches for the current display.
 *
 * Concurrent callers in the same scope await one request. Force refresh creates
 * new ownership, and a changed account or language starts a distinct request;
 * older completions cannot publish into either replacement. Local setters update
 * navigation immediately and attempt storage persistence. The store also owns an
 * operations-disabled listener, removed with pending publication ownership when
 * the Pinia scope is disposed. Settings and payment clients load when fetch is
 * admitted, keeping them outside the initial navigation store dependency graph.
 */
/**
 * useAdminSettingsStore exposes navigation switches and custom menus through Pinia.
 * Callers await fetch before interpreting readiness. Local setters update individual
 * switches, while complete successful fetches establish the current account and language.
 */
export const useAdminSettingsStore = defineStore('adminSettings', () => {
  // loaded records successful publication for loadedScope. loading follows the active
  // pending owner, which ordinary callers share and a forced request may replace.
  const loaded = ref(false)
  const loading = ref(false)

  // readCachedBool reads literal true or false from a browser storage key. Missing,
  // invalid, or inaccessible values return the supplied default so navigation
  // initialization can continue without storage.
  const readCachedBool = (key: string, defaultValue: boolean): boolean => {
    try {
      const raw = localStorage.getItem(key)
      if (raw === 'true') return true
      if (raw === 'false') return false
    } catch {
      // Browser storage may be unavailable during initialization; use the supplied
      // default so cached navigation reads can still complete.
    }
    return defaultValue
  }

  // writeCachedBool attempts to persist a navigation switch as a boolean string. Storage
  // failures are suppressed because reactive navigation state has already been updated
  // by the calling action.
  const writeCachedBool = (key: string, value: boolean) => {
    try {
      localStorage.setItem(key, value ? 'true' : 'false')
    } catch {
      // Reactive switches are already published by the caller; ignore storage
      // failure so navigation keeps the locally selected state.
    }
  }

  // Navigation switches initialize from browser storage before network loading;
  // customMenuItems is populated from administrator settings after the combined request
  // succeeds.
  const opsMonitoringEnabled = ref(readCachedBool('ops_monitoring_enabled_cached', true))
  const opsRealtimeMonitoringEnabled = ref(
    readCachedBool('ops_realtime_monitoring_enabled_cached', true),
  )
  const paymentEnabled = ref(readCachedBool('payment_enabled_cached', false))
  const customMenuItems = ref<CustomMenuItem[]>([])
  // loadedScope identifies successful settings, while pending owns a combined lookup.
  // Replacing pending invalidates earlier publication even when those requests still finish.
  let loadedScope = ''
  let pending: {
    scope: string // Credential and language identity.
    promise: Promise<void> // Combined settings lookup completion.
  } | null = null

  /**
   * requestScope combines the active locale and browser credential for store ownership.
   */
  function requestScope(): string {
    let token = ''
    try {
      token = localStorage.getItem('auth_token') ?? ''
    } catch {
      // Storage restrictions leave an empty credential scope; locale still distinguishes
      // translated navigation, and each forced request receives separate publication ownership.
    }
    return `${getLocale()}:${token}`
  }

  /**
   * fetch loads settings and payment availability for the current credential and language.
   * Ordinary callers await pending work or reuse a complete result for the same scope.
   * Force refresh bypasses both the store's freshness and the settings API cache.
   *
   * The request object owns publication across both endpoint responses. A newer
   * request, disposal, or changed scope prevents older responses from publishing;
   * only the owner can clear loading. Failures retain the prior displayed switches,
   * leave readiness false, and permit a later fetch to retry.
   */
  async function fetch(force = false): Promise<void> {
    const scope = requestScope()
    if (!force && pending?.scope === scope) return pending.promise
    if (!force && loaded.value && loadedScope === scope) return

    loaded.value = false
    loading.value = true
    const request = {
      scope,
      promise: Promise.all([import('@/api/admin/settings'), import('@/api/admin/payment')])
        .then(async ([{ getSettings }, { adminPaymentAPI }]) => {
          if (pending !== request || requestScope() !== scope) return
          const [settings, paymentConfigResp] = await Promise.all([
            getSettings(force),
            adminPaymentAPI.getConfig(),
          ])
          if (pending !== request || requestScope() !== scope) return
          opsMonitoringEnabled.value = settings.ops_monitoring_enabled ?? true
          writeCachedBool('ops_monitoring_enabled_cached', opsMonitoringEnabled.value)
          opsRealtimeMonitoringEnabled.value = settings.ops_realtime_monitoring_enabled ?? true
          writeCachedBool(
            'ops_realtime_monitoring_enabled_cached',
            opsRealtimeMonitoringEnabled.value,
          )
          customMenuItems.value = Array.isArray(settings.custom_menu_items)
            ? settings.custom_menu_items
            : []
          paymentEnabled.value = paymentConfigResp.data?.enabled ?? false
          writeCachedBool('payment_enabled_cached', paymentEnabled.value)
          loaded.value = true
          loadedScope = scope
        })
        .catch((err) => {
          console.error('[adminSettings] Failed to fetch settings:', err)
        })
        .finally(() => {
          if (pending === request) {
            pending = null
            loading.value = false
          }
        }),
    }
    pending = request
    return request.promise
  }

  // setOpsMonitoringEnabledLocal publishes operations monitoring and attempts to persist
  // the switch. A subsequent settings response can replace this local navigation value
  // when the combined fetch completes successfully.
  function setOpsMonitoringEnabledLocal(value: boolean) {
    opsMonitoringEnabled.value = value
    writeCachedBool('ops_monitoring_enabled_cached', value)
  }

  // setOpsRealtimeMonitoringEnabledLocal publishes realtime monitoring and attempts to
  // persist the switch. A subsequent settings response can replace this local navigation
  // value when the combined fetch completes successfully.
  function setOpsRealtimeMonitoringEnabledLocal(value: boolean) {
    opsRealtimeMonitoringEnabled.value = value
    writeCachedBool('ops_realtime_monitoring_enabled_cached', value)
  }

  // setPaymentEnabledLocal publishes payment availability and attempts to persist the
  // switch. A subsequent payment configuration response can replace this local navigation
  // value when the combined fetch completes successfully.
  function setPaymentEnabledLocal(value: boolean) {
    paymentEnabled.value = value
    writeCachedBool('payment_enabled_cached', value)
  }

  // eventHandlerCleanup retains the installed handler closure for this store lifetime
  // and acts as the marker that prevents duplicate listener registration.
  let eventHandlerCleanup: (() => void) | null = null

  // initializeEventListeners registers the operations-disabled event once for this store
  // instance. Its retained cleanup closure also acts as the installation marker; window-access failures are suppressed for environments lacking browser events.
  function initializeEventListeners() {
    if (eventHandlerCleanup) return

    try {
      // handler disables operations navigation after the interceptor reports unavailable
      // monitoring.
      const handler = () => {
        setOpsMonitoringEnabledLocal(false)
      }
      window.addEventListener('ops-monitoring-disabled', handler)
      eventHandlerCleanup = () => {
        window.removeEventListener('ops-monitoring-disabled', handler)
      }
    } catch {
      // A browser event target may be unavailable in this environment; leave the
      // installation marker empty so later initialization can retry.
    }
  }

  if (typeof window !== 'undefined') {
    initializeEventListeners()
  }

  // Release publication ownership before removing the listener so late requests cannot
  // update a disposed store after another application scope takes over navigation state.
  onScopeDispose(() => {
    pending = null
    eventHandlerCleanup?.()
  })

  return {
    loaded,
    loading,
    opsMonitoringEnabled,
    opsRealtimeMonitoringEnabled,
    paymentEnabled,
    customMenuItems,
    fetch,
    setOpsMonitoringEnabledLocal,
    setOpsRealtimeMonitoringEnabledLocal,
    setPaymentEnabledLocal,
  }
})
