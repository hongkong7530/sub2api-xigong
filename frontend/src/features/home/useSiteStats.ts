import { onMounted, onScopeDispose, shallowRef } from 'vue'
import { siteContent, type SiteStats } from '@/api/siteContent'

export const SITE_STATS_INTERVAL_MS = 3000

/** 单一轮询所有者：三秒采样，不重叠；隐藏时停止，恢复可见后立即读取。 */
export function useSiteStats() {
  const data = shallowRef<SiteStats>()
  const failed = shallowRef(false)
  const revision = shallowRef(0)
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | undefined
  let alive = true
  let epoch = 0

  async function refresh() {
    clearTimeout(timer)
    if (!alive || document.hidden || controller) return
    const token = ++epoch
    const current = new AbortController()
    controller = current
    try {
      const next = await siteContent.stats(current.signal)
      if (alive && token === epoch) {
        const previous = data.value
        // 不可用数值保留最后有效采样；模型名不可跨越服务端的公开权限边界。
        data.value = {
          ...next,
          usage: next.usage ?? (next.usage_stale && previous?.usage ? { ...previous.usage, models: [] } : null),
          online_users: next.online_users ?? previous?.online_users ?? null,
          in_flight: next.in_flight ?? previous?.in_flight ?? null,
        }
        failed.value = Boolean(next.usage_stale || next.usage === null || next.online_users === null || next.in_flight === null)
        revision.value++
      }
    } catch {
      if (alive && token === epoch && !current.signal.aborted) failed.value = true
    } finally {
      if (token === epoch) {
        controller = undefined
        if (alive && !document.hidden) timer = setTimeout(refresh, SITE_STATS_INTERVAL_MS)
      }
    }
  }
  function visibilityChanged() {
    clearTimeout(timer)
    if (document.hidden) {
      epoch++
      controller?.abort()
      controller = undefined
    } else void refresh()
  }
  onMounted(() => {
    document.addEventListener('visibilitychange', visibilityChanged)
    void refresh()
  })
  onScopeDispose(() => {
    alive = false
    epoch++
    clearTimeout(timer)
    controller?.abort()
    document.removeEventListener('visibilitychange', visibilityChanged)
  })
  return { data, failed, revision }
}
