<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getUsageServiceTierLabel, normalizeUsageServiceTier, verifiedUltrafastBillingMultiplier } from '@/utils/usageServiceTier'

type TierRow = Parameters<typeof verifiedUltrafastBillingMultiplier>[0]
const props = defineProps<{ row: TierRow }>()
const { t } = useI18n()
const tier = computed(() => normalizeUsageServiceTier(props.row.service_tier))
const sixTimes = computed(() => verifiedUltrafastBillingMultiplier(props.row) === 6)
</script>

<template>
  <span v-if="tier && tier !== 'standard'" class="usage-tier" :class="{ 'usage-tier--ultra': tier === 'ultrafast' }" data-testid="service-tier-badge" :title="t('usage.serviceTierBillingHint')">
    <svg v-if="tier === 'ultrafast'" class="usage-tier__bolt" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M9.5 1.5 3 9h4.5L6.5 14.5 13 7H8.5l1-5.5Z" fill="currentColor" /></svg>
    <span>{{ getUsageServiceTierLabel(row.service_tier, t) }}</span>
    <span v-if="sixTimes" data-testid="service-tier-price-multiplier" class="usage-tier__multiplier" :title="t('usage.ultrafastSixTimesHint')">6×</span>
  </span>
</template>

<style scoped>
.usage-tier { --tier-gold:#ffd595; --tier-rose:#ff8bd4; --tier-ink:#793254; position:relative; isolation:isolate; display:inline-flex; flex-shrink:0; align-items:center; gap:5px; overflow:hidden; padding:4px 8px; border:1px solid #f1b6cf; border-radius:7px 3px 7px 3px; background:#fff0f7; color:var(--tier-ink); font-size:10px; line-height:1.3; font-weight:800; white-space:nowrap; letter-spacing:.015em; vertical-align:middle; }
.usage-tier--ultra { background:linear-gradient(115deg,#fff6d9,#ffe3f1 60%,#f5ddff); border-color:#eeb2c9; box-shadow:inset 0 1px #fff9,0 0 10px #f7a7ce30; }
.usage-tier--ultra::before { content:''; position:absolute; inset:-60% -40%; z-index:-1; pointer-events:none; background:linear-gradient(110deg,transparent 38%,#fff9 49%,transparent 60%); transform:translateX(-90%); }
.usage-tier__bolt { width:13px; height:13px; color:#b53c76; }
.usage-tier__multiplier { border-left:1px solid #bc598b55; padding-left:5px; color:#9f3270; font-variant-numeric:tabular-nums; }
:global(.dark .usage-tier) { --tier-ink:#ffe3f3; border-color:#fc8cd275; background:#361c38; }
:global(.dark .usage-tier--ultra) { background:linear-gradient(110deg,#513326,#4c2046 60%,#372045); box-shadow:inset 0 1px #ffdba03d,0 0 14px #ff52b430,0 0 5px #ffd59520; }
:global(.dark .usage-tier__bolt) { color:var(--tier-gold); filter:drop-shadow(0 0 3px #ffcc85aa); }
:global(.dark .usage-tier__multiplier) { color:var(--tier-gold); border-color:#ffc38666; }
@media(prefers-reduced-motion:no-preference) { .usage-tier--ultra::before { animation:tier-sheen 2.2s .15s 2; } }
@keyframes tier-sheen { to { transform:translateX(90%); } }
:global(html[data-xg-motion='off'] .usage-tier)::before { animation:none; }
</style>
