import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, shallowRef } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useHomeMonitors } from '../useHomeMonitors'
const { monitors } = vi.hoisted(() => ({ monitors: vi.fn() }))
vi.mock('@/api/siteContent', () => ({ siteContent: { monitors } }))
const snapshot = { enabled: true, items: [{ id: 1, name: 'Channel', primary_status: 'failed' }], sampled_at: '2026-10-16T12:00:00Z', refresh_seconds: 5 }
let hidden = false
let dispose: (() => void) | undefined
beforeEach(() => {
  vi.useFakeTimers()
  hidden = false
  vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
  monitors.mockReset().mockResolvedValue(snapshot)
})
afterEach(() => { dispose?.(); dispose = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
function setup(initiallyVisible = true) {
  const visible = shallowRef(initiallyVisible)
  let state!: ReturnType<typeof useHomeMonitors>
  const wrapper = mount(defineComponent({ setup() { state = useHomeMonitors(visible); return () => null } }))
  dispose = () => wrapper.unmount()
  return { visible, state }
}
describe('public homepage monitor reads', () => {
  it('loads only near the viewport and refreshes every five seconds without overlap', async () => {
    const { visible, state } = setup(false)
    await vi.advanceTimersByTimeAsync(10000)
    expect(monitors).not.toHaveBeenCalled()
    visible.value = true; await flushPromises()
    expect(monitors).toHaveBeenCalledTimes(1)
    expect(state.data.value).toEqual(snapshot)
    await vi.advanceTimersByTimeAsync(4999); expect(monitors).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1); expect(monitors).toHaveBeenCalledTimes(2)
  })
  it('honours feature-off and empty responses and clears inaccessible names on failure', async () => {
    const { state } = setup(); await flushPromises()
    monitors.mockResolvedValueOnce({ ...snapshot, enabled: false, items: [] })
    await vi.advanceTimersByTimeAsync(5000)
    expect(state.data.value?.items).toEqual([])
    monitors.mockRejectedValueOnce(new Error('unavailable'))
    await vi.advanceTimersByTimeAsync(5000)
    expect(state.data.value).toBeUndefined()
    expect(state.failed.value).toBe(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(state.data.value).toEqual(snapshot)
    expect(state.failed.value).toBe(false)
  })
  it('cancels hidden requests, ignores late responses and disposes all timers', async () => {
    let finish!: (value: typeof snapshot) => void
    monitors.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const { state, visible } = setup()
    await vi.advanceTimersByTimeAsync(15000)
    expect(monitors).toHaveBeenCalledTimes(1)
    const signal = monitors.mock.calls[0][0] as AbortSignal
    hidden = true; document.dispatchEvent(new Event('visibilitychange'))
    expect(signal.aborted).toBe(true)
    hidden = false; document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    finish({ ...snapshot, items: [] }); await flushPromises()
    expect(state.data.value?.items).toEqual(snapshot.items)
    visible.value = false; await flushPromises()
    await vi.advanceTimersByTimeAsync(15000); expect(monitors).toHaveBeenCalledTimes(2)
    dispose?.(); dispose = undefined
    await vi.advanceTimersByTimeAsync(15000); expect(monitors).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
  })
})
