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
      <p v-if="rateDiscount < 1" class="text-sm">{{
        t('identityGroups.discount', { percent: Math.round(rateDiscount * 100) })
      }}</p>
    </template>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getMyIdentities, identityName, type IdentityCode } from '@/api/identityGroups'

/**
 * UserIdentityBenefits.vue loads the signed-in account identities and their balance rate
 * discount. A discount below one is shown as the percentage of group default rates the
 * user pays on eligible groups. The message explains the original-price exception;
 * a failed request displays an error instead of announcing loaded benefits.
 */

const { t } = useI18n()
const codes = ref<IdentityCode[]>([]) // identity codes held by the signed-in user
const rateDiscount = ref(1) // factor applied to group default balance rates; 1 means none
const loaded = ref(false) // whether the identity request has succeeded
const error = ref('') // localized load failure message
onMounted(async () => {
  try {
    const result = await getMyIdentities()
    codes.value = result.identity_groups
    rateDiscount.value = result.rate_discount
    loaded.value = true
  } catch {
    error.value = t('identityGroups.failed')
  }
})
</script>
