<template>
  <BaseDialog
    :show="show"
    :title="t('monitorCommon.reportTitle', { group: resolvedGroupName })"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div v-if="groups.length" class="space-y-2">
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('monitorCommon.reportGroup') }}</p>
        <select v-model.number="selectedGroupId" class="input">
          <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
        </select>
      </div>
      <div class="space-y-2">
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('monitorCommon.reportCategory') }}</p>
        <label
          v-for="option in categories"
          :key="option.value"
          class="flex items-start gap-3 rounded-xl border border-gray-200 p-3 dark:border-dark-700"
        >
          <input v-model="category" type="radio" class="mt-1" :value="option.value" />
          <span class="text-sm text-gray-800 dark:text-gray-100">{{ option.label }}</span>
        </label>
      </div>
      <div>
        <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('monitorCommon.reportBody') }}</p>
        <LiteMarkdownEditor v-model="body" :max-length="2000" :placeholder="t('monitorCommon.reportBodyPlaceholder')" />
      </div>
      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="submit">{{ t('common.submit') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { createGroupReport } from '@/api/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LiteMarkdownEditor from '@/components/common/LiteMarkdownEditor.vue'

const props = withDefaults(defineProps<{
  show: boolean
  groupId: number | null
  groupName: string
  groups?: Array<{ id: number; name: string }>
}>(), {
  groups: () => [],
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submitted', groupId: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const category = ref('unreachable')
const body = ref('')
const saving = ref(false)
const error = ref('')
const selectedGroupId = ref<number | null>(null)

const resolvedGroupId = computed(() => selectedGroupId.value || props.groupId)
const resolvedGroupName = computed(() => {
  const selected = props.groups.find((item) => item.id === resolvedGroupId.value)
  return selected?.name || props.groupName || ''
})

const categories = computed(() => [
  { value: 'unreachable', label: t('monitorCommon.reportCategories.unreachable') },
  { value: 'slow_first_token', label: t('monitorCommon.reportCategories.slowFirstToken') },
  { value: 'usage_doubt', label: t('monitorCommon.reportCategories.usageDoubt') },
  { value: 'other', label: t('monitorCommon.reportCategories.other') },
])

watch(() => props.show, (open) => {
  if (open) {
    category.value = 'unreachable'
    body.value = ''
    error.value = ''
    selectedGroupId.value = props.groupId || props.groups[0]?.id || null
  }
})

async function submit() {
  if (!resolvedGroupId.value) return
  if (category.value === 'other' && !body.value.trim()) {
    error.value = t('monitorCommon.reportBodyRequired')
    return
  }
  saving.value = true
  error.value = ''
  try {
    await createGroupReport(resolvedGroupId.value, { category: category.value, body: body.value })
    appStore.showSuccess(t('monitorCommon.reportSubmitted'))
    emit('submitted', resolvedGroupId.value)
    emit('close')
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('monitorCommon.reportFailed'))
  } finally {
    saving.value = false
  }
}
</script>
