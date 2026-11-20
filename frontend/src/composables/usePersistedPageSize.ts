import {
  getConfiguredTableDefaultPageSize,
  getConfiguredTablePageSizeOptions,
  normalizeTablePageSize,
} from '@/utils/tablePreferences'

// usePersistedPageSize.ts stores the browser's shared table page-size preference
// and restores it when a list is created. A supported saved selection takes
// precedence over the configured default, so refreshes and navigation retain
// the user's chosen number of rows. Tables still own their active page and
// reset it when their data or page size changes.
//
// Storage is optional: blocked access, corrupt values, and sizes removed from
// the available choices use the configured default. Reading the preference
// leaves storage untouched, so initialization cannot overwrite a user's
// selection. Writes complete before Pagination publishes its size-change event.

// PAGE_SIZE_STORAGE_KEY identifies the shared browser preference restored by list
// initialization and written by page-size controls across the application.
const PAGE_SIZE_STORAGE_KEY = 'table-page-size'

// getPersistedPageSize restores a supported browser preference or returns the
// normalized configured default. Call it when initializing table state, before
// loading the first page. Stored values are accepted only when present in the
// current choices; malformed and obsolete values follow the default path.
// Storage failures also use that path, keeping tables usable when browser
// persistence is unavailable. The fallback argument remains for existing
// callers; the configuration reader supplies a positive built-in default when
// runtime configuration is missing or invalid.
// @project-doc docs/architecture/frontend_ui_conventions.md#table_pagination
export function getPersistedPageSize(fallback = getConfiguredTableDefaultPageSize()): number {
  if (typeof window !== 'undefined') {
    try {
      const saved = window.localStorage.getItem(PAGE_SIZE_STORAGE_KEY)
      const size = Number(saved)
      if (saved !== null && getConfiguredTablePageSizeOptions().includes(size)) {
        return size
      }
    } catch {
      // Browser privacy settings can reject storage access; the configured
      // default keeps the first table request usable in that environment.
    }
  }

  return normalizeTablePageSize(getConfiguredTableDefaultPageSize() || fallback)
}

// setPersistedPageSize saves a normalized choice for subsequent list creation
// and browser reloads. Call it before notifying consumers of a size change;
// unavailable browser storage leaves the caller free to update its current list.
export function setPersistedPageSize(size: number): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(normalizeTablePageSize(size)))
  } catch {
    // A blocked or full storage area prevents persistence, while the caller
    // can still apply the chosen size to the current table.
  }
}
