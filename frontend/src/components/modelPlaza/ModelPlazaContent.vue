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
        <div class="mp-stat">
          <span class="mp-stat__n">{{ stats.models }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.models') }}</span>
        </div>
        <div class="mp-stat">
          <span class="mp-stat__n">{{ stats.groups }}</span>
          <span class="mp-stat__l">{{ t('modelPlaza.stats.groups') }}</span>
        </div>
        <div class="mp-stat">
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
      <div class="mp-filters">
        <div class="mp-filter-row">
          <span class="mp-filter-label">{{ t('modelPlaza.filters.kindLabel') }}</span>
          <div class="mp-chip-track">
            <button
              v-for="k in kindOptions"
              :key="'kind-' + k"
              type="button"
              class="mp-chip"
              :class="{ 'is-active': selectedKind === k }"
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
              class="mp-chip"
              :class="{ 'is-active': selectedPlatform === p }"
              @click="selectedPlatform = p"
            >
              {{ p === 'all' ? t('modelPlaza.filters.all') : p }}
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
              :class="{ 'is-active': selectedGroupId === g.id }"
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
      </div>

      <div v-if="filteredCards.length === 0" class="mp-empty">
        {{ cards.length === 0 ? t('modelPlaza.empty') : t('modelPlaza.noSearchResult') }}
      </div>
      <div v-else class="mp-grid">
        <PlazaModelCard v-for="card in filteredCards" :key="card.key" :card="card" />
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
import { buildPlazaModelCards, type PlazaModelCard as PlazaCard } from './plazaCatalog'

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
  const set = new Set<string>()
  for (const c of cards.value) set.add(c.platform)
  return [...set].sort()
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

const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return cards.value.filter((card: PlazaCard) => {
    if (selectedKind.value !== 'all' && card.kind !== selectedKind.value) return false
    if (selectedPlatform.value !== 'all' && card.platform !== selectedPlatform.value) return false
    if (selectedGroupId.value !== 'all') {
      const gid = Number(selectedGroupId.value)
      if (!card.offers.some((o) => o.groupId === gid)) return false
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
.mp-stage {
  --mp-ink: #0f172a;
  --mp-muted: #64748b;
  --mp-faint: #94a3b8;
  --mp-line: #e2e8f0;
  --mp-paper: transparent;
  --mp-surface: #ffffff;
  --mp-accent: #0f766e;
  --mp-accent-bright: #2dd4bf;
  --mp-accent-2: #22d3ee;
  color: var(--mp-ink);
  font-family:
    "Segoe UI Variable Text",
    "Segoe UI",
    "PingFang SC",
    "Hiragino Sans GB",
    "Microsoft YaHei UI",
    "Microsoft YaHei",
    system-ui,
    sans-serif;
}

.mp-hero {
  position: relative;
  margin-bottom: 16px;
  border: 1px solid var(--mp-line);
  border-radius: 22px;
  overflow: hidden;
  background:
    radial-gradient(120% 90% at 0% 0%, rgba(45, 212, 191, 0.12), transparent 52%),
    radial-gradient(90% 80% at 100% 0%, rgba(14, 116, 144, 0.08), transparent 48%),
    linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03), 0 18px 40px -24px rgb(15 23 42 / 0.28);
}

.mp-hero__rail {
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
  background: linear-gradient(180deg, #2dd4bf, #0f766e 45%, #0369a1);
}

.mp-hero__body {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 18px 18px 8px 20px;
}

.mp-hero__brand {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  min-width: 0;
}

.mp-mark {
  position: relative;
  width: 2.75rem;
  height: 2.75rem;
  flex: 0 0 auto;
  border-radius: 0.95rem;
  background:
    radial-gradient(circle at 30% 25%, rgb(45 212 191 / 0.35), transparent 48%),
    linear-gradient(160deg, #132033 0%, #0a101b 55%, #071018 100%);
  box-shadow:
    inset 0 0 0 1px rgb(255 255 255 / 0.1),
    0 14px 28px -16px rgb(15 118 110 / 0.75);
}

.mp-mark--sm {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.8rem;
}

.mp-mark__frame {
  position: absolute;
  inset: 0.42rem;
  border: 1.5px solid rgb(34 211 238 / 0.55);
  border-radius: 0.35rem;
}

.mp-mark__frame::before,
.mp-mark__frame::after {
  content: "";
  position: absolute;
  width: 0.42rem;
  height: 0.42rem;
  border: 1.5px solid #2dd4bf;
}

.mp-mark__frame::before {
  top: -1px;
  left: -1px;
  border-right: 0;
  border-bottom: 0;
}

.mp-mark__frame::after {
  right: -1px;
  bottom: -1px;
  border-left: 0;
  border-top: 0;
}

.mp-mark__beam {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 58%;
  height: 2px;
  transform: translate(-50%, -50%) rotate(-18deg);
  background: linear-gradient(90deg, transparent, #2dd4bf, transparent);
  box-shadow: 0 0 12px rgb(34 211 238 / 0.55);
}

.mp-hero__kicker {
  margin: 0 0 0.2rem;
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: #0891b2;
}

.mp-hero__title {
  margin: 0;
  font-size: clamp(1.35rem, 2vw, 1.7rem);
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--mp-ink);
}

.mp-hero__sub {
  margin: 6px 0 0;
  max-width: 46rem;
  font-size: 13px;
  line-height: 1.55;
  color: var(--mp-muted);
}

.mp-refresh {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  border: 1px solid #99f6e4;
  background: linear-gradient(180deg, #f0fdfa, #fff);
  color: #0f766e;
  border-radius: 999px;
  padding: 9px 14px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  box-shadow: 0 8px 18px -14px rgb(15 118 110 / 0.8);
  transition: transform 0.15s ease, box-shadow 0.15s ease, opacity 0.15s ease;
}

.mp-refresh:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 12px 22px -14px rgb(15 118 110 / 0.9);
}

.mp-refresh:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.mp-refresh__icon.is-spin {
  animation: mp-spin 0.8s linear infinite;
}

@keyframes mp-spin {
  to {
    transform: rotate(360deg);
  }
}

.mp-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(118px, 1fr));
  gap: 10px;
  padding: 8px 16px 16px 20px;
}

.mp-stat {
  border: 1px solid var(--mp-line);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.82);
  padding: 12px 14px;
  backdrop-filter: blur(6px);
}

.mp-stat--admin {
  border-color: color-mix(in srgb, var(--mp-accent) 35%, var(--mp-line));
  background: linear-gradient(180deg, #f0fdfa 0%, #fff 100%);
}

.mp-stat__n {
  display: block;
  font-size: 22px;
  font-weight: 800;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums;
  color: var(--mp-ink);
}

.mp-stat__n--sm {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.mp-stat__l {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  font-weight: 700;
  color: var(--mp-faint);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.mp-embedded-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border: 1px solid var(--mp-line);
  border-radius: 16px;
  background:
    radial-gradient(100% 120% at 0% 0%, rgba(45, 212, 191, 0.1), transparent 50%),
    #fff;
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
}

.mp-embedded-bar__left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.mp-embedded-title {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.mp-description {
  margin-bottom: 16px;
  border: 1px solid var(--mp-line);
  border-radius: 16px;
  background: var(--mp-surface);
  padding: 14px 16px;
  font-size: 14px;
  line-height: 1.7;
  color: #334155;
}

.mp-description :deep(p) {
  margin: 0 0 0.5em;
}

.mp-description :deep(p:last-child) {
  margin-bottom: 0;
}

.mp-anon {
  margin: 0 0 14px;
  font-size: 12px;
  color: var(--mp-faint);
}

.mp-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.mp-skeleton-card {
  border: 1px solid var(--mp-line);
  border-radius: 18px;
  background: #fff;
  padding: 16px;
  min-height: 180px;
}

.mp-skeleton-line,
.mp-skeleton-block {
  border-radius: 8px;
  background: linear-gradient(90deg, #f1f5f9 25%, #e2e8f0 37%, #f1f5f9 63%);
  background-size: 400% 100%;
  animation: mp-shimmer 1.2s ease infinite;
}

.mp-skeleton-line {
  height: 12px;
  margin-bottom: 10px;
  width: 70%;
}

.mp-skeleton-line--lg {
  height: 18px;
  width: 48%;
}

.mp-skeleton-block {
  margin-top: 18px;
  height: 84px;
  width: 100%;
}

@keyframes mp-shimmer {
  0% {
    background-position: 100% 0;
  }
  100% {
    background-position: 0 0;
  }
}

.mp-error {
  border: 1px solid #fecaca;
  background: #fef2f2;
  color: #b91c1c;
  border-radius: 16px;
  padding: 24px;
  text-align: center;
  font-size: 14px;
}

.mp-filters {
  border: 1px solid var(--mp-line);
  border-radius: 18px;
  background:
    linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  padding: 14px 14px 10px;
  margin-bottom: 16px;
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
}

.mp-filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: flex-start;
  margin-bottom: 10px;
}

.mp-filter-label {
  width: 48px;
  flex-shrink: 0;
  padding-top: 8px;
  font-family: ui-monospace, "Cascadia Mono", Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--mp-faint);
}

.mp-chip-track {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.mp-chip {
  border: 1px solid var(--mp-line);
  background: #fff;
  color: #475569;
  border-radius: 999px;
  padding: 7px 12px;
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mp-chip:hover {
  border-color: color-mix(in srgb, var(--mp-accent) 40%, var(--mp-line));
  color: var(--mp-accent);
}

.mp-chip.is-active {
  background: linear-gradient(180deg, #0f766e, #0d9488);
  border-color: #0f766e;
  color: white;
  box-shadow: 0 8px 16px -10px rgba(15, 118, 110, 0.85);
}

.mp-search {
  flex: 1;
  min-width: 180px;
  border: 1px solid var(--mp-line);
  border-radius: 12px;
  padding: 10px 12px;
  font-size: 13px;
  outline: none;
  background: #fff;
  color: var(--mp-ink);
}

.mp-search:focus {
  border-color: color-mix(in srgb, var(--mp-accent) 50%, var(--mp-line));
  box-shadow: 0 0 0 3px rgba(15, 118, 110, 0.12);
}

.mp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(330px, 1fr));
  gap: 14px;
}

.mp-empty {
  border: 1px dashed #cbd5e1;
  border-radius: 16px;
  padding: 40px 16px;
  text-align: center;
  color: var(--mp-muted);
  font-size: 14px;
  background: #f8fafc;
}

:global(.dark) .mp-stage {
  --mp-ink: #e2e8f0;
  --mp-muted: #94a3b8;
  --mp-faint: #64748b;
  --mp-line: #1f2937;
  --mp-surface: #0f172a;
}

:global(.dark) .mp-hero,
:global(.dark) .mp-embedded-bar,
:global(.dark) .mp-filters,
:global(.dark) .mp-description,
:global(.dark) .mp-skeleton-card {
  background: #0f172a;
  border-color: #1f2937;
  box-shadow: none;
}

:global(.dark) .mp-refresh,
:global(.dark) .mp-search,
:global(.dark) .mp-stat {
  background: #111827;
  border-color: #1f2937;
  color: #e2e8f0;
}

:global(.dark) .mp-chip {
  background: #111827;
  border-color: #1f2937;
  color: #94a3b8;
}

:global(.dark) .mp-chip.is-active {
  background: linear-gradient(180deg, #0f766e, #0d9488);
  border-color: #0f766e;
  color: white;
}

:global(.dark) .mp-empty {
  background: #0b1220;
  border-color: #334155;
}

:global(.dark) .mp-stat--admin {
  background: linear-gradient(180deg, rgba(15, 118, 110, 0.16) 0%, #0f172a 100%);
}

:global(.dark) .mp-error {
  background: rgba(185, 28, 28, 0.12);
  border-color: rgba(248, 113, 113, 0.3);
  color: #fca5a5;
}

:global(.dark) .mp-skeleton-line,
:global(.dark) .mp-skeleton-block {
  background: linear-gradient(90deg, #1e293b 25%, #334155 37%, #1e293b 63%);
  background-size: 400% 100%;
}

@media (max-width: 720px) {
  .mp-hero__body {
    flex-direction: column;
  }
}
</style>
