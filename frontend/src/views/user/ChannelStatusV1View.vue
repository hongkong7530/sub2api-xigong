<template>
  <AppLayout>
    <MonitorHero
      :overall-status="overallStatus"
      :interval-seconds="DEFAULT_INTERVAL_SECONDS"
      :window="currentWindow"
      :cache-window-seconds="cacheRateWindowSeconds"
      :loading="loading"
      :notice="appStore.cachedPublicSettings?.channel_status_notice"
      :auto-refresh="autoRefresh"
      @update:window="handleWindowChange"
      @update:cache-window-seconds="handleCacheRateWindowChange"
      @refresh="refresh()"
    />

    <MonitorCardGrid
      :items="items"
      :window="currentWindow"
      :countdown-seconds="countdown"
      :loading="loading"
      :detail-cache="detailCache"
      :subscriptions="subscriptions"
      :show-subscribe="authStore.isAuthenticated"
      :show-report="authStore.isAuthenticated"
      :submitted-group-ids="submittedGroupIds"
      @card-click="openDetail"
      @subscribe="openSubscribe"
      @report="openReport"
    />

    <MonitorReportDialog
      :show="showReport"
      :group-id="reportTarget?.group_id ?? null"
      :group-name="reportTarget?.group_name ?? ''"
      @close="closeReport"
      @submitted="markSubmitted"
    />

    <MonitorSubscribeDialog
      :show="showSubscribe"
      :group-id="subscribeTarget?.group_id ?? null"
      :group-name="subscribeTarget?.group_name ?? ''"
      :has-email="!!authStore.user?.email"
      :current="subscribeTarget?.group_id ? subscriptions[subscribeTarget.group_id] ?? null : null"
      @close="closeSubscribe"
      @saved="applySubscription"
    />

    <MonitorDetailDialog
      :show="showDetail"
      :monitor-id="detailTarget?.id ?? null"
      :title="detailTitle"
      @close="closeDetail"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  DEFAULT_CACHE_RATE_WINDOW_SECONDS,
  list as listChannelMonitorViews,
  status as fetchChannelMonitorDetail,
  listSubscriptions as listMonitorSubscriptions,
  listSubmittedReports,
  type CacheRateWindowSeconds,
  type UserMonitorView,
  type UserMonitorDetail,
  type MonitorSubscription,
} from '@/api/channelMonitor'
import AppLayout from '@/components/layout/AppLayout.vue'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '@/components/user/monitor/MonitorHero.vue'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import MonitorSubscribeDialog from '@/components/user/monitor/MonitorSubscribeDialog.vue'
import MonitorReportDialog from '@/components/user/monitor/MonitorReportDialog.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '@/constants/channelMonitor'
import { useAutoRefresh } from '@/composables/useAutoRefresh'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

// ── State ──
const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const currentWindow = ref<MonitorWindow>('7d')
const CACHE_RATE_WINDOW_STORAGE_KEY = 'channel-status-cache-rate-window-seconds'
const CACHE_RATE_WINDOW_OPTIONS: readonly CacheRateWindowSeconds[] = [1800, 3600, 21600, 86400]
function loadCacheRateWindow(): CacheRateWindowSeconds {
  try {
    const saved = Number(localStorage.getItem(CACHE_RATE_WINDOW_STORAGE_KEY))
    if (CACHE_RATE_WINDOW_OPTIONS.includes(saved as CacheRateWindowSeconds)) return saved as CacheRateWindowSeconds
  } catch {
    // Storage can be unavailable in restricted browsing contexts.
  }
  return DEFAULT_CACHE_RATE_WINDOW_SECONDS
}
const cacheRateWindowSeconds = ref<CacheRateWindowSeconds>(loadCacheRateWindow())
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

// 订阅状态按 group_id 索引，列表加载时批量拉一次，避免每卡一请求。
const subscriptions = reactive<Record<number, MonitorSubscription>>({})
const submittedGroupIds = ref<number[]>([])
const showReport = ref(false)
const reportTarget = ref<UserMonitorView | null>(null)
const showSubscribe = ref(false)
const subscribeTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-hybrid-auto-refresh',
  intervals: [5, 10, 30, 60, 120] as const,
  defaultInterval: 5,
  defaultEnabled: true,
  onRefresh: () => refresh(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

// ── Computed ──
const overallStatus = computed<OverallStatus>(() => {
  if (items.value.length === 0) return 'operational'
  for (const it of items.value) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const detailTitle = computed(() => {
  return detailTarget.value?.name || t('channelStatus.detailTitle')
})

// ── Loaders ──
async function reload(silent = false) {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listChannelMonitorViews({ signal: ctrl.signal, cacheWindowSeconds: cacheRateWindowSeconds.value })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      if (!silent) loading.value = false
      autoRefresh.resetCountdown()
      abortController = null
    }
  }
}

async function refresh(silent = false) {
  await reload(silent)
  // After base reload, refresh any cached detail records so non-7d availability
  // values stay in sync without forcing the user to switch tabs again.
  if (currentWindow.value !== '7d') {
    await Promise.all(items.value.map(it => loadDetail(it.id, true)))
  }
}

async function loadSubscriptions() {
  if (!authStore.isAuthenticated) return
  try {
    const res = await listMonitorSubscriptions()
    for (const key of Object.keys(subscriptions)) delete subscriptions[Number(key)]
    for (const sub of res.items || []) subscriptions[sub.group_id] = sub
  } catch {
    // 订阅状态属于增强信息，加载失败不打断监控列表。
  }
}

async function loadSubmittedReports() {
  if (!authStore.isAuthenticated) return
  try {
    const res = await listSubmittedReports()
    submittedGroupIds.value = res.group_ids || []
  } catch {
    submittedGroupIds.value = []
  }
}

function openReport(row: UserMonitorView) {
  if (!row.group_id || submittedGroupIds.value.includes(row.group_id)) return
  reportTarget.value = row
  showReport.value = true
}

function closeReport() {
  showReport.value = false
  reportTarget.value = null
}

function markSubmitted(groupId: number) {
  if (!submittedGroupIds.value.includes(groupId)) submittedGroupIds.value = [...submittedGroupIds.value, groupId]
}

function openSubscribe(row: UserMonitorView) {
  if (!row.group_id) return
  subscribeTarget.value = row
  showSubscribe.value = true
}

function closeSubscribe() {
  showSubscribe.value = false
  subscribeTarget.value = null
}

function applySubscription(sub: MonitorSubscription) {
  if (!sub.notify_first_token && !sub.notify_rate_change) {
    delete subscriptions[sub.group_id]
    return
  }
  subscriptions[sub.group_id] = sub
}

async function loadDetail(id: number, force = false) {
  if (!force && detailCache[id]) return
  try {
    detailCache[id] = await fetchChannelMonitorDetail(id)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  }
}

async function ensureDetailsForWindow() {
  if (currentWindow.value === '7d') return
  await Promise.all(items.value.map(it => loadDetail(it.id)))
}

// ── Handlers ──
async function handleWindowChange(value: MonitorWindow) {
  currentWindow.value = value
  await ensureDetailsForWindow()
}

async function handleCacheRateWindowChange(value: CacheRateWindowSeconds) {
  if (!CACHE_RATE_WINDOW_OPTIONS.includes(value)) return
  cacheRateWindowSeconds.value = value
  try {
    localStorage.setItem(CACHE_RATE_WINDOW_STORAGE_KEY, String(value))
  } catch {
    // The current selection remains active for this page even without storage.
  }
  await reload(false)
}

function openDetail(row: UserMonitorView) {
  detailTarget.value = row
  showDetail.value = true
}

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
}

watch(items, () => {
  void ensureDetailsForWindow()
})

watch(
  () => appStore.cachedPublicSettings?.channel_monitor_enabled,
  (enabled) => {
    if (enabled === false) autoRefresh.stop()
    else if (autoRefresh.enabled.value) autoRefresh.start()
  },
)

onMounted(() => {
  void reload(false)
  void loadSubscriptions()
  void loadSubmittedReports()
  if (appStore.cachedPublicSettings?.channel_monitor_enabled !== false) {
    autoRefresh.setEnabled(autoRefresh.enabled.value)
  }
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
})
</script>
