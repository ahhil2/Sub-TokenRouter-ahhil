import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import announcementsAPI from '@/api/announcements'
import type { UserAnnouncement } from '@/api/announcements.types'

// announcements.ts keeps the visible announcement list and sequences unread popup
// announcements for the application shell. Fetch admission records its timestamp before
// requesting data, so ordinary concurrent calls are throttled. A forced fetch bypasses
// that timestamp, and a failed fetch resets it to allow a retry.
//
// Popup candidates come from the first twenty server-ordered records. The queue avoids
// duplicate identifiers, while a session-local set records announcements when displayed.
// Dismissal clears the active popup before marking it read; the component's close
// callback normally continues playback after its exit lifecycle. Fetch admission can
// also start playback whenever currentPopup is empty, including during exit. Reset
// invalidates publication by requests admitted under the previous session.

// Announcement admission uses a twenty-minute ordinary-fetch window and considers at
// most twenty server-ordered records when building popup candidates.
const THROTTLE_MS = 20 * 60 * 1000 // 20 minutes
const MAX_POPUP_CANDIDATES = 20

// useAnnouncementStore exposes the visible announcement list and popup playback
// controls. The application shell owns refresh triggers and session reset, while the
// popup component reports completed closure for normal dismissal continuation. Fetch
// admission can also start playback while the current popup reference is empty.
export const useAnnouncementStore = defineStore('announcements', () => {
  // Announcement refs retain the visible list, request state, admission timestamp,
  // queued popups, and active popup for shared application consumers.
  const announcements = ref<UserAnnouncement[]>([])
  // These counters belong to the current session. Reset detaches older work,
  // while latestFetch prevents an older forced response from replacing a newer list.
  let pendingOperations = 0
  const loading = ref(false)
  let session = 0
  let latestFetch = 0
  const lastFetchTime = ref(0)
  const popupQueue = ref<UserAnnouncement[]>([])
  const currentPopup = ref<UserAnnouncement | null>(null)

  // shownPopupIds belongs to this store session and records identifiers when displayed;
  // reset replaces the set when authentication ends.
  let shownPopupIds = new Set<number>()

  // unreadCount derives the unread badge from every visible announcement, independently
  // from the smaller set considered for popup display.
  const unreadCount = computed(() => announcements.value.filter((a) => !a.read_at).length)

  // fetchAnnouncements refreshes the visible list and popup records unless the twenty-minute admission window is active. Force bypasses throttling and is forwarded to the
  // API cache controls. Admission records its timestamp before awaiting the request so
  // ordinary concurrent calls return early. Success updates existing queued records by
  // identifier before admitting candidates. Failure resets the timestamp for retry and
  // is logged. Only the latest admitted list request may publish its result.
  // Session ownership also guards loading cleanup so an old account's completion
  // cannot clear activity belonging to the newly authenticated account.
  async function fetchAnnouncements(force = false) {
    const now = Date.now()
    if (!force && lastFetchTime.value > 0 && now - lastFetchTime.value < THROTTLE_MS) {
      return
    }
    // Record admission before awaiting the API so another ordinary call during the
    // request observes the throttle window and returns early.
    lastFetchTime.value = now
    const owner = session
    const request = ++latestFetch
    pendingOperations++
    loading.value = true

    try {
      const all = await announcementsAPI.list(false, force)
      // A reset or newer fetch transfers publication ownership; discard this
      // response before it can restore another account's list or popup state.
      if (owner !== session || request !== latestFetch) return
      announcements.value = all
      if (currentPopup.value)
        currentPopup.value = all.find((item) => item.id === currentPopup.value?.id) || null
      popupQueue.value = popupQueue.value.flatMap((item) => {
        const updated = all.find((candidate) => candidate.id === item.id)
        return updated ? [updated] : []
      })
      enqueueNewPopups()
    } catch (err: any) {
      // Reset failed admission so a subsequent ordinary call can retry immediately
      // instead of waiting for the twenty-minute throttle window.
      if (owner !== session || request !== latestFetch) return
      lastFetchTime.value = 0
      console.error('Failed to fetch announcements:', err)
    } finally {
      if (owner === session) loading.value = --pendingOperations > 0
    }
  }

  // enqueueNewPopups admits unread popup records from the first twenty server-ordered
  // announcements. The candidate limit applies before filtering, so lower-ranked popup
  // records cannot enter merely because earlier records are silent or already read. The
  // session set excludes previously displayed identifiers, and the existing queue
  // excludes duplicate pending entries. If no popup is active after admission, playback
  // starts immediately with the first queued record.
  function enqueueNewPopups() {
    // Limit the server-ordered candidate window before unread and display-mode
    // filtering, keeping popup eligibility tied to announcement priority rather than
    // filtered-list size.
    const newPopups = announcements.value
      .slice(0, MAX_POPUP_CANDIDATES)
      .filter((a) => a.notify_mode === 'popup' && !a.read_at && !shownPopupIds.has(a.id))
    if (newPopups.length === 0) return

    for (const p of newPopups) {
      if (!popupQueue.value.some((q) => q.id === p.id)) {
        popupQueue.value.push(p)
      }
    }

    if (!currentPopup.value) {
      showNextPopup()
    }
  }

  // showNextPopup selects the next queued record and marks its identifier shown for this
  // store session. An empty queue clears the active popup. Fetch admission invokes it
  // when no popup is active, while the component close callback uses it to continue
  // playback after dismissal.
  function showNextPopup() {
    if (popupQueue.value.length === 0) {
      currentPopup.value = null
      return
    }
    currentPopup.value = popupQueue.value.shift()!
    shownPopupIds.add(currentPopup.value.id)
  }

  // dismissPopup clears the active popup and starts an asynchronous mark-read request.
  // The component normally continues playback through onPopupClosed after its exit
  // lifecycle. A concurrent fetch can admit candidates and start playback while the
  // active reference is empty, before that close callback runs.
  async function dismissPopup() {
    if (!currentPopup.value) return
    const id = currentPopup.value.id
    // Clear the active reference to trigger component exit before read persistence
    // settles. Fetch admission can use this empty reference to start another popup.
    currentPopup.value = null
    markAsRead(id)
  }

  // onPopupClosed advances the queue after the popup component finishes closing. The
  // component also calls it when unmounted during exit, and an empty queue requires no
  // further action.
  function onPopupClosed() {
    if (popupQueue.value.length > 0) {
      showNextPopup()
    }
  }

  // markAsRead persists one announcement's read status and updates a matching visible
  // record after success. Failures are logged and suppressed, leaving the existing read
  // timestamp available to consumers.
  async function markAsRead(id: number) {
    try {
      await persistRead(id, session)
    } catch (err: any) {
      // Keep the existing local read timestamp when persistence fails; popup dismissal
      // can still complete and the next fetch can reconcile server state.
      console.error('Failed to mark announcement as read:', err)
    }
  }

  // persistRead acknowledges one announcement and publishes its successful timestamp.
  // The captured session must still own the store after persistence finishes;
  // otherwise the response belongs to an account whose visible state was cleared.
  async function persistRead(id: number, owner: number) {
    await announcementsAPI.markRead(id)
    if (owner !== session) return
    const record = announcements.value.find((item) => item.id === id)
    if (record && !record.read_at) record.read_at = new Date().toISOString()
  }

  // markAllAsRead acknowledges the currently unread list in groups of four requests.
  // Successful timestamps publish individually, so one failed request does not
  // hide successful persistence for other announcements. The first failure is
  // propagated after the remaining groups finish. A session reset stops admission
  // of further groups and prevents pending acknowledgments from changing the
  // replacement account's list; already dispatched requests retain their transport.
  // Loading counts only work admitted under the current session.
  async function markAllAsRead() {
    const unread = announcements.value.filter((a) => !a.read_at)
    if (unread.length === 0) return

    const owner = session
    let failure: PromiseRejectedResult | undefined
    pendingOperations++
    loading.value = true
    try {
      for (let index = 0; index < unread.length; index += 4) {
        // Check ownership before each group so logout cannot admit queued updates
        // under credentials belonging to a newly authenticated account.
        if (owner !== session) return
        const results = await Promise.allSettled(
          unread.slice(index, index + 4).map((item) => persistRead(item.id, owner)),
        )
        failure ??= results.find((result) => result.status === 'rejected')
      }
      if (owner === session && failure) throw failure.reason
    } finally {
      if (owner === session) loading.value = --pendingOperations > 0
    }
  }

  // reset clears visible announcements, popup playback, throttling, and session
  // deduplication state. It does not cancel in-flight fetches or mark-read requests,
  // whose completions are ignored after the session changes.
  function reset() {
    session++
    announcements.value = []
    lastFetchTime.value = 0
    shownPopupIds = new Set()
    popupQueue.value = []
    currentPopup.value = null
    pendingOperations = 0
    loading.value = false
  }

  return {
    announcements,
    loading,
    currentPopup,
    unreadCount,
    fetchAnnouncements,
    dismissPopup,
    onPopupClosed,
    markAsRead,
    markAllAsRead,
    reset,
  }
})
