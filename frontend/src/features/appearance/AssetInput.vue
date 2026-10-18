<script setup lang="ts">
import { shallowRef, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { siteContent } from '@/api/siteContent'
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const busy = shallowRef(false), error = shallowRef('')
let generation = 0
async function upload(event: Event) {
 const input = event.target as HTMLInputElement, file = input.files?.[0]
 const token = ++generation
 busy.value = false
 if (!file) return
 if (!['image/jpeg','image/png'].includes(file.type) || file.size > 8 * 1024 * 1024) { error.value = t('experience.uploadLimit'); return }
 busy.value = true; error.value = ''
 try { const asset = await siteContent.uploadAsset(file); if (token === generation) model.value = asset.url }
 catch { if (token === generation) error.value = t('experience.uploadError') }
 finally { if (token === generation) busy.value = false; input.value = '' }
}
function edit(event: Event) { generation++; busy.value = false; model.value = (event.target as HTMLInputElement).value }
onBeforeUnmount(() => generation++)
</script>
<template>
 <div class="asset-input" :aria-busy="busy">
  <input :value="model" class="input" type="text" :aria-label="t('experience.imageAddress')" maxlength="2048" placeholder="https://…" @input="edit" />
  <label class="btn btn-secondary asset-input__upload"><input type="file" accept="image/jpeg,image/png" :disabled="busy" :aria-label="t('experience.uploadImage')" @change="upload" />{{ t(busy ? 'experience.loading' : 'experience.uploadImage') }}</label>
  <button v-if="model" class="btn btn-secondary" type="button" @click="generation++; model = ''; busy = false">{{ t('experience.clearImage') }}</button>
  <small>{{ t('experience.uploadLimit') }}</small><small v-if="error" role="alert" class="asset-input__error">{{ error }}</small>
 </div>
</template>
<style scoped>
.asset-input{display:flex;flex-wrap:wrap;gap:10px;min-width:0}.asset-input>.input{flex:1 1 100%;min-width:0}.asset-input__upload{position:relative;overflow:hidden;cursor:pointer;min-height:44px}.asset-input__upload input{position:absolute;inset:0;opacity:0;width:100%;cursor:pointer}.asset-input small{flex-basis:100%;font-size:12px;line-height:1.7;color:var(--xg-muted)}.asset-input .asset-input__error{color:#b91c1c}
</style>
