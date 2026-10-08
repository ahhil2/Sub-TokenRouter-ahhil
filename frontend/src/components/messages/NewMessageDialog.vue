<template>
  <BaseDialog :show="true" :title="t('messages.newMessage')" :close-on-escape="!sending" @close="!sending && emit('close')">
    <div class="space-y-4">
      <template v-if="!confirmingAll">
        <label class="flex items-center gap-3 text-sm">
          <input v-model="allUsers" type="checkbox" class="checkbox" :disabled="sending" />
          {{ t('messages.allUsers') }}
        </label>
        <template v-if="!allUsers">
          <label for="message-recipient-search" class="sr-only">{{ t('messages.searchRecipients') }}</label>
          <input id="message-recipient-search" v-model="search" class="input" :placeholder="t('messages.searchRecipients')" maxlength="128" :disabled="sending" />
          <div v-if="selected.length" class="flex flex-wrap gap-2">
            <button v-for="contact in selected" :key="contact.id" type="button" class="badge badge-primary max-w-full gap-2" :disabled="sending" :aria-label="t('messages.removeRecipient', { name: contact.username || contact.email })" @click="toggleContact(contact)">
              <span class="truncate">{{ contact.username || contact.email }}</span><Icon name="x" size="xs" />
            </button>
          </div>
          <div class="max-h-menu space-y-1 overflow-y-auto rounded-control border border-gray-200 p-2 dark:border-dark-600" :aria-label="t('messages.recipients')">
            <p v-if="loading" class="p-3 text-sm text-gray-500">{{ t('common.loading') }}</p>
            <p v-else-if="!contacts.length" class="p-3 text-sm text-gray-500">{{ t('messages.noRecipients') }}</p>
            <label v-for="contact in contacts" :key="contact.id" class="flex cursor-pointer items-center gap-3 rounded-control p-2 hover:bg-gray-50 dark:hover:bg-dark-800">
              <input type="checkbox" class="checkbox" :checked="selected.some(item => item.id === contact.id)" :disabled="sending || (selected.length >= 100 && !selected.some(item => item.id === contact.id))" @change="toggleContact(contact)" />
              <span class="min-w-0 text-sm"><span class="block truncate">{{ contact.username || contact.email }}</span><span v-if="contact.username" class="block truncate text-xs text-gray-500 dark:text-dark-400">{{ contact.email }}</span></span>
            </label>
            <button v-if="hasMore" type="button" class="btn btn-secondary w-full" :disabled="loading || sending" @click="loadContacts(false)">{{ t('messages.loadMore') }}</button>
          </div>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('messages.recipientCount', { count: selected.length }) }}</p>
        </template>
        <p v-else class="rounded-control bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-300">{{ t('messages.broadcastHint') }}</p>
        <MessageComposer v-model="body" :disabled="!allUsers && !selected.length" :sending="sending" @submit="allUsers ? confirmingAll = true : send()" />
      </template>
      <template v-else>
        <p class="text-sm font-medium text-amber-700 dark:text-amber-300">{{ t('messages.confirmBroadcast') }}</p>
        <p class="max-h-menu overflow-y-auto whitespace-pre-wrap break-words rounded-surface bg-gray-50 p-4 text-sm dark:bg-dark-800">{{ body }}</p>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="sending" @click="confirmingAll = false">{{ t('common.back') }}</button>
          <button type="button" class="btn btn-primary" :disabled="sending" @click="send">{{ sending ? t('messages.sending') : t('messages.confirmSendAll') }}</button>
        </div>
      </template>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { siteMessagesAPI, type MessageContact } from '@/api/siteMessages'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import MessageComposer from './MessageComposer.vue'
import { messageErrorKey } from './messageErrors'

const emit = defineEmits<{ close: []; sent: [count: number, userID?: number] }>()
const { t } = useI18n()
const search = ref('')
const contacts = ref<MessageContact[]>([])
const selected = ref<MessageContact[]>([])
const allUsers = ref(false)
const confirmingAll = ref(false)
const body = ref('')
const loading = ref(false)
const sending = ref(false)
const error = ref('')
const hasMore = ref(false)
let page = 0
let generation = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined
let retry: { payload: string; id: string } | null = null

function toggleContact(contact: MessageContact) {
  const exists = selected.value.some(item => item.id === contact.id)
  if (exists) selected.value = selected.value.filter(item => item.id !== contact.id)
  else if (selected.value.length < 100) selected.value.push(contact)
}

async function loadContacts(reset = true) {
  const current = ++generation
  const nextPage = reset ? 1 : page + 1
  loading.value = true
  try {
    const result = await siteMessagesAPI.contacts(search.value, nextPage)
    if (current !== generation) return
    contacts.value = reset ? result.items : [...contacts.value, ...result.items]
    hasMore.value = result.has_more
    page = nextPage
  } catch (cause) {
    if (current === generation) error.value = t(messageErrorKey(cause, 'loadFailed'))
  } finally {
    if (current === generation) loading.value = false
  }
}

async function send() {
  if (sending.value || !body.value.trim() || (!allUsers.value && !selected.value.length)) return
  const recipientIDs = allUsers.value ? [] : selected.value.map(item => item.id).sort((first, second) => first - second)
  const payload = { body: body.value.trim(), recipient_ids: recipientIDs, all_users: allUsers.value }
  const key = JSON.stringify(payload)
  if (retry?.payload !== key) retry = { payload: key, id: crypto.randomUUID() }
  sending.value = true
  error.value = ''
  try {
    const result = await siteMessagesAPI.send({ ...payload, client_id: retry.id })
    emit('sent', result.recipient_count, recipientIDs.length === 1 ? recipientIDs[0] : undefined)
  } catch (cause) {
    error.value = t(messageErrorKey(cause, 'sendFailed'))
  } finally {
    sending.value = false
  }
}

watch(search, () => {
  generation++
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void loadContacts(), 250)
})
onMounted(() => void loadContacts())
onUnmounted(() => { generation++; clearTimeout(searchTimer) })
</script>
