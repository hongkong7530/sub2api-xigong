import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { DEFAULT_CACHE_RATE_WINDOW_SECONDS, list } from '../channelMonitor'

afterEach(() => vi.restoreAllMocks())

describe('user channel monitor cache-rate window', () => {
  it('defaults to 30 minutes and sends an explicitly selected window in seconds', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { items: [] } })

    await list()
    expect(get).toHaveBeenLastCalledWith('/channel-monitors', expect.objectContaining({
      params: { cache_window_seconds: DEFAULT_CACHE_RATE_WINDOW_SECONDS },
    }))

    await list({ cacheWindowSeconds: 21600 })
    expect(get).toHaveBeenLastCalledWith('/channel-monitors', expect.objectContaining({
      params: { cache_window_seconds: 21600 },
    }))
  })
})
