import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AuthBackground from '../AuthBackground.vue'

/**
 * AuthBackground.spec.ts verifies deferred artwork initialization, visibility, motion preference, and component cleanup.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

const player = vi.hoisted(() => ({
  loadAnimation: vi.fn(),
  pause: vi.fn(),
  play: vi.fn(),
  destroy: vi.fn(),
}))
vi.mock('lottie-web', () => ({ default: { loadAnimation: player.loadAnimation } }))
vi.mock('@/assets/lottie/auth-bg-light.json', () => ({ default: {} }))
vi.mock('@/assets/lottie/auth-bg-dark.json', () => ({ default: {} }))
vi.mock('@/composables/useTheme', async () => {
  const { ref } = await import('vue')
  return { useTheme: () => ({ isDark: ref(false) }) }
})

enableAutoUnmount(afterEach)

let reduced = false
let changeMotion: (() => void) | undefined
beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  reduced = false
  vi.stubGlobal('matchMedia', () => ({
    get matches() {
      return reduced
    },
    addEventListener: (_event: string, handler: () => void) => {
      changeMotion = handler
    },
    removeEventListener: vi.fn(),
  }))
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  vi.stubGlobal('requestIdleCallback', (callback: () => void) => setTimeout(callback, 0))
  vi.stubGlobal('cancelIdleCallback', (handle: number) => clearTimeout(handle))
  player.loadAnimation.mockReturnValue(player)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('认证页背景', () => {
  // The form mounts before optional animation and later becomes hidden and visible again.
  // The player must initialize after the delay, pause, resume, and destroy on unmount.
  it('表单挂载后才初始化，隐藏页面暂停，卸载释放动画', async () => {
    const wrapper = mount(AuthBackground)
    await vi.advanceTimersByTimeAsync(599)
    expect(player.loadAnimation).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2)
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(player.loadAnimation).toHaveBeenCalledOnce()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(player.pause).toHaveBeenCalledOnce()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(player.play).toHaveBeenCalledOnce()
    wrapper.unmount()
    expect(player.destroy).toHaveBeenCalledOnce()
  })

  // Reduced motion is enabled before mounting and changes while the component remains
  // active. The player must follow the preference without starting an unwanted animation.
  it('减少动画模式显示静态背景，偏好变化即时生效', async () => {
    reduced = true
    const wrapper = mount(AuthBackground)
    await vi.advanceTimersByTimeAsync(2000)
    expect(player.loadAnimation).not.toHaveBeenCalled()
    reduced = false
    changeMotion?.()
    await vi.advanceTimersByTimeAsync(601)
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(player.loadAnimation).toHaveBeenCalledOnce()
    reduced = true
    changeMotion?.()
    expect(player.destroy).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  // The authentication layout leaves before its delayed artwork initialization begins.
  // Advancing timers after unmount must not initialize a player for the removed container.
  it('初始化前离页取消资源加载', async () => {
    const wrapper = mount(AuthBackground)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(2000)
    expect(player.loadAnimation).not.toHaveBeenCalled()
  })
})
