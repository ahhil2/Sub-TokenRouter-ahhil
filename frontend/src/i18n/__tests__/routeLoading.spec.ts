import { beforeEach, describe, expect, it, vi } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

/**
 * routeLoading.spec.ts verifies authentication catalogs and their transition into complete locale dictionaries.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

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

  // Navigation proceeds from login into the marketplace and back to login. Both locale
  // dictionaries must retain every full-catalog entry after the route transitions complete.
  it('从登录进入模型广场前补齐完整目录，返回登录仍能访问已加载文案', async () => {
    const { i18n, loadRouteLocaleMessages } = await import('../index')
    i18n.global.locale.value = 'zh-Hans'
    await loadRouteLocaleMessages('/login')
    await loadRouteLocaleMessages('/models')
    await loadRouteLocaleMessages('/login')
    expect(i18n.global.getLocaleMessage('zh-Hans')).toEqual(zh)
    expect(i18n.global.getLocaleMessage('en')).toEqual(en)
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
})
