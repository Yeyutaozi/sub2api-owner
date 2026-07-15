import { apiClient } from '../client'
import type {
  AgentApp,
  AgentAppVersion,
  AgentAppStatus,
  CreateAgentAppRequest,
  CreateAgentAppVersionRequest,
  PaginatedResponse
} from '@/types'

export interface AgentAppIconUploadResult {
  url: string
  preview_url?: string
  object_key?: string
  storage_provider?: string
  bucket?: string
  size_bytes?: number
  sha256?: string
}

export interface CreateAgentAppWithVersionPayload {
  app: CreateAgentAppRequest
  version: CreateAgentAppVersionRequest
}

export interface CreateAgentAppWithVersionResult {
  app: AgentApp
  version: AgentAppVersion
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: {
    status?: string
    app_type?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AgentApp>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentApp>>('/admin/agent-apps', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    },
    signal: options?.signal
  })
  return data
}

export async function create(payload: CreateAgentAppRequest): Promise<AgentApp> {
  const { data } = await apiClient.post<AgentApp>('/admin/agent-apps', payload)
  return data
}

export async function update(id: number, payload: CreateAgentAppRequest): Promise<AgentApp> {
  const { data } = await apiClient.put<AgentApp>(`/admin/agent-apps/${id}`, payload)
  return data
}

export async function remove(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/agent-apps/${id}`)
  return data
}

export async function createWithVersion(payload: CreateAgentAppWithVersionPayload): Promise<CreateAgentAppWithVersionResult> {
  const { data } = await apiClient.post<CreateAgentAppWithVersionResult>('/admin/agent-apps/with-version', payload)
  return data
}

export async function uploadIcon(file: File): Promise<AgentAppIconUploadResult> {
  const formData = new FormData()
  formData.append('file', file)
  const { data } = await apiClient.post<AgentAppIconUploadResult>('/admin/agent-apps/icon', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
  return data
}

export async function getById(id: number): Promise<AgentApp> {
  const { data } = await apiClient.get<AgentApp>(`/admin/agent-apps/${id}`)
  return data
}

export async function getIconURL(id: number): Promise<{ app_id: number; url: string; expires_at?: string }> {
  const { data } = await apiClient.get<{ app_id: number; url: string; expires_at?: string }>(`/admin/agent-apps/${id}/icon-url`)
  return data
}

export async function listVersions(appId: number): Promise<AgentAppVersion[]> {
  const { data } = await apiClient.get<AgentAppVersion[]>(`/admin/agent-apps/${appId}/versions`)
  return data
}

export async function createVersion(appId: number, payload: CreateAgentAppVersionRequest): Promise<AgentAppVersion> {
  const { data } = await apiClient.post<AgentAppVersion>(`/admin/agent-apps/${appId}/versions`, payload)
  return data
}

export async function publishVersion(appId: number, versionId: number): Promise<AgentAppVersion> {
  const { data } = await apiClient.post<AgentAppVersion>(`/admin/agent-apps/${appId}/versions/${versionId}/publish`)
  return data
}

export async function updateVersionStatus(appId: number, versionId: number, status: AgentAppStatus | string): Promise<AgentAppVersion> {
  const { data } = await apiClient.put<AgentAppVersion>(`/admin/agent-apps/${appId}/versions/${versionId}/status`, { status })
  return data
}

export const agentAppsAPI = {
  list,
  create,
  update,
  remove,
  createWithVersion,
  uploadIcon,
  getById,
  getIconURL,
  listVersions,
  createVersion,
  publishVersion,
  updateVersionStatus
}

export default agentAppsAPI
