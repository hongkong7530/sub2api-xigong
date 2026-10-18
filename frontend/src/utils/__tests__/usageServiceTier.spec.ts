import { describe, expect, it } from 'vitest'

import { formatUsageServiceTier, getUsageServiceTierLabel, normalizeUsageServiceTier, verifiedUltrafastBillingMultiplier } from '../usageServiceTier'

describe('usageServiceTier utils', () => {
  const official = { model: 'gpt-6.1-sol', service_tier: 'ultrafast', input_tokens: 53, input_cost: 0.000636, output_tokens: 1500, output_cost: 0.09 }
  it('proves the six-times label from actual prices, not the tier name', () => {
    expect(verifiedUltrafastBillingMultiplier(official)).toBe(6)
    expect(verifiedUltrafastBillingMultiplier({ ...official, output_cost: 0.015 })).toBeNull()
    expect(verifiedUltrafastBillingMultiplier({ ...official, service_tier: 'priority' })).toBeNull()
    expect(verifiedUltrafastBillingMultiplier({ ...official, model: 'gpt-5.6-sol' })).toBeNull()
    expect(verifiedUltrafastBillingMultiplier({ model: 'gpt-6.1-sol', service_tier: 'ultrafast' })).toBeNull()
    expect(verifiedUltrafastBillingMultiplier({ ...official, input_cost: NaN })).toBeNull()
  })
  it('accounts for long-context pricing separately and every cache bucket', () => {
    expect(verifiedUltrafastBillingMultiplier({ ...official, long_context_billing_applied: true, input_cost: 0.001272, output_cost: 0.135 })).toBe(6)
    expect(verifiedUltrafastBillingMultiplier({ ...official, cache_read_tokens: 1000, cache_read_cost: 0.0006, cache_creation_tokens: 1000, cache_creation_cost: 0.015 })).toBe(6)
    expect(verifiedUltrafastBillingMultiplier({ ...official, cache_read_tokens: 1000, cache_read_cost: 0.0001 })).toBeNull()
  })
  it('normalizes fast/default aliases', () => {
    expect(normalizeUsageServiceTier('fast')).toBe('priority')
    expect(normalizeUsageServiceTier(' default ')).toBe('standard')
    expect(normalizeUsageServiceTier('STANDARD')).toBe('standard')
  })

  it('preserves supported tiers', () => {
    expect(normalizeUsageServiceTier('priority')).toBe('priority')
    expect(normalizeUsageServiceTier('flex')).toBe('flex')
    expect(normalizeUsageServiceTier('ultrafast')).toBe('ultrafast')
  })

  it('formats empty values as standard', () => {
    expect(formatUsageServiceTier()).toBe('standard')
    expect(formatUsageServiceTier('')).toBe('standard')
  })

  it('passes through unknown non-empty tiers for display fallback', () => {
    expect(normalizeUsageServiceTier('custom-tier')).toBe('custom-tier')
    expect(formatUsageServiceTier('custom-tier')).toBe('custom-tier')
  })

  it('maps tiers to translated labels', () => {
    const translate = (key: string) => ({
      'usage.serviceTierPriority': 'Fast',
      'usage.serviceTierUltrafast': 'Ultrafast',
      'usage.serviceTierFlex': 'Flex',
      'usage.serviceTierStandard': 'Standard',
    })[key] ?? key

    expect(getUsageServiceTierLabel('fast', translate)).toBe('Fast')
    expect(getUsageServiceTierLabel('ultrafast', translate)).toBe('Ultrafast')
    expect(getUsageServiceTierLabel('flex', translate)).toBe('Flex')
    expect(getUsageServiceTierLabel(undefined, translate)).toBe('Standard')
    expect(getUsageServiceTierLabel('custom-tier', translate)).toBe('custom-tier')
  })
})
