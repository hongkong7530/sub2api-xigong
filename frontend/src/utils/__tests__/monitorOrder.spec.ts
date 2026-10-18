import { describe, expect, it } from 'vitest'
import { sortMonitors } from '../monitorOrder'

describe('monitor provider order', () => {
  it('uses provider/name/id and ignores changing telemetry without mutating input', () => {
    const input = [
      { id: 9, provider: 'openai', name: 'A', latency: 1 },
      { id: 3, provider: 'anthropic', name: 'B', latency: 999 },
      { id: 2, provider: 'openai', name: 'A', latency: null },
      { id: 7, provider: 'future-provider', name: 'Z', latency: 4 }
    ]
    expect(sortMonitors(input).map(x => x.id)).toEqual([3, 7, 2, 9])
    expect(input.map(x => x.id)).toEqual([9, 3, 2, 7])
    expect(sortMonitors([...input].reverse().map(x => ({ ...x, latency: 5 }))).map(x => x.id)).toEqual([3, 7, 2, 9])
  })
})
