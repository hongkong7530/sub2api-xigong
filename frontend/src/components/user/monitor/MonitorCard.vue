<template>
  <component
    :is="readOnly ? 'article' : 'button'"
    :type="readOnly ? undefined : 'button'"
    :class="{ 'monitor-card--home': appearance === 'home', 'monitor-card--moving': moving }"
    :data-status="overallStatus"
    class="monitor-card group text-left p-5 rounded-2xl min-h-[310px] w-full bg-white/70 backdrop-blur-xl border border-gray-200/80 shadow-card dark:bg-dark-800/60 dark:border-dark-700/70 hover:-translate-y-1 hover:shadow-card-hover dark:hover:border-primary-500/30 hover:border-gray-300 transition-all duration-300 ease-out flex flex-col focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/60 focus-visible:ring-offset-2"
    @click="!readOnly && emit('click')"
  >
    <span v-if="appearance === 'home'" class="monitor-card__halo" aria-hidden="true" />
    <span v-if="appearance === 'home'" class="monitor-card__glint" aria-hidden="true" />
    <!-- Header: icon + name/model + status chip -->
    <div class="monitor-card__header flex items-start gap-3">
      <span
        class="w-9 h-9 rounded-xl ring-1 ring-black/5 dark:ring-white/10 grid place-items-center flex-shrink-0"
        :class="[providerGradient(item.provider), providerTintClass]"
      >
        <ProviderIcon :provider="item.provider" :size="20" />
      </span>
      <div class="flex-1 min-w-0">
        <h3 :title="item.name" class="monitor-card__name text-base font-semibold truncate text-gray-900 dark:text-gray-100">
          {{ item.name }}
        </h3>
        <div class="monitor-card__identity mt-0.5 flex items-center gap-1.5 min-w-0">
          <span
            class="inline-flex items-center rounded-md px-2 py-1 text-xs font-semibold flex-shrink-0"
            :class="providerBadgeClass(item.provider)"
          >
            {{ providerLabel(item.provider) }}
          </span>
          <!-- 纯配额模式主模型是占位符 "quota"，展示层替换为本地化「配额」标签 -->
          <span class="monitor-card__model font-mono text-sm truncate text-gray-600 dark:text-gray-300">
            {{ formatMonitorModel(item.primary_model) }}
          </span>
          <span
            v-if="item.group_name"
            class="inline-flex items-center rounded-md px-2 py-1 text-xs font-semibold bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200 flex-shrink-0"
          >
            {{ item.group_name }}
          </span>
        </div>
      </div>
      <span
        class="monitor-card__status px-2.5 py-1 rounded-full text-xs font-semibold flex-shrink-0"
        :class="statusBadgeClass(overallStatus)"
      >
        {{ statusLabel(overallStatus) }}
      </span>
    </div>

    <!-- 缺失首字不得用总耗时冒充；真实请求样本与主动探测分开展示。 -->
    <MonitorMetricPair
      primary-icon="bolt"
      :primary-label="t(item.status_source === 'user_request' ? 'monitorCommon.realFirstTokenLatest' : 'monitorCommon.probeFirstToken')"
      :primary-value="formatLatency(item.primary_first_token_ms)"
      primary-unit="ms"
      secondary-icon="bolt"
      :secondary-label="t('monitorCommon.outputRate')"
      :secondary-hint="t('monitorCommon.outputRateHint')"
      :secondary-value="formatOutputRate(item.primary_output_tokens_per_second)"
      secondary-unit="tok/s"
    />

    <!-- 分组实时指标：倍率 + 缓存率（分组绑定监控才有） -->
    <div
      v-if="item.group_insight"
      class="monitor-card__insight mt-3 grid gap-2"
      :class="item.group_insight.peak_rate_enabled ? 'grid-cols-1 sm:grid-cols-3' : 'grid-cols-2'"
    >
      <span class="flex flex-col rounded-lg bg-sky-50 px-3 py-2.5 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300">
        <span class="text-xs font-medium opacity-75">{{ t('monitorCommon.groupRate') }}</span>
        <strong class="mt-0.5 text-base font-black tabular-nums">×{{ formatRate(item.group_insight.effective_rate_multiplier) }}</strong>
      </span>
      <span
        v-if="item.group_insight.peak_rate_enabled"
        class="flex flex-col rounded-lg bg-amber-50 px-3 py-2.5 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
      >
        <span class="text-xs font-medium opacity-75">{{ t('monitorCommon.groupPeak') }}</span>
        <strong class="mt-0.5 text-base font-black tabular-nums">×{{ formatRate(item.group_insight.peak_rate_multiplier) }}</strong>
      </span>
      <span :title="t('monitorCommon.cacheSamplesHint', { n: item.group_insight.cache_sample_count ?? 0 })" class="flex flex-col rounded-lg bg-emerald-50 px-3 py-2.5 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">
        <span class="text-xs font-medium opacity-75">{{ t('monitorCommon.groupCache') }} · {{ cacheWindowLabel }}</span>
        <strong class="mt-0.5 text-base font-black tabular-nums">{{ item.group_insight.cache_hit_rate == null ? '—' : `${item.group_insight.cache_hit_rate.toFixed(1)}%` }}</strong>
      </span>
    </div>

    <MonitorFirstTokenSamples v-if="item.group_id" :samples="item.group_insight?.recent_first_tokens ?? []" />

    <!-- 配额模式：最新用量/余额快照（服务端已按系统开关剥离，此处 flag 为纵深防御） -->
    <MonitorQuotaView v-if="quotaVisible" :snapshot="item.latest_quota" class="mt-2" />

    <!-- Divider -->
    <div class="monitor-card__divider mt-4 border-t border-gray-100 dark:border-dark-700/60"></div>

    <!-- Availability row -->
    <MonitorAvailabilityRow
      :window-label="availabilityLabel"
      :value="availabilityValue"
      :samples-label="extraModelsCountLabel"
    />

    <!-- Timeline -->
    <MonitorTimeline
      :buckets="item.timeline"
      :countdown-seconds="countdownSeconds"
    />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import '@/styles/monitor-contrast.css'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import {
  useChannelMonitorFormat,
  providerGradient,
} from '@/composables/useChannelMonitorFormat'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import ProviderIcon from './ProviderIcon.vue'
import MonitorMetricPair from './MonitorMetricPair.vue'
import MonitorAvailabilityRow from './MonitorAvailabilityRow.vue'
import MonitorTimeline from './MonitorTimeline.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import MonitorFirstTokenSamples from './MonitorFirstTokenSamples.vue'

// 图标配色与 utils/platformColors.ts 的平台色对齐（新 4 家）。
const PROVIDER_TINT: Record<string, string> = {
  openai: 'text-emerald-600 dark:text-emerald-300',
  anthropic: 'text-orange-600 dark:text-orange-300',
  gemini: 'text-sky-600 dark:text-sky-300',
  grok: 'text-zinc-700 dark:text-zinc-200',
  antigravity: 'text-purple-600 dark:text-purple-300',
  kimi: 'text-pink-600 dark:text-pink-300',
  zhipu: 'text-indigo-600 dark:text-indigo-300',
  deepseek: 'text-teal-600 dark:text-teal-300',
  opencode_go: 'text-amber-700 dark:text-amber-300',
}

const props = withDefaults(defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  countdownSeconds: number
  appearance?: 'default' | 'home'
  readOnly?: boolean
  moving?: boolean
}>(), { appearance: 'default', readOnly: false, moving: false })

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()

// 整卡颜色按全部被监控模型最优状态（一绿即绿）；旧响应无 overall_status 时回退主模型状态。
const overallStatus = computed(() => props.item.overall_status || props.item.primary_status)

// 最新样本没有完整usage时保持未知，不用历史速度冒充当前速度。
function formatOutputRate(value: number | null | undefined): string {
  return value != null && Number.isFinite(value) && value > 0 ? value.toFixed(1) : '—'
}

function formatRate(value: number): string {
  return Number.isFinite(value) ? value.toFixed(4) : '—'
}
const {
  statusLabel,
  statusBadgeClass,
  providerLabel,
  providerBadgeClass,
  formatLatency,
  formatMonitorModel,
} = useChannelMonitorFormat()

const providerTintClass = computed(() =>
  PROVIDER_TINT[props.item.provider] ?? 'text-gray-500 dark:text-gray-300'
)

const cacheWindowLabel = computed(() => {
  const seconds = props.item.group_insight?.cache_window_seconds ?? 1800
  const key = seconds === 3600 ? '1h' : seconds === 21600 ? '6h' : seconds === 86400 ? '24h' : '30m'
  return t(`monitorCommon.cacheRateWindowTab.${key}`)
})

const quotaVisible = computed(
  () => isChannelMonitorQuotaVisible() && !!props.item.latest_quota
)

const availabilityLabel = computed(() => {
  const win = t(`channelStatus.windowTab.${props.window}`)
  return `${t('monitorCommon.availabilityPrefix')} · ${win}`
})

const extraModelsCountLabel = computed(() => {
  const count = props.item.extra_models?.length ?? 0
  if (count === 0) return undefined
  return t('monitorCommon.extraModelsCount', { n: count })
})
</script>
