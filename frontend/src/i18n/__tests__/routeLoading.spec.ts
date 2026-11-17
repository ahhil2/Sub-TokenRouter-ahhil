import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { adminRoutes } from '@/router/routes/admin'
import { publicRoutes } from '@/router/routes/public'
import en from '../locales/en'
import zh from '../locales/zh'
import enBase from '../locales/en/base'
import zhBase from '../locales/zh/base'

/**
 * routeLoading.spec.ts verifies authentication, ordinary-page, and administrator
 * dictionaries across navigation and language changes. Shared router and store
 * substitutes isolate locale publication from API calls while real catalog imports
 * exercise merging, concurrent demand, and explicit complete-catalog loading.
 */

// Locale publication reads route and account metadata after loading. These substitutes
// keep that stage local while catalog requests still use their production loaders.
vi.mock('@/router', () => ({ default: { currentRoute: { value: { path: '/dashboard' } } } }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Localized page' }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ siteName: 'TokenRouter', cachedPublicSettings: {} }),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false }) }))
vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

describe('页面语言目录加载', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
  })

  // Each authentication or legal destination loads with a selected non-default locale. Both
  // current and default catalogs must contain complete auth text while console namespaces
  // stay unloaded.
  it.each(['/login', '/register', '/email-verify', '/auth/callback', '/legal/terms'])(
    '%s 准备完整认证文案和英文回退文案',
    async (path) => {
      const { i18n, loadRouteLocaleMessages } = await import('../index')
      i18n.global.locale.value = 'zh-Hans'
      await loadRouteLocaleMessages(path)

      for (const [locale, original] of [
        ['en', en],
        ['zh-Hans', zh],
      ] as const) {
        const messages = i18n.global.getLocaleMessage(locale)
        expect(messages.auth).toEqual(original.auth)
        expect(messages.common).toEqual(original.common)
        expect(messages.home).toEqual(original.home)
        expect(messages.legal).toEqual(original.legal)
        expect(messages.admin).toBeUndefined()
      }
    },
  )

  // Navigation proceeds from login into the marketplace and back to login. Shared page
  // text remains available while provider configuration and monitoring stay unloaded.
  it('从登录进入模型广场准备普通页面目录，返回登录保留已加载文案', async () => {
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'zh-Hans'
    await loadRouteLocaleMessages('/login')
    await loadRouteLocaleMessages('/models')
    await loadRouteLocaleMessages('/login')
    expect(i18n.global.getLocaleMessage('zh-Hans')).toEqual(zhBase)
    expect(i18n.global.getLocaleMessage('en')).toEqual(enBase)
    expect(i18n.global.getLocaleMessage('en').admin.providers.title).toBeUndefined()
    expect(i18n.global.getLocaleMessage('en').admin.ops).toBeUndefined()
  })

  // Authentication navigation, console navigation, and a direct catalog request overlap.
  // Whichever import finishes last must retain the complete merged dictionaries for both
  // locales.
  it('并发导航与完整目录加载后仍保留全部文案', async () => {
    const { i18n, loadRouteLocaleMessages, loadLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'zh-Hans'
    await Promise.all([
      loadRouteLocaleMessages('/login'),
      loadRouteLocaleMessages('/dashboard'),
      loadLocaleMessages('zh-Hans'),
      loadLocaleMessages('en'),
    ])
    expect(i18n.global.getLocaleMessage('zh-Hans')).toEqual(zh)
    expect(i18n.global.getLocaleMessage('en')).toEqual(en)
  })

  // The selected language changes while navigation is waiting for its first catalogs.
  // Navigation must wait until the newly selected language also has its authentication
  // messages.
  it('导航期间语言发生变化时，等待新语言准备完成', async () => {
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'en'
    const pending = loadRouteLocaleMessages('/login')
    i18n.global.locale.value = 'zh-Hans'
    await pending
    expect(i18n.global.getLocaleMessage('zh-Hans').auth).toEqual(zh.auth)
  })

  // A fresh monitoring route needs its own domain and ordinary shared page labels.
  // Visiting settings later merges its domains without deleting monitoring messages.
  it('管理页面按需加载领域，页面切换合并已加载文案', async () => {
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'zh-Hans'
    await loadRouteLocaleMessages('/admin/ops/')
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.ops).toEqual(zh.admin.ops)
    expect(i18n.global.getLocaleMessage('en').admin.settings.title).toBeUndefined()
    expect(i18n.global.getLocaleMessage('en').admin.providers.title).toBeUndefined()

    await loadRouteLocaleMessages('/admin/settings')
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.ops).toEqual(zh.admin.ops)
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.settings).toEqual(zh.admin.settings)
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.providers).toEqual(zh.admin.providers)
  })

  // Vue Router's actual records accept uppercase path segments and trailing slashes.
  // The locale loader receives that resolved path and prepares the matched page's
  // administrator domain or compact authentication catalog before component mounting.
  it.each([
    { path: '/admin/settings/', name: 'AdminSettings', domain: 'settings' },
    { path: '/ADMIN/settings', name: 'AdminSettings', domain: 'settings' },
    { path: '/ADMIN/PROVIDERS/', name: 'AdminProviders', domain: 'providers' },
    { path: '/LOGIN/', name: 'Login', domain: 'auth' },
    { path: '/LEGAL/terms/', name: 'LegalDocument', domain: 'auth' },
  ])('$path follows the router matching policy', async ({ path, name, domain }) => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [...publicRoutes, ...adminRoutes],
    })
    const resolved = router.resolve(path)
    expect(resolved.name).toBe(name)
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'en'
    await loadRouteLocaleMessages(resolved.path)
    const messages = i18n.global.getLocaleMessage('en')
    if (domain === 'auth') {
      expect(messages.auth).toEqual(en.auth)
      expect(messages.admin).toBeUndefined()
    } else {
      expect(messages.admin[domain]).toEqual(en.admin[domain as 'settings' | 'providers'])
    }
  })

  // A language switch follows settings navigation, whose dialogs use provider labels.
  // Both demanded domains must be ready when the new locale is published, while the
  // unrequested monitoring dictionary stays absent.
  it('切换语言准备当前已请求的领域目录', async () => {
    const { i18n, loadRouteLocaleMessages, setLocale } = await import('../index')
    i18n.global.locale.value = 'en'
    await loadRouteLocaleMessages('/admin/settings')
    await setLocale('zh-Hans', false)
    expect(i18n.global.locale.value).toBe('zh-Hans')
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.settings).toEqual(zh.admin.settings)
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.providers).toEqual(zh.admin.providers)
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.ops).toBeUndefined()
  })

  // Monitoring navigation begins while a language switch is preparing ordinary text.
  // The added domain must be ready in the winning language when both operations settle.
  it('语言切换期间新增页面领域仍会准备完整文案', async () => {
    const { i18n, loadRouteLocaleMessages, setLocale } = await import('../index')
    i18n.global.locale.value = 'en'
    await loadRouteLocaleMessages('/dashboard')
    await Promise.all([setLocale('zh-Hans', false), loadRouteLocaleMessages('/admin/ops')])
    expect(i18n.global.locale.value).toBe('zh-Hans')
    expect(i18n.global.getLocaleMessage('zh-Hans').admin.ops).toEqual(zh.admin.ops)
    expect(i18n.global.getLocaleMessage('en').admin.ops).toEqual(en.admin.ops)
  })

  // Opposing language switches overlap after an administrator domain has loaded.
  // Only the latest request may publish browser language and preference metadata.
  it('重叠语言切换由最后一次请求发布语言', async () => {
    const { i18n, loadRouteLocaleMessages, setLocale } = await import('../index')
    await loadRouteLocaleMessages('/admin/ops')
    await Promise.all([setLocale('zh-Hans', false), setLocale('en', false)])
    expect(i18n.global.locale.value).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    expect(localStorage.getItem('tokenrouter_locale')).toBe('en')
    expect(i18n.global.getLocaleMessage('en').admin.ops).toEqual(en.admin.ops)
  })
})
