import { apiClient } from '../client'

export interface FleetMember {
  user_id: number; email: string; username: string; role: string
  subscription_id: number; independent_expiry: boolean; active: boolean
  expires_at: string; status: string; daily_usage_usd: number; weekly_usage_usd: number
  monthly_usage_usd: number; key_count: number; owner_fleet_id?: number
}
export interface FleetResetSync {
  enabled: boolean; account_id: number; generation: number; configured_at: string
  next_reset_at: string | null; observed_at: string | null
  status: string; error_code?: string
}
export interface FleetResetEvent {
  id: number; account_id: number; source: string; members: number; created_at: string; cache_pending: boolean
  before?: { used: number; reset_at: string }; after?: { used: number; reset_at: string }
}
export interface Fleet {
  reset_sync?: FleetResetSync

  id: number; name: string; group_id: number; group_name: string; platform: string
  expires_at: string; version: number; daily_limit_usd: number | null
  weekly_limit_usd: number | null; monthly_limit_usd: number | null; members: FleetMember[]
}
export interface FleetMemberInput {
  user_id: number; independent_expiry: boolean; expires_at?: string; adopt_existing: boolean
}
export interface FleetAction {
  action: 'reset_sync' | 'create' | 'rename' | 'archive' | 'add' | 'remove' | 'transfer' | 'expiry' | 'member_expiry' | 'reset' | 'reset_all'
  sync_enabled?: boolean; sync_account_id?: number
  fleet_id?: number; target_fleet_id?: number; name?: string; group_id?: number
  expires_at?: string; days?: number; members?: FleetMemberInput[]; user_id?: number
  daily?: boolean; weekly?: boolean; monthly?: boolean; versions?: Record<number, number>
}
export async function list(): Promise<Fleet[]> {
  return (await apiClient.get<Fleet[]>('/admin/fleets')).data
}
export async function preview(groupId: number): Promise<FleetMember[]> {
  return (await apiClient.get<FleetMember[]>(`/admin/fleets/preview/${groupId}`)).data
}
export async function mutate(input: FleetAction, key: string): Promise<{ fleet_id: number; cache_pending: boolean; replayed: boolean }> {
  return (await apiClient.post('/admin/fleets/actions', input, { headers: { 'Idempotency-Key': key } })).data
}

export async function resetSources(id: number): Promise<Array<{ id: number; name: string }>> {
  return (await apiClient.get(`/admin/fleets/${id}/reset-sources`)).data
}
export async function resetEvents(id: number, before = 0): Promise<FleetResetEvent[]> {
  return (await apiClient.get(`/admin/fleets/${id}/reset-events`, { params: { before } })).data
}
