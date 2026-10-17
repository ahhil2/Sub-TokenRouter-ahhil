<template>
  <div
    class="inline-flex h-8 items-center rounded-control border transition-colors"
    :class="editing || dirty
      ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700 dark:bg-primary-900/20'
      : 'border-transparent hover:border-gray-200 hover:bg-gray-50 dark:hover:border-dark-600 dark:hover:bg-dark-700/60'"
    data-testid="provider-priority-cell"
  >
    <button
      type="button"
      class="flex h-full w-7 items-center justify-center rounded-l-control text-gray-500 transition hover:bg-gray-200/70 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:text-gray-300 dark:text-gray-400 dark:hover:bg-dark-600 dark:hover:text-primary-400 dark:disabled:text-dark-600"
      :disabled="saving || editing || draft <= MIN_PRIORITY"
      :title="t('admin.providers.decreasePriority')"
      :aria-label="t('admin.providers.decreasePriority')"
      data-testid="provider-priority-decrement"
      @click="step(-1)"
    >
      <Icon name="minus" size="xs" />
    </button>

    <input
      v-if="editing"
      ref="inputRef"
      v-model="inputValue"
      type="text"
      inputmode="numeric"
      pattern="[0-9]*"
      class="h-full w-12 border-0 bg-transparent p-0 text-center font-mono text-sm tabular-nums text-gray-900 focus:outline-none focus:ring-0 dark:text-white"
      :aria-label="t('admin.providers.columns.priority')"
      data-testid="provider-priority-input"
      @keydown.enter.prevent="commitInput"
      @keydown.esc.prevent="cancelInput"
      @blur="commitInput"
    />
    <button
      v-else
      type="button"
      class="relative flex h-full min-w-10 items-center justify-center px-1 font-mono text-sm tabular-nums text-gray-700 transition-colors hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-wait dark:text-gray-300 dark:hover:text-white"
      :class="dirty ? 'font-semibold text-primary-600 dark:text-primary-400' : ''"
      :disabled="saving"
      :title="t('admin.providers.editPriority')"
      data-testid="provider-priority-value"
      @click="startEditing"
    >
      {{ draft }}
      <span
        v-if="saving"
        class="absolute -right-0.5 top-1 h-1.5 w-1.5 animate-pulse rounded-full bg-primary-500"
        role="status"
        :aria-label="t('admin.providers.savingPriority')"
        data-testid="provider-priority-saving"
      />
    </button>

    <button
      type="button"
      class="flex h-full w-7 items-center justify-center rounded-r-control text-gray-500 transition hover:bg-gray-200/70 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:text-gray-300 dark:text-gray-400 dark:hover:bg-dark-600 dark:hover:text-primary-400 dark:disabled:text-dark-600"
      :disabled="saving || editing || draft >= MAX_PRIORITY"
      :title="t('admin.providers.increasePriority')"
      :aria-label="t('admin.providers.increasePriority')"
      data-testid="provider-priority-increment"
      @click="step(1)"
    >
      <Icon name="plus" size="xs" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import type { Provider } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const MIN_PRIORITY = 1
const MAX_PRIORITY = 2_147_483_647
const SAVE_DEBOUNCE_MS = 450

const props = defineProps<{ provider: Provider }>()
const emit = defineEmits<{
  (event: 'updated', provider: Provider): void
  (event: 'error', message: string): void
}>()

const { t } = useI18n()
const draft = ref(props.provider.priority)
const editing = ref(false)
const inputValue = ref('')
const inputRef = ref<HTMLInputElement | null>(null)
const saving = ref(false)
const dirty = computed(() => draft.value !== props.provider.priority)
let saveTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.provider.priority,
  (priority) => {
    if (!saving.value && !saveTimer && !editing.value) draft.value = priority
  }
)

const clearSaveTimer = () => {
  if (!saveTimer) return
  clearTimeout(saveTimer)
  saveTimer = null
}

const save = async () => {
  clearSaveTimer()
  const priority = draft.value
  if (!Number.isSafeInteger(priority) || priority < MIN_PRIORITY || priority > MAX_PRIORITY) {
    draft.value = props.provider.priority
    emit('error', t('admin.providers.priorityInvalid'))
    return
  }
  if (priority === props.provider.priority || saving.value) return

  saving.value = true
  try {
    const updated = await adminAPI.providers.update(props.provider.id, { priority })
    draft.value = updated.priority
    emit('updated', updated)
  } catch (error) {
    draft.value = props.provider.priority
    emit('error', extractApiErrorMessage(error, t('admin.providers.prioritySaveFailed')))
  } finally {
    saving.value = false
  }
}

const scheduleSave = () => {
  clearSaveTimer()
  saveTimer = setTimeout(() => void save(), SAVE_DEBOUNCE_MS)
}

const step = (change: number) => {
  if (saving.value || editing.value) return
  const next = Math.min(MAX_PRIORITY, Math.max(MIN_PRIORITY, draft.value + change))
  if (next === draft.value) return
  draft.value = next
  scheduleSave()
}

const startEditing = async () => {
  if (saving.value) return
  clearSaveTimer()
  inputValue.value = String(draft.value)
  editing.value = true
  await nextTick()
  inputRef.value?.focus()
  inputRef.value?.select()
}

const commitInput = () => {
  if (!editing.value) return
  editing.value = false
  const priority = Number(inputValue.value.trim())
  if (!Number.isSafeInteger(priority) || priority < MIN_PRIORITY || priority > MAX_PRIORITY) {
    draft.value = props.provider.priority
    emit('error', t('admin.providers.priorityInvalid'))
    return
  }
  draft.value = priority
  void save()
}

const cancelInput = () => {
  editing.value = false
  if (!saving.value) draft.value = props.provider.priority
}

onBeforeUnmount(() => {
  if (saveTimer) void save()
})
</script>
