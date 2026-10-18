import { defineStore } from 'pinia'
import { ref } from 'vue'
import { siteContent, type Appearance } from '@/api/siteContent'
export const useExperienceStore = defineStore('experience', () => {
 const appearance = ref<Appearance | null>(null)
 const error = ref(false)
 const loaded = ref(false)
 const loading = ref(false)
 let pending: Promise<void> | null = null
 function load(force = false): Promise<void> {
  if (pending) return pending
  if (loaded.value && !force) return Promise.resolve()
  loading.value = true
  error.value = false
  pending = siteContent.appearance().then(value => { appearance.value = value; error.value = false; loaded.value = true }).catch(() => { error.value = true }).finally(() => { loading.value = false; pending = null })
  return pending
 }
 return { appearance, error, loaded, loading, load }
})
