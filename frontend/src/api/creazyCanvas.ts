/**
 * Creazy 画布 API
 * - 元数据 / 作品列表走登录 JWT（/creazy-canvas/*）
 * - 生图 / 生视频走用户 API Key 直调网关（/v1/images/*、/v1/videos/*）
 *
 * 契约：docs/CREAZY_CANVAS_V1_IMPLEMENTATION_CONTRACT_CN.md
 */

import { apiClient, buildApiUrl, buildGatewayUrl } from './client'
import type { VideoBillingUnit } from '@/types'

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
  quality_tiers?: string[]
  aspect_ratios?: string[]
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
  billing_unit?: VideoBillingUnit
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

export interface CreazyCanvasGraph {
  nodes: Array<Record<string, unknown>>
  edges: Array<Record<string, unknown>>
  viewport?: { x?: number; y?: number; zoom?: number }
  [key: string]: unknown
}

export interface CreazyCanvasDocument {
  id: number
  name: string
  graph?: CreazyCanvasGraph
  revision: number
  created_at?: string
  updated_at?: string
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
  source: 'object' | 'playback' | 'session' | 'gateway' | string
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

export const CREAZY_CANVAS_WORK_ID_HEADER = 'X-Creazy-Canvas-Work-ID'

export interface CreazyGatewayRequestOptions {
  workId?: number
}

export function extractGatewayErrorMessage(body: any): { message: string; code?: unknown; param?: string } {
  if (!body || typeof body !== 'object') {
    return { message: '' }
  }
  const err = body.error
  let message = ''
  let code: unknown = body.code
  let param = ''

  if (typeof err === 'string') {
    message = err
  } else if (err && typeof err === 'object') {
    message = String(err.message || err.msg || err.detail || err.error || '')
    code = err.code || err.type || code
    param = String(err.param || err.field || err.property || '')
  }

  if (!message && typeof body.message === 'string') message = body.message
  if (!message && typeof body.msg === 'string') message = body.msg
  if (!message && typeof body.detail === 'string') message = body.detail
  if (!param) param = String(body.param || body.field || '')

  // FastAPI / array detail: [{loc, msg, type}]
  if (!message && Array.isArray(body.detail)) {
    const parts = body.detail
      .map((item: any) => {
        if (typeof item === 'string') return item
        if (!item || typeof item !== 'object') return ''
        const loc = Array.isArray(item.loc) ? item.loc.filter((x: any) => x !== 'body').join('.') : ''
        const msg = String(item.msg || item.message || '')
        if (loc && msg) return `${loc}: ${msg}`
        return msg || loc
      })
      .filter(Boolean)
    message = parts.join('; ')
  }

  // errors: [{field, message}]
  if (!message && Array.isArray(body.errors)) {
    const parts = body.errors
      .map((item: any) => {
        if (typeof item === 'string') return item
        if (!item || typeof item !== 'object') return ''
        const field = String(item.field || item.param || item.path || '')
        const msg = String(item.message || item.msg || '')
        if (field && msg) return `${field}: ${msg}`
        return msg || field
      })
      .filter(Boolean)
    message = parts.join('; ')
  }

  if (param && message && !message.toLowerCase().includes(String(param).toLowerCase())) {
    message = `${message} (param: ${param})`
  } else if (param && !message) {
    message = `Invalid parameter: ${param}`
  }

  return { message: String(message || '').trim(), code, param: param || undefined }
}

async function parseGatewayError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    const extracted = extractGatewayErrorMessage(body)
    const message = extracted.message || response.statusText || `HTTP ${response.status}`
    const error = new Error(typeof message === 'string' ? message : `HTTP ${response.status}`)
    const extra = error as Error & { code?: unknown; status?: number; raw?: unknown; param?: string }
    extra.code = extracted.code || body?.error?.code || body?.code || response.status
    extra.status = response.status
    extra.param = extracted.param
    extra.raw = body
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`)
    ;(error as Error & { status?: number }).status = response.status
    return error
  }
}
function authHeaders(apiKey: string, extra?: HeadersInit, workId?: number): HeadersInit {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${apiKey}`,
  }
  new Headers(extra).forEach((value, name) => {
    headers[name] = value
  })
  if (Number.isSafeInteger(workId) && Number(workId) > 0) {
    headers[CREAZY_CANVAS_WORK_ID_HEADER] = String(workId)
  }
  return headers
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

export async function listDocuments(): Promise<CreazyCanvasDocument[]> {
  const { data } = await apiClient.get('/creazy-canvas/documents')
  return normalizeListPayload<CreazyCanvasDocument>(data)
}

export async function createDocument(payload: {
  name?: string
  graph: CreazyCanvasGraph
}): Promise<CreazyCanvasDocument> {
  const { data } = await apiClient.post<CreazyCanvasDocument>('/creazy-canvas/documents', payload)
  return data
}

export async function getDocument(id: number | string): Promise<CreazyCanvasDocument> {
  const { data } = await apiClient.get<CreazyCanvasDocument>(
    `/creazy-canvas/documents/${encodeURIComponent(String(id))}`,
  )
  return data
}

export async function updateDocument(
  id: number | string,
  payload: { name?: string; graph?: CreazyCanvasGraph; expected_revision?: number },
): Promise<CreazyCanvasDocument> {
  const { data } = await apiClient.patch<CreazyCanvasDocument>(
    `/creazy-canvas/documents/${encodeURIComponent(String(id))}`,
    payload,
  )
  return data
}

export async function deleteDocument(id: number | string): Promise<void> {
  await apiClient.delete(`/creazy-canvas/documents/${encodeURIComponent(String(id))}`)
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
    const raw = data as CreazyWorksListResponse
    return {
      items: raw.items || [],
      total: Number(raw.total || 0),
      page: Number(raw.page || params?.page || 1),
      page_size: Number(raw.page_size || params?.page_size || 20),
      pages: Number(raw.pages || 0) || undefined,
    }
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
    { timeout: 90 * 1000 },
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

/** Short-lived browser-native video URL with Range support. */
export async function getWorkPlaybackURL(id: number | string): Promise<CreazyDownloadURL> {
  const { data } = await apiClient.get<CreazyDownloadURL>(
    `/creazy-canvas/works/${encodeURIComponent(String(id))}/playback-url`,
  )
  if (data.url?.startsWith('/')) {
    data.url = buildApiUrl(data.url)
  }
  return data
}

/** JWT session content stream — no user API key secret required for succeeded works. */
export async function getWorkContentBlob(id: number | string): Promise<string> {
  try {
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
    const blob = response.data as Blob
    const contentType = blob && typeof blob === 'object' ? String((blob as any).type || '') : ''
    // JSON envelope (inline data URL / metadata) or accidental JSON body.
    if (contentType.includes('application/json') || contentType.includes('text/')) {
      const textBody = await blob.text()
      try {
        const json = JSON.parse(textBody)
        if (json?.url) return String(json.url)
        if (json?.data?.url) return String(json.data.url)
        // Non-zero envelope that slipped through as 2xx
        if (json && typeof json === 'object' && Number(json.code) > 0) {
          const err = new Error(String(json.message || json.detail || `HTTP content error`)) as Error & {
            status?: number
            code?: unknown
            reason?: string
            raw?: unknown
          }
          err.status = Number(json.code) || response.status
          err.code = json.code
          err.reason = String(json.reason || '')
          err.raw = json
          throw err
        }
      } catch (e) {
        if (e instanceof Error && (e as any).status) throw e
        // fall through to blob URL only for non-JSON
      }
      // If content-type said json but parse failed / no url, don't create garbage blob URL.
      if (contentType.includes('application/json')) {
        throw Object.assign(new Error('预览返回了无法解析的内容'), { status: response.status || 502 })
      }
    }
    return URL.createObjectURL(blob)
  } catch (error: any) {
    // Normalize interceptor / axios errors for UI mapping.
    if (error && typeof error === 'object' && (error.message || error.reason || error.status)) {
      throw error
    }
    throw error
  }
}

// ==================== Gateway APIs (API Key Bearer) ====================

export async function generateImage(
  apiKey: string,
  payload: ImageGenerationRequest,
  options?: CreazyGatewayRequestOptions & { async?: boolean; edit?: boolean; imageFiles?: File[] },
): Promise<ImageGenerationResponse> {
  const edit = Boolean(options?.edit)
  const path = edit
    ? options?.async
      ? '/v1/images/edits/async'
      : '/v1/images/edits'
    : options?.async
      ? '/v1/images/generations/async'
      : '/v1/images/generations'
  const imageFiles = (options?.imageFiles || []).filter((file) => file instanceof File)
  let headers: HeadersInit
  let body: BodyInit
  if (edit && imageFiles.length) {
    const form = new FormData()
    for (const [name, value] of Object.entries(payload)) {
      if (name === 'images' || name === 'reference_images' || value == null) continue
      if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
        form.append(name, String(value))
      }
    }
    const imageField = imageFiles.length > 1 ? 'image[]' : 'image'
    for (const file of imageFiles) {
      form.append(imageField, file, file.name || 'reference-image')
    }
    headers = authHeaders(apiKey, undefined, options?.workId)
    body = form
  } else {
    headers = authHeaders(apiKey, { 'Content-Type': 'application/json' }, options?.workId)
    body = JSON.stringify(payload)
  }
  const response = await fetch(buildGatewayUrl(path), {
    method: 'POST',
    headers,
    body,
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

export async function generateVideo(
  apiKey: string,
  payload: VideoGenerationRequest,
  options?: CreazyGatewayRequestOptions,
): Promise<VideoJob> {
  const response = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, { 'Content-Type': 'application/json' }, options?.workId),
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
  listDocuments,
  createDocument,
  getDocument,
  updateDocument,
  deleteDocument,
  listWorks,
  createWork,
  updateWork,
  getWork,
  deleteWork,
  getWorkDownloadURL,
  getWorkPlaybackURL,
  getWorkContentBlob,
  generateImage,
  getImageTask,
  uploadVideoAsset,
  generateVideo,
  getVideoJob,
  getVideoContentURL,
}

export default creazyCanvasAPI
