<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsSystemMetricsSnapshot } from '@/api/admin/ops'
import { formatMemorySizeMB } from '../utils/opsFormatters'

const props = withDefaults(defineProps<{ nodes: OpsSystemMetricsSnapshot[]; responseNode?: string; staleAfterSeconds?: number }>(), { staleAfterSeconds: 180 })
const { t } = useI18n()
const nodes = computed(() => [...props.nodes].sort((a, b) => (a.node_id ?? '').localeCompare(b.node_id ?? '')))
const fresh = (node: OpsSystemMetricsSnapshot) => {
  const age = Date.now() - Date.parse(node.created_at)
  return Number.isFinite(age) && age >= -5000 && age <= props.staleAfterSeconds * 1000
}
const percent = (node: OpsSystemMetricsSnapshot, value?: number | null) => fresh(node) && value != null ? `${value.toFixed(1)}%` : '—'
const current = (node: OpsSystemMetricsSnapshot) => Boolean(props.responseNode && node.node_id === props.responseNode)
const memory = (node: OpsSystemMetricsSnapshot) => fresh(node) && node.memory_used_mb != null && node.memory_total_mb != null ? `${formatMemorySizeMB(node.memory_used_mb)} / ${formatMemorySizeMB(node.memory_total_mb)}` : '—'
const opened = (node: OpsSystemMetricsSnapshot) => node.db_conn_active != null && node.db_conn_idle != null ? node.db_conn_active + node.db_conn_idle : '—'
</script>

<template>
  <section class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700" :aria-label="t('clusterV3.nodeResources')">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold">{{ t('clusterV3.nodeResources') }}</h3>
      <p class="text-xs text-gray-500">{{ t('clusterV3.resourceScope') }}</p>
    </div>
    <div class="grid gap-3 xl:grid-cols-3">
      <article v-for="node in nodes" :key="node.node_id" :data-node-id="node.node_id" :data-current="current(node)" class="min-w-0 rounded-2xl border p-4" :class="current(node) ? 'border-primary-500 bg-primary-50/60 ring-1 ring-primary-500/30 dark:bg-primary-900/15' : 'border-gray-200 bg-gray-50/60 dark:border-dark-600 dark:bg-dark-900/60'">
        <header class="flex flex-wrap items-center justify-between gap-2">
          <h4 class="font-semibold">{{ node.node_name || node.node_id }}</h4>
          <span v-if="current(node)" class="rounded-full bg-primary-100 px-2 py-1 text-[10px] font-semibold text-primary-800 dark:bg-primary-900/70 dark:text-primary-100">{{ t('clusterV3.currentResponse') }}</span>
        </header>
        <p class="mt-1 text-[10px] text-gray-500">{{ node.node_id }} · {{ fresh(node) ? t('clusterV3.fresh') : t('clusterV3.metricsUnavailable') }}</p>
        <dl class="mt-4 grid grid-cols-2 gap-x-4 gap-y-4 text-xs">
          <div><dt class="text-gray-500">CPU</dt><dd class="mt-1 text-xl font-semibold tabular-nums">{{ percent(node, node.cpu_usage_percent) }}</dd><p class="mt-1 text-[10px] text-gray-500">{{ node.cpu_cores == null ? '—' : t('clusterV3.cpuCores', { count: node.cpu_cores }) }}</p></div>
          <div><dt class="text-gray-500">{{ t('admin.ops.memory') }}</dt><dd class="mt-1 text-xl font-semibold tabular-nums">{{ percent(node, node.memory_usage_percent) }}</dd><p class="mt-1 text-[10px] text-gray-500">{{ memory(node) }}</p></div>
          <div><dt class="text-gray-500">{{ t('clusterV3.nodeDBPool') }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ opened(node) }} / {{ node.db_max_open_conns ?? '—' }}</dd><p class="mt-1 text-[10px] text-gray-500">{{ t('admin.ops.active') }} {{ node.db_conn_active ?? '—' }} · {{ t('admin.ops.idle') }} {{ node.db_conn_idle ?? '—' }}</p><p class="mt-1 text-[10px] text-gray-500">{{ t('clusterV3.configuredBudget') }} {{ node.db_configured_budget ?? '—' }}</p></div>
          <div><dt class="text-gray-500">{{ t('clusterV3.nodeRedisPool') }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ node.redis_conn_total ?? '—' }} / {{ node.redis_pool_size ?? '—' }}</dd><p class="mt-1 text-[10px] text-gray-500">{{ t('admin.ops.idle') }} {{ node.redis_conn_idle ?? '—' }}</p><p class="mt-1 text-[10px] text-gray-500">{{ t('admin.ops.goroutines') }} {{ fresh(node) ? node.goroutine_count ?? '—' : '—' }}</p></div>
        </dl>
        <time v-if="fresh(node)" class="mt-3 block text-[10px] text-gray-500" :datetime="node.created_at">{{ t('clusterV3.sampleTime') }} {{ new Date(node.created_at).toLocaleTimeString() }}</time>
      </article>
    </div>
  </section>
</template>
