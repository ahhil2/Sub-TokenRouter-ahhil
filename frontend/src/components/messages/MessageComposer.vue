<template>
  <form class="space-y-3" data-testid="message-composer" @submit.prevent="submit">
    <label :for="inputID" class="sr-only">{{ t('messages.writeMessage') }}</label>
    <textarea
      :id="inputID"
      ref="textarea"
      :value="modelValue"
      :disabled="disabled || sending"
      :maxlength="8000"
      :placeholder="t('messages.placeholder')"
      :aria-describedby="`${inputID}-limit`"
      class="input min-h-28 resize-y"
      rows="3"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
      @keydown.ctrl.enter.prevent="submit"
      @keydown.meta.enter.prevent="submit"
    />
    <div
      v-if="emojiOpen"
      :id="`${inputID}-emoji`"
      class="grid grid-cols-8 gap-1 rounded-surface border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800"
      :aria-label="t('messages.emoji')"
    >
      <button
        v-for="emoji in emojis"
        :key="emoji"
        type="button"
        class="flex min-h-10 items-center justify-center rounded-control text-2xl hover:bg-gray-200 focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700"
        :aria-label="t('messages.insertEmoji', { emoji })"
        :disabled="disabled || sending"
        @mousedown.prevent
        @click="insertEmoji(emoji)"
      >
        {{ emoji }}
      </button>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="btn btn-secondary"
          :aria-expanded="emojiOpen"
          :aria-controls="`${inputID}-emoji`"
          :disabled="disabled || sending"
          @click="emojiOpen = !emojiOpen"
        >
          <span aria-hidden="true" class="mr-1 text-lg">😊</span>{{ t('messages.emoji') }}
        </button>
        <span
          :id="`${inputID}-limit`"
          class="text-xs"
          :class="length > 4000 ? 'text-red-500' : 'text-gray-500 dark:text-dark-400'"
          >{{ length }}/4000</span
        >
      </div>
      <button
        type="submit"
        class="btn btn-primary"
        :disabled="disabled || sending || !modelValue.trim() || length > 4000"
      >
        <Icon
          :name="sending ? 'loader' : 'arrowUp'"
          size="sm"
          :class="{ 'animate-spin': sending }"
        />
        {{ sending ? t('messages.sending') : t('messages.send') }}
      </button>
    </div>
    <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('messages.shortcut') }}</p>
  </form>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

/**
 * MessageComposer.vue edits plain message text and emits submission requests after
 * checking emptiness, Unicode character count, and pending state. Emoji insertion
 * replaces the selected range and restores the textarea cursor after its parent
 * updates the draft.
 */

const props = defineProps<{ modelValue: string; disabled?: boolean; sending?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; submit: [] }>()
const { t } = useI18n()
const inputID = `message-${useId()}`
const textarea = ref<HTMLTextAreaElement | null>(null)
const emojiOpen = ref(false)
const length = computed(() => Array.from(props.modelValue).length)
const emojis = [
  '😊',
  '😄',
  '😉',
  '🥰',
  '😎',
  '🤔',
  '😅',
  '🥳',
  '👍',
  '👏',
  '🙏',
  '🤝',
  '❤️',
  '💯',
  '🎉',
  '🎁',
  '🚀',
  '✨',
  '✅',
  '📢',
  '💬',
  '🛠️',
  '☕',
  '🌹',
]

function submit() {
  if (!props.disabled && !props.sending && props.modelValue.trim() && length.value <= 4000)
    emit('submit')
}

async function insertEmoji(emoji: string) {
  const start = textarea.value?.selectionStart ?? props.modelValue.length
  const end = textarea.value?.selectionEnd ?? start
  const value = props.modelValue.slice(0, start) + emoji + props.modelValue.slice(end)
  if (Array.from(value).length > 4000) return
  emit('update:modelValue', value)
  await nextTick()
  textarea.value?.focus()
  textarea.value?.setSelectionRange(start + emoji.length, start + emoji.length)
}
</script>
