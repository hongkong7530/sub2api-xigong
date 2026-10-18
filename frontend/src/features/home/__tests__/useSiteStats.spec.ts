import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { useSiteStats } from '../useSiteStats'
const { stats } = vi.hoisted(() => ({ stats: vi.fn() }))
vi.mock('@/api/siteContent', () => ({ siteContent: { stats } }))
const snapshot = { usage: { requests: 10 }, online_users: 2, in_flight: 1, sampled_at: '2026-10-16T08:00:00Z' }
let hidden = false
let dispose: (() => void) | undefined
beforeEach(() => {
 vi.useFakeTimers()
 hidden = false
 vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
 stats.mockReset().mockResolvedValue(snapshot)
})
afterEach(() => { dispose?.(); dispose = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
function setup() {
 let state!: ReturnType<typeof useSiteStats>
 const wrapper = mount(defineComponent({ setup() { state = useSiteStats(); return () => null } }))
 dispose = () => wrapper.unmount()
 return state
}
describe('three-second site stats ownership', () => {
 it('refreshes every three seconds and retains the last value on failure', async () => {
  const state = setup(); await flushPromises()
  expect(stats).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(2999); expect(stats).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1); expect(stats).toHaveBeenCalledTimes(2)
  stats.mockRejectedValueOnce(new Error('offline'))
  await vi.advanceTimersByTimeAsync(3000)
  expect(state.failed.value).toBe(true); expect(state.data.value).toEqual(snapshot)
  await vi.advanceTimersByTimeAsync(3000); expect(state.failed.value).toBe(false)
 })
 it('does not overlap requests, cancels on hide, and resumes immediately', async () => {
  let finish!: (value: typeof snapshot) => void
  stats.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const state = setup(); await vi.advanceTimersByTimeAsync(9000)
  expect(stats).toHaveBeenCalledTimes(1)
  const signal = stats.mock.calls[0][0] as AbortSignal
  hidden = true; document.dispatchEvent(new Event('visibilitychange'))
  expect(signal.aborted).toBe(true)
  hidden = false; document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
  expect(stats).toHaveBeenCalledTimes(2)
  finish({ ...snapshot, online_users: 99 }); await flushPromises()
  expect(state.data.value?.online_users).toBe(2)
  dispose?.(); dispose = undefined
  await vi.advanceTimersByTimeAsync(9000); expect(stats).toHaveBeenCalledTimes(2)
 })
 it('keeps numeric samples but clears model names when a successful response has no available data', async () => {
  const available = { ...snapshot, usage: { requests: 10, models: [{ model: 'public-model', tokens: 100, requests: 10 }] } }
  stats.mockResolvedValueOnce(available)
  const state = setup(); await flushPromises()
  stats.mockResolvedValueOnce({ ...snapshot, usage: null, online_users: null, in_flight: null, usage_stale: true })
  await vi.advanceTimersByTimeAsync(3000)
  expect(state.data.value?.usage?.requests).toBe(10)
  expect(state.data.value?.online_users).toBe(2)
  expect(state.data.value?.in_flight).toBe(1)
  expect(state.data.value?.usage?.models).toEqual([])
  expect(state.failed.value).toBe(true)
  stats.mockResolvedValueOnce({ ...available, usage: { requests: 12, models: [] } })
  await vi.advanceTimersByTimeAsync(3000)
  expect(state.data.value?.usage?.requests).toBe(12)
  expect(state.data.value?.usage?.models).toEqual([])
  expect(state.failed.value).toBe(false)
 })
 it('keeps genuine zeroes and honours a non-stale explicit empty usage result', async () => {
  const state = setup(); await flushPromises()
  stats.mockResolvedValueOnce({ ...snapshot, usage: null, online_users: 0, in_flight: 0, usage_stale: false })
  await vi.advanceTimersByTimeAsync(3000)
  expect(state.data.value?.usage).toBeNull()
  expect(state.data.value?.online_users).toBe(0)
  expect(state.data.value?.in_flight).toBe(0)
  expect(state.failed.value).toBe(true)
 })
 it('marks a server stale snapshot as stale without discarding it', async () => {
  stats.mockResolvedValueOnce({ ...snapshot, usage_stale: true })
  const state = setup(); await flushPromises()
  expect(state.failed.value).toBe(true); expect(state.data.value?.online_users).toBe(2)
 })
})
