import { apiClient } from './client'

/**
 * channelMonitor.ts defines public channel snapshots and administrator visibility
 * settings. Metrics may be absent when a reporting window has no usable samples;
 * callers retain these null values so empty history is distinguishable from zero.
 */

export interface ChannelMetrics {
  requests: number
  successes: number
  availability: number | null
  cache_rate: number | null
  ttft_p50_ms: number | null
}

export interface ChannelSlot extends ChannelMetrics {
  start: string
  end: string
}

export interface MonitoredChannel extends ChannelMetrics {
  id: number
  name: string
  brand: string
  rate_multiplier: number
  slots: ChannelSlot[]
  status_override?: ChannelMonitorStatus
}

export type ChannelMonitorStatus = 'auto' | 'operational' | 'degraded' | 'failed' | 'maintenance'

export interface ChannelMonitorGroupConfig {
  group_id: number
  enabled: boolean
  status: ChannelMonitorStatus
}

export interface ChannelMonitorGroupOption extends ChannelMonitorGroupConfig {
  name: string
  brand: string
  eligible: boolean
}

export async function getChannelMonitorConfig(
  signal?: AbortSignal,
): Promise<ChannelMonitorGroupOption[]> {
  return (
    await apiClient.get<{ groups: ChannelMonitorGroupOption[] }>('/admin/channel-monitors/config', {
      signal,
    })
  ).data.groups
}

export async function saveChannelMonitorConfig(groups: ChannelMonitorGroupConfig[]): Promise<void> {
  await apiClient.put('/admin/channel-monitors/config', { groups })
}

export interface ChannelMonitorSnapshot {
  generated_at: string
  window_minutes: number
  slot_seconds: number
  refresh_seconds: number
  monitoring_enabled: boolean
  channels: MonitoredChannel[]
}

export async function getChannelMonitor(signal?: AbortSignal): Promise<ChannelMonitorSnapshot> {
  return (
    await apiClient.get<ChannelMonitorSnapshot>('/channel-monitors', { signal, timeout: 10000 })
  ).data
}
