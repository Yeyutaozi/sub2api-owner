import { apiClient, buildApiUrl } from './client'
import type {
  AgentAppCatalog,
  AgentArtifactDownloadURL,
  AgentInputAsset,
  AgentInputAssetDownloadURL,
  AgentRun,
  AgentRunEvent,
  CreateAgentRunRequest,
  PaginatedResponse
} from '@/types'

export async function listApps(
  page = 1,
  pageSize = 12,
  filters?: {
    search?: string
    app_type?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }
): Promise<PaginatedResponse<AgentAppCatalog>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentAppCatalog>>('/agent-apps', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getApp(id: number): Promise<AgentAppCatalog> {
  const { data } = await apiClient.get<AgentAppCatalog>(`/agent-apps/${id}`)
  return data
}

export async function getAppIconURL(id: number): Promise<{ app_id: number; url: string; expires_at?: string }> {
  const { data } = await apiClient.get<{ app_id: number; url: string; expires_at?: string }>(`/agent-apps/${id}/icon-url`)
  return data
}

export async function createRun(appId: number, payload: CreateAgentRunRequest): Promise<AgentRun> {
  const { data } = await apiClient.post<AgentRun>(`/agent-apps/${appId}/runs`, payload)
  return data
}

export async function listRuns(
  page = 1,
  pageSize = 10,
  filters?: {
    app_id?: number
    status?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }
): Promise<PaginatedResponse<AgentRun>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentRun>>('/agent-runs', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getRun(id: number): Promise<AgentRun> {
  const { data } = await apiClient.get<AgentRun>(`/agent-runs/${id}`)
  return data
}

export async function cancelRun(id: number): Promise<AgentRun> {
  const { data } = await apiClient.post<AgentRun>(`/agent-runs/${id}/cancel`)
  return data
}

export async function listRunEvents(
  id: number,
  page = 1,
  pageSize = 100
): Promise<PaginatedResponse<AgentRunEvent>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentRunEvent>>(`/agent-runs/${id}/events`, {
    params: { page, page_size: pageSize }
  })
  return data
}

export async function getArtifactDownloadURL(id: number): Promise<AgentArtifactDownloadURL> {
  const { data } = await apiClient.get<AgentArtifactDownloadURL>(`/agent-artifacts/${id}/download-url`)
  return data
}

export async function getArtifactPreviewURL(id: number): Promise<AgentArtifactDownloadURL> {
  const { data } = await apiClient.get<AgentArtifactDownloadURL>(`/agent-artifacts/${id}/preview-url`)
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(data.url) && !data.url.startsWith('//')) {
    data.url = buildApiUrl(data.url)
  }
  return data
}

export async function uploadInputAsset(
  file: File,
  options?: {
    app_id?: number
    field_name?: string
    asset_type?: string
    asset_role?: string
    metadata?: Record<string, unknown>
    onProgress?: (percent: number) => void
  }
): Promise<AgentInputAsset> {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('name', file.name)
  if (file.type) formData.append('mime_type', file.type)
  if (options?.app_id) formData.append('app_id', String(options.app_id))
  if (options?.field_name) formData.append('field_name', options.field_name)
  if (options?.asset_type) formData.append('asset_type', options.asset_type)
  if (options?.asset_role) formData.append('asset_role', options.asset_role)
  if (options?.metadata) formData.append('metadata', JSON.stringify(options.metadata))

  const { data } = await apiClient.post<AgentInputAsset>('/agent-input-assets', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 120000,
    onUploadProgress: (event) => {
      if (!options?.onProgress || !event.total) return
      options.onProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)))
    }
  })
  return data
}

export async function listInputAssets(
  page = 1,
  pageSize = 20,
  filters?: {
    app_id?: number
    asset_type?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }
): Promise<PaginatedResponse<AgentInputAsset>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentInputAsset>>('/agent-input-assets', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getInputAssetDownloadURL(id: number): Promise<AgentInputAssetDownloadURL> {
  const { data } = await apiClient.get<AgentInputAssetDownloadURL>(`/agent-input-assets/${id}/download-url`)
  return data
}

export const agentAppsAPI = {
  listApps,
  getApp,
  getAppIconURL,
  createRun,
  listRuns,
  getRun,
  cancelRun,
  listRunEvents,
  getArtifactDownloadURL,
  getArtifactPreviewURL,
  uploadInputAsset,
  listInputAssets,
  getInputAssetDownloadURL
}

export default agentAppsAPI
