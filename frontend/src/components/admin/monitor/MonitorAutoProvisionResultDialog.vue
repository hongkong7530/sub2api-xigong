<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.autoProvision.dialogTitle')"
    width="normal"
    @close="$emit('close')"
  >
    <div v-if="report" class="space-y-4">
      <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <div class="rounded-xl bg-gray-50 px-3 py-2.5 text-center dark:bg-dark-900/40">
          <p class="text-lg font-black tabular-nums text-gray-900 dark:text-white">{{ report.groups_scanned }}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.channelMonitor.autoProvision.statScanned') }}</p>
        </div>
        <div class="rounded-xl bg-emerald-50 px-3 py-2.5 text-center dark:bg-emerald-500/10">
          <p class="text-lg font-black tabular-nums text-emerald-700 dark:text-emerald-300">{{ report.monitors_created }}</p>
          <p class="text-[11px] text-emerald-700/70 dark:text-emerald-300/70">{{ t('admin.channelMonitor.autoProvision.statMonitors') }}</p>
        </div>
        <div class="rounded-xl bg-sky-50 px-3 py-2.5 text-center dark:bg-sky-500/10">
          <p class="text-lg font-black tabular-nums text-sky-700 dark:text-sky-300">{{ report.plaza_channels_created }}</p>
          <p class="text-[11px] text-sky-700/70 dark:text-sky-300/70">{{ t('admin.channelMonitor.autoProvision.statChannels') }}</p>
        </div>
        <div class="rounded-xl bg-gray-50 px-3 py-2.5 text-center dark:bg-dark-900/40">
          <p class="text-lg font-black tabular-nums text-gray-900 dark:text-white">{{ skippedCount }}</p>
          <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.channelMonitor.autoProvision.statSkipped') }}</p>
        </div>
      </div>

      <p
        v-if="failedCount"
        class="rounded-xl bg-red-50 px-3 py-2 text-xs font-medium text-red-700 dark:bg-red-500/10 dark:text-red-300"
      >
        {{ t('admin.channelMonitor.autoProvision.failedNotice', { count: failedCount }) }}
      </p>

      <div v-if="report.items.length" class="max-h-72 space-y-1.5 overflow-y-auto pr-1">
        <div
          v-for="(item, index) in report.items"
          :key="`${item.group_id}-${item.action}-${index}`"
          class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
        >
          <div class="min-w-0">
            <span class="font-medium text-gray-900 dark:text-white">{{ item.group_name }}</span>
            <p v-if="item.detail" class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" :title="item.detail">
              {{ item.detail }}
            </p>
          </div>
          <span
            class="inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-[11px]"
            :class="actionBadgeClass(item.action)"
          >
            {{ actionLabel(item.action) }}
          </span>
        </div>
      </div>
      <p v-else class="flex h-16 items-center justify-center text-sm text-gray-400">
        {{ t('admin.channelMonitor.autoProvision.empty') }}
      </p>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-primary">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupAutomationReport } from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  report: GroupAutomationReport | null
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()

const failedCount = computed(
  () => props.report?.items.filter((item) => item.action.endsWith('_error')).length ?? 0
)
const skippedCount = computed(
  () => props.report?.items.filter((item) => item.action.endsWith('_skipped')).length ?? 0
)

function actionLabel(action: string): string {
  switch (action) {
    case 'monitor_created':
      return t('admin.channelMonitor.autoProvision.actionMonitorCreated')
    case 'monitor_updated':
      return t('admin.channelMonitor.autoProvision.actionMonitorUpdated')
    case 'monitor_repaired':
      return t('admin.channelMonitor.autoProvision.actionMonitorRepaired')
    case 'monitor_skipped':
      return t('admin.channelMonitor.autoProvision.actionMonitorSkipped')
    case 'monitor_error':
      return t('admin.channelMonitor.autoProvision.actionMonitorError')
    case 'plaza_channel_created':
      return t('admin.channelMonitor.autoProvision.actionPlazaCreated')
    case 'plaza_channel_updated':
      return t('admin.channelMonitor.autoProvision.actionPlazaUpdated')
    case 'plaza_skipped':
      return t('admin.channelMonitor.autoProvision.actionPlazaSkipped')
    case 'plaza_error':
      return t('admin.channelMonitor.autoProvision.actionPlazaError')
    default:
      return action
  }
}

// 与运行结果弹窗一致的徽章语义：创建=绿、跳过=灰、失败=红。
function actionBadgeClass(action: string): string {
  if (action.endsWith('_error')) return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  if (action.endsWith('_skipped')) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
}
</script>
