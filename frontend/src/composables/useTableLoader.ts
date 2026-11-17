import { ref, reactive, onUnmounted, toRaw } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { SEARCH_DEBOUNCE_MS } from '@/constants/ui'
import type { BasePaginationResponse, FetchOptions } from '@/api/common.types'
import { getPersistedPageSize, setPersistedPageSize } from './usePersistedPageSize'

// useTableLoader.ts loads paginated table data with reactive filters, debounced reloads,
// and request cancellation. Each accepted load aborts the previous controller and passes
// the current page and raw parameter object to the caller's fetch function. Responses
// publish rows and totals while their request signal remains active; transports should
// honor that signal to release their network work promptly.
//
// Controller identity decides which completion may clear the loading flag. Unmount marks
// the loader disposed before aborting the current request. A pending debounce callback
// can still finish its delay, but the disposed loader declines its fetch. Page changes
// retain filters and reload from their chosen page while the component remains mounted.

// PaginationState holds the requested page and the server's result totals.
interface PaginationState {
  // page and page_size select the next fetch, while total and pages describe the latest
  // published response; changing pages retains the same filter object.
  page: number // one-based requested page number
  page_size: number // requested rows per page
  total: number // server-reported matching row count
  pages: number // server-reported result page count
}

// TableLoaderOptions supplies paginated fetching, initial filters, and optional timing
// defaults.
interface TableLoaderOptions<T, P> {
  // fetchFn receives pagination, a raw filter object, and the current AbortSignal. It should
  // honor cancellation to stop superseded transport work; the loader checks the signal
  // before publishing returned rows or reporting a failure.
  fetchFn: (
    page: number,
    pageSize: number,
    params: P,
    options?: FetchOptions,
  ) => Promise<BasePaginationResponse<T>>
  // initialParams is shallow-copied at construction; nested values remain shared with the
  // supplied object and its owners.
  initialParams?: P // initial shallow-copied filter values
  // pageSize overrides the configured runtime default for this table instance when it is
  // supplied, including zero.
  pageSize?: number // initial rows per page
  // debounceMs controls delayed reload dispatch in milliseconds and defaults to
  // SEARCH_DEBOUNCE_MS when omitted from the options.
  debounceMs?: number // reload debounce delay milliseconds
}

// useTableLoader creates rows, filters, pagination, and loading actions for a component.
// Invoke it during setup so unmount stops future fetching and aborts the current request.
// Initial filters are shallow-copied and page size defaults to runtime table preferences.
//
// Each accepted load replaces the controller before awaiting results. The request signal
// guards result publication, while controller identity guards loading completion. Debounced
// reload resets the page when it executes. Its timer may finish after unmount, but the
// disposed loader resolves that invocation without allocating a new request controller or
// calling the supplied fetch function.
export function useTableLoader<T, P extends Record<string, any>>(
  options: TableLoaderOptions<T, P>,
) {
  const { fetchFn, initialParams, pageSize, debounceMs = SEARCH_DEBOUNCE_MS } = options

  // items and pagination publish fetch results, params holds reactive filters, and loading
  // follows the controller currently owned by this table instance.
  const items = ref<T[]>([])
  const loading = ref(false)
  const params = reactive<P>({ ...(initialParams || {}) } as P)
  const pagination = reactive<PaginationState>({
    page: 1,
    page_size: pageSize ?? getPersistedPageSize(),
    total: 0,
    pages: 0,
  })

  // abortController owns cancellation for the current request; disposed prevents pending
  // callbacks from starting another request after the component's unmount cleanup.
  let abortController: AbortController | null = null
  let disposed = false

  // isAbortError recognizes DOM and Axios-style cancellation names and codes.
  const isAbortError = (error: any) => {
    return (
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED' ||
      error?.name === 'CanceledError'
    )
  }

  // load requests the current page and filters after aborting the prior controller, or
  // resolves immediately when the component has unmounted. It passes the raw reactive
  // parameter object, so fetch implementations decide when to snapshot or encode those
  // values. Cancellation-shaped errors are absorbed; current failures are logged and
  // rethrown.
  //
  // Rows and pagination totals publish after success only while the request signal remains
  // active, including when a transport ignores cancellation. Controller identity prevents
  // an older finally block from clearing a newer request's loading indicator.
  const load = async () => {
    // A queued debounce can execute after unmount; reject it before allocating a
    // controller or starting fetch work for a component that has already left.
    if (disposed) return

    // Abort prior transport work before replacing the controller so only the new request
    // owns completion of the loading indicator.
    if (abortController) {
      abortController.abort()
    }
    const currentController = new AbortController()
    abortController = currentController
    loading.value = true

    try {
      const response = await fetchFn(pagination.page, pagination.page_size, toRaw(params) as P, {
        signal: currentController.signal,
      })

      if (currentController.signal.aborted) return
      items.value = response.items || []
      pagination.total = response.total || 0
      pagination.pages = response.pages || 0
    } catch (error) {
      // Superseded loaders may ignore AbortSignal and still reject. Suppress those
      // obsolete errors while keeping failures from the current request observable.
      if (!currentController.signal.aborted && !isAbortError(error)) {
        console.error('Table load error:', error)
        throw error
      }
    } finally {
      // An aborted request can settle after a replacement starts, so its finally block
      // must leave the replacement loading flag active.
      if (abortController === currentController) {
        loading.value = false
      }
    }
  }

  // reload resets pagination to the first page and returns the load promise. Callers
  // awaiting it observe ordinary fetch failures, while recognized cancellation errors
  // resolve through load's cancellation handling.
  const reload = () => {
    pagination.page = 1
    return load()
  }

  const debouncedReload = useDebounceFn(reload, debounceMs)

  // handlePageChange clamps the requested page to available pagination bounds and starts
  // loading. It returns immediately rather than exposing the load promise, so callers
  // cannot await fetch failures through this handler.
  const handlePageChange = (page: number) => {
    // Clamp navigation against at least one available page.
    const validPage = Math.max(1, Math.min(page, pagination.pages || 1))
    pagination.page = validPage
    load()
  }

  // handlePageSizeChange applies a new page size, resets to page one, and starts loading.
  // It invokes the compatibility preference setter before loading and returns before the
  // fetch promise settles.
  const handlePageSizeChange = (size: number) => {
    pagination.page_size = size
    pagination.page = 1
    setPersistedPageSize(size)
    load()
  }

  // Close request admission before aborting the active transport so a delayed search
  // cannot create another controller after the component has released its current work.
  onUnmounted(() => {
    disposed = true
    abortController?.abort()
  })

  return {
    items,
    loading,
    params,
    pagination,
    load,
    reload,
    debouncedReload,
    handlePageChange,
    handlePageSizeChange,
  }
}
