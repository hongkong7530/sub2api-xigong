// eslint-disable-next-line @typescript-eslint/triple-slash-reference
/// <reference path="../../../vite-env.d.ts" />
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub, flushPromises } from '@vue/test-utils'
import { reactive, ref, nextTick } from 'vue'
import ImmersiveHome from '../ImmersiveHome.vue'
import { defaultAppearance } from '../../../api/siteContent'

const { appStore } = vi.hoisted(() => ({ appStore: { siteName: '西宫API', siteLogo: '', cachedPublicSettings: {} } }))
vi.mock('@/stores', () => ({
  useAppStore: () => reactive(appStore),
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false }),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/composables/useExperienceMotion', () => ({ useExperienceMotion: () => ({ enabled: ref(false), toggle: vi.fn() }) }))
vi.mock('../useWallpaperPalette', () => ({ useWallpaperPalette: () => ({ style: ref({}) }) }))
vi.mock('@/api/siteContent', async original => ({ ...await original<typeof import('../../../api/siteContent')>(), siteContent: { quote: vi.fn(async () => ({ en: '', zh: 'Cached external quote' })) } }))

function renderHome() {
  return mount(ImmersiveHome, {
    props: { appearance: defaultAppearance() },
    global: { stubs: { RouterLink: RouterLinkStub, WallpaperStage: true, LiquidGlassSurface: true, AmbientBackdrop: true, PixelLoading: true, ParticleBrand: true, HomeSiteStats: true } },
  })
}

describe('ImmersiveHome brand', () => {
  it('changes a singleton quote even when the remote response is cached', async () => {
    const wrapper = renderHome()
    await flushPromises()
    const before = wrapper.get('.hero-quote').text()
    await wrapper.get('.hero-quote button').trigger('click')
    await flushPromises()
    expect(wrapper.get('.hero-quote').text()).not.toBe(before)
    wrapper.unmount()
  })
  beforeEach(() => {
    appStore.siteLogo = ''
    sessionStorage.clear()
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
  })
  afterEach(() => vi.unstubAllGlobals())

  it.each(['/branding/site.png', 'https://example.com/site.png', 'data:image/png;base64,aGVsbG8='])(
    'uses the configured site logo %s like the sidebar', (logo) => {
      appStore.siteLogo = logo
      const wrapper = renderHome()
      expect(wrapper.get('.home-mark img').attributes('src')).toBe(logo)
      expect(wrapper.get('.home-brand').text()).toContain('西宫API')
      wrapper.unmount()
    },
  )

  it.each(['', 'javascript:alert(1)'])('falls back to the default logo for %s', (logo) => {
    appStore.siteLogo = logo
    const wrapper = renderHome()
    expect(wrapper.get('.home-mark img').attributes('src')).toBe('/logo.svg')
    wrapper.unmount()
  })

  it('updates the logo when public settings arrive after mount', async () => {
    const wrapper = renderHome()
    reactive(appStore).siteLogo = '/branding/loaded.png'
    await nextTick()
    expect(wrapper.get('.home-mark img').attributes('src')).toBe('/branding/loaded.png')
    wrapper.unmount()
  })
})
