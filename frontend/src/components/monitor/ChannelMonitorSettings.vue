<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  getChannelMonitorConfig,
  saveChannelMonitorConfig,
  type ChannelMonitorGroupOption,
  type ChannelMonitorStatus,
} from '@/api/channelMonitor'

/**
 * ChannelMonitorSettings.vue edits public channel visibility, display order, and
 * manual status selections. It loads an editable copy of the server configuration
 * and submits selected groups in their displayed order. Ordering controls move
 * selected groups past unchecked rows, so each move changes the selected sequence.
 * The public page arranges these groups into brand sections after loading them.
 *
 * Changes remain in the local draft until saving succeeds. Saving disables editing
 * to keep the submitted order and statuses consistent with the success message.
 * Failed saves retain the draft for another attempt, while unmounting cancels the
 * configuration read and suppresses late request results.
 */

const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const groups = ref<ChannelMonitorGroupOption[]>([])
const loading = ref(true)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const saved = ref(false)
const controller = new AbortController()
let disposed = false
const statuses: ChannelMonitorStatus[] = [
  'auto',
  'operational',
  'degraded',
  'failed',
  'maintenance',
]
const options = computed(() =>
  statuses.map((value) => ({ value, label: t(`channelMonitor.status.${value}`) })),
)
// selectedGroups follows the draft order and identifies the first and last movable
// rows. Unchecked groups remain available for selection but have no saved position.
const selectedGroups = computed(() => groups.value.filter((group) => group.enabled))

/**
 * moveGroup swaps a selected group with its previous or next selected neighbor.
 * Unchecked rows are skipped, endpoint moves are ignored, and saving locks the
 * draft until the request completes. A successful move clears the saved message.
 */
function moveGroup(groupId: number, direction: -1 | 1) {
  if (saving.value) return
  const selectedIndex = selectedGroups.value.findIndex((group) => group.group_id === groupId)
  const neighbor = selectedGroups.value[selectedIndex + direction]
  if (selectedIndex < 0 || !neighbor) return
  const from = groups.value.findIndex((group) => group.group_id === groupId)
  const to = groups.value.findIndex((group) => group.group_id === neighbor.group_id)
  const reordered = [...groups.value]
  reordered[from] = groups.value[to]
  reordered[to] = groups.value[from]
  groups.value = reordered
  saved.value = false
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await getChannelMonitorConfig(controller.signal)
    if (disposed) return
    groups.value = result.map((group) => ({ ...group }))
    loaded.value = true
  } catch {
    if (!disposed) error.value = t('channelMonitor.configLoadFailed')
  } finally {
    if (!disposed) loading.value = false
  }
}

async function save() {
  if (!loaded.value || saving.value) return
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    await saveChannelMonitorConfig(
      groups.value
        .filter((group) => group.enabled)
        .map(({ group_id, enabled, status }) => ({ group_id, enabled, status })),
    )
    if (disposed) return
    saved.value = true
    emit('saved')
  } catch {
    if (!disposed) error.value = t('channelMonitor.configSaveFailed')
  } finally {
    if (!disposed) saving.value = false
  }
}

onMounted(load)
onBeforeUnmount(() => {
  disposed = true
  controller.abort()
})
</script>

<template>
  <section
    class="mb-4 rounded-surface border border-gray-200 bg-white p-4 dark:border-dark-500 dark:bg-dark-800"
    :aria-label="t('channelMonitor.configure')"
  >
    <h2 class="text-lg font-semibold">{{ t('channelMonitor.configure') }}</h2>
    <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{
      t('channelMonitor.configHint')
    }}</p>
    <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('channelMonitor.orderHint') }}</p>
    <div v-if="loading" role="status" class="mt-4">{{ t('channelMonitor.loading') }}</div>
    <div v-if="error" role="alert" class="mt-4 text-red-600 dark:text-red-400">{{ error }}</div>
    <button v-if="!loading && !loaded" type="button" class="btn btn-secondary mt-4" @click="load">{{
      t('channelMonitor.refresh')
    }}</button>
    <form v-if="loaded" class="mt-4" @submit.prevent="save" @change="saved = false">
      <fieldset :disabled="saving" class="space-y-4">
        <p v-if="!groups.length" class="text-sm text-gray-500">{{
          t('channelMonitor.noGroups')
        }}</p>
        <div
          v-for="group in groups"
          :key="group.group_id"
          class="flex flex-col gap-2 border-b border-gray-100 pb-4 dark:border-dark-600 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="min-w-0">
            <label class="flex items-center gap-2">
              <input v-model="group.enabled" type="checkbox" :data-group-id="group.group_id" />
              <span class="break-words"
                >{{ group.name }} <span class="text-sm text-gray-500">{{ group.brand }}</span></span
              >
            </label>
            <p v-if="!group.eligible" class="mt-2 text-sm text-amber-700 dark:text-amber-400">{{
              t('channelMonitor.ineligible')
            }}</p>
          </div>
          <div class="flex items-center gap-2 sm:shrink-0">
            <div class="flex shrink-0 gap-2">
              <button
                type="button"
                class="btn btn-ghost btn-icon"
                :disabled="
                  saving || !group.enabled || group.group_id === selectedGroups[0]?.group_id
                "
                :title="t('channelMonitor.moveUp', { name: group.name })"
                :aria-label="t('channelMonitor.moveUp', { name: group.name })"
                @click="moveGroup(group.group_id, -1)"
              >
                <Icon name="arrowUp" size="sm" :animate-on-hover="false" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-icon"
                :disabled="
                  saving ||
                  !group.enabled ||
                  group.group_id === selectedGroups[selectedGroups.length - 1]?.group_id
                "
                :title="t('channelMonitor.moveDown', { name: group.name })"
                :aria-label="t('channelMonitor.moveDown', { name: group.name })"
                @click="moveGroup(group.group_id, 1)"
              >
                <Icon name="arrowDown" size="sm" :animate-on-hover="false" />
              </button>
            </div>
            <Select
              v-model="group.status"
              :options="options"
              :disabled="saving || !group.enabled"
              :aria-label="t('channelMonitor.groupStatus', { name: group.name })"
              class="min-w-0 flex-1 sm:w-56"
              @update:model-value="saved = false"
            />
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="submit" class="btn btn-primary" :disabled="saving">{{
            t(saving ? 'channelMonitor.saving' : 'channelMonitor.save')
          }}</button>
          <span v-if="saved" role="status" class="text-sm text-green-700 dark:text-green-400">{{
            t('channelMonitor.saved')
          }}</span>
        </div>
      </fieldset>
    </form>
  </section>
</template>
