<template>
  <div class="xg-motion-region" :class="{ 'pixel-transition--running': running && enabled && active }">
    <div class="pixel-transition__content"><slot /></div>
    <div v-if="running && enabled && active" class="pixel-transition__veil" aria-hidden="true"><i v-for="n in 36" :key="n" :style="{ '--delay': `${n % 6 * 16}ms` }" /></div>
  </div>
</template>
<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
const props = withDefaults(defineProps<{ identity?: string; active?: boolean }>(), { active: true })
const { enabled } = useExperienceMotion()
const running = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
function play() { clearTimeout(timer); running.value = enabled.value && props.active; if (enabled.value && props.active) timer = setTimeout(() => { running.value = false }, 360) }
watch(() => props.identity, play)
watch(() => enabled.value && props.active, value => { if (!value) { clearTimeout(timer); running.value = false } })
onMounted(play)
onBeforeUnmount(() => clearTimeout(timer))
</script>
<style scoped>
.pixel-transition__veil { position: absolute; inset: 0; pointer-events: none; display: grid; grid-template-columns: repeat(6,1fr); grid-template-rows: repeat(6,1fr); z-index: 1; overflow: hidden; }
.pixel-transition__veil i { background: var(--xg-background); animation: resolve-tile .24s var(--delay) ease-out both; }
.pixel-transition--running > .pixel-transition__content { animation: resolve-content .28s ease-out;  }
@keyframes resolve-tile { from { opacity: .5; transform: scale(1.02); } to { opacity: 0; transform: scale(.8); } }
@keyframes resolve-content { from { transform: translateY(5px); opacity: .7; } to { transform: none; opacity: 1; } }
@media(prefers-reduced-motion:reduce) { .pixel-transition__veil { display: none; }.pixel-transition--running > .pixel-transition__content { animation: none; } }
</style>
