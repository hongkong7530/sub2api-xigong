<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ProfitGroup, ProfitWindow } from '@/api/profit'
import { metricProfit, negativeMoney, profitPercent, profitMoney, providerName } from '@/features/profit/formatProfit'
defineProps<{ groups: ProfitGroup[]; window: ProfitWindow; admin?: boolean }>()
const { t } = useI18n()
</script>

<template>
  <section class="profit-groups" data-testid="profit-group-list">
    <header class="profit-section-head"><h2>{{ t('profit.groups') }}</h2><span>{{ t('profit.' + window) }}</span></header>
    <div v-if="!groups.length" class="profit-empty"><h3>{{ t('profit.noGroups') }}</h3><p>{{ t('profit.noGroupsHint') }}</p></div>
    <template v-else>
      <div class="profit-group-head" aria-hidden="true"><span>{{ t('profit.group') }}</span><span>{{ t('profit.currentRate') }}</span><span>{{ t('profit.earned') }}</span><span>{{ t('profit.margin') }}</span></div>
      <article v-for="group in groups" :key="group.id" class="profit-group" :data-group-id="group.id">
        <div class="profit-group__identity"><span class="profit-group__sigil" aria-hidden="true">{{ (providerName[group.platform] || group.platform).slice(0, 1) }}</span><div><h3>{{ group.name }}</h3><span>{{ providerName[group.platform] || group.platform }}</span></div></div>
        <div class="profit-group__rate" :title="t('profit.baseline')"><span class="profit-mobile-label">{{ t('profit.currentRate') }}</span><strong v-if="group.at_cost" class="profit-cost-label">{{ t('profit.atCost') }}</strong><strong v-else>{{ group.rate ? Number(group.rate).toLocaleString(undefined, { maximumFractionDigits: 8 }) + '×' : '—' }}</strong><small v-if="group.at_cost">{{ Number(group.rate).toLocaleString() }}×</small><small v-if="group.peak_enabled">{{ group.peak_start }}–{{ group.peak_end }} · {{ Number(group.peak_multiplier) }}×</small></div>
        <div class="profit-group__money"><span class="profit-mobile-label">{{ t('profit.earned') }}</span><strong :class="{ 'profit-loss': negativeMoney(group.metrics[window].profit) }">{{ metricProfit(group.metrics[window]) }}</strong><small>{{ t('profit.count', { count: group.metrics[window].requests }) }}</small><small v-if="admin">{{ t('profit.revenue') }} {{ profitMoney(group.metrics[window].revenue) }} / {{ t('profit.cost') }} {{ group.metrics[window].known_requests ? profitMoney(group.metrics[window].cost) : '—' }}</small></div>
        <div class="profit-group__margin" :title="group.at_cost && !admin ? t('profit.referenceHint') : t('profit.formula')"><span class="profit-mobile-label">{{ t('profit.margin') }}</span><strong :class="{ 'profit-loss': (group.at_cost && !admin ? group.reference_margin : group.metrics[window].margin)?.startsWith('-') }">{{ profitPercent(group.at_cost && !admin ? group.reference_margin : group.metrics[window].margin) }}</strong><small>{{ group.at_cost && !admin ? t('profit.reference') : t('profit.coverage') + ' ' + profitPercent(group.metrics[window].coverage) }}</small></div>
      </article>
    </template>
  </section>
</template>
