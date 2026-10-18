<template>
  <div ref="host" class="xg-ambient" :class="{ 'xg-ambient--immersive': variant === 'immersive' }" aria-hidden="true">
    <div class="xg-ambient__aurora" />
    <canvas v-if="enabled && active" ref="canvas" />
  </div>
</template>
<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useDocumentVisibility, useIntersectionObserver } from '@vueuse/core'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
const props = withDefaults(defineProps<{ phrases?: string[]; active?: boolean; intensity?: number; variant?: 'subtle' | 'immersive'; color?: string }>(), { phrases: () => ['xigong.cloud'], active: true, intensity: 1, variant: 'subtle', color: '#5eead4' })
const emit = defineEmits<{ frame: [canvas: HTMLCanvasElement] }>()
const { enabled } = useExperienceMotion()
const host = ref<HTMLElement>()
const canvas = ref<HTMLCanvasElement>()
const visible = ref(true)
const documentVisibility = useDocumentVisibility()
useIntersectionObserver(host, ([entry]) => { visible.value = entry.isIntersecting })
let alive = true
let epoch = 0
let frame = 0
let last = 0
let offset = 0
let sprites: HTMLCanvasElement[] = []
let observer: ResizeObserver | undefined
function stop() { cancelAnimationFrame(frame); frame = 0 }
function resize() {
  if (!canvas.value || !host.value) return
  canvas.value.width = Math.min(1440, host.value.clientWidth)
  canvas.value.height = Math.min(1000, host.value.clientHeight)
}
function prepare() {
  sprites = props.phrases.filter(Boolean).slice(0, 20).map(text => {
    const sprite = document.createElement('canvas')
    sprite.width = Math.min(900, text.length * 21 + 12); sprite.height = 32
    const ctx = sprite.getContext('2d')
    if (ctx) { ctx.font = `${props.variant === 'immersive' ? 20 : 16}px "QiuYe Pixel", monospace`; ctx.fillStyle = props.color; ctx.fillText(text, 4, 23) }
    return sprite
  })
}
function draw(time: number) {
  const target = canvas.value
  const ctx = target?.getContext('2d')
  if (!target || !ctx) return
  if (time - last > 50) {
    offset += Math.min(time - last, 100) * .012; last = time
    ctx.clearRect(0, 0, target.width, target.height)
    const immersive = props.variant === 'immersive'
    const strength = Math.max(0, Math.min(2, props.intensity))
    ctx.globalAlpha = Math.min(immersive ? .68 : .24, (immersive ? .52 : .18) * strength)
    // 首页在玻璃背后铺开文字轨道；控制台仍仅绘制外围，避免干扰金额和表格。
    const count = immersive ? Math.round(22 + strength * 6) : 8
    for (let i = 0; i < count && sprites.length; i++) {
      const sprite = sprites[i % sprites.length]
      const direction = i % 2 ? -1 : 1
      const width = target.width + sprite.width
      const x = ((i * 251 + offset * direction) % width + width) % width - sprite.width
      const y = immersive ? 100 + (i * 73) % Math.max(80, target.height - 140) : i % 2 === 0 ? 30 + (i % 3) * 34 : target.height - 40 - (i % 3) * 30
      ctx.shadowBlur = 0
      const depth = immersive ? .65 + (i % 5) * .18 : 1
      ctx.globalAlpha = immersive ? Math.min(.72, (.25 + (i % 4) * .12) * strength) : Math.min(.24,.18 * strength)
      ctx.drawImage(sprite, x, y, sprite.width * depth, sprite.height * depth)
      if (immersive && i % 3 === 0) {
        ctx.globalAlpha = .24 * strength
        ctx.fillStyle = props.color
        ctx.fillRect(x + sprite.width * depth + 14, y + 10, 18 + i % 5 * 8, 2)
        ctx.fillRect(x - 12, y + 22, 3, 3)
      }
    }
    if (immersive) emit('frame', target)
  }
  frame = requestAnimationFrame(draw)
}
async function sync() {
  stop()
  const token = ++epoch
  await nextTick()
  if (!alive || token !== epoch) return
  if (enabled.value && props.active && visible.value && documentVisibility.value === 'visible') {
    resize(); prepare(); last = performance.now(); frame = requestAnimationFrame(draw)
  }
}
watch([enabled, visible, documentVisibility, () => props.active, () => props.phrases, () => props.intensity, () => props.color, () => props.variant], sync)
onMounted(() => {
  if (typeof ResizeObserver !== 'undefined') { observer = new ResizeObserver(resize); if (host.value) observer.observe(host.value) }
  void sync()
  // 字形缓存需在自托管字体就绪后重建，避免首次绘制永久使用回退字体。
  void document.fonts?.ready.then(() => { if (alive) prepare() })
})
onBeforeUnmount(() => { alive = false; epoch++; stop(); observer?.disconnect() })
</script>
<style scoped>
.xg-ambient { position: absolute; inset: 0; overflow: hidden; pointer-events: none; z-index: 0; }
.xg-ambient canvas { width: 100%; height: 100%; position: absolute; inset: 0; }
.xg-ambient__aurora { position: absolute; inset: 0; background: radial-gradient(ellipse at 85% 10%, #14b8a60c, transparent 55%); }
.xg-ambient--immersive .xg-ambient__aurora { background: radial-gradient(ellipse at 5% 80%, #a78bfa30, transparent 50%),radial-gradient(ellipse at 95% 10%, #34d39920, transparent 55%); }
:global(.dark .xg-ambient .xg-ambient__aurora) { background: radial-gradient(ellipse at 8% 10%, #a78bfa19, transparent 48%),radial-gradient(ellipse at 90% 75%, #34d39912, transparent 52%); }
</style>
