/// <reference path="../../../../vite-env.d.ts" />
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorFirstTokenSamples from '../MonitorFirstTokenSamples.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('MonitorFirstTokenSamples', () => {
  it('shows the latest sample rather than the median', () => {
    const wrapper = mount(MonitorFirstTokenSamples, { props: {
      samples: [12000, 1000, 2000].map(first_token_ms => ({ model: 'gpt-test', first_token_ms, created_at: '2026-10-14T00:00:00Z' })),
    } })
    expect(wrapper.get('[data-testid="first-token-latest"]').text()).toBe('12.00s')
  })

  it('does not substitute an older sample for a missing latest first token', () => {
    const wrapper = mount(MonitorFirstTokenSamples, { props: {
      samples: [null, 1000].map(first_token_ms => ({ model: 'gpt-test', first_token_ms, created_at: '2026-10-14T00:00:00Z' })),
    } })
    expect(wrapper.get('[data-testid="first-token-latest"]').text()).toBe('-')
  })

  it('uses the same 10s/30s/60s color boundaries as usage records', () => {
    const values = [null, 1999, 2000, 9999, 10000, 29999, 30000, 59999, 60000]
    const wrapper = mount(MonitorFirstTokenSamples, { props: {
      samples: values.map(first_token_ms => ({ model: 'gpt-test', first_token_ms, created_at: '2026-10-14T00:00:00Z' })),
    } })
    const bars = wrapper.findAll('[role="img"]').reverse()
    expect(bars[0].classes()).toContain('bg-gray-300')
    for (const index of [1, 2, 3]) expect(bars[index].classes()).toContain('bg-emerald-500')
    for (const index of [4, 5]) expect(bars[index].classes()).toContain('bg-amber-400')
    for (const index of [6, 7]) expect(bars[index].classes()).toContain('bg-orange-500')
    expect(bars[8].classes()).toContain('bg-red-500')
  })

  it('does not invent a latest value for missing samples', () => {
    const wrapper = mount(MonitorFirstTokenSamples, { props: {
      samples: [{ model: 'gpt-test', first_token_ms: null, created_at: '2026-10-14T00:00:00Z' }],
    } })
    expect(wrapper.get('[data-testid="first-token-latest"]').text()).toBe('-')
  })
})
