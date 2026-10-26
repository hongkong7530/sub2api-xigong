<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useDocumentVisibility } from '@vueuse/core'
import ProfitGroupList from '@/components/profit/ProfitGroupList.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfitOverview from '@/components/profit/ProfitOverview.vue'
import ProfitScopeNote from '@/components/profit/ProfitScopeNote.vue'
import GroupCostEditor from '@/components/admin/profit/GroupCostEditor.vue'
import ProfitLedgerTable from '@/components/admin/profit/ProfitLedgerTable.vue'
import { useProfitDisclosure } from '@/features/profit/useProfitDisclosure'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
import '@/features/profit/profit.css'
const { t } = useI18n()
const { data, loading, error, window, refresh } = useProfitDisclosure(true)
const { enabled } = useExperienceMotion()
const visibility = useDocumentVisibility()
</script>
<template>
  <AppLayout><div class="profit-page" :data-motion="enabled && visibility === 'visible' ? 'on' : 'off'" data-testid="profit-management">
    <header class="profit-heading"><div><h1>{{ t('profit.adminTitle') }}</h1><p>{{ t('profit.adminSubtitle') }}</p></div><button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ t('profit.refresh') }}</button></header>
    <p v-if="error" class="profit-alert" role="alert">{{ t('profit.error') }}</p><p v-else-if="data?.stale" class="profit-alert">{{ t('profit.stale') }}</p>
    <div v-if="loading && !data" class="profit-skeleton" role="status">{{ t('profit.loading') }}</div>
    <template v-if="data">
      <ProfitOverview :data="data" :selected="window" @select="window = $event" />
      <section class="profit-admin-panel"><h2>{{ t('profit.configure') }}</h2><p>{{ t('profit.configureHint') }}</p><div class="profit-admin-grid"><GroupCostEditor v-for="group in data.groups" :key="group.id" :group="group" @saved="refresh" /></div></section>
      <ProfitGroupList :groups="data.groups" :window="window" admin />
      <ProfitLedgerTable :groups="data.groups" />
      <ProfitScopeNote :data="data" />
    </template>
  </div></AppLayout>
</template>
