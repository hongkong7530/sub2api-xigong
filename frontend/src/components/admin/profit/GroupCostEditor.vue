<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { profitApi, type ProfitGroup } from '@/api/profit'
import { profitTime } from '@/features/profit/formatProfit'
const props = defineProps<{ group: ProfitGroup }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const input = ref('')
const saving = ref(false)
const message = ref('')
const failed = ref(false)
watch(() => [props.group.cost_multiplier, props.group.cost_version], () => { input.value = props.group.cost_multiplier ?? '' }, { immediate: true })
async function save(clear = false) {
  const value = clear || !input.value.trim() ? null : input.value.trim()
  if (value != null && (!/^\d+(\.\d{1,8})?$/.test(value) || Number(value) > 10000)) { failed.value = true; message.value = t('profit.invalid'); return }
  saving.value = true; message.value = ''
  try {
    await profitApi.setCost(props.group.id, value, props.group.cost_version ?? 0)
    failed.value = false; message.value = t('profit.saved'); emit('saved')
  } catch (error: unknown) {
    failed.value = true
    const status = (error as { response?: { status?: number } }).response?.status
    message.value = t(status === 409 ? 'profit.conflict' : 'profit.saveError')
  } finally { saving.value = false }
}
</script>
<template>
  <article class="profit-cost-editor" :data-cost-group="group.id">
    <h3>{{ group.name }}</h3><small>{{ t('profit.currentRate') }} {{ group.rate || '—' }}× · {{ group.platform }}</small>
    <form @submit.prevent="save()">
      <label :for="'profit-cost-' + group.id">{{ t('profit.costRate') }}<input :id="'profit-cost-' + group.id" v-model="input" class="input" inputmode="decimal" autocomplete="off" :disabled="saving" :aria-invalid="failed" :aria-describedby="'profit-cost-message-' + group.id" placeholder="—"></label>
      <button type="submit" class="btn btn-primary" :disabled="saving || group.status === 'deleted'">{{ t(saving ? 'profit.saving' : 'profit.save') }}</button>
      <button type="button" class="btn btn-secondary" :disabled="saving || group.status === 'deleted' || group.cost_multiplier == null" @click="save(true)">{{ t('profit.clear') }}</button>
    </form>
    <div class="profit-cost-editor__meta"><span>{{ t('profit.version') }} {{ group.cost_version ?? 0 }}</span><span>{{ t('profit.effective') }} {{ profitTime(group.effective_at) }}</span></div>
    <p :id="'profit-cost-message-' + group.id" class="profit-cost-editor__notice" :class="{ 'is-error': failed }" role="status">{{ message }}</p>
  </article>
</template>
