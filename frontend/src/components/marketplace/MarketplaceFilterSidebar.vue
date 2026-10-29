<template>
  <aside
    class="card min-w-0 self-start font-sans xl:sticky xl:top-[calc(var(--header-h)+1rem)]"
    :aria-label="t('marketplace.filtersTitle')"
    data-testid="marketplace-filter-sidebar"
  >
    <div class="flex items-center justify-between gap-2 p-4">
      <h2 class="hidden text-sm font-semibold text-gray-900 dark:text-dark-50 xl:block">
        {{ t('marketplace.filtersTitle') }}
      </h2>
      <button
        type="button"
        class="flex min-w-0 items-center gap-2 rounded-control text-sm font-semibold text-gray-900 focus-visible:outline-primary-500 dark:text-dark-50 xl:hidden"
        :aria-expanded="mobileExpanded"
        :aria-controls="panelId"
        data-testid="marketplace-filter-toggle"
        @click="mobileExpanded = !mobileExpanded"
      >
        <Icon name="filter" size="sm" />
        {{ t('marketplace.filtersTitle') }}
        <span
          v-if="activeCount"
          class="rounded-full bg-primary-50 px-2 text-xs text-primary-700 dark:bg-primary-500/10 dark:text-primary-300"
          >{{ activeCount }}</span
        >
        <Icon name="chevronDown" size="sm" :class="mobileExpanded ? 'rotate-180' : ''" />
      </button>
      <button
        type="button"
        class="btn btn-ghost btn-sm shrink-0"
        :disabled="!activeCount"
        data-testid="marketplace-sidebar-reset"
        @click="$emit('reset')"
      >
        {{ t('common.reset') }}
      </button>
    </div>

    <div
      :id="panelId"
      class="max-h-[65dvh] space-y-4 overflow-y-auto px-4 pb-4 xl:max-h-[calc(100dvh-var(--header-h)-6rem)] xl:block"
      :class="mobileExpanded ? 'block' : 'hidden'"
      data-testid="marketplace-filter-panel"
    >
      <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{
        t('marketplace.filtersHint')
      }}</p>

      <section class="space-y-2" :aria-label="t('marketplace.filterProvider')">
        <h3 class="text-xs font-semibold text-gray-600 dark:text-dark-300">{{
          t('marketplace.filterProvider')
        }}</h3>
        <button
          type="button"
          class="marketplace-filter-option"
          :aria-pressed="brand === 'all'"
          data-testid="marketplace-provider-all"
          @click="brand = 'all'"
        >
          <span class="min-w-0 flex-1 text-left">{{ t('marketplace.allProviders') }}</span>
          <span class="marketplace-filter-count">{{ totalGroups }}</span>
        </button>
        <button
          v-for="option in providers"
          :key="option.value"
          type="button"
          class="marketplace-filter-option"
          :aria-pressed="brand === option.value"
          :data-provider="option.value"
          data-testid="marketplace-provider-option"
          @click="brand = option.value"
        >
          <ProviderIcon :brand="option.value" size="16px" />
          <span class="min-w-0 flex-1 truncate text-left" :title="option.label">{{
            option.label
          }}</span>
          <span class="marketplace-filter-count">{{ option.count }}</span>
        </button>
      </section>

      <section
        class="space-y-2 border-t border-gray-100 pt-4 dark:border-dark-700"
        :aria-label="t('marketplace.filterGroup')"
      >
        <h3 class="text-xs font-semibold text-gray-600 dark:text-dark-300">{{
          t('marketplace.filterGroup')
        }}</h3>
        <button
          type="button"
          class="marketplace-filter-option"
          :aria-pressed="groupId === 'all'"
          data-testid="marketplace-group-all"
          @click="groupId = 'all'"
        >
          <span class="min-w-0 flex-1 text-left">{{ t('marketplace.allGroups') }}</span>
          <span class="marketplace-filter-count">{{ groups.length }}</span>
        </button>
        <button
          v-for="group in groups"
          :key="group.id"
          type="button"
          class="marketplace-filter-option"
          :aria-pressed="groupId === group.id"
          :data-group-id="group.id"
          data-testid="marketplace-group-option"
          @click="groupId = group.id"
        >
          <span class="min-w-0 flex-1 break-words text-left [overflow-wrap:anywhere]">{{
            group.name
          }}</span>
          <span
            class="marketplace-filter-count"
            :aria-label="
              t('marketplace.rateMultiplierValue', { multiplier: `×${group.rate_multiplier}` })
            "
            >×{{ group.rate_multiplier }}</span
          >
        </button>
      </section>

      <section
        class="space-y-2 border-t border-gray-100 pt-4 dark:border-dark-700"
        :aria-label="t('marketplace.filterPricingMode')"
      >
        <h3 class="text-xs font-semibold text-gray-600 dark:text-dark-300">{{
          t('marketplace.filterPricingMode')
        }}</h3>
        <button
          v-for="option in pricingOptions"
          :key="option.value"
          type="button"
          class="marketplace-filter-option"
          :aria-pressed="pricing === option.value"
          :data-pricing-mode="option.value"
          data-testid="marketplace-pricing-option"
          @click="pricing = option.value"
        >
          {{ option.label }}
        </button>
      </section>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import type { MarketplaceGroup } from '@/api/marketplace.types'

/**
 * MarketplaceFilterSidebar.vue presents provider, group, and pricing-category filters
 * supplied by the marketplace page. Selection values are parent-owned models, while
 * local state controls mobile expansion; the reset event lets the page clear its
 * complete filter state.
 */

type PricingFilter = 'all' | 'token' | 'image' | 'video' | 'unpriced'

defineProps<{
  providers: { value: string; label: string; count: number }[]
  groups: Pick<MarketplaceGroup, 'id' | 'name' | 'rate_multiplier'>[]
  totalGroups: number
  activeCount: number
  pricingOptions: { value: PricingFilter; label: string }[]
}>()
defineEmits<{ reset: [] }>()

const brand = defineModel<string>('brand', { required: true })
const groupId = defineModel<number | 'all'>('groupId', { required: true })
const pricing = defineModel<PricingFilter>('pricing', { required: true })
const { t } = useI18n()
const panelId = useId()
const mobileExpanded = ref(false)
</script>

<style scoped>
.marketplace-filter-option {
  @apply flex w-full items-center gap-2 rounded-control px-2 py-2 text-sm font-medium text-primary-900 transition-colors hover:bg-gray-50 hover:text-primary-700 focus-visible:outline-primary-500 dark:text-dark-100 dark:hover:bg-dark-800 dark:hover:text-primary-500;
}

.marketplace-filter-option[aria-pressed='true'] {
  @apply bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300;
}

.marketplace-filter-count {
  @apply shrink-0 rounded-compact bg-gray-100 px-2 py-1 text-xs tabular-nums text-gray-500 dark:bg-dark-700 dark:text-dark-300;
}
</style>
