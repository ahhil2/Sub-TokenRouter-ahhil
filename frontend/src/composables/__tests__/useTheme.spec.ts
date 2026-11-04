import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import { initTheme, setTheme, useLightTheme, useTheme } from '../useTheme'

// useTheme.spec.ts checks document theme application and temporary transition suppression
// using controlled frame callbacks. System-following behavior reloads the module against a
// media-query stub so the persistent listener observes the intended per-test operating
// system changes.

describe('useTheme', () => {
  const frameCallbacks: FrameRequestCallback[] = []

  beforeEach(() => {
    frameCallbacks.length = 0
    localStorage.setItem('theme', 'light')
    initTheme()
    document.documentElement.classList.remove('theme-switching')

    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frameCallbacks.push(callback)
      return frameCallbacks.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.documentElement.classList.remove('dark', 'theme-switching')
    localStorage.removeItem('theme')
  })

  // An explicit switch to dark mode schedules two rendering frames. The document must use
  // dark colors immediately while transition suppression remains until the second callback
  // finishes.
  it('temporarily disables transitions while switching themes', () => {
    setTheme(true)

    expect(document.documentElement.classList.contains('dark')).toBe(true) // check-ui-allow: theme primitive tests assert the document class
    expect(document.documentElement.classList.contains('theme-switching')).toBe(true)

    frameCallbacks[0]?.(0)
    expect(document.documentElement.classList.contains('theme-switching')).toBe(true)

    frameCallbacks[1]?.(16)
    expect(document.documentElement.classList.contains('theme-switching')).toBe(false)
  })

  it('uses light authentication layouts and restores the saved dark preference on exit', () => {
    setTheme(true)
    const layout = defineComponent({
      setup() {
        useLightTheme()
        return () => null
      },
    })
    const first = mount(layout)
    const second = mount(layout)

    expect(useTheme().isDark.value).toBe(false)
    expect(localStorage.getItem('theme')).toBe('dark')
    initTheme()
    expect(useTheme().isDark.value).toBe(false)

    first.unmount()
    expect(useTheme().isDark.value).toBe(false)
    second.unmount()
    expect(useTheme().isDark.value).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
  })

  // A fresh theme module observes a mutable system preference after selecting system mode.
  // It must clear stored explicit choice and follow subsequent media-query changes in the
  // document.
  it('follows the system color scheme and clears the saved choice', async () => {
    const listeners: Array<(event: MediaQueryListEvent) => void> = []
    let systemDark = true
    vi.spyOn(window, 'matchMedia').mockImplementation(
      () =>
        ({
          // matches exposes the current simulated operating-system dark preference to
          // theme initialization.
          get matches() {
            return systemDark
          },
          addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) =>
            listeners.push(listener),
        }) as unknown as MediaQueryList,
    )

    // Reload the theme module before initialization because its media-query listener is
    // cached once and must bind to this test's stub.
    vi.resetModules()
    const theme = await import('../useTheme')
    theme.initTheme()
    theme.setThemeMode('system')

    expect(theme.useTheme().themeMode.value).toBe('system')
    expect(localStorage.getItem('theme')).toBeNull()
    expect(document.documentElement.classList.contains('dark')).toBe(true) // check-ui-allow: theme primitive tests assert the document class

    systemDark = false
    listeners.forEach((listener) => listener({ matches: false } as MediaQueryListEvent))
    expect(document.documentElement.classList.contains('dark')).toBe(false) // check-ui-allow: theme primitive tests assert the document class

    const wrapper = mount(
      defineComponent({
        setup() {
          theme.useLightTheme()
          return () => null
        },
      }),
    )
    systemDark = true
    listeners.forEach((listener) => listener({ matches: true } as MediaQueryListEvent))
    expect(theme.useTheme().isDark.value).toBe(false)
    expect(localStorage.getItem('theme')).toBeNull()

    wrapper.unmount()
    expect(theme.useTheme().isDark.value).toBe(true)
  })
})
