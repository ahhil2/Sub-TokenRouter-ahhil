<template>
  <div ref="container" class="relative aspect-[4/3] w-full overflow-hidden bg-white">
    <iframe v-if="documentHTML" :srcdoc="documentHTML" :title="title" sandbox="allow-scripts" referrerpolicy="no-referrer"
      class="absolute left-0 top-0 border-0" :style="frameStyle" :tabindex="interactive ? 0 : -1" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { preparePelicanHTML } from '@/utils/pelicanHtml'

const props = withDefaults(defineProps<{ html: string; title: string; interactive?: boolean }>(), { interactive: false })
const container = ref<HTMLElement>()
const width = ref(0)
const documentHTML = computed(() => preparePelicanHTML(props.html))
const frameStyle = computed(() => ({ width: '1024px', height: '768px', transform: `scale(${width.value / 1024})`, transformOrigin: 'top left', pointerEvents: props.interactive ? 'auto' as const : 'none' as const }))
let observer: ResizeObserver | undefined
onMounted(() => {
  const update = () => { width.value = container.value?.clientWidth || 0 }
  update()
  observer = new ResizeObserver(update)
  if (container.value) observer.observe(container.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>
