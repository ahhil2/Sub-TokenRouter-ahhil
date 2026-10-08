<template>
  <div class="relative" ref="dropdownRef">
    <button
      @click="toggleDropdown"
      :disabled="switching"
      :class="triggerClass"
      :title="currentLocale?.name"
    >
      <!-- 顶栏状态变体用地球图标与相邻图标按钮对齐，也避开国旗 emoji 在 Windows 上无法渲染的问题。 -->
      <Icon v-if="variant === 'status'" name="globe" size="md" />
      <span v-else class="text-base leading-none">{{ currentLocale?.flag }}</span>
      <span v-if="variant !== 'status'" class="hidden sm:inline">{{ currentLocale?.code.toUpperCase() }}</span>
      <Icon
        v-if="variant !== 'status'"
        name="chevronDown"
        size="xs"
        class="text-gray-400 transition-transform duration-normal"
        :class="{ 'rotate-180': isOpen }"
        :animate-on-hover="false"
      />
    </button>

    <MotionTransition name="dropdown-fade">
      <div
        v-if="isOpen"
        class="dropdown right-0 z-50 mt-1 w-32 overflow-hidden py-0"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          :disabled="switching"
          @click="selectLocale(locale.code)"
          class="dropdown-item-sm"
          :class="{
            'bg-primary-50 text-primary-600 dark:bg-primary-500/8 dark:text-primary-500':
              locale.code === currentLocaleCode
          }"
        >
          <span class="text-base">{{ locale.flag }}</span>
          <span>{{ locale.name }}</span>
          <Icon
            v-if="locale.code === currentLocaleCode"
            name="check"
            size="sm"
            class="ml-auto text-primary-500"
            :animate-on-hover="false"
          />
        </button>
      </div>
    </MotionTransition>
  </div>
</template>

<script setup lang="ts">
import MotionTransition from '@/components/common/MotionTransition.vue'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const props = withDefaults(defineProps<{
  variant?: 'default' | 'status'
}>(), {
  variant: 'default'
})

const { locale } = useI18n()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const switching = ref(false)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))
const variant = computed(() => props.variant)
const triggerClass = computed(() => {
  if (variant.value === 'status') {
    return 'flex h-9 w-9 items-center justify-center rounded-control text-primary-900 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-100 dark:hover:bg-dark-700 dark:hover:text-white'
  }
  return 'flex items-center gap-1.5 rounded-control px-2 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-60 dark:text-gray-300 dark:hover:bg-dark-700'
})

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    isOpen.value = false
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    isOpen.value = false
  } finally {
    switching.value = false
  }
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
</style>
