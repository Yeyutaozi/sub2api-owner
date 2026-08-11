<template>
  <article
    class="mp-card"
    :class="{ 'mp-card--multi': card.offerCount > 1 }"
    :data-kind="card.kind"
    :data-vendor="vendor.id"
    :data-offers="card.offerCount"
    :style="cardToneStyle"
  >
    <header class="mp-card__head">
      <div class="mp-card__rail" aria-hidden="true" />
      <div class="mp-card__head-main">
        <div class="mp-card__title-row">
          <PlazaVendorMark :platform="card.platform" :model-name="card.name" size="md" />
          <div class="mp-card__title-text">
            <div class="mp-card__name-line">
              <span class="mp-card__kind" :data-kind="card.kind">{{ kindLabel }}</span>
              <h3 class="mp-card__name" :title="card.name">{{ card.name }}</h3>
            </div>
            <div class="mp-card__meta">
              <span class="mp-card__platform" :title="card.platform">
                <PlazaVendorMark :platform="card.platform" :model-name="card.name" size="sm" />
                <span>{{ vendor.label }}</span>
              </span>
              <span class="mp-card__chip mp-card__chip--groups">
                {{ t('modelPlaza.card.offers', { n: card.offerCount }) }}
              </span>
              <span class="mp-card__chip mp-card__chip--rate">
                <template v-if="card.minRate !== card.maxRate">
                  {{ formatRate(card.minRate) }} - {{ formatRate(card.maxRate) }}
                </template>
                <template v-else>{{ formatRate(card.minRate) }} · </template>
              </span>
            </div>
          </div>
        </div>
      </div>
    </header>

    <!-- Multi-group: compact stacked rows (stay in grid, no full-bleed) -->
    <div v-if="card.offerCount > 1" class="mp-multi" data-testid="plaza-model-groups">
      <div class="mp-multi__bar">
        <span class="mp-multi__title">{{ t('modelPlaza.card.groupMountTitle') }}</span>
        <span class="mp-multi__count">{{ card.offerCount }}</span>
      </div>
      <div class="mp-multi__list">
        <div
          v-for="(offer, idx) in card.offers"
          :key="offer.groupId"
          class="mp-offer"
          :class="{ 'is-best': isBestOffer(idx) }"
          data-testid="plaza-offer-panel"
          :style="groupTicketStyle(idx)"
        >
          <div class="mp-offer__main">
            <div class="mp-offer__identity">
              <span class="mp-offer__dot" aria-hidden="true" />
              <div class="mp-offer__names">
                <span class="mp-offer__group" :title="offer.groupName">{{ offer.groupName }}</span>
                <div class="mp-offer__pills">
                  <span v-if="isBestOffer(idx)" class="mp-pill mp-pill--best">{{ t('modelPlaza.card.bestOffer') }}</span>
                  <span v-if="offer.isExclusive" class="mp-pill mp-pill--exclusive">{{ t('modelPlaza.badges.exclusive') }}</span>
                  <span v-if="tokenPriceSource(offer) === 'fallback'" class="mp-pill mp-pill--est" :title="t('modelPlaza.table.priceFallbackShort')">{{ t('modelPlaza.table.priceEstimateBadge') }}</span>
                </div>
              </div>
            </div>
            <div class="mp-offer__metrics">
              <span class="mp-offer__rate mono" :title="t('modelPlaza.table.rate')">{{ formatRate(offer.effectiveRate) }}</span>
              <span
                class="mp-offer__ttft mono"
                data-testid="plaza-offer-ttft"
                :title="offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer')"
              >{{ formatFirstToken(offer.avgFirstTokenMs) }}</span>
            </div>
          </div>

          <div v-if="card.kind === 'video'" class="mp-offer__prices">
            <span
              v-for="res in multiVideoResolutions.length ? multiVideoResolutions : videoResolutions(offer)"
              :key="offer.groupId + '-mv-' + res"
              class="mp-chip-price mp-chip-price--xs"
              :class="{ 'is-empty': !hasVideoPrice(offer, res) }"
            >
              <em>{{ formatRes(res) }}</em>
              <strong>{{ formatVideoPrice(offer, res) }}</strong>
            </span>
            <span v-if="!(multiVideoResolutions.length || videoResolutions(offer).length)" class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</span>
          </div>
          <div v-else-if="card.kind === 'image'" class="mp-offer__prices">
            <template v-if="hasImageConfiguredPrice(offer) || siblingImagePrices">
              <span
                v-for="tier in multiImageTiers.length ? multiImageTiers : imageTiers(offer)"
                :key="offer.groupId + '-mi-' + tier"
                class="mp-chip-price mp-chip-price--xs"
                :class="{ 'is-empty': formatImagePrice(offer, tier) === emDash }"
              >
                <em>{{ formatImageTier(tier) }}</em>
                <strong>{{ formatImagePrice(offer, tier) }}</strong>
              </span>
            </template>
            <template v-else-if="hasTokenLikePrice(offer)">
              <span class="mp-chip-price mp-chip-price--xs" :class="{ 'is-empty': paidToken(offer, 'input_price') === emDash }">
                <em>{{ t('modelPlaza.table.input') }}</em>
                <strong>{{ paidToken(offer, 'input_price') }} <i>$/M</i></strong>
              </span>
              <span class="mp-chip-price mp-chip-price--xs" :class="{ 'is-empty': paidToken(offer, 'output_price') === emDash }">
                <em>{{ t('modelPlaza.table.output') }}</em>
                <strong>{{ paidToken(offer, 'output_price') }} <i>$/M</i></strong>
              </span>
            </template>
            <span v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</span>
          </div>
          <div v-else class="mp-offer__prices mp-offer__prices--token">
            <span class="mp-chip-price mp-chip-price--xs" :class="{ 'is-empty': paidToken(offer, 'input_price') === emDash }">
              <em>{{ t('modelPlaza.table.input') }}</em>
              <strong>{{ paidToken(offer, 'input_price') }} <i>$/M</i></strong>
            </span>
            <span class="mp-chip-price mp-chip-price--xs" :class="{ 'is-empty': paidToken(offer, 'output_price') === emDash }">
              <em>{{ t('modelPlaza.table.output') }}</em>
              <strong>{{ paidToken(offer, 'output_price') }} <i>$/M</i></strong>
            </span>
            <span class="mp-chip-price mp-chip-price--xs" :class="{ 'is-empty': cachePaid(offer) === emDash }">
              <em>{{ t('modelPlaza.table.cache') }}</em>
              <strong>{{ cachePaid(offer) }} <i>$/M</i></strong>
            </span>
          </div>
        </div>
      </div>
      <p class="mp-multi__note">
        <template v-if="card.kind === 'video'">{{ t('modelPlaza.table.videoBillingPerClipHint') }} · </template>
          <template v-else-if="card.kind === 'image'">{{ t('modelPlaza.table.imageBillingHint') }} · </template>
          <template v-else>{{ t('modelPlaza.table.tokenBillingHint') }} · </template>
        {{ t('modelPlaza.table.rateAppliedNote') }}
        <span v-if="hasAnyFallback" class="mp-multi__fallback"> · {{ t('modelPlaza.table.priceFallbackNote') }}</span>
      </p>
    </div>

    <!-- Single offer: compact card body -->
    <div v-else class="mp-solo" data-testid="plaza-model-groups">
      <section
        v-for="(offer, idx) in card.offers"
        :key="offer.groupId"
        class="mp-group is-solo"
        :style="groupTicketStyle(idx)"
        data-testid="plaza-offer-panel"
      >
        <div class="mp-group__top">
          <div class="mp-group__identity">
            <span class="mp-group__kicker">{{ t('modelPlaza.card.groupLabel') }}</span>
            <h4 class="mp-group__name" :title="offer.groupName">{{ offer.groupName }}</h4>
            <div class="mp-group__pills">
              <span v-if="offer.isExclusive" class="mp-pill mp-pill--exclusive">{{ t('modelPlaza.badges.exclusive') }}</span>
              <span v-if="offer.subscriptionType === 'subscription'" class="mp-pill mp-pill--sub">{{ t('modelPlaza.badges.subscription') }}</span>
              <span v-if="tokenPriceSource(offer) === 'fallback'" class="mp-pill mp-pill--est">{{ t('modelPlaza.table.priceEstimateBadge') }}</span>
            </div>
          </div>
          <div class="mp-group__metrics">
            <div class="mp-metric mp-metric--rate">
              <span class="mp-metric__label">{{ t('modelPlaza.table.rate') }}</span>
              <span class="mp-metric__value">{{ formatRate(offer.effectiveRate) }}</span>
            </div>
            <div
              class="mp-metric mp-metric--ttft"
              :title="offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer')"
              data-testid="plaza-offer-ttft"
            >
              <span class="mp-metric__label">{{ t('modelPlaza.detail.avgFirstToken') }}</span>
              <span class="mp-metric__value">{{ formatFirstToken(offer.avgFirstTokenMs) }}</span>
            </div>
          </div>
        </div>

        <div v-if="card.kind === 'video'" class="mp-price-row">
          <span
            v-for="res in videoResolutions(offer)"
            :key="offer.groupId + '-sv-' + res"
            class="mp-chip-price"
            :class="{ 'is-empty': !hasVideoPrice(offer, res) }"
          >
            <em>{{ formatRes(res) }}</em>
            <strong>{{ formatVideoPrice(offer, res) }}</strong>
          </span>
          <span v-if="!videoResolutions(offer).length" class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</span>
        </div>
        <div v-else-if="card.kind === 'image'" class="mp-price-row">
          <template v-if="hasImageConfiguredPrice(offer) || siblingImagePrices">
            <span
              v-for="tier in imageTiers(offer)"
              :key="offer.groupId + '-si-' + tier"
              class="mp-chip-price"
              :class="{ 'is-empty': formatImagePrice(offer, tier) === emDash }"
            >
              <em>{{ formatImageTier(tier) }}</em>
              <strong>{{ formatImagePrice(offer, tier) }}</strong>
            </span>
          </template>
          <template v-else-if="hasTokenLikePrice(offer)">
            <span class="mp-chip-price" :class="{ 'is-empty': paidToken(offer, 'input_price') === emDash }">
              <em>{{ t('modelPlaza.table.input') }}</em>
              <strong>{{ paidToken(offer, 'input_price') }} <i>$/M</i></strong>
            </span>
            <span class="mp-chip-price" :class="{ 'is-empty': paidToken(offer, 'output_price') === emDash }">
              <em>{{ t('modelPlaza.table.output') }}</em>
              <strong>{{ paidToken(offer, 'output_price') }} <i>$/M</i></strong>
            </span>
            <span v-if="tokenPriceSource(offer) === 'fallback'" class="mp-pill mp-pill--est">{{ t('modelPlaza.table.priceEstimateBadge') }}</span>
          </template>
          <span v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</span>
        </div>
        <div v-else class="mp-price-row mp-price-row--token">
          <span class="mp-chip-price">
            <em>{{ t('modelPlaza.table.input') }}</em>
            <strong>{{ paidToken(offer, 'input_price') }} <i>$/M</i></strong>
          </span>
          <span class="mp-chip-price">
            <em>{{ t('modelPlaza.table.output') }}</em>
            <strong>{{ paidToken(offer, 'output_price') }} <i>$/M</i></strong>
          </span>
          <span class="mp-chip-price">
            <em>{{ t('modelPlaza.table.cache') }}</em>
            <strong>{{ cachePaid(offer) }} <i>$/M</i></strong>
          </span>
        </div>
        <p class="mp-solo__note">
          <template v-if="card.kind === 'video'">{{ videoHint(offer) }} · </template>
          <template v-else-if="card.kind === 'image'">{{ t('modelPlaza.table.imageBillingHint') }} · </template>
          <template v-else>{{ t('modelPlaza.table.tokenBillingHint') }} · </template>
          {{ t('modelPlaza.table.rateAppliedNote') }}
        </p>
        <div v-if="offer.peakRateEnabled" class="mp-group__foot">
          <span class="mp-peak">{{ t('modelPlaza.detail.peakNote', { window: peakWindow(offer), multiplier: offer.peakRateMultiplier }) }}</span>
        </div>
      </section>
    </div>

    <footer v-if="card.official_pricing && (card.kind === 'chat' || card.kind === 'text')" class="mp-card__official">
      <span class="mp-official__k">{{ t('modelPlaza.table.officialPrice') }}</span>
      <span class="mp-official__v">
        {{ t('modelPlaza.table.input') }} {{ officialToken(card.official_pricing.input_price) }}
        · {{ t('modelPlaza.table.output') }} {{ officialToken(card.official_pricing.output_price) }}
        <span class="mp-official__unit">{{ t('modelPlaza.table.unitPerMillion') }}</span>
      </span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  IMAGE_TIER_KEYS,
  VIDEO_RESOLUTION_KEYS,
  type PlazaModelCard,
  type PlazaOffer
} from './plazaCatalog'
import PlazaVendorMark from './PlazaVendorMark.vue'
import { resolvePlazaVendor } from './plazaVendors'

const props = defineProps<{
  card: PlazaModelCard
}>()

const { t } = useI18n()
const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2
const emDash = '\u2014'

const GROUP_ACCENTS = [
  { color: '#0f766e', soft: '#ccfbf1' },
  { color: '#b45309', soft: '#fef3c7' },
  { color: '#1d4ed8', soft: '#dbeafe' },
  { color: '#be123c', soft: '#ffe4e6' },
  { color: '#7c3aed', soft: '#ede9fe' },
  { color: '#0e7490', soft: '#cffafe' }
]

const vendor = computed(() => resolvePlazaVendor(props.card.vendorId || props.card.platform, props.card.name))

const cardToneStyle = computed(() => ({
  '--vendor-color': vendor.value.color,
  '--vendor-soft': vendor.value.soft
}))

const kindLabel = computed(() => {
  switch (props.card.kind) {
    case 'video':
      return t('modelPlaza.kind.video')
    case 'image':
      return t('modelPlaza.kind.image')
    default:
      return t('modelPlaza.kind.chat')
  }
})

/** Union of video resolutions across offers for multi table columns. */
const multiVideoResolutions = computed(() => {
  const set = new Set<string>()
  for (const offer of props.card.offers) {
    for (const r of videoResolutions(offer)) set.add(r)
  }
  return VIDEO_RESOLUTION_KEYS.filter((k) => set.has(k)) as string[]
})

/** Union of image tiers across offers for multi table columns. */
const multiImageTiers = computed(() => {
  const set = new Set<string>()
  for (const offer of props.card.offers) {
    for (const tier of imageTiers(offer)) {
      if (tier && tier !== emDash) set.add(tier)
    }
  }
  const known = IMAGE_TIER_KEYS.filter((k) => set.has(k)) as string[]
  const knownSet = new Set<string>(IMAGE_TIER_KEYS as unknown as string[])
  const extra = [...set].filter((k) => !knownSet.has(k))
  return known.length || extra.length ? [...known, ...extra] : (IMAGE_TIER_KEYS as unknown as string[])
})

const hasAnyFallback = computed(() =>
  props.card.offers.some((o) => tokenPriceSource(o) === 'fallback')
)

/** Sibling channel base pricing when this offer has no pricing of its own. */
const siblingTokenBase = computed(() => {
  for (const o of props.card.offers) {
    const p = o.model.pricing
    if (p && (p.input_price != null || p.output_price != null || p.cache_write_price != null || p.cache_read_price != null)) {
      return p
    }
  }
  return null
})

/** Sibling video/image matrices for multi-group cards missing local prices. */
const siblingVideoPrices = computed(() => {
  for (const o of props.card.offers) {
    const prices = o.model.video_prices
    if (!prices) continue
    if (VIDEO_RESOLUTION_KEYS.some((k) => prices[k] != null)) return prices
  }
  return null
})

const siblingImagePrices = computed(() => {
  for (const o of props.card.offers) {
    const prices = o.model.image_prices
    if (!prices) continue
    if (Object.values(prices).some((v) => v != null)) return prices
  }
  return null
})


function groupTicketStyle(idx: number): Record<string, string> {
  const a = GROUP_ACCENTS[idx % GROUP_ACCENTS.length]
  return {
    '--group-color': a.color,
    '--group-soft': a.soft
  }
}

function isBestOffer(idx: number): boolean {
  if (idx !== 0 || props.card.offers.length < 2) return false
  return props.card.minRate < props.card.maxRate
}

function formatRate(rate: number): string {
  const s = Number.isInteger(rate)
    ? String(rate)
    : rate.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
  return s + 'x'
}

function formatFirstToken(ms: number | null | undefined): string {
  const n = Number(ms || 0)
  if (!Number.isFinite(n) || n <= 0) return emDash
  if (n >= 1000) return (n / 1000).toFixed(1) + 's'
  return Math.round(n) + 'ms'
}

function formatRes(res: string): string {
  return String(res).toUpperCase()
}



function videoHint(offer: PlazaOffer): string {
  const unit = String(offer.model.video_billing_unit || '').toLowerCase()
  if (unit === 'second' || unit === 'per_second' || unit === 'sec') {
    return t('modelPlaza.table.videoBillingPerSecondHint')
  }
  return t('modelPlaza.table.videoBillingPerClipHint')
}

function hasImageConfiguredPrice(offer: PlazaOffer): boolean {
  const tiers = imageTiers(offer)
  if (!tiers.length) return false
  return tiers.some((tier) => formatImagePrice(offer, tier) !== emDash)
}

function hasTokenLikePrice(offer: PlazaOffer): boolean {
  return (
    resolveTokenBase(offer, 'input_price') != null ||
    resolveTokenBase(offer, 'output_price') != null ||
    resolveTokenBase(offer, 'cache_write_price') != null ||
    resolveTokenBase(offer, 'cache_read_price') != null
  )
}


function formatImageTier(tier: string): string {
  const raw = String(tier || '').trim()
  if (!raw || raw === emDash) return emDash
  if (/^[0-9]+[kK]$/.test(raw)) return raw.toUpperCase()
  return raw
}

function videoResolutions(offer: PlazaOffer): string[] {
  const listed = (offer.model.video_resolutions || [])
    .map((x) => String(x).toLowerCase())
    .filter(Boolean)
  if (listed.length) {
    return VIDEO_RESOLUTION_KEYS.filter((k) => listed.includes(k)) as string[]
  }
  const prices = offer.model.video_prices || siblingVideoPrices.value
  if (!prices) return []
  return VIDEO_RESOLUTION_KEYS.filter((k) => prices[k] != null) as string[]
}

function hasVideoPrice(offer: PlazaOffer, res: string): boolean {
  const raw = offer.model.video_prices?.[res as keyof typeof offer.model.video_prices]
  return raw != null
}

function formatScaled(v: number, scale: number, minDec: number): string {
  const n = Number(v) * scale
  if (!Number.isFinite(n)) return emDash
  const abs = Math.abs(n)
  let dec = minDec
  if (abs > 0 && abs < 0.01) dec = 4
  else if (abs > 0 && abs < 0.1) dec = 3
  const s = n.toFixed(dec).replace(/0+$/, '').replace(/\.$/, '')
  return '$' + s
}

function formatVideoPrice(offer: PlazaOffer, res: string): string {
  const raw = offer.model.video_prices?.[res as keyof typeof offer.model.video_prices]
  if (raw == null) return emDash
  return formatScaled(Number(raw) * offer.effectiveRate, 1, MIN_DECIMALS)
}

function imageTiers(offer: PlazaOffer): string[] {
  const prices = offer.model.image_prices || siblingImagePrices.value
  if (!prices) {
    if (offer.model.pricing?.billing_mode === 'image') {
      const labels = (offer.model.pricing.intervals || [])
        .map((i) => i.tier_label)
        .filter(Boolean) as string[]
      if (labels.length) return labels
      return [emDash]
    }
    return []
  }
  const known = IMAGE_TIER_KEYS.filter((k) => k in prices || prices[k] != null)
  const knownSet = new Set<string>(IMAGE_TIER_KEYS as unknown as string[])
  const extras = Object.keys(prices).filter((k) => !knownSet.has(k))
  return [...known, ...extras]
}

function formatImagePrice(offer: PlazaOffer, tier: string): string {
  const prices = offer.model.image_prices
  if (prices && prices[tier] != null) {
    return formatScaled(Number(prices[tier]) * offer.effectiveRate, 1, MIN_DECIMALS)
  }
  const sib = siblingImagePrices.value
  if (sib && sib[tier] != null) {
    return formatScaled(Number(sib[tier]) * offer.effectiveRate, 1, MIN_DECIMALS)
  }
  if (offer.model.pricing?.billing_mode === 'image') {
    const hit = (offer.model.pricing.intervals || []).find((i) => i.tier_label === tier)
    const v = hit?.per_request_price ?? offer.model.pricing.per_request_price
    if (v == null) return emDash
    return formatScaled(Number(v) * offer.effectiveRate, 1, MIN_DECIMALS)
  }
  return emDash
}

type TokenField = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price'

/** Resolve base unit price: own pricing -> sibling group -> official. */
function resolveTokenBase(offer: PlazaOffer, field: TokenField): number | null {
  const own = offer.model.pricing?.[field]
  if (own != null) return Number(own)
  const sib = siblingTokenBase.value?.[field]
  if (sib != null) return Number(sib)
  const off = props.card.official_pricing?.[field as keyof typeof props.card.official_pricing]
  if (off != null) return Number(off)
  // also try offer-level official
  const off2 = offer.model.official_pricing?.[field as keyof NonNullable<typeof offer.model.official_pricing>]
  if (off2 != null) return Number(off2)
  return null
}

function tokenPriceSource(offer: PlazaOffer): 'channel' | 'fallback' | 'none' {
  // media matrix counts as channel pricing
  if (props.card.kind === 'video' && (offer.model.video_prices || siblingVideoPrices.value)) {
    const prices = offer.model.video_prices || siblingVideoPrices.value
    if (prices && Object.values(prices).some((v) => v != null)) {
      return offer.model.video_prices ? 'channel' : 'fallback'
    }
  }
  if (props.card.kind === 'image' && hasImageConfiguredPrice(offer)) return 'channel'
  if (props.card.kind === 'image' && siblingImagePrices.value) return 'fallback'
  const p = offer.model.pricing
  if (p && (p.input_price != null || p.output_price != null || p.cache_write_price != null || p.cache_read_price != null)) {
    return 'channel'
  }
  if (siblingTokenBase.value || props.card.official_pricing || offer.model.official_pricing) {
    return 'fallback'
  }
  return 'none'
}

function paidToken(offer: PlazaOffer, field: 'input_price' | 'output_price'): string {
  const v = resolveTokenBase(offer, field)
  if (v == null || !Number.isFinite(v)) return emDash
  return formatScaled(v * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
}

function cachePaid(offer: PlazaOffer): string {
  const w = resolveTokenBase(offer, 'cache_write_price')
  const r = resolveTokenBase(offer, 'cache_read_price')
  if (w == null && r == null) return emDash
  const ws = w == null ? emDash : formatScaled(w * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  const rs = r == null ? emDash : formatScaled(r * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  if (ws === emDash) return rs
  if (rs === emDash) return ws
  return ws + '/' + rs
}

function officialToken(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(Number(v))) return emDash
  return formatScaled(Number(v), PER_MILLION, MIN_DECIMALS)
}

function peakWindow(offer: PlazaOffer): string {
  const a = offer.peakStart || '?'
  const b = offer.peakEnd || '?'
  return a + ' - ' + b
}

</script>

<style scoped>
.mp-card {
  --mp-ink: #0f172a;
  --mp-muted: #64748b;
  --mp-faint: #94a3b8;
  --mp-line: #e2e8f0;
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  border-radius: 16px;
  border: 1px solid color-mix(in srgb, var(--vendor-color, #0f766e) 18%, #e2e8f0);
  background: #fff;
  box-shadow: 0 1px 0 rgba(15, 23, 42, 0.03), 0 10px 28px -22px rgba(15, 23, 42, 0.35);
  overflow: hidden;
}
.mp-card__rail {
  position: absolute;
  inset: 0 auto 0 0;
  width: 3px;
  background: linear-gradient(180deg, var(--vendor-color, #0f766e), color-mix(in srgb, var(--vendor-color, #0f766e) 40%, #94a3b8));
}
.mp-card__head {
  padding: 12px 14px 10px 16px;
  border-bottom: 1px solid var(--mp-line);
  background:
    radial-gradient(120% 80% at 0% 0%, color-mix(in srgb, var(--vendor-soft, #ccfbf1) 70%, #fff), transparent 60%),
    linear-gradient(180deg, #fcfdff, #fff);
}
.mp-card__title-row { display: flex; gap: 10px; align-items: flex-start; min-width: 0; }
.mp-card__title-text { min-width: 0; flex: 1; }
.mp-card__name-line { display: flex; align-items: center; gap: 8px; min-width: 0; }
.mp-card__kind {
  flex: 0 0 auto;
  height: 20px;
  padding: 0 7px;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #475569;
}
.mp-card__kind[data-kind='video'] { color: #9a3412; background: #ffedd5; border-color: #fed7aa; }
.mp-card__kind[data-kind='image'] { color: #1d4ed8; background: #dbeafe; border-color: #bfdbfe; }
.mp-card__kind[data-kind='chat'],
.mp-card__kind[data-kind='text'] { color: #0f766e; background: #ccfbf1; border-color: #99f6e4; }
.mp-card__name {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--mp-ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mp-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
  align-items: center;
}
.mp-card__platform {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  font-size: 11px;
  font-weight: 650;
  color: var(--mp-muted);
}
.mp-card__chip {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 750;
  border: 1px solid var(--mp-line);
  background: #fff;
  color: #475569;
}
.mp-card__chip--rate { color: #047857; background: #ecfdf5; border-color: #a7f3d0; }

/* Dense multi-group compare */

.mp-multi { padding: 6px 8px 8px; background: linear-gradient(180deg, #f8fafc, #fff); border-top: 1px solid var(--mp-line, #e5e7eb); }
.mp-multi__bar { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-bottom:6px; }
.mp-multi__title { font-family: var(--fc-mono, ui-monospace, monospace); font-size:10px; font-weight:800; letter-spacing:.12em; text-transform:uppercase; color:#64748b; }
.mp-multi__count { min-width:22px; height:22px; border-radius:999px; display:inline-flex; align-items:center; justify-content:center; font-size:11px; font-weight:700; color:#0f766e; background:#ccfbf1; border:1px solid #99f6e4; }
.mp-multi__list { display:flex; flex-direction:column; gap:4px; max-height:210px; overflow:auto; padding-right:2px; }
.mp-offer {
  border-radius:10px;
  border:1px solid color-mix(in srgb, var(--group-color, #0f766e) 22%, var(--mp-line, #e5e7eb));
  background: linear-gradient(180deg, color-mix(in srgb, var(--group-soft, #ccfbf1) 42%, #fff), #fff);
  padding:5px 7px;
}
.mp-offer.is-best { box-shadow: inset 3px 0 0 var(--group-color, #0f766e); }
.mp-offer__main { display:grid; grid-template-columns:minmax(0,1fr) auto; gap:6px; align-items:center; }
.mp-offer__identity { display:flex; gap:8px; min-width:0; align-items:flex-start; }
.mp-offer__dot { width:8px; height:8px; margin-top:5px; border-radius:999px; background:var(--group-color,#0f766e); box-shadow:0 0 0 3px color-mix(in srgb, var(--group-soft,#ccfbf1) 80%, transparent); flex:0 0 auto; }
.mp-offer__names { min-width:0; }
.mp-offer__group { display:block; font-size:12.5px; font-weight:700; color:#0f172a; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; max-width:100%; }
.mp-offer__pills { display:flex; flex-wrap:wrap; gap:4px; margin-top:3px; }
.mp-offer__metrics { display:flex; flex-direction:column; align-items:flex-end; gap:2px; flex:0 0 auto; }
.mp-offer__rate { font-size:12px; font-weight:750; color:#0f766e; }
.mp-offer__ttft { font-size:11px; font-weight:650; color:#64748b; }
.mp-offer__prices { display:flex; flex-wrap:wrap; gap:4px; margin-top:5px; }
.mp-chip-price--xs { padding:2px 6px !important; font-size:10.5px !important; }
.mp-chip-price--xs em { font-size:9.5px !important; }
.mp-chip-price--xs strong { font-size:11px !important; }
.mp-chip-price--xs i { font-style:normal; font-size:9px; opacity:.7; margin-left:2px; }
.mp-empty-price { font-size:11px; color:#94a3b8; }
.mp-multi__note { margin:6px 0 0; font-size:10.5px; line-height:1.4; color:#94a3b8; }
.mp-multi__fallback { color:#b45309; font-weight:650; }
:global(.dark) .mp-multi { background: linear-gradient(180deg, #0b1220, #111827); border-top-color:#243041; }
:global(.dark) .mp-offer { background:#0b1220; border-color:#243041; }
:global(.dark) .mp-offer__group { color:#e2e8f0; }
:global(.dark) .mp-offer__ttft { color:#94a3b8; }
:global(.dark) .mp-multi__title { color:#94a3b8; }

.mp-solo { padding: 10px 12px 12px; }
.mp-group {
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--group-color, #0f766e) 20%, var(--mp-line));
  background: linear-gradient(180deg, color-mix(in srgb, var(--group-soft, #ccfbf1) 35%, #fff), #fff);
  padding: 10px 12px;
}
.mp-group__top {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: start;
}
.mp-group__kicker {
  display: block;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--mp-faint);
}
.mp-group__name {
  margin: 2px 0 0;
  font-size: 14px;
  font-weight: 800;
  color: var(--mp-ink);
  line-height: 1.25;
}
.mp-group__pills { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.mp-group__metrics { display: flex; gap: 8px; }
.mp-metric {
  min-width: 72px;
  padding: 6px 8px;
  border-radius: 10px;
  border: 1px solid var(--mp-line);
  background: #fff;
}
.mp-metric__label {
  display: block;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--mp-faint);
}
.mp-metric__value {
  display: block;
  margin-top: 2px;
  font-family: "JetBrains Mono", ui-monospace, monospace;
  font-size: 15px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: var(--mp-ink);
}
.mp-metric--rate .mp-metric__value { color: #047857; }
.mp-metric--ttft .mp-metric__value { color: #c2410c; }
.mp-price-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}
.mp-chip-price {
  display: inline-flex;
  flex-direction: column;
  gap: 2px;
  min-width: 78px;
  padding: 6px 8px;
  border-radius: 10px;
  border: 1px solid var(--mp-line);
  background: #fff;
}
.mp-chip-price em {
  font-style: normal;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--mp-faint);
}
.mp-chip-price strong {
  font-family: "JetBrains Mono", ui-monospace, monospace;
  font-size: 12px;
  font-weight: 750;
  color: var(--mp-ink);
  font-variant-numeric: tabular-nums;
}
.mp-chip-price strong i {
  font-style: normal;
  font-size: 10px;
  font-weight: 700;
  color: #64748b;
  margin-left: 2px;
}
.mp-chip-price.is-empty strong { color: var(--mp-faint); }
.mp-solo__note {
  margin: 8px 0 0;
  font-size: 11px;
  line-height: 1.4;
  color: var(--mp-muted);
}
.mp-group__foot { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.mp-peak {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  color: #9a3412;
  background: #ffedd5;
  border: 1px solid rgba(234, 88, 12, 0.2);
}
.mp-empty-price { color: var(--mp-faint); font-size: 12px; padding: 4px 2px; }
.mp-card__official {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
  padding: 8px 14px 10px;
  border-top: 1px dashed var(--mp-line);
  background: #f8fafc;
}
.mp-official__k {
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--mp-faint);
}
.mp-official__v {
  font-family: "JetBrains Mono", ui-monospace, monospace;
  font-size: 12px;
  font-weight: 700;
  color: var(--mp-ink);
}
.mp-official__unit {
  display: inline-block;
  margin-left: 6px;
  font-size: 10px;
  font-weight: 600;
  color: #94a3b8;
  font-family: inherit;
}
@media (max-width: 720px) {
  .mp-group__top { grid-template-columns: 1fr; }
}
:global(.dark) .mp-card { background: #111827; border-color: #243041; box-shadow: none; }
:global(.dark) .mp-card__head,
:global(.dark) .mp-card__official { background: #0f172a; border-color: #243041; }
:global(.dark) .mp-group,
:global(.dark) .mp-metric,
:global(.dark) .mp-chip-price { background: #0b1220; border-color: #243041; }
:global(.dark) .mp-card__name,
:global(.dark) .mp-group__name,
:global(.dark) .mp-metric__value,
:global(.dark) .mp-row-name__text,
:global(.dark) .mp-official__v,
:global(.dark) .mp-chip-price strong { color: #e5eef8; }
:global(.dark) .mp-card__chip { background: #0f172a; border-color: #243041; color: #94a3b8; }
</style>
