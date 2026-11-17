import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { useTableLoader } from '@/composables/useTableLoader'
import type { BasePaginationResponse } from '@/api/common.types'

// useTableLoader.lifecycle.spec.ts exercises search scheduling through the real
// VueUse debounce function and a mounted Vue component. Controlled timers expose
// searches still waiting when the component leaves, while deferred fetch results
// verify that unmount continues to abort transport and suppress late publication.

// Row identifies the fetched records published by the test loader.
interface Row {
  id: number // fetched row identity
}

// Filters carries the search term passed to the fetch function.
interface Filters {
  search: string // current search input
}

// LoaderOptions supplies the real composable's typed transport and timing options.
type LoaderOptions = Parameters<typeof useTableLoader<Row, Filters>>[0]

// response supplies one successful page so lifecycle tests can observe publication
// independently of how their fetch transport handles cancellation.
const response: BasePaginationResponse<Row> = {
  items: [{ id: 1 }], // fetched page rows
  total: 1, // total matching records
  pages: 1, // available result pages
  page: 1, // returned page index
  page_size: 20, // requested page capacity
}

// mountLoader creates a component that owns the composable's unmount hook.
// Tests control the supplied transport and retain the returned loader to inspect
// pending work after unmount; automatic teardown releases wrappers after each case.
function mountLoader(options: LoaderOptions) {
  let loader!: ReturnType<typeof useTableLoader<Row, Filters>>
  const wrapper = mount(
    defineComponent({
      // setup installs the real loader within the component's lifecycle scope.
      setup() {
        loader = useTableLoader<Row, Filters>(options)
        return () => null
      },
    }),
  )
  return { loader, wrapper }
}

enableAutoUnmount(afterEach)

describe('table loader component lifecycle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // A search remains inside its debounce delay when the component leaves. Its
  // later callback and direct reload attempts must not dispatch new transport work.
  it('skips a debounced search and later loads after unmount', async () => {
    const fetchFn = vi.fn<LoaderOptions['fetchFn']>().mockResolvedValue(response)
    const { loader, wrapper } = mountLoader({ fetchFn, debounceMs: 300 })
    const pendingSearch = loader.debouncedReload()
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(300)
    await pendingSearch
    await loader.load()
    expect(fetchFn).not.toHaveBeenCalled()
    expect(loader.loading.value).toBe(false)
  })

  // Several edits arrive before a mounted table's debounce delay expires. The
  // final search dispatches once with current filters and publishes the returned page.
  it('coalesces mounted searches and publishes the latest filter results', async () => {
    const fetchFn = vi.fn<LoaderOptions['fetchFn']>().mockResolvedValue(response)
    const { loader } = mountLoader({ fetchFn, initialParams: { search: '' }, debounceMs: 300 })
    loader.pagination.page = 3
    loader.params.search = 'first'
    const earlierSearch = loader.debouncedReload()
    await vi.advanceTimersByTimeAsync(100)
    loader.params.search = 'latest'
    const latestSearch = loader.debouncedReload()
    await vi.advanceTimersByTimeAsync(299)
    expect(fetchFn).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    await Promise.all([earlierSearch, latestSearch])
    expect(fetchFn).toHaveBeenCalledExactlyOnceWith(
      1,
      expect.any(Number),
      { search: 'latest' },
      { signal: expect.any(AbortSignal) },
    )
    expect(loader.items.value).toEqual(response.items)
  })

  // A fetch starts before unmount and its transport later resolves despite abort.
  // The original signal is aborted and the late response cannot publish table rows.
  it('aborts an active request and ignores its late successful response', async () => {
    let resolveResponse!: (value: BasePaginationResponse<Row>) => void
    const pendingResponse = new Promise<BasePaginationResponse<Row>>((resolve) => {
      resolveResponse = resolve
    })
    const fetchFn = vi.fn<LoaderOptions['fetchFn']>().mockReturnValue(pendingResponse)
    const { loader, wrapper } = mountLoader({ fetchFn })
    const request = loader.load()
    const signal = fetchFn.mock.calls[0]![3]!.signal
    expect(signal?.aborted).toBe(false)
    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    resolveResponse(response)
    await request
    expect(loader.items.value).toEqual([])
    expect(loader.pagination.total).toBe(0)
    expect(loader.loading.value).toBe(false)
  })
})
