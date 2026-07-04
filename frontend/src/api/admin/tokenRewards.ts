import { apiClient } from '../client'
import type { TokenRewardConfig } from '../tokenRewards'

export async function getConfig(): Promise<TokenRewardConfig> {
  const { data } = await apiClient.get<TokenRewardConfig>('/admin/token-rewards/config')
  return data
}

export async function updateConfig(config: TokenRewardConfig): Promise<TokenRewardConfig> {
  const { data } = await apiClient.put<TokenRewardConfig>('/admin/token-rewards/config', config)
  return data
}

export const tokenRewardsAPI = {
  getConfig,
  updateConfig
}

export default tokenRewardsAPI
