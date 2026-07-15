import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface AdminAgentRunAudit {
  id: number
  app_id: number
  app_name: string
  app_version_id: number
  app_version: string
  user_id: number
  user_email: string
  username: string
  api_key_id: number
  api_key_name: string
  worker_host_id?: number
  worker_host_name?: string
  status: string
  duration_ms?: number
  started_at?: string
  completed_at?: string
  created_at: string
  updated_at: string
}

export interface AdminAgentRunFilters {
  app_id?: number
  status?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: AdminAgentRunFilters
): Promise<PaginatedResponse<AdminAgentRunAudit>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminAgentRunAudit>>('/admin/agent-runs', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export const agentRunsAPI = { list }

export default agentRunsAPI
