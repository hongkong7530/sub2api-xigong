import { onMounted, onScopeDispose, shallowRef, watch, type Ref } from 'vue'
import { siteContent, type PublicMonitorList } from '@/api/siteContent'

export const HOME_MONITOR_INTERVAL_MS = 5000

/** 一个首页列表一个读取所有者；离屏/后台停止，旧响应不能覆盖新一轮状态。 */
export function useHomeMonitors(visible: Ref<boolean>) {
  const data = shallowRef<PublicMonitorList>()
  const loading = shallowRef(false)
  const failed = shallowRef(false)
  const countdown = shallowRef(5)
  let controller: AbortController | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let ticker: ReturnType<typeof setInterval> | undefined
  let epoch = 0
  let alive = true

  function clearTimers() {
    clearTimeout(timer)
    clearInterval(ticker)
  }
  async function refresh() {
    clearTimers()
    if (!alive || !visible.value || document.hidden || controller) return
    const token = ++epoch
    const current = new AbortController()
    controller = current
    loading.value = true
    try {
      const result = await siteContent.monitors(current.signal)
      if (alive && token === epoch) {
        data.value = result
        failed.value = false
      }
    } catch {
      if (alive && token === epoch && !current.signal.aborted) {
        // 无法确认公开权限时不无限期保留渠道名；成功的空响应同样必须清空。
        data.value = undefined
        failed.value = true
      }
    } finally {
      if (token === epoch) {
        controller = undefined
        loading.value = false
        if (alive && visible.value && !document.hidden) {
          countdown.value = 5
          ticker = setInterval(() => { countdown.value = Math.max(0, countdown.value - 1) }, 1000)
          timer = setTimeout(() => { void refresh() }, HOME_MONITOR_INTERVAL_MS)
        }
      }
    }
  }
  function reconcile() {
    clearTimers()
    if (!visible.value || document.hidden) {
      epoch++
      controller?.abort()
      controller = undefined
      loading.value = false
    } else void refresh()
  }
  watch(visible, reconcile)
  onMounted(() => {
    document.addEventListener('visibilitychange', reconcile)
    reconcile()
  })
  onScopeDispose(() => {
    alive = false
    epoch++
    clearTimers()
    controller?.abort()
    document.removeEventListener('visibilitychange', reconcile)
  })
  return { data, loading, failed, countdown, refresh }
}
