/**
 * Admin Video Jobs API
 * Cross-user video generation task desk for prompt/materials/result inspection,
 * settlement sync, and emergency kill/refund.
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface AdminVideoJob {
  id: number
  job_id: string
  upstream_job_id: string
  user_id: number
  user_email: string
  username: string
  api_key_id: number
  api_key_name: string
  group_id: number
  group_name: string
  account_id: number
  model: string
  fallback_model?: string
  fallback_status?: string
  task_status: string
  refund_status: string
  refund_attempts: number
  settlement_attempts: number
  last_error?: string
  prompt?: string
  request_snapshot?: Record<string, unknown>
  result_path?: string
  next_poll_at?: string | null
  last_polled_at?: string | null
  settled_at?: string | null
  refunded_at?: string | null
  created_at: string
  updated_at: string
}

export interface AdminVideoJobQuery {
  page?: number
  page_size?: number
  job_id?: string
  search?: string
  status?: string
  model?: string
  user_id?: number
  group_id?: number
  api_key_id?: number
  unsettled_only?: boolean
}

export const videoJobsApi = {
  async list(params: AdminVideoJobQuery = {}): Promise<PaginatedResponse<AdminVideoJob>> {
    const { data } = await apiClient.get<PaginatedResponse<AdminVideoJob>>('/admin/video-jobs', {
      params
    })
    return data
  },
  async get(jobId: string): Promise<AdminVideoJob> {
    const { data } = await apiClient.get<AdminVideoJob>(
      `/admin/video-jobs/${encodeURIComponent(jobId)}`
    )
    return data
  },
  async sync(jobId: string): Promise<AdminVideoJob> {
    const { data } = await apiClient.post<AdminVideoJob>(
      `/admin/video-jobs/${encodeURIComponent(jobId)}/sync`
    )
    return data
  },
  async kill(jobId: string, reason?: string): Promise<AdminVideoJob> {
    const { data } = await apiClient.post<AdminVideoJob>(
      `/admin/video-jobs/${encodeURIComponent(jobId)}/kill`,
      { reason: reason || 'admin killed task' }
    )
    return data
  }
}

export default videoJobsApi
