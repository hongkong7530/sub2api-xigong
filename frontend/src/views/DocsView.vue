<template>
 <ExperienceShell variant="public" class="docs-page">
  <header class="docs-header scene-header"><RouterLink to="/home" class="xg-display">{{ app.siteName }}</RouterLink><span>{{ t('experience.docs') }}</span><div class="docs-header-tools"><button class="btn btn-secondary" @click="toggleTheme">{{ t('experience.theme') }}</button><RouterLink class="btn btn-primary" :to="auth.isAdmin ? '/admin/dashboard' : '/dashboard'">{{ t('home.dashboard') }}</RouterLink></div></header>
  <div class="docs-layout">
   <aside class="docs-sidebar scene-surface" data-scene-protect><label for="docs-search">{{ t('experience.search') }}</label><input id="docs-search" v-model="query" class="input mt-2" type="search" :placeholder="t('experience.search')" /><button class="docs-mobile-toggle btn btn-secondary" :aria-expanded="directoryOpen" @click="directoryOpen = !directoryOpen">{{ t('experience.allDocs') }}</button><nav v-show="directoryOpen || !mobile" :aria-label="t('experience.allDocs')"><RouterLink to="/docs" class="docs-all">{{ t('experience.allDocs') }}</RouterLink><section v-for="category in groups" :key="category.id"><h2>{{ category.name }}</h2><RouterLink v-for="doc in category.items" :key="doc.id" :to="`/docs/read/${doc.slug}`" :aria-current="slug === doc.slug ? 'page' : undefined">{{ doc.title }}</RouterLink></section><div class="flex flex-wrap items-center justify-between gap-2 mt-5"><button class="btn btn-secondary btn-sm" :disabled="page === 1 || loading" @click="changePage(-1)">{{ t('experience.previousDoc') }}</button><span>{{ page }}</span><button class="btn btn-secondary btn-sm" :disabled="!index.has_more || loading" @click="changePage(1)">{{ t('experience.nextDoc') }}</button></div></nav></aside>
   <main id="docs-main" class="docs-main scene-surface" data-scene-protect><PixelLoading v-if="loading" /><div v-else-if="error" class="xg-status" role="alert">{{ t('experience.loadError') }} <button class="btn btn-secondary" @click="load">{{ t('experience.retry') }}</button></div><template v-else-if="document"><header class="article-header"><p class="text-sm text-primary-700 dark:text-primary-300">{{ t('experience.updated') }} {{ formatDate(document.updated_at) }}</p><h1>{{ document.title }}</h1><p>{{ document.summary }}</p></header><MarkdownArticle :source="document.content" /><nav class="docs-pagination" :aria-label="t('experience.allDocs')"><RouterLink v-if="previous" :to="`/docs/read/${previous.slug}`"><small>{{ t('experience.previousDoc') }}</small>{{ previous.title }}</RouterLink><RouterLink v-if="next" :to="`/docs/read/${next.slug}`"><small>{{ t('experience.nextDoc') }}</small>{{ next.title }}</RouterLink></nav></template><template v-else><h1>{{ t('experience.docs') }}</h1><p v-if="!index.documents.length" class="xg-status">{{ t(query ? 'experience.noResults' : 'experience.noDocs') }}</p><div v-else class="docs-index"><RouterLink v-for="doc in index.documents" :key="doc.id" :to="`/docs/read/${doc.slug}`"><h2>{{ doc.title }}</h2><p>{{ doc.summary }}</p><span>{{ formatDate(doc.updated_at) }} ↗</span></RouterLink></div></template></main>
   <aside v-if="document" class="docs-toc scene-surface" data-scene-protect><h2>{{ t('experience.contents') }}</h2><a v-for="heading in headings" :key="heading.id" :href="`#${heading.id}`" :style="{ paddingLeft: `${Math.max(0, heading.level - 2) * 12}px` }">{{ heading.title }}</a></aside>
  </div>
 </ExperienceShell>
</template>
<script setup lang="ts">
import ExperienceShell from '@/features/appearance/ExperienceShell.vue'
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { useAppStore, useAuthStore } from '@/stores'
import { siteContent, type DocumentIndex, type PublicDocument } from '@/api/siteContent'
import { documentMarkdown } from '@/features/docs/markdown'
import MarkdownArticle from '@/features/docs/MarkdownArticle.vue'
import PixelLoading from '@/components/common/PixelLoading.vue'
const app = useAppStore(), auth = useAuthStore(), route = useRoute(), router = useRouter()
const { t, locale } = useI18n()
const query = ref(''), document = ref<PublicDocument | null>(null), index = ref<DocumentIndex>({ documents: [], categories: [], page: 1, has_more: false }), loading = ref(true), error = ref(false), directoryOpen = ref(false)
const page = ref(1)
const mobile = useMediaQuery('(max-width: 760px)')
const slug = computed(() => typeof route.params.slug === 'string' ? route.params.slug : '')
const headings = computed(() => documentMarkdown(document.value?.content || '').headings)
const groups = computed(() => [...index.value.categories, { id: 0, name: t('experience.uncategorized'), position: 0 }].map(c => ({ ...c, items: index.value.documents.filter(d => d.category_id === c.id) })).filter(c => c.items.length))
const currentIndex = computed(() => index.value.documents.findIndex(d => d.slug === slug.value))
const previous = computed(() => currentIndex.value > 0 ? index.value.documents[currentIndex.value - 1] : null)
const next = computed(() => currentIndex.value >= 0 ? index.value.documents[currentIndex.value + 1] : null)
let controller: AbortController | undefined, version = 0, timer: ReturnType<typeof setTimeout> | undefined
async function load() {
 controller?.abort(); controller = new AbortController(); const token = ++version; loading.value = true; error.value = false; document.value = null
 try {
  const [list, article] = await Promise.all([siteContent.docs(query.value, controller.signal, page.value), slug.value ? siteContent.document(slug.value, controller.signal) : Promise.resolve(null)])
  if (token !== version) return; index.value = list; document.value = article; directoryOpen.value = false; loading.value = false
  await nextTick(); if (route.hash) window.document.getElementById(route.hash.slice(1))?.scrollIntoView()
 } catch { if (token === version && !controller.signal.aborted) error.value = true } finally { if (token === version) loading.value = false }
}
let resettingSearch = false
watch(slug, value => { clearTimeout(timer); resettingSearch = true; if (value) query.value = ''; resettingSearch = false; void load() }, { immediate: true })
watch(query, () => { if (resettingSearch) return; page.value = 1; clearTimeout(timer); timer = setTimeout(() => { if (slug.value) void router.push('/docs'); else void load() }, 250) }, { flush: 'sync' })
function changePage(delta: number) { page.value += delta; void load() }
function toggleTheme() { const dark = window.document.documentElement.classList.toggle('dark'); localStorage.setItem('theme', dark ? 'dark' : 'light') }
const formatDate = (value: string) => value ? new Date(value).toLocaleDateString(locale.value) : ''
onBeforeUnmount(() => { version++; controller?.abort(); clearTimeout(timer) })
</script>
<style scoped>
.docs-page{min-height:100vh;background:var(--xg-background);color:var(--xg-text)}.docs-header{position:sticky;top:0;z-index:25;display:flex;gap:25px;align-items:center;padding:16px 4%;background:var(--xg-surface);border-bottom:1px solid var(--xg-border)}.docs-header>a{font-size:24px}.docs-header-tools{margin-left:auto;display:flex;gap:10px}.docs-layout{display:grid;grid-template-columns:240px minmax(0,780px) 180px;max-width:1380px;margin:auto;gap:42px;padding:36px 24px}.docs-sidebar{position:sticky;top:105px;align-self:start;max-height:calc(100vh - 130px);overflow:auto}.docs-sidebar>label{font-size:13px;color:var(--xg-muted)}.docs-sidebar nav{margin-top:28px}.docs-sidebar nav a{display:block;padding:10px 12px;border-radius:10px;overflow-wrap:anywhere;font-size:14px}.docs-sidebar nav a[aria-current]{background:#14b8a61a;color:var(--xg-brand)}.docs-sidebar nav h2{font-size:12px;color:var(--xg-muted);margin:24px 12px 8px}.docs-main{min-width:0;padding:28px 32px 60px}.docs-sidebar.scene-surface,.docs-toc.scene-surface{padding:20px}.docs-main h1{font-size:36px;line-height:1.35;margin:12px 0 24px;overflow-wrap:anywhere}.article-header{border-bottom:1px solid var(--xg-border);padding:20px 0 30px;margin-bottom:32px}.article-header>p:last-child{color:var(--xg-muted);line-height:1.8}.docs-toc{position:sticky;top:120px;align-self:start;max-height:70vh;overflow:auto;font-size:12px}.docs-toc h2{margin-bottom:18px;font-weight:600}.docs-toc a{display:block;margin:12px 0;color:var(--xg-muted);overflow-wrap:anywhere}.docs-index{display:grid;gap:18px}.docs-index a{padding:24px;background:var(--xg-surface);border:1px solid var(--xg-border);border-radius:18px}.docs-index h2{font-size:23px;margin-bottom:10px}.docs-index p{color:var(--xg-muted);line-height:1.8}.docs-index span{font-size:12px;display:block;margin-top:18px;color:var(--xg-brand)}.docs-pagination{border-top:1px solid var(--xg-border);margin-top:50px;padding-top:24px;display:flex;justify-content:space-between;gap:20px}.docs-pagination a{max-width:48%;overflow-wrap:anywhere;color:var(--xg-brand)}.docs-pagination small{display:block;color:var(--xg-muted);margin-bottom:8px}.docs-mobile-toggle{display:none}@media(max-width:1150px){.docs-layout{grid-template-columns:220px minmax(0,1fr);gap:28px}.docs-toc{display:none}}@media(max-width:760px){.docs-header{gap:12px;padding:12px 18px;flex-wrap:wrap}.docs-header>span{display:none}.docs-header-tools .btn{font-size:12px;padding:8px 12px}.docs-layout{display:block;padding:22px 18px}.docs-sidebar{position:static;max-height:none;margin-bottom:28px}.docs-mobile-toggle{display:block;margin-top:12px}.docs-sidebar nav{max-height:45vh;overflow:auto}.docs-main h1{font-size:30px}}
</style>
