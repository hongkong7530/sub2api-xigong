import { describe, expect, it } from 'vitest'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN, BILLING_MODE_VIDEO } from '../billingMode'
import { formatUsageCacheRatePercent, isStreamingUsage, usageCacheRatePercent } from '../usageCacheRate'

describe('usageCacheRate', () => {
  it('uses the three mutually exclusive input buckets', () => {
    const row = { request_type: 'stream', billing_mode: BILLING_MODE_TOKEN, input_tokens: 70, cache_read_tokens: 20, cache_creation_tokens: 10 }
    expect(usageCacheRatePercent(row)).toBe(20)
    expect(formatUsageCacheRatePercent(row)).toBe('20.0')
    expect(formatUsageCacheRatePercent({ ...row, cache_read_tokens: 0 })).toBe('0.0')
  })
  it.each(['stream', 'ws_v2', 'live'])('supports streaming request type %s', request_type => {
    expect(isStreamingUsage({ request_type })).toBe(true)
    expect(usageCacheRatePercent({ request_type, input_tokens: 10, cache_read_tokens: 90 })).toBe(90)
  })
  it('supports legacy stream flags and streamed cyber requests', () => {
    expect(isStreamingUsage({ stream: true })).toBe(true)
    expect(isStreamingUsage({ request_type: 'cyber', stream: true })).toBe(true)
    expect(isStreamingUsage({ request_type: 'cyber', stream: false })).toBe(false)
  })
  it.each([BILLING_MODE_IMAGE, BILLING_MODE_VIDEO, BILLING_MODE_PER_REQUEST])('does not invent token cache rate for %s billing', billing_mode => {
    expect(usageCacheRatePercent({ request_type: 'stream', billing_mode, input_tokens: 10, cache_read_tokens: 90 })).toBeNull()
  })
  it('keeps unknown cache rate distinct from zero', () => {
    expect(usageCacheRatePercent(undefined)).toBeNull()
    expect(usageCacheRatePercent({ request_type: 'sync', input_tokens: 10 })).toBeNull()
    expect(usageCacheRatePercent({ request_type: 'stream', input_tokens: 0, cache_read_tokens: 0 })).toBeNull()
    expect(usageCacheRatePercent({ request_type: 'stream', image_count: 2, input_tokens: 10 })).toBeNull()
    expect(usageCacheRatePercent({ request_type: 'stream', input_tokens: 10, cache_read_tokens: 0 })).toBe(0)
  })
})
