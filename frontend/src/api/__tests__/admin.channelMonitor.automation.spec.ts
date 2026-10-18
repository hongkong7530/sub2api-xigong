import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

import { autoProvisionPublicGroups } from '../admin/channelMonitor'

const report = {
  groups_scanned: 13,
  monitors_created: 0,
  plaza_channels_created: 0,
  plaza_models_added: 0,
  items: [],
}

beforeEach(() => post.mockReset())

describe('public group automation API', () => {
  it('uses a dedicated timeout rather than the normal 30-second request budget', async () => {
    post.mockResolvedValue({ data: report })
    expect(await autoProvisionPublicGroups()).toEqual(report)
    expect(post).toHaveBeenCalledWith('/admin/channel-monitors/auto-provision', undefined, {
      timeout: 120000,
    })
  })

  it('preserves partial-action reports and does not retry ambiguous failures', async () => {
    const partial = { ...report, items: [{ group_id: 26, group_name: 'test', action: 'monitor_error', detail: 'provision failed' }] }
    post.mockResolvedValueOnce({ data: partial })
    expect(await autoProvisionPublicGroups()).toEqual(partial)
    const failure = new Error('automation is already running')
    post.mockRejectedValueOnce(failure)
    await expect(autoProvisionPublicGroups()).rejects.toBe(failure)
    expect(post).toHaveBeenCalledTimes(2)
  })
})
