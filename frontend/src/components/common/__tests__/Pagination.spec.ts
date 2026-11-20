import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import Pagination from '../Pagination.vue'
import Select from '../Select.vue'

/**
 * Pagination.spec.ts checks page navigation and persisted size selections through
 * component events. Desktop and mobile jump controls exercise bounded navigation,
 * while the size selector verifies available choices and browser persistence across
 * component remounts with injected application defaults.
 */

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

afterEach(() => {
  localStorage.clear()
  delete window.__APP_CONFIG__
})

describe('Pagination page sizes', () => {
  // A legacy site setting omits thirty and one hundred rows from its option list.
  // The shared selector must still present every standard size to the visitor.
  it('exposes all standard page sizes under legacy table settings', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 20,
      table_page_size_options: [10, 20, 50],
    } as any
    const wrapper = mount(Pagination, {
      props: { page: 1, total: 120, pageSize: 20 },
      global: { stubs: { Icon: true, Select: true } },
    })

    expect(wrapper.getComponent(Select).props('options')).toEqual([
      { value: 10, label: '10' },
      { value: 20, label: '20' },
      { value: 30, label: '30' },
      { value: 50, label: '50' },
      { value: 100, label: '100' },
    ])
    wrapper.unmount()
  })

  // A table endpoint accepts at most one hundred rows while site settings allow larger
  // pages. Its supplied choices must constrain the selector, and selecting thirty rows
  // must still persist the visitor's preference.
  it('respects endpoint-specific choices while persisting the selected size', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 20,
      table_page_size_options: [10, 20, 50, 200, 1000],
    } as any
    const wrapper = mount(Pagination, {
      props: {
        page: 1,
        total: 120,
        pageSize: 20,
        pageSizeOptions: [10, 20, 30, 50, 100],
      },
      global: { stubs: { Icon: true, Select: true } },
    })

    const selector = wrapper.getComponent(Select)
    expect(selector.props('options')).toEqual([
      { value: 10, label: '10' },
      { value: 20, label: '20' },
      { value: 30, label: '30' },
      { value: 50, label: '50' },
      { value: 100, label: '100' },
    ])

    selector.vm.$emit('update:modelValue', 30)

    expect(wrapper.emitted('update:pageSize')).toEqual([[30]])
    expect(localStorage.getItem('table-page-size')).toBe('30')
    wrapper.unmount()
  })

  // A visitor changes the page size while the configured default remains twenty.
  // The parent's change listener must observe the saved selection, and remounting the
  // control with the shared preference must retain that selection.
  it.each([30, 100])('saves %s before notifying the parent and restores it on remount', (size) => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 20,
      table_page_size_options: [10, 20, 50],
    } as any
    // onPageSizeChange verifies that consumers can read the saved selection immediately.
    const onPageSizeChange = vi.fn((selectedSize: number) => {
      expect(getPersistedPageSize()).toBe(selectedSize)
    })
    const wrapper = mount(Pagination, {
      props: {
        page: 1,
        total: 120,
        pageSize: getPersistedPageSize(),
        'onUpdate:pageSize': onPageSizeChange,
      },
      global: { stubs: { Icon: true, Select: true } },
    })

    wrapper.getComponent(Select).vm.$emit('update:modelValue', size)

    expect(onPageSizeChange).toHaveBeenCalledExactlyOnceWith(size)
    expect(wrapper.emitted('update:pageSize')).toEqual([[size]])
    expect(localStorage.getItem('table-page-size')).toBe(String(size))
    wrapper.unmount()

    const restored = mount(Pagination, {
      props: { page: 1, total: 120, pageSize: getPersistedPageSize() },
      global: { stubs: { Icon: true, Select: true } },
    })

    expect(restored.getComponent(Select).props('modelValue')).toBe(size)
    restored.unmount()
  })
})

describe('Pagination jumps', () => {
  const create = (total = 400) =>
    mount(Pagination, {
      props: { page: 1, total, pageSize: 20, showJump: true },
      global: { stubs: { Icon: true, Select: true } },
    })

  it.each([
    ['7', 7],
    ['999', 20],
    ['2', 2],
  ])('jumps from numeric input %s to %s', async (input, expected) => {
    const wrapper = create()
    await wrapper.get('.pagination-desktop input').setValue(input)
    await wrapper.get('.pagination-desktop input').trigger('keyup.enter')
    expect(wrapper.emitted('update:page')).toEqual([[expected]])
    wrapper.unmount()
  })

  it.each(['', '1.5', '0', '-5'])(
    'does not emit an invalid or unchanged page for %s',
    async (input) => {
      const wrapper = create()
      await wrapper.get('.pagination-desktop input').setValue(input)
      await wrapper.get('.pagination-desktop .pagination-jump-button').trigger('click')
      expect(wrapper.emitted('update:page')).toBeUndefined()
      wrapper.unmount()
    },
  )

  it('supports the mobile jump form and never jumps beyond an empty list', async () => {
    const wrapper = create()
    await wrapper.get('.pagination-mobile input').setValue('8')
    await wrapper.get('.pagination-mobile form').trigger('submit')
    expect(wrapper.emitted('update:page')).toEqual([[8]])
    await wrapper.setProps({ total: 0 })
    await wrapper.get('.pagination-desktop input').setValue('8')
    await wrapper.get('.pagination-desktop input').trigger('keyup.enter')
    expect(wrapper.emitted('update:page')).toEqual([[8]])
    wrapper.unmount()
  })
})
