import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useExperienceStore } from '../experience'
import { defaultAppearance, siteContent } from '../../api/siteContent'

vi.mock('../../api/siteContent', async importOriginal => ({
  ...await importOriginal<typeof import('../../api/siteContent')>(),
  siteContent: { appearance: vi.fn() },
}))

beforeEach(() => { setActivePinia(createPinia()); vi.clearAllMocks() })

describe('experience initial request', () => {
  it('deduplicates bootstrap and mount without refreshing resolved appearance', async () => {
    let resolve!: (value: ReturnType<typeof defaultAppearance>) => void
    vi.mocked(siteContent.appearance).mockReturnValue(new Promise(done => { resolve = done }))
    const store = useExperienceStore()
    const first = store.load()
    const second = store.load()
    expect(store.loading).toBe(true)
    expect(store.appearance).toBeNull()
    expect(siteContent.appearance).toHaveBeenCalledTimes(1)
    resolve(defaultAppearance())
    await Promise.all([first, second])
    await store.load()
    expect(siteContent.appearance).toHaveBeenCalledTimes(1)
    expect(store.loading).toBe(false)
    expect(store.appearance?.home_mode).toBe('immersive')
  })

  it('shows a fresh busy state on retry and never invents legacy configuration', async () => {
    vi.mocked(siteContent.appearance).mockRejectedValueOnce(new Error('unavailable'))
    const store = useExperienceStore()
    await store.load()
    expect(store.error).toBe(true)
    expect(store.loading).toBe(false)
    expect(store.appearance).toBeNull()
    let resolve!: (value: ReturnType<typeof defaultAppearance>) => void
    vi.mocked(siteContent.appearance).mockReturnValueOnce(new Promise(done => { resolve = done }))
    const retry = store.load(true)
    expect(store.error).toBe(false)
    expect(store.loading).toBe(true)
    resolve(defaultAppearance())
    await retry
    expect(store.loading).toBe(false)
    expect(store.error).toBe(false)
  })
})
