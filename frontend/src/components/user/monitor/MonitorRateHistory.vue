<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { buildMonitorRateHistory, type MonitorRatePoint } from '@/utils/monitorRateHistory'
const props = defineProps<{ history: MonitorRatePoint[]; currentRate: number }>()
const { t } = useI18n()
const chart = computed(() => buildMonitorRateHistory(props.history, props.currentRate))
const newest = computed(() => chart.value.entries.slice().reverse())
const rate = (value: number) => Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: 4 }) : '—'
const time = (value: string) => new Date(value).toLocaleString(undefined, { hour12: false, timeZoneName: 'short' })
</script>
<template>
  <section class="mt-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-700 dark:bg-dark-900/40" data-testid="monitor-rate-history">
    <header class="flex flex-wrap items-center justify-between gap-2 text-xs">
      <h3 class="font-semibold text-gray-700 dark:text-gray-200">{{ t('monitorCommon.rateHistoryTitle') }}</h3>
      <span class="text-gray-500 dark:text-gray-400">{{ t(chart.changes ? 'monitorCommon.rateChangesCount' : 'monitorCommon.rateNoChanges', { count: chart.changes }) }}</span>
    </header>
    <svg v-if="chart.path" viewBox="0 0 640 96" class="mt-2 h-24 w-full text-primary-500" role="img" :aria-label="t('monitorCommon.groupRateTrend')">
      <path d="M10 80H630 M10 16H630" stroke="currentColor" stroke-opacity=".12" fill="none" />
      <path :d="chart.path" stroke="currentColor" stroke-width="2.5" stroke-linejoin="round" fill="none" vector-effect="non-scaling-stroke" data-testid="rate-step-path" />
      <circle v-for="point in chart.entries" :key="point.index" :cx="point.x" :cy="point.y" r="3.5" :class="point.direction === 'decrease' ? 'text-emerald-500' : point.direction === 'increase' ? 'text-rose-500' : 'text-primary-500'" fill="currentColor"><title>{{ time(point.recorded_at) }} · ×{{ rate(point.rate_multiplier) }}</title></circle>
    </svg>
    <p v-if="!newest.length" class="mt-2 text-xs text-gray-600 dark:text-gray-300">{{ t('monitorCommon.rateCurrentBaseline', { rate: rate(currentRate) }) }}</p>
    <ol v-else class="mt-2 max-h-48 space-y-2 overflow-y-auto pr-1">
      <li v-for="point in newest" :key="point.index" class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 rounded-lg bg-white/80 px-3 py-2 text-xs dark:bg-dark-800/70">
        <div class="flex flex-wrap items-center gap-2"><span class="font-semibold" :class="point.direction === 'decrease' ? 'text-emerald-700 dark:text-emerald-300' : point.direction === 'increase' ? 'text-rose-700 dark:text-rose-300' : 'text-gray-700 dark:text-gray-200'">{{ t('monitorCommon.rateEvent.' + point.direction) }}</span><span class="tabular-nums text-gray-700 dark:text-gray-200"><template v-if="point.previous != null">×{{ rate(point.previous) }} → </template>×{{ rate(point.rate_multiplier) }}</span><span v-if="point.delta" class="tabular-nums text-gray-500">{{ point.delta > 0 ? '+' : '−' }}{{ rate(Math.abs(point.delta)) }}</span></div>
        <time :datetime="point.recorded_at" class="text-gray-500 dark:text-gray-400">{{ time(point.recorded_at) }}</time>
      </li>
    </ol>
    <p class="mt-2 text-[11px] leading-relaxed text-gray-500 dark:text-gray-400">{{ t('monitorCommon.rateHistoryHint') }}</p>
  </section>
</template>
