import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'
import { profitApi, type ProfitSnapshot, type ProfitWindow } from '@/api/profit'

/** Route-local polling: hidden tabs cancel requests, never overwrite newer data. */
export function useProfitDisclosure(admin = false) {
  const data = shallowRef<ProfitSnapshot | null>(null)
  const loading = shallowRef(false)
  const error = shallowRef(false)
  const window = shallowRef<ProfitWindow>('today')
  const provider = shallowRef('all')
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let epoch = 0
  let disposed = false
  const clear = () => { clearTimeout(timer); timer = undefined }
  const cancel = () => { epoch++; controller?.abort(); controller = null; loading.value = false; clear() }
  async function refresh() {
    if (disposed || document.hidden || loading.value) return
    clear(); const current = ++epoch; controller = new AbortController(); loading.value = true
    try {
      const result = await (admin ? profitApi.overview(controller.signal) : profitApi.disclosure(controller.signal))
      if (current === epoch && !disposed) { data.value = result; error.value = false }
    } catch {
      if (current === epoch && !disposed) error.value = true
    } finally {
      if (current === epoch && !disposed) {
        loading.value = false; controller = null
        if (!document.hidden) timer = setTimeout(() => { void refresh() }, 30_000)
      }
    }
  }
  const visibility = () => { if (document.hidden) cancel(); else void refresh() }
  onMounted(() => { document.addEventListener('visibilitychange', visibility); void refresh() })
  onBeforeUnmount(() => { disposed = true; cancel(); document.removeEventListener('visibilitychange', visibility) })
  const providers = computed(() => [...new Set(data.value?.groups.map(g => g.platform) ?? [])])
  const groups = computed(() => data.value?.groups.filter(g => provider.value === 'all' || g.platform === provider.value) ?? [])
  return { data, loading, error, window, provider, providers, groups, refresh }
}
