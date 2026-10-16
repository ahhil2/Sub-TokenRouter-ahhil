import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import RechargeBreakdown from '../RechargeBreakdown.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: ref('en') }) }))
describe('RechargeBreakdown', () => {
  it('shows base, bonus and total independently', () => {
    const wrapper = mount(RechargeBreakdown, { props: { snapshot: { identity_code: 'bonus_10', base_amount: 100, bonus_rate: 0.1, bonus_amount: 10, credited_amount: 110 } } })
    expect(wrapper.findAll('dd').map(item => item.text())).toEqual(['100', '10', '110'])
    expect(wrapper.text()).toContain('10%')
  })
  it('hides historical empty snapshots', () => {
    expect(mount(RechargeBreakdown).find('dl').exists()).toBe(false)
  })
})
