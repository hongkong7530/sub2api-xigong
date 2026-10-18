import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useConsoleReveal } from '../useConsoleReveal'

afterEach(() => vi.unstubAllGlobals())
describe('open-space pointer reveal', () => {
  it('clears on content entry, scroll, route change, disable and disposal', async () => {
    let pending: FrameRequestCallback | undefined
    vi.stubGlobal('requestAnimationFrame', (fn: FrameRequestCallback) => { pending = fn; return 1 })
    vi.stubGlobal('cancelAnimationFrame', () => { pending = undefined })
    const element = document.createElement('div')
    element.innerHTML = '<div class="card"><button>safe</button></div><div class="open"></div>'
    document.body.append(element)
    const host = ref<HTMLElement>(element), allowed = ref(true), route = ref('/one')
    const scope = effectScope(); scope.run(() => useConsoleReveal(host, allowed, route))
    await nextTick()
    const move = (target: Element) => { target.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: 100, clientY: 80 })); pending?.(0) }
    move(element.querySelector('.open')!); expect(element.dataset.revealing).toBe('true')
    move(element.querySelector('button')!); expect(element.dataset.revealing).toBeUndefined()
    move(element); window.dispatchEvent(new Event('scroll')); expect(element.dataset.revealing).toBeUndefined()
    move(element); route.value = '/two'; await nextTick(); expect(element.dataset.revealing).toBeUndefined()
    move(element); allowed.value = false; await nextTick(); expect(element.dataset.revealing).toBeUndefined()
    move(element); expect(element.dataset.revealing).toBeUndefined()
    allowed.value = true; await nextTick(); move(element); scope.stop()
    expect(element.dataset.revealing).toBeUndefined(); expect(pending).toBeUndefined()
    element.remove()
  })
})
