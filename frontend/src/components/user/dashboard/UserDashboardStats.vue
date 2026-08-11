<template>
  <div class="space-y-4">
    <!-- Row 1: Core Stats -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div v-if="!isSimple" class="metric-tile metric-tile--emerald">
        <div class="metric-tile__icon bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" />
          </svg>
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.balance') }}</p>
          <p class="metric-tile__value text-emerald-600 dark:text-emerald-400" :title="'$' + formatBalance(balance)">${{ formatBalance(balance) }}</p>
          <p class="metric-tile__meta">{{ t('common.available') }}</p>
        </div>
      </div>

      <div class="metric-tile metric-tile--primary">
        <div class="metric-tile__icon bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300">
          <Icon name="key" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.apiKeys') }}</p>
          <p class="metric-tile__value">{{ stats?.total_api_keys || 0 }}</p>
          <p class="metric-tile__meta text-emerald-600 dark:text-emerald-400">{{ stats?.active_api_keys || 0 }} {{ t('common.active') }}</p>
        </div>
      </div>

      <div class="metric-tile metric-tile--cyan">
        <div class="metric-tile__icon bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300">
          <Icon name="chart" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.todayRequests') }}</p>
          <p class="metric-tile__value">{{ stats?.today_requests || 0 }}</p>
          <p class="metric-tile__meta">{{ t('common.total') }}: {{ formatNumber(stats?.total_requests || 0) }}</p>
        </div>
      </div>

      <div class="metric-tile metric-tile--signal">
        <div class="metric-tile__icon bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
          <Icon name="dollar" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.todayCost') }}</p>
          <p class="metric-tile__value metric-tile__value--stack">
            <span class="metric-tile__value-main text-amber-700 dark:text-amber-300" :title="t('dashboard.actual')">${{ formatCost(stats?.today_actual_cost || 0) }}</span>
            <span class="metric-tile__value-sub" :title="t('dashboard.standard')">{{ t('dashboard.standard') }} ${{ formatCost(stats?.today_cost || 0) }}</span>
          </p>
          <p class="metric-tile__meta">
            <span>{{ t('common.total') }} </span>
            <span class="text-amber-700 dark:text-amber-300" :title="t('dashboard.actual')">${{ formatCost(stats?.total_actual_cost || 0) }}</span>
            <span class="text-slate-400" :title="t('dashboard.standard')"> · ${{ formatCost(stats?.total_cost || 0) }}</span>
          </p>
        </div>
      </div>
    </div>

    <!-- Row 2: Token / performance -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div class="metric-tile metric-tile--signal">
        <div class="metric-tile__icon bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
          <Icon name="cube" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.todayTokens') }}</p>
          <p class="metric-tile__value">{{ formatTokens(stats?.today_tokens || 0) }}</p>
          <p class="metric-tile__meta">{{ t('dashboard.input') }}: {{ formatTokens(stats?.today_input_tokens || 0) }} / {{ t('dashboard.output') }}: {{ formatTokens(stats?.today_output_tokens || 0) }}</p>
        </div>
      </div>

      <div class="metric-tile metric-tile--primary">
        <div class="metric-tile__icon bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">
          <Icon name="database" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.totalTokens') }}</p>
          <p class="metric-tile__value">{{ formatTokens(stats?.total_tokens || 0) }}</p>
          <p class="metric-tile__meta">{{ t('dashboard.input') }}: {{ formatTokens(stats?.total_input_tokens || 0) }} / {{ t('dashboard.output') }}: {{ formatTokens(stats?.total_output_tokens || 0) }}</p>
        </div>
      </div>

      <div class="metric-tile metric-tile--alarm">
        <div class="metric-tile__icon bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300">
          <Icon name="bolt" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="metric-tile__label">{{ t('dashboard.performance') }}</p>
          <div class="flex items-baseline gap-2">
            <p class="metric-tile__value">{{ formatTokens(stats?.rpm || 0) }}</p>
            <span class="text-xs text-slate-500">RPM</span>
          </div>
          <div class="flex items-baseline gap-2">
            <p class="text-sm font-semibold text-rose-600 dark:text-rose-400">{{ formatTokens(stats?.tpm || 0) }}</p>
            <span class="text-xs text-slate-500">TPM</span>
          </div>
        </div>
      </div>

      <div class="metric-tile metric-tile--emerald">
        <div class="metric-tile__icon bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
          <Icon name="clock" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="metric-tile__label">{{ t('dashboard.avgResponse') }}</p>
          <p class="metric-tile__value">{{ formatDuration(stats?.average_duration_ms || 0) }}</p>
          <p class="metric-tile__meta">{{ t('dashboard.averageTime') }}</p>
        </div>
      </div>
    </div>

    <!-- Row 3: Per-platform breakdown -->
    <div v-if="!isSimple && platformCards.length > 0" class="instrument-panel">
      <div class="instrument-panel__body">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <p class="page-kicker">PLATFORM RAILS</p>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.platformBreakdown') }}</h3>
          </div>
          <span class="rounded-full border border-slate-200 bg-white/80 px-2.5 py-1 text-xs text-slate-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300">
            {{ t('dashboard.platformCount', { count: sortedPlatforms.length }) }}
          </span>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div
            v-for="item in platformCards"
            :key="item.platform"
            :class="[
              'rounded-xl border p-3 shadow-sm',
              item.isOther
                ? 'border-dashed border-slate-300 bg-slate-50/80 dark:border-dark-500 dark:bg-dark-700/30'
                : 'border-slate-200/90 bg-white/70 dark:border-dark-600 dark:bg-dark-800/50'
            ]"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}
              </span>
              <span class="font-mono text-sm text-teal-700 dark:text-teal-300" :title="t('dashboard.actual')">
                ${{ formatCost(item.total_actual_cost) }}
              </span>
            </div>
            <div class="mt-2 space-y-1 text-xs">
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.todayCost') }}</span>
                <span class="font-mono text-gray-900 dark:text-white">${{ formatCost(item.today_actual_cost) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.requests') }}</span>
                <span class="font-mono text-gray-700 dark:text-gray-300">
                  {{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}
                </span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.tokens') }}</span>
                <span class="font-mono text-gray-700 dark:text-gray-300">
                  {{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}
                </span>
              </div>
            </div>

            <div v-if="hasAnyLimit(item.quota) && !item.isOther" class="mt-3 space-y-1.5 border-t border-slate-200 pt-2 dark:border-dark-700">
              <p class="text-[10px] uppercase tracking-wide text-gray-400">
                {{ t('dashboard.platformQuota.title') }}
              </p>
              <template v-for="w in (['daily', 'weekly', 'monthly'] as const)" :key="w">
                <div v-if="quotaVal(item.quota, `${w}_limit_usd`) != null" class="space-y-0.5">
                  <template v-if="(quotaVal(item.quota, `${w}_limit_usd`) as number) === 0">
                    <div class="flex items-center justify-between text-xs">
                      <span class="text-gray-600 dark:text-gray-300">{{ t(`dashboard.platformQuota.${w}`) }}</span>
                      <span class="font-mono text-red-500">{{ t('dashboard.platformQuota.disabled') }}</span>
                    </div>
                    <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                      <div class="h-full w-full rounded-full bg-red-500" />
                    </div>
                  </template>
                  <template v-else>
                    <div class="flex items-center justify-between text-xs">
                      <span class="text-gray-600 dark:text-gray-300">{{ t(`dashboard.platformQuota.${w}`) }}</span>
                      <span class="font-mono text-gray-700 dark:text-gray-200">
                        ${{ formatUsd((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0) }} / ${{ formatUsd(quotaVal(item.quota, `${w}_limit_usd`) as number) }}
                      </span>
                    </div>
                    <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                      <div
                        class="h-full rounded-full transition-all"
                        :class="quotaBarClass(calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number))"
                        :style="{ width: calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) + '%' }"
                      />
                    </div>
                    <p v-if="quotaVal(item.quota, `${w}_window_resets_at`)" class="text-[10px] text-gray-400">
                      {{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(quotaVal(item.quota, `${w}_window_resets_at`) as string) }) }}
                    </p>
                  </template>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

interface FusedPlatformCard {
  platform: string
  total_actual_cost: number
  today_actual_cost: number
  total_requests: number
  total_tokens: number
  isOther?: boolean
  quota?: PlatformQuotaItem
}

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  platformQuotas?: PlatformQuotaItem[] | null
}>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok',
  glm: 'GLM',
  seedance: 'Seedance',
  ltx: 'LTX',
  happyhorse: 'HappyHorse',
  minimax: 'MiniMax'
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

const sortedPlatforms = computed(() => {
  const list = props.stats?.by_platform ?? []
  return [...list].sort((a, b) => b.total_actual_cost - a.total_actual_cost)
})

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, (typeof sortedPlatforms.value)[number]>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // union 平台集合。后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>([...byPlat.keys(), ...byQuota.keys()])

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'glm', 'seedance', 'ltx', 'happyhorse', 'minimax']
  const cards: FusedPlatformCard[] = []

  for (const p of platforms) {
    const stat = byPlat.get(p)
    cards.push({
      platform: p,
      total_actual_cost: stat?.total_actual_cost ?? 0,
      today_actual_cost: stat?.today_actual_cost ?? 0,
      total_requests: stat?.total_requests ?? 0,
      total_tokens: stat?.total_tokens ?? 0,
      quota: byQuota.get(p),
    })
  }

  // 排序：按 PLATFORM_ORDER，未知平台按名称排序
  cards.sort((a, b) => {
    const ai = PLATFORM_ORDER.indexOf(a.platform)
    const bi = PLATFORM_ORDER.indexOf(b.platform)
    if (ai === -1 && bi === -1) return a.platform.localeCompare(b.platform)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  // __other__ 补差逻辑：只对 by_platform 有 usage 数据的总和计算
  const total = props.stats?.total_actual_cost ?? 0
  const today = props.stats?.today_actual_cost ?? 0
  const sumTotal = cards.reduce((s, c) => s + c.total_actual_cost, 0)
  const sumToday = cards.reduce((s, c) => s + c.today_actual_cost, 0)
  const diffTotal = Math.max(0, total - sumTotal)
  const diffToday = Math.max(0, today - sumToday)

  if (diffTotal > OTHER_THRESHOLD || diffToday > OTHER_THRESHOLD) {
    cards.push({
      platform: '__other__',
      total_actual_cost: diffTotal,
      today_actual_cost: diffToday,
      total_requests: 0,
      total_tokens: 0,
      isOther: true,
    })
  }

  return cards
})

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'
type QuotaField = `${QuotaWindow}_limit_usd` | `${QuotaWindow}_usage_usd` | `${QuotaWindow}_window_resets_at`

function quotaVal(q: PlatformQuotaItem | undefined, key: QuotaField): PlatformQuotaItem[QuotaField] {
  return q?.[key]
}

function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
}

function calcPercent(usage: number, limit: number): number {
  if (!limit || limit <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((usage / limit) * 100)))
}

function quotaBarClass(p: number): string {
  if (p >= 95) return 'bg-red-500'
  if (p >= 75) return 'bg-amber-500'
  return 'bg-green-500'
}

// 与 formatBalance 一致使用 Intl.NumberFormat 做半偶舍入，避免 toFixed 在不同 JS 引擎
// 下偶发截断而非四舍五入（与后端展示精度不一致）。
const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
function formatUsd(n: number): string {
  if (!Number.isFinite(n)) return '0.00'
  return usdFormatter.format(n)
}

function formatResetTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`
</script>
