<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useDocumentVisibility } from '@vueuse/core'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ProfitOverview from '@/components/profit/ProfitOverview.vue'
import ProfitProviderStage from '@/components/profit/ProfitProviderStage.vue'
import ProfitGroupList from '@/components/profit/ProfitGroupList.vue'
import ProfitScopeNote from '@/components/profit/ProfitScopeNote.vue'
import { useProfitDisclosure } from '@/features/profit/useProfitDisclosure'
import { useExperienceMotion } from '@/composables/useExperienceMotion'
import type { ProfitWindow } from '@/api/profit'
import '@/features/profit/profit.css'
const { t } = useI18n()
const { data, loading, error, window, provider, providers, groups, refresh } = useProfitDisclosure()
const { enabled, toggle } = useExperienceMotion()
const visibility = useDocumentVisibility()
const periods: ProfitWindow[] = ['total', 'today', 'week']
</script>
<template>
  <AppLayout><div class="profit-page" :data-motion="enabled && visibility === 'visible' ? 'on' : 'off'" data-testid="profit-disclosure">
    <header class="profit-heading"><div><h1>{{ t('profit.title') }}</h1><p>{{ t('profit.subtitle') }}</p></div><div class="profit-actions"><button class="btn btn-secondary" :aria-label="t(enabled ? 'profit.pause' : 'profit.play')" :aria-pressed="enabled" @click="toggle"><Icon name="sparkles" size="sm" /></button><button class="btn btn-secondary" :disabled="loading" :aria-label="t('profit.refresh')" @click="refresh"><Icon name="refresh" size="sm" /></button></div></header>
    <p v-if="error" class="profit-alert" role="alert">{{ t('profit.error') }} <button @click="refresh">{{ t('profit.retry') }}</button></p>
    <p v-else-if="data?.stale" class="profit-alert" role="status">{{ t('profit.stale') }}</p>
    <div v-if="!data && loading" class="profit-skeleton" role="status"><span>{{ t('profit.loading') }}</span><i></i><i></i><i></i></div>
    <template v-if="data">
      <ProfitOverview :data="data" :selected="window" @select="window = $event" />
      <ProfitProviderStage :providers="providers" :selected="provider" @select="provider = $event" />
      <div class="profit-toolbar"><div class="profit-periods" :aria-label="t('profit.range')"><button v-for="period in periods" :key="period" :class="{ 'is-active': window === period }" :aria-pressed="window === period" @click="window = period">{{ t('profit.' + period) }}</button></div><span>{{ t('profit.sevenDays') }}</span></div>
      <ProfitGroupList :groups="groups" :window="window" />
      <ProfitScopeNote :data="data" />
    </template>
  </div></AppLayout>
</template>
