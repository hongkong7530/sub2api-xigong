<template>
  <aside
    v-if="html"
    class="flex min-w-0 max-w-xl items-start gap-3 rounded-2xl border border-sky-200/80 bg-sky-50/80 px-3 py-2.5 text-sm text-sky-900 dark:border-sky-900/40 dark:bg-sky-950/40 dark:text-sky-100"
  >
    <span class="mt-0.5 inline-flex h-6 w-1 shrink-0 rounded-full bg-sky-500" aria-hidden="true" />
    <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0 text-sky-500" />
    <div class="announcement-markdown min-w-0" v-html="html" />
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { renderSafeMarkdown } from '@/utils/safeMarkdown'
import '@/styles/announcement-markdown.css'

const props = defineProps<{ markdown?: string | null }>()
const html = computed(() => renderSafeMarkdown(props.markdown || ''))
</script>
