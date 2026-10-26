import { describe, expect, it } from 'vitest'

import { durationSeverity, firstTokenSeverity, formatUsageOutputRate, formatUsageAverageThroughput, usageAverageThroughput, usageRateQuality, isHighSpeedUsage, isGoldenSpeedUsage, usageOutputRate } from '../latencyHealth'

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
    rate_observation: { version: 1, window_ms: 10000, output_events: 10, event_span_ms: 9000, output_bytes: 1000, max_burst_bytes: 100, quality: 'observed', request_duration_ms: 20000, timing_scope: 'app_request' as const },
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

  it('keeps short-window raw rates without treating them as proven high speed', () => {
    const short = { ...row, output_tokens: 203, output_duration_ms: 204, rate_observation: { ...row.rate_observation, window_ms: 204, quality: 'short_window' } }
    expect(formatUsageOutputRate(short)).toBe('995.1 tok/s')
    expect(usageRateQuality(short)).toBe('short_window')
    expect(isHighSpeedUsage(short)).toBe(false)
    expect(isGoldenSpeedUsage(short)).toBe(false)
  })

  it('never rewards legacy, concentrated or incomplete observations', () => {
    for (const quality of ['concentrated', 'single_batch', 'incomplete', 'unknown']) {
      const sample = { ...row, output_tokens: 10000, rate_observation: { ...row.rate_observation, quality } }
      expect(usageOutputRate(sample)).toBe(1000)
      expect(isGoldenSpeedUsage(sample)).toBe(false)
    }
    expect(isHighSpeedUsage({ ...row, output_tokens: 10000, rate_observation: null })).toBe(false)
    expect(usageRateQuality({ ...row, rate_observation: null })).toBe('unknown')
    expect(isHighSpeedUsage({ ...row, output_tokens: 10000, rate_observation: { ...row.rate_observation, output_events: 1 } })).toBe(false)
  })

  it('restores reception-rate effects without upgrading consumer-timed quality or using request-average speed', () => {
    const sample = { ...row, rate_observation: { ...row.rate_observation, quality: 'timing_uncertain' } }
    expect(usageRateQuality(sample)).toBe('timing_uncertain')
    expect(isHighSpeedUsage({ ...sample, output_tokens: 2000 })).toBe(false)
    expect(isHighSpeedUsage({ ...sample, output_tokens: 2001 })).toBe(true)
    expect(isGoldenSpeedUsage({ ...sample, output_tokens: 2999 })).toBe(false)
    expect(isGoldenSpeedUsage({ ...sample, output_tokens: 3000 })).toBe(true)
    expect(usageAverageThroughput({ ...sample, output_tokens: 3000 })).toBe(150)
    expect(sample.rate_observation.quality).toBe('timing_uncertain')
  })

  it.each([
    { window_ms: 106 },
    { output_events: 2 },
    { event_span_ms: 249 },
    { output_bytes: 0 },
    { max_burst_bytes: 800 },
    { max_burst_bytes: -1 },
    { max_burst_bytes: NaN },
    { output_events: Infinity },
    { event_span_ms: NaN },
    { output_bytes: Infinity },
    { version: 2 },
  ])('does not restore effects for inadequate consumer-timed observations: %j', fields => {
    const sample = { ...row, output_tokens: 10000, rate_observation: { ...row.rate_observation, quality: 'timing_uncertain', ...fields } }
    expect(isHighSpeedUsage(sample)).toBe(false)
    expect(isGoldenSpeedUsage(sample)).toBe(false)
  })

  it.each([999, 1000, 2130, 3000])('uses a one-second effect boundary for a %dms output window', window => {
    const sample = { ...row, output_tokens: Math.ceil(window * 0.31), output_duration_ms: window, rate_observation: { ...row.rate_observation, window_ms: window, event_span_ms: window - 100, quality: window < 1000 ? 'short_window' : 'timing_uncertain' } }
    expect(isHighSpeedUsage(sample)).toBe(window >= 1000)
    expect(isGoldenSpeedUsage(sample)).toBe(window >= 1000)
    expect(usageRateQuality(sample)).toBe(window < 1000 ? 'short_window' : 'timing_uncertain')
  })

  it('keeps the reported 143-token short-window spike unhighlighted', () => {
    const sample = { ...row, output_tokens: 143, output_duration_ms: 106, rate_observation: { ...row.rate_observation, window_ms: 106, quality: 'timing_uncertain' } }
    expect(formatUsageOutputRate(sample)).toBe('1349.1 tok/s')
    expect(isHighSpeedUsage(sample)).toBe(false)
    expect(isGoldenSpeedUsage(sample)).toBe(false)
  })

  it('uses only ingress/turn timing for request-average throughput', () => {
    expect(usageAverageThroughput(row)).toBe(50)
    expect(formatUsageAverageThroughput(row)).toBe('50.0 tok/s')
    expect(formatUsageAverageThroughput({ ...row, rate_observation: null })).toBe('—')
    expect(formatUsageAverageThroughput({ ...row, rate_observation: { ...row.rate_observation, request_duration_ms: 0 } })).toBe('—')
    expect(usageAverageThroughput({ ...row, duration_ms: 1, first_token_ms: 0 })).toBe(50)
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
