import type {
  VideoBillingUnit,
  VideoModelPrice,
  VideoModelPrices as DomainVideoModelPrices
} from '@/types'

export const grokVideoPriceResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '1080p', label: '1080p' }
] as const

export const grokVideoPriceFamilies = [
  { key: 'grok-imagine-video', label: 'grok-imagine-video' },
  { key: 'grok-imagine-video-1.5', label: 'grok-imagine-video-1.5' }
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

function emptyTiers(): Record<string, number | string | null> {
  return Object.fromEntries(grokVideoPriceResolutions.map(({ key }) => [key, null]))
}

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: DomainVideoModelPrices | VideoModelPrices | null
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {}

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {})) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = emptyTiers()
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    }
  }

  for (const { key } of grokVideoPriceFamilies) {
    form[key] ??= emptyTiers()
  }
  return form
}

export function serializeVideoModelPrices(form: VideoModelPricesForm): VideoModelPrices {
  const result: VideoModelPrices = {}
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {}
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = rawResolution.trim().toLowerCase()
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    }
    if (Object.keys(normalizedTiers).length > 0) result[family] = normalizedTiers
  }
  return result
}

export function videoModelPriceFamilyRows(form: VideoModelPricesForm) {
  const known = new Set<string>(grokVideoPriceFamilies.map(({ key }) => key))
  const extra = Object.keys(form)
    .map(normalizeFamily)
    .filter((family) => family && !known.has(family))
    .sort()
    .map((key) => ({ key, label: key }))
  return [...grokVideoPriceFamilies, ...extra]
}

export const DEFAULT_SEEDANCE_VIDEO_MODELS = [
  'seedance-2.0', 'seedance-2.0-fast', 'seedance-2.0-mini', 'seedance-2.5',
  'sd2-mx933', 'sd2-mx933-fast', 'sd-2.0-mx933', 'sd-2.0-900-720p',
  'seedance-2.5-c1-03', 'sd-2.5-ff', 'sd-2.0-933-art', 'sd2-933-25',
  'sd-2.5-mx', 'seedance2.0-one-face-reference-480p',
  'seedance2.0-one-face-reference-720p'
] as const
export const DEFAULT_LTX_VIDEO_MODELS = ['ltx-2.3-pro', 'ltx-2.3-fast'] as const
export const DEFAULT_HAPPYHORSE_VIDEO_MODELS = ['happy-horse-1.1'] as const
export const DEFAULT_MINIMAX_VIDEO_MODELS = ['minimax-h3'] as const
export const DEFAULT_GROKIMAGINE_VIDEO_MODELS = ['grok-imagine-1.5'] as const

export const VIDEO_MODEL_PRICE_RESOLUTIONS = [
  '480p', '720p', '1080p', '1440p', '2160p'
] as const
export type VideoModelPriceResolution = (typeof VIDEO_MODEL_PRICE_RESOLUTIONS)[number]

const VIDEO_MODEL_SUPPORTED_RESOLUTIONS: Record<string, readonly VideoModelPriceResolution[]> = {
  'seedance-2.0': ['480p', '720p', '1080p'],
  'seedance-2.0-fast': ['480p', '720p'],
  'seedance-2.0-mini': ['480p', '720p'],
  'seedance-2.5': ['480p', '720p'],
  'sd2-mx933': ['480p', '720p'],
  'sd2-mx933-fast': ['480p', '720p'],
  'sd-2.0-mx933': ['480p', '720p'],
  'sd-2.0-900-720p': ['720p'],
  'seedance-2.5-c1-03': ['720p'],
  'sd-2.5-ff': ['480p', '720p'],
  'sd-2.0-933-art': ['720p'],
  'sd2-933-25': ['1080p'],
  'sd-2.5-mx': ['720p'],
  'seedance2.0-one-face-reference-480p': ['480p'],
  'seedance2.0-one-face-reference-720p': ['720p'],
  'ltx-2.3-pro': ['1080p', '1440p', '2160p'],
  'ltx-2.3-fast': ['1080p', '1440p', '2160p'],
  'happy-horse-1.1': ['720p', '1080p'],
  'minimax-h3': ['1440p'],
  'grok-imagine-1.5': ['720p']
}

export const videoModelsForPricingPlatform = (platform: string): readonly string[] => {
  if (platform === 'seedance') return DEFAULT_SEEDANCE_VIDEO_MODELS
  if (platform === 'ltx') return DEFAULT_LTX_VIDEO_MODELS
  if (platform === 'happyhorse') return DEFAULT_HAPPYHORSE_VIDEO_MODELS
  if (platform === 'minimax') return DEFAULT_MINIMAX_VIDEO_MODELS
  if (platform === 'grokimagine') return DEFAULT_GROKIMAGINE_VIDEO_MODELS
  return []
}

export const supportedResolutionsForVideoModel = (
  platform: string,
  model: string
): readonly VideoModelPriceResolution[] => {
  const configured = VIDEO_MODEL_SUPPORTED_RESOLUTIONS[model.trim().toLowerCase()]
  if (configured) return configured
  if (platform === 'ltx') return ['1080p', '1440p', '2160p']
  if (platform === 'happyhorse') return ['720p', '1080p']
  if (platform === 'minimax') return ['1440p']
  if (platform === 'grokimagine') return ['720p']
  return ['480p', '720p', '1080p']
}

export const videoModelSupportsResolution = (
  platform: string,
  model: string,
  resolution: VideoModelPriceResolution
): boolean => supportedResolutionsForVideoModel(platform, model).includes(resolution)

export const supportsVideoModelPricingPlatform = (platform: string): boolean =>
  ['seedance', 'ltx', 'happyhorse', 'minimax', 'grokimagine'].includes(platform)

export const supportsPerRequestVideoBilling = (platform: string): boolean => platform === 'seedance'

export const normalizeVideoBillingUnitForPlatform = (
  platform: string,
  billingUnit: VideoBillingUnit | null | undefined
): VideoBillingUnit =>
  supportsPerRequestVideoBilling(platform) && billingUnit === 'per_request'
    ? 'per_request'
    : 'per_second'

const normalizeVideoModelBillingUnitForPlatform = (
  platform: string,
  billingUnit: unknown
): VideoBillingUnit | undefined => {
  if (billingUnit !== 'per_second' && billingUnit !== 'per_request') return undefined
  if (billingUnit === 'per_request' && !supportsPerRequestVideoBilling(platform)) return undefined
  return billingUnit
}

export const resolveVideoModelBillingUnit = (
  billingUnit: unknown,
  groupBillingUnit: VideoBillingUnit,
  platform: string
): VideoBillingUnit =>
  normalizeVideoModelBillingUnitForPlatform(platform, billingUnit)
  ?? normalizeVideoBillingUnitForPlatform(platform, groupBillingUnit)

export type VideoModelPriceInput = number | string | null
export interface VideoModelPriceRow {
  model: string
  billing_unit: VideoBillingUnit | ''
  price_480p: VideoModelPriceInput
  price_720p: VideoModelPriceInput
  price_1080p: VideoModelPriceInput
  price_1440p: VideoModelPriceInput
  price_2160p: VideoModelPriceInput
}

export const videoModelPricePlaceholder = (platform: string): string =>
  videoModelsForPricingPlatform(platform)[0] ?? 'video-model'

export const createVideoModelPriceRow = (
  model = '',
  price: VideoModelPrice = {},
  platform = 'seedance'
): VideoModelPriceRow => ({
  model,
  billing_unit: normalizeVideoModelBillingUnitForPlatform(platform, price.billing_unit) ?? '',
  price_480p: price['480p'] ?? null,
  price_720p: price['720p'] ?? null,
  price_1080p: price['1080p'] ?? null,
  price_1440p: price['1440p'] ?? null,
  price_2160p: price['2160p'] ?? null
})

export const createDefaultVideoModelPriceRows = (platform = 'seedance'): VideoModelPriceRow[] =>
  videoModelsForPricingPlatform(platform).map(model => createVideoModelPriceRow(model, {}, platform))

const LEGACY_SEEDANCE_VIDEO_MODEL_ALIASES: Record<string, string> = {
  'sd2-mx933-720-1s': 'sd2-mx933',
  'sd2-mx933-720-fast-1s': 'sd2-mx933-fast'
}

export const normalizeVideoModelPricesForPlatform = (
  platform: string,
  prices: DomainVideoModelPrices | null | undefined
): DomainVideoModelPrices => {
  const normalized: DomainVideoModelPrices = {}
  const source = prices ?? {}
  const sourceModels = new Set(Object.keys(source).map(model => model.trim().toLowerCase()))
  for (const [model, price] of Object.entries(source)) {
    const normalizedModel = model.trim().toLowerCase()
    const publicModel = platform === 'seedance'
      ? LEGACY_SEEDANCE_VIDEO_MODEL_ALIASES[normalizedModel] ?? normalizedModel
      : normalizedModel
    if (publicModel !== normalizedModel && sourceModels.has(publicModel)) continue
    normalized[publicModel] = { ...price }
  }
  return normalized
}

export const videoModelPricesToRows = (
  prices: DomainVideoModelPrices | null | undefined,
  platform: string
): VideoModelPriceRow[] =>
  Object.entries(normalizeVideoModelPricesForPlatform(platform, prices))
    .map(([model, price]) => createVideoModelPriceRow(model, price, platform))

const rowResolutionFields = [
  ['price_480p', '480p'], ['price_720p', '720p'], ['price_1080p', '1080p'],
  ['price_1440p', '1440p'], ['price_2160p', '2160p']
] as const

export const videoModelPriceRowsToPrices = (
  rows: VideoModelPriceRow[],
  platform = 'seedance'
): DomainVideoModelPrices => {
  const prices: DomainVideoModelPrices = {}
  for (const row of rows) {
    const model = row.model.trim().toLowerCase()
    if (!model) continue
    const card: VideoModelPrice = {}
    for (const [field, resolution] of rowResolutionFields) {
      if (!videoModelSupportsResolution(platform, model, resolution)) continue
      const raw = row[field]
      if (raw === null || raw === '') continue
      const value = Number(raw)
      if (Number.isFinite(value) && value >= 0) card[resolution] = value
    }
    if (Object.keys(card).length === 0) continue
    const unit = normalizeVideoModelBillingUnitForPlatform(platform, row.billing_unit)
    if (unit) card.billing_unit = unit
    prices[model] = card
  }
  return prices
}

export const videoModelPricesPayloadForPlatform = (
  platform: string,
  rows: VideoModelPriceRow[]
): DomainVideoModelPrices | undefined =>
  supportsVideoModelPricingPlatform(platform) ? videoModelPriceRowsToPrices(rows, platform) : undefined
