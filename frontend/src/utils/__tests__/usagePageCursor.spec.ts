import { describe, expect, it } from 'vitest'
import { createUsagePageCursor, usagePageCursor } from '../usagePageCursor'

// usagePageCursor.spec.ts checks continuity when navigation, filters, or ordering change.
// The utility receives server timestamp strings and numeric record identifiers;
// these tests check eligibility for reuse without requiring a browser or database.
// Unsafe identifiers and unsupported sorts retain the offset fallback.
describe('usage list continuation', () => {
  // A displayed page supplies a fractional timestamp boundary for its immediate successor.
  // Changes to selection or navigation must discard that boundary and preserve precision.
  it('only reuses a boundary for an adjacent page with the same query', () => {
    const state = createUsagePageCursor()
    const query = { page: 1, page_size: 1, sort_by: 'created_at', provider_id: 2 }
    const row = { id: 17, created_at: '2026-11-27T00:00:00.000009Z' }
    state.remember(query, [row])
    expect(state.next({ ...query, page: 2 })).toBe(`${row.created_at}|17`)
    for (const changed of [
      { page: 3 },
      { page: 2, provider_id: 3 },
      { page: 2, page_size: 2 },
      { page: 2, sort_order: 'asc' as const },
    ]) {
      expect(state.next({ ...query, ...changed })).toBeUndefined()
    }
    expect(usagePageCursor('id', row)).toBe('17')
    expect(usagePageCursor('model', row)).toBeUndefined()
    expect(usagePageCursor('id', { ...row, id: Number.MAX_SAFE_INTEGER + 1 })).toBeUndefined()
  })
})
