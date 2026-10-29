import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import IdentityGroupsPanel from '../IdentityGroupsPanel.vue'
import * as api from '@/api/identityGroups'

/**
 * IdentityGroupsPanel.spec.ts exercises identity membership changes and rate editing
 * through mocked administration endpoints. Its cases distinguish an explicit zero rate
 * from inheritance, check refreshed member counts, and keep failed saves visible for
 * correction.
 */

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/identityGroups', () => ({
  listIdentityGroups: vi.fn(),
  listIdentityMembers: vi.fn(),
  changeIdentityMembers: vi.fn(),
  getIdentityRates: vi.fn(),
  updateIdentityRates: vi.fn(),
  identityName: (code: string) => code,
}))
const dialog = defineComponent({
  props: ['show'],
  template: '<section v-if="show"><slot/><slot name="footer"/></section>',
})
const createWrapper = () =>
  mount(IdentityGroupsPanel, { global: { stubs: { BaseDialog: dialog } } })
const click = async (wrapper: ReturnType<typeof createWrapper>, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text() === text)
  expect(button).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listIdentityGroups).mockResolvedValue([
    { code: 'ahhilai', name: 'AHHilAI', category: 'pricing', member_count: 0 },
  ])
  vi.mocked(api.listIdentityMembers).mockResolvedValue({
    items: [{ user_id: 1, username: 'member', email: 'member@test.local', identity_groups: [] }],
    total: 1,
  })
  vi.mocked(api.getIdentityRates).mockResolvedValue([
    { group_id: 6, name: '国模-Lv1', default_multiplier: 0.2, rate_multiplier: null },
  ])
  vi.mocked(api.changeIdentityMembers).mockResolvedValue(undefined)
  vi.mocked(api.updateIdentityRates).mockResolvedValue(undefined)
})
describe('IdentityGroupsPanel', () => {
  it('adds selected members and refreshes counts', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await click(wrapper, 'identityGroups.members')
    await click(wrapper, 'identityGroups.addMode')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await click(wrapper, 'identityGroups.add')
    expect(api.changeIdentityMembers).toHaveBeenCalledWith('ahhilai', [1], 'add')
    expect(api.listIdentityGroups).toHaveBeenCalledTimes(2)
  })
  it('persists zero and clears to inheritance', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await click(wrapper, 'identityGroups.rates')
    await wrapper.get('input[type="number"]').setValue('0')
    await click(wrapper, 'identityGroups.save')
    expect(api.updateIdentityRates).toHaveBeenLastCalledWith([{ group_id: 6, rate_multiplier: 0 }])
    await click(wrapper, 'identityGroups.rates')
    await click(wrapper, 'identityGroups.clear')
    await click(wrapper, 'identityGroups.save')
    expect(api.updateIdentityRates).toHaveBeenLastCalledWith([
      { group_id: 6, rate_multiplier: null },
    ])
  })
  it('keeps failed saves open and displays an error', async () => {
    vi.mocked(api.updateIdentityRates).mockRejectedValue(new Error('unavailable'))
    const wrapper = createWrapper()
    await flushPromises()
    await click(wrapper, 'identityGroups.rates')
    await click(wrapper, 'identityGroups.save')
    expect(wrapper.get('[role="alert"]').text()).toBe('identityGroups.failed')
    expect(wrapper.find('input[type="number"]').exists()).toBe(true)
  })
})
