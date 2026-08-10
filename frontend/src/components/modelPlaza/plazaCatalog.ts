/**
 * Flatten groups×models into model cards: one card per model, multi-group offers.
 */
import type {
  ModelPlazaGroup,
  ModelPlazaResponse,
  PlazaModel,
  PlazaModelKind,
  PlazaVideoPrices
} from '@/api/modelPlaza'

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
  platform: string
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
  if (m.kind) return m.kind
  if (m.video_prices || m.video_billing_unit) return 'video'
  if (m.image_prices) return 'image'
  const mode = m.pricing?.billing_mode
  if (mode === 'image') return 'image'
  if (mode === 'video') return 'video'
  return 'chat'
}

export function buildPlazaModelCards(response: ModelPlazaResponse | null | undefined): PlazaModelCard[] {
  const groups = response?.groups ?? []
  const map = new Map<string, PlazaModelCard>()

  for (const g of groups) {
    const rate = effectiveGroupRate(g)
    for (const model of g.models ?? []) {
      const kind = normalizePlazaKind(model)
      const key = kind + '::' + model.platform + '::' + model.name
      let card = map.get(key)
      if (!card) {
        card = {
          key,
          name: model.name,
          platform: model.platform,
          kind,
          official_pricing: model.official_pricing,
          offers: [],
          minRate: rate,
          maxRate: rate,
          offerCount: 0
        }
        map.set(key, card)
      } else if (!card.official_pricing && model.official_pricing) {
        card.official_pricing = model.official_pricing
      }

      card.offers.push({
        groupId: g.id,
        groupName: g.name,
        platform: g.platform,
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
  }

  cards.sort((a, b) => {
    const kr = kindRank(a.kind) - kindRank(b.kind)
    if (kr !== 0) return kr
    if (a.platform !== b.platform) return a.platform.localeCompare(b.platform)
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
