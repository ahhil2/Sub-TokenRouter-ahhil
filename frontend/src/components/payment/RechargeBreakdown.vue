<template>
  <dl v-if="snapshot" class="grid gap-2 text-sm sm:grid-cols-3">
    <div
      ><dt class="text-gray-500">{{ t('identityGroups.base') }}</dt
      ><dd>{{ format(snapshot.base_amount) }}</dd></div
    >
    <div
      ><dt class="text-gray-500"
        >{{ t('identityGroups.gift') }} ({{ snapshot.bonus_rate * 100 }}%)</dt
      ><dd>{{ format(snapshot.bonus_amount) }}</dd></div
    >
    <div
      ><dt class="text-gray-500">{{ t('identityGroups.credited') }}</dt
      ><dd class="font-medium">{{ format(snapshot.credited_amount) }}</dd></div
    >
  </dl>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RechargeSnapshot } from '@/api/identityGroups'

/**
 * RechargeBreakdown.vue displays the base amount, bonus, and credited total from a
 * recharge snapshot. Formatting follows the active locale with up to eight fractional
 * digits, and absent historical snapshots do not produce an empty breakdown.
 */

defineProps<{ snapshot?: RechargeSnapshot | null }>()
const { t, locale } = useI18n()
const format = (amount: number) =>
  new Intl.NumberFormat(locale.value, { maximumFractionDigits: 8 }).format(amount)
</script>
