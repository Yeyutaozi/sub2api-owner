/** Admin image generation jobs backed by Creazy Canvas work records. */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface AdminImageJob {
  id: number
  task_id: string
  user_id: number
  user_email: string
  username: string
  api_key_id: number
  api_key_name: string
  group_id?: number | null
  group_name: string
  model: string
  status: string
  gateway_type: string
  gateway_remote_id: string
  prompt: string
  params: Record<string, unknown>
  preview_url?: string
  object_url?: string
  mime_type?: string
  size_bytes: number
  error_message?: string
  can_terminate: boolean
  termination_scope: 'async_execution' | 'local_record'
  created_at: string
  updated_at: string
  expires_at: string
}

export interface AdminImageJobQuery {
  page?: number
  page_size?: number
  search?: string
  status?: string
  gateway_type?: string
  active_only?: boolean
}

async function getContentURL(id: number): Promise<string> {
  const response = await apiClient.get<Blob>(`/admin/image-jobs/${id}/content`, {
    responseType: 'blob',
    timeout: 10 * 60 * 1000,
    maxRedirects: 5,
    transformResponse: [(data: unknown) => data]
  } as any)
  const blob = response.data as Blob
  const contentType = String(blob?.type || '')
  if (contentType.includes('application/json') || contentType.includes('text/')) {
    const body = await blob.text()
    try {
      const payload = JSON.parse(body)
      if (payload?.url) return String(payload.url)
      if (payload?.data?.url) return String(payload.data.url)
      throw new Error(String(payload?.message || 'Image content is unavailable'))
    } catch (error) {
      if (error instanceof Error) throw error
    }
  }
  return URL.createObjectURL(blob)
}

export const imageJobsApi = {
  async list(params: AdminImageJobQuery = {}): Promise<PaginatedResponse<AdminImageJob>> {
    const { data } = await apiClient.get<PaginatedResponse<AdminImageJob>>('/admin/image-jobs', {
      params
    })
    return data
  },

  async get(id: number): Promise<AdminImageJob> {
    const { data } = await apiClient.get<AdminImageJob>(`/admin/image-jobs/${id}`)
    return data
  },

  async terminate(id: number, reason?: string): Promise<AdminImageJob> {
    const { data } = await apiClient.post<AdminImageJob>(`/admin/image-jobs/${id}/terminate`, {
      reason: reason || 'admin terminated image task'
    })
    return data
  },

  getContentURL
}

export default imageJobsApi
