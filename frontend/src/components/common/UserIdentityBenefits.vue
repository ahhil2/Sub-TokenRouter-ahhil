<template>
  <div class="space-y-2 rounded-control bg-gray-50 p-3 dark:bg-dark-800">
    <p class="text-sm font-medium">{{ t('identityGroups.benefits') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-else-if="!loaded" class="text-sm text-gray-500">{{ t('identityGroups.loading') }}</p>
    <template v-else>
      <span v-for="code in codes" :key="code" class="badge badge-primary mr-1">{{
        identityName(code)
      }}</span>
      <p v-if="codes.length === 0" class="text-sm text-gray-500">{{ t('identityGroups.none') }}</p>
      <p v-if="bonusRate > 0" class="text-sm">{{
        t('identityGroups.bonus', { percent: bonusRate * 100 })
      }}</p>
      <p v-if="bonusRate > 0" class="text-xs text-gray-500"
        >{{ t('identityGroups.snapshot') }} {{ t('identityGroups.refund') }}</p
      >
    </template>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getMyIdentities, identityName, type IdentityCode } from '@/api/identityGroups'

/**
 * UserIdentityBenefits.vue loads the signed-in account identities and recharge bonus
 * rate. A successful response supplies identity labels and emits the rate to its
 * parent; a failed request displays an error instead of announcing loaded benefits.
 */

const { t } = useI18n()
const emit = defineEmits<{ loaded: [rate: number] }>()
const codes = ref<IdentityCode[]>([])
const bonusRate = ref(0)
const loaded = ref(false)
const error = ref('')
onMounted(async () => {
  try {
    const result = await getMyIdentities()
    codes.value = result.identity_groups
    bonusRate.value = result.bonus_rate
    loaded.value = true
    emit('loaded', result.bonus_rate)
  } catch {
    error.value = t('identityGroups.failed')
  }
})
</script>
