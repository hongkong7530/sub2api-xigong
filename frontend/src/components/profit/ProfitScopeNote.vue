<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ProfitSnapshot } from '@/api/profit'
import { metricProfit, profitTime } from '@/features/profit/formatProfit'
defineProps<{ data: ProfitSnapshot }>()
const { t } = useI18n()
</script>
<template>
  <section class="profit-scope">
    <div class="profit-scope__types"><span>{{ t('profit.balance') }} <b>{{ metricProfit(data.balance) }}</b></span><span>{{ t('profit.subscription') }} <b>{{ metricProfit(data.subscription) }}</b></span></div>
    <details><summary>{{ t('profit.accounting') }}</summary><p>{{ t('profit.formula') }}</p><p>{{ t('profit.disclaimer') }}</p><p>{{ t('profit.history') }}</p><p>{{ t('profit.atCostNote') }}</p><p>{{ t('profit.pendingHint') }}</p></details>
    <footer><span>{{ t('profit.started') }} {{ profitTime(data.started_at) }}</span><span>{{ t('profit.updated') }} {{ profitTime(data.as_of) }}</span><span>{{ t('profit.timezone') }} {{ data.timezone }}</span></footer>
    <details class="profit-credits"><summary>{{ t('profit.art') }}</summary><p>{{ t('profit.artHint') }}</p><a href="/experience/profit/sources.json" target="_blank" rel="noopener noreferrer">{{ t('profit.artCredit') }}</a></details>
  </section>
</template>
