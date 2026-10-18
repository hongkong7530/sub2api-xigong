<script setup lang="ts">
import { shallowRef, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import {siteContent,type Partner} from '@/api/siteContent'
import AssetInput from './AssetInput.vue'
const partner=defineModel<Partner>({required:true})
defineProps<{busy:boolean}>()
const emit=defineEmits<{save:[];cancel:[]}>()
const {t}=useI18n(),resolving=shallowRef(false),error=shallowRef(''),warnings=shallowRef<string[]>([])
let controller:AbortController|undefined,generation=0,timer:ReturnType<typeof setTimeout>|undefined,alive=true
async function resolve(){
 const url=partner.value.url;if(!/^https:\/\//.test(url))return
 controller?.abort();controller=new AbortController();const token=++generation,before={...partner.value};resolving.value=true;error.value='';warnings.value=[]
 try{const found=await siteContent.resolvePartner(url,controller.signal);if(!alive||token!==generation||partner.value.url!==url)return
  const next={...partner.value};for(const key of ['name','description','icon','thumbnail'] as const){if(next[key]===before[key]&&(!next[key]||key==='thumbnail'||key==='icon'))next[key]=found[key]||next[key]||''}
  partner.value=next;warnings.value=found.warnings||[]
 }catch{if(alive&&token===generation)error.value=t('experience.resolveError')}
 finally{if(alive&&token===generation)resolving.value=false}
}
watch(()=>partner.value.url,()=>{generation++;controller?.abort();resolving.value=false;clearTimeout(timer);if(/^https:\/\//.test(partner.value.url))timer=setTimeout(resolve,900)})
onBeforeUnmount(()=>{alive=false;generation++;controller?.abort();clearTimeout(timer)})
</script>
<template>
 <form :inert="busy" class="partner-editor" @submit.prevent="emit('save')">
  <label>{{ t('experience.url') }}<div class="partner-editor__url"><input v-model="partner.url" class="input" required type="url" pattern="https://.*" /><button type="button" class="btn btn-secondary" :disabled="resolving" @click="resolve">{{ t(resolving?'experience.loading':'experience.resolveSite') }}</button></div><small>{{ t('experience.resolveHint') }}</small></label>
  <p v-if="error" role="alert" class="partner-editor__error">{{error}}</p><p v-if="warnings.length" role="status">{{t('experience.resolvePartial')}}</p>
  <div class="partner-editor__grid"><label>{{t('experience.name')}}<input v-model="partner.name" class="input" required maxlength="100" /></label><label>{{t('experience.tag')}}<input v-model="partner.tag" class="input" maxlength="40" /></label><label>{{t('experience.icon')}}<AssetInput v-model="partner.icon" /></label><label>{{t('experience.thumbnail')}}<AssetInput v-model="partner.thumbnail" /></label></div>
  <img v-if="partner.thumbnail" class="partner-editor__cover" :src="partner.thumbnail" :alt="t('experience.thumbnail')" />
  <label>{{t('experience.description')}}<textarea v-model="partner.description" class="input" maxlength="600" rows="3" /></label>
  <div class="partner-editor__grid"><label>{{t('experience.position')}}<input v-model.number="partner.position" type="number" min="0" class="input" required /></label><label class="partner-editor__enabled"><input v-model="partner.enabled" type="checkbox" />{{t('experience.enabled')}}</label></div>
  <div class="flex flex-wrap gap-3"><button class="btn btn-primary" :disabled="busy||resolving">{{t('experience.save')}}</button><button type="button" class="btn btn-secondary" @click="emit('cancel')">{{t('experience.cancel')}}</button></div>
 </form>
</template>
<style scoped>
.partner-editor{display:grid;gap:22px;border-top:1px solid var(--xg-border);padding-top:26px}.partner-editor label{display:grid;gap:10px;font-size:13px;min-width:0}.partner-editor__url{display:flex;gap:10px}.partner-editor__url input{min-width:0;flex:1}.partner-editor small,.partner-editor>p{font-size:12px;line-height:1.8;color:var(--xg-muted)}.partner-editor__grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:22px}.partner-editor__cover{width:min(100%,500px);aspect-ratio:4/3;object-fit:cover;border-radius:18px;border:1px solid var(--xg-border)}.partner-editor .partner-editor__error{color:#b91c1c}.partner-editor .partner-editor__enabled{display:flex;align-items:center;gap:10px}@media(max-width:640px){.partner-editor__grid{grid-template-columns:1fr}.partner-editor__url{flex-direction:column}}
</style>
