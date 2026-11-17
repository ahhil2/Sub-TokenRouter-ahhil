<template>
  <BaseDialog
    :show="show"
    :title="t('team.createKey')"
    :close-on-escape="!submitting"
    width="normal"
    @close="closeDialog"
  >
    <div v-if="createdKey" class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('keys.keyCreatedSuccess') }}
      </p>
      <code
        class="block break-all rounded-surface bg-gray-50 p-4 text-sm text-gray-900 dark:bg-dark-800 dark:text-gray-100"
        data-testid="created-team-key"
      >
        {{ createdKey }}
      </code>
    </div>
    <form v-else id="team-key-create-form" class="space-y-5" @submit.prevent="createKey">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('team.createKeyDescription') }}
      </p>
      <div>
        <label for="team-key-name" class="input-label">{{ t('keys.nameLabel') }}</label>
        <input
          id="team-key-name"
          v-model.trim="name"
          class="input"
          type="text"
          required
          :disabled="submitting"
          :placeholder="t('keys.namePlaceholder')"
        />
      </div>
      <div>
        <label for="team-key-group" class="input-label">{{ t('keys.groupLabel') }}</label>
        <Select
          id="team-key-group"
          v-model="groupId"
          :options="groupOptions"
          :placeholder="t('keys.selectGroup')"
          :searchable="true"
          :search-placeholder="t('keys.searchGroup')"
          :disabled="groupsLoading || submitting || !!groupsError"
        />
        <p v-if="groupsLoading" class="input-hint" role="status">{{ t('common.loading') }}</p>
        <div v-else-if="groupsError" class="mt-2 space-y-2">
          <p class="text-sm text-red-600 dark:text-red-400" role="alert">{{ groupsError }}</p>
          <button type="button" class="btn btn-secondary btn-sm" @click="loadGroups">
            {{ t('common.retry') }}
          </button>
        </div>
        <p v-else-if="!groups.length" class="input-hint">{{ t('common.noGroupsAvailable') }}</p>
      </div>
      <p v-if="formError" class="text-sm text-red-600 dark:text-red-400" role="alert">
        {{ formError }}
      </p>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeDialog">
          {{ createdKey ? t('common.close') : t('common.cancel') }}
        </button>
        <button
          v-if="createdKey"
          type="button"
          class="btn btn-primary"
          @click="copyToClipboard(createdKey)"
        >
          <Icon name="copy" size="sm" />
          {{ t('common.copy') }}
        </button>
        <button
          v-else
          form="team-key-create-form"
          type="submit"
          class="btn btn-primary"
          :disabled="submitting || groupsLoading || !!groupsError || !name.trim() || !groupId"
        >
          <Icon name="plus" size="sm" />
          {{ submitting ? t('common.saving') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userGroupsAPI } from '@/api/groups'
import type { Group } from '@/api/groups.types'
import { keysAPI } from '@/api/keys'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'

// TeamKeyCreateDialog.vue creates a named team credential using an available
// routing group and automatic billing. The caller supplies visibility within an
// authorized team context. Group choices and creation both request team scope,
// letting the server resolve the current member and the team's paying owner.
//
// Each opening clears the previous draft and secret. Request generations reject
// responses from earlier openings or an unmounted dialog, while submission locks
// dismissal and duplicate writes until the request finishes. A successful write
// exposes the returned secret for copying and notifies the parent to refresh its
// list; list retrieval cannot turn that completed creation into a retryable write.

// props supplies the visible state used by the dialog shell and its request
// lifecycle, keeping dismissal under the caller's control.
const props = defineProps<{
  // show is controlled by the caller; opening resets the draft and loads the
  // available groups in team scope.
  show: boolean
}>()
// emit reports dismissal requests separately from persisted creation so the parent
// can refresh its masked list while the new secret remains visible.
const emit = defineEmits<{
  close: [] // Parent dismissal request.
  created: [] // Successful credential creation.
}>()

// Translation, notifications, and clipboard handling follow the same account UI
// services used by the full API-key management page.
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
// Draft and result state belongs to this opening; createdKey is cleared when the
// dialog closes so the next creation starts with an empty form.
const name = ref('')
const groupId = ref<number | null>(null)
const createdKey = ref('')
const formError = ref('')
const submitting = ref(false)
// Group lookup state gates submission until the current team's choices arrive;
// a failed lookup supplies a retry action while retaining the typed name.
const groups = ref<Group[]>([])
const groupsLoading = ref(false)
const groupsError = ref('')
// requestGeneration invalidates old reads and writes after dismissal or unmount,
// preventing their results from publishing into another dialog opening.
let requestGeneration = 0

// groupOptions supplies team group labels and searchable descriptions to the selector.
const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description || undefined,
  })),
)

// loadGroups retrieves groups available for automatic billing in team scope.
// A retry clears the selection while retaining the name, and failures leave
// creation unavailable until a later lookup succeeds.
//
// Each lookup captures the current request generation before awaiting the API.
// Closing, reopening, or unmounting invalidates that ownership so late responses
// cannot replace another opening's choices or clear its loading indicator. Model
// expansion is omitted because selection uses group labels and searchable descriptions.
const loadGroups = async () => {
  if (!props.show || submitting.value) return
  // Replace selection and observations before starting the read so a retry cannot
  // submit a group retained from an earlier available-group response.
  const generation = ++requestGeneration
  groups.value = []
  groupId.value = null
  groupsError.value = ''
  groupsLoading.value = true
  try {
    const available = await userGroupsAPI.getAvailable('team', undefined, false)
    if (generation === requestGeneration) groups.value = available
  } catch (error: any) {
    // Expose the current lookup failure and a retry control; stale failures belong
    // to dismissed openings and must not replace the current form's feedback.
    if (generation === requestGeneration) {
      groupsError.value = error?.message || t('team.keyGroupsLoadFailed')
    }
  } finally {
    if (generation === requestGeneration) groupsLoading.value = false
  }
}

// createKey validates the current name and group before creating a team credential.
// Pending or completed submissions reject repeated calls, and errors keep the draft
// available for correction. Automatic billing uses the team's payer configuration.
//
// The request captures dialog ownership before persistence. A valid completion
// displays its secret and emits creation separately from the parent's list refresh,
// so a later read failure cannot cause a duplicate credential. Unmounted or replaced
// openings discard the response and cannot publish feedback into another form.
const createKey = async () => {
  if (
    !props.show ||
    submitting.value ||
    createdKey.value ||
    groupsLoading.value ||
    groupsError.value
  )
    return
  formError.value = ''
  if (!name.value.trim()) {
    formError.value = t('team.keyNameRequired')
    return
  }
  if (!groups.value.some((group) => group.id === groupId.value)) {
    formError.value = t('keys.groupRequired')
    return
  }
  const generation = requestGeneration
  submitting.value = true
  try {
    const key = await keysAPI.createWithPayload({
      name: name.value.trim(),
      scope: 'team',
      group_id: groupId.value,
      billing_mode: 'auto',
    })
    // A completed write belongs to its opening; ignore publication after navigation
    // so the returned secret cannot appear in a replacement dialog.
    if (generation !== requestGeneration) return
    createdKey.value = key.key
    appStore.showSuccess(t('keys.keyCreatedSuccess'))
    emit('created')
  } catch (error: any) {
    // Keep the submitted draft when creation fails so the user can correct it;
    // obsolete requests cannot overwrite feedback belonging to another opening.
    if (generation === requestGeneration) {
      formError.value = error?.message || t('keys.failedToSave')
    }
  } finally {
    if (generation === requestGeneration) submitting.value = false
  }
}

// closeDialog requests dismissal after the creation request has settled. Pending
// writes retain the dialog so their eventual secret or failure remains visible
// and cannot be followed by a second submission from a reopened form.
const closeDialog = () => {
  if (!submitting.value) emit('close')
}

// Visibility changes invalidate pending lookups before clearing secret and draft
// state, ensuring reopened dialogs cannot receive results from an earlier opening.
watch(
  () => props.show,
  (show) => {
    requestGeneration += 1
    createdKey.value = ''
    if (!show) return
    name.value = ''
    groupId.value = null
    formError.value = ''
    submitting.value = false
    void loadGroups()
  },
  { immediate: true },
)

// Navigation invalidates publication ownership even while network requests continue,
// preventing detached dialogs from emitting a secret or stale creation notification.
onBeforeUnmount(() => {
  requestGeneration += 1
})
</script>
