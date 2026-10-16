import { apiClient } from './client'

export type IdentityCode = 'bonus_5' | 'bonus_10' | 'ahhilai'
export interface IdentityGroup { code: IdentityCode; name: string; category: string; member_count: number }
export interface IdentityMember { user_id: number; username: string; email: string; identity_groups: IdentityCode[] | null }
export interface IdentityRate { group_id: number; name: string; default_multiplier: number; rate_multiplier: number | null }
export interface RechargeSnapshot { identity_code: string; base_amount: number; bonus_rate: number; bonus_amount: number; credited_amount: number }

const names: Record<string, string> = { bonus_5: '5%额外', bonus_10: '10%额外', ahhilai: 'AHHilAI' }
export const identityName = (code: string) => names[code] ?? code

export async function listIdentityGroups() {
  return (await apiClient.get<IdentityGroup[]>('/admin/identity-groups')).data
}
export async function listIdentityMembers(code: IdentityCode, search: string, page: number, all = false) {
  return (await apiClient.get<{ items: IdentityMember[]; total: number }>(
    all ? '/admin/identity-groups/users' : `/admin/identity-groups/${code}/members`,
    { params: { search, page, page_size: 20 } }
  )).data
}
export async function changeIdentityMembers(code: IdentityCode, userIds: number[], action: 'add' | 'remove') {
  await apiClient.put(`/admin/identity-groups/${code}/members`, { user_ids: userIds, action })
}
export async function getIdentityRates() {
  return (await apiClient.get<IdentityRate[]>('/admin/identity-groups/ahhilai/rates')).data
}
export async function updateIdentityRates(rates: Pick<IdentityRate, 'group_id' | 'rate_multiplier'>[]) {
  await apiClient.put('/admin/identity-groups/ahhilai/rates', { rates })
}
export async function getMyIdentities() {
  return (await apiClient.get<{ identity_groups: IdentityCode[]; bonus_rate: number }>('/user/identity-groups')).data
}
