/// <reference path="../../../vite-env.d.ts" />
import { nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import UsageCacheRate from '../UsageCacheRate.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh-CN'), t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })
const row = { request_type: 'stream', input_tokens: 549, cache_read_tokens: 3843, cache_creation_tokens: 0 }
describe('UsageCacheRate', () => {
 it('shows the actual rate, opens exact input buckets and restores focus on Escape', async () => {
  const wrapper = mount(UsageCacheRate, { props: { row }, attachTo: document.body })
  expect(wrapper.text()).toContain('87.5%')
  const trigger = wrapper.get('button')
  await trigger.trigger('click'); await nextTick()
  const panel = document.querySelector('[role="dialog"]') as HTMLElement
  expect(panel).not.toBeNull()
  expect(panel.textContent).toContain('3,843')
  expect(panel.textContent).toContain('549')
  expect(trigger.attributes('aria-expanded')).toBe('true')
  panel.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); await nextTick(); await nextTick()
  expect(document.querySelector('[role="dialog"]')).toBeNull()
  expect(document.activeElement).toBe(trigger.element)
 })
 it('distinguishes zero hits from missing sample and non-token billing', () => {
  const zero = mount(UsageCacheRate, { props: { row: { ...row, cache_read_tokens: 0 } } })
  expect(zero.text()).toContain('0.0%'); expect(zero.find('button').exists()).toBe(true)
  const missing = mount(UsageCacheRate, { props: { row: { request_type: 'stream', input_tokens: 0 } } })
  expect(missing.text()).toContain('usage.cacheDetails.noSample'); expect(missing.find('button').exists()).toBe(false)
  const image = mount(UsageCacheRate, { props: { row: { ...row, billing_mode: 'image' } } })
  expect(image.text()).toContain('usage.cacheDetails.notApplicable')
 })
})
