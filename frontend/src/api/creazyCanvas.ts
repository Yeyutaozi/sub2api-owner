/**
 * Creazy 画布 API
 * - 元数据 / 作品列表走登录 JWT（/creazy-canvas/*）
 * - 生图 / 生视频走用户 API Key 直调网关（/v1/images/*、/v1/videos/*）
 *
 * 契约：docs/CREAZY_CANVAS_V1_IMPLEMENTATION_CONTRACT_CN.md
 */

import { apiClient, buildGatewayUrl } from './client'

// ==================== Types ====================

export interface CreazyCanvasKey {
  id: number
  name: string
  status?: string
  group_id?: number | null
  group_name?: string | null
  platform?: string | null
  allow_creazy_canvas?: boolean
  allow_image_generation?: boolean
}

export interface CreazyCanvasImageSizeConstraints {
  /** Max single-edge length in px (gpt-image-2: 3840). */
  max_edge?: number
  /** Both edges must be multiples of this (gpt-image-2: 16). */
  multiple_of?: number
  /** long:short upper bound (gpt-image-2: 3). */
  max_aspect_ratio?: number
  /** Total pixel lower bound (gpt-image-2: 655360). */
  min_pixels?: number
  /** Total pixel upper bound (gpt-image-2: 8294400). */
  max_pixels?: number
  /** Non-WxH free-form aliases when custom size is allowed (e.g. auto). */
  aliases?: string[]
}

export interface CreazyCanvasImageModel {
  id: string
  name?: string
  sizes?: string[]
  allow_custom_size?: boolean
  size_constraints?: CreazyCanvasImageSizeConstraints | null
  prices?: Record<string, number | null | undefined>
  async?: boolean
  max_n?: number
  supports_reference?: boolean
  max_reference_images?: number
  require_reference?: boolean
  [key: string]: unknown
}

export interface CreazyCanvasVideoModel {
  id: string
  name?: string
  platform?: string
  default_resolution?: string
  default_duration?: number
  allowed_resolutions?: string[]
  resolutions?: string[]
  allowed_aspect_ratios?: string[]
  aspect_ratios?: string[]
  allowed_durations?: number[]
  durations?: number[]
  prices?: Record<string, number | null | undefined>
  billing_unit?: string
  allow_start_frame?: boolean
  require_start_frame?: boolean
  allow_end_frame?: boolean
  allow_generated_audio?: boolean
  max_image_references?: number
  max_video_references?: number
  max_audio_references?: number
  max_total_media?: number
  max_total_images?: number
  frames_exclusive_with_refs?: boolean
  audio_requires_image_refs?: boolean
  force_generated_audio?: boolean
  prompt_limit?: number
  [key: string]: unknown
}

export interface CreazyCanvasCatalog {
  api_key_id: number
  group_id?: number | null
  platform?: string
  allow_image_generation?: boolean
  image_models: CreazyCanvasImageModel[]
  video_models: CreazyCanvasVideoModel[]
}

export type CreazyWorkKind = 'image' | 'video'
export type CreazyWorkStatus =
  | 'created'
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'expired'
  | string

export type CreazyGatewayType = 'image_task' | 'image_sync' | 'video_job' | string

export interface CreazyWork {
  id: number
  user_id?: number
  api_key_id: number
  group_id?: number | null
  kind: CreazyWorkKind | string
  public_model: string
  status: CreazyWorkStatus
  prompt: string
  params?: Record<string, unknown>
  gateway_type?: CreazyGatewayType
  gateway_remote_id?: string
  object_key?: string
  storage_provider?: string
  bucket?: string
  object_url?: string
  preview_url?: string
  mime_type?: string
  size_bytes?: number
  error_message?: string
  expires_at?: string
  created_at?: string
  updated_at?: string
}

export interface CreazyWorksListResponse {
  items: CreazyWork[]
  total: number
  page: number
  page_size: number
  pages?: number
}

/** POST /creazy-canvas/works — 与后端契约字段对齐 */
export interface CreateCreazyWorkRequest {
  api_key_id: number
  kind: CreazyWorkKind
  public_model?: string
  prompt?: string
  params?: Record<string, unknown>
  gateway_type?: CreazyGatewayType
  gateway_remote_id?: string
  status?: CreazyWorkStatus
  error_message?: string
  preview_url?: string
  object_url?: string
  mime_type?: string
  size_bytes?: number
}

/** PATCH /creazy-canvas/works/:id — update running/succeeded work metadata */
export interface UpdateCreazyWorkRequest {
  status?: CreazyWorkStatus
  error_message?: string
  params?: Record<string, unknown>
  gateway_type?: CreazyGatewayType
  gateway_remote_id?: string
  preview_url?: string
  object_url?: string
  mime_type?: string
  size_bytes?: number
  public_model?: string
  prompt?: string
}

export interface CreazyDownloadURL {
  work_id: number
  url?: string
  expires_at?: string
  source: 'object' | 'session' | 'gateway' | string
  gateway_hint?: string
}

export interface ImageGenerationRequest {
  model: string
  prompt: string
  n?: number
  size?: string
  aspect_ratio?: string
  response_format?: 'url' | 'b64_json' | string
  [key: string]: unknown
}

export interface ImageGenerationResponse {
  created?: number
  data?: Array<{
    url?: string
    b64_json?: string
    revised_prompt?: string
  }>
  id?: string
  task_id?: string
  status?: string
  image_url?: string
  result?: unknown
  error?: unknown
  [key: string]: unknown
}

export interface VideoGenerationRequest {
  model: string
  prompt: string
  resolution?: string
  duration?: number | string
  aspect_ratio?: string
  size?: string
  [key: string]: unknown
}

export interface VideoJob {
  id?: string
  job_id?: string
  status: string
  model?: string
  progress?: number
  error?: { message?: string; code?: string } | string | null
  result?: {
    url?: string
    content_url?: string
    [key: string]: unknown
  } | null
  video_url?: string
  content_url?: string
  url?: string
  [key: string]: unknown
}

// ==================== Helpers ====================

async function parseGatewayError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    // Prefer structured error; never attach full raw body as UI message
    const message =
      body?.error?.message ||
      body?.message ||
      body?.detail ||
      body?.error ||
      response.statusText ||
      `HTTP ${response.status}`
    const error = new Error(typeof message === 'string' ? message : `HTTP ${response.status}`)
    const extra = error as Error & { code?: unknown; status?: number; raw?: unknown }
    extra.code = body?.error?.code || body?.code || response.status
    extra.status = response.status
    // Keep raw for optional server logs only — UI must use mapGatewayError
    extra.raw = body
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`)
    ;(error as Error & { status?: number }).status = response.status
    return error
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

function normalizeListPayload<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.data)) return obj.data as T[]
    if (Array.isArray(obj.keys)) return obj.keys as T[]
  }
  return []
}

// ==================== JWT APIs ====================

export async function listKeys(): Promise<CreazyCanvasKey[]> {
  const { data } = await apiClient.get('/creazy-canvas/keys')
  return normalizeListPayload<CreazyCanvasKey>(data)
}

export async function getCatalog(apiKeyId: number): Promise<CreazyCanvasCatalog> {
  const { data } = await apiClient.get<CreazyCanvasCatalog>('/creazy-canvas/catalog', {
    params: { api_key_id: apiKeyId },
  })
  return {
    api_key_id: data?.api_key_id ?? apiKeyId,
    group_id: data?.group_id,
    platform: data?.platform,
    allow_image_generation: data?.allow_image_generation,
    image_models: Array.isArray(data?.image_models) ? data.image_models : [],
    video_models: Array.isArray(data?.video_models) ? data.video_models : [],
  }
}

export async function listWorks(params?: {
  page?: number
  page_size?: number
  kind?: string
  status?: string
  api_key_id?: number
}): Promise<CreazyWorksListResponse> {
  const { data } = await apiClient.get('/creazy-canvas/works', { params })
  if (data && typeof data === 'object' && Array.isArray((data as CreazyWorksListResponse).items)) {
    return data as CreazyWorksListResponse
  }
  // Paginated envelope variants
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.data) && typeof obj.total === 'number') {
      return {
        items: obj.data as CreazyWork[],
        total: obj.total as number,
        page: (obj.page as number) ?? params?.page ?? 1,
        page_size: (obj.page_size as number) ?? params?.page_size ?? 20,
      }
    }
  }
  const items = normalizeListPayload<CreazyWork>(data)
  return {
    items,
    total: items.length,
    page: params?.page ?? 1,
    page_size: params?.page_size ?? items.length,
  }
}

export async function createWork(payload: CreateCreazyWorkRequest): Promise<CreazyWork> {
  const { data } = await apiClient.post<CreazyWork>('/creazy-canvas/works', payload)
  return data
}

export async function updateWork(
  id: number | string,
  payload: UpdateCreazyWorkRequest,
): Promise<CreazyWork> {
  const { data } = await apiClient.patch<CreazyWork>(
    `/creazy-canvas/works/${encodeURIComponent(String(id))}`,
    payload,
  )
  return data
}

export async function getWork(id: number | string): Promise<CreazyWork> {
  const { data } = await apiClient.get<CreazyWork>(`/creazy-canvas/works/${encodeURIComponent(String(id))}`)
  return data
}

export async function deleteWork(id: number | string): Promise<void> {
  await apiClient.delete(`/creazy-canvas/works/${encodeURIComponent(String(id))}`)
}

export async function getWorkDownloadURL(id: number | string): Promise<CreazyDownloadURL> {
  const { data } = await apiClient.get<CreazyDownloadURL>(
    `/creazy-canvas/works/${encodeURIComponent(String(id))}/download-url`,
  )
  return data
}

/** JWT session content stream — no user API key secret required for succeeded works. */
export async function getWorkContentBlob(id: number | string): Promise<string> {
  const response = await apiClient.get<Blob>(
    `/creazy-canvas/works/${encodeURIComponent(String(id))}/content`,
    {
      responseType: 'blob',
      timeout: 10 * 60 * 1000,
      // Allow following signed upstream redirects when backend 302s.
      maxRedirects: 5,
      // Do not transform; keep binary.
      transformResponse: [(d: unknown) => d],
    } as any,
  )
  const blob = response.data
  // If server returned JSON envelope (rare inline data URL), try parse.
  if (blob && typeof blob === 'object' && (blob as any).type && String((blob as any).type).includes('application/json')) {
    const text = await (blob as Blob).text()
    try {
      const json = JSON.parse(text)
      if (json?.url) return String(json.url)
      if (json?.data?.url) return String(json.data.url)
    } catch {
      // fall through to blob URL
    }
  }
  return URL.createObjectURL(blob as Blob)
}

// ==================== Gateway APIs (API Key Bearer) ====================

export async function generateImage(
  apiKey: string,
  payload: ImageGenerationRequest,
  options?: { async?: boolean; edit?: boolean },
): Promise<ImageGenerationResponse> {
  const edit = Boolean(options?.edit)
  const path = edit
    ? options?.async
      ? '/v1/images/edits/async'
      : '/v1/images/edits'
    : options?.async
      ? '/v1/images/generations/async'
      : '/v1/images/generations'
  const response = await fetch(buildGatewayUrl(path), {
    method: 'POST',
    headers: authHeaders(apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export async function getImageTask(apiKey: string, taskId: string): Promise<ImageGenerationResponse> {
  const response = await fetch(buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(taskId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export type VideoUploadKind = 'image' | 'video' | 'audio'

export async function uploadVideoAsset(
  apiKey: string,
  file: File | Blob,
  filename = 'upload.bin',
  kind?: VideoUploadKind,
): Promise<{ id?: string; media_url?: string; [key: string]: unknown }> {
  const form = new FormData()
  const mime = (file as File).type || ''
  const resolvedKind: VideoUploadKind =
    kind || (mime.startsWith('video/') ? 'video' : mime.startsWith('audio/') ? 'audio' : 'image')
  form.append(resolvedKind, file, filename || (file as File).name || 'upload.bin')
  const response = await fetch(buildGatewayUrl('/v1/videos/uploads'), {
    method: 'POST',
    headers: authHeaders(apiKey),
    body: form,
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export async function generateVideo(apiKey: string, payload: VideoGenerationRequest): Promise<VideoJob> {
  const response = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export async function getVideoJob(apiKey: string, jobId: string): Promise<VideoJob> {
  const response = await fetch(buildGatewayUrl(`/v1/videos/jobs/${encodeURIComponent(jobId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export async function getVideoContentURL(apiKey: string, jobId: string): Promise<string> {
  const response = await fetch(buildGatewayUrl(`/v1/videos/jobs/${encodeURIComponent(jobId)}/content`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseGatewayError(response)
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    const body = await response.json()
    return body?.url || body?.content_url || body?.video_url || ''
  }
  const blob = await response.blob()
  return URL.createObjectURL(blob)
}

export const creazyCanvasAPI = {
  listKeys,
  getCatalog,
  listWorks,
  createWork,
  updateWork,
  getWork,
  deleteWork,
  getWorkDownloadURL,
  getWorkContentBlob,
  generateImage,
  getImageTask,
  uploadVideoAsset,
  generateVideo,
  getVideoJob,
  getVideoContentURL,
}

export default creazyCanvasAPI
