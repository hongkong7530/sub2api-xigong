<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Appearance } from '@/api/siteContent'
import ExperienceShell from './ExperienceShell.vue'
import ConsoleGlassSurface from './ConsoleGlassSurface.vue'
const props = defineProps<{ appearance: Appearance }>()
const { t } = useI18n()
const dark = shallowRef(false), solid = shallowRef(false), imageError = shallowRef(false)
const showImageError = computed(() => imageError.value && props.appearance.console?.enabled && props.appearance.console.background_mode !== 'none')
</script>

<template>
  <section class="appearance-preview" data-scene-protect>
    <div class="appearance-preview__toolbar"><h3>{{ t('experience.preview') }}</h3><div><button type="button" class="btn btn-secondary btn-sm" :aria-pressed="dark" @click="dark = !dark">{{ t(dark ? 'experience.previewDark' : 'experience.previewLight') }}</button><button type="button" class="btn btn-secondary btn-sm" :aria-pressed="solid" @click="solid = !solid">{{ t('experience.previewSolid') }}</button></div></div>
    <p class="appearance-note">{{ t('experience.previewLocal') }}</p>
    <ExperienceShell :appearance="appearance" preview :theme="dark ? 'dark' : 'light'" :solid="solid" @image-error="imageError = $event" v-slot="{ flow, liquid }">
      <header class="scene-header preview-header"><ConsoleGlassSurface :active="flow" :liquid="liquid" /><span class="scene-title">{{ t('experience.contentAdmin') }}</span><span class="preview-dot">{{ t('experience.previewSample') }}</span></header>
      <div class="preview-body"><h2 class="scene-title">{{ t('experience.appearance') }}</h2><section class="scene-surface preview-form"><h3 class="scene-section-title">{{ t('experience.consoleTitle') }}</h3><label>{{ t('experience.homeMode') }}<select class="input" disabled><option>{{ t('experience.immersive') }}</option></select></label><div class="preview-table"><table><thead><tr><th>{{ t('experience.name') }}</th><th>{{ t('experience.previewState') }}</th></tr></thead><tbody><tr><td>{{ t('experience.consoleLiquid') }}</td><td>{{ t(appearance.console?.enabled ? 'experience.enabled' : 'experience.previewDisabled') }}</td></tr><tr><td>{{ t('experience.console_pointer_reveal') }}</td><td>{{ t(appearance.console?.pointer_reveal ? 'experience.enabled' : 'experience.previewDisabled') }}</td></tr></tbody></table></div><button type="button" class="btn btn-primary" disabled>{{ t('experience.save') }}</button></section><p v-if="showImageError" class="preview-image-error" role="status">{{ t('experience.consoleImageError') }}</p></div>
    </ExperienceShell>
  </section>
</template>

<style scoped>
.appearance-preview { min-width:0; }
.appearance-preview__toolbar { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; }
.appearance-preview__toolbar h3 { font:20px var(--xg-font-display); }
.appearance-preview__toolbar > div { display:flex; flex-wrap:wrap; gap:8px; }
.appearance-preview > p { margin:12px 0; }
.appearance-preview :deep(.experience-shell) { border:1px solid var(--xg-border); color:var(--scene-ink); }
.preview-header { position:relative; min-height:64px; display:flex; align-items:center; justify-content:space-between; gap:12px; padding:14px 22px; border-radius:24px 24px 0 0; }
.preview-header .scene-title { font:20px var(--xg-font-display); }
.preview-dot { font-size:10px; color:var(--scene-muted); }
.preview-body { padding:28px 24px 44px; }
.preview-body > h2 { font:28px var(--xg-font-display); margin-bottom:22px; }
.preview-form { padding:24px; display:grid; gap:18px; }
.preview-form h3 { font:20px var(--xg-font-display); }
.preview-form label { display:grid; gap:8px; font-size:12px; }
.preview-form .btn { justify-self:start; }
.preview-table { overflow:auto; }
.preview-table table { width:100%; border-collapse:collapse; font-size:12px; }
.preview-table th,.preview-table td { padding:10px 0; text-align:left; border-bottom:1px solid var(--scene-line); color:var(--scene-ink); }
.preview-table th { font-weight:500; color:var(--scene-muted); }
.preview-image-error { padding:12px; margin-top:18px; border-radius:12px; background:var(--scene-field); font-size:12px; }
@media(max-width:480px) { .preview-body { padding:22px 14px 32px; }.preview-form { padding:18px; }.preview-header { padding:12px 18px; } }
</style>
