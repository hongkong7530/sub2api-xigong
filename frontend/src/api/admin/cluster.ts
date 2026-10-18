import { apiClient } from '../client'

export interface ClusterNodeConfig {
  cluster_id: string
  node_id: string
  name: string
  roles: string[]
  capabilities: string[]
  address: string
  certificate_sha256: string
  state: string
  revision: number
  apply_error?: string
  base_weight: number
  max_in_flight: number
  max_queue: number
  db_connections: number
  monthly_limit_bytes: number | null
  traffic_reserve_bytes: number
  traffic_cycle_day: number
  traffic_timezone: string
  traffic_source: string
  version: string
}
export interface ClusterNodeStatus {
  node_id: string
  boot_id: string
  sampled_at: string
  active: number
  waiting: number
  pending_settlement: number
  queue_wait_ms: number
  db_ok: boolean
  redis_ok: boolean
  admission_ready: boolean
  observed_revision: number
  raw_traffic_bytes: number | null
  cycle_traffic_bytes: number | null
  db_connections: number
  db_pool_max?: number
  db_pool_idle?: number
  redis_pool_max?: number
  redis_pool_total?: number
  redis_pool_idle?: number
  version: string
  // Reason this node is withheld from admission (version/role/capability drift,
  // authentication refusal, traffic budget). Published by the runtime so a silent
  // removal cannot masquerade as an ordinary 503.
  admission_blocked_by?: string
}
export interface ClusterNodeView { config: ClusterNodeConfig; status: ClusterNodeStatus | null; fresh: boolean; applied_revision: number; suggested_weight: number }
export interface ClusterOverview {
  nodes: ClusterNodeView[]
  response_node_id?: string
  database_capacity?: { budget: number; allocated: number; available: number; server_max: number | null; server_used: number | null; server_reserved: number | null }
  shadow: boolean
  data_authority: 'single_primary'
  control_url: string
  redis_generation: string
  redis_recovery_required: boolean
}
export interface ClusterOperation {
  id: string
  node_id: string
  request_key: string
  action: string
  target_revision: number
  status: string
  error_code?: string
  created_at: string
}
export interface StorageTable { name: string; bytes: number; estimated_rows: number; dead_rows: number; retention_class: string }
export interface ClusterStorage { sampled_at: string; database_bytes: number; tables: StorageTable[]; ledger_detached: boolean; deletion_enabled: boolean }
export interface RetentionPreview { sampled_at: string; cutoff: string; days: number; table: string; rows: number; estimated_reusable_bytes: number; filesystem_bytes_guaranteed: number; can_apply: boolean }
const base = '/admin/ops/cluster-v3'
export async function getOverview(signal?: AbortSignal): Promise<ClusterOverview> { return (await apiClient.get<ClusterOverview>(base, { signal })).data }
export async function getOperations(signal?: AbortSignal): Promise<ClusterOperation[]> { return (await apiClient.get<ClusterOperation[]>(`${base}/operations`, { signal })).data }
export async function getOperation(requestKey: string): Promise<ClusterOperation> { return (await apiClient.get<ClusterOperation>(`${base}/operations`, { params: { request_key: requestKey } })).data }
export async function getStorage(signal?: AbortSignal): Promise<ClusterStorage> { return (await apiClient.get<ClusterStorage>(`${base}/storage`, { signal })).data }
export async function previewRetention(table: string, days: number, signal?: AbortSignal): Promise<RetentionPreview> { return (await apiClient.get<RetentionPreview>(`${base}/retention-preview`, { params: { table, days }, signal })).data }
export async function registerNode(node: ClusterNodeConfig): Promise<{ node_id: string; join_token: string; expires_in_seconds: number }> { return (await apiClient.post(`${base}/nodes`, node)).data }
export async function changeNode(nodeId: string, change: { request_key: string; expected_revision: number; action: string; confirm_force?: boolean; node?: ClusterNodeConfig }): Promise<ClusterOperation> {
  return (await apiClient.post(`${base}/nodes/${encodeURIComponent(nodeId)}/operations`, change)).data
}
export async function acknowledgeRedis(generation: string): Promise<void> { await apiClient.post(`${base}/redis-recovery`, { generation, confirm: true }) }
