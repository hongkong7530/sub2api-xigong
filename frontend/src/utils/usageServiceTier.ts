export function normalizeUsageServiceTier(serviceTier?: string | null): string | null {
  const value = serviceTier?.trim().toLowerCase()
  if (!value) return null
  if (value === 'fast') return 'priority'
  if (value === 'default' || value === 'standard') return 'standard'
  if (value === 'priority' || value === 'flex' || value === 'ultrafast') return value
  return value
}

interface TierBillingUsage {
  service_tier?: string | null
  model: string
  upstream_model?: string | null
  billing_model?: string | null
  input_tokens?: number | null
  output_tokens?: number | null
  cache_read_tokens?: number | null
  cache_creation_tokens?: number | null
  input_cost?: number | null
  output_cost?: number | null
  cache_read_cost?: number | null
  cache_creation_cost?: number | null
  long_context_billing_applied?: boolean
}

// A tier name alone does not prove a price multiplier or upstream acknowledgement.
// Only label six-times billing when every nonempty token bucket matches the
// supported model's official Ultrafast rate; custom prices remain unlabelled.
export function verifiedUltrafastBillingMultiplier(row: TierBillingUsage): number | null {
  if (normalizeUsageServiceTier(row.service_tier) !== 'ultrafast') return null
  const model = (row.billing_model || row.upstream_model || row.model).toLowerCase().replace(/^openai\//, '')
  const rates = model === 'gpt-6.1-sol' ? [12e-6, 60e-6, 0.6e-6, 15e-6]
    : model === 'gpt-6-astra' ? [60e-6, 300e-6, 6e-6, 75e-6] : null
  if (!rates) return null
  const counts = [row.input_tokens, row.output_tokens, row.cache_read_tokens, row.cache_creation_tokens]
  const costs = [row.input_cost, row.output_cost, row.cache_read_cost, row.cache_creation_cost]
  let priced = false
  for (let i = 0; i < counts.length; i++) {
    const count = counts[i] ?? 0
    const cost = costs[i] ?? 0
    if (!Number.isFinite(count) || !Number.isFinite(cost) || count < 0 || cost < 0) return null
    const longMultiplier = row.long_context_billing_applied ? (i === 1 ? 1.5 : 2) : 1
    if (Math.abs(cost - count * rates[i]! * longMultiplier) > 1e-8) return null
    if (count > 0) priced = true
  }
  return priced ? 6 : null
}

export function formatUsageServiceTier(serviceTier?: string | null): string {
  const normalized = normalizeUsageServiceTier(serviceTier)
  if (!normalized) return 'standard'
  return normalized
}

export function getUsageServiceTierLabel(
  serviceTier: string | null | undefined,
  translate: (key: string) => string,
): string {
  const tier = formatUsageServiceTier(serviceTier)
  if (tier === 'priority') return translate('usage.serviceTierPriority')
  if (tier === 'ultrafast') return translate('usage.serviceTierUltrafast')
  if (tier === 'flex') return translate('usage.serviceTierFlex')
  if (tier === 'standard') return translate('usage.serviceTierStandard')
  return tier
}
