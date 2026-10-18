/// <reference types="vite/client" />
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpsNodeResources from '../OpsNodeResources.vue'
import type { OpsSystemMetricsSnapshot } from '../../../../../api/admin/ops'
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const node = (id: string, cpu = 12) => ({ node_id: id, node_name: id.toUpperCase(), id: 1, created_at: new Date().toISOString(), window_minutes: 1, cpu_usage_percent: cpu, memory_usage_percent: 20, db_configured_budget: 96, db_max_open_conns: 88, db_conn_active: 3, db_conn_idle: 2 }) satisfies OpsSystemMetricsSnapshot

describe('per-node resources', () => {
  it('renders all nodes with exact overview-response highlighting and independent pool limits', async () => {
    const wrapper = mount(OpsNodeResources, { props: { nodes: [node('xigong3'), node('xigong'), node('xigong2')], responseNode: 'xigong3' } })
    expect(wrapper.findAll('article').map(x => x.attributes('data-node-id'))).toEqual(['xigong', 'xigong2', 'xigong3'])
    expect(wrapper.findAll('[data-current="true"]')).toHaveLength(1)
    expect(wrapper.get('[data-current="true"]').attributes('data-node-id')).toBe('xigong3')
    expect(wrapper.text()).toContain('5 / 88')
    await wrapper.setProps({ responseNode: 'xigong2' })
    expect(wrapper.get('[data-current="true"]').attributes('data-node-id')).toBe('xigong2')
    wrapper.unmount()
  })
  it('keeps missing and stale machines visible without displaying old CPU as current', () => {
    const stale = { ...node('xigong'), created_at: '2000-01-01T00:00:00Z', cpu_usage_percent: 99 }
    const wrapper = mount(OpsNodeResources, { props: { nodes: [stale, { ...node('xigong3'), created_at: '', cpu_usage_percent: null }] } })
    expect(wrapper.findAll('article')).toHaveLength(2)
    expect(wrapper.text()).toContain('clusterV3.metricsUnavailable')
    expect(wrapper.text()).not.toContain('99.0%')
    expect(wrapper.text()).toContain('—')
    wrapper.unmount()
  })
})
