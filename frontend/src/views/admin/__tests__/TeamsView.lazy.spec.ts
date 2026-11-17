import { flushPromises, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import TeamsView from '../TeamsView.vue'
import type { AdminTeam } from '@/api/admin/teams'

// TeamsView.lazy.spec.ts checks when the team list admits its statistics module.
// A deferred module substitute records loading and component lifetime while the
// real page handles action events, dialog closure, and a second team selection.

// Module and lifecycle spies distinguish downloading the statistics code from
// mounting its retained dialog instance after the administrator selects statistics.
const { list, moduleLoaded, mounted, unmounted } = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue([]), // Team list response.
  moduleLoaded: vi.fn(), // Deferred module evaluation.
  mounted: vi.fn(), // Statistics instance creation.
  unmounted: vi.fn(), // Statistics instance cleanup.
}))

vi.mock('@/api/admin', () => ({ adminAPI: { teams: { list } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({}), // Unused authentication controller.
  isStepUpCancelled: () => false, // Unused cancellation predicate.
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }), // Translation key passthrough.
}))
vi.mock('@/components/admin/team/TeamStatsModal.vue', () => {
  moduleLoaded()
  return {
    __esModule: true, // Unwrap the deferred module's default component.
    default: {
      name: 'TeamStatsModal', // Deferred component identity.
      props: ['show', 'team'], // Parent-selected statistics state.
      emits: ['close'], // Parent-owned dismissal event.
      mounted, // Observed instance creation.
      unmounted, // Observed instance cleanup.
      template: '<section data-test="statistics" :data-visible="show" />', // Observable dialog state.
    },
  }
})

describe('TeamsView statistics loading', () => {
  // Visiting the list must leave chart code unloaded until statistics is requested.
  // Closing and reopening for another team must reuse the instance that observes
  // visibility changes and invalidates pending statistics requests.
  it('loads statistics on demand and retains the dialog across closure', async () => {
    const wrapper = shallowMount(TeamsView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' }, // Keep page-owned overlays mounted.
          TeamStatsModal: false, // Exercise the deferred component loader.
        },
      },
    })
    await flushPromises()
    expect(list).toHaveBeenCalledOnce()
    expect(moduleLoaded).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="statistics"]').exists()).toBe(false)

    const firstTeam = { id: 9, name: 'First team' } as AdminTeam
    wrapper.getComponent({ name: 'TeamActionMenu' }).vm.$emit('statistics', firstTeam)
    await vi.waitFor(() => expect(moduleLoaded).toHaveBeenCalledOnce())
    await flushPromises()
    expect(mounted).toHaveBeenCalledOnce()
    const dialog = wrapper.getComponent({ name: 'TeamStatsModal' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('team')).toEqual(firstTeam)

    dialog.vm.$emit('close')
    await flushPromises()
    expect(dialog.props('show')).toBe(false)
    expect(dialog.props('team')).toBeNull()
    expect(unmounted).not.toHaveBeenCalled()

    const secondTeam = { id: 10, name: 'Second team' } as AdminTeam
    wrapper.getComponent({ name: 'TeamActionMenu' }).vm.$emit('statistics', secondTeam)
    await flushPromises()
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('team')).toEqual(secondTeam)
    expect(moduleLoaded).toHaveBeenCalledOnce()
    expect(mounted).toHaveBeenCalledOnce()
    wrapper.unmount()
    expect(unmounted).toHaveBeenCalledOnce()
  })
})
