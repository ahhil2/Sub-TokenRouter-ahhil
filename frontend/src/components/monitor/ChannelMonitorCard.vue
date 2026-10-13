<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import type { MonitoredChannel } from '@/api/channelMonitor'
import { channelLatency, channelPercent, channelStatus } from '@/utils/channelMonitor'

const props = defineProps<{ channel: MonitoredChannel; countdown: number; windowMinutes: number; slotSeconds: number }>()
const { t, locale } = useI18n()
const activeSlot = ref<number | null>(null)
const active = computed(() => activeSlot.value === null ? null : props.channel.slots[activeSlot.value])
const tooltipID = computed(() => `channel-${props.channel.id}-slot-tooltip`)
const status = computed(() => channelStatus(props.channel.availability))
const tooltipPosition = computed(() => `${((activeSlot.value ?? 0) + 0.5) / props.channel.slots.length * 100}%`)
const timeFormatter = computed(() => new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }))

function time(value: string) {
  return timeFormatter.value.format(new Date(value))
}

function slotLabel(index: number) {
  const slot = props.channel.slots[index]
  return `${time(slot.start)} - ${time(slot.end)}, ${t('channelMonitor.availability')} ${channelPercent(slot.availability)}, ${t('channelMonitor.cacheRate')} ${channelPercent(slot.cache_rate)}, ${t('channelMonitor.requests', { count: slot.requests })}`
}
</script>

<template>
  <article class="monitor-card" :aria-labelledby="`channel-${channel.id}-name`">
    <div class="monitor-card-heading">
      <span class="monitor-brand-icon"><ProviderIcon :brand="channel.brand" size="30px" color="currentColor" /></span>
      <div class="monitor-card-identity">
        <h3 :id="`channel-${channel.id}-name`">{{ channel.name }}</h3>
        <div class="monitor-card-meta">
          <span class="monitor-brand-tag">{{ channel.brand }}</span>
          <span>{{ t('channelMonitor.realRequests') }}</span>
          <span class="monitor-multiplier" :title="t('channelMonitor.multiplierHint')">{{ t('channelMonitor.multiplier') }} <b>{{ channel.rate_multiplier }}×</b></span>
        </div>
      </div>
      <span class="monitor-status" :data-status="status">{{ t(`channelMonitor.status.${status}`) }}</span>
    </div>

    <dl class="monitor-metrics">
      <div :title="t('channelMonitor.cacheDefinition')"><dt>{{ t('channelMonitor.cacheRate') }}</dt><dd>{{ channelPercent(channel.cache_rate) }}</dd></div>
      <div :title="t('channelMonitor.availabilityDefinition')"><dt>{{ t('channelMonitor.availability') }}</dt><dd class="monitor-availability">{{ channelPercent(channel.availability) }}</dd></div>
      <div :title="t('channelMonitor.ttftDefinition')"><dt>{{ t('channelMonitor.firstToken') }} <span>· P50</span></dt><dd>{{ channelLatency(channel.ttft_p50_ms) }}</dd></div>
    </dl>

    <div class="monitor-history-heading">
      <span>{{ t('channelMonitor.history', { count: channel.slots.length, minutes: slotSeconds / 60 }) }}</span>
      <span>{{ t('channelMonitor.refreshIn', { seconds: countdown }) }}</span>
    </div>
    <div class="monitor-history" @mouseleave="activeSlot = null" @keydown.esc="activeSlot = null">
      <div class="monitor-blocks" :aria-label="t('channelMonitor.historyLabel')">
        <button
          v-for="(slot, index) in channel.slots"
          :key="index"
          type="button"
          class="monitor-block"
          :data-status="channelStatus(slot.availability)"
          :aria-label="slotLabel(index)"
          :aria-describedby="activeSlot === index ? tooltipID : undefined"
          @mouseenter="activeSlot = index"
          @focus="activeSlot = index"
          @blur="activeSlot = null"
          @click="activeSlot = index"
        />
      </div>
      <div v-if="active" :id="tooltipID" role="tooltip" class="monitor-tooltip" :style="{ '--slot-position': tooltipPosition }">
        <p class="monitor-tooltip-time">{{ time(active.start) }} - {{ time(active.end) }}</p>
        <dl>
          <div><dt>{{ t('channelMonitor.availability') }}</dt><dd>{{ channelPercent(active.availability) }}</dd></div>
          <div><dt>{{ t('channelMonitor.cacheRate') }}</dt><dd>{{ channelPercent(active.cache_rate) }}</dd></div>
          <div><dt>{{ t('channelMonitor.firstToken') }} · P50</dt><dd>{{ channelLatency(active.ttft_p50_ms) }}</dd></div>
        </dl>
        <p>{{ active.requests ? t('channelMonitor.sampleCount', { successes: active.successes, requests: active.requests }) : t('channelMonitor.noRequests') }}</p>
      </div>
    </div>
    <div class="monitor-history-axis"><span>PAST · {{ windowMinutes }}m</span><span>NOW</span></div>
  </article>
</template>

<style scoped>
.monitor-card { padding: 30px; border-radius: var(--radius-monitor-36); background: var(--monitor-card); box-shadow: 0 16px 44px rgb(30 50 60 / 4%); min-width: 0; }
.monitor-card-heading { display: flex; align-items: flex-start; gap: 15px; }
.monitor-brand-icon { display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 54px; height: 54px; border-radius: var(--radius-monitor-19); background: var(--monitor-mint); color: var(--monitor-icon); }
.monitor-card-identity { flex: 1; min-width: 0; }
h3 { margin: 0 0 9px; font-size: 23px; line-height: 1.3; font-weight: 750; letter-spacing: -0.6px; overflow-wrap: anywhere; }
.monitor-card-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 7px 12px; color: var(--monitor-muted); font-size: 12px; }
.monitor-brand-tag { background: var(--monitor-mint); color: var(--monitor-accent-text); padding: 1px 8px; border-radius: var(--radius-monitor-8); font-size: 14px; }
.monitor-multiplier { color: var(--monitor-accent-text); white-space: nowrap; }
.monitor-multiplier b { font-family: var(--font-mono, monospace); font-weight: 500; margin-left: 4px; }
.monitor-status { flex-shrink: 0; padding: 5px 12px; border-radius: 9999px; font-size: 13px; line-height: 1.5; font-weight: 650; background: var(--monitor-mint); color: var(--monitor-accent-text); }
.monitor-status[data-status='unknown'] { background: var(--monitor-surface); color: var(--monitor-muted); }
.monitor-status[data-status='degraded'] { background: var(--monitor-warning-bg); color: var(--monitor-warning-text); }
.monitor-status[data-status='failed'] { background: var(--monitor-danger-bg); color: var(--monitor-danger-text); }
.monitor-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-top: 30px; }
.monitor-metrics > div { background: var(--monitor-surface); border: 1px solid var(--monitor-border); border-radius: var(--radius-monitor-23); padding: 18px 15px; }
.monitor-metrics dt { font-size: 13px; color: var(--monitor-muted); font-weight: 650; white-space: nowrap; }
.monitor-metrics dt span { font-size: 12px; font-weight: 400; }
.monitor-metrics dd { font-family: var(--font-mono, monospace); font-weight: 650; font-size: 20px; margin-top: 10px; }
.monitor-availability { color: var(--monitor-accent-text); }
.monitor-history-heading { display: flex; justify-content: space-between; gap: 8px; flex-wrap: wrap; margin: 22px 0 11px; color: var(--monitor-muted); font-size: 13px; font-weight: 600; }
.monitor-history { position: relative; }
.monitor-blocks { display: grid; grid-template-columns: repeat(20, minmax(0, 1fr)); gap: 4px; }
.monitor-block { height: 30px; border-radius: var(--radius-monitor-6); background: #41A988; transition: opacity 150ms, transform 150ms; }
.monitor-block:hover, .monitor-block:focus-visible { transform: translateY(-3px); outline: 2px solid var(--monitor-accent-text); outline-offset: 2px; }
.monitor-block[data-status='unknown'] { background: var(--monitor-empty); }
.monitor-block[data-status='degraded'] { background: var(--monitor-warning); }
.monitor-block[data-status='failed'] { background: var(--monitor-danger); }
.monitor-history-axis { display: flex; justify-content: space-between; margin-top: 7px; color: var(--monitor-muted); font-size: 12px; letter-spacing: 0.3px; }
.monitor-tooltip { position: absolute; bottom: calc(100% + 12px); left: clamp(0px, calc(var(--slot-position) - 110px), calc(100% - 220px)); width: 220px; max-width: 100%; padding: 14px 16px; border: 1px solid var(--monitor-border); border-radius: var(--radius-monitor-16); background: var(--monitor-card); box-shadow: 0 8px 32px rgb(20 40 50 / 16%); z-index: var(--z-chart-tooltip); font-size: 12px; pointer-events: none; }
.monitor-tooltip-time { font-weight: 650; margin-bottom: 10px; font-variant-numeric: tabular-nums; }
.monitor-tooltip dl > div { display: flex; justify-content: space-between; gap: 8px; margin-top: 6px; }
.monitor-tooltip dd { font-family: var(--font-mono, monospace); font-weight: 650; }
.monitor-tooltip > p:last-child { color: var(--monitor-muted); margin-top: 10px; }
@media (max-width: 639px) {
  .monitor-card { padding: 22px 18px; border-radius: var(--radius-monitor-28); }
  .monitor-card-heading { gap: 10px; }
  .monitor-brand-icon { width: 42px; height: 42px; border-radius: var(--radius-monitor-15); }
  h3 { font-size: 19px; }
  .monitor-status { padding: 4px 8px; font-size: 12px; }
  .monitor-metrics { gap: 8px; margin-top: 24px; }
  .monitor-metrics > div { padding: 14px 10px; border-radius: var(--radius-monitor-18); }
  .monitor-metrics dt { font-size: 12px; }
  .monitor-metrics dd { font-size: 18px; }
  .monitor-blocks { gap: 3px; }
  .monitor-block { border-radius: var(--radius-monitor-4); }
}
@media (prefers-reduced-motion: reduce) { .monitor-block { transition: none; } }
</style>
