import { apiClient } from './client'

export type TokenRewardCycleType = 'weekly' | 'monthly'
export type TokenRewardTierState = 'locked' | 'claimable' | 'claimed'
export type TokenRewardTokenUnit = 'raw' | 'K' | 'M' | 'B' | 'T'

export interface TokenRewardTier {
  id: string
  tokens: number
  token_unit: TokenRewardTokenUnit
  reward_balance: number
}

export interface TokenRewardConfig {
  enabled: boolean
  cycle: TokenRewardCycleType
  tiers: TokenRewardTier[]
}

export interface TokenRewardCycle {
  type: TokenRewardCycleType
  start: string
  end: string
}

export interface TokenRewardTierStatus {
  tier: TokenRewardTier
  status: TokenRewardTierState
  claimed_at?: string
}

export interface TokenRewardStatus {
  config: TokenRewardConfig
  cycle: TokenRewardCycle
  current_tokens: number
  tiers: TokenRewardTierStatus[]
}

export interface TokenRewardClaimResult {
  claim: {
    id: number
    tier_id: string
    token_unit: TokenRewardTokenUnit
    reward_balance: number
    token_snapshot: number
    claimed_at: string
  }
  new_balance: number
}

export interface TokenRewardClaim {
  id: number
  user_id: number
  tier_id: string
  cycle_type: TokenRewardCycleType
  cycle_start: string
  cycle_end: string
  required_tokens: number
  token_unit: TokenRewardTokenUnit
  reward_balance: number
  token_snapshot: number
  claimed_at: string
}

export interface PaginatedTokenRewardClaims {
  items: TokenRewardClaim[]
  total: number
  page: number
  page_size: number
  pages: number
}

export async function getStatus(): Promise<TokenRewardStatus> {
  const { data } = await apiClient.get<TokenRewardStatus>('/token-rewards/status')
  return data
}

export async function listClaims(params: { page?: number; page_size?: number } = {}): Promise<PaginatedTokenRewardClaims> {
  const { data } = await apiClient.get<PaginatedTokenRewardClaims>('/token-rewards/claims', { params })
  return data
}

export async function claim(tierId: string): Promise<TokenRewardClaimResult> {
  const { data } = await apiClient.post<TokenRewardClaimResult>('/token-rewards/claim', {
    tier_id: tierId
  })
  return data
}

export const tokenRewardsAPI = {
  getStatus,
  listClaims,
  claim
}

export default tokenRewardsAPI
