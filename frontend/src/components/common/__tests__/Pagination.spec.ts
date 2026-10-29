import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Pagination from '../Pagination.vue'

/**
 * Pagination.spec.ts checks direct page navigation and invalid jump input on desktop
 * and mobile forms. Component events expose the requested page, including bounded
 * navigation and empty-list behavior, without requiring a data-loading parent view.
 */

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

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
