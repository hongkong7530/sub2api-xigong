<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ProfitSnapshot, ProfitWindow } from '@/api/profit'
import { metricProfit, profitPercent, profitMoney, negativeMoney } from '@/features/profit/formatProfit'
const props = defineProps<{ data: ProfitSnapshot; selected: ProfitWindow }>()
const emit = defineEmits<{ select: [value: ProfitWindow] }>()
const { t } = useI18n()
const periods: ProfitWindow[] = ['total', 'today', 'week']
const selectedMetric = computed(() => props.data.metrics[props.selected])
</script>

<template>
  <section class="profit-vault" data-testid="profit-overview" :aria-label="t(data.scope === 'all_groups' ? 'profit.adminScope' : 'profit.totalScope')">
    <div class="profit-vault__beam" aria-hidden="true"></div>
    <div class="profit-vault__identity"><span class="profit-vault__seal" aria-hidden="true">✦</span><span>{{ t(data.scope === 'all_groups' ? 'profit.adminScope' : 'profit.totalScope') }}</span><span class="profit-vault__live" :class="{ 'is-stale': data.stale }">{{ t(data.stale ? 'profit.partial' : 'profit.known') }}</span></div>
    <div class="profit-vault__metrics">
      <button v-for="(period, index) in periods" :key="period" type="button" class="profit-vault__metric" :class="{ 'is-selected': selected === period, 'is-primary': index === 0 }" :aria-pressed="selected === period" @click="emit('select', period)">
        <span class="profit-vault__label">{{ t('profit.' + period) }}</span>
        <strong :class="{ 'profit-loss': negativeMoney(data.metrics[period].profit) }" :data-testid="'profit-value-' + period">{{ metricProfit(data.metrics[period]) }}</strong>
        <span class="profit-vault__ratio">{{ t('profit.margin') }} <b>{{ profitPercent(data.metrics[period].margin) }}</b><span v-if="data.metrics[period].known_requests < data.metrics[period].requests" class="profit-partial">{{ t('profit.partial') }}</span></span>
      </button>
    </div>
    <div class="profit-vault__footer">
      <span>{{ t('profit.revenue') }} <b>{{ profitMoney(selectedMetric.revenue) }}</b></span>
      <span v-if="data.scope === 'all_groups'">{{ t('profit.cost') }} <b>{{ selectedMetric.known_requests ? profitMoney(selectedMetric.cost) : '—' }}</b></span>
      <span>{{ t('profit.coverage') }} <b>{{ profitPercent(selectedMetric.coverage) }}</b></span>
      <span :title="t('profit.pendingHint')">{{ t('profit.unknown') }} <b>{{ selectedMetric.requests - selectedMetric.known_requests }}</b></span>
    </div>
  </section>
</template>
