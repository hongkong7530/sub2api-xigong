// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import WallpaperStage from '../WallpaperStage.vue'
const state = vi.hoisted(() => ({ reduced: false }))
vi.mock('@/composables/useExperienceMotion', () => ({ useExperienceMotion: () => ({ enabled: ref(!state.reduced) }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@vueuse/core', () => ({ useMediaQuery: (q: string) => ref(!q.includes('max-width')), useDocumentVisibility: () => ref('visible'), useIntersectionObserver: () => ({ stop: vi.fn() }) }))
describe('WallpaperStage', () => {
 beforeEach(() => { vi.useFakeTimers(); state.reduced = false; vi.stubGlobal('Image', class { src = ''; decode() { return Promise.resolve() } }); vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1)); vi.stubGlobal('cancelAnimationFrame', vi.fn()) })
 afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
 const images = [{ id: 'scene-1', focus_x: 70, focus_y: 45 }, { id: 'scene-2', focus_x: 25, focus_y: 50 }]
 it('decodes the next image before exposing it and supports explicit controls', async () => {
  const wrapper = mount(WallpaperStage, { props: { images, interval: 10, active: true } }); await flushPromises()
  expect(wrapper.get('.wallpaper-peek').attributes('src')).toContain('scene-2')
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
 })
 it('does not auto-play under reduced motion or while a content action has focus', async () => {
  state.reduced = true
  const wrapper = mount(WallpaperStage, { props: { images, interval: 10, active: true, interactionPaused: true } }); await flushPromises(); await vi.advanceTimersByTimeAsync(30000)
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-1'); wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
 })
 it('emits the current wallpaper and forwards full-width pointer coordinates without the old 15% offset', async () => {
  const wrapper = mount(WallpaperStage, { props: { images, interval: 10, active: true } }); await flushPromises()
  expect(wrapper.emitted('change')?.[0]).toEqual(['scene-1'])
  await wrapper.get('.wallpaper-stage').trigger('pointermove', { pointerType: 'mouse', clientX: 100, clientY: 200 })
  expect(wrapper.get('.wallpaper-peek').attributes('style')).toContain('100px 200px')
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.emitted('change')?.at(-1)).toEqual(['scene-2'])
  wrapper.unmount()
 })
 it('retains an outgoing image for the depth crossfade, then removes it', async () => {
  const wrapper = mount(WallpaperStage, { props: { images, interval: 5, active: true } }); await flushPromises()
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-outgoing .wallpaper-image').attributes('src')).toContain('scene-1')
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  expect(wrapper.find('.wallpaper-depth').exists()).toBe(true)
  await vi.advanceTimersByTimeAsync(1020)
  expect(wrapper.find('.wallpaper-outgoing').exists()).toBe(false)
  wrapper.unmount()
 })
 it('auto-plays at five seconds even while the pointer stays over the wallpaper', async () => {
  const wrapper = mount(WallpaperStage, { props: { images, interval: 5, active: true } }); await flushPromises()
  await wrapper.get('.wallpaper-stage').trigger('pointermove', { pointerType: 'mouse', clientX: 100, clientY: 200 })
  await vi.advanceTimersByTimeAsync(5000); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
 })
 it('skips animated layers when motion is reduced but still permits manual navigation', async () => {
  state.reduced = true
  const wrapper = mount(WallpaperStage, { props: { images, interval: 5, active: true } }); await flushPromises()
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  expect(wrapper.find('.wallpaper-outgoing').exists()).toBe(false)
  wrapper.unmount()
 })
 it('invalidates an automatic decode after pause but still permits a manual change', async () => {
  let resolveDecode: (() => void) | undefined
  let calls = 0
  vi.stubGlobal('Image', class { src = ''; decode() { calls++; return calls === 1 ? new Promise<void>(resolve => { resolveDecode = resolve }) : Promise.resolve() } })
  const wrapper = mount(WallpaperStage, { props: { images, interval: 8, active: true } }); await flushPromises()
  await vi.advanceTimersByTimeAsync(8000)
  expect(resolveDecode).toBeTypeOf('function')
  await wrapper.get('[aria-label="experience.pause"]').trigger('click')
  resolveDecode?.(); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-1')
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
 })
 it('clears the reveal when a pointer enters a foreground button', async () => {
  const wrapper = mount(WallpaperStage, { props: { images, interval: 8, active: true } }); await flushPromises()
  await wrapper.get('.wallpaper-stage').trigger('pointermove', { pointerType: 'mouse', clientX: 100, clientY: 200 })
  expect(wrapper.get('.wallpaper-peek').attributes('style')).toContain('opacity: 1')
  await wrapper.get('[aria-label="experience.next"]').trigger('pointermove', { pointerType: 'mouse' })
  expect(wrapper.get('.wallpaper-peek').attributes('style')).toContain('opacity: 0')
  wrapper.unmount()
 })
 it('queues rapid navigation until the current blend finishes', async () => {
  const third = { id: 'scene-3', focus_x: 50, focus_y: 50 }
  const wrapper = mount(WallpaperStage, { props: { images: [...images, third], interval: 5, active: true } }); await flushPromises()
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-2')
  expect(wrapper.get('.wallpaper-outgoing .wallpaper-image').attributes('src')).toContain('scene-1')
  await vi.advanceTimersByTimeAsync(1020); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-3')
  wrapper.unmount(); expect(vi.getTimerCount()).toBe(0)
 })
 it('does not start another image request after an unmounted decode rejects', async () => {
  let rejectDecode: ((error: Error) => void) | undefined
  let calls = 0
  vi.stubGlobal('Image', class { src = ''; decode() { calls++; return new Promise<void>((_resolve, reject) => { rejectDecode = reject }) } })
  const wrapper = mount(WallpaperStage, { props: { images: [...images, { id: 'scene-3', focus_x: 50, focus_y: 50 }], interval: 5, active: true } }); await flushPromises()
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  wrapper.unmount(); rejectDecode?.(new Error('offline')); await flushPromises()
  expect(calls).toBe(1)
  expect(vi.getTimerCount()).toBe(0)
 })
 it('keeps the current image when decoding the next image fails', async () => {
  vi.stubGlobal('Image', class { src = ''; decode() { return Promise.reject(new Error('offline')) } })
  const wrapper = mount(WallpaperStage, { props: { images, interval: 10, active: true } }); await flushPromises()
  expect(wrapper.find('.wallpaper-peek').exists()).toBe(false)
  await wrapper.get('[aria-label="experience.next"]').trigger('click'); await flushPromises()
  expect(wrapper.get('.wallpaper-current > .wallpaper-image').attributes('src')).toContain('scene-1'); wrapper.unmount()
 })
})
