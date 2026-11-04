<template>
  <div ref="root" :style="visible ? undefined : { minHeight }">
    <slot v-if="visible" />
    <slot v-else name="placeholder"><ChartSkeleton /></slot>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import ChartSkeleton from './ChartSkeleton.vue'

/**
 * LazySection.vue mounts its content when the section approaches the viewport.
 * A placeholder reserves the configured height until IntersectionObserver reports
 * intersection. Once shown, content stays mounted so charts retain state while
 * scrolling, and browsers without intersection observation display it immediately.
 */
withDefaults(defineProps<{ minHeight?: string /* Placeholder minimum CSS height. */ }>(), {
  minHeight: '20rem',
})
// The component owns its target, one-way visibility flag, and observer; observation
// ends at first reveal or unmount so hidden placeholders retain no detached listeners.
const root = ref<HTMLElement | null>(null)
const visible = ref(false)
let observer: IntersectionObserver | undefined
onMounted(() => {
  if (typeof IntersectionObserver === 'undefined') {
    visible.value = true
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        visible.value = true
        observer?.disconnect()
      }
    },
    // Reveal before the section reaches the viewport to allow chart initialization.
    { rootMargin: '240px' },
  )
  if (root.value) observer.observe(root.value)
})
onUnmounted(() => observer?.disconnect())
</script>
