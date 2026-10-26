import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  use: vi.fn(),
  mount: vi.fn(),
  initI18n: vi.fn(),
  isReady: vi.fn(),
  router: {} as Record<string, unknown>,
}))
vi.mock('vue', async (original) => ({
  ...await original<typeof import('vue')>(),
  createApp: () => ({ use: mocks.use, mount: mocks.mount }),
}))
vi.mock('pinia', () => ({ createPinia: () => ({}) }))
vi.mock('../App.vue', () => ({ default: {} }))
vi.mock('../router', () => ({ default: mocks.router }))
vi.mock('../i18n', () => ({ default: {}, initI18n: mocks.initI18n }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ initFromInjectedConfig: vi.fn() }) }))
vi.mock('@/stores/experience', () => ({ useExperienceStore: () => ({ load: vi.fn().mockResolvedValue(undefined) }) }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/utils/device', () => ({ isIOSDevice: () => false }))

beforeEach(() => {
  vi.resetModules()
  vi.clearAllMocks()
  localStorage.setItem('theme', 'light')
  mocks.router.isReady = mocks.isReady
})
afterEach(() => localStorage.clear())

describe('application startup gates', () => {
  it.each(['locale', 'route'])('overlaps startup work but waits for both gates when %s finishes first', async (first) => {
    let finishLocale!: () => void
    let finishRoute!: () => void
    mocks.initI18n.mockImplementation(() => new Promise<void>(resolve => { finishLocale = resolve }))
    mocks.isReady.mockImplementation(() => new Promise<void>(resolve => { finishRoute = resolve }))

    await import('../main')
    expect(mocks.initI18n).toHaveBeenCalledOnce()
    expect(mocks.use).toHaveBeenCalledWith(mocks.router)
    expect(mocks.isReady).toHaveBeenCalledOnce()
    expect(mocks.mount).not.toHaveBeenCalled()

    ;(first === 'locale' ? finishLocale : finishRoute)()
    await flushPromises()
    expect(mocks.mount).not.toHaveBeenCalled()
    ;(first === 'locale' ? finishRoute : finishLocale)()
    await flushPromises()
    expect(mocks.mount).toHaveBeenCalledTimes(1)
    expect(mocks.mount).toHaveBeenCalledWith('#app')
  })
})
