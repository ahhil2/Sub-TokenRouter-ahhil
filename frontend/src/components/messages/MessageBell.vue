<template>
  <RouterLink
    to="/messages"
    class="header-status-icon-button relative"
    data-testid="message-bell"
    :aria-label="t('messages.unreadLabel', { count: messages.unreadCount })"
    :title="t('messages.title')"
  >
    <Icon name="chat" size="md" />
    <span
      v-if="messages.unreadCount"
      aria-hidden="true"
      class="absolute -right-1 -top-1 min-w-4 rounded-full bg-primary-600 px-1 text-center text-xs leading-4 text-white"
    >
      {{ messages.unreadCount > 99 ? '99+' : messages.unreadCount }}
    </span>
  </RouterLink>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useSiteMessagesStore } from '@/stores/siteMessages'

/**
 * MessageBell.vue displays unread messages using the shared account messaging store.
 * It refreshes while the document is visible, checks visibility on a periodic timer,
 * and removes its timer and document listener when the component unmounts.
 */

const { t } = useI18n()
const messages = useSiteMessagesStore()
let timer: ReturnType<typeof setInterval> | undefined

function refreshVisible() {
  if (document.visibilityState === 'visible') void messages.refresh()
}

onMounted(() => {
  refreshVisible()
  timer = setInterval(refreshVisible, 30000)
  document.addEventListener('visibilitychange', refreshVisible)
})

onUnmounted(() => {
  clearInterval(timer)
  document.removeEventListener('visibilitychange', refreshVisible)
})
</script>
