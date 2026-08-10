<template>
  <article class="mp-card" :data-kind="card.kind">
    <header class="mp-card__head">
      <div class="mp-card__rail" aria-hidden="true" />
      <div class="mp-card__head-main">
        <div class="mp-card__title-row">
          <span class="mp-card__kind" :data-kind="card.kind">{{ kindLabel }}</span>
          <h3 class="mp-card__name" :title="card.name">{{ card.name }}</h3>
        </div>
        <div class="mp-card__meta">
          <span class="mp-card__platform">{{ card.platform }}</span>
          <span class="mp-card__dot" aria-hidden="true" />
          <span class="mp-card__offers">
            {{ t('modelPlaza.card.offers', { n: card.offerCount }) }}
          </span>
          <span class="mp-card__dot" aria-hidden="true" />
          <span class="mp-card__rate-range">
            <template v-if="card.minRate !== card.maxRate">
              {{ formatRate(card.minRate) }}–{{ formatRate(card.maxRate) }}
            </template>
            <template v-else>{{ formatRate(card.minRate) }}</template>
          </span>
        </div>
      </div>
    </header>

    <div class="mp-card__offers-list">
      <section
        v-for="(offer, idx) in card.offers"
        :key="offer.groupId"
        class="mp-offer"
        :class="{ 'is-best': isBestOffer(idx) }"
      >
        <div class="mp-offer__head">
          <div class="mp-offer__group">
            <span class="mp-offer__name">{{ offer.groupName }}</span>
            <span v-if="isBestOffer(idx)" class="mp-pill mp-pill--best">
              {{ t('modelPlaza.card.bestOffer') }}
            </span>
            <span v-if="offer.isExclusive" class="mp-pill mp-pill--exclusive">
              {{ t('modelPlaza.badges.exclusive') }}
            </span>
            <span
              v-if="offer.subscriptionType === 'subscription'"
              class="mp-pill mp-pill--sub"
            >
              {{ t('modelPlaza.badges.subscription') }}
            </span>
          </div>
          <div class="mp-offer__rate" title="effective group rate">
            <span class="mp-offer__rate-val">{{ formatRate(offer.effectiveRate) }}</span>
            <span
              v-if="
                offer.userRateMultiplier != null &&
                offer.userRateMultiplier !== offer.rateMultiplier
              "
              class="mp-offer__rate-base"
            >
              {{ t('modelPlaza.card.baseRate', { rate: formatRate(offer.rateMultiplier) }) }}
            </span>
          </div>
        </div>

        <p v-if="offer.description" class="mp-offer__desc">{{ offer.description }}</p>

        <div v-if="card.kind === 'video'" class="mp-tiers">
          <div class="mp-tiers__bar">
            <span class="mp-tiers__unit">{{ videoUnitLabel(offer) }}</span>
            <span class="mp-tiers__hint">× {{ formatRate(offer.effectiveRate) }}</span>
          </div>
          <div v-if="videoResolutions(offer).length" class="mp-tiers__grid">
            <div
              v-for="res in videoResolutions(offer)"
              :key="res"
              class="mp-tier"
              :class="{ 'is-empty': !hasVideoPrice(offer, res) }"
            >
              <span class="mp-tier__label">{{ formatRes(res) }}</span>
              <span class="mp-tier__price">{{ formatVideoPrice(offer, res) }}</span>
            </div>
          </div>
          <div v-else class="mp-empty-price">
            {{ t('modelPlaza.detail.noPricing') }}
          </div>
        </div>

        <div v-else-if="card.kind === 'image'" class="mp-tiers">
          <div class="mp-tiers__bar">
            <span class="mp-tiers__unit">{{ t('modelPlaza.table.perUnitImage') }}</span>
            <span class="mp-tiers__hint">× {{ formatRate(offer.effectiveRate) }}</span>
          </div>
          <div v-if="imageTiers(offer).length" class="mp-tiers__grid">
            <div
              v-for="tier in imageTiers(offer)"
              :key="tier"
              class="mp-tier"
              :class="{ 'is-empty': formatImagePrice(offer, tier) === '—' }"
            >
              <span class="mp-tier__label">{{ tier }}</span>
              <span class="mp-tier__price">{{ formatImagePrice(offer, tier) }}</span>
            </div>
          </div>
          <div v-else class="mp-empty-price">
            {{ t('modelPlaza.detail.noPricing') }}
          </div>
        </div>

        <div v-else class="mp-token">
          <div class="mp-token__caption">{{ t('modelPlaza.table.unitPerMillion') }}</div>
          <div class="mp-token__row">
            <div class="mp-token__cell">
              <span class="mp-token__k">{{ t('modelPlaza.table.input') }}</span>
              <span class="mp-token__v">{{ paidToken(offer, 'input_price') }}</span>
            </div>
            <div class="mp-token__cell">
              <span class="mp-token__k">{{ t('modelPlaza.table.output') }}</span>
              <span class="mp-token__v">{{ paidToken(offer, 'output_price') }}</span>
            </div>
            <div class="mp-token__cell">
              <span class="mp-token__k">{{ t('modelPlaza.table.cache') }}</span>
              <span class="mp-token__v">{{ cachePaid(offer) }}</span>
            </div>
          </div>
          <div v-if="card.official_pricing" class="mp-token__official">
            <span>{{ t('modelPlaza.table.officialPrice') }}</span>
            <span>
              {{ officialToken(card.official_pricing.input_price) }}
              /
              {{ officialToken(card.official_pricing.output_price) }}
            </span>
          </div>
          <div
            v-if="offer.model.pricing?.billing_mode && offer.model.pricing.billing_mode !== 'token'"
            class="mp-token__mode"
          >
            {{ nonTokenLabel(offer) }}
          </div>
        </div>

        <p v-if="offer.peakRateEnabled" class="mp-peak">
          {{
            t('modelPlaza.detail.peakNote', {
              window: peakWindow(offer),
              multiplier: formatRate(offer.peakRateMultiplier)
            })
          }}
        </p>
      </section>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatScaled } from '@/utils/pricing'
import type { PlazaModelCard, PlazaOffer } from './plazaCatalog'
import { IMAGE_TIER_KEYS, VIDEO_RESOLUTION_KEYS } from './plazaCatalog'

const props = defineProps<{
  card: PlazaModelCard
}>()

const { t } = useI18n()
const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

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

function isBestOffer(idx: number): boolean {
  if (idx !== 0 || props.card.offers.length < 2) return false
  // Only highlight when the first offer is strictly cheaper on rate.
  return props.card.minRate < props.card.maxRate
}

function formatRate(rate: number): string {
  const s = Number.isInteger(rate)
    ? String(rate)
    : rate.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
  return s + 'x'
}

function formatRes(res: string): string {
  return String(res).toUpperCase()
}

function videoUnitLabel(offer: PlazaOffer): string {
  const unit = offer.model.video_billing_unit || 'per_second'
  return unit === 'per_request'
    ? t('modelPlaza.table.unitPerRequest')
    : t('modelPlaza.table.unitPerSecond')
}

function videoResolutions(offer: PlazaOffer): string[] {
  const listed = (offer.model.video_resolutions || [])
    .map((x) => String(x).toLowerCase())
    .filter(Boolean)
  if (listed.length) {
    return VIDEO_RESOLUTION_KEYS.filter((k) => listed.includes(k)) as string[]
  }
  const prices = offer.model.video_prices
  if (!prices) return []
  return VIDEO_RESOLUTION_KEYS.filter((k) => prices[k] != null) as string[]
}

function hasVideoPrice(offer: PlazaOffer, res: string): boolean {
  const raw = offer.model.video_prices?.[res as keyof typeof offer.model.video_prices]
  return raw != null
}

function formatVideoPrice(offer: PlazaOffer, res: string): string {
  const raw = offer.model.video_prices?.[res as keyof typeof offer.model.video_prices]
  if (raw == null) return '—'
  return formatScaled(Number(raw) * offer.effectiveRate, 1, MIN_DECIMALS)
}

function imageTiers(offer: PlazaOffer): string[] {
  const prices = offer.model.image_prices
  if (!prices) {
    if (offer.model.pricing?.billing_mode === 'image') {
      const labels = (offer.model.pricing.intervals || [])
        .map((i) => i.tier_label)
        .filter(Boolean) as string[]
      if (labels.length) return labels
      return ['—']
    }
    return []
  }
  // Show known tiers even if unpriced so users see the matrix shape.
  const known = IMAGE_TIER_KEYS.filter((k) => k in prices || prices[k] != null)
  if (known.length) return [...known]
  return IMAGE_TIER_KEYS.filter((k) => prices[k] != null)
}

function formatImagePrice(offer: PlazaOffer, tier: string): string {
  const prices = offer.model.image_prices
  if (prices && prices[tier] != null) {
    return formatScaled(Number(prices[tier]) * offer.effectiveRate, 1, MIN_DECIMALS)
  }
  if (offer.model.pricing?.billing_mode === 'image') {
    const hit = (offer.model.pricing.intervals || []).find((i) => i.tier_label === tier)
    const v = hit?.per_request_price ?? offer.model.pricing.per_request_price
    if (v != null) return formatScaled(v * offer.effectiveRate, 1, MIN_DECIMALS)
  }
  return '—'
}

function paidToken(offer: PlazaOffer, field: 'input_price' | 'output_price'): string {
  const v = offer.model.pricing?.[field]
  if (v == null) return '—'
  return formatScaled(v * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
}

function cachePaid(offer: PlazaOffer): string {
  const w = offer.model.pricing?.cache_write_price
  const r = offer.model.pricing?.cache_read_price
  if (w == null && r == null) return '—'
  const ws = w == null ? '—' : formatScaled(w * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  const rs = r == null ? '—' : formatScaled(r * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  return ws + ' / ' + rs
}

function officialToken(v: number | null | undefined): string {
  if (v == null) return '—'
  return formatScaled(v, PER_MILLION, MIN_DECIMALS)
}

function nonTokenLabel(offer: PlazaOffer): string {
  const mode = offer.model.pricing?.billing_mode
  if (mode === 'image') return t('modelPlaza.table.perImage')
  if (mode === 'video' || mode === 'per_request') return t('modelPlaza.table.perRequest')
  return String(mode || '')
}

function peakWindow(offer: PlazaOffer): string {
  const a = offer.peakStart || '?'
  const b = offer.peakEnd || '?'
  return a + '–' + b
}
</script>

<style scoped>
.mp-card {
  --mp-ink: #0f172a;
  --mp-muted: #64748b;
  --mp-faint: #94a3b8;
  --mp-line: #e2e8f0;
  --mp-surface: #ffffff;
  --mp-surface-2: #f8fafc;
  --mp-stage: #0b1220;
  --mp-accent: #0f766e;
  --mp-accent-bright: #2dd4bf;
  --mp-accent-2: #22d3ee;
  --mp-accent-soft: #ccfbf1;
  --mp-shadow: 0 1px 0 rgb(15 23 42 / 0.03), 0 14px 36px -20px rgb(15 23 42 / 0.22);
  border: 1px solid var(--mp-line);
  border-radius: 18px;
  background: var(--mp-surface);
  box-shadow: var(--mp-shadow);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 100%;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.mp-card:hover {
  border-color: color-mix(in srgb, var(--mp-accent) 32%, var(--mp-line));
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.04), 0 20px 44px -20px rgb(15 118 110 / 0.3);
  transform: translateY(-1px);
}

.mp-card[data-kind='video'] {
  --mp-accent: #0e7490;
  --mp-accent-bright: #22d3ee;
  --mp-accent-soft: #cffafe;
}

.mp-card[data-kind='image'] {
  --mp-accent: #0f766e;
  --mp-accent-bright: #2dd4bf;
  --mp-accent-soft: #ccfbf1;
}

.mp-card[data-kind='chat'] {
  --mp-accent: #334155;
  --mp-accent-bright: #94a3b8;
  --mp-accent-soft: #f1f5f9;
}

.mp-card__head {
  position: relative;
  display: grid;
  grid-template-columns: 4px 1fr;
  background:
    radial-gradient(120% 120% at 0% 0%, color-mix(in srgb, var(--mp-accent-soft) 70%, transparent), transparent 55%),
    linear-gradient(180deg, color-mix(in srgb, var(--mp-surface) 92%, #fff), var(--mp-surface));
  border-bottom: 1px solid var(--mp-line);
}

.mp-card__rail {
  background: linear-gradient(180deg, var(--mp-accent-bright), var(--mp-accent) 48%, #0369a1);
}

.mp-card__head-main {
  padding: 15px 16px 13px 14px;
  min-width: 0;
}

.mp-card__title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.mp-card__kind {
  flex-shrink: 0;
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  padding: 4px 8px;
  border-radius: 999px;
  color: var(--mp-accent);
  background: color-mix(in srgb, var(--mp-accent-soft) 85%, white);
  border: 1px solid color-mix(in srgb, var(--mp-accent) 18%, transparent);
}

.mp-card__name {
  margin: 0;
  font-size: 15px;
  font-weight: 750;
  color: var(--mp-ink);
  letter-spacing: -0.015em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mp-card__meta {
  margin-top: 9px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px 8px;
  align-items: center;
  font-size: 12px;
  color: var(--mp-muted);
}

.mp-card__platform {
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 6px;
  background: #f1f5f9;
  color: #475569;
  border: 1px solid #e2e8f0;
}

.mp-card__dot {
  width: 3px;
  height: 3px;
  border-radius: 999px;
  background: #cbd5e1;
}

.mp-card__offers {
  font-weight: 650;
  color: var(--mp-ink);
}

.mp-card__rate-range {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  color: var(--mp-accent);
}

.mp-card__offers-list {
  display: flex;
  flex-direction: column;
}

.mp-offer {
  padding: 14px 16px 15px;
  border-top: 1px solid #f1f5f9;
  background: linear-gradient(180deg, #fff, #fcfdff);
}

.mp-offer:first-child {
  border-top: 0;
}

.mp-offer.is-best {
  background:
    radial-gradient(120% 90% at 100% 0%, color-mix(in srgb, var(--mp-accent-soft) 55%, transparent), transparent 50%),
    linear-gradient(180deg, #fff, #f8fffd);
}

.mp-offer__head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 10px;
}

.mp-offer__group {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  min-width: 0;
}

.mp-offer__name {
  font-size: 13px;
  font-weight: 700;
  color: var(--mp-ink);
}

.mp-offer__desc {
  margin: -2px 0 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--mp-muted);
}

.mp-pill {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 999px;
  letter-spacing: 0.02em;
}

.mp-pill--best {
  color: #0f766e;
  background: #ecfdf5;
  border: 1px solid #99f6e4;
}

.mp-pill--exclusive {
  color: #0f766e;
  background: #ecfdf5;
  border: 1px solid #a7f3d0;
}

.mp-pill--sub {
  color: #0e7490;
  background: #ecfeff;
  border: 1px solid #a5f3fc;
}

.mp-offer__rate {
  text-align: right;
  flex-shrink: 0;
  min-width: 3.5rem;
}

.mp-offer__rate-val {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 3.2rem;
  padding: 4px 8px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: #0f766e;
  background: #f0fdfa;
  border: 1px solid #99f6e4;
}

.mp-offer[data-kind],
.mp-card[data-kind='video'] .mp-offer__rate-val {
  color: #0e7490;
  background: #ecfeff;
  border-color: #a5f3fc;
}

.mp-offer__rate-base {
  display: block;
  margin-top: 4px;
  font-size: 11px;
  color: var(--mp-faint);
  text-decoration: line-through;
}

.mp-tiers__bar {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: baseline;
  margin-bottom: 8px;
}

.mp-tiers__unit {
  font-size: 11px;
  color: var(--mp-faint);
  letter-spacing: 0.01em;
}

.mp-tiers__hint {
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  color: var(--mp-accent);
  opacity: 0.85;
}

.mp-tiers__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(92px, 1fr));
  gap: 8px;
}

.mp-tier {
  position: relative;
  border: 1px solid var(--mp-line);
  border-radius: 12px;
  background:
    linear-gradient(180deg, #fff, var(--mp-surface-2));
  padding: 9px 10px 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.7);
}

.mp-tier.is-empty {
  opacity: 0.72;
  background: #f8fafc;
}

.mp-tier__label {
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  color: var(--mp-muted);
  letter-spacing: 0.06em;
}

.mp-tier__price {
  font-size: 13px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: var(--mp-ink);
  letter-spacing: -0.01em;
}

.mp-empty-price {
  border: 1px dashed #cbd5e1;
  border-radius: 12px;
  padding: 12px;
  text-align: center;
  color: var(--mp-faint);
  font-size: 12px;
  background: #f8fafc;
}

.mp-token__caption {
  margin-bottom: 8px;
  font-size: 11px;
  color: var(--mp-faint);
}

.mp-token__row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.mp-token__cell {
  border: 1px solid var(--mp-line);
  border-radius: 12px;
  background: var(--mp-surface-2);
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.mp-token__k {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--mp-faint);
}

.mp-token__v {
  font-size: 12px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  color: var(--mp-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mp-token__official {
  margin-top: 8px;
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  color: var(--mp-muted);
  font-variant-numeric: tabular-nums;
}

.mp-token__mode {
  margin-top: 6px;
  font-size: 11px;
  color: var(--mp-muted);
}

.mp-peak {
  margin: 10px 0 0;
  font-size: 11px;
  color: #b45309;
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: 10px;
  padding: 7px 9px;
}

:global(.dark) .mp-card {
  --mp-ink: #e2e8f0;
  --mp-muted: #94a3b8;
  --mp-faint: #64748b;
  --mp-line: #1f2937;
  --mp-surface: #0f172a;
  --mp-surface-2: #111827;
  background: #0f172a;
  border-color: #1f2937;
  box-shadow: none;
}

:global(.dark) .mp-card__head {
  background:
    radial-gradient(120% 120% at 0% 0%, color-mix(in srgb, var(--mp-accent) 18%, transparent), transparent 55%),
    linear-gradient(180deg, #111c2e, #0f172a);
  border-bottom-color: #1f2937;
}

:global(.dark) .mp-card__platform {
  background: #1e293b;
  color: #cbd5e1;
  border-color: #334155;
}

:global(.dark) .mp-offer {
  border-top-color: #1e293b;
  background: #0f172a;
}

:global(.dark) .mp-offer.is-best {
  background:
    radial-gradient(120% 90% at 100% 0%, rgba(15, 118, 110, 0.18), transparent 50%),
    #0f172a;
}

:global(.dark) .mp-tier,
:global(.dark) .mp-token__cell {
  border-color: #1e293b;
  background: #111827;
  box-shadow: none;
}

:global(.dark) .mp-offer__rate-val {
  color: #5eead4;
  background: rgba(15, 118, 110, 0.2);
  border-color: rgba(45, 212, 191, 0.25);
}

:global(.dark) .mp-pill--best,
:global(.dark) .mp-pill--exclusive {
  color: #5eead4;
  background: rgba(15, 118, 110, 0.2);
  border-color: rgba(45, 212, 191, 0.25);
}

:global(.dark) .mp-pill--sub {
  color: #67e8f9;
  background: rgba(14, 116, 144, 0.2);
  border-color: rgba(34, 211, 238, 0.25);
}

:global(.dark) .mp-empty-price {
  background: #0b1220;
  border-color: #334155;
}

:global(.dark) .mp-peak {
  background: rgba(180, 83, 9, 0.12);
  border-color: rgba(251, 191, 36, 0.25);
  color: #fbbf24;
}

@media (max-width: 640px) {
  .mp-token__row {
    grid-template-columns: 1fr;
  }
}
</style>
