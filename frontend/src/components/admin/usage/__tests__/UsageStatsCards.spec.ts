/// <reference path="../../../../vite-env.d.ts" />
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheTotal': 'Cache',
  'usage.cacheRate': 'Cache rate {rate}%',
  'usage.cacheRateUnavailable': '—',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        let message = messages[key] ?? key
        for (const [name, value] of Object.entries(params ?? {})) {
          message = message.replace(`{${name}}`, value)
        }
        return message
      },
    }),
  }
})

const stats = {
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('shows cache-read hits over all input buckets without changing token totals', () => {
    const wrapper = mount(UsageStatsCards, { props: { stats }, global: { stubs: { Icon: true } } })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe('Cache rate 16.4%')
    expect(wrapper.text()).toContain('184')
  })

  it('shows unknown rather than zero when the summary has no tokens', () => {
    const wrapper = mount(UsageStatsCards, { props: { stats: null }, global: { stubs: { Icon: true } } })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe('—')
  })

  it.each([
    {
      name: 'first screenshot', total_input_tokens: 429_300, total_output_tokens: 10_170_000,
      total_cache_tokens: 793_580_000, total_cache_creation_tokens: 66_660_000,
      total_cache_read_tokens: 726_910_000, total_tokens: 804_170_000, expected: '91.5',
    },
    {
      name: 'second screenshot', total_input_tokens: 429_310, total_output_tokens: 10_190_000,
      total_cache_tokens: 794_210_000, total_cache_creation_tokens: 66_690_000,
      total_cache_read_tokens: 727_520_000, total_tokens: 804_820_000, expected: '91.6',
    },
  ])('uses input cache hits for $name rather than cache share', ({ name: _name, expected, ...tokens }) => {
    // Screenshot figures are rounded; these fixtures test those stated values,
    // not the unavailable full-precision production aggregates.
    const wrapper = mount(UsageStatsCards, { props: { stats: { ...stats, ...tokens } }, global: { stubs: { Icon: true } } })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe(`Cache rate ${expected}%`)
  })

  it('does not change cache hit rate when output token volume changes', async () => {
    const wrapper = mount(UsageStatsCards, { props: { stats }, global: { stubs: { Icon: true } } })
    await wrapper.setProps({ stats: { ...stats, total_output_tokens: 1_000_000, total_tokens: 1_000_134 } })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe('Cache rate 16.4%')
  })

  it('counts cache creation in the denominator but not as a hit', () => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats: { ...stats, total_input_tokens: 10, total_cache_tokens: 90, total_cache_creation_tokens: 90, total_cache_read_tokens: 0 } },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe('Cache rate 0.0%')
  })

  it('shows unknown for output-only totals without an input sample', () => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats: { ...stats, total_input_tokens: 0, total_cache_tokens: 0, total_cache_creation_tokens: 0, total_cache_read_tokens: 0 } },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.get('[data-testid="usage-summary-cache-rate"]').text()).toBe('—')
  })

  it('shows cache token breakdown values', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache: 34')
    expect(text).toContain('Cache Token Breakdown')
    expect(text).toContain('Cache Creation')
    expect(text).toContain('12')
    expect(text).toContain('Cache Read')
    expect(text).toContain('22')
  })

  it('keeps the cache tooltip out of the layout while it is hidden', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const tooltip = wrapper.findAll('span').find((el) => el.classes().includes('group-hover:block'))

    expect(tooltip).toBeDefined()
    // `opacity-0` hides the tooltip visually but keeps it in the layout, so its
    // fixed width still widens the document and causes horizontal scrolling on
    // narrow screens. `hidden` (display: none) takes it out of the flow.
    expect(tooltip?.classes()).toContain('hidden')
    expect(tooltip?.classes()).not.toContain('opacity-0')
  })
})
