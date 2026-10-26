import { apiClient } from './client'

export interface ProfitMetric {
  requests: number
  known_requests: number
  revenue: string
  known_revenue: string
  cost?: string
  profit: string
  margin: string | null
  coverage: string | null
}
export type ProfitWindow = 'total' | 'today' | 'week'
export type ProfitWindows = Record<ProfitWindow, ProfitMetric>
export interface ProfitGroup {
  id: number; name: string; platform: string; status: string; rate: string
  peak_enabled: boolean; peak_start: string; peak_end: string; peak_multiplier: string
  at_cost: boolean; reference_margin: string | null
  cost_multiplier?: string | null; cost_version?: number; effective_at?: string | null
  metrics: ProfitWindows
}
export interface ProfitSnapshot {
  scope: 'public_groups' | 'all_groups'; timezone: string; started_at: string; as_of: string | null; stale: boolean
  metrics: ProfitWindows; balance: ProfitMetric; subscription: ProfitMetric; groups: ProfitGroup[]
}
export interface ProfitContribution {
  started_at: string; as_of: string | null; stale: boolean
  total: ProfitMetric; balance: ProfitMetric; subscription: ProfitMetric
}
export interface ProfitEntry {
  id: number; user_id: number; group_id: number; group_name: string; platform: string; billing_type: number
  settled_at: string; base: string; revenue: string; cost: string | null; profit: string | null; cost_multiplier: string | null
}
export interface ProfitEntryPage { items: ProfitEntry[]; page: number; has_more: boolean }
export const profitApi = {
  disclosure: async (signal?: AbortSignal) => (await apiClient.get<ProfitSnapshot>('/profit/disclosure', { signal, timeout: 8000 })).data,
  overview: async (signal?: AbortSignal) => (await apiClient.get<ProfitSnapshot>('/admin/profit/overview', { signal, timeout: 8000 })).data,
  contribution: async (signal?: AbortSignal) => (await apiClient.get<ProfitContribution>('/user/profit-contribution', { signal, timeout: 8000 })).data,
  setCost: async (id: number, multiplier: string | null, version: number) => (await apiClient.put<{ version: number; multiplier: string | null; effective_at: string }>(`/admin/profit/groups/${id}/cost`, { multiplier, version })).data,
  entries: async (params: { page: number; group_id?: number; from?: string; to?: string }, signal?: AbortSignal) => (await apiClient.get<ProfitEntryPage>('/admin/profit/entries', { params, signal, timeout: 8000 })).data,
}
