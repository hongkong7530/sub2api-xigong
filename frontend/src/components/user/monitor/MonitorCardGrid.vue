<template>
  <div>
    <div
      v-if="loading && items.length === 0"
      class="grid gap-5 grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
    >
      <div
        v-for="i in 6"
        :key="i"
        class="p-5 rounded-2xl min-h-[280px] bg-white/70 dark:bg-dark-800/60 border border-gray-200/80 dark:border-dark-700/70 animate-pulse"
      >
        <div class="flex items-start gap-3">
          <div class="w-9 h-9 rounded-xl bg-gray-200 dark:bg-dark-700"></div>
          <div class="flex-1 space-y-2">
            <div class="h-4 w-2/3 rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-3 w-1/2 rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div class="h-6 w-16 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        </div>
        <div class="mt-5 grid grid-cols-2 gap-2">
          <div class="h-16 rounded-xl bg-gray-100 dark:bg-dark-900/40"></div>
          <div class="h-16 rounded-xl bg-gray-100 dark:bg-dark-900/40"></div>
        </div>
        <div class="mt-6 h-5 w-full rounded bg-gray-100 dark:bg-dark-900/40"></div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <div
      v-else
      class="grid gap-5 grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
    >
      <div v-for="item in orderedItems" :key="item.id" class="relative">
        <MonitorCard
          :item="item"
          :window="window"
          :availability-value="resolveAvailability(item)"
          :countdown-seconds="countdownSeconds"
          @click="emit('cardClick', item)"
        />
        <!-- 订阅入口叠加在卡片外层：MonitorCard 根元素是 button，不能再嵌套按钮 -->
        <div
          v-if="item.group_id && (showSubscribe || showReport)"
          class="absolute right-4 bottom-4 flex items-center gap-1"
        >
          <button
            v-if="showReport"
            type="button"
            class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold shadow-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/60"
            :class="isReported(item) ? 'bg-amber-600 text-white' : 'bg-white/90 text-gray-600 hover:bg-gray-100 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'"
            :disabled="isReported(item)"
            :title="isReported(item) ? t('monitorCommon.reported') : t('monitorCommon.report')"
            @click.stop="emit('report', item)"
          >
            <Icon name="exclamationCircle" size="xs" />
            <span>{{ isReported(item) ? t('monitorCommon.reported') : t('monitorCommon.report') }}</span>
          </button>
          <button
            v-if="showSubscribe"
            type="button"
            class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-semibold shadow-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/60"
            :class="isSubscribed(item) ? 'bg-primary-600 text-white hover:bg-primary-700' : 'bg-white/90 text-gray-600 hover:bg-gray-100 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'"
            :title="isSubscribed(item) ? t('monitorCommon.subscribed') : t('monitorCommon.subscribe')"
            @click.stop="emit('subscribe', item)"
          >
            <Icon name="bell" size="xs" />
            <span>{{ isSubscribed(item) ? t('monitorCommon.subscribed') : t('monitorCommon.subscribe') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import { sortMonitors } from '@/utils/monitorOrder'
import type { UserMonitorView, UserMonitorDetail, MonitorSubscription } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import MonitorCard from './MonitorCard.vue'

const orderedItems = computed(() => sortMonitors(props.items))
const props = withDefaults(defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
  /** 当前用户按 group_id 索引的订阅；未登录时传空对象并关闭入口。 */
  subscriptions?: Record<number, MonitorSubscription>
  showSubscribe?: boolean
  showReport?: boolean
  submittedGroupIds?: number[]
}>(), {
  subscriptions: () => ({}),
  showSubscribe: true,
  showReport: false,
  submittedGroupIds: () => [],
})

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
  (e: 'subscribe', item: UserMonitorView): void
  (e: 'report', item: UserMonitorView): void
}>()

function isSubscribed(item: UserMonitorView): boolean {
  if (!item.group_id) return false
  const sub = props.subscriptions[item.group_id]
  return !!sub && (sub.notify_first_token || sub.notify_rate_change)
}

function isReported(item: UserMonitorView): boolean {
  return !!item.group_id && props.submittedGroupIds.includes(item.group_id)
}

const { t } = useI18n()

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const groupRate = detail.group_availability?.[props.window === '15d' ? '15' : '30']
  if (detail.group_availability) return groupRate ?? null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
