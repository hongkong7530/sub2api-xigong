import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useProfitDisclosure } from '../useProfitDisclosure'
const { disclosure, overview } = vi.hoisted(() => ({ disclosure: vi.fn(), overview: vi.fn() }))
vi.mock('@/api/profit', () => ({ profitApi: { disclosure, overview } }))
const snapshot = { scope: 'public_groups', groups: [{ id: 1, platform: 'openai' }], metrics: {} }
let hidden = false
let dispose: (() => void) | undefined
beforeEach(() => { vi.useFakeTimers(); hidden = false; vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden); disclosure.mockReset().mockResolvedValue(snapshot); overview.mockReset().mockResolvedValue(snapshot) })
afterEach(() => { dispose?.(); dispose = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
function setup(admin = false) {
  let state!: ReturnType<typeof useProfitDisclosure>
  const wrapper = mount(defineComponent({ setup() { state = useProfitDisclosure(admin); return () => null } }))
  dispose = () => wrapper.unmount()
  return state
}
describe('profit reads', () => {
  it('loads once and polls only every 30 seconds', async () => {
    const state = setup(); await flushPromises(); expect(state.data.value).toEqual(snapshot)
    await vi.advanceTimersByTimeAsync(29999); expect(disclosure).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1); expect(disclosure).toHaveBeenCalledTimes(2)
    state.provider.value = 'other'; expect(state.groups.value).toEqual([])
  })
  it('pauses hidden work, rejects late completion and clears all tasks on disposal', async () => {
    let finish!: (v: typeof snapshot) => void
    disclosure.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const state = setup(); const signal = disclosure.mock.calls[0][0] as AbortSignal
    hidden = true; document.dispatchEvent(new Event('visibilitychange')); expect(signal.aborted).toBe(true)
    hidden = false; document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    finish({ ...snapshot, groups: [] }); await flushPromises(); expect(state.data.value).toEqual(snapshot)
    dispose?.(); dispose = undefined; await vi.advanceTimersByTimeAsync(60000)
    expect(disclosure).toHaveBeenCalledTimes(2); expect(vi.getTimerCount()).toBe(0)
  })
  it('never overlaps requests and preserves stale data visibly on failure', async () => {
    const state = setup(); await flushPromises()
    disclosure.mockRejectedValueOnce(new Error('db unavailable')); await vi.advanceTimersByTimeAsync(30000)
    expect(state.error.value).toBe(true); expect(state.data.value).toEqual(snapshot)
    let finish!: (v: typeof snapshot) => void
    disclosure.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    void state.refresh(); void state.refresh(); expect(disclosure).toHaveBeenCalledTimes(3)
    finish(snapshot); await flushPromises(); expect(state.error.value).toBe(false)
  })
  it('uses the separate admin endpoint, never public cache as a fallback', async () => {
    setup(true); await flushPromises(); expect(overview).toHaveBeenCalledTimes(1); expect(disclosure).not.toHaveBeenCalled()
  })
})
