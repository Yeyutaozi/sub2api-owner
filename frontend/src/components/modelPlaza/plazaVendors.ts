/**
 * Vendor visual identity for Model Plaza cards.
 * Brand-associated colors + official-style SVG marks (simplified, trademark-safe).
 */

export type PlazaVendorId =
  | 'openai'
  | 'anthropic'
  | 'google'
  | 'xai'
  | 'minimax'
  | 'seedance'
  | 'byteplus'
  | 'deepseek'
  | 'qwen'
  | 'meta'
  | 'mistral'
  | 'flux'
  | 'midjourney'
  | 'sora'
  | 'ltx'
  | 'kling'
  | 'runway'
  | 'glm'
  | 'antigravity'
  | 'happyhorse'
  | 'generic'

export interface PlazaVendorMeta {
  id: PlazaVendorId
  label: string
  monogram: string
  /** Primary brand-ish accent */
  color: string
  /** Soft wash for surfaces */
  soft: string
  /** Optional second accent (multi-color marks) */
  color2?: string
  /** SVG mark kind rendered by PlazaVendorMark */
  mark:
    | 'openai'
    | 'anthropic'
    | 'google'
    | 'xai'
    | 'minimax'
    | 'seedance'
    | 'deepseek'
    | 'qwen'
    | 'meta'
    | 'mistral'
    | 'flux'
    | 'midjourney'
    | 'sora'
    | 'ltx'
    | 'kling'
    | 'runway'
    | 'byteplus'
    | 'glm'
    | 'antigravity'
    | 'happyhorse'
    | 'letter'
}

const VENDORS: Record<PlazaVendorId, PlazaVendorMeta> = {
  openai: {
    id: 'openai',
    label: 'OpenAI',
    monogram: 'OA',
    color: '#10a37f',
    soft: '#d1fae5',
    mark: 'openai'
  },
  anthropic: {
    id: 'anthropic',
    label: 'Anthropic',
    monogram: 'AN',
    color: '#d4a27f',
    soft: '#faf6f1',
    mark: 'anthropic'
  },
  google: {
    id: 'google',
    label: 'Google',
    monogram: 'G',
    color: '#4285f4',
    soft: '#dbeafe',
    color2: '#ea4335',
    mark: 'google'
  },
  xai: {
    id: 'xai',
    label: 'xAI',
    monogram: 'X',
    color: '#111827',
    soft: '#e5e7eb',
    mark: 'xai'
  },
  minimax: {
    id: 'minimax',
    label: 'MiniMax',
    monogram: 'MM',
    color: '#e11d48',
    soft: '#ffe4e6',
    mark: 'minimax'
  },
  seedance: {
    id: 'seedance',
    label: 'Seedance',
    monogram: 'SD',
    color: '#0d9488',
    soft: '#ccfbf1',
    mark: 'seedance'
  },
  byteplus: {
    id: 'byteplus',
    label: 'BytePlus',
    monogram: 'BP',
    color: '#0f766e',
    soft: '#ccfbf1',
    mark: 'byteplus'
  },
  deepseek: {
    id: 'deepseek',
    label: 'DeepSeek',
    monogram: 'DS',
    color: '#4d6bfe',
    soft: '#e0e7ff',
    mark: 'deepseek'
  },
  qwen: {
    id: 'qwen',
    label: 'Qwen',
    monogram: 'QW',
    color: '#615ced',
    soft: '#e0e7ff',
    mark: 'qwen'
  },
  meta: {
    id: 'meta',
    label: 'Meta',
    monogram: 'ME',
    color: '#0668e1',
    soft: '#dbeafe',
    mark: 'meta'
  },
  mistral: {
    id: 'mistral',
    label: 'Mistral',
    monogram: 'MI',
    color: '#f97316',
    soft: '#ffedd5',
    mark: 'mistral'
  },
  flux: {
    id: 'flux',
    label: 'Flux',
    monogram: 'FX',
    color: '#000000',
    soft: '#f3f4f6',
    mark: 'flux'
  },
  midjourney: {
    id: 'midjourney',
    label: 'Midjourney',
    monogram: 'MJ',
    color: '#0ea5e9',
    soft: '#e0f2fe',
    mark: 'midjourney'
  },
  sora: {
    id: 'sora',
    label: 'Sora',
    monogram: 'SO',
    color: '#10a37f',
    soft: '#d1fae5',
    mark: 'sora'
  },
  ltx: {
    id: 'ltx',
    label: 'LTX',
    monogram: 'LT',
    color: '#ca8a04',
    soft: '#fef9c3',
    mark: 'ltx'
  },
  kling: {
    id: 'kling',
    label: 'Kling',
    monogram: 'KL',
    color: '#111827',
    soft: '#e5e7eb',
    mark: 'kling'
  },
  runway: {
    id: 'runway',
    label: 'Runway',
    monogram: 'RW',
    color: '#000000',
    soft: '#f3f4f6',
    mark: 'runway'
  },
  glm: {
    id: 'glm',
    label: 'Zhipu GLM',
    monogram: 'GL',
    color: '#3859ff',
    soft: '#dbeafe',
    mark: 'glm'
  },
  antigravity: {
    id: 'antigravity',
    label: 'Antigravity',
    monogram: 'AG',
    color: '#7c3aed',
    soft: '#ede9fe',
    mark: 'antigravity'
  },
  happyhorse: {
    id: 'happyhorse',
    label: 'HappyHorse',
    monogram: 'HH',
    color: '#ea580c',
    soft: '#ffedd5',
    mark: 'happyhorse'
  },
  generic: {
    id: 'generic',
    label: 'Model',
    monogram: 'MD',
    color: '#0f766e',
    soft: '#ccfbf1',
    mark: 'letter'
  }
}

function normalize(raw: string): string {
  return String(raw || '')
    .trim()
    .toLowerCase()
    .replace(/[\s_]+/g, '-')
}

/** Resolve vendor from platform / model name. */
export function resolvePlazaVendor(platform: string, modelName = ''): PlazaVendorMeta {
  const p = normalize(platform)
  const n = normalize(modelName)
  const hay = p + ' ' + n

  if (/(seedance|sd-?2|sd2|sd-?mx|jimeng)/.test(hay)) return VENDORS.seedance
  if (/byteplus|volc|doubao|ark/.test(hay)) return VENDORS.byteplus
  if (/minimax|hailuo|video-?01|\bh3\b|speech-?0/.test(hay)) return VENDORS.minimax
  if (/sora/.test(hay)) return VENDORS.sora
  if (/openai|gpt|o1|o3|o4|chatgpt|codex|dall-?e|whisper/.test(hay)) return VENDORS.openai
  if (/anthropic|claude/.test(hay)) return VENDORS.anthropic
  if (/google|gemini|imagen|veo|palm|bard/.test(hay)) return VENDORS.google
  if (/(^|[\s-])(xai|grok|grokimagine)([\s-]|$)/.test(hay) || /grok-?imagine|grokimagine/.test(hay)) {
    return VENDORS.xai
  }
  if (/deepseek/.test(hay)) return VENDORS.deepseek
  if (/qwen|tongyi|dashscope|alibaba/.test(hay)) return VENDORS.qwen
  if (/meta|llama|llama-?3|facebook/.test(hay)) return VENDORS.meta
  if (/mistral|mixtral|codestral|pixtral/.test(hay)) return VENDORS.mistral
  if (/flux|black-?forest|bfl/.test(hay)) return VENDORS.flux
  if (/midjourney|\bmj\b/.test(hay)) return VENDORS.midjourney
  if (/\bltx\b|lightricks/.test(hay)) return VENDORS.ltx
  if (/kling|kuaishou/.test(hay)) return VENDORS.kling
  if (/runway|gen-?3|gen3|gen-?4/.test(hay)) return VENDORS.runway
  if (/\bglm\b|zhipu|chatglm|智谱/.test(hay) || p === 'glm') return VENDORS.glm
  if (/antigravity/.test(hay)) return VENDORS.antigravity
  if (/happyhorse|happy-?horse/.test(hay)) return VENDORS.happyhorse

  const letters = (p || 'md').replace(/[^a-z0-9]/g, '')
  const mono = (letters.slice(0, 2) || 'md').toUpperCase()
  return {
    ...VENDORS.generic,
    label: platform || 'Model',
    monogram: mono,
    mark: 'letter'
  }
}

export function vendorLabel(platform: string, modelName = ''): string {
  return resolvePlazaVendor(platform, modelName).label
}

export function listPlazaVendors(): PlazaVendorMeta[] {
  return Object.values(VENDORS).filter((v) => v.id !== 'generic')
}
