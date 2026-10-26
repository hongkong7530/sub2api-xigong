<template>
  <BaseDialog
    :show="show"
    :title="title"
    width="wide"
    @close="$emit('close')"
  >
    <div v-if="loading" class="py-8 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="!detail" class="py-8 text-center text-sm text-gray-500">
      {{ t('channelStatus.detailLoadError') }}
    </div>
    <div v-else class="space-y-4">
      <!-- 分组实时指标：倍率/峰值/缓存率 + 倍率变更曲线（分组绑定监控才有） -->
      <div v-if="detail.group_insight" class="rounded-xl border border-gray-200/80 p-3 dark:border-dark-700/70">
        <div class="flex flex-wrap items-center gap-3 text-xs">
          <span class="inline-flex items-center gap-1 rounded-lg bg-sky-50 px-2.5 py-1.5 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300">
            {{ t('monitorCommon.groupRate') }}
            <strong class="font-black tabular-nums">×{{ formatRate(detail.group_insight.effective_rate_multiplier) }}</strong>
          </span>
          <span v-if="detail.group_insight.peak_rate_enabled" class="inline-flex items-center gap-1 rounded-lg bg-amber-50 px-2.5 py-1.5 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">
            {{ t('monitorCommon.groupPeak') }}
            <strong class="font-black tabular-nums">×{{ formatRate(detail.group_insight.peak_rate_multiplier) }}</strong>
          </span>
          <span class="inline-flex items-center gap-1 rounded-lg bg-emerald-50 px-2.5 py-1.5 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">
            {{ t('monitorCommon.groupCache') }}
            <strong class="font-black tabular-nums">{{ detail.group_insight.cache_hit_rate == null ? '—' : `${detail.group_insight.cache_hit_rate.toFixed(1)}%` }}</strong>
          </span>
        </div>
        <MonitorRateHistory :history="detail.group_insight.rate_history ?? []" :current-rate="detail.group_insight.rate_multiplier" />
      </div>

      <MonitorFirstTokenSamples v-if="detail.group_insight" :samples="detail.group_insight.recent_first_tokens ?? []" detailed />

      <div class="overflow-x-auto">
      <table class="w-full text-left text-sm">
        <thead class="border-b border-gray-200 dark:border-dark-700">
          <tr class="text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.model') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.latestStatus') }}</th>
            <th class="py-2 pr-3">{{ t('monitorCommon.probeFirstToken') }}</th>
            <th class="py-2 pr-3">{{ t('monitorCommon.outputRate') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability7d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability15d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability30d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.avgLatency7d') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="m in detail.models"
            :key="m.model"
            class="border-b border-gray-100 dark:border-dark-800"
          >
            <td class="py-2 pr-3 font-medium text-gray-900 dark:text-gray-100">{{ formatMonitorModel(m.model) }}</td>
            <td class="py-2 pr-3">
              <span
                class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px]"
                :class="statusBadgeClass(m.latest_status)"
              >
                {{ statusLabel(m.latest_status) }}
              </span>
            </td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatLatency(m.latest_first_token_ms) }}</td>
            <td class="py-2 pr-3 tabular-nums text-gray-700 dark:text-gray-300">{{ m.latest_output_tokens_per_second != null && m.latest_output_tokens_per_second > 0 ? `${m.latest_output_tokens_per_second.toFixed(1)} tok/s` : '—' }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(m.availability_7d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(m.availability_15d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(m.availability_30d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatLatency(m.avg_latency_7d_ms) }}</td>
          </tr>
        </tbody>
      </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('channelStatus.closeDetail') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  status as fetchChannelMonitorDetail,
  type UserMonitorDetail,
} from '@/api/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import MonitorFirstTokenSamples from './monitor/MonitorFirstTokenSamples.vue'
import MonitorRateHistory from './monitor/MonitorRateHistory.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { useAutoRefresh } from '@/composables/useAutoRefresh'

const props = defineProps<{
  show: boolean
  monitorId: number | null
  title: string
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()

function formatRate(value: number): string {
  return Number.isFinite(value) ? value.toFixed(4) : '—'
}


const appStore = useAppStore()
const { statusLabel, statusBadgeClass, formatLatency, formatPercent, formatMonitorModel } = useChannelMonitorFormat()

const detail = ref<UserMonitorDetail | null>(null)
const loading = ref(false)
let requestVersion = 0

async function load(id: number, silent = false) {
  const version = ++requestVersion
  if (!silent) detail.value = null
  loading.value = true
  try {
    const result = await fetchChannelMonitorDetail(id)
    if (version === requestVersion) detail.value = result
  } catch (err: unknown) {
    if (version !== requestVersion) return
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-monitor-detail-refresh',
  intervals: [5] as const,
  defaultInterval: 5,
  defaultEnabled: true,
  shouldPause: () => !props.show || document.hidden || loading.value,
  onRefresh: () => props.monitorId == null ? undefined : load(props.monitorId, true),
})

watch(
  () => [props.show, props.monitorId] as const,
  ([show, id], _, onCleanup) => {
    onCleanup(() => { requestVersion++ })
    if (!show) {
      autoRefresh.stop()
      detail.value = null
      return
    }
    autoRefresh.resetCountdown()
    autoRefresh.start()
    if (id != null) void load(id)
  },
  { immediate: true },
)
</script>
