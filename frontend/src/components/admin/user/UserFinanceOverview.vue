<template>
  <section class="space-y-4" :aria-busy="loading" data-testid="user-finance-overview">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.users.finance.title') }}</h2>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.users.finance.scope') }}</span>
    </div>
    <p v-if="error" role="status" class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('admin.users.finance.loadFailed') }}</p>
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div v-for="card in cards" :key="card.key" class="rounded-xl border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800/60">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ card.title }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ loading && !overview ? '…' : formatAmount(card.value) }}</p>
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ card.hint }}</p>
      </div>
    </div>
    <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/60">
      <h3 class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.users.finance.spendTrend') }}</h3>
      <div v-if="chartData" class="mt-3 h-40"><Line :data="chartData" :options="options" /></div>
      <p v-else class="flex h-40 items-center justify-center text-xs text-gray-500">{{ loading ? t('common.loading') : t('admin.users.finance.unavailable') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler } from 'chart.js'
import { Line } from 'vue-chartjs'
import type { UserFinanceOverview } from '@/api/admin/users'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler)
const props = defineProps<{ overview: UserFinanceOverview | null; loading: boolean; error: boolean }>()
const { t } = useI18n()
const formatAmount = (value: number | undefined) => value == null || !Number.isFinite(value) ? '—' : `$${value.toFixed(4)}`
const cards = computed(() => [
  { key: 'historical_recharged', title: t('admin.users.finance.historicalRecharged'), value: props.overview?.historical_recharged, hint: t('admin.users.finance.historicalRechargedHint') },
  { key: 'current_recharged', title: t('admin.users.finance.currentRecharged'), value: props.overview?.current_recharged, hint: t('admin.users.finance.currentRechargedHint') },
  { key: 'today_spent', title: t('admin.users.finance.todaySpent'), value: props.overview?.today_spent, hint: t('admin.users.finance.todaySpentHint') },
  { key: 'historical_spent', title: t('admin.users.finance.historicalSpent'), value: props.overview?.historical_spent, hint: t('admin.users.finance.historicalSpentHint') },
])
const chartData = computed(() => {
  const trend = props.overview?.spend_trend
  if (!trend?.length) return null
  return { labels: trend.map(p => p.date), datasets: [{ label: t('admin.users.finance.spendTrend'), data: trend.map(p => p.actual_cost), borderColor: '#14b8a6', backgroundColor: '#14b8a620', fill: true, tension: 0.2, pointRadius: 0, pointHitRadius: 8 }] }
})
const options = { responsive: true, maintainAspectRatio: false, animation: false as const, interaction: { intersect: false, mode: 'index' as const }, plugins: { legend: { display: false } }, scales: { x: { ticks: { color: '#94a3b8', maxTicksLimit: 8 }, grid: { display: false } }, y: { ticks: { color: '#94a3b8', callback: (v: string | number) => `$${Number(v).toFixed(2)}` }, grid: { color: '#64748b20' } } } }
</script>
