/**
 * Work params helpers for Creazy Canvas persistence / reuse.
 * Keep only non-blob absolute media so records survive refresh.
 */
export function isReusableMediaUrl(url: unknown): url is string {
  const u = String(url || '').trim()
  if (!u) return false
  if (u.startsWith('blob:') || u.startsWith('data:')) return false
  return /^https?:\/\//i.test(u) || u.startsWith('/')
}

export function sanitizeReusableUrlList(urls: unknown): string[] {
  if (!Array.isArray(urls)) return []
  const out: string[] = []
  const seen = new Set<string>()
  for (const item of urls) {
    const u = String(item || '').trim()
    if (!isReusableMediaUrl(u) || seen.has(u)) continue
    seen.add(u)
    out.push(u)
  }
  return out
}

export function buildImageWorkParams(input: {
  size: string
  refs?: string[]
  resultUrls?: string[]
  extra?: Record<string, unknown>
}): Record<string, unknown> {
  const refs = sanitizeReusableUrlList(input.refs || [])
  const resultUrls = sanitizeReusableUrlList(input.resultUrls || [])
  return {
    size: input.size,
    n: 1,
    reference_count: refs.length,
    edit: refs.length > 0,
    image_refs: refs,
    ...(resultUrls.length ? { result_urls: resultUrls } : {}),
    ...(input.extra || {}),
  }
}

export function buildVideoWorkParams(input: {
  resolution: string
  duration: number
  aspectRatio: string
  generateAudio?: boolean
  startFrame?: string
  endFrame?: string
  refImages?: string[]
  refVideos?: string[]
  refAudios?: string[]
  resultUrl?: string
  extra?: Record<string, unknown>
}): Record<string, unknown> {
  const startFrame = isReusableMediaUrl(input.startFrame) ? String(input.startFrame).trim() : ''
  const endFrame = isReusableMediaUrl(input.endFrame) ? String(input.endFrame).trim() : ''
  const refImages = sanitizeReusableUrlList(input.refImages || [])
  const refVideos = sanitizeReusableUrlList(input.refVideos || [])
  const refAudios = sanitizeReusableUrlList(input.refAudios || [])
  const resultUrl = isReusableMediaUrl(input.resultUrl) ? String(input.resultUrl).trim() : ''
  return {
    resolution: input.resolution,
    duration: input.duration,
    aspect_ratio: input.aspectRatio,
    generate_audio: input.generateAudio || undefined,
    ...(startFrame ? { start_frame: startFrame } : {}),
    ...(endFrame ? { end_frame: endFrame } : {}),
    ...(refImages.length ? { ref_images: refImages } : {}),
    ...(refVideos.length ? { ref_videos: refVideos } : {}),
    ...(refAudios.length ? { ref_audios: refAudios } : {}),
    ...(resultUrl ? { result_url: resultUrl } : {}),
    ...(input.extra || {}),
  }
}

export function pickStringParam(params: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const v = params[key]
    if (v == null) continue
    const s = String(v).trim()
    if (s) return s
  }
  return ''
}

export function pickStringListParam(params: Record<string, unknown>, ...keys: string[]): string[] {
  for (const key of keys) {
    const v = params[key]
    if (Array.isArray(v)) return sanitizeReusableUrlList(v)
    if (typeof v === 'string' && v.trim()) return sanitizeReusableUrlList([v])
  }
  return []
}

export type CanvasDraftV1 = {
  v: 1
  selectedKeyId?: number
  activeTab?: 'image' | 'video' | 'works'
  image?: {
    prompt?: string
    model?: string
    size?: string
    refs?: string[]
  }
  video?: {
    prompt?: string
    model?: string
    resolution?: string
    duration?: number
    aspectRatio?: string
    generateAudio?: boolean
    startFrame?: string
    endFrame?: string
    refImages?: string[]
    refVideos?: string[]
    refAudios?: string[]
  }
  savedAt?: number
}

export const CANVAS_DRAFT_KEY = 'creazy-canvas-draft-v1'

export function readCanvasDraft(): CanvasDraftV1 | null {
  try {
    const raw = sessionStorage.getItem(CANVAS_DRAFT_KEY)
    if (!raw) return null
    const data = JSON.parse(raw) as CanvasDraftV1
    if (!data || data.v !== 1) return null
    return data
  } catch {
    return null
  }
}

export function writeCanvasDraft(draft: CanvasDraftV1): void {
  try {
    sessionStorage.setItem(CANVAS_DRAFT_KEY, JSON.stringify({ ...draft, v: 1, savedAt: Date.now() }))
  } catch {
    // ignore quota / private mode
  }
}

export function clearCanvasDraft(): void {
  try {
    sessionStorage.removeItem(CANVAS_DRAFT_KEY)
  } catch {
    // ignore
  }
}

/** Map gateway/OpenAPI param names to stable i18n keys under creazyCanvas.errors.fields.* */
export function gatewayParamFieldKey(param: string): string {
  const p = String(param || '')
    .trim()
    .toLowerCase()
    .replace(/^\//, '')
    .replace(/^body\./, '')
  const aliases: Record<string, string> = {
    size: 'size',
    width: 'size',
    height: 'size',
    prompt: 'prompt',
    model: 'model',
    n: 'n',
    duration: 'duration',
    seconds: 'duration',
    resolution: 'resolution',
    aspect_ratio: 'aspectRatio',
    aspectratio: 'aspectRatio',
    generate_audio: 'generateAudio',
    audio: 'generateAudio',
    image: 'image',
    images: 'image',
    reference_images: 'image',
    start_frame: 'startFrame',
    first_frame: 'startFrame',
    end_frame: 'endFrame',
    last_frame: 'endFrame',
    video: 'video',
    videos: 'video',
    ref_videos: 'video',
    audio_file: 'audio',
    audios: 'audio',
    ref_audios: 'audio',
  }
  return aliases[p] || aliases[p.replace(/-/g, '_')] || 'param'
}
