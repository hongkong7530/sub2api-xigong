<script setup lang="ts">
import { onBeforeUnmount, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { profitApi, type ProfitGroup, type ProfitEntryPage } from '@/api/profit'
import { negativeMoney, profitMoney, profitTime } from '@/features/profit/formatProfit'
defineProps<{ groups: ProfitGroup[] }>()
const { t } = useI18n()
const group = shallowRef(0)
const page = shallowRef(1)
const from = shallowRef('')
const to = shallowRef('')
const data = shallowRef<ProfitEntryPage | null>(null)
const loading = shallowRef(false)
const error = shallowRef(false)
let controller: AbortController | null = null
let epoch = 0
async function load() {
  const current = ++epoch; controller?.abort(); controller = new AbortController(); loading.value = true; error.value = false
  try { const result = await profitApi.entries({ page: page.value, group_id: group.value, from: from.value ? new Date(from.value).toISOString() : undefined, to: to.value ? new Date(to.value).toISOString() : undefined }, controller.signal); if (current === epoch) data.value = result }
  catch { if (current === epoch) error.value = true }
  finally { if (current === epoch) loading.value = false }
}
watch([group, from, to], () => { if (page.value !== 1) page.value = 1; else void load() })
watch(page, () => { void load() }, { immediate: true })
onBeforeUnmount(() => { epoch++; controller?.abort() })
</script>
<template>
  <section class="profit-admin-panel" data-testid="profit-ledger">
    <h2>{{ t('profit.ledger') }}</h2><p>{{ t('profit.ledgerHint') }}</p>
    <div class="profit-ledger-filter"><label>{{ t('profit.group') }}<select v-model="group" class="input"><option :value="0">{{ t('profit.all') }}</option><option v-for="item in groups" :key="item.id" :value="item.id">{{ item.name }}</option></select></label><label>{{ t('profit.from') }}<input v-model="from" type="datetime-local" class="input mt-1"></label><label>{{ t('profit.to') }}<input v-model="to" type="datetime-local" class="input mt-1"></label><button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('profit.refresh') }}</button></div>
    <p v-if="error" class="profit-alert" role="alert">{{ t('profit.error') }}</p>
    <p v-if="loading" role="status" class="mt-4 text-sm">{{ t('profit.loading') }}</p>
    <div v-if="data?.items.length" class="profit-ledger-scroll"><table class="profit-ledger"><thead><tr><th>{{ t('profit.settled') }}</th><th>{{ t('profit.group') }}</th><th>{{ t('profit.user') }}</th><th>{{ t('profit.revenue') }}</th><th>{{ t('profit.cost') }}</th><th>{{ t('profit.earned') }}</th><th>{{ t('profit.range') }}</th></tr></thead><tbody><tr v-for="entry in data.items" :key="entry.id"><td>{{ profitTime(entry.settled_at) }}</td><td>{{ entry.group_name }}</td><td>{{ entry.user_id }}</td><td>{{ profitMoney(entry.revenue, 8) }}</td><td>{{ entry.cost == null ? t('profit.unknown') : profitMoney(entry.cost, 8) }}</td><td><strong :class="{ 'profit-loss': negativeMoney(entry.profit) }">{{ profitMoney(entry.profit, 8) }}</strong></td><td>{{ t(entry.billing_type === 1 ? 'profit.subscription' : 'profit.balance') }}</td></tr></tbody></table></div>
    <p v-else-if="!loading && !error" class="profit-empty">{{ t('profit.noEntries') }}</p>
    <div class="profit-pagination"><button class="btn btn-secondary" :disabled="page <= 1 || loading" @click="page--">{{ t('profit.previous') }}</button><span>{{ t('profit.page', { page }) }}</span><button class="btn btn-secondary" :disabled="!data?.has_more || loading" @click="page++">{{ t('profit.next') }}</button></div>
  </section>
</template>
