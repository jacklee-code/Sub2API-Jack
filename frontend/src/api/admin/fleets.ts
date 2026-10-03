import { apiClient } from '../client'

export interface FleetMember {
  user_id: number; email: string; username: string; role: string
  subscription_id: number; independent_expiry: boolean; active: boolean
  expires_at: string; status: string; daily_usage_usd: number; weekly_usage_usd: number
  monthly_usage_usd: number; key_count: number; owner_fleet_id?: number
}
export interface Fleet {
  id: number; name: string; group_id: number; group_name: string; platform: string
  expires_at: string; version: number; daily_limit_usd: number | null
  weekly_limit_usd: number | null; monthly_limit_usd: number | null; members: FleetMember[]
}
export interface FleetMemberInput {
  user_id: number; independent_expiry: boolean; expires_at?: string; adopt_existing: boolean
}
export interface FleetAction {
  action: 'create' | 'rename' | 'archive' | 'add' | 'remove' | 'transfer' | 'expiry' | 'member_expiry' | 'reset' | 'reset_all'
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
