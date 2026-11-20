import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsErrorDetailsModal from '../OpsErrorDetailsModal.vue'
import OpsErrorLogTable from '../OpsErrorLogTable.vue'

// OpsErrorDetailsModal.spec.ts verifies saved pagination sizes at the request
// and upstream error endpoints. API doubles capture query parameters while
// mounted dialog openings exercise preference restoration and the local limit
// used by the parent-owned error table.

// Endpoint doubles capture the selected error category and return an empty page
// so the assertions can focus on request limits and retained browser preferences.
const { listRequestErrors, listUpstreamErrors } = vi.hoisted(() => ({
  listRequestErrors: vi.fn(),
  listUpstreamErrors: vi.fn(),
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { listRequestErrors, listUpstreamErrors },
  default: { listRequestErrors, listUpstreamErrors },
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('OpsErrorDetailsModal page-size restoration', () => {
  beforeEach(() => {
    localStorage.clear()
    listRequestErrors.mockReset().mockResolvedValue({ items: [], total: 0 })
    listUpstreamErrors.mockReset().mockResolvedValue({ items: [], total: 0 })
  })

  afterEach(() => {
    localStorage.clear()
    delete window.__APP_CONFIG__
  })

  // A saved thousand-row preference exceeds both error endpoint limits. Opening
  // either category must select a supported size and leave browser storage
  // available to other tables that accept the larger preference.
  it.each(['request', 'upstream'] as const)(
    'bounds the saved size for %s errors',
    async (errorType) => {
      window.__APP_CONFIG__ = { table_page_size_options: [500, 1000] } as any
      localStorage.setItem('table-page-size', '1000')
      const wrapper = mount(OpsErrorDetailsModal, {
        props: { show: false, timeRange: '1h', errorType },
        global: {
          stubs: {
            BaseDialog: { template: '<div><slot /></div>' },
            OpsErrorLogTable: true,
            Select: true,
            IpGeoBatchToolbar: true,
          },
        },
      })
      await wrapper.setProps({ show: true })
      await flushPromises()
      const request = errorType === 'request' ? listRequestErrors : listUpstreamErrors
      expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 500 }))
      expect(wrapper.getComponent(OpsErrorLogTable).props('pageSize')).toBe(500)
      await wrapper.setProps({ show: false })
      await wrapper.setProps({ show: true })
      await flushPromises()
      expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 500 }))
      expect(localStorage.getItem('table-page-size')).toBe('1000')
      wrapper.unmount()
    },
  )
})
