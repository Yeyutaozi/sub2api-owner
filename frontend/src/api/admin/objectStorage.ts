import { apiClient } from '../client'

export interface AgentArtifactStorageConfig {
  enabled: boolean
  provider: string
  account_id?: string
  endpoint?: string
  region?: string
  bucket?: string
  access_key_id?: string
  secret_access_key?: string
  secret_access_key_configured?: boolean
  prefix?: string
  public_base_url?: string
  force_path_style: boolean
  virtual_host_style: boolean
  disable_checksum: boolean
  max_upload_bytes: number
  download_url_ttl_seconds: number
  retention_days: number
  cleanup_expired_artifacts_enabled: boolean
  encryption_key_configured?: boolean
  runtime_error?: string
  source?: string
  resolved_endpoint?: string
}

export async function getConfig(): Promise<AgentArtifactStorageConfig> {
  const { data } = await apiClient.get<AgentArtifactStorageConfig>('/admin/agent-artifact-storage')
  return data
}

export async function updateConfig(payload: AgentArtifactStorageConfig): Promise<AgentArtifactStorageConfig> {
  const { data } = await apiClient.put<AgentArtifactStorageConfig>('/admin/agent-artifact-storage', payload)
  return data
}

export async function validateConfig(payload: AgentArtifactStorageConfig): Promise<{ valid: boolean }> {
  const { data } = await apiClient.post<{ valid: boolean }>('/admin/agent-artifact-storage/validate', payload)
  return data
}

export default {
  getConfig,
  updateConfig,
  validateConfig
}
