// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { defaultAppearance } from '../../../api/siteContent'
import AppearancePreview from '../AppearancePreview.vue'
import AppearanceEditor from '../AppearanceEditor.vue'
import ExperienceShell from '../ExperienceShell.vue'
vi.mock('vue-i18n', async original => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({ useRoute: () => ({ fullPath: '/preview' }) }))
vi.mock('../../../api/siteContent', async original => ({ ...await original<typeof import('../../../api/siteContent')>(), siteContent: { appearance: vi.fn() } }))
const global = () => ({ plugins: [createPinia()], stubs: { AmbientBackdrop: true, ConsoleGlassSurface: true } })
afterEach(() => vi.unstubAllGlobals())
describe('isolated appearance editing', () => {
  it('changes only the preview theme, never the document or saved store', async () => {
    vi.stubGlobal('Image', class { decode() { return Promise.resolve() } })
    const before = document.documentElement.className
    const draft = defaultAppearance()
    const wrapper = mount(AppearancePreview, { props: { appearance: draft }, global: global() })
    await wrapper.get('.appearance-preview__toolbar button').trigger('click')
    expect(wrapper.getComponent(ExperienceShell).props('theme')).toBe('dark')
    expect(document.documentElement.className).toBe(before)
    expect(wrapper.getComponent(ExperienceShell).props('preview')).toBe(true)
    await flushPromises(); wrapper.unmount()
  })
  it('edits independent console fields and emits one explicit save', async () => {
    const draft = defaultAppearance(), originals = JSON.stringify(draft.wallpapers)
    const wrapper = mount(AppearanceEditor, { props: { modelValue: draft, busy: false }, global: { ...global(), stubs: { AppearancePreview: true } } })
    await wrapper.get('input[value="scene-3"]').setValue(true)
    expect(draft.console?.wallpaper_id).toBe('scene-3')
    expect(JSON.stringify(draft.wallpapers)).toBe(originals)
    const selects = wrapper.findAll('select')
    await selects[1].setValue('custom')
    await wrapper.get('input[placeholder="https://…"]').setValue('https://')
    await selects[1].setValue('wallpaper')
    expect(draft.console?.background_url).toBe('')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')).toHaveLength(1)
    wrapper.unmount()
  })
})
