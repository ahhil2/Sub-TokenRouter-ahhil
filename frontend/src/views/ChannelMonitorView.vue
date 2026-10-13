<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppHeader from '@/components/layout/AppHeader.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import ChannelMonitorCard from '@/components/monitor/ChannelMonitorCard.vue'
import { getChannelMonitor, type ChannelMonitorSnapshot, type MonitoredChannel } from '@/api/channelMonitor'

const { t, locale } = useI18n()
const snapshot = ref<ChannelMonitorSnapshot | null>(null)
const loading = ref(false)
const error = ref(false)
const countdown = ref(30)
let controller: AbortController | undefined
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
const groups = computed(() => {
  const brands = new Map<string, MonitoredChannel[]>()
  for (const channel of snapshot.value?.channels ?? []) {
    if (!brands.has(channel.brand)) brands.set(channel.brand, [])
    brands.get(channel.brand)!.push(channel)
  }
  return [...brands].map(([brand, channels]) => ({ brand, channels }))
})
const updatedAt = computed(() => snapshot.value ? new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(snapshot.value.generated_at)) : '')

async function refresh() {
  if (loading.value || disposed) return
  controller = new AbortController()
  loading.value = true
  try {
    const result = await getChannelMonitor(controller.signal)
    if (disposed) return
    snapshot.value = result
    error.value = false
    countdown.value = Math.max(5, result.refresh_seconds)
  } catch {
    if (!disposed) { error.value = true; countdown.value = 30 }
  } finally {
    if (!disposed) loading.value = false
  }
}

function resume() {
  if (!document.hidden) void refresh()
}

onMounted(() => {
  void refresh()
  timer = setInterval(() => {
    if (document.hidden || loading.value) return
    countdown.value = Math.max(0, countdown.value - 1)
    if (countdown.value === 0) void refresh()
  }, 1000)
  document.addEventListener('visibilitychange', resume)
})
onBeforeUnmount(() => {
  disposed = true
  controller?.abort()
  clearInterval(timer)
  document.removeEventListener('visibilitychange', resume)
})
</script>

<template>
  <div class="channel-monitor-page">
    <AppHeader public-page />
    <main class="monitor-main">
      <div class="monitor-page-heading">
        <div><h1>{{ t('channelMonitor.title') }}</h1><p>{{ t('channelMonitor.subtitle') }}</p></div>
        <div class="monitor-refresh"><span v-if="snapshot">{{ t('channelMonitor.updatedAt', { time: updatedAt }) }}</span><button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="refresh">{{ t(loading ? 'channelMonitor.refreshing' : 'channelMonitor.refresh') }}</button></div>
      </div>
      <div v-if="error" role="alert" class="monitor-notice">{{ t(snapshot ? 'channelMonitor.stale' : 'channelMonitor.loadFailed') }}</div>
      <div v-if="loading && !snapshot" role="status" class="monitor-loading">{{ t('channelMonitor.loading') }}</div>
      <div v-else-if="snapshot && !snapshot.monitoring_enabled" role="status" class="monitor-loading">{{ t('channelMonitor.disabled') }}</div>
      <div v-else-if="snapshot && !groups.length" class="monitor-loading">{{ t('channelMonitor.empty') }}</div>
      <section v-for="group in groups" :key="group.brand" class="monitor-brand-group" :aria-label="group.brand">
        <h2><span class="monitor-group-icon"><ProviderIcon :brand="group.brand" size="25px" color="currentColor" /></span>{{ group.brand }}<span class="monitor-group-count">{{ group.channels.length }}</span></h2>
        <div class="monitor-grid">
          <ChannelMonitorCard v-for="channel in group.channels" :key="channel.id" :channel="channel" :countdown="countdown" :window-minutes="snapshot!.window_minutes" :slot-seconds="snapshot!.slot_seconds" />
        </div>
      </section>
      <footer v-if="snapshot?.monitoring_enabled" class="monitor-footer">
        <div class="monitor-legend"><span><i class="legend-operational" />{{ t('channelMonitor.status.operational') }}</span><span><i class="legend-degraded" />{{ t('channelMonitor.status.degraded') }}</span><span><i class="legend-failed" />{{ t('channelMonitor.status.failed') }}</span><span><i class="legend-unknown" />{{ t('channelMonitor.status.unknown') }}</span></div>
        <p>{{ t('channelMonitor.availabilityDefinition') }}</p><p>{{ t('channelMonitor.cacheDefinition') }}</p><p>{{ t('channelMonitor.ttftDefinition') }}</p>
      </footer>
    </main>
  </div>
</template>

<style scoped>
.channel-monitor-page { --radius-monitor-36: 36px; --radius-monitor-28: 28px; --radius-monitor-23: 23px; --radius-monitor-19: 19px; --radius-monitor-18: 18px; --radius-monitor-16: 16px; --radius-monitor-15: 15px; --radius-monitor-12: 12px; --radius-monitor-8: 8px; --radius-monitor-6: 6px; --radius-monitor-4: 4px; --radius-monitor-3: 3px; --monitor-card: #fff; --monitor-surface: #f7fafc; --monitor-border: #e4edf3; --monitor-muted: #7d8fa8; --monitor-mint: #dff8ef; --monitor-icon: #385c54; --monitor-accent-text: #008566; --monitor-empty: #dde5eb; --monitor-warning: #efb84a; --monitor-danger: #ef6580; --monitor-warning-bg: #fff5dd; --monitor-warning-text: #946500; --monitor-danger-bg: #ffedf1; --monitor-danger-text: #bd3153; min-height: 100vh; background: #f5f8f9; color: #293546; }
.dark .channel-monitor-page { --monitor-card: #19242d; --monitor-surface: #202e38; --monitor-border: #30414d; --monitor-muted: #9bafc2; --monitor-mint: #203e35; --monitor-icon: #b2d9ca; --monitor-accent-text: #75cfb0; --monitor-empty: #3c4b55; --monitor-warning-bg: #463a20; --monitor-warning-text: #f2cf7d; --monitor-danger-bg: #462632; --monitor-danger-text: #f5a3b5; background: #121c23; color: #e4edf4; }
.monitor-main { max-width: 1740px; margin: 0 auto; padding: calc(var(--header-h) + 30px) 30px 30px; }
.monitor-page-heading { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 20px; margin-bottom: 32px; }
h1 { font-size: 28px; font-weight: 750; letter-spacing: -0.5px; }
.monitor-page-heading p { color: var(--monitor-muted); font-size: 14px; margin-top: 7px; }
.monitor-refresh { display: flex; align-items: center; gap: 14px; font-size: 12px; color: var(--monitor-muted); }
.monitor-brand-group { margin-bottom: 35px; }
h2 { display: flex; align-items: center; gap: 14px; font-size: 24px; font-weight: 700; margin-bottom: 24px; }
.monitor-group-icon { display: flex; align-items: center; justify-content: center; width: 42px; height: 42px; border-radius: 9999px; background: var(--monitor-mint); color: var(--monitor-icon); }
.monitor-group-count { font-size: 16px; font-weight: 400; padding: 1px 10px; border-radius: var(--radius-monitor-12); background: var(--monitor-card); color: var(--monitor-muted); }
.monitor-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 28px; }
.monitor-loading { padding: 70px 24px; text-align: center; color: var(--monitor-muted); background: var(--monitor-card); border-radius: var(--radius-monitor-28); }
.monitor-notice { background: var(--monitor-warning-bg); color: var(--monitor-warning-text); padding: 14px 20px; border-radius: var(--radius-monitor-16); margin-bottom: 24px; }
.monitor-footer { margin-top: 36px; color: var(--monitor-muted); font-size: 12px; line-height: 1.9; }
.monitor-legend { display: flex; flex-wrap: wrap; gap: 20px; margin-bottom: 12px; }
.monitor-legend span { display: inline-flex; align-items: center; gap: 6px; }
.monitor-legend i { width: 10px; height: 10px; border-radius: var(--radius-monitor-3); }
.legend-operational { background: #41A988; }.legend-degraded { background: var(--monitor-warning); }.legend-failed { background: var(--monitor-danger); }.legend-unknown { background: var(--monitor-empty); }
@media (max-width: 1279px) { .monitor-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; } }
@media (max-width: 767px) { .monitor-main { padding: calc(var(--header-h) + 24px) 16px 24px; }.monitor-grid { grid-template-columns: minmax(0, 1fr); }.monitor-page-heading { margin-bottom: 28px; }h1 { font-size: 24px; }h2 { font-size: 22px; } }
</style>
