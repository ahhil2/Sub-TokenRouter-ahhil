<template>
  <div class="mb-2 flex flex-wrap items-center gap-2">
    <button type="button" class="btn btn-secondary btn-sm font-bold" @mousedown.prevent @click="format('bold')">{{ t('admin.announcements.form.bold') }}</button>
    <label :for="`${inputId}-size`" class="text-xs text-gray-600 dark:text-dark-300">{{ t('admin.announcements.form.fontSize') }}</label>
    <input :id="`${inputId}-size`" v-model.number="fontSize" type="number" min="12" max="48" step="1" class="input w-20" />
    <button type="button" class="btn btn-secondary btn-sm" @mousedown.prevent @click="format('size')">{{ t('admin.announcements.form.applySize') }}</button>
    <p class="w-full text-xs text-gray-500 dark:text-dark-400">{{ t('admin.announcements.form.formatHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ inputId: string; modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const fontSize = ref(20)

async function format(kind: 'bold' | 'size') {
  const input = document.getElementById(props.inputId) as HTMLTextAreaElement | null
  if (!input || !props.modelValue) return
  const start = input.selectionStart === input.selectionEnd ? 0 : input.selectionStart
  const end = input.selectionStart === input.selectionEnd ? props.modelValue.length : input.selectionEnd
  const size = Math.min(48, Math.max(12, Math.round(Number(fontSize.value) || 20)))
  fontSize.value = size
  const opening = kind === 'bold' ? '<strong>' : `<span style="font-size: ${size}px">`
  const closing = kind === 'bold' ? '</strong>' : '</span>'
  const selectedText = props.modelValue.slice(start, end)
  const selection = kind === 'size'
    ? selectedText.replace(/<span\b[^>]*>/gi, tag => tag.replace(/font-size\s*:\s*[\d.]+px/gi, `font-size: ${size}px`))
    : selectedText
  emit('update:modelValue', props.modelValue.slice(0, start) + opening + selection + closing + props.modelValue.slice(end))
  await nextTick()
  input.focus()
  input.setSelectionRange(start + opening.length, start + opening.length + selection.length)
}
</script>
