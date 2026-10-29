import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderGroupsCell from '../ProviderGroupsCell.vue'
import type { Group } from '@/api/groups.types'

/**
 * ProviderGroupsCell.spec.ts checks the provider table's full group list and
 * empty-assignment placeholder. Badge stubs expose forwarded names, titles, and
 * rate visibility so the tests can inspect column behavior independently of
 * badge rendering details.
 */

describe('ProviderGroupsCell', () => {
  it('shows every group vertically with wider badges and full-name titles', () => {
    const groups = Array.from({ length: 6 }, (_, index) => ({
      id: index + 1,
      name: `Claude-long-provider-group-${index + 1}`,
      display_brand: 'anthropic',
      rate_multiplier: 1,
    })) as Group[]
    const wrapper = mount(ProviderGroupsCell, {
      props: { groups },
      global: { stubs: { GroupBadge: true } },
    })

    const list = wrapper.get('[data-testid="provider-groups"]')
    expect(list.classes()).toContain('flex-col')
    expect(list.classes()).toContain('w-56')
    const badges = wrapper.findAllComponents({ name: 'GroupBadge' })
    expect(badges).toHaveLength(6)
    badges.forEach((badge, index) => {
      expect(badge.props('name')).toBe(groups[index].name)
      expect(badge.attributes('title')).toBe(groups[index].name)
      expect(badge.classes()).toContain('max-w-full')
      expect(badge.props('showRate')).toBe(false)
    })
    expect(wrapper.find('button').exists()).toBe(false)
    wrapper.unmount()
  })

  it.each([{ groups: null }, { groups: undefined }, { groups: [] }])(
    'shows a placeholder for empty groups: $groups',
    ({ groups }) => {
      const wrapper = mount(ProviderGroupsCell, { props: { groups } })
      expect(wrapper.text()).toBe('-')
      wrapper.unmount()
    },
  )
})
