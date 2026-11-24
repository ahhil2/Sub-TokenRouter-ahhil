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
          :title="
            t(
              periodAverage
                ? 'admin.dashboard.periodPerformanceHint'
                : 'admin.dashboard.performanceHint',
            )
          "
        >
          {{
            t(periodAverage ? 'admin.dashboard.periodPerformance' : 'admin.dashboard.performance')
          }}
        </p>
        <div class="mt-0.5 flex items-baseline gap-2">
          <p
            class="whitespace-nowrap text-lg font-bold tabular-nums text-gray-900 dark:text-white lg:text-xl"
            data-testid="performance-rpm"
          >
            {{ metrics ? formatRate(metrics.rpm) : '—' }}
          </p>
          <span class="text-xs text-gray-500 dark:text-gray-400">RPM</span>
        </div>
        <div class="mt-0.5 flex items-baseline gap-2">
          <p
            class="whitespace-nowrap text-sm font-semibold tabular-nums text-violet-600 dark:text-violet-400"
            data-testid="performance-tpm"
          >
            {{ metrics ? formatRate(metrics.tpm) : '—' }}
          </p>
          <span class="text-xs text-gray-500 dark:text-gray-400">TPM</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { getLocale } from '@/i18n'
import type { DashboardStats } from '@/api/admin/dashboard.types'
import Icon from '@/components/icons/Icon.vue'
import { formatTokens } from '@/utils/format'

// PerformanceMetricsCard.vue displays administrator request and text-token rates in a
// shared card. The parent selects either preceding-minute dashboard measurements or
// interval averages from filtered usage statistics. Missing observations display a dash,
// while sub-unit interval averages retain significant digits so sparse traffic stays visible.

// props pairs the displayed values with their measurement mode and explanatory labels.
const props = defineProps<{
  // metrics contains the latest successful rates; null displays a dash until
  // a response provides measurements over a defined time interval.
  metrics: Pick<DashboardStats, 'rpm' | 'tpm'> | null
  // periodAverage selects filtered interval-average labels and fractional precision.
  // Omission displays the dashboard's whole-site rates for the preceding minute.
  periodAverage?: boolean
}>()

// Shared dashboard translations label the rates and explain their measurement
// window consistently wherever this administrator summary card is mounted.
const { t } = useI18n()

// formatRate renders large measurements with compact suffixes and keeps three
// significant digits for positive interval averages below one. Dashboard counts
// retain their ordinary locale formatting, while zero remains visibly distinct.
const formatRate = (value: number) =>
  props.periodAverage && value > 0 && value < 1
    ? value.toLocaleString(getLocale(), { maximumSignificantDigits: 3 })
    : formatTokens(value)
</script>
