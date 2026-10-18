<script setup lang="ts">
import { watch,computed } from 'vue'
import AssetInput from './AssetInput.vue'
import { consoleImageUrl } from './consoleAppearance'
import { useI18n } from 'vue-i18n'
import type { ConsoleAppearance } from '@/api/siteContent'
const config = defineModel<ConsoleAppearance>({ required: true })
const { t } = useI18n()
const mobileUrl=computed({get:()=>config.value.mobile_background_url||'',set:v=>{config.value.mobile_background_url=v}})
const ranges = [
  { key: 'focus_x', min: 0, max: 100, step: 1, unit: '%' },
  { key: 'focus_y', min: 0, max: 100, step: 1, unit: '%' },
  { key: 'surface_opacity', min: .55, max: .95, step: .01, unit: '' },
  { key: 'blur', min: 0, max: 24, step: 1, unit: 'px' },
  { key: 'background_dim', min: 0, max: .65, step: .01, unit: '' },
] as const
const switches = ['enabled', 'flow_enabled', 'pointer_reveal', 'colorful_headings'] as const
// 已隐藏的无效草稿地址不能阻塞内置图保存；合法地址保留供切回使用。
watch(() => config.value.background_mode, mode => {
  if (mode !== 'custom' && config.value.background_url && !consoleImageUrl(config.value.background_url)) config.value.background_url = ''
})
</script>

<template>
  <section class="console-controls" aria-labelledby="console-controls-title">
    <div class="appearance-section-heading"><h3 id="console-controls-title">{{ t('experience.consoleTitle') }}</h3><span>{{ t('experience.consoleIndependent') }}</span></div>
    <div class="appearance-switches"><label v-for="key in switches" :key="key"><input v-model="config[key]" type="checkbox" /><span>{{ t(`experience.console_${key}`) }}</span></label></div>
    <div class="appearance-fields">
      <label>{{ t('experience.consoleBackground') }}<select v-model="config.background_mode" :aria-label="t('experience.consoleBackground')" class="input"><option value="wallpaper">{{ t('experience.consoleBuiltin') }}</option><option value="custom">{{ t('experience.consoleCustom') }}</option><option value="none">{{ t('experience.consoleNone') }}</option></select></label>
      <label>{{ t('experience.consoleMaterial') }}<select v-model="config.surface_mode" :aria-label="t('experience.consoleMaterial')" class="input"><option value="liquid">{{ t('experience.consoleLiquid') }}</option><option value="frosted">{{ t('experience.consoleFrosted') }}</option></select></label>
    </div>
    <div v-if="config.background_mode === 'custom'" class="appearance-url"><span>{{ t('experience.desktop') }}</span><AssetInput v-model="config.background_url" /><span>{{ t('experience.mobile') }}</span><AssetInput v-model="mobileUrl" /><label>{{ t('experience.mobileFocusX') }}<input v-model.number="config.mobile_focus_x" type="range" min="0" max="100" /></label><label>{{ t('experience.mobileFocusY') }}<input v-model.number="config.mobile_focus_y" type="range" min="0" max="100" /></label></div>
    <fieldset v-if="config.background_mode === 'wallpaper'" class="console-wallpapers"><legend>{{ t('experience.wallpaper') }}</legend><label v-for="n in 6" :key="n" :class="{ selected: config.wallpaper_id === `scene-${n}` }"><input v-model="config.wallpaper_id" type="radio" :value="`scene-${n}`" name="console-wallpaper" /><img :src="`/experience/wallpapers/scene-${n}-thumb.webp`" alt="" loading="lazy" /><span>{{ t('experience.wallpaper') }} {{ n }}</span></label></fieldset>
    <div class="appearance-ranges"><label v-for="range in ranges" :key="range.key"><span>{{ t(`experience.console_${range.key}`) }}<output>{{ config[range.key] }}{{ range.unit }}</output></span><input v-model.number="config[range.key]" type="range" :min="range.min" :max="range.max" :step="range.step" /></label></div>
    <p class="appearance-note">{{ t('experience.consoleMotionHint') }}</p>
  </section>
</template>

<style scoped>
.console-controls { border-top:1px solid var(--xg-border); padding-top:28px; display:grid; gap:22px; }
.console-wallpapers { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:12px; min-width:0; }
.console-wallpapers legend { font-size:13px; margin-bottom:10px; }
.console-wallpapers label { position:relative; overflow:hidden; border:2px solid transparent; border-radius:14px; background:var(--xg-surface); cursor:pointer; }
.console-wallpapers label.selected { border-color:var(--xg-accent); }
.console-wallpapers img { width:100%; height:90px; object-fit:cover; }
.console-wallpapers input { position:absolute; top:8px; left:8px; accent-color:var(--xg-brand); z-index:1; }
.console-wallpapers span { display:block; padding:6px 10px; font-size:11px; }
.appearance-url { display:grid; gap:8px; font-size:13px; }
.appearance-url small { color:var(--xg-muted); }
@media(max-width:480px) { .console-wallpapers { grid-template-columns:repeat(2,minmax(0,1fr)); } }
</style>
