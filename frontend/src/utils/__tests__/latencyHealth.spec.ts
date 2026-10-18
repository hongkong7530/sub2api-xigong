import { describe, expect, it } from 'vitest'

import { durationSeverity, firstTokenSeverity, formatUsageOutputRate, isHighSpeedUsage, isGoldenSpeedUsage, usageOutputRate } from '../latencyHealth'

describe('latencyHealth', () => {
  it('classifies first-token latency at 10s/30s/60s boundaries', () => {
    expect(firstTokenSeverity(0)).toBe('good')
    expect(firstTokenSeverity(9_999)).toBe('good')
    expect(firstTokenSeverity(10_000)).toBe('warn')
    expect(firstTokenSeverity(29_999)).toBe('warn')
    expect(firstTokenSeverity(30_000)).toBe('slow')
    expect(firstTokenSeverity(59_999)).toBe('slow')
    expect(firstTokenSeverity(60_000)).toBe('critical')
  })

  it('classifies total duration at 1min/3min/5min boundaries', () => {
    expect(durationSeverity(0)).toBe('good')
    expect(durationSeverity(59_999)).toBe('good')
    expect(durationSeverity(60_000)).toBe('warn')
    expect(durationSeverity(179_999)).toBe('warn')
    expect(durationSeverity(180_000)).toBe('slow')
    expect(durationSeverity(299_999)).toBe('slow')
    expect(durationSeverity(300_000)).toBe('critical')
  })
})

describe('formatUsageOutputRate', () => {
  const row = {
    output_tokens: 1000,
    duration_ms: 20_000,
    first_token_ms: 10_000,
    output_duration_ms: 10_000,
    image_count: 0,
    image_output_tokens: 0,
    billing_mode: 'token',
  }

  it.each(['sync', 'stream', 'ws_v2', 'cyber', undefined] as const)(
    'uses observed formal stream window for %s requests', (request_type) => {
      expect(formatUsageOutputRate({ ...row, request_type })).toBe('100.0 tok/s')
    }
  )

  it('rounds to one decimal place', () => {
    expect(formatUsageOutputRate({ ...row, output_tokens: 101, output_duration_ms: 3000 })).toBe('33.7 tok/s')
  })

  it('uses a strict 200 tok/s threshold without rounding up', () => {
    expect(isHighSpeedUsage({ ...row, output_tokens: 2000 })).toBe(false)
    expect(isHighSpeedUsage({ ...row, output_tokens: 2001 })).toBe(true)
    expect(usageOutputRate({ ...row, first_token_ms: 0 })).toBe(100)
    expect(isGoldenSpeedUsage({ ...row, output_tokens: 2999 })).toBe(false)
    expect(isGoldenSpeedUsage({ ...row, output_tokens: 3000 })).toBe(true)
  })

  it.each([undefined, null, -1, NaN, Infinity, 0, 99])('rejects missing or invalid output window %s', output_duration_ms => {
    expect(formatUsageOutputRate({ ...row, output_duration_ms })).toBe('—')
  })

  it('accepts exactly 100ms generation', () => {
    expect(formatUsageOutputRate({ ...row, output_tokens: 20, output_duration_ms: 100 })).toBe('200.0 tok/s')
  })

  it('never reconstructs legacy timing from total duration and TTFT', () => {
    expect(formatUsageOutputRate({ ...row, output_duration_ms: null })).toBe('—')
    expect(formatUsageOutputRate({ ...row, first_token_ms: null })).toBe('100.0 tok/s')
  })

  it.each([0, -1, NaN, Infinity])('does not invent a rate for output %s', (output_tokens) => {
    expect(formatUsageOutputRate({ ...row, output_tokens })).toBe('—')
  })

  it.each([
    { image_count: 1 },
    { image_output_tokens: 100 },
    { billing_mode: 'image' },
    { request_type: 'live' as const },
    { request_type: 'unknown' as const },
  ])('omits non-text or unknown request rates: %j', (fields) => {
    expect(formatUsageOutputRate({ ...row, ...fields })).toBe('—')
  })
})
