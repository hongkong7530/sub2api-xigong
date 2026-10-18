// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { defaultConsoleAppearance } from '../../../api/siteContent'
import ConsoleBackdrop from '../ConsoleBackdrop.vue'

afterEach(() => vi.unstubAllGlobals())
describe('ConsoleBackdrop loading boundary', () => {
  it('invalidates an old decode after switching resources', async () => {
    const resolvers: (() => void)[] = []
    vi.stubGlobal('Image', class { src = ''; decode() { return new Promise<void>(resolve => resolvers.push(resolve)) } })
    const config = defaultConsoleAppearance()
    const wrapper = mount(ConsoleBackdrop, { props: { config } })
    await wrapper.setProps({ config: { ...config, wallpaper_id: 'scene-2' } })
    resolvers[0](); await flushPromises()
    expect(wrapper.find('img').exists()).toBe(false)
    resolvers[1](); await flushPromises()
    expect(wrapper.get('img').attributes('src')).toContain('scene-2')
    expect(wrapper.get('img').attributes('referrerpolicy')).toBe('no-referrer')
    wrapper.unmount()
  })
  it('keeps a readable empty scene on image failure', async () => {
    vi.stubGlobal('Image', class { decode() { return Promise.reject(new Error('image')) } })
    const wrapper = mount(ConsoleBackdrop, { props: { config: defaultConsoleAppearance() } })
    await flushPromises()
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.emitted('error')?.at(-1)).toEqual([true])
    expect(wrapper.find('.console-backdrop__shade').exists()).toBe(true)
    wrapper.unmount()
  })
})
