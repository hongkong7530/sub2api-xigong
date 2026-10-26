<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminGroup } from '@/types'
import type { ChannelMonitor, GroupAutomationSelection, ProbeMode } from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
const props = defineProps<{ show: boolean; busy: boolean }>()
const emit = defineEmits<{ close: []; submit: [groups: GroupAutomationSelection[]] }>()
const { t } = useI18n()
const groups = ref<AdminGroup[]>([])
const choices = ref<Record<number, GroupAutomationSelection & { selected: boolean; existing: boolean }>>({})
const search = ref('')
const loading = ref(false)
const failed = ref(false)
let epoch = 0
const visible = computed(() => groups.value.filter(g => g.name.toLowerCase().includes(search.value.trim().toLowerCase())))
const selected = computed(() => Object.values(choices.value).filter(x => x.selected))
const invalid = computed(() => selected.value.some(x => !Number.isInteger(x.interval_seconds) || x.interval_seconds < 15 || x.interval_seconds > 3600))
async function load() {
  const version = ++epoch; loading.value = true; failed.value = false
  try {
    const [all, first] = await Promise.all([adminAPI.groups.getAll(), adminAPI.channelMonitor.list({ page: 1, page_size: 100 })])
    const monitors: ChannelMonitor[] = [...first.items]
    for (let page = 2; page <= first.pages; page++) {
      if (version !== epoch) return
      monitors.push(...(await adminAPI.channelMonitor.list({ page, page_size: 100 })).items)
    }
    if (version !== epoch) return
    groups.value = all.filter(g => g.status === 'active' && !g.is_exclusive)
    choices.value = Object.fromEntries(groups.value.map(g => {
      const existing = monitors.find(m => m.group_id === g.id && m.key_provision_mode === 'auto')
      return [g.id, { group_id: g.id, probe_mode: existing?.probe_mode ?? 'hybrid', interval_seconds: existing?.interval_seconds ?? 60, selected: false, existing: !!existing }]
    }))
  } catch { if (version === epoch) failed.value = true }
  finally { if (version === epoch) loading.value = false }
}
function selectVisible(value: boolean) { for (const g of visible.value) { const c = choices.value[g.id]; if (c) c.selected = value } }
function setMode(id: number, event: Event) { const c = choices.value[id]; if (c) c.probe_mode = (event.target as HTMLSelectElement).value as ProbeMode }
function submit() {
  if (props.busy || loading.value || failed.value || !selected.value.length || invalid.value) return
  emit('submit', selected.value.map(({ group_id, probe_mode, interval_seconds }) => ({ group_id, probe_mode, interval_seconds })))
}
watch(() => props.show, show => { if (show) { search.value = ''; void load() } else epoch++ }, { immediate: true })
onBeforeUnmount(() => { epoch++ })
</script>
<template>
  <BaseDialog :show="show" :title="t('admin.channelMonitor.autoProvision.setupTitle')" width="wide" @close="!busy && emit('close')">
    <div class="space-y-4" data-testid="monitor-automation-options">
      <p class="text-sm leading-relaxed text-gray-600 dark:text-gray-300">{{ t('admin.channelMonitor.autoProvision.setupHint') }}</p>
      <div class="flex flex-wrap items-center gap-2"><input v-model="search" :disabled="busy" class="input w-full min-w-0 sm:flex-1" :aria-label="t('admin.channelMonitor.autoProvision.searchGroups')" :placeholder="t('admin.channelMonitor.autoProvision.searchGroups')"><button type="button" class="btn btn-secondary" :disabled="busy || loading" @click="selectVisible(true)">{{ t('admin.channelMonitor.autoProvision.selectVisible') }}</button><button type="button" class="btn btn-secondary" :disabled="busy || loading" @click="selectVisible(false)">{{ t('admin.channelMonitor.autoProvision.selectNone') }}</button></div>
      <p v-if="loading" role="status" class="py-5 text-center text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-else-if="failed" role="alert" class="rounded-xl bg-red-50 p-3 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-300">{{ t('admin.channelMonitor.loadError') }} <button type="button" class="underline" @click="load">{{ t('admin.channelMonitor.autoProvision.retry') }}</button></div>
      <div v-else class="max-h-[52vh] space-y-2 overflow-y-auto pr-1">
        <article v-for="group in visible" :key="group.id" class="rounded-xl border p-3 transition-colors" :class="choices[group.id]?.selected ? 'border-primary-400 bg-primary-50/50 dark:bg-primary-500/10' : 'border-gray-200 dark:border-dark-700'">
          <label class="flex min-h-10 items-center gap-3"><input v-model="choices[group.id]!.selected" type="checkbox" :disabled="busy" class="h-4 w-4 accent-primary-600"><span class="min-w-0 flex-1 font-semibold text-gray-900 dark:text-white">{{ group.name }}<small class="ml-2 font-normal text-gray-500">#{{ group.id }} · {{ group.platform }}</small></span><span class="text-xs text-gray-500">{{ t(choices[group.id]?.existing ? 'admin.channelMonitor.autoProvision.existing' : 'admin.channelMonitor.autoProvision.missing') }}</span></label>
          <div class="mt-2 grid gap-3 sm:grid-cols-2"><label class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.channelMonitor.probeMode.label') }}<select :value="choices[group.id]?.probe_mode" :disabled="busy || !choices[group.id]?.selected" class="input mt-1" @change="setMode(group.id, $event)"><option value="hybrid">{{ t('admin.channelMonitor.probeMode.hybrid') }}</option><option value="passive">{{ t('admin.channelMonitor.probeMode.passive') }}</option></select></label><label class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.channelMonitor.form.intervalSeconds') }}<input v-model.number="choices[group.id]!.interval_seconds" type="number" min="15" max="3600" step="1" :disabled="busy || !choices[group.id]?.selected || choices[group.id]?.probe_mode === 'passive'" class="input mt-1"></label></div>
          <p v-if="choices[group.id]?.probe_mode === 'passive'" class="mt-2 text-xs text-gray-500">{{ t('admin.channelMonitor.probeMode.passiveHint') }}</p>
        </article>
        <p v-if="!visible.length" class="py-5 text-center text-sm text-gray-500">{{ t('admin.channelMonitor.autoProvision.noMatchingGroups') }}</p>
      </div>
      <p v-if="invalid" role="alert" class="text-sm text-red-600">{{ t('admin.channelMonitor.autoProvision.invalidInterval') }}</p>
    </div>
    <template #footer><div class="flex w-full flex-wrap items-center justify-between gap-3"><span class="text-sm text-gray-500">{{ t('admin.channelMonitor.autoProvision.selectedCount', { count: selected.length }) }}</span><div class="flex gap-2"><button type="button" class="btn btn-secondary" :disabled="busy" @click="emit('close')">{{ t('common.cancel') }}</button><button type="button" class="btn btn-primary" :disabled="busy || loading || failed || !selected.length || invalid" @click="submit">{{ t(busy ? 'admin.channelMonitor.autoProvision.running' : 'admin.channelMonitor.autoProvision.createSelected') }}</button></div></div></template>
  </BaseDialog>
</template>
