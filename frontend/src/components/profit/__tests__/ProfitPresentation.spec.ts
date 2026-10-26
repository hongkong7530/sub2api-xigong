/// <reference path="../../../vite-env.d.ts" />
import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProfitOverview from '../ProfitOverview.vue'
import ProfitGroupList from '../ProfitGroupList.vue'
import ProfitProviderStage from '../ProfitProviderStage.vue'
import GroupCostEditor from '../../admin/profit/GroupCostEditor.vue'
import ProfitLedgerTable from '../../admin/profit/ProfitLedgerTable.vue'
import type { ProfitGroup, ProfitSnapshot, ProfitMetric } from '../../../api/profit'
const { setCost, entries } = vi.hoisted(() => ({ setCost: vi.fn(), entries: vi.fn() }))
vi.mock('@/api/profit', () => ({ profitApi: { setCost, entries } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const metric: ProfitMetric = { requests: 5, known_requests: 5, revenue: '0.3', known_revenue: '0.3', cost: '0.2', profit: '0.1', margin: '0.33333333', coverage: '1' }
const group: ProfitGroup = { id: 1, name: 'Public', platform: 'openai', status: 'active', rate: '0.3', peak_enabled: false, peak_start: '', peak_end: '', peak_multiplier: '1', at_cost: true, reference_margin: '-0.02', metrics: { total: metric, today: metric, week: metric }, cost_multiplier: '0.2', cost_version: 1 }
describe('profit presentation', () => {
  it('shows raw earned amount while keeping the special reference distinct', () => {
    const w = mount(ProfitGroupList, { props: { groups: [group], window: 'today' } })
    expect(w.text()).toContain('$0.10'); expect(w.text()).toContain('profit.atCost'); expect(w.text()).toContain('profit.reference'); expect(w.text()).toContain('-2.0%')
  })
  it('does not invent zero profit for unconfigured cost', () => {
    const w = mount(ProfitGroupList, { props: { groups: [{ ...group, at_cost: false, metrics: { ...group.metrics, today: { ...metric, known_requests: 0, profit: '0' } } }], window: 'today' } })
    expect(w.find('.profit-group__money').text()).toContain('—')
  })
  it('renders totals with their own margins and emits selected period', async () => {
    const data = { scope: 'public_groups', stale: false, metrics: group.metrics } as ProfitSnapshot
    const w = mount(ProfitOverview, { props: { data, selected: 'today' } })
    expect(w.find('[data-testid="profit-value-today"]').text()).toBe('$0.10')
    await w.findAll('button')[2]!.trigger('click'); expect(w.emitted('select')?.[0]).toEqual(['week'])
  })
  it('retains provider name when artwork fails and keeps accessible selection', async () => {
    const w = mount(ProfitProviderStage, { props: { providers: ['openai'], selected: 'openai' } })
    await w.find('img').trigger('error'); expect(w.find('img').exists()).toBe(false); expect(w.text()).toContain('OpenAI')
    expect(w.findAll('button')[1]!.attributes('aria-pressed')).toBe('true')
  })
  it('submits normalized intent with optimistic version and reports conflicts', async () => {
    setCost.mockReset()
    setCost.mockRejectedValueOnce({ response: { status: 409 } })
    const w = mount(GroupCostEditor, { props: { group } }); await w.find('input').setValue('0.25'); await w.find('form').trigger('submit'); await flushPromises()
    expect(setCost).toHaveBeenCalledWith(1, '0.25', 1); expect(w.text()).toContain('profit.conflict'); expect(w.emitted('saved')).toBeUndefined()
    setCost.mockResolvedValueOnce({}); await w.find('form').trigger('submit'); await flushPromises(); expect(w.emitted('saved')).toHaveLength(1)
    w.unmount()
  })
  it('keeps clear distinct from explicit zero and rejects invalid cost locally', async () => {
    setCost.mockReset().mockResolvedValue({})
    const w = mount(GroupCostEditor, { props: { group } })
    await w.find('input').setValue('1e3'); await w.find('form').trigger('submit'); await flushPromises()
    expect(setCost).not.toHaveBeenCalled(); expect(w.text()).toContain('profit.invalid')
    await w.find('input').setValue('0'); await w.find('form').trigger('submit'); await flushPromises()
    expect(setCost).toHaveBeenLastCalledWith(1, '0', 1)
    await w.find('button[type="button"]').trigger('click'); await flushPromises()
    expect(setCost).toHaveBeenLastCalledWith(1, null, 1)
    await w.setProps({ group: { ...group, status: 'deleted' } })
    expect(w.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    w.unmount()
  })
  it('resets pagination and sends exclusive ISO date bounds when filters change', async () => {
    entries.mockReset().mockResolvedValue({ items: [], page: 1, has_more: true })
    const w = mount(ProfitLedgerTable, { props: { groups: [group] } }); await flushPromises()
    await w.findAll('.profit-pagination button')[1]!.trigger('click'); await flushPromises()
    expect(entries.mock.lastCall?.[0].page).toBe(2)
    const start = '2026-10-19T09:00', end = '2026-10-19T10:00'
    await w.findAll('input')[0]!.setValue(start); await flushPromises()
    expect(entries.mock.lastCall?.[0]).toMatchObject({ page: 1, from: new Date(start).toISOString() })
    await w.findAll('input')[1]!.setValue(end); await w.find('select').setValue('1'); await flushPromises()
    expect(entries.mock.lastCall?.[0]).toMatchObject({ page: 1, group_id: 1, from: new Date(start).toISOString(), to: new Date(end).toISOString() })
    w.unmount()
  })
})
