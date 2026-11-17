import { flushPromises, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileTotpCard from '../ProfileTotpCard.vue'

// ProfileTotpCard.lazy.spec.ts checks that TOTP enrollment resources load after
// the account owner opens setup. The real leaving-presence controller retains a
// closing enrollment until its transition ends, then permits a fresh instance
// for another attempt using the already resolved component module.

// API and lifecycle spies distinguish status retrieval, deferred module evaluation,
// and enrollment instance cleanup while the card handles its production events.
const { getStatus, moduleLoaded, mounted, unmounted } = vi.hoisted(() => ({
  getStatus: vi.fn().mockResolvedValue({
    enabled: false, // Account enrollment status.
    enabled_at: null, // Missing enrollment timestamp.
    feature_enabled: true, // Site enrollment availability.
  }),
  moduleLoaded: vi.fn(), // Deferred module evaluation.
  mounted: vi.fn(), // Enrollment instance creation.
  unmounted: vi.fn(), // Enrollment instance cleanup.
}))

vi.mock('@/api', () => ({ totpAPI: { getStatus } }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }), // Translation key passthrough.
}))
vi.mock('../TotpSetupModal.vue', () => {
  moduleLoaded()
  return {
    __esModule: true, // Unwrap the deferred module's default component.
    default: {
      name: 'TotpSetupModal', // Deferred component identity.
      props: ['show'], // Parent-owned visibility state.
      emits: ['close', 'success', 'after-leave'], // Enrollment lifecycle events.
      mounted, // Observed instance creation.
      unmounted, // Observed instance cleanup.
      template: '<section data-test="totp-setup" :data-visible="show" />', // Observable enrollment state.
    },
  }
})

describe('ProfileTotpCard setup loading', () => {
  // Viewing an unenrolled account must not load the QR encoder's owning dialog.
  // Enrollment opens lazily, remains present through exit, and remounts cleanly
  // after the shell reports that the previous transition completed.
  it('loads setup on demand and preserves exit cleanup before another enrollment', async () => {
    const wrapper = shallowMount(ProfileTotpCard, {
      global: { stubs: { TotpSetupModal: false } }, // Exercise the deferred enrollment loader.
    })
    await flushPromises()
    expect(getStatus).toHaveBeenCalledOnce()
    expect(moduleLoaded).not.toHaveBeenCalled()

    await wrapper.get('button').trigger('click')
    await vi.waitFor(() => expect(moduleLoaded).toHaveBeenCalledOnce())
    await flushPromises()
    const dialog = wrapper.getComponent({ name: 'TotpSetupModal' })
    expect(dialog.props('show')).toBe(true)
    expect(mounted).toHaveBeenCalledOnce()

    dialog.vm.$emit('close')
    await flushPromises()
    expect(dialog.props('show')).toBe(false)
    expect(unmounted).not.toHaveBeenCalled()
    dialog.vm.$emit('after-leave')
    await flushPromises()
    expect(unmounted).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-test="totp-setup"]').exists()).toBe(false)

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.getComponent({ name: 'TotpSetupModal' }).props('show')).toBe(true)
    expect(moduleLoaded).toHaveBeenCalledOnce()
    expect(mounted).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(unmounted).toHaveBeenCalledTimes(2)
  })
})
