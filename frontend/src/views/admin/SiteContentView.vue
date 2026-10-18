<template>
 <AppLayout><div class="space-y-6"><header class="flex flex-wrap items-center justify-between gap-3"><h1 class="text-2xl">{{ t('experience.contentAdmin') }}</h1><RouterLink class="btn btn-secondary" to="/admin/docs">{{ t('experience.docsAdmin') }}</RouterLink></header><p v-if="error" role="alert" class="xg-status text-red-600 dark:text-red-300">{{ error }} <button class="btn btn-secondary" @click="reload">{{ t('experience.retry') }}</button></p><p v-if="message" role="status" class="xg-status">{{ message }}</p><PixelLoading v-if="loading" /><template v-else>
 <AppearanceEditor v-model="appearance" :busy="busy" @save="saveAppearance" />
 <section class="card space-y-5 p-5 sm:p-7"><p class="text-sm text-gray-500">{{ t('experience.contentLimit') }}</p><div class="flex items-center justify-between gap-3"><h2 class="text-xl">{{ t('experience.partners') }}</h2><button class="btn btn-primary" @click="newPartner">{{ t('experience.newPartner') }}</button></div><p v-if="!partners.length" class="text-sm text-gray-500">{{ t('experience.noPartners') }}</p><div v-for="p in partners" :key="p.id" class="partner-row"><div class="min-w-0"><strong>{{ p.name }}</strong><p class="break-all text-sm text-gray-500">{{ p.url }}</p></div><span>{{ p.enabled ? t('experience.enabled') : t('experience.unpublished') }}</span><button class="btn btn-secondary" @click="editPartner(p)">{{ t('experience.edit') }}</button><button class="btn btn-ghost text-red-600" :disabled="busy" @click="removePartner(p)">{{ t('experience.delete') }}</button></div>
 <PartnerEditor v-if="partner" :key="partner.id" v-model="partner" :busy="busy" @save="savePartner" @cancel="cancelPartner" /></section>
 </template></div></AppLayout>
</template>
<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import PixelLoading from '@/components/common/PixelLoading.vue'
import AppearanceEditor from '@/features/appearance/AppearanceEditor.vue'
import PartnerEditor from '@/features/appearance/PartnerEditor.vue'
import { siteContent, defaultAppearance, type Partner } from '@/api/siteContent'
import { useExperienceStore } from '@/stores/experience'
const { t } = useI18n(), experience = useExperienceStore()
const appearance = ref(defaultAppearance()), partners = ref<Partner[]>([]), partner = ref<Partner | null>(null), loading = ref(true), busy = ref(false), error = ref(''), message = ref('')
const savedAppearance = ref(''), savedPartner = ref('null')
const dirty = computed(() => savedAppearance.value !== JSON.stringify(appearance.value) || savedPartner.value !== JSON.stringify(partner.value))
function confirmLeave() { return !dirty.value || window.confirm(t('experience.leaveWarning')) }
function fail(e: unknown) { error.value = (e as { status?: number }).status === 409 ? t('experience.conflict') : t('experience.saveError') }
async function load() { loading.value = true; try { const [a,p] = await Promise.all([siteContent.adminAppearance(),siteContent.adminPartners()]); appearance.value = a; partners.value = p; savedAppearance.value = JSON.stringify(a); error.value = '' } catch { error.value = t('experience.loadError') } finally { loading.value = false } }
async function reload() { if (!confirmLeave()) return; partner.value = null; savedPartner.value = 'null'; await load() }
async function saveAppearance() { if (busy.value) return; busy.value = true; error.value = ''; try { appearance.value.phrases = appearance.value.phrases.map(s => s.trim()).filter(Boolean); appearance.value.quotes = appearance.value.quotes.filter(q => q.en || q.zh); appearance.value = await siteContent.saveAppearance(appearance.value); savedAppearance.value = JSON.stringify(appearance.value); await experience.load(true); message.value = t('experience.saved') } catch(e) { fail(e) } finally { busy.value = false } }
function partnerClean() { return savedPartner.value === JSON.stringify(partner.value) || window.confirm(t('experience.leaveWarning')) }
function newPartner() { if (!partnerClean()) return; partner.value = { id:0,name:'',description:'',url:'',icon:'',tag:'',position:partners.value.length,enabled:true,version:0 }; savedPartner.value = JSON.stringify(partner.value) }
function editPartner(p: Partner) { if (!partnerClean()) return; partner.value = { ...p }; savedPartner.value = JSON.stringify(partner.value) }
function cancelPartner() { if (!partnerClean()) return; partner.value = null; savedPartner.value = 'null' }
async function savePartner() { if (!partner.value || busy.value) return; busy.value = true; error.value = ''; try { await siteContent.savePartner(partner.value); partners.value = await siteContent.adminPartners(); partner.value = null; savedPartner.value = 'null'; message.value = t('experience.saved') } catch(e) { fail(e) } finally { busy.value = false } }
async function removePartner(p: Partner) { if (busy.value || !window.confirm(t('experience.deleteWarning'))) return; busy.value = true; try { await siteContent.deletePartner(p); partners.value = await siteContent.adminPartners(); if (partner.value?.id === p.id) { partner.value = null; savedPartner.value = 'null' } } catch(e) { fail(e) } finally { busy.value = false } }
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value) { e.preventDefault(); e.returnValue = '' } }
onBeforeRouteLeave(confirmLeave)
onMounted(() => { void load(); window.addEventListener('beforeunload',beforeUnload) })
onBeforeUnmount(() => window.removeEventListener('beforeunload',beforeUnload))
</script>
<style scoped>
.wallpaper-settings{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.wallpaper-setting{border:1px solid var(--xg-border);border-radius:14px;overflow:hidden}.wallpaper-setting>img{width:100%;height:140px;object-fit:cover}.wallpaper-setting>div{padding:16px;display:grid;gap:12px;font-size:13px}.partner-row{display:flex;align-items:center;gap:16px;border-bottom:1px solid var(--xg-border);padding:16px 0}.partner-row>div{flex:1}@media(max-width:1100px){.wallpaper-settings{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:640px){.wallpaper-settings{grid-template-columns:1fr}.partner-row{flex-wrap:wrap}.partner-row>div{flex-basis:100%}}
</style>
