<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { defaultConsoleAppearance, type Appearance } from '@/api/siteContent'
import ConsoleAppearanceControls from './ConsoleAppearanceControls.vue'
import AppearancePreview from './AppearancePreview.vue'
import WallpaperEditor from './WallpaperEditor.vue'
const appearance = defineModel<Appearance>({ required: true })
defineProps<{ busy: boolean }>()
const emit = defineEmits<{ save: [] }>()
const { t } = useI18n()
const consoleConfig = computed({ get: () => appearance.value.console || defaultConsoleAppearance(), set: value => { appearance.value.console = value } })
const phrases = computed({ get: () => appearance.value.phrases.join('\n'), set: value => { appearance.value.phrases = value.split('\n') } })
const quotes = computed({ get: () => appearance.value.quotes.map(q => `${q.en} | ${q.zh}`).join('\n'), set: value => { appearance.value.quotes = value.split('\n').map(line => { const [en, ...zh] = line.split('|'); return { en: en.trim(), zh: zh.join('|').trim() } }) } })
const switches = ['docs_enabled', 'partners_enabled', 'motion_enabled'] as const
const labels = { docs_enabled: 'experience.docsEnabled', partners_enabled: 'experience.partnersEnabled', motion_enabled: 'experience.motionEnabled' }
function move(i: number, delta: number) { const items = appearance.value.wallpapers; [items[i], items[i + delta]] = [items[i + delta], items[i]] }
</script>

<template>
  <form class="appearance-editor card" :inert="busy" @submit.prevent="emit('save')">
    <div class="appearance-editor__heading"><div><h2>{{ t('experience.appearance') }}</h2><p>{{ t('experience.appearanceIntro') }}</p></div><span class="appearance-editor__accent" aria-hidden="true"><i /><i /><i /></span></div>
    <div class="appearance-editor__layout"><div class="appearance-editor__settings">
      <div class="appearance-fields"><label>{{ t('experience.homeMode') }}<select v-model="appearance.home_mode" :aria-label="t('experience.homeMode')" class="input"><option value="immersive">{{ t('experience.immersive') }}</option><option value="legacy">{{ t('experience.legacy') }}</option></select></label><label>{{ t('experience.intensity') }}<input v-model.number="appearance.intensity" class="input" type="number" min="0" max="2" step=".1" required /></label><label>{{ t('experience.interval') }}<input v-model.number="appearance.interval" class="input" type="number" min="3" max="30" required /></label></div>
      <div class="appearance-switches"><label v-for="key in switches" :key="key"><input v-model="appearance[key]" type="checkbox" /><span>{{ t(labels[key]) }}</span></label></div>
      <ConsoleAppearanceControls v-model="consoleConfig" />
    </div><div class="appearance-editor__preview"><AppearancePreview :appearance="appearance" /></div></div>
    <details class="appearance-editor__home"><summary>{{ t('experience.homeDetails') }}</summary><div class="appearance-fields"><label>{{ t('experience.phrases') }}<textarea v-model="phrases" class="input" rows="5" /></label><label>{{ t('experience.quotes') }}<textarea v-model="quotes" class="input" rows="5" /></label></div><p class="appearance-note">{{ t('experience.orderHint') }}</p><div class="home-wallpapers"><WallpaperEditor v-for="(wallpaper,i) in appearance.wallpapers" :key="wallpaper.id" v-model="appearance.wallpapers[i]"><div class="flex gap-2"><button type="button" class="btn btn-secondary btn-sm" :disabled="i === 0" @click="move(i, -1)">{{ t('experience.moveUp') }}</button><button type="button" class="btn btn-secondary btn-sm" :disabled="i === appearance.wallpapers.length - 1" @click="move(i, 1)">{{ t('experience.moveDown') }}</button></div></WallpaperEditor></div></details>
    <footer class="appearance-editor__footer"><p>{{ t('experience.previewLocal') }}</p><button class="btn btn-primary" :disabled="busy">{{ t(busy ? 'experience.loading' : 'experience.save') }}</button></footer>
  </form>
</template>

<style>
.appearance-editor { padding:28px 30px; display:grid; gap:28px; min-width:0; }
.appearance-editor__heading { display:flex; justify-content:space-between; gap:24px; align-items:center; }
.appearance-editor__heading h2 { font:26px var(--xg-font-display); }
.appearance-editor__heading p { margin-top:8px; font-size:13px; color:var(--xg-muted); line-height:1.8; }
.appearance-editor__accent { display:flex; gap:5px; flex-shrink:0; }
.appearance-editor__accent i { width:6px; height:28px; border-radius:6px; background:#14b8a6; transform:rotate(25deg); }.appearance-editor__accent i:nth-child(2){height:20px;background:#a78bfa}.appearance-editor__accent i:nth-child(3){height:14px;background:#d99bbd}
.appearance-editor__layout { display:grid; grid-template-columns:minmax(0,1fr) minmax(300px,.85fr); gap:32px; align-items:start; }
.appearance-editor__settings { display:grid; gap:24px; min-width:0; }
.appearance-editor__preview { position:sticky; top:88px; min-width:0; }
.appearance-fields { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:18px; }
.appearance-fields label { display:grid; gap:8px; font-size:12px; color:var(--xg-text); align-content:start; }
.appearance-switches { display:flex; flex-wrap:wrap; gap:12px 22px; }.appearance-switches label { display:flex; align-items:center; gap:8px; min-height:32px; font-size:12px; }.appearance-switches input { width:16px; height:16px; accent-color:var(--xg-brand); }
.appearance-ranges { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:20px; }.appearance-ranges label { display:grid; gap:12px; font-size:12px; }.appearance-ranges label > span { display:flex; justify-content:space-between; gap:12px; }.appearance-ranges output { color:var(--xg-brand); font-variant-numeric:tabular-nums; }.appearance-ranges input,.home-wallpapers input { width:100%; accent-color:var(--xg-brand); }
.appearance-section-heading h3 { font:22px var(--xg-font-display); }.appearance-section-heading > span,.appearance-note { font-size:12px; color:var(--xg-muted); line-height:1.8; }.appearance-section-heading > span { display:block; margin-top:8px; }
.appearance-editor__home { border-top:1px solid var(--xg-border); padding-top:20px; }.appearance-editor__home summary { cursor:pointer; min-height:44px; font:18px var(--xg-font-display); }.appearance-editor__home > div { margin:18px 0; }
.home-wallpapers { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:18px; }.home-wallpapers > div { border:1px solid var(--xg-border); border-radius:14px; overflow:hidden; }.home-wallpapers img { width:100%; height:110px; object-fit:cover; }.home-wallpapers > div > div { padding:14px; display:grid; gap:12px; font-size:12px; }
.appearance-editor__footer { display:flex; justify-content:space-between; align-items:center; gap:18px; border-top:1px solid var(--xg-border); padding-top:24px; }.appearance-editor__footer p { color:var(--xg-muted); font-size:12px; }
@media(max-width:1200px) { .appearance-editor__layout { grid-template-columns:1fr; }.appearance-editor__preview { position:static; }.appearance-editor__settings { grid-template-columns:1fr; } }
@media(max-width:640px) { .appearance-editor { padding:22px 18px; }.appearance-fields,.appearance-ranges { grid-template-columns:1fr; }.home-wallpapers { grid-template-columns:1fr; }.appearance-editor__footer { flex-wrap:wrap; }.appearance-editor__footer .btn { width:100%; } }
</style>
