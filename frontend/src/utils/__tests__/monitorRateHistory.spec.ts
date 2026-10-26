import { describe, expect, it } from 'vitest'
import { buildMonitorRateHistory } from '../monitorRateHistory'
const at = (hours: number) => new Date(Date.UTC(2026, 9, 20, hours)).toISOString()
describe('monitor rate history', () => {
  it('renders a visible horizontal baseline for one initial record', () => {
    const c = buildMonitorRateHistory([{ rate_multiplier: 0.3, recorded_at: at(1), is_initial: true }], 0.3, Date.parse(at(4)))
    expect(c.path).toBe('M10 48 H630'); expect(c.entries[0]?.direction).toBe('initial'); expect(c.changes).toBe(0)
  })
  it('shows current baseline without inventing a historical time', () => {
    const c = buildMonitorRateHistory([], 0.25)
    expect(c.path).toBe('M10 48 H630'); expect(c.entries).toHaveLength(0)
  })
  it('uses recorded time for steps, with lower and higher prices and exact dates', () => {
    const c = buildMonitorRateHistory([{ rate_multiplier: 0.28, recorded_at: at(4) }, { rate_multiplier: 0.2, recorded_at: at(2) }, { rate_multiplier: 0.3, recorded_at: at(1), is_initial: true }], 0.28, Date.parse(at(5)))
    expect(c.entries.map(p => p.direction)).toEqual(['initial', 'decrease', 'increase'])
    expect(c.entries[1]?.recorded_at).toBe(at(2)); expect(c.entries[1]?.x).toBe(165)
    expect(c.path).toContain('H165 V'); expect(c.path).toContain('H475 V'); expect(c.changes).toBe(2)
  })
  it('does not call a truncated history window the original rate', () => {
    const c = buildMonitorRateHistory([{ rate_multiplier: 0.3, recorded_at: at(3) }], 0.3)
    expect(c.entries[0]?.direction).toBe('baseline')
  })
  it('preserves order for edits recorded at the same timestamp and ignores invalid data', () => {
    const c = buildMonitorRateHistory([{ rate_multiplier: 0.2, recorded_at: at(1) }, { rate_multiplier: 0.3, recorded_at: at(1), is_initial: true }, { rate_multiplier: NaN, recorded_at: at(2) }], 0.2)
    expect(c.entries.map(p => p.rate_multiplier)).toEqual([0.3, 0.2]); expect(c.changes).toBe(1)
  })
})
