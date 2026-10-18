<template>
  <div class="immersive-home" :data-panel="panel" :style="paletteStyle" @pointermove="wallpaper?.move($event)" @pointerleave="wallpaper?.leave()">
    <div ref="controlsLayer" class="home-controls-layer" />
    <div class="home-scene">
      <WallpaperStage ref="wallpaper" :controls-target="controlsLayer" :images="appearance.wallpapers" :interval="appearance.interval" :active="appearance.motion_enabled" :interaction-paused="heroFocused || panel !== 'home'" @change="wallpaperId = $event" @scene="displayedWallpaper = $event.url" />
      <AmbientBackdrop variant="immersive" :color="paletteStyle['--xg-stream']" :phrases="appearance.phrases" :intensity="appearance.intensity" :active="appearance.motion_enabled" />
    </div>
    <header class="home-navigation xg-navigation">
      <LiquidGlassSurface variant="navigation" surface="prism" :active="enabled && appearance.motion_enabled" />
      <RouterLink to="/home" class="home-brand"><span class="home-mark"><img :src="siteLogo || '/logo.svg'" alt="" width="40" height="40" /></span><span>{{ app.siteName }}<small>xigong.cloud</small></span></RouterLink>
      <nav :aria-label="t('experience.home')"><button :aria-current="panel === 'home' ? 'page' : undefined" @click="selectPanel('home')">{{ t('experience.home') }}</button><button v-if="appearance.partners_enabled" :aria-current="panel === 'partners' ? 'page' : undefined" @click="selectPanel('partners')">{{ t('experience.partners') }}</button><RouterLink v-if="appearance.docs_enabled" to="/docs">{{ t('experience.docs') }}</RouterLink><RouterLink v-if="showPlaza" to="/model-plaza">{{ t('nav.modelPlaza') }}</RouterLink></nav>
      <span v-if="responseNode" class="home-response-node" :title="t('clusterV3.responseNodeHelp')"><i aria-hidden="true" />{{ t('clusterV3.responseNode') }}: <b>{{ responseNode }}</b></span>
      <div class="home-tools"><button :title="t('experience.motion')" :aria-label="t('experience.motion')" :aria-pressed="enabled" @click="toggle">▦</button><button :aria-label="t('experience.theme')" @click="toggleTheme">{{ dark ? '☼' : '☾' }}</button><RouterLink class="home-login" :to="auth.isAuthenticated ? dashboard : '/login'">{{ auth.isAuthenticated ? t('home.dashboard') : t('home.login') }}</RouterLink></div>
    </header>
    <main ref="main" tabindex="-1" @touchstart.passive="touchStart" @touchend.passive="touchEnd">
      <div v-if="panel === 'home'" class="xg-reveal">
        <section class="home-hero" @focusin="heroFocused = true" @focusout="heroFocused = false">
          <div class="hero-content">
            <LiquidGlassSurface surface="prism" :active="enabled && appearance.motion_enabled" />
            <span class="hero-kicker">{{ t('experience.kicker') }}</span><ParticleBrand :text="app.siteName" :active="enabled && appearance.motion_enabled" /><h2>{{ t('experience.tagline') }}</h2><p>{{ t('experience.intro') }}</p>
            <div class="hero-actions"><RouterLink class="home-cta" :to="auth.isAuthenticated ? dashboard : '/login'">{{ t('experience.start') }}</RouterLink><RouterLink v-if="appearance.docs_enabled" class="home-cta home-cta-secondary" to="/docs">{{ t('experience.docs') }}</RouterLink></div>
            <div v-if="quote" class="hero-quote"><small>{{ t('experience.quoteToday') }}</small><span v-if="quote.en">{{ quote.en }}</span><span>{{ quote.zh }}</span><small v-if="remoteQuote?.source">{{ remoteQuote.source }}</small><button type="button" :aria-label="t('experience.quoteRefresh')" @click="chooseQuote">{{ t('experience.quoteRefresh') }} ↻</button></div>
          </div>
          <a class="hero-explore" href="#site-details">↓ <span>{{ t('experience.explore') }}</span></a>
        </section>
        <section id="site-details" class="home-details"><HomeSiteStats :active="enabled && appearance.motion_enabled" /><HomeChannelMonitors :active="enabled && appearance.motion_enabled" />
          <div class="detail-intro"><h2>{{ t('experience.capability') }}</h2><p>{{ t('experience.capabilityCopy') }}</p></div>
          <div class="capability-grid"><RouterLink to="/keys" class="capability-main xg-glass"><svg class="feature-symbol" viewBox="0 0 96 72" aria-hidden="true"><path d="M7 38C17 7 49 8 56 28S39 63 32 42S69 12 89 35M7 49C27 25 57 68 89 43" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="55" cy="30" r="6" fill="none" stroke="currentColor" /></svg><h3>{{ t('experience.keysTitle') }}</h3><p>{{ t('experience.keysCopy') }}</p><span class="feature-link">{{ t('keys.title') }} ↗</span></RouterLink><div class="capability-secondary"><RouterLink to="/usage" class="xg-glass"><h3>{{ t('experience.usageTitle') }}</h3><p>{{ t('experience.usageCopy') }}</p></RouterLink><RouterLink v-if="showPlaza" to="/model-plaza" class="xg-glass"><h3>{{ t('experience.modelsTitle') }}</h3><p>{{ t('experience.modelsCopy') }}</p></RouterLink></div></div>
          <section class="home-steps xg-glass"><h2>{{ t('experience.stepsTitle') }}</h2><ol><li v-for="n in 3" :key="n"><span>{{ n }}</span>{{ t(`experience.step${n}`) }}</li></ol></section>
          <section class="home-faq xg-glass"><h2>{{ t('experience.faq') }}</h2><details v-for="n in 2" :key="n"><summary>{{ t(`experience.faq${n}`) }}</summary><p>{{ t(`experience.answer${n}`) }}</p></details></section>
        </section>
      </div>
      <section v-else class="partners-page xg-reveal"><div class="partners-heading"><span class="hero-kicker">{{ app.siteName }}</span><h1>{{ t('experience.partners') }}</h1><p>{{ t('experience.partnersIntro') }}</p></div><PixelLoading v-if="loadingPartners" /><div v-else-if="partnerError" class="xg-status" role="alert">{{ t('experience.loadError') }} <button class="btn btn-secondary" @click="loadPartners">{{ t('experience.retry') }}</button></div><p v-else-if="!partners.length" class="xg-status">{{ t('experience.noPartners') }}</p><div v-else class="partners-grid"><a v-for="partner in partners" :key="partner.id" class="partner-card xg-glass" :href="safeLink(partner.url)" target="_blank" rel="noopener noreferrer"><div v-if="partner.thumbnail" class="partner-cover"><img :src="safeLink(partner.thumbnail)" alt="" loading="lazy" referrerpolicy="no-referrer" /></div><img v-if="partner.icon && !failedPartnerIcons.has(partner.icon)" :src="safeLink(partner.icon)" alt="" loading="lazy" @error="failedPartnerIcons.add(partner.icon)" /><div v-else class="partner-initial">{{ partner.name.slice(0, 1) }}</div><span v-if="partner.tag" class="partner-tag">{{ partner.tag }}</span><h2>{{ partner.name }}</h2><p>{{ partner.description }}</p><span class="partner-address">{{ domain(partner.url) }} ↗</span></a></div><p class="partner-disclaimer">{{ t('experience.partnerDisclaimer') }}</p></section>
    </main>
    <footer class="home-footer"><RouterLink to="/home">{{ app.siteName }}</RouterLink><span>© {{ new Date().getFullYear() }} xigong.cloud</span><RouterLink v-if="appearance.docs_enabled" to="/docs">{{ t('experience.docs') }}</RouterLink></footer>
  </div>
</template>
<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useResponseNode } from '@/composables/useResponseNode'
const { responseNode } = useResponseNode()
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import { siteContent, type Appearance, type Partner } from '@/api/siteContent'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
import { sanitizeUrl } from '@/utils/url'
import WallpaperStage from './WallpaperStage.vue'
import LiquidGlassSurface from './LiquidGlassSurface.vue'
import { useWallpaperPalette } from './useWallpaperPalette'
import AmbientBackdrop from '@/components/common/AmbientBackdrop.vue'
import PixelLoading from '@/components/common/PixelLoading.vue'
import HomeSiteStats from './HomeSiteStats.vue'
import HomeChannelMonitors from './HomeChannelMonitors.vue'
import ParticleBrand from './ParticleBrand.vue'
import {wallpaperSource} from './wallpaperSource'
const props = defineProps<{ appearance: Appearance }>()
const app = useAppStore(), auth = useAuthStore(), route = useRoute(), router = useRouter()
const { t } = useI18n(), { enabled, toggle } = useExperienceMotion()
const siteLogo = computed(() => sanitizeUrl(app.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const heroFocused = ref(false)
const wallpaper = ref<InstanceType<typeof WallpaperStage>>()
const controlsLayer = ref<HTMLElement>()
const wallpaperId = ref(props.appearance.wallpapers[0]?.id || 'scene-1')
const displayedWallpaper = ref('')
const paletteSource = computed(() => {if (displayedWallpaper.value) return displayedWallpaper.value; const w=props.appearance.wallpapers.find(w=>w.id===wallpaperId.value);return w?.desktop_url ? wallpaperSource(w) : `/experience/wallpapers/${wallpaperId.value}-thumb.webp`})
const { style: paletteStyle } = useWallpaperPalette(paletteSource)
const dark = ref(document.documentElement.classList.contains('dark'))
function toggleTheme() { dark.value = !dark.value; document.documentElement.classList.toggle('dark', dark.value); localStorage.setItem('theme', dark.value ? 'dark' : 'light') }
const dashboard = computed(() => auth.isAdmin ? '/admin/dashboard' : '/dashboard')
const showPlaza = computed(() => app.cachedPublicSettings?.model_plaza_enabled !== false && (!app.cachedPublicSettings?.model_plaza_require_auth || auth.isAuthenticated))
const panel = computed(() => route.query.panel === 'partners' && props.appearance.partners_enabled ? 'partners' : 'home')
const main = ref<HTMLElement>()
const positions: Record<string, number> = { home: 0, partners: 0 }
const remoteQuote = ref<{en:string;zh:string;source?:string}|null>(null)
const localQuote = ref<{en:string;zh:string}>()
const quotePool = computed(() => [...props.appearance.quotes, ...[1,2,3].map(n => ({en:"",zh:t(`experience.quoteFallback${n}`)}))])
let quoteController:AbortController|undefined
async function fetchQuote(previous:string){quoteController?.abort();const controller=new AbortController();quoteController=controller;try{const q=await siteContent.quote(controller.signal);if(!controller.signal.aborted&&q&&q.zh!==previous){remoteQuote.value=q;sessionStorage.setItem("xg-quote-text",q.zh)}}catch{/* 外部服务不可用时保留本次已选择的不同短句。 */}}
function chooseQuote(){
 const previous=sessionStorage.getItem("xg-quote-text")||""
 remoteQuote.value=null
 const choices=quotePool.value.filter(q=>(q.zh||q.en)!==previous)
 localQuote.value=choices[Math.floor(Math.random()*choices.length)]
 if(localQuote.value)sessionStorage.setItem("xg-quote-text",localQuote.value.zh||localQuote.value.en)
 void fetchQuote(previous)
}
watch(()=>route.fullPath,()=>{if(panel.value==="home")chooseQuote()},{immediate:true})
const quote = computed(() => remoteQuote.value || localQuote.value)
const partners = ref<Partner[]>([]), loadingPartners = ref(false), partnerError = ref(false)
const failedPartnerIcons = ref(new Set<string>())
let alive = true
async function loadPartners() { loadingPartners.value = true; partnerError.value = false; try { const value = await siteContent.partners(); if (alive) partners.value = value } catch { if (alive) partnerError.value = true } finally { if (alive) loadingPartners.value = false } }
watch(panel, async (value, old) => { if (old) positions[old] = window.scrollY; if (value === 'partners') void loadPartners(); await nextTick(); requestAnimationFrame(() => { if (!alive) return; window.scrollTo({ top: positions[value] || 0, behavior: 'instant' }); main.value?.focus({ preventScroll: true }) }) }, { immediate: true })
function selectPanel(value: string) { if (value !== panel.value) void router.push({ path: '/home', query: value === 'partners' ? { panel: value } : {} }) }
let gesture: {x:number;y:number} | null = null
function touchStart(e: TouchEvent) { if (e.touches.length !== 1 || (e.target as HTMLElement).closest('a,button,input,textarea,pre,table,summary') || e.touches[0].clientX < 30 || e.touches[0].clientX > innerWidth - 30) { gesture = null; return } gesture = { x: e.touches[0].clientX, y: e.touches[0].clientY } }
function touchEnd(e: TouchEvent) { if (!gesture) return; const dx = e.changedTouches[0].clientX - gesture.x, dy = e.changedTouches[0].clientY - gesture.y; gesture = null; if (Math.abs(dx) > 90 && Math.abs(dx) > Math.abs(dy) * 2) { if (dx > 0 && props.appearance.partners_enabled) selectPanel('partners'); if (dx < 0) selectPanel('home') } }
const safeLink = (raw: string) => sanitizeUrl(raw, { allowRelative: true })
function domain(raw: string) { try { return new URL(raw).hostname } catch { return '' } }
onBeforeUnmount(() => { alive = false;quoteController?.abort() })
</script>
<style scoped>
.immersive-home { --xg-text:#f4faff; --xg-muted:#dbe8ef; --xg-border:#d5f5ff40; --xg-background:#101c2b; --xg-surface:#142638; --xg-radius:28px; --home-ink:#081c2e; position:relative; isolation:isolate; color:var(--xg-text); background:var(--xg-background); min-height:100svh; overflow-x:clip; }
.home-scene { position:fixed; inset:0; z-index:-1; }
/* 前景控件不进入光学裁切层，保留完整点击区和焦点环。 */
.home-controls-layer { position:fixed; inset:0; z-index:10; pointer-events:none; }
.home-scene :deep(.xg-ambient) { z-index:1; }
.immersive-home[data-panel='partners'] .home-controls-layer { display:none; }
.immersive-home main { position:relative; pointer-events:none; background:linear-gradient(180deg,transparent 65vh,#07111c35 110vh,#07111c88); }
.home-details,.partners-page { pointer-events:auto; }
.immersive-home .xg-reveal { animation:none; }
/* 禁止外投影与祖先滤镜；厚度由镜片法线、内缘反光和色散表达。 */
.immersive-home .xg-glass { position:relative; background:linear-gradient(125deg,#e7ffff14,transparent 55%),#0a2035db; border:1px solid var(--xg-border); border-radius:var(--xg-radius); box-shadow:inset 0 1px #ffffff60,inset 0 -1px #bcefff25; backdrop-filter:blur(3px) saturate(1.3); -webkit-backdrop-filter:blur(3px) saturate(1.3); }
.immersive-home .home-navigation { position:fixed; z-index:20; top:max(20px,env(safe-area-inset-top)); left:max(24px,calc((100% - 1360px)/2)); right:max(24px,calc((100% - 1360px)/2)); min-height:76px; display:flex; align-items:center; gap:26px; padding:12px 24px; border:0; background:none; box-shadow:none; }
.home-navigation::before { content:""; position:absolute; inset:5px 10px; border-radius:24px; background:linear-gradient(115deg,hsl(var(--xg-h,174) 38% 13% / .76),#10263aa8 55%,hsl(var(--xg-h,174) 40% 17% / .82)); pointer-events:none; }
.home-navigation::after { content:'';position:absolute;inset:auto 38px 4px;height:1px;background:linear-gradient(90deg,transparent,var(--xg-glow-a,#8de8df),#ffffffbb,var(--xg-glow-b,#e0c8ff),transparent);opacity:.75;pointer-events:none; }
.home-navigation > :not(.water-glass) { position:relative; z-index:1; }
.home-navigation { --xg-text:#f5fcff; --xg-muted:#e4f2f7; --xg-accent:#adffed; }
.home-brand { display:flex; gap:11px; align-items:center; min-width:154px; font-size:20px; color:var(--xg-text); }
.home-brand small { display:block; font:11px var(--xg-font-display); color:var(--xg-muted); margin-top:3px; }
.home-mark { display:grid; place-items:center; flex-shrink:0; width:40px; height:40px; border-radius:13px; overflow:hidden; background:#f4ffff14; border:1px solid #ffffff60; box-shadow:inset 0 1px #ffffff80; }
.home-mark img { display:block; width:100%; height:100%; object-fit:contain; filter:none; }
.home-navigation nav { display:flex; align-items:center; gap:6px; flex:1; min-width:0; }
.home-navigation nav a,.home-navigation nav button { display:flex; align-items:center; padding:10px 12px; white-space:nowrap; border-radius:14px; min-height:44px; color:var(--xg-muted); transition:background-color .18s,color .18s; }
.home-navigation nav :is(a,button):hover { color:var(--xg-text); background:#ffffff16; }
.home-navigation nav [aria-current="page"] { background:linear-gradient(145deg,#ffffff2b,hsl(var(--xg-h,174) 80% 70% / .13));color:#f4ffff;box-shadow:inset 0 1px #ffffff90,inset 0 -1px var(--xg-accent); }
.home-tools { display:flex; align-items:center; gap:6px; order:3; }
.home-brand > span:last-child { min-width:0; overflow-wrap:anywhere; }
.home-navigation nav { order:1; }
.home-response-node { order:2; display:inline-flex; align-items:center; gap:6px; flex-shrink:0; font-size:11px; line-height:1.4; color:var(--xg-muted); white-space:nowrap; }
.home-response-node i { width:5px; height:5px; border-radius:50%; background:#b6ffe1; box-shadow:0 0 8px #8affbf70; }
.home-response-node b { font-weight:500; font-variant-numeric:tabular-nums; }
.home-tools button { width:44px; height:44px; border-radius:14px; color:var(--xg-muted); }
.home-tools button:hover { background:#ffffff16; color:var(--xg-text); }
.home-tools button[aria-pressed="false"] { opacity:.7; }
.home-login,.home-cta { display:inline-flex; align-items:center; justify-content:center; gap:8px; min-height:48px; padding:12px 22px; border:1px solid #ffffff90; border-radius:19px 15px 21px 14px; color:var(--home-ink); background:linear-gradient(145deg,#ffffff90,transparent 65%),var(--xg-cta,#b7f4e9); box-shadow:inset 0 2px 1px #ffffffaa,inset 0 -2px 2px #18394f25; font-size:14px; font-weight:600; white-space:nowrap; transition:background-color .18s,transform .18s; }
.home-login { min-height:44px; padding:10px 18px; }
.home-login,.home-cta { position:relative; isolation:isolate; overflow:hidden; border-radius:18px; background:linear-gradient(135deg,#e8fff4e8,#9ceadbd9 50%,#c3e7ffeb); color:#062d37; border-color:#efffffc9; box-shadow:inset 0 1px 1px #ffffff,inset 0 -2px 6px #388f9c40; }
.home-login::before,.home-cta::before { content:""; position:absolute; inset:-80% -45%; background:linear-gradient(100deg,transparent 35%,#ffffff7a 49%,transparent 63%); transform:translateX(-65%) rotate(12deg); transition:transform .8s cubic-bezier(.22,1,.36,1); pointer-events:none; z-index:-1; }
.home-login:hover,.home-cta:hover { transform:translateY(-2px); background-color:#e9ffff; }
.home-login:hover::before,.home-cta:hover::before,.home-login:focus-visible::before,.home-cta:focus-visible::before { transform:translateX(65%) rotate(12deg); }
.home-login:active,.home-cta:active { transform:translateY(1px) scale(.98); }
.home-cta-secondary { color:#f3ffff; background:linear-gradient(135deg,#26495cd9,#102b41eb); border-color:#e2ffff85; box-shadow:inset 0 1px #ffffff70,inset 0 -1px #82dae638; }
.home-cta-secondary:hover { background:#23415bee; }
.immersive-home :is(a,button,summary):focus-visible { outline:2px solid var(--xg-accent,#b7f4e9); outline-offset:4px; }
.immersive-home :is(a,button,summary) { touch-action:manipulation; }
.immersive-home :is(a,button):active { transform:translateY(1px); }
.immersive-home button:disabled { opacity:.5; cursor:not-allowed; }
.home-hero { min-height:810px; min-height:max(810px,100svh); position:relative; display:flex; align-items:center; padding:170px max(6vw,calc((100% - 1280px)/2)) 145px; pointer-events:none; }
.hero-content,.hero-explore { pointer-events:auto; }
.hero-content { position:relative; width:min(610px,55%); padding:48px 50px 48px 46px; }
.hero-content::after {content:'';position:absolute;inset:22px 10px;pointer-events:none;border-radius:32px;background:linear-gradient(132deg,#ffffff0c,transparent 32%,hsl(var(--xg-h,174) 75% 70% / .06));}
.hero-content > :not(.water-glass) { position:relative; z-index:1; }
.hero-content::before { content:"";position:absolute;inset:12px 4px;border-radius:32px;background:linear-gradient(125deg,#0c2a3e9c,hsl(var(--xg-h,174) 35% 12% / .5) 60%,#183a4e66);pointer-events:none; }
.hero-content :is(h2,p,.hero-kicker,.hero-quote) {text-shadow:0 1px 3px #071c2b,0 0 12px #071c2b80;}
.hero-content :deep(.water-glass--pool) { inset:-8px -16px -8px -16px; }
.hero-depth-title { font-size:clamp(52px,6.2vw,88px); line-height:1.16; margin:28px -12px 26px; letter-spacing:-.045em; overflow-wrap:anywhere; transform:perspective(800px) rotateY(-18deg) rotateX(7deg) rotateZ(-5deg); transform-origin:left center; color:#f4ffff; text-shadow:1px 1px #a7d9e2,2px 2px #81b5c8,3px 3px #59889f,4px 4px #355b76; }
/* 平行的轮廓字向场景后方消隐，正文仅保留一份可访问标题。 */
.hero-depth-title::before { content:attr(data-title); position:absolute; inset:0; transform:translate3d(13px,-17px,-24px); color:transparent; -webkit-text-stroke:1px #deffff55; text-shadow:none; opacity:.65; mask-image:linear-gradient(110deg,#000,transparent 85%); pointer-events:none; }
.hero-content h2 { font-size:clamp(23px,2.3vw,31px); line-height:1.55; letter-spacing:.035em; text-wrap:balance; }
.hero-content p { color:var(--xg-muted); font-size:15px; line-height:1.95; margin:20px 0 28px; max-width:37ch; text-wrap:pretty; }
.hero-kicker { font:13px var(--xg-font-display); color:var(--xg-brand,#ccfff1); display:flex; align-items:center; gap:12px; letter-spacing:.08em; }
.hero-kicker::before { content:""; width:25px; height:10px; border:1px solid currentColor; border-radius:70% 30% 65% 35%; transform:rotate(-18deg); }
.hero-actions { display:flex; flex-wrap:wrap; gap:12px; }
.hero-quote { display:grid; gap:6px; font-size:12px; line-height:1.6; color:var(--xg-muted); margin-top:30px; padding:0 0 0 16px; border-left:1px solid var(--xg-border); }
.hero-quote button {justify-self:start;min-height:44px;padding:8px 0;color:#c4fff0;font-size:12px}.hero-quote small {font-size:11px;letter-spacing:.08em;color:#e1f0f6}.hero-quote span:last-of-type { color:var(--xg-brand); font-family:var(--xg-font-display); letter-spacing:.04em; }
.hero-explore { position:absolute; bottom:52px; left:max(6vw,calc((100% - 1280px)/2)); font:13px var(--xg-font-display); display:flex; align-items:center; gap:12px; z-index:3; padding:12px 18px; min-height:44px; border:1px solid #ffffff48; border-radius:30px 24px 28px 20px; color:var(--xg-text); background:#091827dd; }
.home-details { max-width:1220px; margin:auto; padding:80px 28px 85px; scroll-margin-top:110px; }
.detail-intro { display:grid; grid-template-columns:1.1fr 1fr; gap:70px; align-items:center; margin-bottom:32px; padding:32px; background:linear-gradient(110deg,#071525ee,#071525b3); border-radius:42px 16px 42px 16px; border:1px solid #dbffff26; }
.detail-intro h2,.home-steps h2,.home-faq h2 { font-size:clamp(26px,3vw,38px); line-height:1.5; text-wrap:balance; }
.detail-intro p { color:var(--xg-muted); line-height:1.9; text-wrap:pretty; }
.capability-grid { display:grid; grid-template-columns:1.1fr 1fr; gap:24px; }
.capability-main { padding:38px; min-width:0; --xg-radius:52px 22px 64px 24px; overflow:hidden; }
/* 涟漪刻线延续主水镜，不给卡片网格增加昂贵的动态 SVG 滤镜。 */
.capability-main::after { content:""; position:absolute; width:240px; height:160px; right:-95px; bottom:-65px; border:1px solid #ceffff40; border-radius:50%; box-shadow:0 0 0 22px #c7ffff08,0 0 0 44px #c7ffff06; transform:rotate(-25deg); pointer-events:none; }
.feature-symbol { width:96px; height:72px; color:var(--xg-brand); }
.capability-grid h3 { font-family:var(--xg-font-display); font-size:25px; margin:18px 0 12px; font-weight:400; letter-spacing:.03em; }
.capability-grid p { color:var(--xg-muted); line-height:1.85; max-width:42ch; }
.feature-link { display:block; margin-top:36px; color:var(--xg-brand); }
.capability-secondary { display:grid; gap:20px; }
.capability-secondary a { padding:28px 32px; min-width:0; --xg-radius:24px 42px 26px 38px; }
.capability-secondary a:last-child { --xg-radius:38px 24px 44px 24px; }
.capability-secondary h3 { margin-top:0; }
.capability-grid a,.partner-card { transition:transform .24s ease,border-color .2s; }
@media(hover:hover) and (prefers-reduced-motion:no-preference) { .capability-grid a:hover,.partner-card:hover { transform:translateY(-4px); border-color:var(--xg-accent); } }
.home-steps { padding:38px; margin-top:28px; --xg-radius:24px 48px 24px 48px; }
.home-steps ol { display:grid; grid-template-columns:repeat(3,1fr); gap:28px; margin-top:32px; }
.home-steps li { display:flex; gap:16px; align-items:center; font-size:15px; line-height:1.7; }
.home-steps li span { display:grid; place-items:center; border:1px solid #e0ffff70; background:linear-gradient(145deg,#e0ffff2b,#e0ffff05); border-radius:55% 45% 64% 36%; width:48px; height:48px; color:var(--xg-brand); flex-shrink:0; font:20px var(--xg-font-display); }
.home-faq { margin-top:28px; padding:38px; --xg-radius:48px 24px 48px 24px; }
.home-faq details { border-bottom:1px solid var(--xg-border); padding:18px 0; }
.home-faq details:last-child { border-bottom:0; padding-bottom:0; }
.home-faq summary { cursor:pointer; font-weight:500; min-height:44px; padding:10px 0; }
.home-faq details[open] summary { color:var(--xg-brand); }
.home-faq p { color:var(--xg-muted); line-height:1.85; padding:8px 0 10px; }
.partners-page { min-height:calc(100svh - 100px); position:relative; padding:180px max(6vw,calc((100% - 1220px)/2)) 80px; }
.partners-heading { margin-bottom:40px; position:relative; padding:32px; border-radius:38px 18px 38px 18px; background:linear-gradient(100deg,#071525ed,#071525aa); }
.partners-heading h1 { font-size:clamp(36px,4vw,56px); margin:16px 0; }
.partners-heading p,.partner-disclaimer { color:var(--xg-muted); line-height:1.8; }
.partners-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:24px; position:relative; }
.partner-card { position:relative; padding:30px; --xg-radius:36px 20px 42px 20px; }
.partner-card img,.partner-initial { width:56px; height:56px; object-fit:contain; border-radius:14px; }
.partner-initial { display:grid; place-items:center; background:#ffffff13; color:var(--xg-brand); font:24px var(--xg-font-display); }
.partner-cover {margin:-30px -30px 24px;height:180px;overflow:hidden;border-radius:35px 19px 12px 12px;background:#163a50}.partner-card .partner-cover img {width:100%;height:100%;object-fit:cover;border-radius:0;transition:transform .6s}.partner-card:hover .partner-cover img {transform:scale(1.04)}
.partner-card h2 { font-size:23px; margin:24px 0 12px; }
.partner-card p { color:var(--xg-muted); line-height:1.8; overflow-wrap:anywhere; }
.partner-tag { position:absolute; top:30px; right:24px; font-size:12px; color:var(--xg-brand); }
.partner-address { display:block; margin-top:24px; font-size:12px; overflow-wrap:anywhere; }
.partner-disclaimer { font-size:12px; margin-top:36px; padding:16px 20px; border-radius:16px; background:#071525dd; }
.partners-page .xg-status { position:relative; background:#102132f5; }
.home-footer { border-top:1px solid var(--xg-border); padding:30px 6%; display:flex; gap:24px; align-items:center; font-size:13px; color:var(--xg-muted); background:#071525ee; }
.home-footer span { flex:1; }
:global(.dark .immersive-home) { --xg-background:#020617; }
:global(.dark .immersive-home .xg-glass) { background:linear-gradient(135deg,#ffffff09,transparent 60%),#071525e8; }
@media(max-width:1100px) { .immersive-home .home-navigation { gap:10px; padding:12px 18px; }.home-brand { min-width:118px; }.home-navigation nav a,.home-navigation nav button { padding:10px 8px; font-size:13px; }.hero-content { width:58%; padding:40px 30px; }.partners-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }.detail-intro { gap:30px; } }
@media(max-width:700px) { .home-controls-layer { position:absolute; height:310px; bottom:auto; }.immersive-home .home-navigation { left:12px; right:12px; top:max(12px,env(safe-area-inset-top)); flex-wrap:wrap; gap:6px; padding:14px 12px; }.home-brand { flex:1; font-size:18px; min-width:0; }.home-mark { width:36px; height:36px; }.home-tools { margin-left:auto; gap:2px; }.home-tools button { width:44px; }.home-login { padding:10px 12px; }.home-navigation nav { order:3; flex:0 0 100%; width:100%; overflow-x:auto; }.home-navigation nav a,.home-navigation nav button { flex-shrink:0; padding:8px 12px; }.home-hero { min-height:960px; padding:350px 18px 112px; align-items:flex-end; }.hero-content { width:100%; padding:38px 24px; }.hero-content :deep(.water-glass--pool) { inset:-6px -7px; }.hero-depth-title { font-size:clamp(44px,13vw,62px); margin:25px 0; transform:perspective(650px) rotateY(-12deg) rotateZ(-4deg); }.hero-content h2 { font-size:24px; }.hero-content p { font-size:14px; }.hero-explore { left:18px; bottom:35px; font-size:12px; }.home-details { padding:40px 18px 50px; scroll-margin-top:170px; }.detail-intro,.capability-grid { grid-template-columns:1fr; gap:20px; }.detail-intro { padding:26px; }.home-steps,.home-faq { padding:28px; }.home-steps ol { grid-template-columns:1fr; gap:22px; }.capability-main { padding:30px; }.capability-secondary a { padding:28px; }.partners-page { padding:200px 18px 50px; }.partners-heading { padding:24px; }.partners-grid { grid-template-columns:1fr; }.home-footer { flex-wrap:wrap; gap:16px; }.home-footer span { flex-basis:60%; } }
/* 窄屏独立分配品牌、工具、导航与响应节点四个区域，不互相挤压标题。 */
@media(max-width:700px) {
 .immersive-home .home-navigation { display:grid; grid-template-columns:minmax(0,1fr) auto; gap:8px 6px; padding:12px; }
 .home-brand { grid-column:1; grid-row:1; gap:7px; font-size:clamp(14px,4.5vw,18px); }
 .home-brand > span:last-child { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
 .home-brand small { font-size:9px; }
 .home-tools { grid-column:2; grid-row:1; margin:0; gap:0; }
 .home-tools button { width:40px; min-width:40px; }
 .home-login { padding:10px; font-size:12px; }
 .home-navigation nav { grid-column:1 / -1; grid-row:2; width:100%; min-width:0; }
 .home-response-node { grid-column:1 / -1; grid-row:3; justify-self:end; max-width:100%; font-size:10px; padding:0 8px 2px; overflow:hidden; text-overflow:ellipsis; }
 .hero-content { min-width:0; }
 .hero-content :is(h2,p,.hero-quote) { overflow-wrap:anywhere; }
 .partners-heading :is(h1,p),.partner-card h2 { overflow-wrap:anywhere; }
}
@media(max-width:380px) { .home-brand { font-size:15px; gap:6px; }.home-mark { width:30px; height:30px; }.hero-content { padding:35px 20px; }.home-cta { padding:12px 18px; }.hero-explore { width:44px; padding:10px; justify-content:center; }.hero-explore span { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip-path:inset(50%); white-space:nowrap; } }
@media(prefers-reduced-motion:reduce) { .hero-depth-title,.immersive-home :is(a,button):hover { transform:none; }.immersive-home :is(a,button) { transition:background-color .15s,color .15s; } }
:global(html[data-xg-motion="off"] .immersive-home .hero-depth-title),:global(html[data-xg-motion="off"] .immersive-home :is(a,button):hover) { transform:none; }
@media(prefers-reduced-transparency:reduce) { .immersive-home .xg-glass { background:#102337 !important; backdrop-filter:none; -webkit-backdrop-filter:none; } }
@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))) { .immersive-home .xg-glass { background:#102337 !important; } }
</style>
