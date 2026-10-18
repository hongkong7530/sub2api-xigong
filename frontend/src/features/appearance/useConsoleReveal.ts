import { onScopeDispose, watch, type Ref } from 'vue'

const protectedArea = 'a,button,input,select,textarea,label,nav,header,aside,table,pre,article,form,dialog,[role="dialog"],[data-scene-protect],.card,.scene-surface,.xg-glass,.glass-card,.card-glass,.docs-main,.docs-sidebar,.docs-toc,h1,h2,h3,p'

/** 留白才透视；监听只在许可期间存在，帧任务不跨路由或失焦存活。 */
export function useConsoleReveal(host: Ref<HTMLElement | undefined>, allowed: Ref<boolean>, identity: Ref<string>) {
  let frame = 0
  let cleanup: (() => void) | undefined
  function clear() {
    cancelAnimationFrame(frame); frame = 0
    host.value?.removeAttribute('data-revealing')
    host.value?.style.removeProperty('--reveal-x')
    host.value?.style.removeProperty('--reveal-y')
  }
  watch([host, allowed], ([element, active]) => {
    cleanup?.(); cleanup = undefined; clear()
    if (!element || !active) return
    const move = (event: PointerEvent) => {
      if (event.pointerType === 'touch' || (event.target as Element | null)?.closest(protectedArea)) { clear(); return }
      const x = event.clientX, y = event.clientY
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        frame = 0
        // 只测场景自身，不逐帧扫描所有卡片的布局。
        const rect = element.classList.contains('experience-shell--preview') ? element.getBoundingClientRect() : { left: 0, top: 0 }
        element.style.setProperty('--reveal-x', `${x - rect.left}px`)
        element.style.setProperty('--reveal-y', `${y - rect.top}px`)
        element.dataset.revealing = 'true'
      })
    }
    element.addEventListener('pointermove', move, { passive: true })
    element.addEventListener('pointerleave', clear)
    window.addEventListener('blur', clear)
    window.addEventListener('scroll', clear, true)
    document.addEventListener('visibilitychange', clear)
    cleanup = () => {
      element.removeEventListener('pointermove', move)
      element.removeEventListener('pointerleave', clear)
      window.removeEventListener('blur', clear)
      window.removeEventListener('scroll', clear, true)
      document.removeEventListener('visibilitychange', clear)
    }
  }, { immediate: true, flush: 'post' })
  watch(identity, clear)
  onScopeDispose(() => { cleanup?.(); clear() })
  return { clear }
}
