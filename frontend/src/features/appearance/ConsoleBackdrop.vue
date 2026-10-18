<script setup lang="ts">
import { computed, shallowRef, watch, onScopeDispose } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import type { ConsoleAppearance } from '@/api/siteContent'
import { consoleSource } from './consoleAppearance'

const props = defineProps<{ config: ConsoleAppearance; preview?: boolean }>()
const emit = defineEmits<{ error: [failed: boolean] }>()
const mobile = useMediaQuery('(max-width: 700px)')
const source = computed(() => consoleSource(props.config, mobile.value))
const displayed = shallowRef('')
let generation = 0
watch(source, async value => {
  const token = ++generation
  displayed.value = ''
  emit('error', false)
  if (!value) return
  const image = new Image()
  image.referrerPolicy = 'no-referrer'
  image.src = value
  try {
    await image.decode()
    if (token === generation) displayed.value = value
  } catch { if (token === generation) emit('error', true) }
}, { immediate: true })
onScopeDispose(() => { generation++ })
const position = computed(() => ({ objectPosition: `${mobile.value ? props.config.mobile_focus_x ?? props.config.focus_x : props.config.focus_x}% ${mobile.value ? props.config.mobile_focus_y ?? props.config.focus_y : props.config.focus_y}%` }))
</script>

<template>
  <div class="console-backdrop" aria-hidden="true">
    <img v-if="displayed" :src="displayed" :style="position" alt="" referrerpolicy="no-referrer" class="console-backdrop__image" />
    <div class="console-backdrop__wash" />
    <div class="console-backdrop__shade" />
    <div class="console-backdrop__grain" />
  </div>
</template>

<style scoped>
.console-backdrop { position:absolute; inset:0; overflow:hidden; pointer-events:none; background:var(--scene-floor); }
.console-backdrop__image { width:100%; height:100%; object-fit:cover; }
.console-backdrop__wash { position:absolute; inset:0; background:linear-gradient(130deg,var(--scene-wash),transparent 65%),linear-gradient(0deg,var(--scene-floor-soft),transparent 65%); }
.console-backdrop__shade { position:absolute; inset:0; background:var(--scene-shade); opacity:var(--scene-dim); }
.console-backdrop__grain { position:absolute; inset:0; opacity:.1; background-image:url("data:image/svg+xml,%3Csvg viewBox='0 0 180 180' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.8' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Cpath fill='%23fff' filter='url(%23n)' opacity='.22' d='M0 0h180v180H0z'/%3E%3C/svg%3E"); }
:global(.experience-shell[data-revealing='true'] .console-backdrop__shade) { mask-image:radial-gradient(circle 180px at var(--reveal-x) var(--reveal-y),transparent 12%,#000 100%); -webkit-mask-image:radial-gradient(circle 180px at var(--reveal-x) var(--reveal-y),transparent 12%,#000 100%); }
</style>
