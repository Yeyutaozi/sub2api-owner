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
                  {{ formatRate(card.minRate) }} – {{ formatRate(card.maxRate) }}
                </template>
                <template v-else>{{ formatRate(card.minRate) }}</template>
              </span>
            </div>
          </div>
        </div>
      </div>
    </header>

    <div class="mp-card__groups" data-testid="plaza-model-groups">
      <div class="mp-card__groups-bar">
        <span class="mp-card__groups-title">{{ t('modelPlaza.card.groupMountTitle') }}</span>
        <span class="mp-card__groups-hint">{{ t('modelPlaza.card.groupMountHint') }}</span>
      </div>
      <div class="mp-card__groups-list">
        <section
          v-for="(offer, idx) in card.offers"
          :key="offer.groupId"
          class="mp-group"
          :class="{ 'is-best': isBestOffer(idx), 'is-solo': card.offerCount === 1 }"
          :style="groupTicketStyle(idx)"
          data-testid="plaza-offer-panel"
        >
          <div class="mp-group__rail" aria-hidden="true" />
          <div class="mp-group__body">
            <div class="mp-group__top">
              <div class="mp-group__identity">
                <div class="mp-group__name-row">
                  <span class="mp-group__index" aria-hidden="true">{{ String(idx + 1).padStart(2, '0') }}</span>
                  <div class="mp-group__name-wrap">
                    <span class="mp-group__kicker">{{ t('modelPlaza.card.groupLabel') }}</span>
                    <h4 class="mp-group__name" :title="offer.groupName">{{ offer.groupName }}</h4>
                  </div>
                </div>
                <div class="mp-group__pills">
                  <span v-if="isBestOffer(idx)" class="mp-pill mp-pill--best">{{ t('modelPlaza.card.bestOffer') }}</span>
                  <span v-if="offer.isExclusive" class="mp-pill mp-pill--exclusive">{{ t('modelPlaza.badges.exclusive') }}</span>
                  <span v-if="offer.subscriptionType === 'subscription'" class="mp-pill mp-pill--sub">{{ t('modelPlaza.badges.subscription') }}</span>
                </div>
              </div>
              <div class="mp-group__metrics">
                <div class="mp-metric mp-metric--rate" title="effective group rate">
                  <span class="mp-metric__label">{{ t('modelPlaza.table.rate') }}</span>
                  <span class="mp-metric__value">{{ formatRate(offer.effectiveRate) }}</span>
                  <span
                    v-if="offer.userRateMultiplier != null && offer.userRateMultiplier !== offer.rateMultiplier"
                    class="mp-metric__sub"
                  >{{ t('modelPlaza.card.baseRate', { rate: formatRate(offer.rateMultiplier) }) }}</span>
                </div>
                <div
                  class="mp-metric mp-metric--ttft"
                  :title="offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer')"
                  data-testid="plaza-offer-ttft"
                >
                  <span class="mp-metric__label">
                    <svg class="mp-metric__bolt" viewBox="0 0 16 16" width="11" height="11" aria-hidden="true">
                      <path fill="currentColor" d="M9.2 1.1 3.4 8.4c-.2.3 0 .7.4.7h3.1l-.8 5.5c-.1.4.5.6.7.3l5.8-7.3c.2-.3 0-.7-.4-.7H8.9l1-5.5c.1-.4-.5-.6-.7-.3Z" />
                    </svg>
                    {{ t('modelPlaza.detail.avgFirstToken') }}
                  </span>
                  <span class="mp-metric__value">{{ formatFirstToken(offer.avgFirstTokenMs) }}</span>
                  <span class="mp-metric__sub">{{ offer.ttftDisclaimer || t('modelPlaza.detail.ttftDisclaimer') }}</span>
                </div>
              </div>
            </div>
            <p v-if="offer.description" class="mp-group__desc" :title="offer.description">{{ offer.description }}</p>

            <div v-if="card.kind === 'video'" class="mp-price-block">
              <div class="mp-price-block__bar">
                <span class="mp-price-block__mode">{{ videoModeLabel(offer) }}</span>
                <span class="mp-price-block__unit">{{ videoUnitLabel(offer) }}</span>
              </div>
              <p class="mp-price-block__hint">{{ videoHint(offer) }} · {{ t('modelPlaza.table.rateAppliedNote') }}</p>
              <div v-if="videoResolutions(offer).length" class="mp-price-grid">
                <div
                  v-for="res in videoResolutions(offer)"
                  :key="offer.groupId + '-v-' + res"
                  class="mp-price-cell"
                  :class="{ 'is-empty': !hasVideoPrice(offer, res) }"
                >
                  <span class="mp-price-cell__k">{{ formatRes(res) }}</span>
                  <span class="mp-price-cell__v">{{ formatVideoPrice(offer, res) }}</span>
                </div>
              </div>
              <p v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</p>
            </div>

            <div v-else-if="card.kind === 'image'" class="mp-price-block">
              <div class="mp-price-block__bar">
                <span class="mp-price-block__mode">{{ t('modelPlaza.table.perImage') }}</span>
                <span class="mp-price-block__unit">{{ t('modelPlaza.table.perUnitImage') }}</span>
              </div>
              <p class="mp-price-block__hint">{{ t('modelPlaza.table.imageBillingHint') }} · {{ t('modelPlaza.table.rateAppliedNote') }}</p>
              <div v-if="hasImageConfiguredPrice(offer)" class="mp-price-grid">
                <div
                  v-for="tier in imageTiers(offer)"
                  :key="offer.groupId + '-i-' + tier"
                  class="mp-price-cell"
                  :class="{ 'is-empty': formatImagePrice(offer, tier) === emDash }"
                >
                  <span class="mp-price-cell__k">{{ formatImageTier(tier) }}</span>
                  <span class="mp-price-cell__v">{{ formatImagePrice(offer, tier) }}</span>
                </div>
              </div>
              <p v-else class="mp-empty-price">{{ t('modelPlaza.detail.noPricing') }}</p>
            </div>

            <div v-else class="mp-token-grid">
              <div class="mp-token-cell">
                <div class="mp-token-cell__k">{{ t('modelPlaza.table.input') }}</div>
                <div class="mp-token-cell__v">{{ paidToken(offer, 'input_price') }}</div>
              </div>
              <div class="mp-token-cell">
                <div class="mp-token-cell__k">{{ t('modelPlaza.table.output') }}</div>
                <div class="mp-token-cell__v">{{ paidToken(offer, 'output_price') }}</div>
              </div>
              <div class="mp-token-cell">
                <div class="mp-token-cell__k">{{ t('modelPlaza.table.cache') }}</div>
                <div class="mp-token-cell__v">{{ cachePaid(offer) }}</div>
              </div>
            </div>

            <div v-if="offer.peakRateEnabled" class="mp-group__foot">
              <span class="mp-peak">{{ t('modelPlaza.detail.peakNote', { window: peakWindow(offer), multiplier: offer.peakRateMultiplier }) }}</span>
            </div>
            <p
              v-if="offer.model.pricing?.billing_mode && card.kind === 'chat' && offer.model.pricing.billing_mode !== 'token'"
              class="mp-mode-note"
            >{{ nonTokenLabel(offer) }}</p>
          </div>
        </section>
      </div>
    </div>

        <footer v-if="card.official_pricing && (card.kind === 'chat' || card.kind === 'text')" class="mp-card__official">
      <span class="mp-official__k">{{ t('modelPlaza.table.officialPrice') }}</span>
      <span class="mp-official__v">
        {{ t('modelPlaza.table.input') }} {{ officialToken(card.official_pricing.input_price) }} · {{ t('modelPlaza.table.output') }} {{ officialToken(card.official_pricing.output_price) }}
        <span class="mp-official__unit">{{ t('modelPlaza.table.unitPerMillion') }}</span>
      </span>
    </footer>
  </article>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaModelCard, PlazaOffer } from './plazaCatalog'
import { IMAGE_TIER_KEYS, VIDEO_RESOLUTION_KEYS } from './plazaCatalog'
import PlazaVendorMark from './PlazaVendorMark.vue'
import { resolvePlazaVendor } from './plazaVendors'

const props = defineProps<{
  card: PlazaModelCard
}>()

const { t } = useI18n()
const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2
const emDash = "—"

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

function videoUnitLabel(offer: PlazaOffer): string {
  const unit = String(offer.model.video_billing_unit || '').toLowerCase()
  if (unit === 'second' || unit === 'per_second' || unit === 'sec') {
    return t('modelPlaza.table.unitPerSecond')
  }
  return t('modelPlaza.table.unitPerRequest')
}


function videoModeLabel(offer: PlazaOffer): string {
  const unit = String(offer.model.video_billing_unit || '').toLowerCase()
  if (unit === 'second' || unit === 'per_second' || unit === 'sec') {
    return t('modelPlaza.table.perSecond')
  }
  return t('modelPlaza.table.perRequest')
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
  // avoid showing a placeholder-only grid
  return tiers.some((tier) => formatImagePrice(offer, tier) !== emDash)
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
  const prices = offer.model.video_prices
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
  const prices = offer.model.image_prices
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
  return emDash
}

function paidToken(offer: PlazaOffer, field: 'input_price' | 'output_price'): string {
  const v = offer.model.pricing?.[field]
  if (v == null) return emDash
  return formatScaled(v * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
}

function cachePaid(offer: PlazaOffer): string {
  const w = offer.model.pricing?.cache_write_price
  const r = offer.model.pricing?.cache_read_price
  if (w == null && r == null) return emDash
  const ws = w == null ? emDash : formatScaled(w * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  const rs = r == null ? emDash : formatScaled(r * offer.effectiveRate, PER_MILLION, MIN_DECIMALS)
  return ws + ' / ' + rs
}

function officialToken(v: number | null | undefined): string {
  if (v == null) return emDash
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
  return a + "–" + b
}
</script>
<style scoped>
.mp-card{--mp-ink:#0f172a;--mp-muted:#64748b;--mp-faint:#94a3b8;--mp-line:#e2e8f0;position:relative;display:flex;flex-direction:column;border-radius:18px;border:1px solid color-mix(in srgb,var(--vendor-color,#0f766e) 22%,var(--mp-line));background:linear-gradient(180deg,#fff 0%,#f8fafc 100%);box-shadow:0 1px 0 rgba(15,23,42,.04),0 10px 28px -18px rgba(15,23,42,.28);overflow:hidden;min-width:0}
.mp-card--multi{box-shadow:0 1px 0 rgba(15,23,42,.05),0 16px 36px -20px rgba(15,23,42,.32)}
.mp-card__head{display:grid;grid-template-columns:6px 1fr;background:radial-gradient(120% 140% at 0% 0%,color-mix(in srgb,var(--vendor-soft,#ccfbf1) 85%,#fff),transparent 60%),linear-gradient(180deg,#fff,#f8fafc);border-bottom:1px solid var(--mp-line)}
.mp-card__rail{background:linear-gradient(180deg,var(--vendor-color,#0f766e),color-mix(in srgb,var(--vendor-color,#0f766e) 45%,#fff))}
.mp-card__head-main{padding:14px 16px 12px;min-width:0}
.mp-card__title-row{display:flex;align-items:flex-start;gap:12px;min-width:0}
.mp-card__title-text{min-width:0;flex:1}
.mp-card__name-line{display:flex;flex-wrap:wrap;align-items:center;gap:8px;min-width:0}
.mp-card__kind{display:inline-flex;align-items:center;height:22px;padding:0 8px;border-radius:999px;font-size:11px;font-weight:800;letter-spacing:.04em;border:1px solid transparent}
.mp-card__kind[data-kind='video']{color:#9a3412;background:#ffedd5;border-color:#fed7aa}
.mp-card__kind[data-kind='image']{color:#1d4ed8;background:#dbeafe;border-color:#bfdbfe}
.mp-card__kind[data-kind='chat'],.mp-card__kind[data-kind='text']{color:#0f766e;background:#ccfbf1;border-color:#99f6e4}
.mp-card__name{margin:0;font-size:16px;font-weight:800;letter-spacing:-.02em;color:var(--mp-ink);line-height:1.25;word-break:break-word}
.mp-card__meta{display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin-top:8px}
.mp-card__platform{display:inline-flex;align-items:center;gap:6px;font-size:12px;font-weight:650;color:var(--mp-muted)}
.mp-card__chip{display:inline-flex;align-items:center;height:22px;padding:0 8px;border-radius:7px;font-size:11px;font-weight:700;color:var(--mp-muted);background:#f1f5f9;border:1px solid var(--mp-line)}
.mp-card__chip--groups{color:#1d4ed8;background:#eff6ff;border-color:#bfdbfe}
.mp-card__chip--rate{font-family:"JetBrains Mono",ui-monospace,monospace;color:#047857;background:#ecfdf5;border-color:#a7f3d0}
.mp-card__groups{display:flex;flex-direction:column;background:linear-gradient(180deg,#f1f5f9,#f8fafc 40%,#fff);padding:0 0 12px}
.mp-card__groups-bar{display:flex;flex-wrap:wrap;align-items:baseline;justify-content:space-between;gap:6px 12px;padding:10px 16px 8px}
.mp-card__groups-title{font-size:11px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;color:#334155}
.mp-card__groups-hint{font-size:11px;color:var(--mp-faint)}
.mp-card__groups-list{display:flex;flex-direction:column;gap:10px;padding:0 12px}
.mp-group{position:relative;display:grid;grid-template-columns:5px 1fr;border-radius:14px;border:1px solid color-mix(in srgb,var(--group-color,#0f766e) 28%,var(--mp-line));background:linear-gradient(135deg,color-mix(in srgb,var(--group-soft,#ccfbf1) 55%,#fff),#fff 55%);box-shadow:0 1px 0 rgba(15,23,42,.03);overflow:hidden}
.mp-group.is-best{border-color:color-mix(in srgb,#059669 45%,var(--mp-line));box-shadow:0 0 0 1px rgba(5,150,105,.12),0 8px 20px -14px rgba(5,150,105,.45)}
.mp-group__rail{background:linear-gradient(180deg,var(--group-color,#0f766e),color-mix(in srgb,var(--group-color,#0f766e) 50%,#fff))}
.mp-group__body{padding:12px 12px 12px 14px;min-width:0}
.mp-group__top{display:grid;grid-template-columns:minmax(0,1.2fr) minmax(0,1fr);gap:12px;align-items:start}
.mp-group__identity{min-width:0}
.mp-group__name-row{display:flex;align-items:flex-start;gap:10px;min-width:0}
.mp-group__index{flex:0 0 auto;display:inline-flex;align-items:center;justify-content:center;min-width:28px;height:28px;border-radius:8px;font-family:"JetBrains Mono",ui-monospace,monospace;font-size:11px;font-weight:800;color:var(--group-color,#0f766e);background:color-mix(in srgb,var(--group-soft,#ccfbf1) 80%,#fff);border:1px solid color-mix(in srgb,var(--group-color,#0f766e) 22%,#fff)}
.mp-group__name-wrap{min-width:0}
.mp-group__kicker{display:block;font-size:10px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;color:var(--mp-faint);margin-bottom:2px}
.mp-group__name{margin:0;font-size:14px;font-weight:800;color:var(--mp-ink);line-height:1.3;word-break:break-word}
.mp-group__pills{display:flex;flex-wrap:wrap;gap:6px;margin-top:8px}
.mp-pill{display:inline-flex;align-items:center;height:20px;padding:0 7px;border-radius:999px;font-size:10px;font-weight:800;border:1px solid transparent}
.mp-pill--best{color:#047857;background:#d1fae5;border-color:#a7f3d0}
.mp-pill--exclusive{color:#1d4ed8;background:#dbeafe;border-color:#bfdbfe}
.mp-pill--sub{color:#7c3aed;background:#ede9fe;border-color:#ddd6fe}
.mp-group__metrics{display:grid;grid-template-columns:1fr 1fr;gap:8px;min-width:0}
.mp-metric{border-radius:11px;border:1px solid var(--mp-line);background:rgba(255,255,255,.92);padding:8px 10px;min-width:0}
.mp-metric__label{display:inline-flex;align-items:center;gap:4px;font-size:10px;font-weight:800;letter-spacing:.04em;text-transform:uppercase;color:var(--mp-faint)}
.mp-metric__value{display:block;margin-top:4px;font-family:"JetBrains Mono",ui-monospace,monospace;font-size:18px;font-weight:800;letter-spacing:-.03em;color:var(--mp-ink);font-variant-numeric:tabular-nums;line-height:1.1}
.mp-metric--rate .mp-metric__value{color:#047857}
.mp-metric--ttft .mp-metric__value{color:#c2410c}
.mp-metric__sub{display:block;margin-top:3px;font-size:10px;line-height:1.35;color:var(--mp-faint)}
.mp-metric__bolt{flex:0 0 auto}
.mp-group__desc{margin:10px 0 0;font-size:12px;line-height:1.45;color:var(--mp-muted);display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.mp-price-block{margin-top:10px;border-radius:12px;border:1px solid var(--mp-line);overflow:hidden;background:#f8fafc}
.mp-price-block__bar{display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;padding:8px 10px;background:linear-gradient(90deg,color-mix(in srgb,var(--group-soft,#eef2ff) 70%,#fff),#f8fafc);border-bottom:1px solid var(--mp-line)}
.mp-price-block__mode{display:inline-flex;align-items:center;height:22px;padding:0 8px;border-radius:999px;font-size:11px;font-weight:800;letter-spacing:.02em;color:color-mix(in srgb,var(--group-color,#4338ca) 90%,#0f172a);background:color-mix(in srgb,var(--group-soft,#eef2ff) 80%,#fff);border:1px solid color-mix(in srgb,var(--group-color,#4338ca) 22%,#e2e8f0)}
.mp-price-block__unit{font-size:11px;font-weight:700;letter-spacing:.01em;color:#475569;text-transform:none;font-family:inherit}
.mp-price-block__hint{margin:0;padding:6px 10px 0;font-size:11px;line-height:1.45;color:#64748b}
.mp-official__unit{display:inline-block;margin-left:6px;font-size:10px;font-weight:600;color:#94a3b8}
.mp-price-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(88px,1fr));gap:1px;background:var(--mp-line)}
.mp-price-cell{background:#fff;padding:9px 10px;min-width:0}
.mp-price-cell__k{display:block;font-size:10px;font-weight:800;letter-spacing:.06em;text-transform:uppercase;color:var(--mp-faint)}
.mp-price-cell__v{display:block;margin-top:4px;font-family:"JetBrains Mono",ui-monospace,monospace;font-size:13px;font-weight:750;color:var(--mp-ink);font-variant-numeric:tabular-nums}
.mp-token-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-top:10px}
.mp-token-cell{border-radius:10px;border:1px solid var(--mp-line);background:#fff;padding:8px 9px;min-width:0}
.mp-token-cell__k{font-size:10px;font-weight:800;letter-spacing:.05em;text-transform:uppercase;color:var(--mp-faint)}
.mp-token-cell__v{margin-top:3px;font-family:"JetBrains Mono",ui-monospace,monospace;font-size:12px;font-weight:750;color:var(--mp-ink);word-break:break-word}
.mp-group__foot{display:flex;flex-wrap:wrap;gap:6px;margin-top:10px}
.mp-peak{display:inline-flex;align-items:center;height:22px;padding:0 8px;border-radius:6px;font-size:11px;font-weight:700;color:#9a3412;background:#ffedd5;border:1px solid rgba(234,88,12,.2)}
.mp-empty-price,.mp-group .is-empty .mp-price-cell__v{color:var(--mp-faint);font-size:12px}
.mp-empty-price{padding:8px 10px}
.mp-mode-note{margin:8px 0 0;font-size:11px;color:var(--mp-muted);line-height:1.4}
.mp-card__official{display:flex;flex-wrap:wrap;gap:8px 12px;align-items:center;padding:10px 16px 12px;border-top:1px dashed var(--mp-line);background:#f8fafc}
.mp-official__k{font-size:10px;font-weight:800;letter-spacing:.05em;text-transform:uppercase;color:var(--mp-faint)}
.mp-official__v{font-family:"JetBrains Mono",ui-monospace,monospace;font-size:12px;font-weight:700;color:var(--mp-ink)}
@media (max-width:720px){.mp-group__top{grid-template-columns:1fr}.mp-token-grid{grid-template-columns:1fr}}
:global(.dark) .mp-card{background:#111827;border-color:#243041;box-shadow:none}
:global(.dark) .mp-card__head{background:linear-gradient(180deg,#0f172a,#111827);border-bottom-color:#243041}
:global(.dark) .mp-card__groups{background:linear-gradient(180deg,#0b1220,#111827)}
:global(.dark) .mp-group{background:#0b1220;border-color:#243041}
:global(.dark) .mp-group__name,:global(.dark) .mp-card__name,:global(.dark) .mp-metric__value,:global(.dark) .mp-price-cell__v,:global(.dark) .mp-token-cell__v,:global(.dark) .mp-official__v{color:#e5eef8}
:global(.dark) .mp-metric,:global(.dark) .mp-price-cell,:global(.dark) .mp-token-cell,:global(.dark) .mp-price-block,:global(.dark) .mp-card__official{background:#0f172a;border-color:#243041}
:global(.dark) .mp-price-grid{background:#243041}
:global(.dark) .mp-price-block__bar{background:linear-gradient(90deg,rgba(15,118,110,.18),#0f172a);border-bottom-color:#243041}
:global(.dark) .mp-card__chip{background:#0f172a;border-color:#243041;color:#94a3b8}
</style>
