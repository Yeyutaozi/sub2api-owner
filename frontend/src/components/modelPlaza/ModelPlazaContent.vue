<template>
  <div class="mp-stage">
    <header v-if="!embedded" class="mp-hero">
      <div class="mp-hero__rail" aria-hidden="true" />
      <div class="mp-hero__body">
        <div class="mp-hero__brand">
          <div class="mp-mark" aria-hidden="true">
            <span class="mp-mark__frame" />
            <span class="mp-mark__beam" />
          </div>
          <div class="min-w-0">
            <p class="mp-hero__kicker">MODEL CATALOG</p>
            <h1 class="mp-hero__title">{{ t('modelPlaza.title') }}</h1>
            <p class="mp-hero__sub">{{ t('modelPlaza.description') }}</p>
          </div>
        </div>

        <button
          v-if="showRefresh"
          type="button"
          class="mp-refresh"
          :disabled="refreshing || loading"
          @click="$emit('refresh')"
        >
          <svg
            class="mp-refresh__icon"
            :class="{ 'is-spin': refreshing || loading }"
            viewBox="0 0 24 24"
            width="16"
            height="16"
            aria-hidden="true"
          >
            <path
              fill="currentColor"
              d="M17.65 6.35A7.95 7.95 0 0 0 12 4a8 8 0 1 0 7.75 10h-2.1A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35Z"
            />
          </svg>
          {{ isAdminView ? t('modelPlaza.admin.syncAll') : t('modelPlaza.admin.refresh') }}
        </button>
      </div>

      <div v-if="stats" class="mp-stats">
        <div class="mp-stat mp-stat--models">
          <span class="mp-stat__n">{{ stats.models }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.models') }}</span>
        </div>
        <div class="mp-stat mp-stat--groups">
          <span class="mp-stat__n">{{ stats.groups }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.groups') }}</span>
        </div>
        <div class="mp-stat mp-stat--offers">
          <span class="mp-stat__n">{{ stats.offers }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.offers') }}</span>
        </div>
        <div v-if="syncedLabel" class="mp-stat mp-stat--wide">
          <span class="mp-stat__n mp-stat__n--sm">{{ syncedLabel }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.syncedAt') }}</span>
        </div>
        <div v-if="isAdminView" class="mp-stat mp-stat--admin">
          <span class="mp-stat__n mp-stat__n--sm">ADMIN</span>
          <span class="mp-stat__l">{{ t('modelPlaza.admin.fullCatalog') }}</span>
        </div>
      </div>
    </header>

    <div v-else class="mp-embedded-bar">
      <div class="mp-embedded-bar__left">
        <div class="mp-mark mp-mark--sm" aria-hidden="true">
          <span class="mp-mark__frame" />
          <span class="mp-mark__beam" />
        </div>
        <div>
          <p class="mp-hero__kicker">MODEL CATALOG</p>
          <h2 class="mp-embedded-title">{{ t('modelPlaza.title') }}</h2>
        </div>
      </div>
      <button
        v-if="showRefresh"
        type="button"
        class="mp-refresh"
        :disabled="refreshing || loading"
        @click="$emit('refresh')"
      >
        <svg
          class="mp-refresh__icon"
          :class="{ 'is-spin': refreshing || loading }"
          viewBox="0 0 24 24"
          width="16"
          height="16"
          aria-hidden="true"
        >
          <path
            fill="currentColor"
            d="M17.65 6.35A7.95 7.95 0 0 0 12 4a8 8 0 1 0 7.75 10h-2.1A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35Z"
          />
        </svg>
        {{ isAdminView ? t('modelPlaza.admin.syncAll') : t('modelPlaza.admin.refresh') }}
      </button>
    </div>

    <div v-if="descriptionHtml" class="mp-description" v-html="descriptionHtml"></div>

    <p v-if="!isAuthenticated" class="mp-anon">
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <div v-if="loading" class="mp-skeleton-grid" aria-busy="true">
      <div v-for="n in 6" :key="'sk-' + n" class="mp-skeleton-card">
        <div class="mp-skeleton-line mp-skeleton-line--lg" />
        <div class="mp-skeleton-line" />
        <div class="mp-skeleton-block" />
      </div>
    </div>

    <div v-else-if="error" class="mp-error">
      {{ t('modelPlaza.loadFailed') }}
    </div>

    <template v-else>
      <div class="mp-shelf">
      <aside class="mp-filters" aria-label="catalog filters">
        <div class="mp-filter-row">
          <span class="mp-filter-label">{{ t('modelPlaza.filters.kindLabel') }}</span>
          <div class="mp-chip-track">
            <button
              v-for="k in kindOptions"
              :key="'kind-' + k"
              type="button"
              class="mp-chip mp-chip--kind"
              :class="{ 'is-active': selectedKind === k }"
              :data-kind="k"
              @click="selectedKind = k"
            >
              {{ kindChipLabel(k) }}
            </button>
          </div>
        </div>

        <div class="mp-filter-row">
          <span class="mp-filter-label">{{ t('modelPlaza.filters.platformLabel') }}</span>
          <div class="mp-chip-track">
            <button
              v-for="p in ['all', ...platforms]"
              :key="'platform-' + p"
              type="button"
              class="mp-chip mp-chip--platform"
              :class="{ 'is-active': selectedPlatform === p }"
              :data-platform="p"
              @click="selectedPlatform = p"
            >
              <PlazaVendorMark
                v-if="p !== 'all'"
                :platform="p"
                size="sm"
              />
              <span>{{ p === 'all' ? t('modelPlaza.filters.all') : platformLabel(p) }}</span>
            </button>
          </div>
        </div>

        <div class="mp-filter-row">
          <span class="mp-filter-label">{{ t('modelPlaza.filters.groupLabel') }}</span>
          <div class="mp-chip-track">
            <button
              type="button"
              class="mp-chip"
              :class="{ 'is-active': selectedGroupId === 'all' }"
              @click="selectedGroupId = 'all'"
            >
              {{ t('modelPlaza.filters.all') }}
            </button>
            <button
              v-for="g in groupOptions"
              :key="'group-' + g.id"
              type="button"
              class="mp-chip"
              :class="{ 'is-active': String(selectedGroupId) === String(g.id) }"
              @click="selectedGroupId = g.id"
            >
              {{ g.name }}
            </button>
          </div>
        </div>

        <div class="mp-filter-row mp-filter-row--search">
          <span class="mp-filter-label">{{ t('modelPlaza.filters.modelLabel') }}</span>
          <input
            v-model="searchQuery"
            type="search"
            class="mp-search"
            :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          />
        </div>
      </aside>

      <div class="mp-shelf__main">
        <div class="mp-shelf__toolbar">
          <span class="mp-shelf__count data-mono">{{ filteredCards.length }} MODELS</span>
          <span class="mp-shelf__hint">模型下挂分组比价 · 首字仅供参考</span>
        </div>
        <div v-if="filteredCards.length === 0" class="mp-empty">
          {{ cards.length === 0 ? t('modelPlaza.empty') : t('modelPlaza.noSearchResult') }}
        </div>
        <div v-else class="mp-vendor-shelves">
          <section
            v-for="shelf in vendorShelves"
            :key="shelf.id"
            class="mp-vendor-shelf"
            :style="{ '--vendor-color': shelf.color, '--vendor-soft': shelf.soft }"
          >
            <header class="mp-vendor-shelf__head">
              <PlazaVendorMark :platform="shelf.samplePlatform" :model-name="shelf.sampleModel" size="lg" />
              <div class="mp-vendor-shelf__meta">
                <h3 class="mp-vendor-shelf__title">{{ shelf.label }}</h3>
                <p class="mp-vendor-shelf__sub">
                  <span class="data-mono">{{ shelf.cards.length }}</span> 款模型 · 模型下挂分组
                </p>
              </div>
              <div class="mp-vendor-shelf__kinds">
                <span
                  v-for="k in shelf.kinds"
                  :key="shelf.id + '-' + k"
                  class="mp-vendor-shelf__kind"
                  :data-kind="k"
                >{{ kindChipLabel(k) }}</span>
              </div>
            </header>
            <div class="mp-grid">
              <PlazaModelCard v-for="card in shelf.cards" :key="card.key" :card="card" />
            </div>
          </section>
        </div>
      </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'
import PlazaModelCard from './PlazaModelCard.vue'
import PlazaVendorMark from './PlazaVendorMark.vue'
import { buildPlazaModelCards, type PlazaModelCard as PlazaCard } from './plazaCatalog'
import { resolvePlazaVendor, vendorLabel } from './plazaVendors'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading?: boolean
  error?: boolean
  refreshing?: boolean
  embedded?: boolean
}>()

defineEmits<{
  refresh: []
}>()

const { t, locale } = useI18n()
const authStore = useAuthStore()

const selectedKind = ref<string>('all')
const selectedPlatform = ref<string>('all')
const selectedGroupId = ref<string | number>('all')
const searchQuery = ref('')

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdminView = computed(() => !!props.response?.is_admin_view)
const showRefresh = computed(() => isAuthenticated.value)

const cards = computed(() => buildPlazaModelCards(props.response))

const stats = computed(() => {
  if (!props.response?.stats) return null
  // Avoid zero flash while first payload is still empty after failed loads.
  if (props.loading && !props.response.groups?.length) return null
  return props.response.stats
})

const syncedLabel = computed(() => {
  const raw = props.response?.synced_at
  if (!raw) return ''
  try {
    return new Date(raw).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
      hour12: false
    })
  } catch {
    return raw
  }
})

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  const html = marked.parse(md, { async: false }) as string
  return DOMPurify.sanitize(html)
})

const platforms = computed(() => {
  // Vendor ids (not raw platform strings) so merged cards filter correctly.
  const set = new Set<string>()
  for (const c of cards.value) {
    const id = c.vendorId || resolvePlazaVendor(c.platform, c.name).id
    set.add(id)
  }
  return [...set].sort((a, b) => vendorLabel(a).localeCompare(vendorLabel(b)))
})

const groupOptions = computed(() => {
  const map = new Map<number, string>()
  for (const g of props.response?.groups || []) {
    map.set(g.id, g.name)
  }
  return [...map.entries()]
    .map(([id, name]) => ({ id, name }))
    .sort((a, b) => a.name.localeCompare(b.name))
})

const kindOptions = ['all', 'video', 'image', 'chat'] as const

function kindChipLabel(k: string): string {
  if (k === 'all') return t('modelPlaza.filters.all')
  if (k === 'video') return t('modelPlaza.kind.video')
  if (k === 'image') return t('modelPlaza.kind.image')
  return t('modelPlaza.kind.chat')
}

function platformLabel(p: string): string {
  // Filter values are vendor ids; vendorLabel also accepts platform strings.
  return vendorLabel(p)
}

const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return cards.value.filter((card: PlazaCard) => {
    if (selectedKind.value !== 'all' && card.kind !== selectedKind.value) return false
    if (
      selectedPlatform.value !== 'all' &&
      (card.vendorId || resolvePlazaVendor(card.platform, card.name).id) !== selectedPlatform.value
    )
      return false
    if (selectedGroupId.value !== 'all') {
      const gid = String(selectedGroupId.value)
      if (!card.offers.some((o) => String(o.groupId) === gid)) return false
    }
    if (q) {
      const hay = [
        card.name,
        card.platform,
        card.kind,
        ...card.offers.map((o) => o.groupName),
        ...card.offers.map((o) => o.description)
      ]
        .join(' ')
        .toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  })
})

const vendorShelves = computed(() => {
  type Shelf = {
    id: string
    label: string
    color: string
    soft: string
    samplePlatform: string
    sampleModel: string
    kinds: string[]
    cards: PlazaCard[]
  }
  const map = new Map<string, Shelf>()
  for (const card of filteredCards.value) {
    const v = resolvePlazaVendor(card.vendorId || card.platform, card.name)
    let shelf = map.get(v.id)
    if (!shelf) {
      shelf = {
        id: v.id,
        label: v.label,
        color: v.color,
        soft: v.soft,
        samplePlatform: card.platform,
        sampleModel: card.name,
        kinds: [],
        cards: []
      }
      map.set(v.id, shelf)
    }
    shelf.cards.push(card)
    if (!shelf.kinds.includes(String(card.kind))) shelf.kinds.push(String(card.kind))
  }
  const kindOrder = (k: string) => (k === 'video' ? 0 : k === 'image' ? 1 : 2)
  return [...map.values()]
    .map((shelf) => {
      shelf.kinds.sort((a, b) => kindOrder(a) - kindOrder(b))
      return shelf
    })
    .sort((a, b) => {
      const rank = (id: string) =>
        ({ openai: 0, anthropic: 1, google: 2, xai: 3, minimax: 4, seedance: 5, deepseek: 6 } as Record<string, number>)[id] ?? 50
      const d = rank(a.id) - rank(b.id)
      if (d !== 0) return d
      return a.label.localeCompare(b.label)
    })
})

// Reset filters when catalog identity changes drastically.
watch(
  () => props.response?.synced_at,
  () => {
    // keep user filters; only clear invalid group filter
    if (
      selectedGroupId.value !== 'all' &&
      !groupOptions.value.some((g) => g.id === selectedGroupId.value)
    ) {
      selectedGroupId.value = 'all'
    }
  }
)
</script>

<style scoped>
.mp-root { display:flex; flex-direction:column; gap:16px; min-width:0; }
.mp-hero { position:relative; overflow:hidden; border-radius:22px; border:1px solid rgba(67,56,202,.22); background:radial-gradient(90% 120% at 100% 0%, rgba(79,70,229,.35), transparent 48%), radial-gradient(70% 100% at 0% 100%, rgba(217,119,6,.18), transparent 50%), linear-gradient(135deg,#0b1020 0%,#121a2e 46%,#132033 100%); color:#e8eefc; padding:22px 22px 18px; box-shadow:0 22px 48px -28px rgba(8,12,24,.55); }
.mp-hero::before { content:""; position:absolute; inset:0; background-image:linear-gradient(rgba(255,255,255,.035) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.035) 1px, transparent 1px); background-size:24px 24px; mask-image:linear-gradient(180deg,#000,transparent 85%); pointer-events:none; }
.mp-hero__body { position:relative; display:flex; align-items:flex-start; justify-content:space-between; gap:18px; flex-wrap:wrap; }
.mp-hero__copy { min-width:0; max-width:640px; }
.mp-hero__kicker { margin:0 0 6px; font-family:"JetBrains Mono",ui-monospace,monospace; font-size:11px; font-weight:700; letter-spacing:.18em; text-transform:uppercase; color:#a5b4fc; }
.mp-hero__title { margin:0; font-family:"Space Grotesk",Sora,sans-serif; font-size:clamp(1.5rem,2.4vw,2rem); font-weight:700; letter-spacing:-.04em; color:#f8fafc; }
.mp-hero__sub { margin:8px 0 0; font-size:14px; line-height:1.55; color:rgba(226,232,240,.82); }
.mp-hero__stats { display:grid; grid-template-columns:repeat(3,minmax(84px,1fr)); gap:8px; min-width:min(100%,320px); }
.mp-stat { border-radius:14px; border:1px solid rgba(255,255,255,.12); background:rgba(255,255,255,.06); padding:10px 12px; backdrop-filter:blur(8px); }
.mp-stat__n { display:block; font-family:"JetBrains Mono",ui-monospace,monospace; font-size:20px; font-weight:800; letter-spacing:-.03em; color:#fff; }
.mp-stat__n--sm { font-size:13px; letter-spacing:0; }
.mp-stat__l { display:block; margin-top:2px; font-size:11px; font-weight:700; letter-spacing:.04em; text-transform:uppercase; color:rgba(203,213,225,.78); }
.mp-stat--groups { border-color:rgba(16,185,129,.28); }
.mp-stat--offers { border-color:rgba(245,158,11,.28); }
.mp-stat--admin { border-color:rgba(244,63,94,.28); }
.mp-embedded-bar { display:flex; align-items:center; justify-content:space-between; gap:14px; padding:16px 18px; border-radius:18px; border:1px solid rgba(99,102,241,.28); background:radial-gradient(90% 140% at 100% 0%, rgba(79,70,229,.32), transparent 48%), radial-gradient(70% 120% at 0% 100%, rgba(217,119,6,.16), transparent 50%), linear-gradient(135deg,#0b1220 0%,#141d33 48%,#121a2c 100%); box-shadow:0 18px 40px -28px rgba(8,12,24,.55); color:#e8eefc; }
.mp-embedded-bar__left { display:flex; align-items:center; gap:12px; min-width:0; }
.mp-embedded-title { margin:0; font-family:"Space Grotesk",Sora,sans-serif; font-size:1.28rem; font-weight:700; letter-spacing:-.03em; color:#f8fafc !important; }
.mp-embedded-bar .mp-hero__kicker { color:#a5b4fc !important; }
.mp-mark { position:relative; width:42px; height:42px; border-radius:14px; background:linear-gradient(145deg,rgba(255,255,255,.12),rgba(255,255,255,.04)); border:1px solid rgba(255,255,255,.14); flex:0 0 auto; }
.mp-mark--sm { width:36px; height:36px; border-radius:12px; }
.mp-mark__frame { position:absolute; inset:8px; border:1.5px solid rgba(165,180,252,.65); border-radius:8px; }
.mp-mark__beam { position:absolute; left:10px; right:10px; top:50%; height:2px; background:linear-gradient(90deg,transparent,#a5b4fc,#fbbf24,transparent); transform:translateY(-50%); }
.mp-refresh { appearance:none; display:inline-flex; align-items:center; gap:8px; border-radius:12px; border:1px solid rgba(165,180,252,.35); background:linear-gradient(180deg,rgba(79,70,229,.22),rgba(255,255,255,.04)); color:#bfdbfe !important; padding:9px 14px; font-size:13px; font-weight:700; cursor:pointer; transition:transform .15s ease,border-color .15s ease; }
.mp-refresh:hover:not(:disabled) { transform:translateY(-1px); border-color:rgba(165,180,252,.55); }
.mp-refresh:disabled { opacity:.55; cursor:not-allowed; }
.mp-refresh__icon.is-spin { animation:mp-spin .9s linear infinite; }
@keyframes mp-spin { to { transform:rotate(360deg); } }
.mp-description { border-radius:14px; border:1px solid #d7dde8; background:linear-gradient(180deg,#fff,#f5f7fb); padding:12px 14px; font-size:13px; color:#5c6678; line-height:1.55; }
.mp-anon { margin:0; border-radius:12px; border:1px dashed #bfdbfe; background:#eef2ff; color:#1e3a8a; padding:10px 12px; font-size:13px; font-weight:600; }
.mp-shelf { display:grid; grid-template-columns:280px minmax(0,1fr); gap:14px; align-items:start; }
.mp-filters { position:sticky; top:84px; border-radius:18px; border:1px solid rgba(67,56,202,.12); background:linear-gradient(180deg,#fff 0%,#f4f6fc 100%); box-shadow:0 1px 0 rgba(255,255,255,.85) inset, 0 18px 36px -28px rgba(18,21,28,.4); padding:14px; display:flex; flex-direction:column; gap:14px; }
.mp-filter-row { display:flex; flex-direction:column; gap:8px; }
.mp-filter-label { font-family:"JetBrains Mono",ui-monospace,monospace; font-size:10px; font-weight:800; letter-spacing:.14em; text-transform:uppercase; color:#1d4ed8; }
.mp-chip-track { display:flex; flex-wrap:wrap; gap:6px; }
.mp-chip { appearance:none; border:1px solid #d7dde8; background:#fff; color:#5c6678; border-radius:10px; padding:7px 11px; font-size:12px; font-weight:700; cursor:pointer; transition:all .15s ease; }
.mp-chip:hover { border-color:#a5b4fc; color:#1e3a8a; }
.mp-chip.is-active { color:#fff; border-color:transparent; background:linear-gradient(180deg,#3b82f6,#1d4ed8); box-shadow:0 10px 18px -12px rgba(67,56,202,.75); }
.mp-chip--kind[data-kind='image'].is-active { background:linear-gradient(180deg,#60a5fa,#2563eb); }
.mp-chip--kind[data-kind='video'].is-active { background:linear-gradient(180deg,#fb923c,#ea580c); }
.mp-chip--kind[data-kind='chat'].is-active, .mp-chip--kind[data-kind='text'].is-active { background:linear-gradient(180deg,#a78bfa,#7c3aed); }
.mp-chip--platform { display:inline-flex; align-items:center; gap:6px; }
.mp-search { width:100%; border:1px solid #d7dde8; border-radius:12px; background:#fff; padding:10px 12px; font-size:13px; color:#0b1220; outline:none; transition:border-color .15s ease, box-shadow .15s ease; }
.mp-search:focus { border-color:#3b82f6; box-shadow:0 0 0 3px rgba(99,102,241,.16); }
.mp-shelf__main { min-width:0; }
.mp-shelf__toolbar { display:flex; align-items:baseline; justify-content:space-between; gap:12px; margin-bottom:12px; padding:11px 14px; border-radius:14px; border:1px solid rgba(67,56,202,.12); background:linear-gradient(180deg,#fff,#f3f5fc); }
.mp-shelf__count { font-family:"JetBrains Mono",ui-monospace,monospace; font-size:12px; font-weight:700; color:#1d4ed8; }
.mp-shelf__hint { font-size:12px; color:#8b95a7; }
.mp-vendor-shelves { display:flex; flex-direction:column; gap:18px; }
.mp-vendor-shelf {
  position:relative;
  overflow:hidden;
  border-radius:20px;
  border:1px solid color-mix(in srgb, var(--vendor-color, #1d4ed8) 18%, #d5dce8);
  background:
    radial-gradient(90% 120% at 0% 0%, color-mix(in srgb, var(--vendor-soft, #dbeafe) 70%, #fff), transparent 55%),
    linear-gradient(180deg, #ffffff 0%, #f4f7fc 100%);
  box-shadow: 0 18px 36px -28px rgba(15, 23, 42, 0.45);
  padding:14px;
}
.mp-vendor-shelf__head {
  display:flex;
  align-items:center;
  gap:12px;
  margin-bottom:12px;
  padding:4px 4px 12px;
  border-bottom:1px dashed color-mix(in srgb, var(--vendor-color, #1d4ed8) 22%, #d7dee8);
}
.mp-vendor-shelf__meta { min-width:0; flex:1; }
.mp-vendor-shelf__title {
  margin:0;
  font-family: var(--fc-display), Outfit, system-ui, sans-serif;
  font-size:17px;
  font-weight:750;
  letter-spacing:-0.03em;
  color:#0f172a;
}
.mp-vendor-shelf__sub {
  margin:3px 0 0;
  font-size:12px;
  color:#7b879a;
}
.mp-vendor-shelf__kinds { display:flex; flex-wrap:wrap; gap:6px; justify-content:flex-end; }
.mp-vendor-shelf__kind {
  display:inline-flex;
  align-items:center;
  height:22px;
  padding:0 8px;
  border-radius:999px;
  font-size:10px;
  font-weight:800;
  letter-spacing:.08em;
  text-transform:uppercase;
  border:1px solid #dbe2ee;
  background:#fff;
  color:#475569;
}
.mp-vendor-shelf__kind[data-kind='video'] { color:#9a3412; background:#ffedd5; border-color:#fed7aa; }
.mp-vendor-shelf__kind[data-kind='image'] { color:#1d4ed8; background:#dbeafe; border-color:#bfdbfe; }
.mp-vendor-shelf__kind[data-kind='chat'],
.mp-vendor-shelf__kind[data-kind='text'] { color:#0f766e; background:#ccfbf1; border-color:#99f6e4; }
.mp-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(420px,1fr)); gap:14px; }
.mp-grid :deep(.mp-card--multi) { grid-column: 1 / -1; }
.mp-skeleton-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(420px,1fr)); gap:14px; }
:global(.dark) .mp-vendor-shelf { background:linear-gradient(180deg,#111827,#0b1220); border-color:#243041; }
:global(.dark) .mp-vendor-shelf__title { color:#e2e8f0; }
:global(.dark) .mp-vendor-shelf__sub { color:#94a3b8; }
:global(.dark) .mp-vendor-shelf__kind { background:#0f172a; border-color:#334155; color:#cbd5e1; }
.mp-skeleton-card { border-radius:18px; border:1px solid #d7dde8; background:#fff; padding:16px; min-height:220px; }
.mp-skeleton-line { height:12px; border-radius:6px; background:linear-gradient(90deg,#eef1f7,#f8fafc,#eef1f7); background-size:200% 100%; animation:mp-shimmer 1.2s ease-in-out infinite; margin-bottom:10px; }
.mp-skeleton-line--lg { height:18px; width:55%; }
.mp-skeleton-block { height:88px; border-radius:12px; background:linear-gradient(90deg,#eef1f7,#f8fafc,#eef1f7); background-size:200% 100%; animation:mp-shimmer 1.2s ease-in-out infinite; }
@keyframes mp-shimmer { 0% { background-position:100% 0; } 100% { background-position:-100% 0; } }
.mp-error { border-radius:14px; border:1px solid #fecdd3; background:#fff1f2; color:#be123c; padding:14px 16px; font-weight:650; }
.mp-empty { border-radius:16px; border:1px dashed #c3cbd8; background:#f8fafc; padding:32px 18px; text-align:center; color:#8b95a7; font-size:13px; }
.mp-results-head { display:flex; align-items:center; justify-content:space-between; gap:10px; margin-bottom:12px; }
.mp-results-count { font-size:12px; font-weight:700; color:#5c6678; }
.mp-filter-row--search { margin-top:2px; }
.mp-stat--wide { grid-column:span 1; }
:global(.dark) .mp-filters, :global(.dark) .mp-description, :global(.dark) .mp-skeleton-card { background:#111827; border-color:#243041; color:#cbd5e1; }
:global(.dark) .mp-chip, :global(.dark) .mp-search { background:#0f172a; border-color:#334155; color:#cbd5e1; }
:global(.dark) .mp-shelf__toolbar { background:#111827; border-color:#243041; }
:global(.dark) .mp-shelf__count { color:#a5b4fc; }
:global(.dark) .mp-shelf__hint { color:#94a3b8; }
:global(.dark) .mp-empty { background:#0f172a; border-color:#334155; color:#94a3b8; }
:global(.dark) .mp-filter-label { color:#a5b4fc; }
@media (max-width:1024px) { .mp-shelf { grid-template-columns:1fr; } .mp-filters { position:static; } }
@media (max-width:640px) { .mp-grid, .mp-skeleton-grid { grid-template-columns:1fr; } .mp-hero__body { flex-direction:column; } .mp-hero__stats { width:100%; grid-template-columns:repeat(2,1fr); } }
@media (prefers-reduced-motion:reduce) { .mp-refresh:hover:not(:disabled) { transform:none; } .mp-refresh__icon.is-spin, .mp-skeleton-line, .mp-skeleton-block { animation:none; } }

.mp-stage { display:flex; flex-direction:column; gap:16px; }
.mp-hero__rail { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:12px; margin-bottom:14px; }
.mp-hero__brand { display:flex; align-items:center; gap:12px; min-width:0; }
.mp-stats { display:grid; grid-template-columns:repeat(3,minmax(84px,1fr)); gap:8px; }
.mp-stat--models { border-color:rgba(99,102,241,.28); }

</style>

