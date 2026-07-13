import { apiClient } from '../client'
import type {
  AgentWorkerHost,
  AgentWorkerHostHealthResult,
  CreateAgentWorkerHostRequest,
  PaginatedResponse,
  UpdateAgentWorkerHostRequest
} from '@/types'

export async function list(
  page = 1,
  pageSize = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AgentWorkerHost>> {
  const { data } = await apiClient.get<PaginatedResponse<AgentWorkerHost>>('/admin/agent-worker-hosts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    },
    signal: options?.signal
  })
  return data
}

export async function getAll(status?: string): Promise<AgentWorkerHost[]> {
  const { data } = await apiClient.get<AgentWorkerHost[]>('/admin/agent-worker-hosts/all', {
    params: status ? { status } : undefined
  })
  return data
}

export async function create(payload: CreateAgentWorkerHostRequest): Promise<AgentWorkerHost> {
  const { data } = await apiClient.post<AgentWorkerHost>('/admin/agent-worker-hosts', payload)
  return data
}

export async function update(id: number, payload: UpdateAgentWorkerHostRequest): Promise<AgentWorkerHost> {
  const { data } = await apiClient.put<AgentWorkerHost>(`/admin/agent-worker-hosts/${id}`, payload)
  return data
}

export async function deleteWorkerHost(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/agent-worker-hosts/${id}`)
  return data
}

export async function healthCheck(id: number): Promise<AgentWorkerHostHealthResult> {
  const { data } = await apiClient.post<AgentWorkerHostHealthResult>(`/admin/agent-worker-hosts/${id}/health-check`)
  return data
}

export const agentWorkerHostsAPI = {
  list,
  getAll,
  create,
  update,
  delete: deleteWorkerHost,
  healthCheck
}

export default agentWorkerHostsAPI
