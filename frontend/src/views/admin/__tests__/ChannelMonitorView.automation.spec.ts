/// <reference path="../../../vite-env.d.ts" />
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChannelMonitorView from '../ChannelMonitorView.vue'
import MonitorFiltersBar from '@/components/admin/monitor/MonitorFiltersBar.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), automate: vi.fn(), showError: vi.fn(), v1: true,
}))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorV1Mode: () => mocks.v1 }))
vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitor: { list: mocks.list, autoProvisionPublicGroups: mocks.automate } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => key === 'admin.channelMonitor.autoProvision.failedNotice' ? `Failed actions: ${params?.count}` : key }),
}))

const report = {
  groups_scanned: 2, monitors_created: 1, plaza_channels_created: 0, plaza_models_added: 0,
  items: [
    { group_id: 1, group_name: 'Created group', action: 'monitor_created', detail: 'created successfully' },
    { group_id: 2, group_name: 'Failed group', action: 'monitor_error', detail: 'specific creation failure' },
  ],
}
const row = { id: 1, name: 'monitor', provider: 'openai', group_id: 1, group_name: 'Bound group' }
const layout = defineComponent({ template: '<main><slot /></main>' })
const tableLayout = defineComponent({ template: '<section data-testid="legacy-list"><slot name="filters"/><slot name="table"/><slot name="pagination"/></section>' })
function mountView() {
  return mount(ChannelMonitorView, {
    attachTo: document.body,
    global: { stubs: {
      AppLayout: layout, TablePageLayout: tableLayout, DataTable: true, Pagination: true,
      ConfirmDialog: true, EmptyState: true, HelpTooltip: true, Icon: true, Toggle: true, Select: true,
      MonitorFormDialog: true, MonitorTemplateManagerDialog: true, MonitorRunResultDialog: true,
      MonitorSettingsPanel: { template: '<div data-testid="v2-panel"/>' },
      MonitorGroupInsightCard: { props: ['groupId'], template: '<div data-testid="insight"/>' },
    } },
  })
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

beforeEach(() => {
  localStorage.clear()
  mocks.v1 = true
  mocks.list.mockReset().mockResolvedValue({ items: [row], total: 1 })
  mocks.automate.mockReset().mockResolvedValue(report)
  mocks.showError.mockReset()
})

describe('channel monitor automation end-to-end view wiring', () => {
  it('keeps insights and the legacy list together, and hides both on V2', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="insight"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="legacy-list"]').exists()).toBe(true)
    await wrapper.findAll('[role="tab"]')[0]!.trigger('click')
    expect(wrapper.find('[data-testid="legacy-list"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="insight"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="v2-panel"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('starts V2 without showing or loading the legacy automation button', async () => {
    mocks.v1 = false
    const wrapper = mountView()
    await flushPromises()
    expect(mocks.list).not.toHaveBeenCalled()
    expect(wrapper.findComponent(MonitorFiltersBar).exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows explicit progress, prevents repeat clicks, then shows the real report before refresh finishes', async () => {
    const pending = deferred<typeof report>()
    const refresh = deferred<{ items: typeof row[]; total: number }>()
    mocks.automate.mockReturnValueOnce(pending.promise)
    mocks.list.mockResolvedValueOnce({ items: [row], total: 1 }).mockReturnValueOnce(refresh.promise)
    const wrapper = mountView()
    await flushPromises()
    const filters = wrapper.getComponent(MonitorFiltersBar)
    filters.vm.$emit('auto-provision')
    filters.vm.$emit('auto-provision')
    await wrapper.vm.$nextTick()
    expect(mocks.automate).toHaveBeenCalledTimes(1)
    expect(filters.find('[aria-busy="true"]').exists()).toBe(true)
    expect(filters.text()).toContain('admin.channelMonitor.autoProvision.runningHint')
    pending.resolve(report)
    await flushPromises()
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog?.textContent).toContain('Created group')
    expect(dialog?.textContent).toContain('specific creation failure')
    expect(dialog?.textContent).toContain('Failed actions: 1')
    expect(filters.find('[aria-busy="true"]').exists()).toBe(false)
    refresh.resolve({ items: [row], total: 1 })
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('retains a completed report when the follow-up list refresh fails', async () => {
    mocks.list.mockResolvedValueOnce({ items: [], total: 0 }).mockRejectedValueOnce(new Error('refresh failed'))
    const wrapper = mountView()
    await flushPromises()
    wrapper.getComponent(MonitorFiltersBar).vm.$emit('auto-provision')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('Created group')
    expect(mocks.showError).toHaveBeenCalledWith('refresh failed')
    wrapper.unmount()
  })

  it('uses a request-error fallback rather than a zero-action failure report', async () => {
    mocks.automate.mockRejectedValueOnce({})
    const wrapper = mountView()
    await flushPromises()
    wrapper.getComponent(MonitorFiltersBar).vm.$emit('auto-provision')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('admin.channelMonitor.autoProvision.requestFailed')
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    wrapper.unmount()
  })
})
