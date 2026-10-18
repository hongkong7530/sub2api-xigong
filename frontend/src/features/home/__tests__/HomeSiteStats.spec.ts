import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { shallowRef, nextTick } from 'vue'
import HomeSiteStats from '../HomeSiteStats.vue'
import type { SiteStats } from '../../../api/siteContent'
const { state } = vi.hoisted(() => ({ state: {} as Record<string, unknown> }))
vi.mock('../useSiteStats', () => ({ useSiteStats: () => state }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: shallowRef('en-US') }) }))
vi.mock('@vueuse/core', () => ({
 useMediaQuery: () => shallowRef(false),
 useDocumentVisibility: () => shallowRef('visible'),
 useIntersectionObserver: (_host: unknown, callback: (entries: { isIntersecting: boolean }[]) => void) => { callback([{ isIntersecting: true }]); return { stop: vi.fn() } },
}))
afterEach(() => vi.restoreAllMocks())
function setup(data?: SiteStats) {
 state.data = shallowRef(data)
 state.failed = shallowRef(false)
 state.revision = shallowRef(1)
 return mount(HomeSiteStats, { props: { active: true } })
}
describe('glass site stats', () => {
 it('renders unknown values instead of invented zeroes', () => {
  const wrapper = setup()
  expect(wrapper.findAll('dl strong').map(node => node.text())).toEqual(['—', '—', '—', '—', '—'])
  expect(wrapper.text()).toContain('experience.modelsUnavailable')
  wrapper.unmount()
 })
 it('uses real model ratios and keeps stale values readable when motion is off', async () => {
  const wrapper = setup({ usage: { requests: 1200, today_tokens: 4000000, average_per_million: .13, price_samples: 12, sampled_at: '2026-10-16T08:00:00Z', day: '2026-10-16', models: [{ model: 'gpt-6.1-sol', tokens: 3000000, requests: 10 }, { model: 'deepseek-v4.1-flash', tokens: 1000000, requests: 2 }] }, online_users: 8, in_flight: 3, sampled_at: '2026-10-16T08:00:00Z', refresh_seconds: 3, online_window_seconds: 300 })
  expect(wrapper.classes()).toContain('site-stats--moving')
  expect(wrapper.get('.site-stats__model-track i').attributes('style')).toContain('scaleX(1)')
  expect(wrapper.text()).toContain('1,200')
  ;(state.failed as ReturnType<typeof shallowRef<boolean>>).value = true
  await wrapper.setProps({ active: false }); await nextTick()
  expect(wrapper.classes()).not.toContain('site-stats--moving')
  expect(wrapper.classes()).toContain('site-stats--stale')
  expect(wrapper.text()).toContain('experience.statsStale')
  expect(wrapper.text()).toContain('1,200')
  wrapper.unmount()
 })
})
