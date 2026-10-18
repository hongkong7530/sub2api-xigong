import { BILLING_MODE_TOKEN, getDisplayBillingMode, isImageUsage } from './billingMode'
import { resolveUsageRequestType, type UsageRequestTypeLike } from './usageRequestType'

export interface UsageCacheRateRow extends UsageRequestTypeLike {
  input_tokens?: number | null
  cache_read_tokens?: number | null
  cache_creation_tokens?: number | null
  image_count?: number | null
  billing_mode?: string | null
}

function billingShape(row: UsageCacheRateRow) {
  return {
    image_count: row.image_count ?? 0,
    billing_mode: row.billing_mode,
  }
}

export function isStreamingUsage(row: UsageRequestTypeLike | null | undefined): boolean {
  if (!row) return false
  const requestType = resolveUsageRequestType(row)
  if (requestType === 'stream' || requestType === 'ws_v2' || requestType === 'live') {
    return true
  }
  return Boolean(row.stream)
}

export function isTokenUsageBilling(row: UsageCacheRateRow | null | undefined): boolean {
  if (!row || isImageUsage(billingShape(row))) return false
  const mode = getDisplayBillingMode(billingShape(row))
  return !mode || mode === BILLING_MODE_TOKEN
}

export function usageCacheRateDenominator(row: UsageCacheRateRow | null | undefined): number {
  if (!row) return 0
  return (row.input_tokens ?? 0) + (row.cache_read_tokens ?? 0) + (row.cache_creation_tokens ?? 0)
}

/** Cache hit percent, matching group-monitor: cache_read / (input + cache_read + cache_creation) * 100. */
export function usageCacheRatePercent(row: UsageCacheRateRow | null | undefined): number | null {
  if (!row || !isStreamingUsage(row) || !isTokenUsageBilling(row)) {
    return null
  }
  const denominator = usageCacheRateDenominator(row)
  if (denominator <= 0) return null
  return ((row.cache_read_tokens ?? 0) / denominator) * 100
}

export function formatUsageCacheRatePercent(row: UsageCacheRateRow | null | undefined): string | null {
  const percent = usageCacheRatePercent(row)
  if (percent == null) return null
  return percent.toFixed(1)
}
