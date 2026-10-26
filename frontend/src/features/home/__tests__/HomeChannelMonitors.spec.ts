import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { shallowRef } from 'vue'
import type { UserMonitorView } from '../../../api/channelMonitor'
import HomeChannelMonitors from '../HomeChannelMonitors.vue'

const { state, motion } = vi.hoisted(() => ({ state: {} as Record<string, any>, motion: {} as Record<string, any> }))
vi.mock('../useHomeMonitors', () => ({ useHomeMonitors: () => state }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, te: () => true, locale: shallowRef('en-US') }),
}))
vi.mock('@vueuse/core', () => ({
  useMediaQuery: () => motion.reduced,
  useDocumentVisibility: () => motion.visibility,
  useIntersectionObserver: (_host: unknown, callback: (entries: { isIntersecting: boolean }[]) => void) => { callback([{ isIntersecting: true }]); return { stop: vi.fn() } },
}))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorQuotaVisible: () => true }))

const item: UserMonitorView = {
  id: 1, name: 'Real channel', provider: 'openai', group_name: 'Public group', group_id: 7,
  primary_model: '', primary_status: 'failed', overall_status: 'failed', status_source: 'user_request',
  primary_latency_ms: null, primary_first_token_ms: null, primary_output_tokens_per_second: null,
  availability_7d: null, extra_models: [],
  timeline: [{ source: 'user_request', model: '', status: 'failed', latency_ms: null, first_token_ms: null, checked_at: '2026-10-16T12:00:00Z' }],
  group_insight: { rate_multiplier: 1, effective_rate_multiplier: 1.25, peak_rate_enabled: false, peak_rate_multiplier: 1, applied_peak_multiplier: 1, cache_hit_rate: null, cache_window_seconds: 1800, recent_first_tokens: [] },
}
let dispose: (() => void) | undefined
beforeEach(() => {
  state.data = shallowRef({ enabled: true, items: [item], sampled_at: '2026-10-16T12:00:00Z', refresh_seconds: 5 })
  state.loading = shallowRef(false)
  state.failed = shallowRef(false)
  state.countdown = shallowRef(5)
  state.refresh = vi.fn()
  motion.reduced = shallowRef(false)
  motion.visibility = shallowRef('visible')
})
afterEach(() => { dispose?.(); dispose = undefined })
function setup(active = true) {
  const wrapper = mount(HomeChannelMonitors, { props: { active }, global: { stubs: { ProviderIcon: true } } })
  dispose = () => wrapper.unmount()
  return wrapper
}
describe('public homepage channel cards', () => {
  it('keeps provider ordering across reversed refresh responses while updating metrics', async () => {
    const a = { ...item, id: 3, provider: 'anthropic', name: 'Anthropic channel' }
    const b = { ...item, id: 4, provider: 'gemini', name: 'Gemini channel' }
    state.data.value = { ...state.data.value, items: [item, b, a] }
    const wrapper = setup()
    const names = () => wrapper.findAll('article.monitor-card--home').map(c => c.text().match(/Anthropic channel|Gemini channel|Real channel/)?.[0])
    expect(names()).toEqual(['Anthropic channel', 'Gemini channel', 'Real channel'])
    state.data.value = { ...state.data.value, items: [b, a, { ...item, primary_latency_ms: 123 }] }
    await flushPromises()
    expect(names()).toEqual(['Anthropic channel', 'Gemini channel', 'Real channel'])
  })
  it('renders the existing complete card read-only, preserving latest failure and unknown metrics', () => {
    const wrapper = setup()
    const card = wrapper.get('article.monitor-card--home')
    expect(card.attributes('data-status')).toBe('failed')
    expect(card.text()).toContain('Real channel')
    expect(card.text()).toContain('1.2500')
    expect(card.text()).toContain('monitorCommon.cacheRateWindowTab.30m')
    expect(card.findAll('.monitor-metrics__value').map(n => n.text())).toEqual(['monitorCommon.latencyEmptyms', '—tok/s'])
    expect(card.findAll('.monitor-timeline__bar')).toHaveLength(60)
    expect(card.get('.monitor-availability__value').text()).toBe('monitorCommon.latencyEmpty')
    expect(card.find('button').exists()).toBe(false)
    expect(wrapper.find('[data-testid="monitor-quota-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="user-first-token-samples"]').exists()).toBe(true)
  })
  it('gates the glow motion with the site preference, reduced motion and document visibility', async () => {
    const wrapper = setup()
    expect(wrapper.classes()).toContain('home-monitors--moving')
    await wrapper.setProps({ active: false })
    expect(wrapper.classes()).not.toContain('home-monitors--moving')
    await wrapper.setProps({ active: true })
    motion.reduced.value = true; await flushPromises()
    expect(wrapper.find('.monitor-card--moving').exists()).toBe(false)
    motion.reduced.value = false; motion.visibility.value = 'hidden'; await flushPromises()
    expect(wrapper.classes()).not.toContain('home-monitors--moving')
  })
  it('has explicit loading, failure/retry and disabled empty states without invented cards', async () => {
    state.data.value = undefined; state.loading.value = true
    const wrapper = setup()
    expect(wrapper.findAll('.home-monitors__skeleton')).toHaveLength(3)
    state.loading.value = false; state.failed.value = true; await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('experience.monitorUnavailable')
    await wrapper.get('button').trigger('click')
    expect(state.refresh).toHaveBeenCalledTimes(1)
    state.failed.value = false; state.data.value = { enabled: false, items: [], sampled_at: '2026-10-16T12:00:00Z', refresh_seconds: 5 }; await flushPromises()
    expect(wrapper.text()).toContain('experience.monitorEmpty')
    expect(wrapper.find('.monitor-card').exists()).toBe(false)
  })
})
