import { onScopeDispose, ref } from 'vue'

/**
 * useLatestRequest.ts coordinates replaceable reads for one Vue effect scope.
 * Each run aborts its predecessor and owns a controller until settlement. A result
 * or error becomes observable only while its controller remains un-aborted, so
 * loaders that ignore cancellation still cannot publish obsolete page data.
 *
 * Loading state belongs to the current controller; older completion cannot clear
 * it while another read is active. Scope disposal marks the helper unavailable
 * before aborting outstanding work, and subsequent runs return undefined. Explicit
 * cancellation permits later runs. A current failure aborts sibling work sharing
 * the same signal before propagating the error to the calling view.
 */
/**
 * useLatestRequest binds replaceable request ownership to the active Vue effect scope.
 * Call during setup or inside an effect scope. Consumers publish defined run results,
 * handle current errors, and may cancel a read while retaining the helper for retry.
 */
export function useLatestRequest() {
  // Scope-owned state separates visible loading, request cancellation, and permanent
  // disposal; only the active controller can end its corresponding loading state.
  const loading = ref(false)
  let current: AbortController | null = null
  let disposed = false
  /**
   * cancel aborts the current read and clears its visible loading state.
   * The helper remains reusable until scope disposal. A loader that completes after
   * cancellation returns no publishable result through its outstanding run promise.
   */
  function cancel() {
    current?.abort()
    current = null
    loading.value = false
  }

  onScopeDispose(() => {
    disposed = true
    cancel()
  })

  /**
   * run replaces the active request and returns its result while it owns the signal.
   * A cancelled or disposed run resolves undefined; a current rejection aborts the
   * signal and propagates to its caller. Loaders may use that signal for related reads.
   *
   * Ownership is tested again after awaiting the loader because cancellation can
   * arrive while an implementation ignores the signal. Finally clears loading only
   * for its own controller, leaving a newer run's loading indicator intact.
   */
  async function run<T>(fetch: (signal: AbortSignal) => Promise<T>): Promise<T | undefined> {
    if (disposed) return
    // Abort before replacing ownership so loaders sharing the previous signal stop
    // together; their late settlement cannot clear the new request's loading flag.
    current?.abort()
    const controller = new AbortController()
    current = controller
    loading.value = true
    try {
      const result = await fetch(controller.signal)
      if (!controller.signal.aborted) return result
    } catch (error) {
      if (!controller.signal.aborted) {
        controller.abort()
        throw error
      }
    } finally {
      if (current === controller) {
        current = null
        loading.value = false
      }
    }
  }

  return { loading, run, cancel }
}
