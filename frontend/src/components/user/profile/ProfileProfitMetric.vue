<script setup lang="ts">
import { onMounted, onBeforeUnmount, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { profitApi, type ProfitContribution } from '@/api/profit'
import { metricProfit, profitPercent, negativeMoney } from '@/features/profit/formatProfit'
const { t } = useI18n()
const data = shallowRef<ProfitContribution | null>(null)
const failed = shallowRef(false)
const controller = new AbortController()
onMounted(async () => { try { data.value = await profitApi.contribution(controller.signal) } catch { if (!controller.signal.aborted) failed.value = true } })
onBeforeUnmount(() => controller.abort())
</script>
<template>
  <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700" data-testid="profile-profit-metric" :title="t('profit.personalHint')">
    <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('profit.personal') }}</p>
    <p class="mt-1 text-lg font-semibold tabular-nums" :class="negativeMoney(data?.total.profit) ? 'text-rose-700 dark:text-rose-300' : 'text-primary-700 dark:text-primary-300'">{{ data ? metricProfit(data.total) : '—' }}</p>
    <p v-if="failed" class="mt-1 text-[10px] text-gray-500" role="status">{{ t('profit.personalError') }}</p>
    <p v-else-if="data" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">{{ t(data.stale ? 'profit.stale' : 'profit.coverage') }} {{ data.stale ? '' : profitPercent(data.total.coverage) }}</p>
  </div>
</template>
