<template>
  <div class="pixel-loading" role="status" :aria-label="label || t('common.loading')" :class="{ 'pixel-loading--still': !enabled }">
    <div class="pixel-loading__field" aria-hidden="true">
      <i v-for="n in 28" :key="n" :style="{ '--i': n, '--delay': `${(n % 7) * 45}ms` }" />
    </div>
    <span>{{ label || t('common.loading') }}</span>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
defineProps<{ label?: string }>()
const { t } = useI18n()
const { enabled } = useExperienceMotion()
</script>
<style scoped>
.pixel-loading { display: grid; place-content: center; gap: 18px; min-height: 200px; text-align: center; color: var(--xg-muted); font-size: 13px; }
.pixel-loading__field { width: 142px; display: grid; grid-template-columns: repeat(7, 1fr); gap: 5px; margin: auto; }
.pixel-loading i { display: block; width: 16px; height: 16px; background: var(--xg-accent); opacity: .3; border-radius: 2px; animation: pixel-resolve 1.4s var(--delay) ease-in-out infinite alternate; }
@keyframes pixel-resolve { from { transform: scale(1.22); filter: blur(2px); opacity: .12; } to { transform: scale(.72); filter: blur(0); opacity: .65; } }
.pixel-loading--still i { animation: none; }
@media (prefers-reduced-motion: reduce) { .pixel-loading i { animation: none; } }
</style>
