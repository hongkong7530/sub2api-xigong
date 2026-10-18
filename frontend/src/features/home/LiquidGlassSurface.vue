<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { useDocumentVisibility, useIntersectionObserver, useMediaQuery } from '@vueuse/core'

const props = withDefaults(defineProps<{ active: boolean; variant?: 'pool' | 'navigation'; surface?: 'organic' | 'prism' | 'header' }>(), { variant: 'pool', surface: 'organic' })
const prism = computed(() => props.surface === 'prism')
const header = computed(() => props.surface === 'header')
const organicNavigation = computed(() => props.variant === 'navigation' && props.surface === 'organic')
const prismPool = 'M 98 24 H 858 C 933 24 978 65 978 143 V 817 C 978 916 932 976 842 976 H 139 C 60 976 22 930 22 851 V 151 C 22 68 44 24 98 24 Z'
const prismNavigation = 'M 42 44 H 958 C 979 44 992 116 992 232 V 768 C 992 884 979 956 958 956 H 42 C 21 956 8 884 8 768 V 232 C 8 116 21 44 42 44 Z'
const id = `xg-liquid-${useId().replace(/:/g, '')}`
const pool = 'M 130 90 C 180 42 270 85 360 66 C 460 44 530 65 615 50 C 705 32 754 92 826 78 C 872 69 897 22 917 37 C 944 61 878 102 908 161 C 945 222 905 268 922 337 C 944 406 906 478 928 539 C 956 598 916 643 899 692 C 874 748 949 780 919 810 C 894 832 859 778 825 823 C 786 880 715 907 638 893 C 552 876 506 928 431 912 C 360 896 339 948 299 926 C 267 907 296 875 237 872 C 164 869 99 815 105 745 C 111 681 65 655 76 591 C 87 523 56 481 78 414 C 102 346 47 327 63 285 C 75 253 120 265 112 215 C 105 170 80 134 130 90 Z'
const navigation = 'M 40 70 C 165 10 265 95 390 48 C 530 5 650 90 790 40 C 880 12 958 38 976 156 C 992 294 972 366 982 500 C 993 670 976 715 978 826 C 973 967 896 978 790 954 C 650 917 543 998 390 958 C 250 919 155 995 45 942 C 13 926 12 807 20 655 C 27 510 9 433 18 290 C 23 176 20 105 40 70 Z'
const navigationFlow = 'M 40 70 C 165 120 265 5 390 58 C 530 105 650 2 790 60 C 880 92 958 18 976 156 C 990 275 980 389 982 500 C 982 625 990 751 978 826 C 981 951 896 912 790 944 C 650 999 543 912 390 948 C 250 990 155 910 45 942 C 14 930 28 788 20 655 C 10 520 24 434 18 290 C 15 180 20 105 40 70 Z'
const headerContour = computed(() => {
  const rx = Math.min(18, size.value.height / 2) / size.value.width * 1000
  const ry = Math.min(18, size.value.height / 2) / size.value.height * 1000
  return `M ${rx} 0 H ${1000-rx} Q 1000 0 1000 ${ry} V ${1000-ry} Q 1000 1000 ${1000-rx} 1000 H ${rx} Q 0 1000 0 ${1000-ry} V ${ry} Q 0 0 ${rx} 0 Z`
})
const contour = computed(() => header.value ? headerContour.value : prism.value ? (props.variant === 'navigation' ? prismNavigation : prismPool) : (props.variant === 'navigation' ? navigation : pool))
const droplets = [{ cx: 51, cy: 188, rx: 12, ry: 20 }, { cx: 941, cy: 892, rx: 22, ry: 14 }, { cx: 969, cy: 849, rx: 8, ry: 11 }, { cx: 157, cy: 940, rx: 16, ry: 10 }, { cx: 830, cy: 36, rx: 9, ry: 13 }]
const host = ref<HTMLElement>()
const visible = ref(true)
const visibility = useDocumentVisibility()
const solid = useMediaQuery('(prefers-reduced-transparency: reduce)')
const reduced = useMediaQuery('(prefers-reduced-motion: reduce)')
const moving = computed(() => props.active && !reduced.value && !solid.value && visible.value && visibility.value === 'visible')
const shape = computed(() => contour.value)
const maps = ref<{ displacement: string; light: string }[]>([])
const size = ref({ width: 1, height: 1 })
const lensScale = computed(() => header.value ? 32 : prism.value ? (props.variant === 'pool' ? 148 : 92) : (props.variant === 'pool' ? 100 : 58))
const phase = ref(0)
const frameIndex = computed(() => props.variant === 'navigation' ? phase.value * 8 : 0)
const lowMap = computed(() => maps.value[Math.floor(frameIndex.value)])
const highMap = computed(() => maps.value[Math.min(8, Math.floor(frameIndex.value) + 1)] || lowMap.value)
const blend = computed(() => frameIndex.value % 1)
const surfaceStyle = computed(() => ({ '--liquid-clip': `url("#${id}-clip")`, '--liquid-lens': `url("#${id}-lens")` }))
useIntersectionObserver(host, ([entry]) => { visible.value = entry.isIntersecting })
let observer: ResizeObserver | undefined
let frame = 0, resizeFrame = 0
const numbers = (s: string) => s.match(/[0-9.]+/g)!.map(Number)
const start = numbers(navigation), end = numbers(navigationFlow)
function interpolate(t: number) { let i = 0; return navigation.replace(/[0-9.]+/g, () => { const n = i++; return (start[n] + (end[n] - start[n]) * t).toFixed(2) }) }

/** 同一轮廓生成折射和反光；只缓存几何图，不复制壁纸或前景文字。 */
function buildLens() {
  if (!host.value || solid.value) return
  const { width, height } = host.value.getBoundingClientRect()
  if (!width || !height) return
  if (maps.value.length && size.value.width === width && size.value.height === height) return
  size.value = { width, height }
  // A thin header needs enough vertical samples to retain smooth physical corners.
  const ratio = Math.min(1, (header.value ? 1920 : 560) / width)
  const w = Math.max(1, Math.round(width * ratio)), h = Math.max(1, Math.round(height * ratio))
  const tile = document.createElement('canvas'); tile.width = w; tile.height = h
  const ctx = tile.getContext('2d', { willReadFrequently: true })
  if (!ctx) return
  const next: typeof maps.value = []
  // 位移编码按lensScale归一；实际折射强度由曲面宽度和厚度决定。
  const bezel = header.value ? Math.min(12, height / 4) : prism.value ? (props.variant === 'pool' ? 54 : 28) : (props.variant === 'pool' ? 30 : 16)
  const thickness = header.value ? 12 : prism.value ? (props.variant === 'pool' ? 62 : 36) : (props.variant === 'pool' ? 30 : 18)
  for (let step = 0; step < (organicNavigation.value ? 9 : 1); step++) {
    ctx.clearRect(0, 0, w, h); ctx.save(); ctx.scale(w / 1000, h / 1000)
    ctx.fill(new Path2D(organicNavigation.value ? interpolate(step / 8) : contour.value))
    if (props.variant === 'pool' && props.surface === 'organic') for (const drop of droplets) { ctx.beginPath(); ctx.ellipse(drop.cx, drop.cy, drop.rx, drop.ry, 0, 0, Math.PI * 2); ctx.fill() }
    ctx.restore()
    const pixels = ctx.getImageData(0, 0, w, h), light = ctx.createImageData(w, h)
    const distance = new Float32Array(w * h)
    for (let i = 0; i < distance.length; i++) distance[i] = pixels.data[i * 4 + 3] > 128 ? w + h : 0
    // 双向八邻域距离避免斜边法线偏向单一对角线。
    for (let y = 1; y < h - 1; y++) for (let x = 1; x < w - 1; x++) {
      const i = y * w + x
      distance[i] = Math.min(distance[i], distance[i - 1] + 1, distance[i - w] + 1, distance[i - w - 1] + Math.SQRT2, distance[i - w + 1] + Math.SQRT2)
    }
    for (let y = h - 2; y > 0; y--) for (let x = w - 2; x > 0; x--) {
      const i = y * w + x
      distance[i] = Math.min(distance[i], distance[i + 1] + 1, distance[i + w] + 1, distance[i + w + 1] + Math.SQRT2, distance[i + w - 1] + Math.SQRT2)
    }
    const smooth = new Float32Array(distance.length)
    for (let y = 1; y < h - 1; y++) for (let x = 1; x < w - 1; x++) {
      const i = y * w + x
      smooth[i] = (distance[i] * 4 + (distance[i - 1] + distance[i + 1] + distance[i - w] + distance[i + w]) * 2 + distance[i - w - 1] + distance[i - w + 1] + distance[i + w - 1] + distance[i + w + 1]) / 16
    }
    for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
      const p = y * w + x, i = p * 4, coverage = pixels.data[i + 3] / 255
      const d = smooth[p] / ratio
      const dx = smooth[y * w + Math.min(w - 1, x + 1)] - smooth[y * w + Math.max(0, x - 1)]
      const dy = smooth[Math.min(h - 1, y + 1) * w + x] - smooth[Math.max(0, y - 1) * w + x]
      const length = Math.hypot(dx, dy) || 1, nx = dx / length, ny = dy / length
      // 四次超椭圆剖面接到平面时斜率归零；Snell 折射将边缘光线收束到镜内。
      const t = Math.min(1, Math.max(.001, d / bezel)), q = 1 - t
      const rise = Math.pow(1 - q ** 4, .25)
      const slope = thickness / bezel * q ** 3 / Math.pow(1 - q ** 4, .75)
      const incident = Math.atan(slope)
      const refracted = incident - Math.asin(Math.sin(incident) / 1.46)
      const shift = coverage * thickness * rise * Math.tan(refracted)
      pixels.data[i] = 128 + nx * shift / lensScale.value * 255
      pixels.data[i + 1] = 128 + ny * shift / lensScale.value * 255
      pixels.data[i + 2] = 128; pixels.data[i + 3] = 255
      // 反光仅来自曲面法线和掠射角；中心保持全透，不用暗罩或外投影制造厚度。
      const facing = -nx * .6 - ny * .8
      const rim = Math.exp(-(((d - 1.4) / 1.15) ** 2))
      const shoulder = Math.exp(-(((d - bezel * .27) / (bezel * .17)) ** 2))
      const strength = rim * (.3 + .55 * Math.abs(facing)) + shoulder * (.06 + .22 * Math.max(0, facing) ** 5)
      light.data[i] = 212 + 43 * Math.max(0, facing)
      light.data[i + 1] = 238 - 18 * Math.max(0, -facing)
      light.data[i + 2] = 255
      light.data[i + 3] = Math.min(.86, strength) * coverage * 255
    }
    ctx.putImageData(pixels, 0, 0); const displacement = tile.toDataURL()
    ctx.putImageData(light, 0, 0); next.push({ displacement, light: tile.toDataURL() })
  }
  maps.value = next
}
/* 曲面法线静态缓存；持续改变几何会使整块真实后景每帧重新采样。 */
function syncMotion() { cancelAnimationFrame(frame); frame = 0 }
function resize() { cancelAnimationFrame(resizeFrame); resizeFrame = requestAnimationFrame(buildLens) }
watch(moving, syncMotion)
watch(solid, value => { if (!value) buildLens() })
onMounted(() => {
  buildLens(); syncMotion()
  observer = new ResizeObserver(resize); if (host.value) observer.observe(host.value)
})
onBeforeUnmount(() => { cancelAnimationFrame(frame); cancelAnimationFrame(resizeFrame); observer?.disconnect() })
</script>

<template>
  <div ref="host" class="water-glass" :class="[`water-glass--${variant}`, { 'water-glass--moving': moving, 'water-glass--ready': maps.length > 0, 'water-glass--prism': prism, 'water-glass--header': header }]" :style="surfaceStyle" aria-hidden="true">
    <svg class="water-glass__definitions" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <clipPath :id="`${id}-clip`" clipPathUnits="objectBoundingBox">
          <path :d="shape" transform="scale(.001)" />
          <template v-if="variant === 'pool' && surface === 'organic'"><ellipse v-for="(drop, i) in droplets" :key="i" :cx="drop.cx / 1000" :cy="drop.cy / 1000" :rx="drop.rx / 1000" :ry="drop.ry / 1000" /></template>
        </clipPath>
        <filter :id="`${id}-lens`" filterUnits="userSpaceOnUse" primitiveUnits="userSpaceOnUse" x="0" y="0" :width="size.width" :height="size.height" color-interpolation-filters="sRGB">
          <!-- 百分比会按 backdrop 的滤镜区域解析，必须与实际镜片使用相同 CSS 像素坐标。 -->
          <feImage :href="lowMap?.displacement" x="0" y="0" :width="size.width" :height="size.height" preserveAspectRatio="none" result="lens-a" />
          <template v-if="organicNavigation">
            <feImage :href="highMap?.displacement" x="0" y="0" :width="size.width" :height="size.height" preserveAspectRatio="none" result="lens-b" />
            <feComposite in="lens-a" in2="lens-b" operator="arithmetic" k1="0" :k2="1 - blend" :k3="blend" k4="0" result="lens-mix" />
          </template>
          <feComponentTransfer :in="organicNavigation ? 'lens-mix' : 'lens-a'" result="lens">
            <feFuncR type="linear" slope="1" :intercept="-.5 / 255" />
            <feFuncG type="linear" slope="1" :intercept="-.5 / 255" />
          </feComponentTransfer>
          <feDisplacementMap in="SourceGraphic" in2="lens" :scale="lensScale" xChannelSelector="R" yChannelSelector="G" />
        </filter>
      </defs>
    </svg>
    <!-- 独立采样真实后景；不得包进裁切祖先或加深色/阴影以伪造厚度。 -->
    <div class="water-glass__refraction" />
    <div class="water-glass__finish"><div class="water-glass__sheen" /><div class="water-glass__caustic" />
      <img v-if="lowMap" class="water-glass__specular" :src="lowMap.light" alt="" :style="{ opacity: 1 - blend }" />
      <img v-if="organicNavigation && highMap" class="water-glass__specular" :src="highMap.light" alt="" :style="{ opacity: blend }" />
    </div>
    <svg class="water-glass__rim" viewBox="0 0 1000 1000" preserveAspectRatio="none">
      <defs><linearGradient :id="`${id}-edge`" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f0ffff" stop-opacity=".65" /><stop offset=".3" stop-color="#b7f4ee" stop-opacity=".08" /><stop offset=".56" stop-color="#d7ccff" stop-opacity=".45" /><stop offset=".8" stop-color="#e1ffff" stop-opacity=".08" /><stop offset="1" stop-color="#eaffff" stop-opacity=".55" /></linearGradient></defs>
      <g fill="none" :stroke="`url(#${id}-edge)`" stroke-width="1">
        <path :d="shape" vector-effect="non-scaling-stroke" />
        <template v-if="variant === 'pool' && surface === 'organic'"><ellipse v-for="(drop, i) in droplets" :key="i" v-bind="drop" vector-effect="non-scaling-stroke" /></template>
      </g>
    </svg>
  </div>
</template>

<style scoped>
.water-glass { position:absolute; inset:0; pointer-events:none; background:transparent; box-shadow:none; }
.water-glass__definitions { position:absolute; width:0; height:0; }
.water-glass__refraction { position:absolute; inset:0; clip-path:var(--liquid-clip); background:#e5faff03; box-shadow:none; -webkit-backdrop-filter:blur(1.5px) saturate(1.14); backdrop-filter:blur(1.5px) saturate(1.14); }
@supports (backdrop-filter:url("#lens")) { .water-glass--ready .water-glass__refraction { -webkit-backdrop-filter:var(--liquid-lens) saturate(1.08); backdrop-filter:var(--liquid-lens) saturate(1.08); } }
.water-glass__finish { position:absolute; inset:0; overflow:hidden; clip-path:var(--liquid-clip); background:linear-gradient(125deg,#f1ffff0c,transparent 32%,transparent 72%,#c5e3ff06); }
.water-glass__sheen { position:absolute;inset:-60%;background:linear-gradient(112deg,transparent 42%,#eaffff20 48%,#ffffff08 52%,transparent 59%);transform:translateX(-10%) rotate(-12deg);opacity:.55; }
.water-glass__caustic { position:absolute;inset:0;background:radial-gradient(ellipse at 12% 9%,#d6fff32b,transparent 28%),radial-gradient(ellipse at 88% 92%,#a5d8ff26,transparent 24%);opacity:.45; }
.water-glass--moving .water-glass__sheen { animation:liquid-sheen 1.2s ease-out both; }
/* 焦散保持静态，交互景深与轮播仍由壁纸所有者驱动。 */
@keyframes liquid-sheen { to {transform:translateX(17%) rotate(-8deg);opacity:.2} }
@keyframes liquid-caustic { to {transform:translate3d(1%,-1%,0);opacity:.7} }
.water-glass__specular { position:absolute; inset:0; width:100%; height:100%; max-width:none; }
.water-glass--ready .water-glass__rim { opacity:.4; }
.water-glass__rim { position:absolute; inset:0; width:100%; height:100%; overflow:visible; }
.water-glass--navigation { inset:-8px -6px; }
.water-glass--header { inset:0; border-radius:18px; }
.water-glass--header .water-glass__sheen { opacity:.25; }
/* 首页镜片保持真实后景取样；彩色反光独立在前景，避免连续重建法线。 */
.water-glass--prism .water-glass__refraction { background:hsl(var(--xg-h,174) 70% 90% / .045); }
.water-glass--prism .water-glass__finish { background:linear-gradient(128deg,#ffffff1c,transparent 34%,hsl(var(--xg-h,174) 80% 76% / .08) 70%,#ffffff12); }
.water-glass--prism .water-glass__finish::after { content:'';position:absolute;inset:0;border-radius:32px;box-shadow:inset 0 2px 2px #ffffff,inset 3px 0 8px #b7ffffaa,inset -3px -3px 7px #bdc6ffb3;opacity:.9; }
.water-glass--prism .water-glass__finish::before {content:'';position:absolute;inset:0;background:linear-gradient(135deg,#ffffff80,transparent 7%,transparent 85%,#c8f4ff50),linear-gradient(90deg,hsl(var(--xg-h,174) 100% 85% / .38),transparent 6%,transparent 94%,#e0baff40);opacity:.7;}
/* 动态焦散集中在镜片上沿，宽面静态法线保持强折射，避免全幅合成开销。 */
.water-glass--prism .water-glass__caustic {inset:0;width:100%;height:36px;bottom:auto;background:radial-gradient(ellipse at 10% 14%,#ffffff80,transparent 14%),radial-gradient(ellipse at 90% 82%,hsl(var(--xg-h,174) 100% 80% / .6),transparent 22%),conic-gradient(from 50deg at 50% 50%,transparent 32%,#ffffff12 37%,transparent 42%,#a6eaff22 65%,transparent 72%);opacity:.55;}
.water-glass--prism.water-glass--moving .water-glass__caustic {animation:prism-caustic 9s ease-in-out infinite alternate;}
@keyframes prism-caustic {from {transform:translate3d(-1%,1%,0) rotate(-2deg);opacity:.35} to {transform:translate3d(2%,-2%,0) rotate(3deg);opacity:.75}}
.water-glass--prism .water-glass__sheen { inset:-30%;background:linear-gradient(115deg,transparent 39%,#ffffff22 47%,hsl(var(--xg-h,174) 95% 85% / .2) 49%,transparent 56%);opacity:.4; }
.water-glass--prism .water-glass__rim { opacity:.85; }
.water-glass--prism.water-glass--moving .water-glass__sheen { animation:prism-arrival 9s cubic-bezier(.16,1,.3,1) infinite; }
@keyframes prism-arrival {0%,55% {transform:translateX(-65%) rotate(-12deg);opacity:0} 64% {opacity:.65} 85%,100% {transform:translateX(65%) rotate(-12deg);opacity:0}}
@media(prefers-reduced-motion:reduce){.water-glass--prism .water-glass__caustic,.water-glass--prism .water-glass__sheen{animation:none!important;transform:none;}}
/* 只有系统明确要求降低透明度时使用浅色静态面，正常模式始终透光。 */
@media(prefers-reduced-transparency:reduce) { .water-glass .water-glass__refraction { background:#102337; -webkit-backdrop-filter:none; backdrop-filter:none; }.water-glass__finish,.water-glass__rim { display:none; } }
@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))) { .water-glass__refraction { background:#102337; } }
</style>
