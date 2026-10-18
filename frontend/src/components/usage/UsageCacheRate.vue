<template>
  <div class="w-28" data-testid="usage-cache-rate">
    <button
      v-if="percent != null"
      ref="trigger"
      type="button"
      class="group w-full rounded-md px-1 py-1 text-left transition hover:bg-primary-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-primary-500/10"
      :aria-label="t('usage.cacheDetails.open', { rate: percent.toFixed(1) })"
      :aria-expanded="open"
      aria-haspopup="dialog"
      @click.stop="toggle"
      @keydown.esc.stop="close"
    >
      <span class="flex items-baseline justify-between gap-2">
        <strong class="text-sm font-semibold tabular-nums" :class="percent > 0 ? 'text-primary-700 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'">{{ percent.toFixed(1) }}%</strong>
        <span class="text-xs text-gray-400 group-hover:text-primary-600" aria-hidden="true">ⓘ</span>
      </span>
      <span class="mt-1.5 flex h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700" aria-hidden="true">
        <span class="bg-primary-500 dark:bg-primary-300" :style="{ width: `${read / total * 100}%` }"></span>
        <span class="bg-amber-400" :style="{ width: `${write / total * 100}%` }"></span>
        <span class="bg-slate-400 dark:bg-slate-500" :style="{ width: `${input / total * 100}%` }"></span>
      </span>
    </button>
    <div v-else class="px-1 py-1 text-xs text-gray-400 dark:text-gray-500">
      <span aria-hidden="true">—</span>
      <span class="ml-1">{{ t(availableShape ? 'usage.cacheDetails.noSample' : 'usage.cacheDetails.notApplicable') }}</span>
    </div>
    <Teleport to="body">
      <div v-if="open" class="fixed inset-0 z-[70]" @click="close" @keydown.esc.stop="close">
        <section
          ref="panel"
          role="dialog"
          aria-modal="true"
          @keydown.tab.prevent="panel?.querySelector<HTMLButtonElement>('button')?.focus()"
          :aria-label="t('usage.cacheDetails.title')"
          tabindex="-1"
          class="fixed w-80 max-w-[calc(100vw-2rem)] rounded-xl border border-gray-200 bg-white p-4 text-gray-800 shadow-xl focus:outline-none dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100"
          :style="position"
          @click.stop
        >
          <div class="flex items-center justify-between gap-3">
            <h4 class="text-sm font-semibold">{{ t('usage.cacheDetails.title') }}</h4>
            <button type="button" class="rounded p-1 text-gray-500 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700" :aria-label="t('common.close')" @click="close">×</button>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('usage.cacheDetails.inputBasis') }}</p>
          <dl class="mt-4 space-y-2.5 text-xs">
            <div class="flex justify-between gap-4"><dt class="text-primary-700 dark:text-primary-300">{{ t('usage.cacheDetails.read') }}</dt><dd class="font-mono tabular-nums">{{ number(read) }} · {{ percent?.toFixed(1) }}%</dd></div>
            <div class="flex justify-between gap-4"><dt class="text-amber-700 dark:text-amber-300">{{ t('usage.cacheDetails.write') }}</dt><dd class="font-mono tabular-nums">{{ number(write) }}</dd></div>
            <div class="flex justify-between gap-4"><dt class="text-gray-500 dark:text-gray-400">{{ t('usage.cacheDetails.uncached') }}</dt><dd class="font-mono tabular-nums">{{ number(input) }}</dd></div>
            <div class="flex justify-between gap-4 border-t border-gray-100 pt-2 dark:border-dark-700"><dt>{{ t('usage.cacheDetails.totalInput') }}</dt><dd class="font-mono tabular-nums">{{ number(total) }}</dd></div>
          </dl>
          <p class="mt-3 border-t border-gray-100 pt-3 text-xs leading-relaxed text-gray-500 dark:border-dark-700 dark:text-gray-400">{{ t('usage.cacheDetails.formula') }}</p>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { isStreamingUsage, isTokenUsageBilling, usageCacheRatePercent, type UsageCacheRateRow } from '@/utils/usageCacheRate'
const props = defineProps<{ row: UsageCacheRateRow }>()
const { t, locale } = useI18n()
const percent = computed(() => usageCacheRatePercent(props.row))
const availableShape = computed(() => isStreamingUsage(props.row) && isTokenUsageBilling(props.row))
const input = computed(() => props.row.input_tokens ?? 0)
const read = computed(() => props.row.cache_read_tokens ?? 0)
const write = computed(() => props.row.cache_creation_tokens ?? 0)
const total = computed(() => input.value + read.value + write.value)
const number = (n: number) => new Intl.NumberFormat(locale.value).format(n)
const open = ref(false)
const trigger = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const position = ref<Record<string, string>>({})
function close() {
  if (!open.value) return
  open.value = false
  window.removeEventListener('resize', close)
  window.removeEventListener('scroll', close, true)
  void nextTick(() => trigger.value?.focus())
}
async function toggle() {
  if (open.value) { close(); return }
  const rect = trigger.value?.getBoundingClientRect()
  if (!rect) return
  position.value = { left: `${Math.max(16, Math.min(rect.left, window.innerWidth - 336))}px`, top: `${Math.max(16, Math.min(rect.bottom + 8, window.innerHeight - 310))}px` }
  open.value = true
  window.addEventListener('resize', close)
  window.addEventListener('scroll', close, true)
  await nextTick()
  panel.value?.focus()
}
watch(() => props.row, close)
onBeforeUnmount(() => { window.removeEventListener('resize', close); window.removeEventListener('scroll', close, true) })
</script>
