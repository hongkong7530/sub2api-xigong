<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useDocumentVisibility, useIntersectionObserver, useMediaQuery } from '@vueuse/core'
import { useSiteStats } from './useSiteStats'
import { useI18n } from 'vue-i18n'
const props = withDefaults(defineProps<{ active?: boolean }>(), { active: true })
const { t, locale } = useI18n()
const { data, failed, revision } = useSiteStats()
const host = shallowRef<HTMLElement>()
const visible = shallowRef(false)
const reduced = useMediaQuery('(prefers-reduced-motion: reduce)')
const visibility = useDocumentVisibility()
useIntersectionObserver(host, ([entry]) => { visible.value = entry.isIntersecting })
const moving = computed(() => props.active && !reduced.value && visible.value && visibility.value === 'visible')
const stamp = computed(() => data.value?.sampled_at ? new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(data.value.sampled_at)) : '—')
const models = computed(() => (data.value?.usage?.models || []).map(model => ({ ...model, share: Math.max(.015, model.tokens / Math.max(1, data.value?.usage?.models[0]?.tokens || 1)) })))
const number=(value:number|null|undefined)=>value==null?'—':new Intl.NumberFormat(locale.value,{notation:value>=1000000?'compact':'standard',maximumFractionDigits:2}).format(value)
const metrics=computed(()=>[
 {key:'onlineUsers',value:number(data.value?.online_users),hint:'onlineHint'},
 {key:'totalRequests',value:number(data.value?.usage?.requests),hint:'requestsHint'},
 {key:'inFlight',value:number(data.value?.in_flight),hint:'inFlightHint'},
 {key:'todayTokens',value:number(data.value?.usage?.today_tokens),hint:'tokensHint'},
 {key:'averagePrice',value:data.value?.usage?.average_per_million==null?'—':new Intl.NumberFormat(locale.value,{style:'currency',currency:'USD',maximumFractionDigits:2}).format(data.value.usage.average_per_million),hint:'priceHint'}
])
</script>
<template>
 <section ref="host" class="site-stats" :class="{ 'site-stats--moving': moving, 'site-stats--stale': failed }" aria-labelledby="site-stats-title">
  <div class="site-stats__optics" aria-hidden="true"><div class="site-stats__aurora" /><div class="site-stats__orbit" /><div class="site-stats__sweep" /><div class="site-stats__gridlines" /></div>
  <header class="site-stats__head"><div><p class="site-stats__eyebrow"><span class="site-stats__signal" />{{ t('experience.liveData') }}</p><h2 id="site-stats-title">{{ t('experience.statsTitle') }}</h2></div><div class="site-stats__sync"><span class="site-stats__stamp" role="status">{{ t(failed?'experience.statsStale':'experience.statsCadence') }}</span><time :datetime="data?.sampled_at">{{ stamp }}</time><span :key="revision" class="site-stats__packet" aria-hidden="true" /></div></header>
  <dl class="site-stats__grid"><div v-for="(metric,index) in metrics" :key="metric.key" class="site-stats__metric" :class="`site-stats__metric--${metric.key}`" :style="{ '--metric-delay': `${index * 75}ms` }"><dt><span class="site-stats__glyph" aria-hidden="true">{{ ['◉','↗','⋈','◇','◈'][index] }}</span>{{ t(`experience.${metric.key}`) }}</dt><dd><Transition name="stat-value" mode="out-in"><strong :key="metric.value">{{ metric.value }}</strong></Transition><small>{{ t(`experience.${metric.hint}`) }}</small></dd><div class="site-stats__rail" aria-hidden="true"><i /></div></div></dl>
  <div class="site-stats__models"><div class="site-stats__model-heading"><h3>{{ t('experience.popularModels') }}</h3><span class="site-stats__model-mark" aria-hidden="true"><i /><i /><i /><i /><i /></span></div><ol v-if="models.length"><li v-for="model in models" :key="model.model"><span class="site-stats__model-dot" aria-hidden="true" /><strong>{{ model.model }}</strong><span>{{ number(model.tokens) }} Token</span><div class="site-stats__model-track" aria-hidden="true"><i :style="{ transform: `scaleX(${model.share})` }" /></div></li></ol><p v-else>{{ t('experience.modelsUnavailable') }}</p></div>
 </section>
</template>
<style scoped>
.site-stats{--pulse-accent:var(--xg-brand,#bcffe8);--pulse-ink:#f1fbff;--pulse-muted:#c9e0eb;position:relative;isolation:isolate;margin-bottom:72px;padding:38px;border-radius:36px 36px 64px 36px;border:1px solid #d9ffff70;background:linear-gradient(130deg,#ffffff10,transparent 40%,hsl(var(--xg-h,174) 70% 70% / .09)),linear-gradient(140deg,hsl(var(--xg-h,174) 38% 17% / .79),#0b1d35d9);color:var(--pulse-ink);box-shadow:inset 0 2px 2px #ffffffa0,inset 2px 0 10px #b9ffff23,inset -1px -2px 5px #d3bcff50;backdrop-filter:blur(10px) saturate(1.5);-webkit-backdrop-filter:blur(10px) saturate(1.5)}
.site-stats::before{content:"";position:absolute;inset:8px;border:1px solid #ecffff25;border-radius:28px 28px 56px 28px;pointer-events:none}
.site-stats__optics{position:absolute;inset:0;overflow:hidden;border-radius:inherit;pointer-events:none;z-index:-1;contain:paint}
.site-stats__aurora{position:absolute;inset:-15%;background:radial-gradient(ellipse at 12% 4%,hsl(var(--xg-h,174) 100% 83% / .36),transparent 44%),radial-gradient(ellipse at 91% 87%,var(--xg-glow-b,#d0b5ff),transparent 40%);opacity:.3}
.site-stats__orbit{position:absolute;right:-150px;top:-230px;width:620px;height:620px;border:1px solid #d5ffff3b;border-radius:50%;box-shadow:0 0 0 32px #c9ffff07,0 0 0 90px #c9ffff04;transform:rotate(-26deg) scaleY(.56)}
.site-stats__sweep{position:absolute;inset:-25%;background:linear-gradient(112deg,transparent 43%,#c9fff921 48%,#ffffff2e 49%,transparent 54%);transform:translateX(-80%);opacity:0}
.site-stats__gridlines{position:absolute;inset:0;background-image:linear-gradient(#dcffff06 1px,transparent 1px),linear-gradient(90deg,#dcffff06 1px,transparent 1px);background-size:42px 42px;mask-image:linear-gradient(135deg,#000,transparent 65%)}
.site-stats__head{position:relative;display:flex;justify-content:space-between;align-items:flex-start;gap:24px}
.site-stats__eyebrow{display:flex;align-items:center;gap:10px;font-size:13px;color:var(--pulse-muted)}
.site-stats h2{font:clamp(26px,3vw,40px)/1.4 var(--xg-font-display);letter-spacing:.015em;color:var(--pulse-ink);margin-top:12px;text-wrap:balance}
.site-stats__signal{position:relative;width:8px;height:8px;flex-shrink:0;border-radius:50%;background:var(--pulse-accent);box-shadow:0 0 12px var(--pulse-accent)}
.site-stats__signal::after{content:"";position:absolute;inset:-5px;border:1px solid var(--pulse-accent);border-radius:50%;opacity:.4}
.site-stats__sync{position:relative;display:grid;gap:8px;justify-items:end;max-width:240px;padding:12px 16px;border-radius:16px;border:1px solid #efffff35;background:#091e304f;overflow:hidden}
.site-stats__stamp{font-size:12px;line-height:1.7;color:var(--pulse-muted)}
.site-stats__sync time{font-size:20px;font-variant-numeric:tabular-nums;letter-spacing:.08em;color:var(--pulse-accent)}
.site-stats__packet{position:absolute;bottom:0;left:0;right:0;height:2px;transform-origin:left;background:linear-gradient(90deg,transparent,var(--pulse-accent),#ffffff);transform:scaleX(0)}
.site-stats__grid{position:relative;display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px;margin-top:36px}
.site-stats__metric{position:relative;padding:20px 15px 22px;min-width:0;border-radius:20px;background:linear-gradient(145deg,#ffffff13,#ffffff03 70%);border:1px solid #e3ffff25;box-shadow:inset 0 1px #ffffff60;overflow:hidden}
.site-stats__metric dt{display:flex;gap:8px;align-items:center;font-size:12px;line-height:1.5;color:#e1f4f8}
.site-stats__glyph{display:grid;place-items:center;flex-shrink:0;color:var(--pulse-accent);font-size:18px;width:24px;height:24px}
.site-stats__metric dd{margin:0}
.site-stats__metric strong{display:block;min-height:1.4em;font-size:clamp(24px,2.6vw,38px);font-weight:650;font-variant-numeric:tabular-nums;line-height:1.4;letter-spacing:-.045em;margin:14px 0 10px;color:var(--pulse-accent);text-shadow:0 0 20px hsl(var(--xg-h,174) 90% 80% / .24);overflow-wrap:anywhere}
.site-stats__metric small{font-size:11px;line-height:1.8;color:var(--pulse-muted);display:block}
.site-stats__rail{position:absolute;bottom:0;left:15px;right:15px;height:2px;background:#ffffff13;overflow:hidden}
.site-stats__rail i{display:block;width:32%;height:100%;background:linear-gradient(90deg,transparent,var(--pulse-accent),transparent);transform:translateX(-150%)}
.site-stats__models{position:relative;border-top:1px solid #e5ffff30;margin-top:28px;padding-top:24px}
.site-stats__model-heading{display:flex;justify-content:space-between;align-items:center;gap:18px;margin-bottom:16px}
.site-stats__models h3{font-size:14px;font-weight:550;color:#e2f9f6}
.site-stats__model-mark{display:flex;align-items:center;gap:4px;height:20px}
.site-stats__model-mark i{height:10px;width:3px;border-radius:2px;background:var(--pulse-accent)}
.site-stats__model-mark i:nth-child(2n){height:17px}
.site-stats__models ol{display:grid;grid-template-columns:1fr 1fr;gap:18px 32px;list-style:none;margin:0;padding:0}
.site-stats__models li{position:relative;display:flex;gap:12px;align-items:center;padding:12px 0 19px;font-size:12px;min-width:0}
.site-stats__models strong{flex:1;min-width:0;overflow-wrap:anywhere;font-weight:600}
.site-stats__models li>span:not(.site-stats__model-dot){font-variant-numeric:tabular-nums;color:#d1e6ee;white-space:nowrap}
.site-stats__model-dot{width:5px;height:5px;border-radius:50%;background:var(--pulse-accent);flex-shrink:0}
.site-stats__model-track{position:absolute;bottom:0;left:0;right:0;height:3px;border-radius:3px;background:#eaffff12;overflow:hidden}
.site-stats__model-track i{display:block;width:100%;height:100%;background:linear-gradient(90deg,var(--pulse-accent),#f3ffff,var(--xg-glow-b,#cfb6ed));transform-origin:left;transition:none}
.site-stats__models p{font-size:13px;color:var(--pulse-muted);line-height:1.8}
.site-stats--stale .site-stats__signal{background:#f6d6a4;box-shadow:none}
.site-stats--stale .site-stats__stamp{color:#ffe1b4}
.site-stats--moving .site-stats__model-track i{transition:transform .8s cubic-bezier(.16,1,.3,1)}
.site-stats--moving .site-stats__aurora{animation:pulse-aurora 12s ease-in-out infinite alternate}
.site-stats--moving .site-stats__sweep{animation:pulse-sweep 9s ease-in-out infinite}
.site-stats--moving .site-stats__signal::after{animation:pulse-beacon 3s ease-out infinite}
.site-stats--moving .site-stats__packet{animation:pulse-packet 3s linear both}
.site-stats--moving .site-stats__rail i{animation:pulse-rail 6s ease-in-out infinite;animation-delay:var(--metric-delay)}
.site-stats--moving .stat-value-enter-active,.site-stats--moving .stat-value-leave-active{transition:opacity .15s,transform .2s}
.site-stats--moving .stat-value-enter-from{opacity:0;transform:translateY(7px)}
.site-stats--moving .stat-value-leave-to{opacity:0;transform:translateY(-5px)}
@keyframes pulse-aurora{to{transform:translate3d(3%,-3%,0) rotate(4deg);opacity:.5}}
@keyframes pulse-sweep{0%,55%{transform:translateX(-80%) rotate(-4deg);opacity:0}65%{opacity:.7}90%,100%{transform:translateX(80%) rotate(-4deg);opacity:0}}
@keyframes pulse-beacon{from{transform:scale(.7);opacity:.6}to{transform:scale(1.8);opacity:0}}
@keyframes pulse-packet{to{transform:scaleX(1)}}
@keyframes pulse-rail{0%,30%{transform:translateX(-150%)}80%,100%{transform:translateX(450%)}}
@media(max-width:1000px){.site-stats__grid{grid-template-columns:repeat(3,minmax(0,1fr))}.site-stats__metric:last-child{grid-column:span 2}}
@media(max-width:700px){.site-stats{padding:28px 20px;border-radius:28px 28px 44px 28px;margin-bottom:44px}.site-stats__head{flex-direction:column;gap:18px}.site-stats__sync{width:100%;max-width:none;grid-template-columns:1fr auto;align-items:center;justify-items:start;padding:12px 14px}.site-stats__sync time{font-size:17px;letter-spacing:.04em}.site-stats__grid{grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;margin-top:24px}.site-stats__metric{padding:16px 12px 20px;border-radius:16px}.site-stats__metric:last-child{grid-column:1/-1}.site-stats__metric strong{font-size:29px}.site-stats__metric dt{font-size:12px;gap:5px}.site-stats__glyph{width:18px;font-size:16px}.site-stats__models ol{grid-template-columns:1fr;gap:10px}.site-stats__models li{font-size:11px;gap:8px}.site-stats__models strong{font-size:12px}.site-stats__orbit{right:-360px;top:-180px}}
@media(prefers-reduced-motion:reduce){.site-stats *,.site-stats *::after{animation:none!important}.site-stats__model-track i{transition:none}}
@media(prefers-reduced-transparency:reduce){.site-stats{background:#102b3c;backdrop-filter:none;-webkit-backdrop-filter:none}.site-stats__optics{display:none}}
@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))){.site-stats{background:#102b3c}}
</style>
