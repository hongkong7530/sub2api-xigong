<template>
  <BaseDialog
    :show="show"
    :title="t('monitorCommon.subscribeTitle', { group: groupName })"
    width="normal"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p
        v-if="!hasEmail"
        class="rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-200"
      >
        {{ t('monitorCommon.subscribeNeedEmail') }}
      </p>

      <label class="flex items-start gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <input
          v-model="firstToken"
          type="checkbox"
          class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          :disabled="saving || !hasEmail"
        />
        <span class="text-sm leading-relaxed text-gray-800 dark:text-gray-100">
          {{ t('monitorCommon.subscribeFirstToken') }}
        </span>
      </label>

      <label class="flex items-start gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <input
          v-model="rateChange"
          type="checkbox"
          class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          :disabled="saving || !hasEmail"
        />
        <span class="text-sm leading-relaxed text-gray-800 dark:text-gray-100">
          {{ t('monitorCommon.subscribeRateChange') }}
        </span>
      </label>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button
          type="button"
          class="rounded-lg px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
          :disabled="saving"
          @click="emit('close')"
        >
          {{ t('monitorCommon.subscribeClose') }}
        </button>
        <button
          type="button"
          class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-700 disabled:opacity-50"
          :disabled="saving || !hasEmail"
          @click="save"
        >
          {{ t('common.save') }}
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
import { setSubscription, type MonitorSubscription } from '@/api/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  groupId: number | null
  groupName: string
  hasEmail: boolean
  current: MonitorSubscription | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', value: MonitorSubscription): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const firstToken = ref(true)
const rateChange = ref(true)
const saving = ref(false)

// 打开时回填当前订阅；未订阅默认两开关全开，用户按需关闭。
watch(
  () => [props.show, props.current] as const,
  ([show, current]) => {
    if (!show) return
    firstToken.value = current ? current.notify_first_token : true
    rateChange.value = current ? current.notify_rate_change : true
  },
  { immediate: true },
)

async function save() {
  if (!props.groupId) return
  saving.value = true
  try {
    const result = await setSubscription(props.groupId, {
      notify_first_token: firstToken.value,
      notify_rate_change: rateChange.value,
    })
    appStore.showSuccess(t('monitorCommon.subscribeSaved'))
    emit('saved', result)
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('monitorCommon.subscribeFailed')))
  } finally {
    saving.value = false
  }
}
</script>
