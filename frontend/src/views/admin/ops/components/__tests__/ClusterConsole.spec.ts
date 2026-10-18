/// <reference types="vite/client" />
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import type { Component } from 'vue'
const ClusterConsole = import.meta.glob<Component>('../ClusterConsole.vue', { eager: true, import: 'default' })['../ClusterConsole.vue']!

const api = vi.hoisted(() => ({ getOverview: vi.fn(), getOperations: vi.fn(), getOperation: vi.fn(), getStorage: vi.fn(), previewRetention: vi.fn(), registerNode: vi.fn(), changeNode: vi.fn(), acknowledgeRedis: vi.fn() }))
vi.mock('@/api/admin/cluster', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const snapshot = () => ({ nodes: [{ config: { cluster_id: 'test', node_id: 'worker', name: 'Worker', roles: ['worker'], capabilities: ['http'], state: 'ready', revision: 3, max_in_flight: 8, max_queue: 2, db_connections: 8, base_weight: 1, monthly_limit_bytes: null }, status: { sampled_at: new Date().toISOString(), admission_ready: true, observed_revision: 3, active: 2, waiting: 0, pending_settlement: 0, cycle_traffic_bytes: null }, fresh: true, applied_revision: 2 }], shadow: true, redis_recovery_required: false, redis_generation: 'generation', data_authority: 'single_primary' })
const mountConsole = () => mount(ClusterConsole, { global: { stubs: { BaseDialog: { props: ['show'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' } } } })

describe('cluster control operation boundaries', () => {
  it('shows database budget separately and blocks a 1000-connection pool before submission', async () => {
    api.getOverview.mockResolvedValue({ ...snapshot(), response_node_id: 'worker', database_capacity: { budget: 260, allocated: 8, available: 252, server_max: 300, server_used: 40, server_reserved: 3 } })
    const wrapper = mountConsole(); await flushPromises()
    expect(wrapper.find('[data-testid="database-capacity"]').exists()).toBe(true)
    await wrapper.findAll('nav button')[1]!.trigger('click'); await flushPromises()
    expect(wrapper.get('[data-current="true"]').text()).toContain('Worker')
    await wrapper.findAll('button').find(b => b.text() === 'clusterV3.configure')!.trigger('click')
    const pool = wrapper.findAll('label').find(l => l.text().includes('clusterV3.dbConnections'))!.get('input')
    await pool.setValue(1000)
    expect(wrapper.get('button[form="cluster-node-editor"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#cluster-node-editor').trigger('submit'); await flushPromises()
    expect(api.changeNode).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks(); sessionStorage.clear(); Object.defineProperty(document, 'hidden', { configurable: true, value: false }); api.getOverview.mockResolvedValue(snapshot()); api.getOperations.mockResolvedValue([]) })
  // The shared client flattens errors to { status } and keeps no axios response. Reading
  // err.response.status therefore never matched, so a leftover key for an operation the
  // authority never recorded locked the console permanently.
  it('releases the lock when the authority has no record of the pending request', async () => {
    sessionStorage.setItem('cluster-v3-pending-operation', 'dead-key')
    api.getOperation.mockRejectedValue({ status: 404, code: 'NOT_FOUND', message: 'Node not found' })
    const wrapper = mountConsole(); await flushPromises()
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBeNull()
    expect(wrapper.text()).toContain('clusterV3.operationNotRecorded')
    expect(wrapper.text()).not.toContain('clusterV3.operationUncertain')
    wrapper.unmount()
  })
  it('releases the lock when the pending request is rejected as invalid', async () => {
    sessionStorage.setItem('cluster-v3-pending-operation', 'bad-key')
    api.getOperation.mockRejectedValue({ status: 400, code: 'INVALID', message: 'request_key must be a UUID' })
    const wrapper = mountConsole(); await flushPromises()
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBeNull()
    wrapper.unmount()
  })
  it('keeps the lock when the outcome is genuinely unknown', async () => {
    sessionStorage.setItem('cluster-v3-pending-operation', 'slow-key')
    api.getOperation.mockRejectedValue(new Error('network'))
    const wrapper = mountConsole(); await flushPromises()
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBe('slow-key')
    wrapper.unmount()
  })
  afterEach(() => { vi.useRealTimers() })
  it('shows unknown telemetry after refresh failure instead of a fresh healthy snapshot', async () => {
    const wrapper = mountConsole(); await flushPromises()
    expect(wrapper.emitted('enabled')?.[0]).toEqual([true])
    api.getOverview.mockRejectedValue(new Error('network'))
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(wrapper.text()).toContain('clusterV3.fetchFailed')
    expect(wrapper.text()).toContain('clusterV3.unknown')
    wrapper.unmount()
  })
  it('does not treat node observation as proxy configuration confirmation', async () => {
    const wrapper = mountConsole(); await flushPromises()
    await wrapper.findAll('nav button')[1]!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('clusterV3.pendingApply')
    expect(wrapper.text()).not.toContain('clusterV3.applied')
    wrapper.unmount()
  })
  it('stops polling while the page is hidden and cancels work on unmount', async () => {
    const wrapper = mountConsole(); await flushPromises()
    const signal = api.getOverview.mock.calls[0]?.[0] as AbortSignal
    Object.defineProperty(document, 'hidden', { configurable: true, value: true }); document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(15000)
    expect(api.getOverview).toHaveBeenCalledTimes(1); expect(signal.aborted).toBe(true)
    wrapper.unmount()
  })
  it('does not enable v3 UI when the deployment returns the legacy 404', async () => {
    // The shared client rejects with a flattened { status } and keeps no axios response,
    // so the deployment-404 branch is only reachable with that shape.
    api.getOverview.mockRejectedValue({ status: 404, code: 'NOT_FOUND', message: 'Cluster v3 is not enabled' })
    const wrapper = mountConsole(); await flushPromises()
    expect(wrapper.emitted('enabled')?.[0]).toEqual([false]); expect(wrapper.find('nav').exists()).toBe(false)
    wrapper.unmount()
  })
  it('classifies a configuration conflict and keeps the page out of the stale state', async () => {
    api.changeNode.mockRejectedValue({ status: 409, code: 'CONFLICT', message: 'Node or operation revision conflict' })
    const wrapper = mountConsole(); await flushPromises()
    await wrapper.findAll('nav button')[1]!.trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.pause')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.confirm')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('clusterV3.conflict')
    expect(wrapper.text()).not.toContain('clusterV3.fetchFailed')
    wrapper.unmount()
  })
  it('retries an uncertain mutation with the same request identity', async () => {
    api.changeNode.mockRejectedValueOnce({ response: { status: 503 }, message: 'ack lost' }).mockResolvedValueOnce({ id: 'operation', status: 'pending' })
    const wrapper = mountConsole(); await flushPromises()
    await wrapper.findAll('nav button')[1]!.trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.pause')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.confirm')!.trigger('click'); await flushPromises()
    const first = api.changeNode.mock.calls[0]
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBe(first?.[1].request_key)
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.retrySameOperation')!.trigger('click'); await flushPromises()
    expect(api.changeNode.mock.calls[1]).toEqual(first)
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBeNull()
    wrapper.unmount()
  })
  it('recovers an uncertain request after remount without another mutation', async () => {
    sessionStorage.setItem('cluster-v3-pending-operation', 'saved-request')
    // The console probes a leftover key on refresh, so the stub must answer for both the
    // automatic probe and the explicit query the test then performs.
    api.getOperation.mockResolvedValue({ id: 'recovered-operation', status: 'applied' })
    const wrapper = mountConsole(); await flushPromises()
    expect(sessionStorage.getItem('cluster-v3-pending-operation')).toBeNull()
    // The stubbed authority answers, so the lock is released and no query control remains;
    // the assertion below is what proves the recovery actually happened.
    expect(api.getOperation).toHaveBeenCalledWith('saved-request')
    expect(api.changeNode).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('recovered-operation')
    wrapper.unmount()
  })
  it('requires a typed node identity for force retirement', async () => {
    const wrapper = mountConsole(); await flushPromises()
    await wrapper.findAll('nav button')[1]!.trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'clusterV3.force_retire')!.trigger('click')
    const confirm = wrapper.findAll('button').find(button => button.text() === 'clusterV3.confirm')!
    expect(confirm.attributes('disabled')).toBeDefined(); expect(api.changeNode).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
