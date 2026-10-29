<template>
  <AppLayout>
    <template #page-heading-actions>
      <button
        v-if="auth.isAdmin"
        type="button"
        class="btn btn-primary"
        @click="newMessageOpen = true"
        ><Icon name="plus" size="sm" />{{ t('messages.newMessage') }}</button
      >
    </template>
    <p class="mb-4 text-sm text-gray-500 dark:text-dark-400">{{
      auth.isAdmin ? t('messages.adminHint') : t('messages.privacyHint')
    }}</p>
    <div
      class="grid min-w-0 gap-4"
      :class="auth.isAdmin ? 'lg:grid-cols-[20rem_minmax(0,1fr)]' : ''"
    >
      <aside
        v-if="auth.isAdmin"
        class="card min-w-0 self-start overflow-hidden"
        :aria-label="t('messages.conversations')"
      >
        <div class="border-b border-gray-100 p-4 dark:border-dark-700">
          <label for="message-conversation-search" class="sr-only">{{
            t('messages.searchRecipients')
          }}</label>
          <input
            id="message-conversation-search"
            v-model="search"
            class="input"
            maxlength="128"
            :placeholder="t('messages.searchRecipients')"
          />
        </div>
        <div class="max-h-[65dvh] overflow-y-auto p-2">
          <p
            v-if="loadingConversations && !conversations.length"
            class="p-4 text-sm text-gray-500"
            >{{ t('common.loading') }}</p
          >
          <p v-else-if="!conversations.length" class="p-4 text-sm text-gray-500">{{
            t('messages.noConversations')
          }}</p>
          <button
            v-for="conversation in conversations"
            :key="conversation.user_id"
            type="button"
            class="mb-1 w-full rounded-control p-3 text-left transition-colors"
            :class="
              selectedUserID === conversation.user_id
                ? 'bg-primary-50 dark:bg-primary-500/10'
                : 'hover:bg-gray-50 dark:hover:bg-dark-800'
            "
            :aria-pressed="selectedUserID === conversation.user_id"
            @click="selectUser(conversation.user_id)"
          >
            <span class="flex min-w-0 items-center justify-between gap-2"
              ><span class="truncate text-sm font-semibold">{{
                conversation.username || conversation.email
              }}</span
              ><span
                v-if="conversation.unread_count"
                class="rounded-full bg-primary-600 px-1.5 text-xs text-white"
                >{{ conversation.unread_count }}</span
              ></span
            >
            <span
              v-if="conversation.username"
              class="mt-1 block truncate text-xs text-gray-500 dark:text-dark-400"
              >{{ conversation.email }}</span
            >
            <span class="mt-2 block truncate text-sm text-gray-500 dark:text-dark-300">{{
              conversation.body
            }}</span>
          </button>
          <button
            v-if="hasMoreConversations"
            type="button"
            class="btn btn-secondary w-full"
            :disabled="loadingConversations"
            @click="loadConversations(false)"
            >{{ t('messages.loadMore') }}</button
          >
        </div>
        <p v-if="listError" role="alert" class="p-4 text-sm text-red-500">{{ listError }}</p>
      </aside>
      <section
        class="card flex min-w-0 flex-col overflow-hidden"
        :aria-label="t('messages.conversation')"
      >
        <template v-if="selectedUserID">
          <header class="flex items-center gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300"
              ><Icon name="chat" size="lg"
            /></div>
            <div class="min-w-0"
              ><h2 class="truncate text-base font-semibold">{{ threadTitle }}</h2
              ><p class="truncate text-xs text-gray-500 dark:text-dark-400">{{
                auth.isAdmin ? contact?.email : t('messages.privateConversation')
              }}</p></div
            >
          </header>
          <div
            ref="scrollArea"
            class="h-[min(52dvh,32rem)] space-y-4 overflow-y-auto p-4 sm:p-6"
            data-testid="message-thread"
            aria-live="polite"
            aria-relevant="additions"
          >
            <div v-if="hasMoreMessages" class="text-center"
              ><button
                type="button"
                class="btn btn-secondary"
                :disabled="loadingThread"
                @click="loadThread(messages[0]?.id || 0)"
                >{{ t('messages.loadEarlier') }}</button
              ></div
            >
            <p
              v-if="loadingThread && !messages.length"
              class="py-10 text-center text-sm text-gray-500"
              >{{ t('common.loading') }}</p
            >
            <p
              v-else-if="!messages.length && !threadError"
              class="py-10 text-center text-sm text-gray-500"
              >{{ t('messages.noMessages') }}</p
            >
            <article
              v-for="message in messages"
              :key="message.id"
              class="flex"
              :class="message.is_admin === auth.isAdmin ? 'justify-end' : 'justify-start'"
              :data-message-id="message.id"
            >
              <div class="min-w-0 max-w-[88%] sm:max-w-[80%]">
                <p
                  class="mb-1 text-xs text-gray-500 dark:text-dark-400"
                  :class="{ 'text-right': message.is_admin === auth.isAdmin }"
                  >{{
                    message.is_admin
                      ? t('messages.siteOwner')
                      : auth.isAdmin
                        ? threadTitle
                        : t('messages.you')
                  }}</p
                >
                <p
                  class="whitespace-pre-wrap break-words rounded-surface px-4 py-3 text-sm leading-relaxed"
                  :class="
                    message.is_admin === auth.isAdmin
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-dark-100'
                  "
                  >{{ message.body }}</p
                >
                <p
                  class="mt-1 flex flex-wrap gap-x-2 text-xs text-gray-500 dark:text-dark-400"
                  :class="{ 'justify-end': message.is_admin === auth.isAdmin }"
                  ><time :datetime="message.created_at">{{ formatTime(message.created_at) }}</time
                  ><span v-if="message.is_admin === auth.isAdmin">{{
                    message.id <= peerReadThroughID ? t('messages.read') : t('messages.sent')
                  }}</span></p
                >
              </div>
            </article>
          </div>
          <div
            v-if="threadError"
            role="alert"
            class="flex items-center justify-between gap-3 px-4 pb-3 text-sm text-red-500"
            ><span>{{ threadError }}</span
            ><button type="button" class="btn btn-secondary" @click="loadThread()">{{
              t('messages.retry')
            }}</button></div
          >
          <div class="border-t border-gray-100 p-4 dark:border-dark-700"
            ><MessageComposer
              v-model="currentDraft"
              :disabled="!contact"
              :sending="sending"
              @submit="sendReply"
          /></div>
        </template>
        <div
          v-else
          class="flex min-h-80 flex-col items-center justify-center gap-3 p-8 text-center text-gray-500 dark:text-dark-400"
          ><Icon name="chat" size="xl" /><p class="text-sm">{{
            t('messages.selectConversation')
          }}</p
          ><button type="button" class="btn btn-primary" @click="newMessageOpen = true">{{
            t('messages.newMessage')
          }}</button></div
        >
      </section>
    </div>
    <NewMessageDialog
      v-if="newMessageOpen && auth.isAdmin"
      @close="newMessageOpen = false"
      @sent="onNewMessage"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import MessageComposer from '@/components/messages/MessageComposer.vue'
import NewMessageDialog from '@/components/messages/NewMessageDialog.vue'
import { messageErrorKey } from '@/components/messages/messageErrors'
import {
  siteMessagesAPI,
  type SiteMessage,
  type MessageContact,
  type MessageConversation,
} from '@/api/siteMessages'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useSiteMessagesStore } from '@/stores/siteMessages'

/**
 * SiteMessagesView.vue displays account conversations and sends plain-text replies
 * through the messaging API. Administrator conversation selection and ordinary user
 * access follow separate page behavior, while read positions and unread summaries
 * connect thread activity to the shared message badge.
 */

const { t, locale } = useI18n()
const auth = useAuthStore()
const app = useAppStore()
const unread = useSiteMessagesStore()
const route = useRoute()
const router = useRouter()
const selectedUserID = ref(0)
const conversations = ref<MessageConversation[]>([])
const messages = ref<SiteMessage[]>([])
const contact = ref<MessageContact | null>(null)
const search = ref('')
const peerReadThroughID = ref(0)
const hasMoreConversations = ref(false)
const hasMoreMessages = ref(false)
const loadingConversations = ref(false)
const loadingThread = ref(false)
const sending = ref(false)
const newMessageOpen = ref(false)
const listError = ref('')
const threadError = ref('')
const drafts = reactive<Record<number, string>>({})
const scrollArea = ref<HTMLElement | null>(null)
const currentDraft = computed({
  get: () => drafts[selectedUserID.value] || '',
  set: (value) => {
    drafts[selectedUserID.value] = value
  },
})
const threadTitle = computed(() =>
  auth.isAdmin
    ? contact.value?.username ||
      contact.value?.email ||
      t('messages.userNumber', { id: selectedUserID.value })
    : t('messages.siteOwner'),
)
let conversationPage = 0
let listGeneration = 0
let threadGeneration = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let retry: { key: string; id: string } | null = null

function formatTime(value: string) {
  return new Intl.DateTimeFormat(locale.value, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function selectUser(userID: number) {
  selectedUserID.value = userID
  void router.replace({ path: '/messages', query: { user: String(userID) } })
}

async function loadConversations(reset = true) {
  if (!auth.isAdmin) return
  const current = ++listGeneration
  const nextPage = reset ? 1 : conversationPage + 1
  loadingConversations.value = true
  try {
    const result = await siteMessagesAPI.conversations(search.value, nextPage)
    if (current !== listGeneration) return
    conversations.value = reset ? result.items : [...conversations.value, ...result.items]
    hasMoreConversations.value = result.has_more
    conversationPage = nextPage
    listError.value = ''
    if (!selectedUserID.value && result.items.length) selectUser(result.items[0].user_id)
  } catch (cause) {
    if (current === listGeneration) listError.value = t(messageErrorKey(cause, 'loadFailed'))
  } finally {
    if (current === listGeneration) loadingConversations.value = false
  }
}

async function loadThread(before = 0, initial = false) {
  const userID = selectedUserID.value
  if (!userID) return
  const current = ++threadGeneration
  const previousHeight = scrollArea.value?.scrollHeight || 0
  const atBottom =
    initial ||
    !scrollArea.value ||
    scrollArea.value.scrollHeight - scrollArea.value.scrollTop - scrollArea.value.clientHeight < 96
  loadingThread.value = true
  try {
    const result = await siteMessagesAPI.thread(userID, before)
    if (current !== threadGeneration || userID !== selectedUserID.value) return
    contact.value = result.contact
    peerReadThroughID.value = result.peer_read_through_id
    const knownIDs = new Set(messages.value.map((message) => message.id))
    const disconnected =
      !before &&
      messages.value.length > 0 &&
      result.items.length > 0 &&
      !result.items.some((message) => knownIDs.has(message.id))
    if (before || initial || disconnected || !messages.value.length)
      hasMoreMessages.value = result.has_more
    const merged = new Map(
      (disconnected ? [] : messages.value).map((message) => [message.id, message]),
    )
    for (const message of result.items) merged.set(message.id, message)
    messages.value = [...merged.values()].sort((first, second) => first.id - second.id)
    threadError.value = ''
    await nextTick()
    if (current !== threadGeneration || userID !== selectedUserID.value) return
    if (scrollArea.value) {
      if (before) scrollArea.value.scrollTop += scrollArea.value.scrollHeight - previousHeight
      else if (atBottom) scrollArea.value.scrollTop = scrollArea.value.scrollHeight
    }
    const latest = result.items[result.items.length - 1]?.id || 0
    if (
      !before &&
      atBottom &&
      document.visibilityState === 'visible' &&
      latest > result.read_through_id
    ) {
      await siteMessagesAPI.markRead(userID, latest)
      void unread.refresh()
      const conversation = conversations.value.find((item) => item.user_id === userID)
      if (conversation) conversation.unread_count = 0
    }
  } catch (cause) {
    if (current === threadGeneration) threadError.value = t(messageErrorKey(cause, 'loadFailed'))
  } finally {
    if (current === threadGeneration) loadingThread.value = false
  }
}

async function sendReply() {
  const userID = selectedUserID.value
  const body = currentDraft.value.trim()
  if (!userID || !body || sending.value || !contact.value) return
  const key = JSON.stringify([userID, body])
  if (retry?.key !== key) retry = { key, id: crypto.randomUUID() }
  sending.value = true
  threadError.value = ''
  try {
    await siteMessagesAPI.send({
      body,
      client_id: retry.id,
      ...(auth.isAdmin ? { recipient_ids: [userID] } : {}),
    })
    drafts[userID] = ''
    retry = null
    if (selectedUserID.value === userID) await loadThread(0, true)
    void loadConversations()
    void unread.refresh()
  } catch (cause) {
    if (selectedUserID.value === userID) threadError.value = t(messageErrorKey(cause, 'sendFailed'))
  } finally {
    sending.value = false
  }
}

function onNewMessage(count: number, userID?: number) {
  newMessageOpen.value = false
  app.showSuccess(t('messages.delivered', { count }))
  if (userID === selectedUserID.value) void loadThread(0, true)
  else if (userID) selectUser(userID)
  void loadConversations()
  void unread.refresh()
}

function refreshVisible() {
  if (document.visibilityState !== 'visible' || sending.value) return
  if (!loadingThread.value) void loadThread()
  if (!loadingConversations.value) void loadConversations()
  void unread.refresh()
}

watch(
  () => route.query.user,
  (value) => {
    if (!auth.isAdmin) {
      selectedUserID.value = auth.user?.id || 0
      return
    }
    const userID = Number(value)
    if (Number.isSafeInteger(userID) && userID > 0) selectedUserID.value = userID
  },
  { immediate: true },
)

watch(
  selectedUserID,
  () => {
    threadGeneration++
    messages.value = []
    contact.value = null
    peerReadThroughID.value = 0
    hasMoreMessages.value = false
    threadError.value = ''
    void loadThread(0, true)
  },
  { immediate: true },
)

watch(search, () => {
  listGeneration++
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void loadConversations(), 250)
})

onMounted(() => {
  void loadConversations()
  pollTimer = setInterval(refreshVisible, 15000)
  document.addEventListener('visibilitychange', refreshVisible)
})

onUnmounted(() => {
  listGeneration++
  threadGeneration++
  clearTimeout(searchTimer)
  clearInterval(pollTimer)
  document.removeEventListener('visibilitychange', refreshVisible)
})
</script>
