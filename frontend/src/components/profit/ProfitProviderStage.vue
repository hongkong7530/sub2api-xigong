<script setup lang="ts">
import { shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { providerName, providerPortrait } from '@/features/profit/formatProfit'
defineProps<{ providers: string[]; selected: string }>()
const emit = defineEmits<{ select: [value: string] }>()
const { t } = useI18n()
const failed = shallowRef<Set<string>>(new Set())
const markFailed = (provider: string) => { failed.value = new Set([...failed.value, provider]) }
</script>

<template>
  <section class="profit-providers" :aria-label="t('profit.provider')" data-testid="profit-provider-stage">
    <button class="profit-provider profit-provider--all" :class="{ 'is-active': selected === 'all' }" :aria-pressed="selected === 'all'" @click="emit('select', 'all')"><span class="profit-provider__all-mark" aria-hidden="true">✦</span><span>{{ t('profit.all') }}</span></button>
    <button v-for="provider in providers" :key="provider" class="profit-provider" :class="{ 'is-active': selected === provider }" :aria-pressed="selected === provider" @click="emit('select', provider)">
      <span class="profit-provider__portrait">
        <img v-if="!failed.has(provider)" :src="'/experience/profit/' + (providerPortrait[provider] || 'chatgpt') + '.webp'" :alt="(providerName[provider] || provider) + ' · ' + t('profit.artHint')" width="144" height="144" loading="lazy" decoding="async" @error="markFailed(provider)">
        <span v-else class="profit-provider__fallback">{{ (providerName[provider] || provider).slice(0, 2) }}</span>
      </span>
      <span class="profit-provider__name">{{ providerName[provider] || provider }}</span>
      <span class="profit-provider__dot" aria-hidden="true"></span>
    </button>
  </section>
</template>
