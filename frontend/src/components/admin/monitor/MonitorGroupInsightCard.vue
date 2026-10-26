<template>
  <section class="card p-4">
    <div class="flex items-start justify-between gap-3">
      <div class="flex min-w-0 items-start gap-2.5">
        <span class="mt-0.5 inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-sky-50 text-sky-500 dark:bg-sky-900/30 dark:text-sky-400">
          <Icon name="chart" size="sm" />
        </span>
        <div class="min-w-0">
          <p class="truncate text-base font-bold text-gray-900 dark:text-white">{{ groupName }}</p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channelMonitor.groupInsight.subtitle') }}</p>
        </div>
      </div>
      <button
        type="button"
        class="rounded-lg p-1.5 text-gray-500 transition hover:bg-gray-100 hover:text-primary-600 disabled:opacity-50 dark:hover:bg-dark-700"
        :disabled="loading"
        :title="t('admin.channelMonitor.groupInsight.refresh')"
        @click="load"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <div v-if="loading && !metrics" class="mt-4 h-24 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-900" />
    <div v-else-if="error" class="mt-4 rounded-xl bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-500/10 dark:text-red-300">
      {{ error }}
    </div>
    <template v-else-if="metrics">
      <div class="mt-4 grid grid-cols-2 gap-2">
        <div class="rounded-xl bg-sky-50 px-3 py-2.5 dark:bg-sky-500/10">
          <p class="text-sm font-medium text-sky-700/75 dark:text-sky-300/75">{{ t('admin.channelMonitor.groupInsight.currentRate') }}</p>
          <p class="mt-0.5 text-2xl font-black tabular-nums text-sky-700 dark:text-sky-300">{{ metrics.effective_rate_multiplier.toFixed(4) }}</p>
          <p v-if="metrics.peak_rate_enabled" class="text-xs font-medium text-sky-700/75 dark:text-sky-300/75">{{ t('admin.channelMonitor.groupInsight.peak', { value: metrics.peak_rate_multiplier.toFixed(4) }) }}</p>
        </div>
        <div class="rounded-xl bg-emerald-50 px-3 py-2.5 dark:bg-emerald-500/10">
          <p class="text-sm font-medium text-emerald-700/75 dark:text-emerald-300/75">{{ t('admin.channelMonitor.groupInsight.cacheRate') }}</p>
          <p class="mt-0.5 text-2xl font-black tabular-nums text-emerald-700 dark:text-emerald-300">{{ metrics.cache_hit_rate == null ? '—' : `${metrics.cache_hit_rate.toFixed(1)}%` }}</p>
          <p class="text-xs font-medium text-emerald-700/75 dark:text-emerald-300/75">{{ t('admin.channelMonitor.groupInsight.cacheRead', { value: compact(metrics.cache_read_tokens) }) }}</p>
        </div>
      </div>

      <MonitorRateHistory :history="history" :current-rate="metrics.rate_multiplier" />

      <div class="mt-3 rounded-xl border border-gray-100 bg-gray-50/70 p-2 dark:border-dark-700 dark:bg-dark-900/40">
        <div class="mb-2 flex items-center justify-between text-xs font-medium text-gray-600 dark:text-gray-300">
          <span>{{ t('admin.channelMonitor.groupInsight.firstTokens') }}</span>
          <span>{{ firstTokens.length ? t('admin.channelMonitor.groupInsight.records', { count: firstTokens.length }) : t('admin.channelMonitor.groupInsight.noRecords') }}</span>
        </div>
        <div v-if="firstTokens.length" class="mb-2 flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
          <span class="inline-flex items-center gap-1"><span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>&lt;2s</span>
          <span class="inline-flex items-center gap-1"><span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span>&lt;5s</span>
          <span class="inline-flex items-center gap-1"><span class="h-1.5 w-1.5 rounded-full bg-red-500"></span>≥5s</span>
        </div>
        <ul v-if="firstTokens.length" class="max-h-44 space-y-1 overflow-y-auto pr-1">
          <li
            v-for="item in firstTokens"
            :key="item.id"
            class="flex items-center justify-between gap-3 rounded-lg bg-white/80 px-3 py-2 text-sm dark:bg-dark-800/70"
          >
            <span class="shrink-0 tabular-nums text-gray-400 dark:text-gray-500">{{ shortTime(item.created_at) }}</span>
            <span class="min-w-0 flex-1 truncate text-gray-600 dark:text-gray-300">{{ item.requested_model || item.model }}</span>
            <span class="shrink-0 tabular-nums text-gray-400 dark:text-gray-500">{{ formatMs(item.duration_ms) }}</span>
            <span class="shrink-0 font-semibold tabular-nums" :class="firstTokenClass(item.first_token_ms)">{{ formatMs(item.first_token_ms) }}</span>
          </li>
        </ul>
        <p v-else class="flex h-10 items-center justify-center text-xs text-gray-400">{{ t('admin.channelMonitor.groupInsight.emptyFirstTokens') }}</p>
      </div>

      <div class="mt-3 rounded-xl border border-gray-100 bg-gray-50/70 p-2 dark:border-dark-700 dark:bg-dark-900/40">
        <button
          type="button"
          class="flex w-full items-center justify-between text-xs font-medium text-gray-600 dark:text-gray-300"
          @click="reportsOpen = !reportsOpen"
        >
          <span>{{ t('admin.channelMonitor.groupInsight.reports', { count: reports.length }) }}</span>
          <Icon :name="reportsOpen ? 'chevronUp' : 'chevronDown'" size="xs" />
        </button>
        <ul v-if="reportsOpen" class="mt-2 max-h-44 space-y-1 overflow-y-auto pr-1">
          <li
            v-for="item in reports"
            :key="item.id"
            class="rounded-lg bg-white/80 px-3 py-2 text-sm dark:bg-dark-800/70"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="inline-flex items-center gap-1.5">
                <span class="h-1.5 w-1.5 rounded-full" :class="reportDot(item.category)"></span>
                {{ t(`admin.channelMonitor.groupInsight.reportCategories.${item.category}`) }}
              </span>
              <span class="tabular-nums text-xs text-gray-400">{{ shortTime(item.created_at) }}</span>
            </div>
            <p class="mt-1 truncate text-xs text-gray-500">{{ item.deleted ? t('admin.channelMonitor.groupInsight.deletedUser') : (item.username || item.email) }}</p>
            <p v-if="item.body" class="mt-1 line-clamp-2 text-xs text-gray-600 dark:text-gray-300">{{ item.body }}</p>
          </li>
          <li v-if="!reports.length" class="py-2 text-center text-xs text-gray-400">{{ t('admin.channelMonitor.groupInsight.emptyReports') }}</li>
        </ul>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { GroupFirstTokenItem, GroupMonitorMetrics, GroupRateHistoryItem, GroupReportItem } from '@/api/admin/channelMonitor'
import Icon from '@/components/icons/Icon.vue'
import MonitorRateHistory from '@/components/user/monitor/MonitorRateHistory.vue'

const { t } = useI18n()
const props = defineProps<{ groupId: number; groupName: string }>()
const metrics = ref<GroupMonitorMetrics | null>(null)
const history = ref<GroupRateHistoryItem[]>([])
const firstTokens = ref<GroupFirstTokenItem[]>([])
const reports = ref<GroupReportItem[]>([])
const reportsOpen = ref(false)
const loading = ref(false)
const error = ref('')
let timer: ReturnType<typeof setInterval> | null = null


function compact(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
}

function shortTime(value: string): string {
  // 跟随浏览器区域设置，避免硬编码 zh-CN 在英文界面显示不一致。
  return new Date(value).toLocaleTimeString(undefined, { hour12: false })
}

function formatMs(value: number | null): string {
  if (value == null) return '—'
  return value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${value}ms`
}

// 首字延迟分档与渠道监控探活的体感阈值保持一致：<2s 佳、<5s 可接受、更慢标红。
function reportDot(category: string): string {
  switch (category) {
    case 'unreachable':
      return 'bg-red-500'
    case 'slow_first_token':
      return 'bg-amber-500'
    case 'usage_doubt':
      return 'bg-violet-500'
    default:
      return 'bg-gray-400'
  }
}

function firstTokenClass(value: number | null): string {
  if (value == null) return 'text-gray-400 dark:text-gray-500'
  if (value < 2000) return 'text-emerald-600 dark:text-emerald-400'
  if (value < 5000) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

async function load() {
  if (!props.groupId) return
  loading.value = true
  error.value = ''
  try {
    const [nextMetrics, nextHistory, nextFirstTokens, nextReports] = await Promise.all([
      adminAPI.channelMonitor.getGroupMetrics(props.groupId),
      adminAPI.channelMonitor.listGroupRateHistory(props.groupId, 120),
      adminAPI.channelMonitor.listGroupFirstTokens(props.groupId),
      adminAPI.channelMonitor.listGroupReports(props.groupId),
    ])
    metrics.value = nextMetrics
    history.value = nextHistory.items || []
    firstTokens.value = nextFirstTokens.items || []
    reports.value = nextReports.items || []
  } catch {
    error.value = t('admin.channelMonitor.groupInsight.unavailable')
  } finally {
    loading.value = false
  }
}

watch(() => props.groupId, () => void load(), { immediate: true })
onMounted(() => { timer = setInterval(() => void load(), 30_000) })
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>
