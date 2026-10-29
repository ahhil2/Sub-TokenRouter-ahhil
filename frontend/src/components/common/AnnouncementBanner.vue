<template>
  <aside
    v-if="banners.length"
    class="announcement-banner max-h-56 min-w-0 flex-1 self-center overflow-y-auto rounded-surface border border-primary-200 bg-primary-50 px-4 py-3 text-gray-900 dark:border-primary-800 dark:bg-primary-950/40 dark:text-dark-50"
    :aria-label="t('announcements.title')"
    data-testid="page-announcement"
  >
    <article v-for="announcement in banners" :key="announcement.id" class="space-y-2">
      <h2 class="font-bold">{{ announcement.title }}</h2>
      <div
        class="announcement-banner-content"
        v-html="renderAnnouncementBanner(announcement.content)"
      ></div>
    </article>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAnnouncementStore } from '@/stores/announcements'
import { renderAnnouncementBanner } from '@/utils/announcementBanner'

/**
 * AnnouncementBanner.vue displays announcements configured for banner delivery through
 * the shared sanitized renderer. Filtering belongs to this display component, while
 * announcement loading and read state remain in the store used by other announcement
 * interfaces.
 */

const { t } = useI18n()
const store = useAnnouncementStore()
const banners = computed(() =>
  store.announcements.filter((announcement) => announcement.notify_mode === 'banner'),
)
</script>

<style scoped>
.announcement-banner {
  flex-basis: 20rem;
  font-family:
    'Times New Roman', 'SimSun', '宋体', 'Songti SC', 'Noto Serif CJK SC', 'Noto Sans SC Variable',
    serif;
  font-size: 18px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.announcement-banner article + article {
  margin-top: 16px;
}
.announcement-banner-content :deep(p + p) {
  margin-top: 8px;
}
.announcement-banner-content :deep(ul),
.announcement-banner-content :deep(ol) {
  padding-left: 24px;
}
.announcement-banner-content :deep(ul) {
  list-style: disc;
}
.announcement-banner-content :deep(ol) {
  list-style: decimal;
}
</style>
