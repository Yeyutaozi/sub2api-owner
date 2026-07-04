import { apiClient } from '../client'
import type { PaginatedTokenRewardClaims, TokenRewardClaim, TokenRewardConfig } from '../tokenRewards'

export interface AdminTokenRewardClaim extends TokenRewardClaim {
  user_email: string
}

export interface PaginatedAdminTokenRewardClaims extends Omit<PaginatedTokenRewardClaims, 'items'> {
  items: AdminTokenRewardClaim[]
  stats: AdminTokenRewardClaimStats
}

export interface AdminTokenRewardClaimStats {
  total_claims: number
  unique_users: number
  total_reward_balance: number
  total_token_snapshot: number
}

export interface AdminTokenRewardClaimQuery {
  page?: number
  page_size?: number
  search?: string
  user_id?: number
  tier_id?: string
  cycle_type?: 'weekly' | 'monthly'
  claimed_from?: string
  claimed_to?: string
}

export async function getConfig(): Promise<TokenRewardConfig> {
  const { data } = await apiClient.get<TokenRewardConfig>('/admin/token-rewards/config')
  return data
}

export async function updateConfig(config: TokenRewardConfig): Promise<TokenRewardConfig> {
  const { data } = await apiClient.put<TokenRewardConfig>('/admin/token-rewards/config', config)
  return data
}

export async function listClaims(params: AdminTokenRewardClaimQuery = {}): Promise<PaginatedAdminTokenRewardClaims> {
  const { data } = await apiClient.get<PaginatedAdminTokenRewardClaims>('/admin/token-rewards/claims', { params })
  return data
}

export const tokenRewardsAPI = {
  getConfig,
  updateConfig,
  listClaims
}

export default tokenRewardsAPI
