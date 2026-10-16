<template>
  <BaseDialog :show="show" :title="t('identityGroups.assign')" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-sm">{{ t('identityGroups.selected', { count: userIds.length }) }}</p>
      <p class="text-sm text-gray-500">{{ t('identityGroups.replace') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="saved" role="status" class="text-sm">{{ t('identityGroups.saved') }}</p>
      <div v-for="code in codes" :key="code" class="flex flex-wrap items-center gap-2">
        <span class="mr-auto text-sm font-medium">{{ identityName(code) }}</span>
        <button class="btn btn-primary" :disabled="busy" @click="change(code, 'add')">{{ t('identityGroups.add') }}</button>
        <button class="btn btn-secondary" :disabled="busy" @click="change(code, 'remove')">{{ t('identityGroups.remove') }}</button>
      </div>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { changeIdentityMembers, identityName, type IdentityCode } from '@/api/identityGroups'
const props = defineProps<{ show: boolean; userIds: number[] }>()
const emit = defineEmits<{ close: []; success: [] }>()
const { t } = useI18n()
const codes: IdentityCode[] = ['bonus_5', 'bonus_10', 'ahhilai']
const busy = ref(false)
const error = ref('')
const saved = ref(false)
watch(() => props.show, () => { error.value = ''; saved.value = false })
async function change(code: IdentityCode, action: 'add' | 'remove') {
  busy.value = true
  error.value = ''
  saved.value = false
  try { await changeIdentityMembers(code, props.userIds, action); saved.value = true; emit('success') }
  catch { error.value = t('identityGroups.failed') }
  finally { busy.value = false }
}
</script>
