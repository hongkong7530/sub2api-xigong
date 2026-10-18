<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDocumentVisibility, useIntersectionObserver } from '@vueuse/core'
import LiquidGlassSurface from '@/features/home/LiquidGlassSurface.vue'
const props = defineProps<{ active: boolean; liquid: boolean }>()
const host = ref<HTMLElement>()
const visible = ref(true)
const visibility = useDocumentVisibility()
useIntersectionObserver(host, ([entry]) => { visible.value = entry.isIntersecting })
const flowing = computed(() => props.active && visible.value && visibility.value === 'visible')
</script>

<template>
  <div ref="host" class="console-optics" :class="{ 'console-optics--flowing': flowing }" aria-hidden="true">
    <LiquidGlassSurface v-if="liquid" variant="navigation" surface="header" :active="flowing" />
    <div class="console-optics__light" />
    <div class="console-optics__edge" />
  </div>
</template>

<style scoped>
.console-optics { position:absolute; inset:0; pointer-events:none; border-radius:18px; }
.console-optics :deep(.water-glass) { inset:0; }
.console-optics__light { position:absolute; inset:0; overflow:hidden; border-radius:18px; }
.console-optics__light::before { content:''; position:absolute; width:45%; height:200%; top:-50%; left:-45%; background:linear-gradient(110deg,transparent,#c7fff929, #ead7ff29,transparent); transform:translateX(0) skewX(-25deg); }
.console-optics--flowing .console-optics__light::before { animation:console-current 14s ease-in-out infinite; }
.console-optics__edge { position:absolute; inset:0; border-radius:18px; box-shadow:inset 0 1px #ffffffaa,inset 0 -1px #c5dcf345; }
@keyframes console-current { 0%,15% { transform:translateX(0) skewX(-25deg); opacity:0; } 40% { opacity:.85; } 75%,100% { transform:translateX(340%) skewX(-25deg); opacity:0; } }
@media(prefers-reduced-motion:reduce) { .console-optics--flowing .console-optics__light::before { animation:none; } }
@media(prefers-reduced-transparency:reduce) { .console-optics { display:none; } }
</style>
