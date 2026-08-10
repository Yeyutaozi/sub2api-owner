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
          <span class="mp-card__chip">
            {{ t('modelPlaza.card.offers', { n: card.offerCount }) }}
          </span>
          <span class="mp-card__chip mp-card__chip--rate">
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
        data-testid="plaza-offer-panel"
      >
        <div class="mp-offer__top">
          <div class="mp-offer__identity">
            <div class="mp-offer__name-row">
              <span class="mp-offer__index" aria-hidden="true">{{ String(idx + 1).padStart(2, '0') }}</span>
              <h4 class="mp-offer__name" :title="offer.groupName">{{ offer.groupName }}</h4>
            </div>
            <div class="mp-offer__pills">
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
          </div>

          <div class="mp-offer__metrics">
            <div class="mp-metric mp-metric--rate" title="effective group rate">
              <span class="mp-metric__label">{{ t('modelPlaza.table.rate') }}</span>
              <span class="mp-metric__value">{{ formatRate(offer.effectiveRate) }}</span>
              <span
                v-if="
                  offer.userRateMultiplier != null &&
                  offer.userRateMultiplier !== offer.rateMultiplier
                "
                class="mp-metric__sub"
              >
                {{ t('modelPlaza.card.baseRate', { rate: formatRate(offer.rateMultiplier) }) }}
              </span>
            </div>
            <div
              class="mp-metric mp-metric--ttft"
              :title="offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer')"
              data-testid="plaza-offer-ttft"
            >
              <span class="mp-metric__label">
                <svg class="mp-metric__bolt" viewBox="0 0 16 16" width="11" height="11" aria-hidden="true">
                  <path
                    fill="currentColor"
                    d="M9.2 1.1 3.4 8.4c-.2.3 0 .7.4.7h3.1l-.8 5.5c-.1.4.5.6.7.3l5.8-7.3c.2-.3 0-.7-.4-.7H8.9l1-5.5c.1-.4-.5-.6-.7-.3Z"
                  />
                </svg>
                {{ t('modelPlaza.detail.avgFirstToken') }}
              </span>
              <span class="mp-metric__value">{{ formatFirstToken(offer.avgFirstTokenMs) }}</span>
              <span class="mp-metric__sub">
                {{ offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer') }}
              </span>
            </div>
          </div>
        </div>

        <p v-if="offer.description" class="mp-offer__desc" :title="offer.description">
          {{ offer.description }}
        </p>

        <!-- Video pricing matrix -->
        <div v-if="card.kind === 'video'" class="mp-price-block">
          <div class="mp-price-block__bar">
            <span class="mp-price-block__unit">{{ videoUnitLabel(offer) }}</span>
          </div>
          <div v-if="videoResolutions(offer).length" class="mp-price-grid">
            <div
              v-for="res in videoResolutions(offer)"
              :key="res"
              class="mp-price-cell"
              :class="{ 'is-empty': !hasVideoPrice(offer, res) }"
            >
              <span class="mp-price-cell__k">{{ formatRes(res) }}</span>
              <span class="mp-price-cell__v">{{ formatVideoPrice(offer, res) }}</span>
            </div>
          </div>
          <div v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</div>
        </div>

        <!-- Image pricing matrix -->
        <div v-else-if="card.kind === 'image'" class="mp-price-block">
          <div class="mp-price-block__bar">
            <span class="mp-price-block__unit">{{ t('modelPlaza.table.perUnitImage') }}</span>
          </div>
          <div v-if="imageTiers(offer).length" class="mp-price-grid">
            <div
              v-for="tier in imageTiers(offer)"
              :key="tier"
              class="mp-price-cell"
              :class="{ 'is-empty': formatImagePrice(offer, tier) === '—' }"
            >
              <span class="mp-price-cell__k">{{ tier }}</span>
              <span class="mp-price-cell__v">{{ formatImagePrice(offer, tier) }}</span>
            </div>
          </div>
          <div v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</div>
        </div>

        <!-- Token pricing -->
        <div v-else class="mp-price-block">
          <div class="mp-price-block__bar">
            <span class="mp-price-block__unit">{{ t('modelPlaza.table.unitPerMillion') }}</span>
          </div>
          <div class="mp-price-grid mp-price-grid--token">
            <div class="mp-price-cell">
              <span class="mp-price-cell__k">{{ t('modelPlaza.table.input') }}</span>
              <span class="mp-price-cell__v">{{ paidToken(offer, 'input_price') }}</span>
            </div>
            <div class="mp-price-cell">
              <span class="mp-price-cell__k">{{ t('modelPlaza.table.output') }}</span>
              <span class="mp-price-cell__v">{{ paidToken(offer, 'output_price') }}</span>
            </div>
            <div class="mp-price-cell">
              <span class="mp-price-cell__k">{{ t('modelPlaza.table.cache') }}</span>
              <span class="mp-price-cell__v">{{ cachePaid(offer) }}</span>
            </div>
          </div>
          <div v-if="card.official_pricing" class="mp-official">
            <span class="mp-official__k">{{ t('modelPlaza.table.officialPrice') }}</span>
            <span class="mp-official__v">
              {{ officialToken(card.official_pricing.input_price) }}
              /
              {{ officialToken(card.official_pricing.output_price) }}
            </span>
          </div>
          <div
            v-if="offer.model.pricing?.billing_mode && offer.model.pricing.billing_mode !== 'token'"
            class="mp-mode-note"
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
  if (!Number.isFinite(n) || n <= 0) return '—'
  if (n >= 1000) return (n / 1000).toFixed(1) + 's'
  return Math.round(n) + 'ms'
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
  --mp-panel: #f1f5f9;
  --mp-accent: #0f766e;
  --mp-accent-bright: #14b8a6;
  --mp-accent-soft: #ccfbf1;
  --mp-shadow: 0 1px 0 rgb(15 23 42 / 0.04), 0 18px 40px -24px rgb(15 23 42 / 0.28);
  border: 1px solid var(--mp-line);
  border-radius: 20px;
  background: var(--mp-surface);
  box-shadow: var(--mp-shadow);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 100%;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.mp-card:hover {
  border-color: color-mix(in srgb, var(--mp-accent) 28%, var(--mp-line));
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.04), 0 22px 48px -22px rgb(15 118 110 / 0.28);
  transform: translateY(-1px);
}

.mp-card[data-kind='video'] {
  --mp-accent: #0e7490;
  --mp-accent-bright: #06b6d4;
  --mp-accent-soft: #cffafe;
}

.mp-card[data-kind='image'] {
  --mp-accent: #0f766e;
  --mp-accent-bright: #14b8a6;
  --mp-accent-soft: #ccfbf1;
}

.mp-card[data-kind='chat'] {
  --mp-accent: #475569;
  --mp-accent-bright: #64748b;
  --mp-accent-soft: #f1f5f9;
}

.mp-card__head {
  position: relative;
  display: grid;
  grid-template-columns: 4px 1fr;
  background:
    radial-gradient(120% 120% at 0% 0%, color-mix(in srgb, var(--mp-accent-soft) 75%, transparent), transparent 58%),
    linear-gradient(180deg, #fff, var(--mp-surface));
  border-bottom: 1px solid var(--mp-line);
}

.mp-card__rail {
  background: linear-gradient(180deg, var(--mp-accent-bright), var(--mp-accent));
}

.mp-card__head-main {
  padding: 14px 16px 12px 14px;
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
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--mp-accent);
  background: color-mix(in srgb, var(--mp-accent-soft) 80%, #fff);
  border: 1px solid color-mix(in srgb, var(--mp-accent) 18%, var(--mp-line));
  border-radius: 999px;
  padding: 3px 8px;
}

.mp-card__name {
  margin: 0;
  min-width: 0;
  font-size: 15px;
  font-weight: 750;
  letter-spacing: -0.02em;
  color: var(--mp-ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mp-card__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}

.mp-card__platform,
.mp-card__chip {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 600;
  color: var(--mp-muted);
  background: var(--mp-surface-2);
  border: 1px solid var(--mp-line);
  border-radius: 999px;
  padding: 2px 8px;
  line-height: 1.4;
}

.mp-card__chip--rate {
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  color: var(--mp-accent);
  background: color-mix(in srgb, var(--mp-accent-soft) 55%, #fff);
  border-color: color-mix(in srgb, var(--mp-accent) 18%, var(--mp-line));
}

.mp-card__offers-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--mp-panel) 55%, #fff), var(--mp-surface-2));
  flex: 1;
}

.mp-offer {
  position: relative;
  border: 1px solid color-mix(in srgb, var(--mp-line) 88%, #94a3b8);
  border-radius: 16px;
  background: var(--mp-surface);
  box-shadow:
    0 1px 0 rgb(255 255 255 / 0.8) inset,
    0 8px 20px -16px rgb(15 23 42 / 0.28);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  isolation: isolate;
}

.mp-offer::before {
  content: '';
  position: absolute;
  left: 0;
  top: 12px;
  bottom: 12px;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: color-mix(in srgb, var(--mp-accent) 55%, #cbd5e1);
  opacity: 0.75;
}

.mp-offer.is-best {
  border-color: color-mix(in srgb, var(--mp-accent) 35%, var(--mp-line));
  background:
    radial-gradient(120% 100% at 100% 0%, color-mix(in srgb, var(--mp-accent-soft) 70%, transparent), transparent 55%),
    var(--mp-surface);
  box-shadow:
    0 1px 0 rgb(255 255 255 / 0.85) inset,
    0 12px 28px -18px color-mix(in srgb, var(--mp-accent) 45%, transparent);
}

.mp-offer.is-best::before {
  background: linear-gradient(180deg, var(--mp-accent-bright), var(--mp-accent));
  opacity: 1;
}

.mp-offer__top {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-left: 6px;
}

.mp-offer__identity {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.mp-offer__name-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.mp-offer__index {
  flex-shrink: 0;
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--mp-faint);
  background: var(--mp-surface-2);
  border: 1px solid var(--mp-line);
  border-radius: 8px;
  padding: 2px 6px;
}

.mp-offer__name {
  margin: 0;
  min-width: 0;
  font-size: 13.5px;
  font-weight: 750;
  letter-spacing: -0.01em;
  color: var(--mp-ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mp-offer__pills {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.mp-pill {
  display: inline-flex;
  align-items: center;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.02em;
  border-radius: 999px;
  padding: 2px 7px;
  border: 1px solid transparent;
  white-space: nowrap;
}

.mp-pill--best,
.mp-pill--exclusive {
  color: #0f766e;
  background: #f0fdfa;
  border-color: #99f6e4;
}

.mp-pill--sub {
  color: #0e7490;
  background: #ecfeff;
  border-color: #a5f3fc;
}

.mp-offer__metrics {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
  gap: 8px;
}

.mp-metric {
  border: 1px solid var(--mp-line);
  border-radius: 12px;
  background: var(--mp-surface-2);
  padding: 8px 10px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mp-metric--rate {
  background: color-mix(in srgb, var(--mp-accent-soft) 42%, #fff);
  border-color: color-mix(in srgb, var(--mp-accent) 18%, var(--mp-line));
}

.mp-metric--ttft {
  background: #fff;
}

.mp-metric__label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--mp-faint);
}

.mp-metric__bolt {
  color: var(--mp-accent);
}

.mp-metric__value {
  font-size: 15px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
  color: var(--mp-ink);
  line-height: 1.2;
}

.mp-metric--rate .mp-metric__value {
  color: var(--mp-accent);
}

.mp-metric__sub {
  font-size: 10px;
  color: var(--mp-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mp-offer__desc {
  margin: 0;
  padding-left: 6px;
  font-size: 11.5px;
  line-height: 1.45;
  color: var(--mp-muted);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.mp-price-block {
  padding-left: 6px;
}

.mp-price-block__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.mp-price-block__unit {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--mp-faint);
  letter-spacing: 0.01em;
}

.mp-price-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 6px;
}

.mp-price-grid--token {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.mp-price-cell {
  border: 1px solid var(--mp-line);
  border-radius: 11px;
  background: linear-gradient(180deg, #fff, var(--mp-surface-2));
  padding: 8px 9px 7px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.7);
}

.mp-price-cell.is-empty {
  opacity: 0.68;
  background: #f8fafc;
}

.mp-price-cell__k {
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.05em;
  color: var(--mp-muted);
}

.mp-price-cell__v {
  font-size: 12.5px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  color: var(--mp-ink);
  letter-spacing: -0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.mp-official {
  margin-top: 8px;
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  color: var(--mp-muted);
  font-variant-numeric: tabular-nums;
}

.mp-mode-note {
  margin-top: 6px;
  font-size: 11px;
  color: var(--mp-muted);
}

.mp-peak {
  margin: 0;
  margin-left: 6px;
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
  --mp-line: #243041;
  --mp-surface: #0f172a;
  --mp-surface-2: #111827;
  --mp-panel: #0b1220;
  background: #0f172a;
  border-color: #1f2937;
  box-shadow: none;
}

:global(.dark) .mp-card__head {
  background:
    radial-gradient(120% 120% at 0% 0%, color-mix(in srgb, var(--mp-accent) 16%, transparent), transparent 55%),
    linear-gradient(180deg, #111c2e, #0f172a);
  border-bottom-color: #1f2937;
}

:global(.dark) .mp-card__platform,
:global(.dark) .mp-card__chip,
:global(.dark) .mp-offer__index {
  background: #1e293b;
  color: #cbd5e1;
  border-color: #334155;
}

:global(.dark) .mp-card__chip--rate {
  color: #5eead4;
  background: rgba(15, 118, 110, 0.18);
  border-color: rgba(45, 212, 191, 0.22);
}

:global(.dark) .mp-card__offers-list {
  background: linear-gradient(180deg, #0b1220, #0f172a);
}

:global(.dark) .mp-offer {
  background: #111827;
  border-color: #243041;
  box-shadow: none;
}

:global(.dark) .mp-offer.is-best {
  background:
    radial-gradient(120% 90% at 100% 0%, rgba(15, 118, 110, 0.18), transparent 50%),
    #111827;
  border-color: rgba(45, 212, 191, 0.28);
}

:global(.dark) .mp-metric,
:global(.dark) .mp-price-cell {
  border-color: #243041;
  background: #0f172a;
  box-shadow: none;
}

:global(.dark) .mp-metric--rate {
  background: rgba(15, 118, 110, 0.14);
  border-color: rgba(45, 212, 191, 0.22);
}

:global(.dark) .mp-metric--rate .mp-metric__value,
:global(.dark) .mp-pill--best,
:global(.dark) .mp-pill--exclusive {
  color: #5eead4;
}

:global(.dark) .mp-pill--best,
:global(.dark) .mp-pill--exclusive {
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
  .mp-offer__metrics {
    grid-template-columns: 1fr;
  }

  .mp-price-grid--token {
    grid-template-columns: 1fr;
  }
}
</style>
