/**
 * Flatten groups×models into model cards: one card per model, multi-group offers.
 * Merge key = kind + vendor + normalized model name (platform string noise ignored).
 * Offers are de-duplicated by groupId so the same model never appears twice under one group.
 */
import type {
  ModelPlazaGroup,
  ModelPlazaResponse,
  PlazaModel,
  PlazaModelKind,
  PlazaVideoPrices
} from '@/api/modelPlaza'
import type { VideoBillingUnit } from '@/types'
import { resolvePlazaVendor } from './plazaVendors'

export interface PlazaOffer {
  groupId: number
  groupName: string
  platform: string
  description: string
  subscriptionType: string
  rateMultiplier: number
  userRateMultiplier?: number
  effectiveRate: number
  peakRateEnabled: boolean
  peakStart: string
  peakEnd: string
  peakRateMultiplier: number
  isExclusive: boolean
  /** Group-level average first-token latency (ms). Soft sample baseline when cold. */
  avgFirstTokenMs: number
  /** Short disclaimer for TTFT, e.g. based on recent requests. */
  ttftDisclaimer: string
  model: PlazaModel
}

export interface PlazaModelCard {
  key: string
  name: string
  /** Display / primary platform string from first offer. */
  platform: string
  /** Stable vendor id used for merge + shelf grouping. */
  vendorId: string
  kind: PlazaModelKind | string
  official_pricing: PlazaModel['official_pricing']
  offers: PlazaOffer[]
  minRate: number
  maxRate: number
  offerCount: number
}

export function effectiveGroupRate(
  g: Pick<ModelPlazaGroup, 'rate_multiplier' | 'user_rate_multiplier'>
): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

export function normalizePlazaKind(m: PlazaModel): PlazaModelKind | string {
  // Explicit kind from backend wins, except chat+image_prices mis-hints for dual models.
  if (m.kind && m.kind !== 'chat' && m.kind !== 'text') return m.kind
  if (m.video_prices || m.video_billing_unit) return 'video'
  const mode = String(m.pricing?.billing_mode || '').toLowerCase()
  if (mode === 'video') return 'video'
  // Token/chat pricing is $/M — never classify as image just because image_prices is attached.
  if (mode === 'token' || (m.pricing && (m.pricing.input_price != null || m.pricing.output_price != null))) {
    return m.kind === 'text' ? 'text' : 'chat'
  }
  if (mode === 'image' || m.image_prices) return 'image'
  if (m.kind) return m.kind
  return 'chat'
}

export function resolvePlazaVideoBillingUnit(
  model: Pick<PlazaModel, 'video_billing_unit'>
): VideoBillingUnit {
  return model.video_billing_unit === 'per_request' ? 'per_request' : 'per_second'
}

/** Normalize model name so GPT-4o / gpt-4o / gpt_4o merge into one card. */
export function normalizeModelKey(name: string): string {
  return String(name || '')
    .trim()
    .toLowerCase()
    .replace(/[\s_]+/g, '-')
    .replace(/-+/g, '-')
}

export function buildPlazaModelCards(response: ModelPlazaResponse | null | undefined): PlazaModelCard[] {
  const groups = response?.groups ?? []
  const map = new Map<string, PlazaModelCard>()

  for (const g of groups) {
    const rate = effectiveGroupRate(g)
    for (const model of g.models ?? []) {
      const kind = normalizePlazaKind(model)
      const vendor = resolvePlazaVendor(model.platform || g.platform, model.name)
      const nameKey = normalizeModelKey(model.name)
      // One card per model family: kind + vendor + normalized name
      const key = kind + '::' + vendor.id + '::' + nameKey

      let card = map.get(key)
      if (!card) {
        card = {
          key,
          name: model.name,
          platform: model.platform || g.platform || vendor.label,
          vendorId: vendor.id,
          kind,
          official_pricing: model.official_pricing,
          offers: [],
          minRate: rate,
          maxRate: rate,
          offerCount: 0
        }
        map.set(key, card)
      } else {
        // Prefer a more readable display name if a better-cased variant appears later
        if (model.name && model.name.length >= card.name.length && /[A-Z]/.test(model.name)) {
          card.name = model.name
        }
        if (!card.official_pricing && model.official_pricing) {
          card.official_pricing = model.official_pricing
        }
      }

      // De-dupe by group: same model must not list the same group twice
      if (card.offers.some((o) => o.groupId === g.id)) {
        continue
      }

      card.offers.push({
        groupId: g.id,
        groupName: g.name,
        platform: g.platform || model.platform || '',
        description: g.description,
        subscriptionType: g.subscription_type,
        rateMultiplier: g.rate_multiplier,
        userRateMultiplier: g.user_rate_multiplier,
        effectiveRate: rate,
        peakRateEnabled: g.peak_rate_enabled,
        peakStart: g.peak_start,
        peakEnd: g.peak_end,
        peakRateMultiplier: g.peak_rate_multiplier,
        isExclusive: g.is_exclusive,
        avgFirstTokenMs: Number(g.avg_first_token_ms || 0),
        ttftDisclaimer: g.ttft_disclaimer || '',
        model
      })
      card.minRate = Math.min(card.minRate, rate)
      card.maxRate = Math.max(card.maxRate, rate)
      card.offerCount = card.offers.length
    }
  }

  const cards = [...map.values()]
  for (const card of cards) {
    card.offers.sort(
      (a, b) => a.effectiveRate - b.effectiveRate || a.groupName.localeCompare(b.groupName)
    )
    card.offerCount = card.offers.length
  }

  cards.sort((a, b) => {
    const kr = kindRank(a.kind) - kindRank(b.kind)
    if (kr !== 0) return kr
    if (a.vendorId !== b.vendorId) return a.vendorId.localeCompare(b.vendorId)
    return a.name.localeCompare(b.name)
  })
  return cards
}

function kindRank(kind: string): number {
  switch (kind) {
    case 'video':
      return 0
    case 'image':
      return 1
    default:
      return 2
  }
}

export const VIDEO_RESOLUTION_KEYS: (keyof PlazaVideoPrices)[] = [
  '480p',
  '720p',
  '1080p',
  '1440p',
  '2160p'
]

export const IMAGE_TIER_KEYS = ['1K', '2K', '4K'] as const


/** Resolve token unit base for an offer: own channel pricing → sibling → official. */
export function resolveOfferTokenBase(
  offer: Pick<PlazaOffer, 'model' | 'effectiveRate'>,
  field: 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price',
  opts?: {
    siblingPricing?: PlazaModel['pricing'] | null
    official?: PlazaModel['official_pricing'] | null
  }
): number | null {
  const own = offer.model.pricing?.[field]
  if (own != null) return Number(own)
  const sib = opts?.siblingPricing?.[field]
  if (sib != null) return Number(sib)
  const off = opts?.official?.[field as keyof NonNullable<PlazaModel['official_pricing']>]
  if (off != null) return Number(off)
  const off2 = offer.model.official_pricing?.[field as keyof NonNullable<PlazaModel['official_pricing']>]
  if (off2 != null) return Number(off2)
  return null
}

/** Paid $/token * rate (caller multiplies by 1e6 for $/M display). */
export function paidTokenUnit(
  offer: Pick<PlazaOffer, 'model' | 'effectiveRate'>,
  field: 'input_price' | 'output_price',
  opts?: {
    siblingPricing?: PlazaModel['pricing'] | null
    official?: PlazaModel['official_pricing'] | null
  }
): number | null {
  const base = resolveOfferTokenBase(offer, field, opts)
  if (base == null || !Number.isFinite(base)) return null
  return base * offer.effectiveRate
}
