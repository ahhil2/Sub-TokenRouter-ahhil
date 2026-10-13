export type ChannelStatus = 'operational' | 'degraded' | 'failed' | 'unknown'

export function channelStatus(availability: number | null): ChannelStatus {
  if (availability === null || !Number.isFinite(availability)) return 'unknown'
  if (availability >= 95) return 'operational'
  return availability > 0 ? 'degraded' : 'failed'
}

export function channelPercent(value: number | null): string {
  return value === null || !Number.isFinite(value) ? '—' : `${value.toFixed(1)}%`
}

export function channelLatency(value: number | null): string {
  return value === null || !Number.isFinite(value) ? '—' : `${(value / 1000).toFixed(2)}s`
}
