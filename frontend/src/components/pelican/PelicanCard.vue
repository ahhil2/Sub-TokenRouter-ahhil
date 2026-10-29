<template>
  <article
    ref="container"
    class="overflow-hidden rounded-card border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
  >
    <button
      type="button"
      class="block w-full text-left"
      :aria-label="t('pelican.preview') + ' ' + item.model"
      @click="$emit('open', item)"
    >
      <PelicanPreview v-if="active && html" :html="html" :title="item.model" />
      <div
        v-else
        class="flex aspect-[4/3] items-center justify-center bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-900"
      >
        {{
          item.status !== 'success'
            ? t(`pelican.status.${item.status}`)
            : error
              ? t('pelican.loadFailed')
              : active
                ? t('pelican.loading')
                : t('pelican.preview')
        }}
      </div>
      <div class="space-y-2 p-4">
        <p class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-gray-100">{{
          item.model
        }}</p>
        <div class="flex flex-wrap justify-between gap-2 text-xs text-gray-500 dark:text-dark-400">
          <time>{{ new Date(item.started_at).toLocaleString() }}</time>
          <span>{{ (item.latency_ms / 1000).toFixed(1) }} s</span>
        </div>
      </div>
    </button>
  </article>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { pelicanApi, type PelicanResult } from '@/api/pelican'
import PelicanPreview from './PelicanPreview.vue'

/**
 * PelicanCard.vue displays a model drawing result and reports viewport visibility to
 * the gallery. Active successful results load their generated document through an
 * abortable request; deactivation clears the preview and unmounting releases both
 * observation and request resources.
 */

const props = defineProps<{ item: PelicanResult; active: boolean }>()
const emit = defineEmits<{ visible: [id: number, value: boolean]; open: [item: PelicanResult] }>()
const { t } = useI18n()
const container = ref<HTMLElement>()
const html = ref('')
const error = ref(false)
let observer: IntersectionObserver | undefined
let controller: AbortController | undefined
onMounted(() => {
  observer = new IntersectionObserver((entries) =>
    emit('visible', props.item.id, entries[0]?.isIntersecting ?? false),
  )
  if (container.value) observer.observe(container.value)
})
watch(
  () => props.active,
  async (active) => {
    controller?.abort()
    if (!active || props.item.status !== 'success') {
      html.value = ''
      return
    }
    const request = new AbortController()
    controller = request
    error.value = false
    try {
      const result = await pelicanApi.result(props.item.id, false, request.signal)
      if (!request.signal.aborted) html.value = result.html || ''
    } catch {
      if (!request.signal.aborted) error.value = true
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  observer?.disconnect()
  controller?.abort()
  emit('visible', props.item.id, false)
})
</script>
