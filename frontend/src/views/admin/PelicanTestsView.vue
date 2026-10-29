<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('pelican.adminDescription') }}</p>
        <div class="flex gap-2"
          ><router-link to="/pelican-showcase" class="btn btn-secondary">{{
            t('pelican.viewGallery')
          }}</router-link
          ><button class="btn btn-primary" @click="edit()">{{ t('pelican.create') }}</button></div
        >
      </div>
      <details
        v-if="prompt"
        class="rounded-card border border-gray-200 bg-white p-4 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200"
      >
        <summary class="cursor-pointer font-medium">{{ t('pelican.prompt') }}</summary>
        <p class="mt-4 whitespace-pre-wrap break-words">{{ prompt }}</p>
      </details>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="notice" role="status" class="text-sm text-primary-600">{{ notice }}</p>
      <div class="overflow-x-auto rounded-card border border-gray-200 dark:border-dark-700">
        <table class="w-full text-left text-sm">
          <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800"
            ><tr
              ><th class="p-4">{{ t('pelican.group') }} / {{ t('pelican.model') }}</th
              ><th class="p-4">{{ t('pelican.schedule') }}</th
              ><th class="p-4">{{ t('pelican.lastRun') }}</th
              ><th class="p-4">{{ t('pelican.actions') }}</th></tr
            ></thead
          >
          <tbody
            ><tr
              v-for="task in tasks"
              :key="task.id"
              class="border-t border-gray-100 dark:border-dark-700"
            >
              <td class="p-4"
                ><p class="font-medium">{{ task.group_name }}</p
                ><p class="font-mono text-xs text-gray-500">{{ task.model }}</p></td
              >
              <td class="space-y-1 p-4"
                ><p>{{
                  task.enabled
                    ? t('pelican.every', { minutes: task.interval_minutes })
                    : t('pelican.paused')
                }}</p
                ><p v-if="task.enabled" class="text-xs text-gray-500"
                  >{{ t('pelican.nextRun') }}: {{ formatTime(task.next_run_at) }}</p
                ><p
                  v-if="task.lease_until || task.manual_requested"
                  class="text-xs text-primary-600"
                  >{{ t(task.lease_until ? 'pelican.status.running' : 'pelican.queued') }}</p
                ></td
              >
              <td class="p-4"
                ><p>{{
                  task.last_status ? t(`pelican.status.${task.last_status}`) : t('pelican.notRun')
                }}</p
                ><p class="text-xs text-gray-500">{{ formatTime(task.last_run_at) }}</p></td
              >
              <td class="p-4"
                ><div class="flex flex-wrap gap-2">
                  <button
                    class="btn btn-secondary btn-sm"
                    :disabled="busy || taskBusy(task)"
                    @click="edit(task)"
                    >{{ t('pelican.edit') }}</button
                  >
                  <button
                    class="btn btn-secondary btn-sm"
                    :disabled="busy || taskBusy(task)"
                    @click="queue(task)"
                    >{{ t('pelican.run') }}</button
                  >
                  <button class="btn btn-secondary btn-sm" @click="selectHistory(task.id)">{{
                    t('pelican.history')
                  }}</button>
                  <button
                    class="btn btn-secondary btn-sm"
                    :disabled="busy || taskBusy(task)"
                    @click="confirmDelete = { task: task.id }"
                    >{{ t('pelican.delete') }}</button
                  >
                </div></td
              >
            </tr></tbody
          >
        </table>
        <p v-if="!tasks.length" class="p-6 text-center text-gray-500">{{ t('pelican.noTasks') }}</p>
      </div>
      <section class="space-y-4">
        <div class="flex flex-wrap items-end justify-between gap-4"
          ><h2 class="text-lg font-semibold">{{ t('pelican.history') }}</h2
          ><div class="flex gap-2"
            ><button class="btn btn-secondary" @click="selectHistory(0)">{{
              t('pelican.all')
            }}</button
            ><button class="btn btn-secondary" :disabled="busy" @click="refresh">{{
              t('pelican.refresh')
            }}</button></div
          ></div
        >
        <div
          v-for="item in results"
          :key="item.id"
          class="flex flex-col gap-4 rounded-card border border-gray-200 p-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="min-w-0 space-y-1"
            ><p class="truncate font-medium">{{ item.group_name }} · {{ item.model }}</p
            ><p class="text-xs text-gray-500"
              >{{ formatTime(item.started_at) }} · {{ t(`pelican.status.${item.status}`) }} ·
              {{ (item.latency_ms / 1000).toFixed(1) }} s ·
              {{ t(item.published ? 'pelican.published' : 'pelican.hidden') }}</p
            ><p v-if="item.error_message" class="break-words text-xs text-red-600">{{
              item.error_message
            }}</p></div
          >
          <div class="flex shrink-0 flex-wrap gap-2"
            ><button class="btn btn-secondary btn-sm" @click="detail(item)">{{
              t('pelican.details')
            }}</button
            ><button
              class="btn btn-secondary btn-sm"
              :disabled="busy || item.status === 'running'"
              @click="togglePublication(item)"
              >{{ t(item.published ? 'pelican.hide' : 'pelican.publish') }}</button
            ><button
              class="btn btn-secondary btn-sm"
              :disabled="busy || item.status === 'running'"
              @click="confirmDelete = { result: item.id }"
              >{{ t('pelican.delete') }}</button
            ></div
          >
        </div>
        <p v-if="!results.length" class="text-sm text-gray-500">{{ t('pelican.empty') }}</p>
        <div class="flex justify-between"
          ><button class="btn btn-secondary" :disabled="page === 1" @click="changePage(-1)">{{
            t('pelican.previous')
          }}</button
          ><button class="btn btn-secondary" :disabled="!hasMore" @click="changePage(1)">{{
            t('pelican.next')
          }}</button></div
        >
      </section>
    </div>
    <BaseDialog
      :show="editing"
      :title="t(editID ? 'pelican.edit' : 'pelican.create')"
      @close="editing = false"
    >
      <form class="space-y-4" @submit.prevent="save">
        <label class="block space-y-2 text-sm"
          ><span>{{ t('pelican.group') }}</span
          ><Select
            v-model="form.group_id"
            :options="groupOptions"
            searchable
            @update:model-value="groupChanged"
        /></label>
        <label class="block space-y-2 text-sm"
          ><span>{{ t('pelican.model') }}</span
          ><Select
            v-model="form.model"
            :options="modelOptions"
            searchable
            creatable
            :disabled="modelsLoading"
        /></label>
        <label class="block space-y-2 text-sm"
          ><span>{{ t('pelican.interval') }}</span
          ><input
            v-model.number="form.interval_minutes"
            type="number"
            min="5"
            max="10080"
            required
            class="input"
        /></label>
        <div class="flex items-center justify-between gap-4 text-sm"
          ><span>{{ t('pelican.enabled') }}</span
          ><Toggle v-model="form.enabled" :disabled="!editID" :aria-label="t('pelican.enabled')"
        /></div>
        <p v-if="!editID" class="text-xs text-gray-500">{{ t('pelican.newPaused') }}</p>
        <label class="flex items-start gap-3 text-sm"
          ><input v-model="form.public_consent" type="checkbox" class="mt-1" /><span>{{
            t('pelican.consent')
          }}</span></label
        >
        <p class="text-xs text-gray-500">{{ t('pelican.cost') }}</p>
        <p v-if="formError" role="alert" class="text-sm text-red-600">{{ formError }}</p>
        <button
          class="btn btn-primary w-full"
          :disabled="busy || modelsLoading || !form.group_id || !form.model"
          >{{ t('pelican.save') }}</button
        >
      </form>
    </BaseDialog>
    <BaseDialog
      :show="selected !== null"
      :title="t('pelican.details')"
      width="wide"
      @close="selected = null"
    >
      <template v-if="selected"
        ><PelicanPreview
          v-if="selected.html"
          :html="selected.html"
          :title="selected.model"
          interactive
        /><p class="my-4 break-words text-sm">{{
          selected.error_message || `${selected.model} · ${selected.execution_model || ''}`
        }}</p
        ><p class="text-xs text-gray-500"
          >{{ t('pelican.provider') }}: {{ selected.provider_id || '—' }}</p
        ><details class="mt-4"
          ><summary class="cursor-pointer text-sm">{{ t('pelican.raw') }}</summary
          ><pre
            class="mt-2 max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-card bg-gray-50 p-4 text-xs dark:bg-dark-900"
            >{{ selected.raw_response }}</pre>
        </details></template
      >
    </BaseDialog>
    <ConfirmDialog
      :show="confirmDelete !== null"
      :title="t('pelican.delete')"
      :message="t('pelican.deleteConfirm')"
      danger
      @cancel="confirmDelete = null"
      @confirm="remove"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import PelicanPreview from '@/components/pelican/PelicanPreview.vue'
import { getAll, getModelsListCandidates } from '@/api/admin/groups'
import {
  pelicanApi,
  type PelicanTask,
  type PelicanResult,
  type PelicanTaskInput,
} from '@/api/pelican'
import type { AdminGroup } from '@/api/groups.types'

/**
 * PelicanTestsView.vue manages scheduled model drawing tasks and inspects their
 * execution results. Administrators choose groups and models, configure task intervals
 * and publication consent, and inspect generated output through the same preview
 * component used by the public gallery.
 */

const { t } = useI18n()
const tasks = ref<PelicanTask[]>([])
const groups = ref<AdminGroup[]>([])
const models = ref<string[]>([])
const results = ref<PelicanResult[]>([])
const prompt = ref('')
const page = ref(1)
const historyID = ref(0)
const hasMore = ref(false)
const error = ref('')
const notice = ref('')
const formError = ref('')
const busy = ref(false)
const editing = ref(false)
const editID = ref<number>()
const modelsLoading = ref(false)
const selected = ref<PelicanResult | null>(null)
const confirmDelete = ref<{ task?: number; result?: number } | null>(null)
const form = reactive<PelicanTaskInput>({
  group_id: 0,
  model: '',
  enabled: false,
  interval_minutes: 360,
  public_consent: false,
})
const groupOptions = computed(() =>
  groups.value.map((group) => ({ value: group.id, label: group.name })),
)
const modelOptions = computed(() => models.value.map((value) => ({ value, label: value })))
let timer: ReturnType<typeof setInterval> | undefined
let refreshing = false
let modelsVersion = 0
let historyVersion = 0
let disposed = false
function message(value: unknown) {
  return value instanceof Error ? value.message : t('pelican.loadFailed')
}
function formatTime(value: string | null) {
  return value ? new Date(value).toLocaleString() : '—'
}
function taskBusy(task: PelicanTask) {
  return !!task.lease_until || task.manual_requested
}
async function loadHistory() {
  const version = ++historyVersion
  const result = await pelicanApi.results({ page: page.value, task_id: historyID.value }, true)
  if (!disposed && version === historyVersion) {
    results.value = result.items
    hasMore.value = result.has_more
    prompt.value = result.prompt
  }
}
async function refresh() {
  if (refreshing) return
  refreshing = true
  try {
    const values = await pelicanApi.tasks()
    if (!disposed) tasks.value = values
    await loadHistory()
  } catch (value) {
    if (!disposed) error.value = message(value)
  } finally {
    refreshing = false
  }
}
function selectHistory(id: number) {
  historyID.value = id
  page.value = 1
  void loadHistory().catch((value) => {
    error.value = message(value)
  })
}
function changePage(delta: number) {
  page.value += delta
  void loadHistory().catch((value) => {
    error.value = message(value)
  })
}
async function loadModels() {
  const version = ++modelsVersion
  modelsLoading.value = true
  models.value = []
  try {
    const values = form.group_id ? await getModelsListCandidates(form.group_id) : []
    if (version === modelsVersion) models.value = values
  } catch (value) {
    if (version === modelsVersion) formError.value = message(value)
  } finally {
    if (version === modelsVersion) modelsLoading.value = false
  }
}
function groupChanged() {
  form.model = ''
  void loadModels()
}
function edit(task?: PelicanTask) {
  editID.value = task?.id
  Object.assign(form, {
    group_id: task?.group_id || groups.value[0]?.id || 0,
    model: task?.model || '',
    enabled: task?.enabled || false,
    interval_minutes: task?.interval_minutes || 360,
    public_consent: task?.public_consent || false,
  })
  editing.value = true
  formError.value = ''
  void loadModels()
}
async function save() {
  if (form.enabled && !form.public_consent) {
    formError.value = t('pelican.consentRequired')
    return
  }
  busy.value = true
  formError.value = ''
  try {
    await pelicanApi.save({ ...form, group_id: Number(form.group_id) }, editID.value)
    editing.value = false
    await refresh()
  } catch (value) {
    formError.value = message(value)
  } finally {
    busy.value = false
  }
}
async function operate(action: () => Promise<unknown>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await action()
    await refresh()
  } catch (value) {
    error.value = message(value)
  } finally {
    busy.value = false
  }
}
function togglePublication(item: PelicanResult) {
  void operate(() => pelicanApi.publish(item.id, !item.published))
}
function queue(task: PelicanTask) {
  if (!task.public_consent) {
    edit(task)
    formError.value = t('pelican.consentRequired')
    return
  }
  void operate(async () => {
    await pelicanApi.run(task.id)
    notice.value = t('pelican.queued')
  })
}
async function remove() {
  const target = confirmDelete.value
  confirmDelete.value = null
  if (!target) return
  await operate(() =>
    target.task ? pelicanApi.removeTask(target.task) : pelicanApi.removeResult(target.result!),
  )
}
async function detail(item: PelicanResult) {
  try {
    selected.value = await pelicanApi.result(item.id, true)
  } catch (value) {
    error.value = message(value)
  }
}
onMounted(async () => {
  void refresh()
  try {
    groups.value = await getAll()
  } catch (value) {
    error.value = message(value)
  }
  timer = setInterval(() => {
    if (document.visibilityState === 'visible') void refresh()
  }, 10000)
})
onBeforeUnmount(() => {
  disposed = true
  clearInterval(timer)
  modelsVersion++
  historyVersion++
})
</script>
