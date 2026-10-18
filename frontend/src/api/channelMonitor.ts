/**
 * User-facing Channel Monitor API endpoints
 * Read-only views for end users to inspect channel availability/status.
 */

import { apiClient } from './client'
import type { MonitorQuotaSnapshot, Provider, MonitorStatus } from './admin/channelMonitor'

export type { Provider, MonitorStatus } from './admin/channelMonitor'

export interface UserMonitorExtraModel {
  model: string
  status: MonitorStatus
  latency_ms: number | null
  /** 最近一次首字延迟（TTFT）毫秒；非流式探测为 null。 */
  first_token_ms: number | null
}

export interface MonitorTimelinePoint {
  model?: string
  source?: 'probe' | 'user_request'
  status: MonitorStatus
  latency_ms: number | null
  first_token_ms: number | null
  output_tokens_per_second?: number | null
  ping_latency_ms?: number | null
  checked_at: string
}

/** 分组倍率变更历史单点。 */
export interface UserGroupRatePoint {
  rate_multiplier: number
  effective_rate_multiplier: number
  recorded_at: string
}

/** 分组绑定监控的首字延迟样本（详情响应附带）。 */
export interface UserFirstTokenSample {
  model: string
  first_token_ms: number | null
  created_at: string
}

export interface UserGroupInsight {
  /** 分组配置的基础倍率（精确到四位）。 */
  rate_multiplier: number
  /** 当前计费有效倍率 = 基础倍率 × 当前应用的高峰因子。 */
  effective_rate_multiplier: number
  peak_rate_enabled: boolean
  peak_rate_multiplier: number
  applied_peak_multiplier: number
  cache_hit_rate: number | null
  cache_sample_count?: number
  cache_window_seconds?: number
  rate_history?: UserGroupRatePoint[]
  recent_first_tokens?: UserFirstTokenSample[]
}

export interface UserMonitorView {
  id: number
  name: string
  provider: Provider
  group_name: string
  /** 绑定分组 ID（订阅通知以此为键）；未绑定为 null。 */
  group_id: number | null
  primary_model: string
  primary_status: MonitorStatus
  status_source?: 'probe' | 'user_request'
  status_observed_at?: string | null
  /** 全部被监控模型的最优状态（一绿即绿、双黄为黄、双红为红），整卡颜色用。 */
  overall_status: MonitorStatus | ''
  primary_latency_ms: number | null
  /** 主模型最近一次首字延迟（TTFT）毫秒。 */
  primary_first_token_ms: number | null
  primary_output_tokens_per_second?: number | null
  primary_ping_latency_ms?: number | null
  availability_7d: number | null
  extra_models: UserMonitorExtraModel[]
  timeline: MonitorTimelinePoint[]
  /**
   * 主模型最近配额快照。仅当系统开启 channel_monitor_show_quota 时
   * 服务端才会下发（关闭时服务端已剥离，前端 flag 仅作纵深防御）。
   */
  latest_quota?: MonitorQuotaSnapshot | null
  /** 分组绑定监控的分组实时指标；未绑定分组为空。 */
  group_insight?: UserGroupInsight | null
}

export interface UserMonitorListResponse {
  items: UserMonitorView[]
}

export interface UserMonitorModelDetail {
  model: string
  latest_status: MonitorStatus
  latest_latency_ms: number | null
  /** 最近一次首字延迟（TTFT）毫秒。 */
  latest_first_token_ms: number | null
  latest_output_tokens_per_second?: number | null
  availability_7d: number | null
  availability_15d: number | null
  availability_30d: number | null
  avg_latency_7d_ms: number | null
}

export interface UserMonitorDetail {
  group_availability?: Record<string, number | null>
  id: number
  name: string
  provider: Provider
  group_name: string
  models: UserMonitorModelDetail[]
  /** 分组实时指标 + 倍率变更历史（画曲线用）。 */
  group_insight?: UserGroupInsight | null
}

/** 用户对分组监控的邮件订阅。 */
export interface MonitorSubscription {
  group_id: number
  notify_first_token: boolean
  notify_rate_change: boolean
}

export interface MonitorSubscriptionListResponse {
  items: MonitorSubscription[]
}

/**
 * List all monitor views available to the current user.
 */
export async function list(options?: { signal?: AbortSignal }): Promise<UserMonitorListResponse> {
  const { data } = await apiClient.get<UserMonitorListResponse>('/channel-monitors', {
    signal: options?.signal,
  })
  return data
}

/**
 * Get detailed status (multi-window availability + latency) for a single monitor.
 */
export async function status(id: number): Promise<UserMonitorDetail> {
  const { data } = await apiClient.get<UserMonitorDetail>(`/channel-monitors/${id}/status`)
  return data
}

/** 当前用户全部分组订阅。 */
export async function listSubscriptions(): Promise<MonitorSubscriptionListResponse> {
  const { data } = await apiClient.get<MonitorSubscriptionListResponse>('/channel-monitor-subscriptions')
  return data
}

/** 设置某分组订阅开关；两开关全关等价于退订。 */
export async function setSubscription(
  groupId: number,
  input: { notify_first_token: boolean; notify_rate_change: boolean },
): Promise<MonitorSubscription> {
  const { data } = await apiClient.put<MonitorSubscription>(
    `/channel-monitor-subscriptions/${groupId}`,
    input,
  )
  return data
}

export async function createGroupReport(
  groupId: number,
  input: { category: string; body: string },
): Promise<{ id: number; group_id: number; category: string; submitted: boolean }> {
  const { data } = await apiClient.post(`/channel-monitor-groups/${groupId}/reports`, input)
  return data
}

export async function listSubmittedReports(): Promise<{ group_ids: number[] }> {
  const { data } = await apiClient.get<{ group_ids: number[] }>('/channel-monitor-groups/reports/submitted')
  return data
}

export const channelMonitorUserAPI = {
  list,
  status,
  listSubscriptions,
  setSubscription,
  createGroupReport,
  listSubmittedReports,
}

export default channelMonitorUserAPI
