import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { useAuthStore } from './auth'
import { siteMessagesAPI } from '@/api/siteMessages'

export const useSiteMessagesStore = defineStore('siteMessages', () => {
  const auth = useAuthStore()
  const unreadCount = ref(0)
  let generation = 0
  let pending: Promise<void> | null = null

  watch(() => auth.user?.id, () => {
    generation++
    unreadCount.value = 0
    pending = null
  }, { flush: 'sync' })

  function refresh(): Promise<void> {
    if (!auth.user) return Promise.resolve()
    if (pending) return pending
    const current = generation
    pending = siteMessagesAPI.summary().then(result => {
      if (current === generation) unreadCount.value = result.unread_count
    }).catch(() => {}).finally(() => {
      if (current === generation) pending = null
    })
    return pending
  }

  return { unreadCount, refresh }
})
