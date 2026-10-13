<template>
  <component :is="auth.isAuthenticated ? AppLayout : 'div'" :class="auth.isAuthenticated ? '' : 'min-h-screen bg-gray-50 dark:bg-dark-900'">
    <AppHeader v-if="!auth.isAuthenticated" public-page />
    <main class="mx-auto w-full max-w-7xl space-y-6" :class="auth.isAuthenticated ? '' : 'px-4 pb-8 pt-[calc(var(--header-h)+1.5rem)] md:px-6 lg:px-8'">
      <header class="space-y-2">
        <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ t('pelican.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('pelican.description') }}</p>
        <blockquote class="rounded-card border border-gray-200 bg-white p-4 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200">{{ PELICAN_PROMPT }}</blockquote>
      </header>
      <form class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4" @submit.prevent="reload">
        <label class="space-y-2 text-sm"><span>{{ t('pelican.group') }}</span><Select v-model="groupID" :options="groupOptions" /></label>
        <label class="space-y-2 text-sm"><span>{{ t('pelican.model') }}</span><Select v-model="model" :options="modelOptions" searchable /></label>
        <label class="space-y-2 text-sm"><span>{{ t('pelican.date') }}</span><input v-model="date" type="date" class="input" /></label>
        <button class="btn btn-primary self-end" :disabled="loading">{{ t('pelican.refresh') }}</button>
      </form>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ t('pelican.loadFailed') }}</p>
      <p v-if="loading" class="text-sm text-gray-500" role="status">{{ t('pelican.loading') }}</p>
      <p v-else-if="!items.length" class="rounded-card border border-dashed border-gray-300 p-6 text-center text-gray-500 dark:border-dark-700">{{ t('pelican.empty') }}</p>
      <section v-for="group in grouped" :key="group.id" class="space-y-4">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{{ group.name }}</h2>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <PelicanCard v-for="item in group.items" :key="item.id" :item="item" :active="activeIDs.has(item.id)" @visible="visibility" @open="open" />
        </div>
      </section>
      <footer class="flex items-center justify-between gap-4">
        <button class="btn btn-secondary" :disabled="page === 1 || loading" @click="changePage(-1)">{{ t('pelican.previous') }}</button>
        <span class="text-sm text-gray-500">{{ page }}</span>
        <button class="btn btn-secondary" :disabled="!hasMore || loading" @click="changePage(1)">{{ t('pelican.next') }}</button>
      </footer>
    </main>
    <BaseDialog :show="selected !== null" :title="selected ? `${selected.group_name} · ${selected.model}` : t('pelican.preview')" width="wide" @close="close">
      <PelicanPreview v-if="selected?.html" :html="selected.html" :title="selected.model" interactive />
      <p v-else class="p-6 text-sm text-gray-500">{{ previewError ? t('pelican.loadFailed') : selected?.status !== 'success' ? t(`pelican.status.${selected?.status || 'failed'}`) : t('pelican.loading') }}</p>
      <p class="mt-4 text-xs text-gray-500">{{ t('pelican.sandbox') }}</p>
    </BaseDialog>
  </component>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import AppHeader from '@/components/layout/AppHeader.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import PelicanCard from '@/components/pelican/PelicanCard.vue'
import PelicanPreview from '@/components/pelican/PelicanPreview.vue'
import { pelicanApi, PELICAN_PROMPT, type PelicanCatalogItem, type PelicanResult } from '@/api/pelican'

const { t } = useI18n()
const auth = useAuthStore()
const items = ref<PelicanResult[]>([])
const catalog = ref<PelicanCatalogItem[]>([])
const groupID = ref<number | string>(0)
const model = ref('')
const date = ref('')
const page = ref(1)
const hasMore = ref(false)
const loading = ref(false)
const error = ref(false)
const previewError = ref(false)
const selected = ref<PelicanResult | null>(null)
const visible = ref(new Set<number>())
let listController: AbortController | undefined
let previewController: AbortController | undefined
const groupOptions = computed(() => [{ value: 0, label: t('pelican.all') }, ...Array.from(new Map(catalog.value.map(item => [item.group_id, { value: item.group_id, label: item.group_name }])).values())])
const modelOptions = computed(() => [{ value: '', label: t('pelican.all') }, ...Array.from(new Set(catalog.value.filter(item => !Number(groupID.value) || item.group_id === Number(groupID.value)).map(item => item.model))).map(value => ({ value, label: value }))])
const activeIDs = computed(() => new Set(items.value.filter(item => visible.value.has(item.id) && item.status === 'success').slice(0, selected.value ? 5 : 6).map(item => item.id)))
const grouped = computed(() => {
  const groups = new Map<number, { id: number; name: string; items: PelicanResult[] }>()
  for (const item of items.value) {
    if (!groups.has(item.group_id)) groups.set(item.group_id, { id: item.group_id, name: item.group_name, items: [] })
    groups.get(item.group_id)!.items.push(item)
  }
  return [...groups.values()]
})
function visibility(id: number, value: boolean) { if (value) visible.value.add(id); else visible.value.delete(id) }
async function load() {
  listController?.abort()
  const controller = new AbortController()
  listController = controller
  loading.value = true
  error.value = false
  try {
    const result = await pelicanApi.results({ page: page.value, group_id: Number(groupID.value), model: model.value, date: date.value }, false, controller.signal)
    if (!controller.signal.aborted) { items.value = result.items; hasMore.value = result.has_more }
  } catch { if (!controller.signal.aborted) error.value = true }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function reload() { page.value = 1; void load() }
function changePage(delta: number) { page.value += delta; void load() }
async function open(item: PelicanResult) {
  previewController?.abort()
  const controller = new AbortController()
  previewController = controller
  selected.value = item
  previewError.value = false
  try { const result = await pelicanApi.result(item.id, false, controller.signal); if (!controller.signal.aborted) selected.value = result }
  catch { if (!controller.signal.aborted) previewError.value = true }
}
function close() { previewController?.abort(); selected.value = null }
watch(groupID, () => { model.value = '' })
onMounted(async () => { void load(); try { catalog.value = await pelicanApi.catalog() } catch { error.value = true } })
onBeforeUnmount(() => { listController?.abort(); previewController?.abort() })
</script>
