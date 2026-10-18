<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useDocumentVisibility, useIntersectionObserver, useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import MonitorCard from '@/components/user/monitor/MonitorCard.vue'
import { useHomeMonitors } from './useHomeMonitors'
import { sortMonitors } from '@/utils/monitorOrder'

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: true })
const { t, locale } = useI18n()
const host = shallowRef<HTMLElement>()
const visible = shallowRef(false)
const visibility = useDocumentVisibility()
const reduced = useMediaQuery('(prefers-reduced-motion: reduce)')
useIntersectionObserver(host, ([entry]) => { visible.value = Boolean(entry?.isIntersecting) }, { rootMargin: '160px' })
const moving = computed(() => props.active && visible.value && visibility.value === 'visible' && !reduced.value)
const { data, failed, loading, countdown, refresh } = useHomeMonitors(visible)
const items = computed(() => sortMonitors(data.value?.items ?? []))
const sampledAt = computed(() => {
  if (!data.value?.sampled_at) return '—'
  return new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(data.value.sampled_at))
})
</script>

<template>
  <section ref="host" class="home-monitors" :class="{ 'home-monitors--moving': moving }" aria-labelledby="home-monitors-title" :aria-busy="loading">
    <div class="home-monitors__atmosphere" aria-hidden="true"><i /><i /></div>
    <header class="home-monitors__head">
      <div class="home-monitors__intro">
        <h2 id="home-monitors-title">{{ t('experience.monitorTitle') }}</h2>
        <p>{{ t('experience.monitorIntro') }}</p>
      </div>
      <div class="home-monitors__sync">
        <span class="home-monitors__count">{{ t('experience.monitorCount', { n: items.length }) }}</span>
        <time v-if="data" :datetime="data.sampled_at">{{ t('experience.monitorUpdated', { time: sampledAt }) }}</time>
        <span class="home-monitors__cadence">{{ t('experience.monitorCadence') }}</span>
      </div>
    </header>
    <div v-if="failed" class="home-monitors__message" role="alert">
      <p>{{ t('experience.monitorUnavailable') }}</p>
      <button type="button" :disabled="loading" @click="refresh">{{ t('experience.retry') }}</button>
    </div>
    <div v-else-if="loading && !data" class="home-monitors__grid" role="status">
      <span class="sr-only">{{ t('experience.loading') }}</span>
      <div v-for="n in 3" :key="n" class="home-monitors__skeleton" aria-hidden="true"><i /><i /><i /></div>
    </div>
    <p v-else-if="!items.length" class="home-monitors__message" role="status">{{ t('experience.monitorEmpty') }}</p>
    <div v-else class="home-monitors__grid">
      <MonitorCard
        v-for="item in items" :key="item.id" :item="item" window="7d"
        :availability-value="item.availability_7d" :countdown-seconds="countdown"
        appearance="home" read-only :moving="moving"
      />
    </div>
  </section>
</template>

<style scoped>
.home-monitors{--monitor-light:var(--xg-glow-a,#99fff0);--monitor-secondary:var(--xg-glow-b,#efadff);--monitor-flare:#ff97da;--monitor-ink:#f2fbff;--monitor-muted:#c8dde8;position:relative;isolation:isolate;margin:0 0 82px;color:var(--monitor-ink)}
.home-monitors__atmosphere{position:absolute;inset:-70px -25px -45px;z-index:-1;overflow:hidden;pointer-events:none;contain:paint}
.home-monitors__atmosphere i{position:absolute;width:72%;height:60%;border-radius:50%;background:radial-gradient(ellipse,var(--monitor-light),transparent 67%);opacity:.18;left:-18%;top:5%;transform:rotate(-18deg)}
.home-monitors__atmosphere i+ i{background:radial-gradient(ellipse,var(--monitor-flare),transparent 67%);left:auto;right:-24%;top:25%;opacity:.2}
.home-monitors__head{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;margin-bottom:28px;padding:0 8px}
.home-monitors__intro{min-width:0;max-width:640px}
.home-monitors__intro h2{font:clamp(26px,3vw,40px)/1.4 var(--xg-font-display);color:var(--monitor-ink);text-shadow:0 0 24px color-mix(in srgb,var(--monitor-light) 36%,transparent);text-wrap:balance}
.home-monitors__intro p{margin-top:12px;color:var(--monitor-muted);font-size:13px;line-height:1.9}
.home-monitors__sync{display:grid;gap:7px;flex-shrink:0;text-align:right;font-size:11px;color:var(--monitor-muted)}
.home-monitors__count{font-size:15px;font-weight:650;color:var(--monitor-light)}
.home-monitors__sync time{font-variant-numeric:tabular-nums}
.home-monitors__cadence{max-width:250px;line-height:1.8}
.home-monitors__grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:24px}
.home-monitors__message{display:flex;align-items:center;justify-content:space-between;gap:18px;padding:24px;border:1px solid #d9ffff60;border-radius:22px;background:#102439d9;font-size:14px;line-height:1.8}
.home-monitors__message button{flex-shrink:0;min-height:44px;padding:8px 16px;border:1px solid var(--monitor-light);border-radius:12px;color:var(--monitor-light);background:#baffeb0d;transition:background-color .2s}
.home-monitors__message button:hover{background:#baffeb20}.home-monitors__message button:active{background:#baffeb30}.home-monitors__message button:disabled{opacity:.5;cursor:wait}.home-monitors__message button:focus-visible{outline:2px solid #fff;outline-offset:4px}
.home-monitors__skeleton{min-height:370px;border-radius:28px;border:1px solid #e2ffff3d;background:linear-gradient(130deg,#dfffff16,#cbaaff0c),#102638a6;padding:26px}
.home-monitors__skeleton i{display:block;height:20px;width:70%;margin-bottom:28px;background:#d8f5ff1c;border-radius:8px}.home-monitors__skeleton i+ i{height:90px;width:100%}.home-monitors__skeleton i:last-child{height:40px;margin-top:55px}
/* 首页材质完全限于这个容器，不改变控制台卡片的主题与交互。 */
.home-monitors :deep(.monitor-card--home){--card-light:#a3ffdf;--card-flare:#89e4ff;position:relative;isolation:isolate;overflow:hidden;min-width:0;padding:24px;border-radius:28px;border:1px solid color-mix(in srgb,var(--card-light) 60%,#fff4);background:linear-gradient(135deg,#e8ffff17,transparent 45%,#e9b4ff12),linear-gradient(155deg,#102c44b8,#091729dc);color:var(--monitor-ink);box-shadow:inset 0 1px 1px #fff9,inset 0 -1px 12px #b5dfff16,0 0 26px color-mix(in srgb,var(--card-light) 15%,transparent),0 0 48px color-mix(in srgb,var(--card-flare) 9%,transparent);backdrop-filter:blur(14px) saturate(1.6);-webkit-backdrop-filter:blur(14px) saturate(1.6);transform:none;transition:none;cursor:default}
.home-monitors :deep(.monitor-card--home[data-status='degraded']){--card-light:#ffe0a0;--card-flare:#ffb0db}
.home-monitors :deep(.monitor-card--home[data-status='failed']),.home-monitors :deep(.monitor-card--home[data-status='error']){--card-light:#ffb0cb;--card-flare:#ffc3a1}
.home-monitors :deep(.monitor-card--home[data-status='']){--card-light:#d0e2ff;--card-flare:#d6bcff}
.home-monitors :deep(.monitor-card--home)::before{content:"";position:absolute;inset:6px;border-radius:22px;border:1px solid #dfffff1c;pointer-events:none}
.home-monitors :deep(.monitor-card__halo){position:absolute;inset:0;z-index:-1;pointer-events:none;background:radial-gradient(ellipse at 0 0,color-mix(in srgb,var(--card-light) 36%,transparent),transparent 55%),radial-gradient(ellipse at 100% 100%,color-mix(in srgb,var(--card-flare) 24%,transparent),transparent 65%);opacity:.65}
.home-monitors :deep(.monitor-card__glint){position:absolute;z-index:-1;inset:-40% -60%;pointer-events:none;background:linear-gradient(112deg,transparent 40%,#fff0 44%,#d5ffff1c 48%,#fff6 49%,#fff0 51%,transparent 58%);transform:translateX(-65%);opacity:0}
.home-monitors :deep(.monitor-card__header){gap:10px;position:relative}
.home-monitors :deep(.monitor-card__name){font-family:var(--xg-font-display);font-weight:650;color:#f5fcff;font-size:16px;line-height:1.5}
.home-monitors :deep(.monitor-card__identity){flex-wrap:wrap;gap:6px;line-height:1.5;margin-top:7px}
.home-monitors :deep(.monitor-card__identity > span){max-width:100%;min-width:0;overflow-wrap:anywhere;white-space:normal}
.home-monitors :deep(.monitor-card__identity > .monitor-card__model){color:#dceaf3;font-size:11px;flex-basis:100%;order:2}
.home-monitors :deep(.monitor-card__identity > span:not(.monitor-card__model)){font-size:10px;border:1px solid #dcffff2e;background:#e5ffff10;color:#d9f6f6}
.home-monitors :deep(.monitor-card__status){color:var(--card-light);background:color-mix(in srgb,var(--card-light) 11%,#08182e);border:1px solid color-mix(in srgb,var(--card-light) 50%,transparent);font-size:10px;box-shadow:0 0 16px color-mix(in srgb,var(--card-light) 20%,transparent)}
.home-monitors :deep(.monitor-metrics__cell){padding:12px 10px;border:1px solid #d5ffff28;background:#e4faff08;box-shadow:inset 0 1px #fff3}
.home-monitors :deep(.monitor-metrics__label){color:var(--monitor-muted);font-size:10px;letter-spacing:0;text-transform:none;align-items:flex-start;line-height:1.6}
.home-monitors :deep(.monitor-metrics__value){color:var(--card-light);font-size:24px;font-family:inherit;text-shadow:0 0 18px color-mix(in srgb,var(--card-light) 35%,transparent);overflow-wrap:anywhere}
.home-monitors :deep(.monitor-metrics__value > span){font-size:10px;color:#c8dbe7;white-space:nowrap}
.home-monitors :deep(.monitor-card__insight){gap:7px}
.home-monitors :deep(.monitor-card__insight > span){min-width:0;padding:10px;background:#e2faff08;border:1px solid #d5ffff1a;color:#cef7ee;line-height:1.6}
.home-monitors :deep(.monitor-card__insight > span > span){font-size:10px;opacity:1;color:var(--monitor-muted)}
.home-monitors :deep(.monitor-card__insight strong){font-size:15px}
.home-monitors :deep(.monitor-samples){color:var(--monitor-muted);border-color:#d5ffff26}
.home-monitors :deep(.monitor-samples :is(h4,strong)){color:#edf9ff}
.home-monitors :deep(.monitor-samples .text-gray-500),.home-monitors :deep(.monitor-samples .text-gray-400){color:var(--monitor-muted)}
.home-monitors :deep(.monitor-card__divider),.home-monitors :deep(.monitor-timeline){border-color:#d5ffff26}
.home-monitors :deep(.monitor-availability__label),.home-monitors :deep(.monitor-timeline__caption){color:var(--monitor-muted);letter-spacing:0;text-transform:none;line-height:1.6}
.home-monitors :deep(.monitor-availability__value){font-size:29px}
.home-monitors :deep(.monitor-timeline){margin-top:auto;padding-top:16px}
.home-monitors :deep(.monitor-timeline__caption){gap:10px}
.home-monitors :deep(.monitor-timeline__bar.bg-emerald-500){background:#9affd5;box-shadow:0 0 7px #8fffc94d}
.home-monitors :deep(.monitor-timeline__bar.bg-amber-500){background:#ffda9d}
.home-monitors :deep(.monitor-timeline__bar.bg-red-500){background:#ffacc8}
.home-monitors :deep(.monitor-timeline__bar.bg-gray-300){background:#7894ac66}
.home-monitors--moving .home-monitors__atmosphere i{animation:channel-aurora 14s ease-in-out infinite alternate}
.home-monitors--moving .home-monitors__atmosphere i+ i{animation-delay:-7s}
.home-monitors :deep(.monitor-card--moving .monitor-card__glint){animation:channel-glint 11s ease-in-out infinite}
.home-monitors :deep(.monitor-card--moving .monitor-card__halo){animation:channel-halo 7s ease-in-out infinite alternate}
@keyframes channel-aurora{to{transform:translate3d(5%,4%,0) rotate(8deg);opacity:.27}}
@keyframes channel-halo{to{opacity:.95;transform:translate3d(2%,-1%,0)}}
@keyframes channel-glint{0%,64%{transform:translateX(-65%);opacity:0}72%{opacity:.65}94%,100%{transform:translateX(65%);opacity:0}}
@media(max-width:1150px){.home-monitors__grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:760px){.home-monitors{margin-bottom:52px}.home-monitors__head{display:block;padding:0 4px}.home-monitors__sync{margin-top:20px;text-align:left;grid-template-columns:1fr auto;align-items:center;gap:8px}.home-monitors__cadence{grid-column:1/-1;max-width:none}.home-monitors__grid{grid-template-columns:1fr;gap:22px}.home-monitors :deep(.monitor-card--home){padding:22px;border-radius:24px}.home-monitors__message{align-items:flex-start;flex-direction:column}.home-monitors :deep(.monitor-card__insight){grid-template-columns:repeat(2,minmax(0,1fr))}.home-monitors :deep(.monitor-card__insight > span:last-child:nth-child(3)){grid-column:1/-1}}
/* 手机卡片以实底承载读数；饱和状态与形状编码不被壁纸或全卡辉光冲淡。 */
@media(max-width:767px){
 .home-monitors__head{padding:18px;border:1px solid #c1d8ec55;border-radius:18px;background:#0b1c30f5}
 .home-monitors :deep(.monitor-card.monitor-card--home){--card-light:#55edb5;--card-flare:#6fdfff;background:#102238;border-color:#6a8199;backdrop-filter:none;-webkit-backdrop-filter:none;box-shadow:inset 3px 0 var(--card-light),0 4px 16px #0003}
 .home-monitors :deep(.monitor-card.monitor-card--home[data-status='degraded']){--card-light:#ffd262;--card-flare:#ffbf70}
 .home-monitors :deep(.monitor-card.monitor-card--home[data-status='failed']),.home-monitors :deep(.monitor-card.monitor-card--home[data-status='error']){--card-light:#ff83a1;--card-flare:#ffa1bc}
 .home-monitors :deep(.monitor-card__halo){opacity:.16}
 .home-monitors :deep(.monitor-card__glint){display:none}
 .home-monitors :deep(.monitor-card__status){font-size:12px;font-weight:800;background:#071424;border-color:var(--card-light);box-shadow:none;padding:6px 10px}
 .home-monitors :deep(.monitor-metrics__cell){background:#08182b;border-color:#5f7890}
 .home-monitors :deep(.monitor-metrics__label),.home-monitors :deep(.monitor-timeline__caption),.home-monitors :deep(.monitor-availability__label){color:#d7e6f4;font-size:11px}
 .home-monitors :deep(.monitor-metrics__value){color:#f7fbff;text-shadow:none;font-size:26px}
 .home-monitors :deep(.monitor-card__insight > span){background:#0a192c;border-color:#516c82;color:#f4f9ff}
 .home-monitors :deep(.monitor-card__insight > span > span){color:#d1e4f2;font-size:11px}
 .home-monitors :deep(.monitor-timeline__bar.bg-emerald-500){background:#34d399;box-shadow:none}
 .home-monitors :deep(.monitor-timeline__bar.bg-amber-500){background:#fbbf24}
 .home-monitors :deep(.monitor-timeline__bar.bg-red-500){background:#fb7185}
 .home-monitors :deep(.monitor-timeline__bar.bg-gray-300){background:#6b829a}
}
@media(prefers-reduced-motion:reduce){.home-monitors *,.home-monitors :deep(*){animation:none!important;transition:none!important}}
@media(prefers-reduced-transparency:reduce){.home-monitors :deep(.monitor-card--home){background:#10263b;backdrop-filter:none;-webkit-backdrop-filter:none}.home-monitors__atmosphere,.home-monitors :deep(.monitor-card__halo),.home-monitors :deep(.monitor-card__glint){display:none}}
@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))){.home-monitors :deep(.monitor-card--home){background:#10263b}}
</style>
