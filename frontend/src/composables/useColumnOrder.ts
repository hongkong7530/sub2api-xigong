import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { Column } from '@/components/common/types'

/** Ignore stale/duplicate saved keys and append new columns without losing user order. */
export function normalizeColumnOrder(saved: unknown, available: readonly string[]): string[] {
  const allowed = new Set(available)
  const keys = Array.isArray(saved) ? saved.filter((key): key is string => typeof key === 'string' && allowed.has(key)) : []
  return [...new Set([...keys, ...available])]
}

export function useColumnOrder(columns: ComputedRef<Column[]>, storageKey: Ref<string>) {
  const order = ref<string[]>([])
  watch(storageKey, (key) => {
    try {
      const saved = localStorage.getItem(key)
      order.value = normalizeColumnOrder(saved ? JSON.parse(saved) : null, columns.value.map(column => column.key))
    } catch {
      order.value = columns.value.map(column => column.key)
    }
  }, { immediate: true })

  const orderedColumns = computed(() => {
    const byKey = new Map(columns.value.map(column => [column.key, column]))
    return normalizeColumnOrder(order.value, [...byKey.keys()]).map(key => byKey.get(key)!)
  })
  const save = (keys: string[]) => {
    order.value = keys
    // Browser storage may be unavailable; the current session remains usable.
    try { localStorage.setItem(storageKey.value, JSON.stringify(keys)) } catch { /* Session-only preference. */ }
  }
  const moveColumn = (key: string, targetKey: string) => {
    const keys = orderedColumns.value.map(column => column.key)
    const from = keys.indexOf(key), to = keys.indexOf(targetKey)
    if (from < 0 || to < 0 || from === to) return
    keys.splice(from, 1)
    keys.splice(to, 0, key)
    save(keys)
  }
  const shiftColumn = (key: string, direction: -1 | 1) => {
    const keys = orderedColumns.value.map(column => column.key)
    const index = keys.indexOf(key)
    const target = keys[index + direction]
    if (index >= 0 && target) moveColumn(key, target)
  }
  const resetColumnOrder = () => save(columns.value.map(column => column.key))
  return { orderedColumns, moveColumn, shiftColumn, resetColumnOrder }
}
