import { describe, expect, it } from 'vitest'
import { channelLatency, channelPercent, channelStatus } from '../channelMonitor'

describe('channel monitoring metrics', () => {
  it('classifies availability without treating missing data as success', () => {
    expect(channelStatus(null)).toBe('unknown')
    expect(channelStatus(NaN)).toBe('unknown')
    expect(channelStatus(0)).toBe('failed')
    expect(channelStatus(94.9)).toBe('degraded')
    expect(channelStatus(95)).toBe('operational')
    expect(channelStatus(100)).toBe('operational')
  })
  it('formats nullable rates and latency', () => {
    expect(channelPercent(null)).toBe('—')
    expect(channelPercent(0)).toBe('0.0%')
    expect(channelPercent(85.24)).toBe('85.2%')
    expect(channelLatency(null)).toBe('—')
    expect(channelLatency(7660)).toBe('7.66s')
  })
})
