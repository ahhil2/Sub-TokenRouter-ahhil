import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProviderSortDialog from '../ProviderSortDialog.vue'
import { providersAPI } from '@/api/admin/providers'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/providers', () => ({ providersAPI: { list: vi.fn(), updateDisplayOrder: vi.fn() } }))
const notifications = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => notifications }))

const create = async () => {
  const wrapper = mount(ProviderSortDialog, {
    props: { show: true },
    global: { stubs: {
      BaseDialog: { template: '<section><slot /><slot name="footer" /></section>' },
      VueDraggable: { template: '<div><slot /></div>' },
      Icon: true,
    } },
  })
  await flushPromises()
  return wrapper
}

describe('ProviderSortDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(providersAPI.list).mockResolvedValue({ items: [{ id: 1, name: 'First' }, { id: 2, name: 'Second' }], total: 2 } as any)
    vi.mocked(providersAPI.updateDisplayOrder).mockResolvedValue()
  })

  it('saves a reordered list and supports cancel without writes', async () => {
    const wrapper = await create()
    await wrapper.get('[data-provider-id="2"] [aria-label="admin.providers.moveUp"]').trigger('click')
    expect(wrapper.findAll('[data-provider-id]').map(item => item.attributes('data-provider-id'))).toEqual(['2', '1'])
    await wrapper.get('[data-testid="provider-sort-save"]').trigger('click')
    await flushPromises()
    expect(providersAPI.updateDisplayOrder).toHaveBeenCalledWith([2, 1])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
    vi.mocked(providersAPI.updateDisplayOrder).mockClear()
    const cancelled = await create()
    await cancelled.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    expect(providersAPI.updateDisplayOrder).not.toHaveBeenCalled()
    expect(cancelled.emitted('close')).toHaveLength(1)
    cancelled.unmount()
  })

  it('loads every page before saving and retains the dialog after a failed save', async () => {
    vi.mocked(providersAPI.list)
      .mockResolvedValueOnce({ items: [{ id: 3, name: 'Third' }], total: 2 } as any)
      .mockResolvedValueOnce({ items: [{ id: 4, name: 'Fourth' }], total: 2 } as any)
    vi.mocked(providersAPI.updateDisplayOrder).mockRejectedValueOnce(new Error('offline'))
    const wrapper = await create()
    expect(providersAPI.list).toHaveBeenNthCalledWith(2, 2, 100, expect.objectContaining({ sort_by: 'display_order' }), expect.anything())
    await wrapper.get('[data-testid="provider-sort-save"]').trigger('click')
    await flushPromises()
    expect(providersAPI.updateDisplayOrder).toHaveBeenCalledWith([3, 4])
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(notifications.showError).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="provider-sort-save"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('disables saving an incomplete list', async () => {
    vi.mocked(providersAPI.list).mockRejectedValueOnce(new Error('offline'))
    const wrapper = await create()
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="provider-sort-save"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
