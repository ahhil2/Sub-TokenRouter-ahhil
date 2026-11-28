import type { UsageLog, UsageQueryParams } from '@/api/usage.types'

// usagePageCursor.ts preserves the server's timestamp precision while carrying the last
// displayed row into the next page of the same ordered selection.
// The original timestamp text and record ID together match the server's tuple
// ordering, including records created within the same millisecond. Unsupported
// sorts and unsafe numeric identifiers fall back to ordinary offset pagination.
//
// Each view owns one remembered boundary and records it only after displaying a
// successful response. A successor may reuse that boundary when every query
// property except the page number is identical. Page jumps, backward navigation,
// and changed filters or page sizes therefore use the server's existing offset
// path instead of applying an unrelated record's boundary.

// usagePageCursor encodes a row for supported ordering or returns undefined.
// Callers preserve the server's timestamp text rather than converting it through
// Date, which would discard fractional precision needed to identify the boundary.
export function usagePageCursor(
  sort: string | undefined,
  row?: Pick<UsageLog, 'id' | 'created_at'>,
) {
  if (!row || !Number.isSafeInteger(row.id) || row.id <= 0) return undefined
  if (sort === 'id') return String(row.id)
  if (sort === 'created_at' && row.created_at) return `${row.created_at}|${row.id}`
  return undefined
}

// createUsagePageCursor remembers the last successfully displayed page for one view.
// Callers build queries consistently and call remember only for responses they
// actually publish. Only an immediate successor with the same selection may reuse
// its last row, because another page or filter set has no verified relationship to
// that boundary. Missing rows and unsupported sorts yield undefined so ordinary
// offset pagination remains available without retaining a cursor for every page.
export function createUsagePageCursor() {
  let boundary: { page: number; selection: string; cursor?: string } | undefined
  // selection identifies every query property that must match across adjacent pages.
  function selection(query: UsageQueryParams) {
    return JSON.stringify({ ...query, page: undefined })
  }
  return {
    // next returns a boundary only for the remembered page's matching successor.
    next(query: UsageQueryParams) {
      if (boundary && query.page === boundary.page + 1 && selection(query) === boundary.selection) {
        return boundary.cursor
      }
    },
    // remember replaces this view's boundary after its caller displays a successful page.
    remember(query: UsageQueryParams, items: Pick<UsageLog, 'id' | 'created_at'>[]) {
      boundary = {
        page: query.page ?? 1,
        selection: selection(query),
        cursor: usagePageCursor(query.sort_by, items[items.length - 1]),
      }
    },
  }
}
