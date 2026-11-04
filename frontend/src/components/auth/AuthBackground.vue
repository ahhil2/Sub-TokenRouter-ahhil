<template>
  <div
    class="pointer-events-none absolute inset-0 z-0 select-none overflow-hidden transition-opacity duration-700"
    :class="loaded ? 'opacity-100' : 'opacity-0'"
    aria-hidden="true"
  >
    <div
      ref="container"
      class="absolute left-1/2 top-1/2 h-[2160px] w-[3840px] -translate-x-1/2 -translate-y-1/2"
    ></div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AnimationItem, LottiePlayer } from 'lottie-web'
import { useTheme } from '@/composables/useTheme'

/**
 * AuthBackground.vue renders optional theme artwork after the authentication form
 * has mounted. A delay followed by browser idle time defers the player and artwork
 * imports. Reduced-motion preference suppresses animation entirely, hidden documents
 * pause an attached player, and data-saving mode skips new artwork loads.
 *
 * Each synchronization advances a generation before checking presentation state.
 * An import may attach only when its generation still owns a mounted, visible
 * container with motion enabled. Theme replacement destroys the previous player
 * instance after the new resources arrive. Unmount invalidates pending imports,
 * cancels scheduled work, and removes preference and visibility listeners before
 * destroying the attached animation.
 */
// isDark follows the shared theme preference. The background watcher uses it to
// choose which artwork import may replace the currently attached animation.
const { isDark } = useTheme() // current authentication background theme

// container and loaded connect player attachment to background visibility. player,
// animation, and loadToken retain the module, current resource, and generation
// needed for theme replacement and cleanup.
const container = ref<HTMLDivElement>() // mounted Lottie target element
const loaded = ref(false) // first successful artwork display flag

let player: LottiePlayer | null = null // reusable dynamically imported player
// animation owns the currently attached player instance. Replacement destroys it,
// and unmount destroys it again if an instance is still attached.
let animation: AnimationItem | null = null // currently attached animation instance
// loadToken advances for each artwork request and unmount. A load may replace the
// player only while its captured value remains current.
let loadToken = 0 // latest artwork request generation
// reducedMotion gates resource loading; animationTheme identifies reusable artwork.
// cancelScheduled and disposed prevent queued work from attaching after component cleanup.
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
let animationTheme: boolean | null = null
let cancelScheduled: (() => void) | undefined
let disposed = false

/**
 * loadAnimation imports the requested theme artwork and the reusable Lottie player.
 * Call after the container mounts; failures leave the static layout background
 * visible. Each call captures the current generation before awaiting both resources.
 *
 * A later synchronization or unmount invalidates that generation. Attachment also
 * requires a visible document and motion permission, preventing an import that
 * finishes after a preference change from starting animation. The winning request
 * destroys the previous instance, attaches its replacement, and records the theme
 * used to decide whether later visibility changes can resume the same player.
 */
async function loadAnimation(dark: boolean) {
  const token = ++loadToken
  try {
    const [lottieModule, dataModule] = await Promise.all([
      player ? Promise.resolve(null) : import('lottie-web'),
      dark
        ? import('@/assets/lottie/auth-bg-dark.json')
        : import('@/assets/lottie/auth-bg-light.json'),
    ])
    if (
      token !== loadToken ||
      disposed ||
      !container.value ||
      reducedMotion.matches ||
      document.hidden
    )
      return
    if (lottieModule) player = lottieModule.default
    animation?.destroy()
    animation = player!.loadAnimation({
      container: container.value,
      renderer: 'svg',
      loop: true,
      autoplay: true,
      animationData: dataModule.default,
      rendererSettings: { preserveAspectRatio: 'xMidYMid slice' },
    })
    animationTheme = dark
    loaded.value = true
  } catch {
    // Artwork failure leaves the static layout background visible so the form remains
    // usable when optional resources cannot be downloaded or initialized.
  }
}

/**
 * syncAnimation reconciles pending work and the attached animation with current UI state.
 * It cancels scheduled work before advancing the generation. Reduced motion destroys
 * the player; hidden documents pause it; a visible matching theme resumes it.
 *
 * A different theme schedules loading after a short delay and browser idle time,
 * unless data saving is enabled. The retained cancellation handle changes from the
 * delay timer to the idle callback as scheduling progresses, allowing unmount or
 * another preference change to cancel whichever phase still owns queued work.
 */
function syncAnimation() {
  cancelScheduled?.()
  cancelScheduled = undefined
  loadToken++
  if (disposed || !container.value) return
  if (reducedMotion.matches) {
    animation?.destroy()
    animation = null
    loaded.value = false
    return
  }
  if (document.hidden) {
    animation?.pause()
    return
  }
  if (animation && animationTheme === isDark.value) {
    animation.play()
    return
  }
  const connection = (navigator as Navigator & { connection?: { saveData?: boolean } }).connection
  if (connection?.saveData) return
  // Give form mounting and its initial requests time to start before optional artwork
  // imports compete for browser work; the idle callback then chooses an available turn.
  const delay = window.setTimeout(() => {
    if (typeof window.requestIdleCallback === 'function') {
      const idle = window.requestIdleCallback(() => {
        cancelScheduled = undefined
        void loadAnimation(isDark.value)
      })
      cancelScheduled = () => window.cancelIdleCallback(idle)
    } else {
      cancelScheduled = undefined
      void loadAnimation(isDark.value)
    }
  }, 600)
  cancelScheduled = () => window.clearTimeout(delay)
}

// Register component-owned preference listeners after mounting the target container;
// synchronization then decides whether to queue artwork or retain the static background.
onMounted(() => {
  reducedMotion.addEventListener('change', syncAnimation)
  document.addEventListener('visibilitychange', syncAnimation)
  syncAnimation()
})
watch(isDark, syncAnimation)

// Release the component-owned timer or rendered resource when this view leaves, so
// detached UI does not retain its scheduled work.
onBeforeUnmount(() => {
  disposed = true
  // Invalidate outstanding imports before destroying the animation, so a late
  // theme load cannot attach another player after this component leaves.
  loadToken++
  cancelScheduled?.()
  reducedMotion.removeEventListener('change', syncAnimation)
  document.removeEventListener('visibilitychange', syncAnimation)
  // Destroy the attached player after invalidating imports and removing listeners so
  // its animation work and container references end with this component's lifetime.
  animation?.destroy()
  animation = null
})
</script>
