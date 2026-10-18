<template>
  <div
    :class="[
      'xg-pixel-skeleton bg-gray-200 dark:bg-dark-700',
      variant === 'circle' ? 'rounded-full' : 'rounded-lg',
      customClass
    ]"
    :style="style"
  ></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'rect' | 'circle' | 'text'
  width?: string | number
  height?: string | number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'rect',
  width: '100%'
})

const customClass = computed(() => props.class || '')

const style = computed(() => {
  const s: Record<string, string> = {}

  if (props.width) {
    s.width = typeof props.width === 'number' ? `${props.width}px` : props.width
  }

  if (props.height) {
    s.height = typeof props.height === 'number' ? `${props.height}px` : props.height
  } else if (props.variant === 'text') {
    s.height = '1em'
    s.marginTop = '0.25em'
    s.marginBottom = '0.25em'
  }

  return s
})
</script>

<style scoped>
.xg-pixel-skeleton{background-image:conic-gradient(from 90deg,#14b8a630 25%,transparent 0 50%,#14b8a618 0 75%,transparent 0);background-size:12px 12px;animation:xg-pixel-wait 1.4s ease-in-out infinite alternate}
@keyframes xg-pixel-wait{from{opacity:.45;filter:blur(1px)}to{opacity:.85;filter:blur(0)}}
@media(prefers-reduced-motion:reduce){.xg-pixel-skeleton{animation:none}}
:global(html[data-xg-motion="off"]) .xg-pixel-skeleton{animation:none}
</style>
