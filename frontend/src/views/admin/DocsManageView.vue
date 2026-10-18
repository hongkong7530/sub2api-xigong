<template>
 <AppLayout>
  <div class="doc-manager">
   <header class="flex flex-wrap items-center justify-between gap-3"><h1 class="text-2xl">{{ t('experience.docsAdmin') }}</h1><button class="btn btn-primary" @click="newDoc">{{ t('experience.newDoc') }}</button></header>
   <p v-if="message" class="xg-status" role="status">{{ message }}</p><p v-if="error" class="xg-status text-red-600 dark:text-red-300" role="alert">{{ error }} <button class="btn btn-secondary" @click="reload">{{ t('experience.retry') }}</button></p>
   <PixelLoading v-if="loading" />
   <div v-else class="doc-manager-grid">
    <aside class="card p-4"><h2 class="mb-3 font-semibold">{{ t('experience.allDocs') }}</h2><p v-if="!documents.length" class="text-sm text-gray-500">{{ t('experience.selectDoc') }}</p><button v-for="item in documents" :key="item.id" class="doc-choice" :class="{ selected: selected?.id === item.id }" @click="select(item)"><strong>{{ item.draft.title }}</strong><span>{{ t(item.published ? 'experience.published' : 'experience.draft') }}</span></button><div class="flex justify-between gap-2 mt-4"><button class="btn btn-secondary btn-sm" :disabled="page === 1 || busy" @click="changePage(-1)">{{ t('experience.previousDoc') }}</button><span class="self-center">{{ page }}</span><button class="btn btn-secondary btn-sm" :disabled="!hasMore || busy" @click="changePage(1)">{{ t('experience.nextDoc') }}</button></div><hr class="my-5" /><h2 class="mb-3 font-semibold">{{ t('experience.category') }}</h2><p class="text-xs text-gray-500 mb-3">{{ t('experience.contentLimit') }}</p><form class="space-y-2" @submit.prevent="saveCategory"><label for="category-name" class="text-sm">{{ t('experience.name') }}</label><input id="category-name" v-model="categoryName" class="input" maxlength="80" required /><button class="btn btn-secondary w-full" :disabled="busy">{{ t('experience.newCategory') }}</button></form><div v-for="cat in categories" :key="cat.id" class="category-row"><input v-model="cat.name" class="input" :aria-label="t('experience.name')" maxlength="80" /><input v-model.number="cat.position" class="input w-20" type="number" min="0" :aria-label="t('experience.position')" /><button class="btn btn-ghost btn-sm" :disabled="busy" @click="updateCategory(cat)">{{ t('experience.save') }}</button><button class="btn btn-ghost btn-sm" :disabled="busy" @click="deleteCategory(cat)">{{ t('experience.delete') }}</button></div></aside>
    <form v-if="selected" class="card min-w-0 space-y-5 p-5 sm:p-7" :inert="busy" @submit.prevent="save">
     <p class="text-sm leading-7 text-gray-600 dark:text-dark-300">{{ t('experience.publishHint') }}</p>
     <div class="grid gap-4 sm:grid-cols-2"><label>{{ t('experience.title') }}<input v-model="selected.draft.title" class="input mt-2" required maxlength="160" /></label><label>{{ t('experience.slug') }}<input v-model="selected.slug" class="input mt-2 font-mono" :readonly="!!selected.id" required pattern="[a-z0-9][a-z0-9-]{0,95}" maxlength="96" /></label><label>{{ t('experience.category') }}<select v-model.number="selected.draft.category_id" class="input mt-2"><option :value="0">{{ t('experience.uncategorized') }}</option><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label><label>{{ t('experience.position') }}<input v-model.number="selected.draft.position" class="input mt-2" type="number" min="0" required /></label></div>
     <label class="block">{{ t('experience.summary') }}<textarea v-model="selected.draft.summary" class="input mt-2" maxlength="600" rows="2" /></label>
     <div class="markdown-workbench"><label class="block min-w-0">{{ t('experience.body') }}<textarea v-model="selected.draft.content" class="input mt-2 min-h-[420px] font-mono text-sm" maxlength="60000" required spellcheck="false" /></label><section class="min-w-0"><h2 class="mb-2">{{ t('experience.preview') }}</h2><div class="doc-preview"><MarkdownArticle :source="selected.draft.content" /></div></section></div>
     <div class="flex flex-wrap gap-3"><button type="submit" class="btn btn-primary" :disabled="busy">{{ t('experience.saveDraft') }}</button><button type="button" class="btn btn-secondary" :disabled="busy || dirty || !selected.id" @click="action('publish')">{{ t('experience.publish') }}</button><button v-if="selected.published" type="button" class="btn btn-secondary" :disabled="busy || dirty" @click="action('unpublish')">{{ t('experience.unpublish') }}</button><button v-if="selected.id" type="button" class="btn btn-ghost text-red-600" :disabled="busy" @click="action('delete')">{{ t('experience.delete') }}</button></div>
    </form><p v-else class="xg-status self-start">{{ t('experience.selectDoc') }}</p>
   </div>
  </div>
 </AppLayout>
</template>
<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import PixelLoading from '@/components/common/PixelLoading.vue'
import MarkdownArticle from '@/features/docs/MarkdownArticle.vue'
import { siteContent, type SiteDocument, type Category } from '@/api/siteContent'
const { t } = useI18n()
const documents = ref<SiteDocument[]>([]), categories = ref<Category[]>([]), selected = ref<SiteDocument | null>(null)
const page = ref(1), hasMore = ref(false)
const loading = ref(true), busy = ref(false), error = ref(''), message = ref(''), categoryName = ref(''), snapshot = ref('')
const dirty = computed(() => !!selected.value && JSON.stringify(selected.value) !== snapshot.value)
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value))
function confirmLeave() { return !dirty.value || window.confirm(t('experience.leaveWarning')) }
async function select(item: SiteDocument) { if (busy.value || !confirmLeave()) return; busy.value = true; try { selected.value = await siteContent.adminDoc(item.id); snapshot.value = JSON.stringify(selected.value); error.value = ''; message.value = '' } catch(e) { fail(e) } finally { busy.value = false } }
function newDoc() { if (busy.value || !confirmLeave()) return; selected.value = { id: 0, slug: '', draft: { title: '', summary: '', content: '', category_id: 0, position: 0 }, published: null, version: 0, updated_at: '', published_at: null }; snapshot.value = JSON.stringify(selected.value); error.value = ''; message.value = '' }
function fail(e: unknown) { error.value = (e as { status?: number }).status === 409 ? t('experience.conflict') : t('experience.saveError') }
async function load() { loading.value = true; try { const [docs, cats] = await Promise.all([siteContent.adminDocs(page.value), siteContent.categories()]); documents.value = docs.documents; hasMore.value = docs.has_more; categories.value = cats; error.value = '' } catch { error.value = t('experience.loadError') } finally { loading.value = false } }
async function reload() { if (!confirmLeave()) return; const id = selected.value?.id; selected.value = null; await load(); if (id) { const found = documents.value.find(d => d.id === id); if (found) select(found) } }
async function changePage(delta: number) { if (busy.value || !confirmLeave()) return; selected.value = null; page.value += delta; await load() }
async function save() { if (!selected.value || busy.value) return; busy.value = true; error.value = ''; try { const updated = await siteContent.saveDoc(clone(selected.value)); selected.value = updated; snapshot.value = JSON.stringify(updated); const list = await siteContent.adminDocs(page.value); documents.value = list.documents; hasMore.value = list.has_more; message.value = t('experience.saved') } catch(e) { fail(e) } finally { busy.value = false } }
async function action(name: 'publish' | 'unpublish' | 'delete') { if (!selected.value || busy.value) return; if (name === 'delete' && !window.confirm(t('experience.deleteWarning'))) return; busy.value = true; error.value = ''; try { const id = selected.value.id; await siteContent.docAction(selected.value, name); selected.value = name === 'delete' ? null : await siteContent.adminDoc(id); snapshot.value = JSON.stringify(selected.value); const list = await siteContent.adminDocs(page.value); documents.value = list.documents; hasMore.value = list.has_more; message.value = t('experience.saved') } catch(e) { fail(e) } finally { busy.value = false } }
async function updateCategory(cat: Category) { if (busy.value) return; busy.value = true; try { await siteContent.saveCategory(cat); categories.value = await siteContent.categories(); message.value = t('experience.saved') } catch(e) { fail(e) } finally { busy.value = false } }
async function saveCategory() { await updateCategory({ id: 0, name: categoryName.value, position: categories.value.length }); if (!error.value) categoryName.value = '' }
async function deleteCategory(cat: Category) { if (busy.value || !window.confirm(t('experience.deleteWarning'))) return; busy.value = true; try { await siteContent.deleteCategory(cat); categories.value = await siteContent.categories() } catch(e) { fail(e) } finally { busy.value = false } }
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value) { e.preventDefault(); e.returnValue = '' } }
onBeforeRouteLeave(() => confirmLeave())
onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload) })
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>
<style scoped>
.doc-manager{display:grid;gap:24px}.doc-manager-grid{display:grid;grid-template-columns:270px minmax(0,1fr);gap:24px}.doc-choice{display:block;width:100%;text-align:left;padding:12px;border-radius:12px;margin:4px 0;overflow-wrap:anywhere}.doc-choice strong{display:block;font-size:14px}.doc-choice span{font-size:12px;color:var(--xg-muted)}.doc-choice.selected{background:#14b8a61a;color:var(--xg-brand)}.markdown-workbench{display:grid;grid-template-columns:1fr 1fr;gap:20px}.doc-preview{max-height:600px;min-height:420px;overflow:auto;border:1px solid var(--xg-border);padding:20px;border-radius:12px}.category-row{display:flex;flex-wrap:wrap;gap:5px;margin-top:15px}.category-row input:first-child{width:100%}@media(max-width:1200px){.markdown-workbench{grid-template-columns:1fr}}@media(max-width:800px){.doc-manager-grid{grid-template-columns:1fr}.doc-manager-grid>aside{max-height:330px;overflow:auto}}
</style>
