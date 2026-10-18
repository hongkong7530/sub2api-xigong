<template>
  <section class="monitor-samples mt-4 min-w-0 border-t border-gray-100 pt-3 dark:border-dark-700" data-testid="user-first-token-samples">
    <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
      <h4 class="font-semibold text-gray-700 dark:text-gray-200">{{ t('monitorCommon.realFirstTokens') }}</h4>
      <span class="tabular-nums">{{ t('monitorCommon.firstTokenSamples', { n: recent.length }) }}</span>
    </div>
    <div class="mt-2 flex items-baseline gap-2">
      <strong class="text-lg font-bold tabular-nums text-gray-900 dark:text-gray-100" data-testid="first-token-latest">{{ latestLabel }}</strong>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('monitorCommon.firstTokenLatest') }}</span>
    </div>
    <div v-if="recent.length" class="mt-2 grid h-6 items-end gap-0.5" style="grid-template-columns: repeat(30, minmax(0, 1fr))">
      <span
        v-for="(sample, index) in chronological"
        :key="index"
        class="min-w-0 rounded-sm"
        :class="sampleClass(sample.first_token_ms)"
        :style="{ height: sampleHeight(sample.first_token_ms) }"
        :title="sampleTitle(sample)"
        :aria-label="sampleTitle(sample)"
        role="img"
      ></span>
    </div>
    <p v-else class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('monitorCommon.realFirstTokensEmpty') }}</p>
    <ol v-if="detailed && recent.length" class="mt-3 max-h-64 space-y-1 overflow-y-auto">
      <li v-for="(sample, index) in recent" :key="index" class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 py-1.5 text-xs dark:border-dark-700">
        <span class="min-w-0 flex-1 break-words text-gray-600 dark:text-gray-300">{{ sample.model }}</span>
        <span class="tabular-nums text-gray-500">{{ sample.created_at.replace('T', ' ').replace('Z', ' UTC') }}</span>
        <strong class="tabular-nums text-gray-900 dark:text-gray-100">{{ formatSample(sample.first_token_ms) }}</strong>
      </li>
    </ol>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserFirstTokenSample } from '@/api/channelMonitor'
import { firstTokenSeverity, LATENCY_BAR_CLASSES } from '@/utils/latencyHealth'

const props = withDefaults(defineProps<{
  samples?: UserFirstTokenSample[]
  detailed?: boolean
}>(), { samples: () => [], detailed: false })
const { t } = useI18n()
const recent = computed(() => props.samples.slice(0, 30))
const chronological = computed(() => [...recent.value].reverse())
// 最新请求未记录首字时保持未知，不用更早样本或中位数冒充。
const latestLabel = computed(() => {
  const value = recent.value[0]?.first_token_ms
  return value == null || !Number.isFinite(value) || value < 0 ? '-' : `${(value / 1000).toFixed(2)}s`
})
function formatSample(value: number | null): string {
  return value == null ? t('monitorCommon.firstTokenMissing') : `${(value / 1000).toFixed(2)}s`
}
function sampleClass(value: number | null): string {
  if (value == null) return 'bg-gray-300 dark:bg-dark-600'
  return LATENCY_BAR_CLASSES[firstTokenSeverity(value)]
}
function sampleHeight(value: number | null): string {
  if (value == null) return '20%'
  return { good: '100%', warn: '75%', slow: '50%', critical: '30%' }[firstTokenSeverity(value)]
}
function sampleTitle(sample: UserFirstTokenSample): string {
  return `${sample.model} · ${formatSample(sample.first_token_ms)} · ${sample.created_at}`
}
</script>
