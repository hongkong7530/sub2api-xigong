import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { analyzeWallpaper, useWallpaperPalette } from '../useWallpaperPalette'

let pixels: Uint8ClampedArray
const image = { decode: vi.fn(), draw: vi.fn() }
beforeEach(() => {
  pixels = new Uint8ClampedArray([40, 170, 190, 255, 248, 248, 248, 255])
  image.decode.mockResolvedValue(undefined)
  vi.stubGlobal('Image', class {
    src = ''
    naturalWidth = 100
    naturalHeight = 60
    decode() { return image.decode(this.src) }
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    drawImage: image.draw,
    getImageData: () => ({ data: pixels }),
  } as unknown as CanvasRenderingContext2D)
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('wallpaper palette', () => {
  it('picks chromatic midtones rather than a white sky and caches the sampled result', async () => {
    const result = await analyzeWallpaper('/palette-cyan.webp')
    expect(result.hue).toBeGreaterThan(180)
    expect(result.hue).toBeLessThan(200)
    expect(result.saturation).toBeGreaterThanOrEqual(46)
    const calls = image.decode.mock.calls.length
    expect(await analyzeWallpaper('/palette-cyan.webp')).toEqual(result)
    expect(image.decode).toHaveBeenCalledTimes(calls)
  })
  it('handles a neutral image without producing NaN tokens', async () => {
    pixels = new Uint8ClampedArray([240, 240, 240, 255])
    const result = await analyzeWallpaper('/palette-neutral.webp')
    expect(Number.isFinite(result.hue)).toBe(true)
    expect(Number.isFinite(result.saturation)).toBe(true)
  })
  it('does not apply an old decode after the user changes wallpapers', async () => {
    let finish: (() => void) | undefined
    image.decode.mockImplementation((src: string) => src.includes('slow') ? new Promise<void>(resolve => { finish = resolve }) : Promise.resolve())
    const scope = effectScope()
    const source = ref<string | undefined>('/palette-slow.webp')
    const colors = scope.run(() => useWallpaperPalette(source))!
    source.value = '/palette-fast.webp'
    await nextTick(); await vi.waitFor(() => expect(colors.style.value['--xg-h']).not.toBe('174'))
    const current = colors.style.value['--xg-h']
    pixels = new Uint8ClampedArray([190, 80, 40, 255])
    finish?.(); await Promise.resolve(); await Promise.resolve()
    expect(colors.style.value['--xg-h']).toBe(current)
    scope.stop()
  })
})
