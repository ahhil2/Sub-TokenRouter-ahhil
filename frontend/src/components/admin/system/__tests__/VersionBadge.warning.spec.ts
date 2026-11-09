import { shallowMount, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VersionBadge from '../VersionBadge.vue'

// VersionBadge.warning.spec.ts exercises administrator feedback when release
// discovery fails. Mocked application state supplies warnings and cached versions;
// rendering and refresh assertions verify that a failed check remains visible
// and cannot display the confirmed-current indicator.

// state supplies the discovery result consumed when each badge fixture mounts;
// fetchVersion records automatic checks and administrator refresh requests.
const state = vi.hoisted(() => ({
  currentVersion: '0.4.6',
  latestVersion: '0.4.8',
  hasUpdate: false,
  buildType: 'release',
  versionWarning: 'GitHub API returned 404',
  fetchVersion: vi.fn(),
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/i18n', () => ({ getLocale: () => 'en-US' }))
vi.mock('@/directives/segmented', () => ({ vSegmented: {} }))
vi.mock('@/stores', () => ({ useAuthStore: () => ({ isAdmin: true }), useAppStore: () => state }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }),
}))
vi.mock('@/api/admin/system', () => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  rollback: vi.fn(),
  getRollbackVersions: vi.fn(),
}))
enableAutoUnmount(afterEach)

describe('VersionBadge discovery warnings', () => {
  beforeEach(() => {
    state.hasUpdate = false
    state.fetchVersion.mockReset()
  })

  // Discovery failure has no trustworthy latest version. The badge must show
  // the warning, suppress the current-version claim, and support a forced refresh.
  it('shows a failed check and offers refresh', async () => {
    const wrapper = shallowMount(VersionBadge, { global: { renderStubDefaultSlot: true } })
    expect(wrapper.get('button').attributes('title')).toBe('version.checkFailed')
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain('GitHub API returned 404')
    expect(wrapper.text()).not.toContain('version.upToDate')
    await wrapper.get('button[title="version.refresh"]').trigger('click')
    expect(state.fetchVersion).toHaveBeenLastCalledWith(true)
  })

  // A remote failure can accompany usable cached release information. The
  // warning remains visible while the previously discovered update stays actionable.
  it('retains an available cached update alongside its warning', async () => {
    state.hasUpdate = true
    const wrapper = shallowMount(VersionBadge, { global: { renderStubDefaultSlot: true } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.upToDate')
  })
})
