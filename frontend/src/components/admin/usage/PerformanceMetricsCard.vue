<template>
  <div class="card p-4" data-testid="performance-metrics-card">
    <div class="flex flex-col gap-2 lg:flex-row lg:items-center">
      <div class="shrink-0 self-start rounded-control bg-violet-100 p-2 dark:bg-violet-900/30">
        <Icon
          name="bolt"
          size="md"
          class="text-violet-600 dark:text-violet-400"
          :stroke-width="2"
        />
      </div>
      <div class="min-w-0 flex-1">
        <p
          class="text-xs font-medium text-gray-500 dark:text-gray-400"
          :title="t('admin.dashboard.performanceHint')"
        >
          {{ t('admin.dashboard.performance') }}
        </p>
        <div class="mt-0.5 flex items-baseline gap-2">
          <p
            class="whitespace-nowrap text-lg font-bold tabular-nums text-gray-900 dark:text-white lg:text-xl"
            data-testid="performance-rpm"
          >
            {{ metrics ? formatTokens(metrics.rpm) : '—' }}
          </p>
          <span class="text-xs text-gray-500 dark:text-gray-400">RPM</span>
        </div>
        <div class="mt-0.5 flex items-baseline gap-2">
          <p
            class="whitespace-nowrap text-sm font-semibold tabular-nums text-violet-600 dark:text-violet-400"
            data-testid="performance-tpm"
          >
            {{ metrics ? formatTokens(metrics.tpm) : '—' }}
          </p>
          <span class="text-xs text-gray-500 dark:text-gray-400">TPM</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DashboardStats } from '@/api/admin/dashboard.types'
import Icon from '@/components/icons/Icon.vue'
import { formatTokens } from '@/utils/format'

// PerformanceMetricsCard.vue displays the administrator dashboard's global request
// and text-token rates in a shared card. Both administrator pages pass the same
// snapshot fields for the preceding minute; the component formats those measurements
// and shows missing observations separately from measured zero traffic.

defineProps<{
  // metrics contains the latest successful dashboard rates; null displays a dash
  // until the parent has received an observation from the snapshot endpoint.
  metrics: Pick<DashboardStats, 'rpm' | 'tpm'> | null
}>()

// Shared dashboard translations label the rates and explain their measurement
// window consistently wherever this administrator summary card is mounted.
const { t } = useI18n()
</script>
