<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import * as api from '@/api/admin/cluster'
import type { ClusterNodeConfig, ClusterNodeView, ClusterOperation, ClusterOverview, ClusterStorage, RetentionPreview } from '@/api/admin/cluster'

const emit = defineEmits<{ (event: 'enabled', enabled: boolean): void }>()
const { t } = useI18n()
const tabs = ['overview', 'nodes', 'capacity', 'storage', 'operations'] as const
const tab = ref<(typeof tabs)[number]>('overview')
const overview = ref<ClusterOverview | null>(null)
const operations = ref<ClusterOperation[]>([])
const storage = ref<ClusterStorage | null>(null)
const preview = ref<RetentionPreview | null>(null)
const error = ref('')
const notice = ref('')
const recoveryKey = 'cluster-v3-pending-operation'
const pendingRequestKey = ref(sessionStorage.getItem(recoveryKey) ?? '')
let pendingSubmission: { nodeId: string; change: Parameters<typeof api.changeNode>[1] } | undefined
async function submitChange(nodeId: string, change: Omit<Parameters<typeof api.changeNode>[1], 'request_key'>) {
  const submission = { nodeId, change: { ...change, request_key: crypto.randomUUID() } }
  pendingSubmission = submission; pendingRequestKey.value = submission.change.request_key
  sessionStorage.setItem(recoveryKey, pendingRequestKey.value)
  try {
    const op = await api.changeNode(submission.nodeId, submission.change)
    clearPending(); return op
  } catch (err: any) {
    // A definitive client rejection clears the key so a stale lock cannot outlive the
    // answer; unknown outcomes keep it.
    if (httpStatus(err) !== undefined && httpStatus(err)! >= 400 && httpStatus(err)! < 500) clearPending()
    throw err
  }
}
function clearPending() { pendingSubmission = undefined; pendingRequestKey.value = ''; sessionStorage.removeItem(recoveryKey) }
// An unresolved request key blocks every mutation. Returning silently made the
// Configure button look dead, so the reason is stated and the operator can clear
// the lock right here: a key the authority never recorded means nothing executed.
let autoProbedKey = ''
// A key the authority does not know is a dead lock, and leaving it for the operator to
// clear by hand left the console unusable after an upgrade. Probe it once per key on
// refresh and release the lock as soon as the authority answers "no such operation".
async function autoResolvePending() {
  const key = pendingRequestKey.value
  if (!key || key === autoProbedKey || busy.value || pendingSubmission) return
  autoProbedKey = key
  try {
    const op = await api.getOperation(key)
    notice.value = `${t('clusterV3.saved')} ${op.id}`; clearPending()
  } catch (err: any) {
    const status = httpStatus(err)
    if (status === 404 || status === 400) {
      notice.value = `${t('clusterV3.operationNotRecorded')} ${key}`
      clearPending()
    }
    // Anything else genuinely may have executed; keep the lock and stay silent here so
    // the operator still sees the original uncertainty notice.
  }
}
function blockedByPending(): boolean {
  if (!pendingRequestKey.value) return false
  // A lock notice must not be written into `error`: that field also drives the stale
  // computation, so stating the reason blanked every metric on the page as "unknown".
  notice.value = t('clusterV3.blockedByPending')
  return true
}
// The shared client flattens errors to { status, code, message } and does not keep an
// axios `response`, so status must be read from the flattened shape.
function httpStatus(err: any): number | undefined {
  const direct = err?.status ?? err?.response?.status
  return typeof direct === 'number' ? direct : undefined
}
async function recoverOperation(retry = false) {
  if (busy.value || !pendingRequestKey.value) return
  busy.value = true
  try {
    const op = retry && pendingSubmission
      ? await api.changeNode(pendingSubmission.nodeId, pendingSubmission.change)
      : await api.getOperation(pendingRequestKey.value)
    notice.value = `${t('clusterV3.saved')} ${op.id}`; clearPending(); error.value = ''; editing.value = null; confirmation.value = null
    await refresh()
  } catch (err: any) {
    // A 4xx is a definitive answer, not an unknown one: no side effect happened, so
    // holding the key would lock every control on the page forever. 404 means the
    // authority never recorded the request; other 4xx means it rejected it outright.
    // The request id stays in the change log, so the operator can still verify absence.
    const status = httpStatus(err)
    if (status === 404 || status === 400) {
      notice.value = `${t('clusterV3.operationNotRecorded')} ${pendingRequestKey.value}`
      clearPending(); error.value = ''; await refresh(); return
    }
    if (status !== undefined && status >= 400 && status < 500) {
      clearPending(); error.value = t('clusterV3.operationFailed'); await refresh(); return
    }
    error.value = t('clusterV3.operationUncertain')
  } finally { busy.value = false }
}
const lastReceived = ref<Date | null>(null)
const now = ref(Date.now())
const busy = ref(false)
const loading = ref(false)
const enabled = ref(false)
let controller: AbortController | undefined
let sequence = 0
let timer: ReturnType<typeof setInterval> | undefined
const stale = computed(() => !!overview.value && (Boolean(error.value) || !lastReceived.value || now.value - lastReceived.value.getTime() > 15000))
const nodes = computed(() => overview.value?.nodes ?? [])
const databaseCapacity = computed(() => overview.value?.database_capacity)
const maxDraftConnections = computed(() => {
  if (!databaseCapacity.value) return undefined
  const others = nodes.value.filter(n => n.config.state !== 'retired' && n.config.node_id !== editing.value?.config.node_id).reduce((sum, n) => sum + n.config.db_connections, 0)
  return Math.max(0, databaseCapacity.value.budget - others)
})
const budgetExceeded = computed(() => maxDraftConnections.value !== undefined && Number(draft.db_connections) > maxDraftConnections.value)
const validNodes = computed(() => nodes.value.filter(node => node.config.state !== 'retired' && node.fresh && !stale.value))
const accepting = computed(() => nodes.value.some(node => node.config.state !== 'retired' && node.config.state !== 'pending' && (!node.fresh || !node.status)) ? t('clusterV3.unknown') : validNodes.value.filter(node => node.status?.admission_ready).length)
const total = (field: 'active' | 'waiting' | 'pending_settlement') => {
  const relevant = nodes.value.filter(node => node.config.state !== 'retired' && node.config.state !== 'pending')
  if (stale.value || relevant.some(node => !node.fresh || !node.status)) return t('clusterV3.unknown')
  return relevant.reduce((sum, node) => sum + (node.status?.[field] ?? 0), 0)
}
const fmtTime = (value?: string | Date | null) => value ? new Date(value).toLocaleString() : t('clusterV3.unknown')
function bytes(value?: number | null) {
  if (value == null || !Number.isFinite(value)) return t('clusterV3.unknown')
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let index = 0
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index++ }
  return `${value.toFixed(index ? 1 : 0)} ${units[index]}`
}
function nodeState(node: ClusterNodeView) { return t(`clusterV3.${node.config.state === 'pending' ? 'registered' : node.config.state}`) }
function known(node: ClusterNodeView, field: 'active' | 'waiting' | 'pending_settlement') { return node.fresh && !stale.value && node.status ? node.status[field] : t('clusterV3.unknown') }
async function refresh() {
  if (document.hidden) return
  controller?.abort(); controller = new AbortController(); const signal = controller.signal; const run = ++sequence
  loading.value = true
  try {
    const snapshot = await api.getOverview(signal)
    if (run !== sequence) return
    overview.value = snapshot; enabled.value = true; emit('enabled', true); error.value = ''; lastReceived.value = new Date()
    if (tab.value === 'operations') { const value = await api.getOperations(signal); if (run === sequence) operations.value = value }
    if (tab.value === 'storage') { const value = await api.getStorage(signal); if (run === sequence) storage.value = value }
    await autoResolvePending()
  } catch (err: any) {
    if (run !== sequence || signal.aborted) return
    // Errors arrive flattened by the shared client, so the status is read from `status`.
    if (!overview.value && httpStatus(err) === 404) { enabled.value = false; emit('enabled', false); return }
    error.value = t('clusterV3.fetchFailed')
  } finally { if (run === sequence) loading.value = false }
}
function visibility() { if (document.hidden) { sequence++; controller?.abort(); loading.value = false } else void refresh() }
onMounted(() => { void refresh(); timer = setInterval(() => { now.value = Date.now(); void refresh() }, 5000); document.addEventListener('visibilitychange', visibility) })
onUnmounted(() => { sequence++; controller?.abort(); if (timer) clearInterval(timer); document.removeEventListener('visibilitychange', visibility); token.value = '' })
watch(tab, () => { preview.value = null; void refresh() })

const editing = ref<ClusterNodeView | null>(null)
const registering = ref(false)
const token = ref('')
const draft = reactive<ClusterNodeConfig>({ cluster_id: '', node_id: '', name: '', roles: ['worker'], capabilities: ['http'], address: '', certificate_sha256: '', state: 'pending', revision: 1, base_weight: 1, max_in_flight: 32, max_queue: 8, db_connections: 32, monthly_limit_bytes: null, traffic_reserve_bytes: 0, traffic_cycle_day: 1, traffic_timezone: 'UTC', traffic_source: 'unknown', version: '' })
const monthlyGiB = ref<number | null>(null)
const reserveGiB = ref(0)
const rolesText = ref('worker')
const capabilitiesText = ref('http')
const editorOpen = computed(() => registering.value || editing.value !== null)
function openRegistration() {
  if (blockedByPending()) return
  Object.assign(draft, { cluster_id: '', node_id: '', name: '', roles: ['worker'], capabilities: ['http'], address: '', certificate_sha256: '', state: 'pending', revision: 1, base_weight: 1, max_in_flight: 32, max_queue: 8, db_connections: 32, monthly_limit_bytes: null, traffic_reserve_bytes: 0, traffic_cycle_day: 1, traffic_timezone: 'UTC', traffic_source: 'unknown', version: '' })
  rolesText.value = 'worker'; capabilitiesText.value = 'http'; monthlyGiB.value = null; reserveGiB.value = 0; token.value = ''; registering.value = true
}
function openEdit(node: ClusterNodeView) { if (blockedByPending()) return; editing.value = node; Object.assign(draft, JSON.parse(JSON.stringify(node.config))); monthlyGiB.value = draft.monthly_limit_bytes == null ? null : draft.monthly_limit_bytes / 1073741824; reserveGiB.value = draft.traffic_reserve_bytes / 1073741824 }
function closeEditor() { if (busy.value) return; registering.value = false; editing.value = null; token.value = '' }
function operationError(err: any) { notice.value = ''; error.value = httpStatus(err) === 409 ? t('clusterV3.conflict') : httpStatus(err) === 400 && typeof err?.message === 'string' ? err.message : t('clusterV3.operationFailed') }
async function save() {
  if (busy.value || pendingRequestKey.value || budgetExceeded.value) return
  busy.value = true; error.value = ''
  try {
    draft.monthly_limit_bytes = monthlyGiB.value == null || String(monthlyGiB.value) === '' ? null : Math.round(Number(monthlyGiB.value) * 1073741824)
    draft.traffic_reserve_bytes = Math.round(Number(reserveGiB.value) * 1073741824)
    if (registering.value) {
      draft.roles = rolesText.value.split(',').map(s => s.trim()).filter(Boolean); draft.capabilities = capabilitiesText.value.split(',').map(s => s.trim()).filter(Boolean)
      const result = await api.registerNode({ ...draft }); token.value = result.join_token
    } else if (editing.value) {
      const op = await submitChange(editing.value.config.node_id, { expected_revision: editing.value.config.revision, action: 'configure', node: { ...draft } })
      notice.value = `${t('clusterV3.saved')} ${op.id}`; editing.value = null
    }
    await refresh()
  } catch (err) { operationError(err) } finally { busy.value = false }
}
const confirmation = ref<{ node: ClusterNodeView | null; action: string } | null>(null)
const typedIdentity = ref('')
function requestAction(node: ClusterNodeView | null, action: string) { if (blockedByPending()) return; typedIdentity.value = ''; confirmation.value = { node, action } }
const mayConfirm = computed(() => confirmation.value?.action !== 'force_retire' || typedIdentity.value === confirmation.value.node?.config.node_id)
async function confirmAction() {
  const pending = confirmation.value; if (!pending || busy.value || pendingRequestKey.value || !mayConfirm.value) return
  busy.value = true; error.value = ''
  try {
    if (pending.action === 'redis') await api.acknowledgeRedis(overview.value!.redis_generation)
    else if (pending.node) {
      const op = await submitChange(pending.node.config.node_id, { expected_revision: pending.node.config.revision, action: pending.action, confirm_force: pending.action === 'force_retire' })
      notice.value = `${t('clusterV3.saved')} ${op.id}`
    }
    confirmation.value = null; await refresh()
  } catch (err) { operationError(err) } finally { busy.value = false }
}
const retentionTable = ref('usage_logs')
const retentionDays = ref(30)
async function loadPreview() {
  if (busy.value) return
  busy.value = true; preview.value = null
  try { preview.value = await api.previewRetention(retentionTable.value, retentionDays.value) } catch { error.value = t('clusterV3.storageError') } finally { busy.value = false }
}
</script>

<template>
  <section v-if="enabled || error" class="card p-4 sm:p-6" :aria-label="t('clusterV3.title')">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div><h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('clusterV3.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('clusterV3.subtitle') }}</p></div>
      <div class="flex flex-wrap items-center gap-2"><span class="rounded-full bg-blue-50 px-3 py-1 text-xs text-blue-700 dark:bg-blue-950 dark:text-blue-200">{{ overview ? t(overview.shadow ? 'clusterV3.shadow' : 'clusterV3.activeController') : t('clusterV3.unknown') }}</span><button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ t('clusterV3.refresh') }}</button><button class="btn btn-primary" :disabled="!overview || stale" @click="openRegistration">{{ t('clusterV3.addNode') }}</button></div>
    </header>
    <p v-if="lastReceived" class="mt-2 text-xs text-gray-500">{{ t('clusterV3.lastUpdated', { time: fmtTime(lastReceived) }) }} <span v-if="stale" class="font-semibold text-amber-600"> · {{ t('clusterV3.stale') }}</span></p>
    <p v-if="error" role="alert" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">{{ error }}</p>
    <p v-if="notice" role="status" class="mt-3 break-all text-sm text-blue-700 dark:text-blue-300">{{ notice }}</p>
    <div v-if="overview?.redis_recovery_required" class="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200"><span>{{ t('clusterV3.redisHeld') }}</span><button class="btn btn-secondary" :disabled="stale || busy" @click="requestAction(null, 'redis')">{{ t('clusterV3.acknowledge') }}</button></div>
    <div v-if="pendingRequestKey" role="status" class="mt-3 rounded-lg border border-amber-300 p-3 text-sm"><p>{{ t('clusterV3.operationUncertain') }}</p><p class="mt-1 break-all font-mono">{{ pendingRequestKey }}</p><div class="mt-2 flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="recoverOperation()">{{ t('clusterV3.queryOperation') }}</button><button v-if="pendingSubmission" class="btn btn-secondary" :disabled="busy" @click="recoverOperation(true)">{{ t('clusterV3.retrySameOperation') }}</button></div></div>
    <nav class="mt-5 flex gap-1 overflow-x-auto border-b border-gray-200 pb-2 dark:border-dark-600" :aria-label="t('clusterV3.title')"><button v-for="item in tabs" :key="item" class="shrink-0 rounded-lg px-3 py-2 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500" :class="tab === item ? 'bg-blue-50 font-semibold text-blue-700 dark:bg-blue-950 dark:text-blue-200' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'" :aria-current="tab === item ? 'page' : undefined" @click="tab = item">{{ t(`clusterV3.${item}`) }}</button></nav>
    <div v-if="tab === 'overview'" class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div v-for="metric in [{ label: 'online', value: validNodes.length }, { label: 'accepting', value: accepting }, { label: 'active', value: total('active') }, { label: 'pending', value: total('pending_settlement') }]" :key="metric.label" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"><p class="text-sm text-gray-500">{{ t(`clusterV3.${metric.label}`) }}</p><p class="mt-2 text-2xl font-semibold tabular-nums">{{ stale ? t('clusterV3.unknown') : metric.value }}</p></div>
      <div class="rounded-lg bg-gray-50 p-4 sm:col-span-2 lg:col-span-4 dark:bg-dark-900"><h3 class="font-medium">{{ t('clusterV3.singlePrimary') }}</h3><p class="mt-1 text-sm text-gray-500">{{ t('clusterV3.noReplica') }}</p></div>
    </div>
    <div v-if="databaseCapacity" class="mt-4 rounded-xl border border-primary-300/40 p-4 text-sm" data-testid="database-capacity">
      <h3 class="font-semibold">{{ t('clusterV3.databaseBudgetTitle') }}</h3>
      <p class="mt-2">{{ t('clusterV3.databaseBudgetSummary', { allocated: databaseCapacity.allocated, budget: databaseCapacity.budget, available: databaseCapacity.available }) }}</p>
      <p class="mt-1 text-xs text-gray-500">{{ t('clusterV3.databaseServerSummary', { used: databaseCapacity.server_used ?? '—', max: databaseCapacity.server_max ?? '—', reserved: databaseCapacity.server_reserved ?? '—' }) }}</p>
      <p class="mt-2 text-xs text-gray-500">{{ t('clusterV3.databaseBudgetHelp') }}</p>
    </div>
    <div v-if="tab === 'nodes' || tab === 'capacity'" class="mt-4 space-y-3">
      <p v-if="nodes.length === 0" class="py-8 text-center text-sm text-gray-500">{{ t('clusterV3.empty') }}</p>
      <article v-for="node in nodes" :key="node.config.node_id" :data-current="node.config.node_id === overview?.response_node_id" class="rounded-lg border p-4" :class="node.config.node_id === overview?.response_node_id ? 'border-primary-500 ring-1 ring-primary-500/30 bg-primary-50/40 dark:bg-primary-900/15' : 'border-gray-200 dark:border-dark-600'">
        <span v-if="node.config.node_id === overview?.response_node_id" class="mb-2 inline-block rounded-full bg-primary-100 px-2 py-1 text-xs text-primary-800 dark:bg-primary-900 dark:text-primary-100">{{ t('clusterV3.currentResponse') }}</span>
        <div class="flex flex-wrap items-start justify-between gap-3"><div><h3 class="font-semibold">{{ node.config.name }} <span class="ml-2 text-xs font-normal text-gray-500">{{ node.config.node_id }}</span></h3><div class="mt-2 flex flex-wrap gap-2"><span v-for="role in node.config.roles" :key="role" class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ t(`clusterV3.${role}`) }}</span><span class="text-xs" :class="node.fresh && !stale ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'">{{ t(node.fresh && !stale ? 'clusterV3.fresh' : 'clusterV3.stale') }}</span><span class="text-xs">{{ t('clusterV3.desiredState') }}: {{ nodeState(node) }}</span><span class="text-xs">{{ t('clusterV3.state') }}: {{ !node.fresh || stale || !node.status ? t('clusterV3.unknown') : t(node.status.admission_ready ? 'clusterV3.ready' : 'clusterV3.paused') }}</span></div></div><button class="btn btn-secondary" :disabled="busy || stale || node.config.state === 'retired'" @click="openEdit(node)">{{ t('clusterV3.configure') }}</button></div>
        <p v-if="node.config.apply_error" class="mt-2 text-sm text-red-600">{{ t('clusterV3.error') }}: {{ node.config.apply_error }}</p>
        <p v-if="node.status?.admission_blocked_by" class="mt-2 text-sm text-amber-600">{{ t('clusterV3.admissionBlocked') }}: {{ node.status.admission_blocked_by }}</p>
        <dl class="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4"><div><dt class="text-gray-500">{{ t('clusterV3.active') }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ known(node, 'active') }} / {{ node.config.max_in_flight }}</dd></div><div><dt class="text-gray-500">{{ t('clusterV3.waiting') }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ known(node, 'waiting') }} / {{ node.config.max_queue }}</dd></div><div><dt class="text-gray-500">{{ t('clusterV3.pending') }}</dt><dd class="mt-1 font-semibold">{{ known(node, 'pending_settlement') }}</dd></div><div><dt class="text-gray-500">{{ t('clusterV3.configState') }}</dt><dd class="mt-1">{{ t(node.fresh && !stale && node.status?.observed_revision === node.config.revision && node.applied_revision === node.config.revision ? 'clusterV3.applied' : 'clusterV3.pendingApply') }} · r{{ node.status?.observed_revision ?? '—' }}/{{ node.config.revision }}</dd></div></dl>
        <div v-if="tab === 'capacity'" class="mt-4 grid gap-3 border-t border-gray-100 pt-3 text-sm sm:grid-cols-3 dark:border-dark-700"><p>{{ t('clusterV3.dbConnections') }}: {{ node.config.db_connections }}</p><p>{{ t('clusterV3.baseWeight') }}: {{ node.config.base_weight }} · {{ t('clusterV3.suggested') }}: {{ node.suggested_weight }}</p><p>{{ t('clusterV3.consumed') }}: {{ bytes(node.status?.cycle_traffic_bytes) }} / {{ bytes(node.config.monthly_limit_bytes) }}</p><p class="text-xs text-gray-500 sm:col-span-3">{{ t('clusterV3.trafficCaveat') }}</p></div>
        <div v-else class="mt-4 flex flex-wrap gap-2"><button v-if="node.config.state !== 'retired'" class="btn btn-secondary" :disabled="busy || stale" @click="requestAction(node, 'reapply')">{{ t('clusterV3.reapply') }}</button><button v-if="node.config.state === 'pending'" class="btn btn-primary" :disabled="busy || stale || !node.fresh" @click="requestAction(node, 'activate')">{{ t('clusterV3.activate') }}</button><button v-if="node.config.state === 'ready'" class="btn btn-secondary" :disabled="busy || stale" @click="requestAction(node, 'pause')">{{ t('clusterV3.pause') }}</button><button v-if="node.config.state === 'paused'" class="btn btn-primary" :disabled="busy || stale || !node.fresh" @click="requestAction(node, 'resume')">{{ t('clusterV3.resume') }}</button><button v-if="['ready','paused','pending'].includes(node.config.state)" class="btn btn-secondary" :disabled="busy || stale" @click="requestAction(node, 'drain')">{{ t('clusterV3.drain') }}</button><button v-if="node.config.state === 'draining'" class="btn btn-secondary" :disabled="busy || stale || !node.fresh" @click="requestAction(node, 'retire')">{{ t('clusterV3.retire') }}</button><button v-if="node.config.state !== 'retired'" class="rounded px-3 py-2 text-sm text-red-600 hover:bg-red-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-red-500 dark:text-red-300 dark:hover:bg-red-950" :disabled="busy || stale" @click="requestAction(node, 'force_retire')">{{ t('clusterV3.force_retire') }}</button></div>
      </article>
    </div>
    <div v-if="tab === 'operations'" class="mt-4 space-y-3"><p v-if="operations.length === 0" class="py-8 text-center text-gray-500">{{ t('clusterV3.noOperations') }}</p><article v-for="op in operations" :key="op.id" class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600"><div class="flex flex-wrap justify-between gap-2"><strong>{{ op.node_id }} · {{ t(`clusterV3.${op.action}`) }}</strong><span>{{ t(`clusterV3.${op.status === 'pending' ? 'pendingApply' : op.status}`) }}</span></div><p class="mt-2 break-all font-mono text-xs text-gray-500">{{ op.id }}</p><p class="mt-1 text-xs">{{ fmtTime(op.created_at) }} · r{{ op.target_revision }} <span v-if="op.error_code"> · {{ op.error_code }}</span></p></article></div>
    <div v-if="tab === 'storage'" class="mt-4 space-y-4"><template v-if="storage"><div class="flex flex-wrap justify-between gap-3"><p>{{ t('clusterV3.totalDatabase') }} <strong>{{ bytes(storage.database_bytes) }}</strong></p><p class="text-sm" :class="storage.ledger_detached ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-600'">{{ t(storage.ledger_detached ? 'clusterV3.ledgerProtected' : 'clusterV3.ledgerUnsafe') }}</p></div><div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr class="border-b border-gray-200 dark:border-dark-600"><th class="p-2">{{ t('clusterV3.table') }}</th><th class="p-2">{{ t('clusterV3.bytes') }}</th><th class="p-2">{{ t('clusterV3.rows') }}</th><th class="p-2">{{ t('clusterV3.storage') }}</th></tr></thead><tbody><tr v-for="row in storage.tables" :key="row.name" class="border-b border-gray-100 dark:border-dark-700"><td class="p-2 font-mono text-xs">{{ row.name }}</td><td class="p-2">{{ bytes(row.bytes) }}</td><td class="p-2 tabular-nums">{{ row.estimated_rows.toLocaleString() }}</td><td class="p-2">{{ t(`clusterV3.${row.retention_class}`) }}</td></tr></tbody></table></div></template><form class="flex flex-wrap items-end gap-3" @submit.prevent="loadPreview"><label class="text-sm">{{ t('clusterV3.table') }}<select v-model="retentionTable" class="input mt-1 block"><option v-for="table in ['usage_logs','ops_error_logs','ops_system_logs']" :key="table">{{ table }}</option></select></label><label class="text-sm">{{ t('clusterV3.retentionDays') }}<input v-model.number="retentionDays" type="number" min="1" max="3650" required class="input mt-1 block w-28" /></label><button class="btn btn-secondary" :disabled="busy || stale">{{ t('clusterV3.preview') }}</button></form><p class="text-sm text-gray-500">{{ t('clusterV3.previewWarning') }}</p><div v-if="preview" role="status" class="rounded-lg bg-gray-50 p-4 text-sm dark:bg-dark-900"><p>{{ t('clusterV3.before', { date: fmtTime(preview.cutoff) }) }} · {{ preview.rows.toLocaleString() }} {{ t('clusterV3.previewRows') }}</p><p class="mt-2">{{ t('clusterV3.reusable') }}: {{ bytes(preview.estimated_reusable_bytes) }}</p><p>{{ t('clusterV3.physical') }}: {{ bytes(preview.filesystem_bytes_guaranteed) }}</p><p class="mt-2 font-medium">{{ t('clusterV3.noDelete') }}</p></div></div>
  </section>
  <BaseDialog :show="editorOpen" :title="t(registering ? 'clusterV3.joinTitle' : 'clusterV3.configure')" width="wide" :show-close-button="!busy" @close="closeEditor">
    <form v-if="!token" id="cluster-node-editor" class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <p v-if="registering" class="text-sm text-gray-500 sm:col-span-2">{{ t('clusterV3.joinHelp') }}</p>
      <label class="text-sm">{{ t('clusterV3.name') }}<input v-model="draft.name" required maxlength="128" class="input mt-1 block w-full" /></label>
      <label class="text-sm">{{ t('clusterV3.identity') }}<input v-model="draft.node_id" :disabled="!registering" required pattern="[a-z0-9][a-z0-9_-]{0,63}" class="input mt-1 block w-full" /></label>
      <template v-if="registering"><label class="text-sm">{{ t('clusterV3.address') }}<input v-model="draft.address" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.version') }}<input v-model="draft.version" required class="input mt-1 block w-full" /></label><label class="text-sm sm:col-span-2">{{ t('clusterV3.fingerprint') }}<input v-model="draft.certificate_sha256" required pattern="[a-f0-9]{64}" class="input mt-1 block w-full font-mono" /></label><label class="text-sm">{{ t('clusterV3.roles') }}<input v-model="rolesText" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.capabilities') }}<input v-model="capabilitiesText" required class="input mt-1 block w-full" /></label></template>
      <label class="text-sm">{{ t('clusterV3.baseWeight') }}<input v-model.number="draft.base_weight" type="number" min="1" max="256" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.dbConnections') }}<input v-model.number="draft.db_connections" type="number" min="1" :max="maxDraftConnections" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.maxActive') }}<input v-model.number="draft.max_in_flight" type="number" min="1" max="100000" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.maxQueue') }}<input v-model.number="draft.max_queue" type="number" min="0" max="100000" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.monthlyLimit') }}<input v-model.number="monthlyGiB" type="number" min="0" step="0.1" class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.reserve') }}<input v-model.number="reserveGiB" type="number" min="0" step="0.1" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.cycleDay') }}<input v-model.number="draft.traffic_cycle_day" type="number" min="1" max="28" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.cycleTimezone') }}<input v-model="draft.traffic_timezone" required class="input mt-1 block w-full" /></label><label class="text-sm">{{ t('clusterV3.trafficSource') }}<select v-model="draft.traffic_source" class="input mt-1 block w-full"><option value="unknown">{{ t('clusterV3.unknown') }}</option><option value="interface_estimate">{{ t('clusterV3.interfaceEstimate') }}</option></select></label>
      <p class="text-xs text-gray-500 sm:col-span-2">{{ t('clusterV3.databaseBudgetHelp') }}</p>
      <p v-if="maxDraftConnections !== undefined" class="text-sm sm:col-span-2" :class="budgetExceeded ? 'text-red-600' : 'text-gray-500'" :role="budgetExceeded ? 'alert' : undefined">{{ t('clusterV3.nodeBudgetAvailable', { max: maxDraftConnections }) }}</p>
      <p class="text-xs text-gray-500 sm:col-span-2">{{ t('clusterV3.certificateRequired') }}</p>
    </form>
    <div v-else class="space-y-3"><h3 class="font-medium">{{ t('clusterV3.joinToken') }}</h3><p class="text-sm text-amber-700 dark:text-amber-300">{{ t('clusterV3.tokenWarning') }}</p><input :value="token" readonly type="password" autocomplete="off" class="input w-full font-mono" :aria-label="t('clusterV3.joinToken')" @focus="($event.target as HTMLInputElement).select()" /><p class="text-sm">{{ t('clusterV3.command') }}</p><code class="block break-all rounded bg-gray-100 p-3 text-xs dark:bg-dark-900">python3 node_admin.py enroll --directory /etc/sub2api/node-identity --controller {{ overview?.control_url || t('clusterV3.unknown') }}</code></div>
    <template #footer><button class="btn btn-secondary" :disabled="busy" @click="closeEditor">{{ t(token ? 'clusterV3.close' : 'clusterV3.cancel') }}</button><button v-if="!token" type="submit" form="cluster-node-editor" class="btn btn-primary" :disabled="busy || budgetExceeded">{{ t(busy ? 'clusterV3.saving' : 'clusterV3.save') }}</button></template>
  </BaseDialog>
  <BaseDialog :show="confirmation !== null" :title="t(confirmation?.action === 'redis' ? 'clusterV3.acknowledge' : `clusterV3.${confirmation?.action ?? 'confirm'}`)" :show-close-button="!busy" @close="!busy && (confirmation = null)">
    <p class="text-sm">{{ confirmation?.action === 'redis' ? t('clusterV3.recoveryWarning') : confirmation?.action === 'force_retire' ? t('clusterV3.forceWarning', { active: confirmation.node ? known(confirmation.node, 'active') : '—', waiting: confirmation.node ? known(confirmation.node, 'waiting') : '—', pending: confirmation.node ? known(confirmation.node, 'pending_settlement') : '—' }) : t('clusterV3.normalExitWarning') }}</p>
    <label v-if="confirmation?.action === 'force_retire'" class="mt-4 block text-sm">{{ confirmation.node?.config.node_id }}<input v-model="typedIdentity" class="input mt-1 w-full" autocomplete="off" /></label>
    <template #footer><button class="btn btn-secondary" :disabled="busy" @click="confirmation = null">{{ t('clusterV3.cancel') }}</button><button class="btn btn-primary" :disabled="busy || !mayConfirm" @click="confirmAction">{{ t(busy ? 'clusterV3.saving' : 'clusterV3.confirm') }}</button></template>
  </BaseDialog>
</template>
