// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import DataTable from '../DataTable.vue'
import PixelTransition from '../PixelTransition.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useExperienceMotion', () => ({ useExperienceMotion: () => ({ enabled: ref(false) }) }))
describe('pixel loading contracts', () => {
 it('only shows an initial loader without data and preserves current rows during refresh', async () => {
  const w = mount(DataTable, { props: { columns: [{ key: 'name', label: 'Name' }], data: [], loading: true } })
  expect(w.find('.pixel-loading').exists()).toBe(true)
  await w.setProps({ data: [{ id: 1, name: 'Existing result' }] })
  expect(w.text()).toContain('Existing result')
  expect(w.text()).toContain('experience.refreshing')
  expect(w.find('.pixel-loading').exists()).toBe(false)
  await w.setProps({ loading: false, data: [] })
  expect(w.text()).toContain('empty.noData')
  w.unmount()
 })
 it('does not hide content behind an animation when motion is disabled', () => {
  const w = mount(PixelTransition, { slots: { default: '<button>Action</button>' } })
  expect(w.find('button').text()).toBe('Action')
  expect(w.find('.pixel-transition__veil').exists()).toBe(false)
  w.unmount()
 })
})
