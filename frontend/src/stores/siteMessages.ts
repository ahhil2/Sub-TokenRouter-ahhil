import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { useAuthStore } from './auth'
import { siteMessagesAPI } from '@/api/siteMessages'

/**
 * siteMessages.ts stores the unread message count for the current account and
 * coalesces concurrent summary refreshes. The authentication watcher increments a
 * generation whenever the account changes, clears its visible count, and releases the
 * reference to pending work.
 *
 * Each refresh captures that generation before sending its request. Response and
 * completion handlers update shared state only when their generation still belongs to
 * the current account. Failed summary requests leave the current count unchanged,
 * allowing the visible-page polling performed by MessageBell to try again later
 * without replacing the badge with an error state.
 */

export const useSiteMessagesStore = defineStore('siteMessages', () => {
  const auth = useAuthStore()
  const unreadCount = ref(0)
  let generation = 0
  let pending: Promise<void> | null = null

  watch(
    () => auth.user?.id,
    () => {
      generation++
      unreadCount.value = 0
      pending = null
    },
    { flush: 'sync' },
  )

  function refresh(): Promise<void> {
    if (!auth.user) return Promise.resolve()
    if (pending) return pending
    const current = generation
    pending = siteMessagesAPI
      .summary()
      .then((result) => {
        if (current === generation) unreadCount.value = result.unread_count
      })
      .catch(() => {})
      .finally(() => {
        if (current === generation) pending = null
      })
    return pending
  }

  return { unreadCount, refresh }
})
