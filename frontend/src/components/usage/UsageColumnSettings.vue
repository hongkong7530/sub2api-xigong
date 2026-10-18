<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Column } from '@/components/common/types'

const props = defineProps<{
  columns: Column[]
  hiddenColumns: ReadonlySet<string>
  alwaysVisible: string[]
}>()
const emit = defineEmits<{
  toggle: [key: string]
  move: [key: string, targetKey: string]
  shift: [key: string, direction: -1 | 1]
  reset: []
  close: []
}>()
const { t } = useI18n()
const dragging = ref<string | null>(null)
const beginDrag = (event: DragEvent, key: string) => {
  dragging.value = key
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', key)
  }
}
const drop = (targetKey: string) => {
  if (dragging.value) emit('move', dragging.value, targetKey)
  dragging.value = null
}
</script>

<template>
  <div data-testid="usage-column-order-panel" class="absolute right-0 top-full z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-xl dark:border-dark-600 dark:bg-dark-800" @keydown.esc.stop="$emit('close')">
    <div class="border-b border-gray-100 px-3 py-3 dark:border-dark-700">
      <p class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('usage.columns.title') }}</p>
      <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('usage.columns.hint') }}</p>
    </div>
    <div class="max-h-80 overflow-y-auto p-1.5">
      <div v-for="(column, index) in props.columns" :key="column.key" :data-column-key="column.key" class="flex items-center gap-1 rounded-lg px-1 hover:bg-gray-50 dark:hover:bg-dark-700" :class="{ 'opacity-50': dragging === column.key }" @dragover.prevent @drop.prevent="drop(column.key)">
        <button type="button" draggable="true" class="flex h-9 w-7 shrink-0 cursor-grab items-center justify-center text-gray-400 active:cursor-grabbing" :aria-label="t('usage.columns.drag', { column: column.label })" @dragstart="beginDrag($event, column.key)" @dragend="dragging = null">
          <svg width="12" height="16" viewBox="0 0 12 16" fill="currentColor" aria-hidden="true"><circle cx="4" cy="4" r="1"/><circle cx="8" cy="4" r="1"/><circle cx="4" cy="8" r="1"/><circle cx="8" cy="8" r="1"/><circle cx="4" cy="12" r="1"/><circle cx="8" cy="12" r="1"/></svg>
        </button>
        <button type="button" :data-testid="`usage-column-toggle-${column.key}`" :disabled="alwaysVisible.includes(column.key)" :aria-pressed="alwaysVisible.includes(column.key) || !hiddenColumns.has(column.key)" :title="alwaysVisible.includes(column.key) ? t('usage.columns.alwaysVisible') : undefined" class="flex min-h-10 min-w-0 flex-1 items-center justify-between gap-2 py-2 text-left text-sm text-gray-700 disabled:cursor-default dark:text-gray-200" @click="emit('toggle', column.key)">
          <span class="break-words">{{ column.label }}</span>
          <svg v-if="alwaysVisible.includes(column.key) || !hiddenColumns.has(column.key)" class="h-4 w-4 shrink-0 text-primary-500" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m4 10 4 4 8-8"/></svg>
        </button>
        <button type="button" :data-testid="`usage-column-up-${column.key}`" :disabled="index === 0" :aria-label="t('usage.columns.moveUp', { column: column.label })" class="flex h-9 w-8 shrink-0 items-center justify-center rounded-md text-gray-600 hover:bg-gray-100 disabled:opacity-25 dark:text-gray-300 dark:hover:bg-dark-600" @click="emit('shift', column.key, -1)"><svg class="h-4 w-4" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m5 12 5-5 5 5"/></svg></button>
        <button type="button" :data-testid="`usage-column-down-${column.key}`" :disabled="index === columns.length - 1" :aria-label="t('usage.columns.moveDown', { column: column.label })" class="flex h-9 w-8 shrink-0 items-center justify-center rounded-md text-gray-600 hover:bg-gray-100 disabled:opacity-25 dark:text-gray-300 dark:hover:bg-dark-600" @click="emit('shift', column.key, 1)"><svg class="h-4 w-4" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m5 8 5 5 5-5"/></svg></button>
      </div>
    </div>
    <div class="border-t border-gray-100 p-2 dark:border-dark-700"><button type="button" data-testid="usage-column-reset-order" class="w-full rounded-lg px-3 py-2 text-sm text-primary-600 hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-dark-700" @click="emit('reset')">{{ t('usage.columns.reset') }}</button></div>
  </div>
</template>
