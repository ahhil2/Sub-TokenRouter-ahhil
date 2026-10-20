<template>
  <BaseDialog :show="show" :title="t('admin.providers.displayOrder')" width="normal" @close="close">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.providers.displayOrderHint') }}</p>
      <p v-if="loading" role="status">{{ t('common.loading') }}</p>
      <p v-else-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <VueDraggable v-else v-model="providers" :animation="200" :disabled="saving" handle=".provider-sort-handle" class="space-y-2" data-testid="provider-sort-list">
        <div v-for="(provider, index) in providers" :key="provider.id" :data-provider-id="provider.id" class="flex items-center gap-3 rounded-surface border border-gray-200 p-3 dark:border-dark-600">
          <span class="provider-sort-handle flex h-9 w-9 shrink-0 touch-none cursor-grab items-center justify-center text-gray-400 active:cursor-grabbing"><Icon name="menu" size="md" class="pointer-events-none" :animate-on-hover="false" /></span>
          <span class="min-w-0 flex-1 break-words text-sm font-medium">{{ provider.name }}</span>
          <span class="text-xs text-gray-400">#{{ provider.id }}</span>
          <button type="button" class="btn btn-secondary btn-icon" :disabled="saving || index === 0" :aria-label="t('admin.providers.moveUp')" @click="move(index, -1)"><Icon name="chevronUp" size="sm" /></button>
          <button type="button" class="btn btn-secondary btn-icon" :disabled="saving || index === providers.length - 1" :aria-label="t('admin.providers.moveDown')" @click="move(index, 1)"><Icon name="chevronDown" size="sm" /></button>
        </div>
      </VueDraggable>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="loading || saving || !!error || !providers.length" data-testid="provider-sort-save" @click="save">{{ t(saving ? 'common.saving' : 'common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { providersAPI } from '@/api/admin/providers'
import { useAppStore } from '@/stores/app'
import type { Provider } from '@/types'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const providers = ref<Provider[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')

watch(() => props.show, async (show, _previousShow, onCleanup) => {
  if (!show) return
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  loading.value = true
  error.value = ''
  providers.value = []
  try {
    const loaded: Provider[] = []
    let page = 1
    while (true) {
      const result = await providersAPI.list(page, 100, { sort_by: 'display_order', sort_order: 'asc', lite: '1' }, { signal: controller.signal })
      if (controller.signal.aborted) return
      if (result.total > 10000) throw new Error('provider order limit exceeded')
      loaded.push(...result.items)
      if (loaded.length >= result.total) break
      if (!result.items.length || page >= 100) throw new Error('incomplete provider list')
      page += 1
    }
    providers.value = loaded
  } catch {
    if (!controller.signal.aborted) error.value = t('admin.providers.displayOrderLoadFailed')
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}, { immediate: true })

const close = () => {
  if (!saving.value) emit('close')
}

const move = (index: number, offset: number) => {
  const next = index + offset
  if (saving.value || next < 0 || next >= providers.value.length) return
  const provider = providers.value.splice(index, 1)[0]
  if (provider) providers.value.splice(next, 0, provider)
}

const save = async () => {
  if (saving.value || loading.value || error.value || !providers.value.length) return
  saving.value = true
  try {
    await providersAPI.updateDisplayOrder(providers.value.map(provider => provider.id))
    appStore.showSuccess(t('admin.providers.displayOrderSaved'))
    emit('saved')
  } catch {
    appStore.showError(t('admin.providers.displayOrderSaveFailed'))
  } finally {
    saving.value = false
  }
}
</script>
