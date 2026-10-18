<script setup lang="ts">
import { computed, nextTick, ref, shallowRef, watch, onBeforeUnmount, onMounted } from 'vue'
import { useMediaQuery, useDocumentVisibility, useIntersectionObserver } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
import type { Wallpaper } from '@/api/siteContent'
import {wallpaperSource,wallpaperFocus} from './wallpaperSource'

type Scene = { wallpaper: Wallpaper; url: string }
type Navigation = { id: string; direction: number; automatic: boolean; epoch: number }
const props = defineProps<{ images: Wallpaper[]; interval: number; active: boolean; interactionPaused?: boolean; controlsTarget?: HTMLElement }>()
const emit = defineEmits<{ change: [id: string]; scene: [value: Scene] }>()
const { t } = useI18n()
const { enabled } = useExperienceMotion()
const fine = useMediaQuery('(hover: hover) and (pointer: fine)')
const mobile = useMediaQuery('(max-width: 700px)')
const visibility = useDocumentVisibility()
const stage = ref<HTMLElement>(), space = ref<HTMLElement>(), scene = ref<HTMLElement>(), oldScene = ref<HTMLElement>()
const onscreen = ref(true)
useIntersectionObserver(stage, ([e]) => { onscreen.value = e.isIntersecting })
const fallback: Wallpaper = { id: 'scene-1', focus_x: 70, focus_y: 45 }
const images = computed(() => props.images.length ? props.images : [fallback])
const source = (w: Wallpaper) => wallpaperSource(w,mobile.value)
const snapshot = (w: Wallpaper): Scene => ({ wallpaper: { ...w }, url: source(w) })
// 显示层持有已经准备好的资源，断点和配置变化不会直接改写旧图的 URL。
const displayed = shallowRef<Scene>(snapshot(images.value[0]))
const outgoing = shallowRef<Scene>()
const preview = shallowRef<Scene>()
const index = computed(() => Math.max(0, images.value.findIndex(w => w.id === displayed.value.wallpaper.id)))
const paused = ref(false), focused = ref(false), hovering = ref(false), transitioning = ref(false), cssBlend = ref(false)
const playbackEpoch = ref(0)
const point = ref({ x: 0, y: 0 })
let target = { x: 0, y: 0 }, frame = 0, timer: ReturnType<typeof setTimeout> | undefined
let cleanupTimer: ReturnType<typeof setTimeout> | undefined
let alive = true, resourceEpoch = 0, intent = 0, preloadEpoch = 0
let requestedId: string | undefined, queued: Navigation | undefined
let animations: Animation[] = []
const motion = computed(() => enabled.value && props.active && visibility.value === 'visible' && onscreen.value)
// 指针预览不阻止轮播；显式暂停及键盘阅读控制自动播放。
const canAutoplay = computed(() => motion.value && !paused.value && !focused.value && !props.interactionPaused)
const intervalMs = computed(() => Math.max(3, Math.min(30, props.interval)) * 1000)
const imageStyle = (w: Wallpaper) => {const f=wallpaperFocus(w,mobile.value);return {objectPosition:`${f.x}% ${f.y}%`}}
const portrait = (w: Wallpaper) => !w.desktop_url && !w.mobile_url && ['scene-4','scene-6'].includes(w.id)
const revealStyle = computed(() => ({ opacity: hovering.value && motion.value && !transitioning.value ? '1' : '0', maskImage: 'radial-gradient(circle 160px at ' + point.value.x + 'px ' + point.value.y + 'px, black 25%, transparent 100%)', WebkitMaskImage: 'radial-gradient(circle 160px at ' + point.value.x + 'px ' + point.value.y + 'px, black 25%, transparent 100%)' }))
const decoded = new Map<string, Promise<void>>()
function decode(url: string) {
 let task = decoded.get(url)
 if (!task) {
  const image = new Image(); image.src = url
  task = image.decode().catch(error => { decoded.delete(url); throw error })
  decoded.set(url, task)
 }
 return task
}
async function preload() {
 const token = ++preloadEpoch, resources = resourceEpoch
 const next = snapshot(images.value[(index.value + 1) % images.value.length])
 preview.value = undefined
 try {
  await decode(next.url)
  if (alive && resources === resourceEpoch && token === preloadEpoch) preview.value = next
 } catch { /* 坏图不进入指针预览，当前画面保持可见。 */ }
}
function finishTransition(runQueue = true) {
 clearTimeout(cleanupTimer); cleanupTimer = undefined
 for (const animation of animations) animation.cancel()
 animations = []; outgoing.value = undefined; transitioning.value = false; cssBlend.value = false
 const pending = queued; queued = undefined
 if (alive && pending && runQueue) {
  void navigate(pending)
 } else if (alive && runQueue) {
  requestedId = undefined; schedule()
 }
}
async function commit(next: Scene, direction: number) {
 const previous = displayed.value
 displayed.value = next; leave()
 if (!motion.value || previous.url === next.url) { requestedId = undefined; schedule(); return }
 outgoing.value = previous; transitioning.value = true
 await nextTick()
 if (!alive || !motion.value || !transitioning.value) { finishTransition(); return }
 const incoming = scene.value, departing = oldScene.value
 if (incoming?.animate) {
  const options: KeyframeAnimationOptions = { duration: 1000, easing: 'cubic-bezier(.22,.65,.22,1)', fill: 'both' }
  // 旧图保持不透明，覆盖率始终为1；只有上方新图渐入，不透出舞台底色。
  animations.push(incoming.animate([{ opacity: 0, transform: 'translate3d(' + direction * 32 + 'px,0,0) scale(1.12)' }, { opacity: 1, transform: 'translate3d(0,0,0) scale(1.035)' }], options))
  if (departing) animations.push(departing.animate([{ transform: 'scale(1.035)' }, { transform: 'translate3d(' + direction * -24 + 'px,0,0) scale(1.09)' }], options))
  const depth = incoming.querySelector('.wallpaper-depth')
  if (depth) animations.push(depth.animate([{ opacity: .8 }, { opacity: 0 }], options))
 } else {
  cssBlend.value = true
 }
 cleanupTimer = setTimeout(() => finishTransition(), 1020)
}
async function navigate(request: Navigation) {
 const token = ++intent, resources = resourceEpoch
 const list = images.value.map(snapshot)
 const start = list.findIndex(s => s.wallpaper.id === request.id)
 if (start < 0) return
 const valid = () => alive && token === intent && resources === resourceEpoch && (!request.automatic || (request.epoch === playbackEpoch.value && canAutoplay.value))
 for (let n = 0; n < list.length; n++) {
  if (!valid()) { if (token === intent) requestedId = undefined; return }
  const candidate = list[(start + request.direction * n + list.length * n) % list.length]
  try {
   await decode(candidate.url)
   if (!valid()) { if (token === intent) requestedId = undefined; return }
   if (candidate.url !== displayed.value.url || JSON.stringify(candidate.wallpaper) !== JSON.stringify(displayed.value.wallpaper)) await commit(candidate, request.direction)
   else { requestedId = undefined; void preload(); schedule() }
   return
  } catch {
   // 卸载、资源变化和新的操作都会使失败重试失效，不再创建后续 Image。
   if (!valid()) { if (token === intent) requestedId = undefined; return }
  }
 }
 if (valid()) { requestedId = undefined; schedule() }
}
function advance(direction: number, automatic = false) {
 if (images.value.length < 2) return
 const start = images.value.findIndex(w => w.id === (requestedId || displayed.value.wallpaper.id))
 const next = images.value[(Math.max(0, start) + direction + images.value.length) % images.value.length]
 const request: Navigation = { id: next.id, direction, automatic, epoch: playbackEpoch.value }
 requestedId = next.id; clearTimeout(timer)
 // 连点记录最新目标；先完成正在呈现的混合，不把半幅旧图瞬间丢弃。
 if (transitioning.value) { queued = request; return }
 void navigate(request)
}
function track() {
 if (!alive || !motion.value || !hovering.value) { leave(); return }
 point.value = { x: point.value.x + (target.x - point.value.x) * .14, y: point.value.y + (target.y - point.value.y) * .14 }
 if (space.value && stage.value) {
  const width = stage.value.clientWidth || innerWidth, height = stage.value.clientHeight || innerHeight
  space.value.style.transform = 'translate3d(' + (point.value.x / width - .5) * -38 + 'px,' + (point.value.y / height - .5) * -26 + 'px,0)'
 }
 frame = requestAnimationFrame(track)
}
function move(e: PointerEvent) {
 if (!fine.value || !motion.value || (e.target as HTMLElement).closest('a,button,input,textarea,summary,.wallpaper-controls,.hero-content,.home-details,.home-navigation,.partners-page')) { leave(); return }
 const rect = stage.value!.getBoundingClientRect(); target = { x: e.clientX - rect.left, y: e.clientY - rect.top }
 if (!hovering.value) { point.value = target; hovering.value = true; frame = requestAnimationFrame(track) }
}
function leave() { hovering.value = false; cancelAnimationFrame(frame); if (space.value) space.value.style.transform = '' }
function schedule() {
 clearTimeout(timer); playbackEpoch.value++
 if (canAutoplay.value && !transitioning.value) timer = setTimeout(() => advance(1, true), intervalMs.value)
}
async function refreshResources() {
 resourceEpoch++; intent++; preloadEpoch++; preview.value = undefined; requestedId = undefined; queued = undefined; clearTimeout(timer)
 const request = { id: images.value.some(w => w.id === displayed.value.wallpaper.id) ? displayed.value.wallpaper.id : images.value[0].id, direction: 1, automatic: false, epoch: playbackEpoch.value }
 // 已有转场先结束，期间仍显示固定 URL 的旧画面。
 if (transitioning.value) { queued = request; return }
 await navigate(request)
}
watch(displayed, value => { emit('change', value.wallpaper.id); emit('scene', value); void preload() })
watch([mobile, () => props.images], () => void refreshResources(), { deep: true })
watch([canAutoplay, intervalMs], schedule, { flush: 'sync' })
watch(motion, value => {
 if (!value) { leave(); finishTransition(false); requestedId = undefined }
}, { flush: 'sync' })
defineExpose({ move, leave })
onMounted(() => { emit('change', displayed.value.wallpaper.id); emit('scene', displayed.value); void preload(); schedule() })
onBeforeUnmount(() => { alive = false; resourceEpoch++; preloadEpoch++; intent++; queued = undefined; leave(); clearTimeout(timer); finishTransition(false); decoded.clear() })
</script>

<template>
 <div ref="stage" class="wallpaper-stage" :class="{ 'wallpaper-stage--motion': motion, 'wallpaper-stage--transitioning': transitioning }" @pointermove="move" @pointerleave="leave">
  <div ref="space" class="wallpaper-space">
   <div v-if="outgoing" ref="oldScene" class="wallpaper-plane wallpaper-outgoing">
    <img class="wallpaper-extension" :src="outgoing.url" alt="" />
    <img class="wallpaper-image wallpaper-old" :class="{ 'wallpaper-portrait': portrait(outgoing.wallpaper) }" :src="outgoing.url" :style="imageStyle(outgoing.wallpaper)" alt="" />
   </div>
   <div :key="displayed.url" ref="scene" class="wallpaper-plane wallpaper-current" :class="{ 'wallpaper-current--css': cssBlend }">
    <img class="wallpaper-extension" :src="displayed.url" alt="" />
    <img class="wallpaper-image" :class="{ 'wallpaper-portrait': portrait(displayed.wallpaper) }" :src="displayed.url" :style="imageStyle(displayed.wallpaper)" alt="" fetchpriority="high" @error="advance(1)" />
    <img v-if="transitioning" class="wallpaper-image wallpaper-depth" :class="{ 'wallpaper-portrait': portrait(displayed.wallpaper) }" :src="displayed.url" :style="imageStyle(displayed.wallpaper)" alt="" />
   </div>
   <img v-if="preview" :src="preview.url" :style="[imageStyle(preview.wallpaper), revealStyle]" class="wallpaper-image wallpaper-peek" :class="{ 'wallpaper-portrait': portrait(preview.wallpaper) }" alt="" />
  </div>
  <div class="wallpaper-shade" />
  <div class="wallpaper-atmosphere" /><div class="wallpaper-depth-haze" />
  <Teleport :to="controlsTarget || 'body'" :disabled="!controlsTarget">
   <div class="wallpaper-controls" @pointerenter="leave" @focusin="focused = ($event.target as HTMLElement).matches(':focus-visible')" @focusout="focused = false">
    <button type="button" :aria-label="t('experience.previous')" @click="advance(-1)">‹</button>
    <button type="button" :aria-label="paused ? t('experience.play') : t('experience.pause')" :aria-pressed="paused" @click="paused = !paused">{{ paused ? '▷' : 'Ⅱ' }}</button>
    <span aria-live="polite">{{ String(index + 1).padStart(2, '0') }} / {{ String(images.length).padStart(2, '0') }}</span>
    <button type="button" :aria-label="t('experience.next')" @click="advance(1)">›</button>
    <span :key="displayed.url + ':' + playbackEpoch" class="wallpaper-progress" :class="{ 'wallpaper-progress--playing': canAutoplay && !transitioning }" :style="{ '--scene-time': intervalMs + 'ms' }"><i /></span>
   </div>
  </Teleport>
 </div>
</template>

<style scoped>
.wallpaper-stage { position:absolute; inset:0; overflow:hidden; pointer-events:none; background:var(--xg-background); }
.wallpaper-space { position:absolute; inset:-26px; transition:transform .65s cubic-bezier(.22,.65,.22,1); }
.wallpaper-plane { position:absolute; inset:0; overflow:hidden; transform:scale(1.035); }
.wallpaper-current { z-index:1; }
.wallpaper-current--css { animation:wallpaper-fade 1s cubic-bezier(.22,.65,.22,1) both; }
@keyframes wallpaper-fade { from { opacity:0; } to { opacity:1; } }
.wallpaper-outgoing { z-index:0; }
.wallpaper-extension { position:absolute; inset:-5%; width:110%; height:110%; object-fit:cover; filter:blur(28px) saturate(.9); pointer-events:none; }
.wallpaper-image { position:absolute; width:100%; height:100%; object-fit:cover; inset:0; }
.wallpaper-portrait { object-fit:contain; object-position:78% center !important; }
.wallpaper-depth { filter:blur(10px); opacity:0; pointer-events:none; }
.wallpaper-stage--transitioning .wallpaper-plane { will-change:transform,opacity; }
.wallpaper-peek { z-index:2; pointer-events:none; transform:scale(1.035); transition:opacity .25s; }
.wallpaper-shade { position:absolute; inset:0; pointer-events:none; background:linear-gradient(90deg,#07152512,transparent 68%),linear-gradient(0deg,#07152550,transparent 40%); }
.wallpaper-depth-haze { position:absolute;inset:0;pointer-events:none;background:radial-gradient(ellipse at 50% 45%,transparent 30%,#06182f38 100%);mix-blend-mode:multiply; }
.wallpaper-atmosphere { position:absolute; inset:-5%; pointer-events:none; opacity:.38; background:radial-gradient(ellipse at 76% 78%,#caf5ff45,transparent 44%),radial-gradient(ellipse at 15% 20%,#8ecaff26,transparent 42%); }
/* 静止背景不持续重绘整屏折射；景深由实际指针及换图触发。 */
@keyframes scene-breathe { to { transform:translate3d(1.5%,-1%,0) scale(1.03); opacity:.14; } }
:global(.dark .wallpaper-stage .wallpaper-shade) { background:linear-gradient(90deg,#0206172b,#02061705),linear-gradient(0deg,#02061780,transparent 50%); }
.wallpaper-controls { pointer-events:auto; position:absolute; right:5%; bottom:35px; display:flex; align-items:center; gap:8px; color:#f4fcff; border:1px solid #eaffff78; border-radius:20px; background:linear-gradient(130deg,#deffff28,#deffff09),#08223855; backdrop-filter:blur(8px) saturate(1.35); -webkit-backdrop-filter:blur(8px) saturate(1.35); box-shadow:inset 0 2px 1px #ffffff6b,inset 0 -2px 2px #00142670,0 8px 25px #00142638; padding:4px 8px; font:12px ui-monospace,monospace; overflow:hidden; }
.wallpaper-controls button { min-width:44px; height:44px; border-radius:13px; font-size:20px; }
.wallpaper-controls button:hover { background:#ffffff25; }
.wallpaper-controls button:focus-visible { outline:2px solid #d7ffff; outline-offset:-3px; }
.wallpaper-progress { position:absolute; inset:auto 12px 0; height:2px; pointer-events:none; }
.wallpaper-progress i { display:block; height:100%; background:#e5ffffba; transform:scaleX(0); transform-origin:left; }
.wallpaper-progress--playing i { animation:scene-time var(--scene-time) linear both; }
@keyframes scene-time { to { transform:scaleX(1); } }
@media(max-width:700px) {
 .wallpaper-space { inset:0; }
 .wallpaper-plane { transform:none; }
 .wallpaper-depth,.wallpaper-extension { display:none; }
 .wallpaper-atmosphere { opacity:.12; }
 .wallpaper-portrait { object-fit:cover; object-position:center 25% !important; }
 .wallpaper-controls { bottom:18px; right:18px; }
}
@media(prefers-reduced-motion:reduce) { .wallpaper-space,.wallpaper-peek { transition:none; }.wallpaper-atmosphere { animation:none !important; } }
:global(html[data-xg-motion='off'] .wallpaper-space),:global(html[data-xg-motion='off'] .wallpaper-peek) { transition:none; }
@media(prefers-reduced-transparency:reduce) { .wallpaper-controls { background:#102337; backdrop-filter:none; } }
@supports not ((mask-image:radial-gradient(black,transparent)) or (-webkit-mask-image:radial-gradient(black,transparent))) { .wallpaper-peek { display:none; } }
</style>
