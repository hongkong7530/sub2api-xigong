/// <reference path="../../../../vite-env.d.ts" />
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorRateHistory from '../MonitorRateHistory.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
describe('monitor rate history presentation', () => {
  it('renders an initial horizontal line and the recorded timestamp instead of blank space', () => {
    const date = '2026-10-20T00:00:00Z'
    const w = mount(MonitorRateHistory, { props: { currentRate: .3, history: [{ rate_multiplier: .3, recorded_at: date, is_initial: true }] } })
    expect(w.get('[data-testid="rate-step-path"]').attributes('d')).toBe('M10 48 H630')
    expect(w.text()).toContain('monitorCommon.rateEvent.initial'); expect(w.get('time').attributes('datetime')).toBe(date)
    w.unmount()
  })
  it('describes increases and decreases with old and new values and specific dates', () => {
    const history = [{ rate_multiplier: .3, recorded_at: '2026-10-20T02:00:00Z' }, { rate_multiplier: .2, recorded_at: '2026-10-20T01:00:00Z' }, { rate_multiplier: .25, recorded_at: '2026-10-20T00:00:00Z', is_initial: true }]
    const w = mount(MonitorRateHistory, { props: { currentRate: .3, history } })
    expect(w.text()).toContain('monitorCommon.rateEvent.decrease'); expect(w.text()).toContain('monitorCommon.rateEvent.increase')
    expect(w.findAll('time').map(x => x.attributes('datetime'))).toEqual(history.map(x => x.recorded_at))
    expect(w.text()).toContain('0.25'); expect(w.text()).toContain('0.2'); w.unmount()
  })
  it('shows a current baseline without inventing a historical timestamp when history is absent', () => {
    const w = mount(MonitorRateHistory, { props: { currentRate: .3, history: [] } })
    expect(w.text()).toContain('monitorCommon.rateCurrentBaseline'); expect(w.find('time').exists()).toBe(false); expect(w.find('path[data-testid="rate-step-path"]').exists()).toBe(true); w.unmount()
  })
})
