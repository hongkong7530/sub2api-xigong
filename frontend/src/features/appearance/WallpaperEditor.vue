<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Wallpaper } from '@/api/siteContent'
import AssetInput from './AssetInput.vue'
import { wallpaperSource, wallpaperFocus } from '../home/wallpaperSource'
const wallpaper = defineModel<Wallpaper>({ required: true })
const { t } = useI18n()
const mobile = shallowRef(false)
const url = computed({get: () => (mobile.value ? wallpaper.value.mobile_url : wallpaper.value.desktop_url) || '',set: value => {wallpaper.value = {...wallpaper.value,[mobile.value ? 'mobile_url' : 'desktop_url']:value}}})
const focusX = computed({get:()=>wallpaperFocus(wallpaper.value,mobile.value).x,set:value=>{wallpaper.value={...wallpaper.value,[mobile.value?'mobile_focus_x':'focus_x']:value}}})
const focusY = computed({get:()=>wallpaperFocus(wallpaper.value,mobile.value).y,set:value=>{wallpaper.value={...wallpaper.value,[mobile.value?'mobile_focus_y':'focus_y']:value}}})
</script>
<template>
 <article class="wallpaper-editor">
  <div class="wallpaper-editor__devices"><button type="button" :aria-pressed="!mobile" @click="mobile=false">{{ t('experience.desktop') }}</button><button type="button" :aria-pressed="mobile" @click="mobile=true">{{ t('experience.mobile') }}</button></div>
  <div class="wallpaper-editor__frame" :class="{'wallpaper-editor__frame--mobile':mobile}"><img :src="wallpaperSource(wallpaper,mobile)" :style="{objectPosition:`${focusX}% ${focusY}%`}" :alt="t('experience.wallpaper')" loading="lazy" /></div>
  <AssetInput :key="mobile ? 'mobile' : 'desktop'" v-model="url" />
  <label>{{ t('experience.focusX') }}<input v-model.number="focusX" type="range" min="0" max="100" /></label><label>{{ t('experience.focusY') }}<input v-model.number="focusY" type="range" min="0" max="100" /></label>
  <slot />
 </article>
</template>
<style scoped>
.wallpaper-editor{display:grid;gap:14px;padding:16px;border:1px solid var(--xg-border);border-radius:20px;min-width:0;background:var(--scene-field)}.wallpaper-editor__devices{display:flex;gap:8px}.wallpaper-editor__devices button{min-height:44px;padding:8px 16px;border:1px solid var(--xg-border);border-radius:12px}.wallpaper-editor__devices [aria-pressed=true]{background:#0f766e;color:#fff}.wallpaper-editor__frame{aspect-ratio:16/9;overflow:hidden;border-radius:14px;background:var(--scene-floor)}.wallpaper-editor__frame--mobile{aspect-ratio:9/12;max-height:320px;width:100%}.wallpaper-editor__frame img{width:100%;height:100%;object-fit:cover}.wallpaper-editor label{display:grid;gap:10px;font-size:13px}.wallpaper-editor input[type=range]{width:100%;accent-color:#0f766e}
</style>
