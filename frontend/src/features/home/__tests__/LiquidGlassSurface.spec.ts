// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref, defineComponent } from 'vue'
import LiquidGlassSurface from '../LiquidGlassSurface.vue'

vi.mock('@vueuse/core', () => ({
  useMediaQuery: () => ref(false),
  useDocumentVisibility: () => ref('visible'),
  useIntersectionObserver: () => ({ stop: vi.fn() }),
}))

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('LiquidGlassSurface optical boundaries', () => {
  it('uses one smooth header geometry for clipping and refraction without organic mixing', () => {
    const wrapper = mount(LiquidGlassSurface, { props: { active: false, surface: 'header', variant: 'navigation' } })
    const clip = wrapper.get('clipPath path').attributes('d')
    expect(clip).toContain(' H ')
    expect(clip).toContain(' Q ')
    expect(clip).not.toContain(' C ')
    expect(wrapper.get('.water-glass__rim path').attributes('d')).toBe(clip)
    expect(wrapper.find('feComposite').exists()).toBe(false)
    expect(wrapper.get('feComponentTransfer').attributes('in')).toBe('lens-a')
    expect(wrapper.get('feDisplacementMap').attributes('scale')).toBe('32')
    expect(wrapper.findAll('clipPath ellipse')).toHaveLength(0)
    wrapper.unmount()
  })
  it('gives homepage prism lenses a stronger real displacement without changing organic console lenses', () => {
    const parent = mount(defineComponent({ components: { LiquidGlassSurface }, template: '<div><LiquidGlassSurface :active="true" surface="prism" /><LiquidGlassSurface :active="true" surface="prism" variant="navigation" /><LiquidGlassSurface :active="false" /></div>' }))
    const [hero, nav, organic] = parent.findAllComponents(LiquidGlassSurface)
    expect(hero.classes()).toContain('water-glass--prism')
    expect(hero.get('feDisplacementMap').attributes('scale')).toBe('148')
    expect(nav.get('feDisplacementMap').attributes('scale')).toBe('92')
    expect(organic.get('feDisplacementMap').attributes('scale')).toBe('100')
    expect(hero.findAll('clipPath ellipse')).toHaveLength(0)
    expect(organic.findAll('clipPath ellipse')).toHaveLength(5)
    expect(nav.find('feComposite').exists()).toBe(false)
    parent.unmount()
  })
  it('keeps backdrop sampling outside the clipped overlay ancestor', () => {
    const wrapper = mount(LiquidGlassSurface, { props: { active: false } })
    expect(wrapper.get('.water-glass__refraction').element.parentElement).toBe(wrapper.element)
    expect(wrapper.find('.water-glass__body .water-glass__refraction').exists()).toBe(false)
    expect(wrapper.findAll('clipPath ellipse')).toHaveLength(5)
    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(wrapper.find('.water-glass__reading').exists()).toBe(false)
    expect(wrapper.findAll('animate')).toHaveLength(0)
    wrapper.unmount()
  })

  it('isolates filter identifiers between the pool and navigation', () => {
    const parent = mount(defineComponent({ components: { LiquidGlassSurface }, template: '<div><LiquidGlassSurface :active="true" /><LiquidGlassSurface :active="true" variant="navigation" /></div>' }))
    const [pool, nav] = parent.findAllComponents(LiquidGlassSurface)
    expect(pool.get('clipPath').attributes('id')).not.toBe(nav.get('clipPath').attributes('id'))
    expect(pool.get('linearGradient').attributes('id')).not.toBe(nav.get('linearGradient').attributes('id'))
    expect(nav.findAll('animate')).toHaveLength(0)
    expect(nav.findAll('clipPath ellipse')).toHaveLength(0)
    expect(nav.find('canvas').exists()).toBe(false)
    expect(pool.get('filter').attributes('id')).not.toBe(nav.get('filter').attributes('id'))
    parent.unmount()
  })

  it('keeps a static lens but stops all automatic optical motion when disabled', async () => {
    const wrapper = mount(LiquidGlassSurface, { props: { active: true } })
    expect(wrapper.find('feDisplacementMap').exists()).toBe(true)
    expect(wrapper.classes()).toContain('water-glass--moving')
    await wrapper.setProps({ active: false })
    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(wrapper.find('.water-glass__reading').exists()).toBe(false)
    expect(wrapper.find('animate').exists()).toBe(false)
    expect(wrapper.classes()).not.toContain('water-glass--moving')
    wrapper.unmount()
  })
})
