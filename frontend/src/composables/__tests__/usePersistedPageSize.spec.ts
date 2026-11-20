import { afterEach, describe, expect, it, vi } from 'vitest'

import { getPersistedPageSize, setPersistedPageSize } from '@/composables/usePersistedPageSize'

// usePersistedPageSize.spec.ts checks browser persistence and initialization of table
// page sizes through the public helpers. Saved choices take precedence over injected
// defaults, while unusable storage entries and browser storage failures leave the
// configured default available for list initialization.

describe('usePersistedPageSize', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    delete window.__APP_CONFIG__
  })

  // Existing browser storage contains a user choice and its legacy source marker.
  // The selected size remains authoritative even when runtime settings specify a larger
  // default for visitors who have not chosen a size.
  it('restores an existing user choice ahead of the system table default', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 1000,
      table_page_size_options: [20, 50, 1000],
    } as any
    localStorage.setItem('table-page-size', '50')
    localStorage.setItem('table-page-size-source', 'user')

    expect(getPersistedPageSize()).toBe(50)
  })

  // A visitor chooses thirty rows while the site supplies its legacy option list.
  // Reloading the helper must restore that choice despite either common system default.
  it.each([20, 50])('keeps thirty rows after reloading with default %s', async (defaultSize) => {
    window.__APP_CONFIG__ = {
      table_default_page_size: defaultSize,
      table_page_size_options: [10, 20, 50],
    } as any

    setPersistedPageSize(30)
    expect(localStorage.getItem('table-page-size')).toBe('30')

    // Reload the module while retaining browser storage to distinguish durable choices
    // from values held only in the preceding page's JavaScript module instance.
    vi.resetModules()
    const reloaded = await import('@/composables/usePersistedPageSize')

    expect(reloaded.getPersistedPageSize()).toBe(30)
  })

  // A browser has no saved size and receives a valid configured default.
  // Initial list requests use that default until the visitor selects another size.
  it('uses the configured default when no choice has been saved', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 50,
      table_page_size_options: [10, 20, 50],
    } as any

    expect(getPersistedPageSize()).toBe(50)
  })

  // Browser storage can contain malformed numbers or a size absent from current choices.
  // Such entries use the configured default so corrupted storage cannot produce an
  // unrelated remembered selection through rounding.
  it.each(['', 'invalid', '30px', '0', '-10', '30.5', '35', '1001', 'Infinity', 'NaN'])(
    'ignores invalid saved page size %j',
    (storedSize) => {
      window.__APP_CONFIG__ = {
        table_default_page_size: 20,
        table_page_size_options: [10, 20, 50],
      } as any
      localStorage.setItem('table-page-size', storedSize)

      expect(getPersistedPageSize()).toBe(20)
    },
  )

  // A caller passes an intermediate size to the shared persistence helper.
  // Storage and later list initialization must agree on the same supported selection.
  it('normalizes a requested size before storing it', () => {
    setPersistedPageSize(35)

    expect(localStorage.getItem('table-page-size')).toBe('50')
    expect(getPersistedPageSize()).toBe(50)
  })

  // Browser policy blocks reads of existing local storage for this page.
  // Table initialization must still return the configured default so data loading can
  // proceed with a valid page size.
  it('uses the configured default when storage reads fail', () => {
    window.__APP_CONFIG__ = { table_default_page_size: 50 } as any
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('Storage blocked', 'SecurityError')
    })

    expect(getPersistedPageSize()).toBe(50)
  })

  // Browser storage rejects a selected size because its quota is exhausted.
  // The setter must return normally, and a later read must still resolve the unsaved
  // preference through the configured default.
  it('tolerates storage write failures', () => {
    window.__APP_CONFIG__ = { table_default_page_size: 20 } as any
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('Storage full', 'QuotaExceededError')
    })

    expect(() => setPersistedPageSize(30)).not.toThrow()
    expect(getPersistedPageSize()).toBe(20)
  })
})
