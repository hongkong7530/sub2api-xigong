/// <reference path="../../../../vite-env.d.ts" />
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import MonitorAutoProvisionDialog from '../MonitorAutoProvisionDialog.vue'
const mocks = vi.hoisted(() => ({ groups: vi.fn(), list: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAll: mocks.groups }, channelMonitor: { list: mocks.list } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const dialog = defineComponent({ template: '<div><slot/><slot name="footer"/></div>' })
const mountDialog = () => mount(MonitorAutoProvisionDialog, { props: { show: true, busy: false }, global: { stubs: { BaseDialog: dialog } } })
beforeEach(() => {
  mocks.groups.mockReset().mockResolvedValue([{ id: 9, name: 'OpenAI public', platform: 'openai', status: 'active', is_exclusive: false }, { id: 10, name: 'Gemini public', platform: 'gemini', status: 'active', is_exclusive: false }, { id: 11, name: 'Private', status: 'active', is_exclusive: true }])
  mocks.list.mockReset().mockResolvedValue({ items: [{ id: 1, group_id: 9, key_provision_mode: 'auto', probe_mode: 'hybrid', interval_seconds: 300 }], pages: 1 })
})
describe('monitor automation options', () => {
  it('requires explicit selection and preserves existing interval while submitting only chosen groups', async () => {
    const w = mountDialog(); await flushPromises()
    expect(w.findAll('article')).toHaveLength(2)
    expect(w.findAll('input[type="checkbox"]').every(x => !(x.element as HTMLInputElement).checked)).toBe(true)
    expect(w.find('button.btn-primary').attributes('disabled')).toBeDefined()
    const row = w.findAll('article')[0]!
    expect((row.find('input[type="number"]').element as HTMLInputElement).value).toBe('300')
    await row.find('input[type="checkbox"]').setValue(true)
    await row.find('select').setValue('passive')
    expect(row.find('input[type="number"]').attributes('disabled')).toBeDefined()
    await w.find('button.btn-primary').trigger('click')
    expect(w.emitted('submit')?.[0]).toEqual([[{ group_id: 9, probe_mode: 'passive', interval_seconds: 300 }]])
    w.unmount()
  })
  it('rejects invalid intervals and keeps execution disabled while busy', async () => {
    const w = mountDialog(); await flushPromises(); const row = w.findAll('article')[1]!
    await row.find('input[type="checkbox"]').setValue(true)
    await row.find('input[type="number"]').setValue(14)
    expect(w.find('button.btn-primary').attributes('disabled')).toBeDefined()
    await row.find('input[type="number"]').setValue(120)
    expect(w.find('button.btn-primary').attributes('disabled')).toBeUndefined()
    await w.setProps({ busy: true }); expect(w.find('button.btn-primary').attributes('disabled')).toBeDefined()
    w.unmount()
  })
  it('does not submit when group discovery fails', async () => {
    mocks.groups.mockRejectedValueOnce(new Error('offline'))
    const w = mountDialog(); await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(true)
    expect(w.find('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(w.emitted('submit')).toBeUndefined(); w.unmount()
  })
})
