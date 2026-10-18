<template>
  <div class="space-y-2">
    <div class="flex flex-wrap items-center gap-1">
      <button
        v-for="action in actions"
        :key="action.id"
        type="button"
        class="rounded-lg px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
        :title="action.title"
        @click="apply(action)"
      >
        {{ action.label }}
      </button>
    </div>
    <div class="grid gap-3 md:grid-cols-2">
      <textarea
        ref="textareaRef"
        class="input min-h-[12rem] font-mono text-sm"
        :value="modelValue"
        :maxlength="maxLength"
        :placeholder="placeholder"
        @input="onInput"
      />
      <div
        class="announcement-markdown min-h-[12rem] overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-900/40"
        v-html="previewHtml || '&nbsp;'"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderSafeMarkdown } from '@/utils/safeMarkdown'
import '@/styles/announcement-markdown.css'

const props = withDefaults(defineProps<{
  modelValue: string
  maxLength?: number
  placeholder?: string
}>(), {
  maxLength: 4000,
  placeholder: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const { t } = useI18n()
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const previewHtml = computed(() => renderSafeMarkdown(props.modelValue))

const actions = computed(() => [
  { id: 'bold', label: t('liteMarkdown.bold'), title: t('liteMarkdown.bold'), before: '**', after: '**' },
  { id: 'italic', label: t('liteMarkdown.italic'), title: t('liteMarkdown.italic'), before: '*', after: '*' },
  { id: 'link', label: t('liteMarkdown.link'), title: t('liteMarkdown.link'), before: '[', after: '](https://)' },
  { id: 'list', label: t('liteMarkdown.list'), title: t('liteMarkdown.list'), before: '- ', after: '' },
  { id: 'hr', label: t('liteMarkdown.hr'), title: t('liteMarkdown.hr'), before: '\n---\n', after: '' },
])

function onInput(event: Event) {
  const value = (event.target as HTMLTextAreaElement).value
  emit('update:modelValue', value.slice(0, props.maxLength))
}

function apply(action: { before: string; after: string }) {
  const el = textareaRef.value
  const current = props.modelValue ?? ''
  if (!el) {
    emit('update:modelValue', `${current}${action.before}${action.after}`.slice(0, props.maxLength))
    return
  }
  const start = el.selectionStart
  const end = el.selectionEnd
  const selected = current.slice(start, end) || (action.after ? t('liteMarkdown.selectedText') : '')
  const next = `${current.slice(0, start)}${action.before}${selected}${action.after}${current.slice(end)}`.slice(0, props.maxLength)
  emit('update:modelValue', next)
}
</script>
