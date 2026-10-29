import { shallowMount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppSidebar from '../AppSidebar.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { initTheme, setThemeMode, useTheme } from '@/composables/useTheme'

/**
 * AppSidebar.theme.spec.ts verifies sidebar theme controls against shared theme state
 * and persistence. Desktop, collapsed, mobile, and administrator variants are
 * exercised so toggling appearance updates the preference without unexpectedly closing
 * the mobile navigation drawer.
 */

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard', fullPath: '/dashboard' }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')
  const appStore = reactive({
    sidebarCollapsed: false,
    mobileOpen: false,
    sidebarScrollTop: 0,
    backendModeEnabled: false,
    cachedPublicSettings: {},
    setMobileOpen: vi.fn(),
  })
  const authStore = reactive({ isAdmin: false })
  return {
    useAppStore: () => appStore,
    useAuthStore: () => authStore,
    useOnboardingStore: () => ({ isCurrentStep: () => false }),
    useAdminSettingsStore: () => ({ fetch: vi.fn(), customMenuItems: [] }),
  }
})

vi.mock('@/composables/useBatchImageAccess', async () => {
  const { ref } = await import('vue')
  return {
    useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
  }
})

describe('AppSidebar theme toggle', () => {
  let wrapper: VueWrapper
  const appStore = useAppStore()
  const authStore = useAuthStore()
  const toggleSelector = '[data-testid="sidebar-theme-toggle"]'

  function mountSidebar() {
    return shallowMount(AppSidebar, { global: { stubs: { RouterLink: true } } })
  }

  beforeEach(() => {
    appStore.sidebarCollapsed = false
    appStore.mobileOpen = false
    Object.assign(authStore, { isAdmin: false })
    vi.clearAllMocks()
    vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(1)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    localStorage.setItem('theme', 'light')
    initTheme()
    wrapper = mountSidebar()
  })

  afterEach(() => {
    wrapper.unmount()
    setThemeMode('light')
    localStorage.removeItem('theme')
    document.documentElement.classList.remove('theme-switching')
    vi.restoreAllMocks()
  })

  it('switches both ways and saves the choice using the shared theme state', async () => {
    const button = wrapper.get(toggleSelector)
    expect(button.attributes('type')).toBe('button')
    expect(button.text()).toBe('nav.darkMode')
    expect(button.find('icon-stub').attributes('name')).toBe('moon')

    await button.trigger('click')
    expect(useTheme().isDark.value).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(button.text()).toBe('nav.lightMode')
    expect(button.find('icon-stub').attributes('name')).toBe('sun')

    await button.trigger('click')
    expect(useTheme().isDark.value).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
    expect(button.text()).toBe('nav.darkMode')
  })

  it('restores the saved theme when initialized and remounted', async () => {
    await wrapper.get(toggleSelector).trigger('click')
    wrapper.unmount()
    initTheme()
    wrapper = mountSidebar()
    expect(wrapper.get(toggleSelector).attributes('aria-label')).toBe('nav.lightMode')
    expect(useTheme().themeMode.value).toBe('dark')
  })

  it('keeps a labeled icon outside the scroll area when collapsed', async () => {
    appStore.sidebarCollapsed = true
    await nextTick()
    const button = wrapper.get(toggleSelector)
    expect(wrapper.get('nav').find(toggleSelector).exists()).toBe(false)
    expect(button.classes()).toContain('sidebar-link-collapsed')
    expect(button.attributes('aria-label')).toBe('nav.darkMode')
    expect(button.attributes('title')).toBe('nav.darkMode')
    expect(button.get('.sidebar-label').attributes('aria-hidden')).toBe('true')
    await button.trigger('click')
    expect(button.attributes('aria-label')).toBe('nav.lightMode')
  })

  it('reflects other theme controls and replaces system mode with an explicit preference', async () => {
    setThemeMode('dark')
    await nextTick()
    expect(wrapper.get(toggleSelector).text()).toBe('nav.lightMode')
    setThemeMode('system')
    await nextTick()
    const nextMode = useTheme().isDark.value ? 'light' : 'dark'
    await wrapper.get(toggleSelector).trigger('click')
    expect(useTheme().themeMode.value).toBe(nextMode)
    expect(localStorage.getItem('theme')).toBe(nextMode)
  })

  it('works in the mobile drawer without closing it', async () => {
    appStore.mobileOpen = true
    await nextTick()
    await wrapper.get(toggleSelector).trigger('click')
    expect(useTheme().isDark.value).toBe(true)
    expect(appStore.mobileOpen).toBe(true)
    expect(appStore.setMobileOpen).not.toHaveBeenCalled()
  })

  it('also provides the switch in the shared admin sidebar', async () => {
    Object.assign(authStore, { isAdmin: true })
    await nextTick()
    await wrapper.get(toggleSelector).trigger('click')
    expect(useTheme().isDark.value).toBe(true)
  })
})
