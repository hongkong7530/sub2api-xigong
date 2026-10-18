<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between gap-3">
      <label class="input-label mb-0">{{ t('keys.jailbreak.title') }}</label>
      <Toggle v-model="enabled" />
    </div>
    <p class="input-hint">{{ t('keys.jailbreak.enableHint') }}</p>

    <div
      v-if="props.modelValue.enabled"
      class="space-y-4 rounded-xl border border-purple-200 bg-purple-50/50 p-4 dark:border-purple-900/40 dark:bg-purple-900/10"
    >
      <div>
        <label class="input-label">{{ t('keys.jailbreak.templateLabel') }}</label>
        <Select
          :model-value="props.modelValue.template"
          :options="templateOptions"
          :placeholder="t('keys.jailbreak.templatePlaceholder')"
          @update:model-value="onTemplateChange"
        >
          <!-- 模板描述是选择的唯一依据：只显示名称会让同名/相似模板无从分辨。 -->
          <template #option="{ option }">
            <span class="flex min-w-0 flex-col">
              <span class="select-option-label">{{ option.label }}</span>
              <span
                v-if="option.description"
                class="truncate text-xs text-gray-500 dark:text-dark-300"
              >
                {{ option.description }}
              </span>
            </span>
          </template>
        </Select>
        <p class="input-hint mt-1.5">{{ t('keys.jailbreak.templateHint') }}</p>
      </div>

      <TextArea
        :model-value="props.modelValue.system_prompt"
        :label="t('keys.jailbreak.systemPromptLabel')"
        :placeholder="t('keys.jailbreak.systemPromptPlaceholder')"
        :hint="t('keys.jailbreak.systemPromptHint')"
        :rows="5"
        @update:model-value="onSystemPromptChange"
      />

      <TextArea
        :model-value="props.modelValue.user_prompt"
        :label="t('keys.jailbreak.userPromptLabel')"
        :placeholder="t('keys.jailbreak.userPromptPlaceholder')"
        :hint="t('keys.jailbreak.userPromptHint')"
        :rows="3"
        @update:model-value="onUserPromptChange"
      />

      <div
        class="rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-900/40 dark:bg-blue-900/10"
      >
        <p class="text-xs leading-relaxed text-blue-700 dark:text-blue-300">
          {{ t('keys.jailbreak.notice') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import type { JailbreakFormState, JailbreakTemplateInfo } from '@/types'

const props = defineProps<{
  modelValue: JailbreakFormState
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: JailbreakFormState): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const templates = ref<JailbreakTemplateInfo[]>([])

const enabled = computed({
  get: () => props.modelValue.enabled,
  set: (value: boolean) => emit('update:modelValue', { ...props.modelValue, enabled: value })
})

const templateOptions = computed(() => [
  { value: '', label: t('keys.jailbreak.templateNone'), description: '' },
  ...templates.value.map((tpl) => ({
    value: tpl.name,
    label: tpl.name,
    description: tpl.description ?? '',
  })),
])

const onTemplateChange = (value: string | number | boolean | null) => {
  emit('update:modelValue', { ...props.modelValue, template: value == null ? '' : String(value) })
}

const onSystemPromptChange = (value: string) => {
  emit('update:modelValue', { ...props.modelValue, system_prompt: value })
}

const onUserPromptChange = (value: string) => {
  emit('update:modelValue', { ...props.modelValue, user_prompt: value })
}

onMounted(async () => {
  try {
    templates.value = await keysAPI.listJailbreakTemplates()
  } catch (error) {
    console.error('Failed to load jailbreak templates:', error)
    appStore.showError(t('keys.jailbreak.templatesLoadFailed'))
  }
})
</script>
