import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageColumnSettings from '../UsageColumnSettings.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const mountSettings = () => mount(UsageColumnSettings, {
  props: {
    columns: [{ key: 'model', label: 'Model' }, { key: 'group', label: 'Group' }, { key: 'created_at', label: 'Time' }],
    hiddenColumns: new Set(['group']), alwaysVisible: ['created_at'],
  },
})

describe('UsageColumnSettings', () => {
  it('allows visibility changes but retains required columns', async () => {
    const wrapper = mountSettings()
    expect(wrapper.get('[data-testid="usage-column-toggle-group"]').attributes('aria-pressed')).toBe('false')
    await wrapper.get('[data-testid="usage-column-toggle-group"]').trigger('click')
    expect(wrapper.emitted('toggle')).toEqual([['group']])
    expect(wrapper.get('[data-testid="usage-column-toggle-created_at"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('provides keyboard and touch friendly up/down buttons, including required columns', async () => {
    const wrapper = mountSettings()
    expect(wrapper.get('[data-testid="usage-column-up-model"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="usage-column-down-created_at"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="usage-column-up-created_at"]').trigger('click')
    await wrapper.get('[data-testid="usage-column-down-model"]').trigger('click')
    expect(wrapper.emitted('shift')).toEqual([['created_at', -1], ['model', 1]])
    wrapper.unmount()
  })
  it('emits drag reorder only from a drag started in this panel', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-column-key="model"]').trigger('drop')
    expect(wrapper.emitted('move')).toBeUndefined()
    await wrapper.get('[data-column-key="created_at"] [draggable="true"]').trigger('dragstart')
    await wrapper.get('[data-column-key="model"]').trigger('drop')
    expect(wrapper.emitted('move')).toEqual([['created_at', 'model']])
    wrapper.unmount()
  })
  it('cancels an abandoned drag', async () => {
    const wrapper = mountSettings()
    const handle = wrapper.get('[data-column-key="created_at"] [draggable="true"]')
    await handle.trigger('dragstart')
    await handle.trigger('dragend')
    await wrapper.get('[data-column-key="model"]').trigger('drop')
    expect(wrapper.emitted('move')).toBeUndefined()
    wrapper.unmount()
  })
  it('resets only order and closes on Escape', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-testid="usage-column-reset-order"]').trigger('click')
    expect(wrapper.emitted('reset')).toHaveLength(1)
    expect(wrapper.emitted('toggle')).toBeUndefined()
    await wrapper.get('[data-testid="usage-column-order-panel"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
