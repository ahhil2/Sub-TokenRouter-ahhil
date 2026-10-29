<template>
  <div
    v-if="groups && groups.length > 0"
    class="flex w-56 max-w-full flex-col items-start gap-1"
    data-testid="provider-groups"
  >
    <GroupBadge
      v-for="group in groups"
      :key="group.id"
      :name="group.name"
      :display-brand="group.display_brand"
      :rate-multiplier="group.rate_multiplier"
      :show-rate="false"
      :title="group.name"
      class="max-w-full"
    />
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import GroupBadge from '@/components/common/GroupBadge.vue'
import type { Group } from '@/api/groups.types'

/**
 * ProviderGroupsCell.vue renders every assigned provider group in a vertical
 * badge list. Full group names remain available through titles, rate labels are
 * hidden in this table column, and empty assignments display a placeholder.
 */

defineProps<{
  groups: Group[] | null | undefined
}>()
</script>
