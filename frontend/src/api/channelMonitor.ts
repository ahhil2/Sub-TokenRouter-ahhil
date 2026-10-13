import { apiClient } from './client'

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
  return (await apiClient.get<ChannelMonitorSnapshot>('/channel-monitors', { signal, timeout: 10000 })).data
}
