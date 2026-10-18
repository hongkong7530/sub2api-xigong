<template><div ref="article" class="xg-prose" @click="copyCode" v-html="rendered.html" /></template>
<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { documentMarkdown } from './markdown'
const props = defineProps<{ source: string }>()
const { t } = useI18n()
const article = ref<HTMLElement>()
const rendered = computed(() => documentMarkdown(props.source))
async function buttons() { await nextTick(); article.value?.querySelectorAll('pre').forEach(pre => { const button = document.createElement('button'); button.type = 'button'; button.className = 'docs-copy'; button.textContent = t('experience.copy'); pre.querySelector('.docs-copy')?.remove(); pre.prepend(button) }) }
watch(rendered, buttons)
onMounted(buttons)
async function copyCode(event: MouseEvent) { const button = (event.target as HTMLElement).closest<HTMLButtonElement>('.docs-copy'); if (!button) return; try { await navigator.clipboard.writeText(button.parentElement?.querySelector('code')?.textContent || ''); button.textContent = t('experience.copied') } catch { button.textContent = t('experience.retry') } }
</script>
<style scoped>
:deep(pre) { position: relative; padding-top: 48px; }
:deep(.docs-copy) { position: absolute; right: 8px; top: 4px; min-height: 36px; padding: 4px 12px; font-size: 12px; color: var(--xg-brand); border: 1px solid var(--xg-border); border-radius: 8px; }
</style>
