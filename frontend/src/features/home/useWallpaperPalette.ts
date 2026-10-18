import { computed, ref, watch, onScopeDispose, type Ref } from 'vue'

/**
 * 从当前壁纸提取主色，供玻璃层、按钮与卡片自适应。
 * 只取有彩度的中间调像素做聚类，避免被大面积天空／高光拉成灰白。
 */
export interface WallpaperPalette {
  hue: number
  saturation: number
  ink: 'light' | 'dark'
  accent: string
  stream: string
}

const FALLBACK: WallpaperPalette = { hue: 174, saturation: 60, ink: 'dark', accent: 'hsl(174, 60%, 38%)', stream: 'hsl(174, 76%, 58%)' }
const CACHE = new Map<string, WallpaperPalette>()

function hsl(hue: number, saturation: number, lightness: number) {
  return `hsl(${Math.round(hue)}, ${Math.round(saturation)}%, ${Math.round(lightness)}%)`
}

function channel(value: number) {
  const c = value / 255
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

function relativeLuminance(r: number, g: number, b: number) {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

function toHsl(r: number, g: number, b: number) {
  const rn = r / 255, gn = g / 255, bn = b / 255
  const max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn)
  const lightness = (max + min) / 2
  if (max === min) return { hue: 0, saturation: 0, lightness }
  const delta = max - min
  const saturation = lightness > 0.5 ? delta / (2 - max - min) : delta / (max + min)
  let hue: number
  if (max === rn) hue = ((gn - bn) / delta + (gn < bn ? 6 : 0)) / 6
  else if (max === gn) hue = ((bn - rn) / delta + 2) / 6
  else hue = ((rn - gn) / delta + 4) / 6
  return { hue: hue * 360, saturation, lightness }
}

export async function analyzeWallpaper(source: string): Promise<WallpaperPalette> {
  const cached = CACHE.get(source)
  if (cached) return cached
  const image = new Image()
  image.src = source
  await image.decode()
  const canvas = document.createElement('canvas')
  const ratio = image.naturalHeight / image.naturalWidth || 0.5625
  canvas.width = 48
  canvas.height = Math.max(16, Math.round(48 * Math.min(ratio, 2)))
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return FALLBACK
  ctx.drawImage(image, 0, 0, canvas.width, canvas.height)
  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const buckets = new Array(24).fill(0)
  const sums: number[][] = Array.from({ length: 24 }, () => [0, 0, 0])
  let luminanceSum = 0, sampled = 0
  for (let i = 0; i < data.length; i += 4) {
    const r = data[i], g = data[i + 1], b = data[i + 2]
    luminanceSum += relativeLuminance(r, g, b)
    sampled++
    const { hue, saturation, lightness } = toHsl(r, g, b)
    // 近中性／过暗／过亮像素不参与主色投票，只用于判断整体明暗。
    if (saturation < 0.18 || lightness < 0.14 || lightness > 0.9) continue
    const slot = Math.floor(hue / 15) % 24
    const weight = saturation * (1 - Math.abs(lightness - 0.5) * 0.9)
    buckets[slot] += weight
    sums[slot][0] += r * weight
    sums[slot][1] += g * weight
    sums[slot][2] += b * weight
  }
  let best = -1, bestWeight = 0
  for (let i = 0; i < buckets.length; i++) {
    if (buckets[i] > bestWeight) { bestWeight = buckets[i]; best = i }
  }
  const averageLuminance = sampled ? luminanceSum / sampled : 0.5
  if (best < 0 || bestWeight === 0) {
    const result: WallpaperPalette = { ...FALLBACK, ink: averageLuminance > 0.55 ? 'dark' : 'light' }
    CACHE.set(source, result)
    return result
  }
  const [r, g, b] = sums[best].map(total => total / bestWeight)
  const { hue, saturation } = toHsl(r, g, b)
  const result: WallpaperPalette = {
    hue,
    // 保留壁纸的彩度倾向但设下限，避免自适应后按钮变得灰扑扑。
    saturation: Math.min(78, Math.max(46, saturation * 100)),
    ink: averageLuminance > 0.55 ? 'dark' : 'light',
    accent: hsl(hue, Math.min(78, Math.max(46, saturation * 100)), 38),
    stream: hsl(hue, Math.min(88, Math.max(58, saturation * 100 + 14)), 62),
  }
  CACHE.set(source, result)
  return result
}

/**
 * 把提取结果展开为 CSS 变量。明暗两套值分开声明，由 .dark 选择器接管，
 * 因此浅色不会自作主张变深，深色也不继承浅色的可读性假设。
 */
export function useWallpaperPalette(source: Ref<string | undefined>) {
  const palette = ref<WallpaperPalette>(FALLBACK)
  let generation = 0
  watch(source, async value => {
    const token = ++generation
    if (!value) { palette.value = FALLBACK; return }
    try {
      const next = await analyzeWallpaper(value)
      if (token === generation) palette.value = next
    } catch { /* 取色失败时沿用上一组颜色，不把界面打回纯白。 */ }
  }, { immediate: true })

  onScopeDispose(() => { generation++ })
  const style = computed(() => {
    const { hue, saturation } = palette.value
    return {
      '--xg-h': String(Math.round(hue)),
      '--xg-accent': hsl(hue, saturation, 80),
      '--xg-brand': hsl(hue, saturation, 84),
      '--xg-stream': hsl(hue, saturation, 82),
      '--xg-cta': hsl(hue, Math.min(80, saturation), 84),
      '--xg-glass-tint': `hsl(${Math.round(hue)} 32% 12% / .66)`,
      '--xg-glow-a': hsl(hue, 78, 62),
      '--xg-glow-b': hsl((hue + 118) % 360, 72, 60),
    } as Record<string, string>
  })

  return { palette, style }
}
