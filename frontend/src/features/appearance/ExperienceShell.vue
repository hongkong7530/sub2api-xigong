<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useDocumentVisibility, useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { Appearance } from '@/api/siteContent'
import { useExperienceStore } from '@/stores/experience'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
import AmbientBackdrop from '@/components/common/AmbientBackdrop.vue'
import ConsoleBackdrop from './ConsoleBackdrop.vue'
import { resolveConsole } from './consoleAppearance'
import { useConsoleReveal } from './useConsoleReveal'
import './console.css'

const props = withDefaults(defineProps<{ appearance?: Appearance | null; variant?: 'console' | 'auth' | 'public'; preview?: boolean; theme?: 'light' | 'dark'; solid?: boolean }>(), { variant: 'console', preview: false, solid: false })
const emit = defineEmits<{ imageError: [failed: boolean] }>()
const experience = useExperienceStore()
const route = useRoute()
const { t } = useI18n()
const { enabled, toggle } = useExperienceMotion()
const host = ref<HTMLElement>()
const actual = computed(() => props.appearance || experience.appearance)
const config = computed(() => resolveConsole(actual.value?.console))
const ready = computed(() => !!actual.value)
const fine = useMediaQuery('(hover: hover) and (pointer: fine)')
const reducedTransparency = useMediaQuery('(prefers-reduced-transparency: reduce)')
const visibility = useDocumentVisibility()
const motion = computed(() => config.value.enabled && !props.preview && !!actual.value?.motion_enabled && enabled.value && visibility.value === 'visible' && !reducedTransparency.value && !props.solid)
const reveal = computed(() => ready.value && motion.value && fine.value && config.value.pointer_reveal && config.value.background_mode !== 'none')
useConsoleReveal(host, reveal, computed(() => route?.fullPath || ''))
const style = computed(() => ({ '--scene-opacity': String(config.value.surface_mode === 'frosted' ? Math.max(.86, config.value.surface_opacity) : config.value.surface_opacity), '--scene-blur': `${config.value.blur}px`, '--scene-dim': String(config.value.background_dim) }))
onMounted(() => { if (!props.preview) void experience.load() })
</script>

<template>
  <div ref="host" class="experience-shell" :class="[`experience-shell--${variant}`, { 'experience-shell--preview': preview, 'experience-shell--enabled': config.enabled, 'experience-shell--color': config.enabled && config.colorful_headings, 'experience-shell--flow': motion && config.flow_enabled, 'experience-shell--solid': solid, 'experience-shell--frosted': config.surface_mode === 'frosted' }]" :style="style" :data-theme="theme" :data-motion="motion ? 'on' : 'off'">
    <div v-if="config.enabled" class="experience-shell__scene">
      <ConsoleBackdrop v-if="ready" :config="config" :preview="preview" @error="emit('imageError', $event)" />
      <AmbientBackdrop :phrases="actual?.phrases" :active="motion" :intensity="actual?.intensity" />
    </div>
    <slot :flow="motion && config.flow_enabled" :liquid="config.enabled && config.surface_mode === 'liquid' && !solid && !reducedTransparency" />
    <button v-if="variant !== 'console' && !preview && actual?.motion_enabled" type="button" class="scene-motion-toggle" :aria-label="t('experience.motion')" :aria-pressed="enabled" @click="toggle"><span aria-hidden="true">{{ enabled ? 'Ⅱ' : '▷' }}</span><span>{{ t('experience.motion') }}</span></button>
  </div>
</template>
