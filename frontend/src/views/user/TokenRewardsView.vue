<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div class="grid gap-4 md:grid-cols-3">
        <div class="card p-5">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('tokenRewards.currentTokens') }}</p>
              <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
                {{ formatTokens(status?.current_tokens || 0) }}
              </p>
            </div>
            <Icon name="chartBar" size="lg" class="text-primary-500" />
          </div>
        </div>
        <div class="card p-5">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('tokenRewards.cycle') }}</p>
              <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">
                {{ status?.cycle.type === 'monthly' ? t('tokenRewards.monthly') : t('tokenRewards.weekly') }}
              </p>
            </div>
            <Icon name="calendar" size="lg" class="text-emerald-500" />
          </div>
        </div>
        <div class="card p-5">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('tokenRewards.endsAt') }}</p>
              <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">
                {{ status ? formatDateTime(status.cycle.end) : '-' }}
              </p>
            </div>
            <Icon name="clock" size="lg" class="text-amber-500" />
          </div>
        </div>
      </div>

      <div v-if="!loading && status && !status.config.enabled" class="card border-amber-200 bg-amber-50 p-5 dark:border-amber-800/60 dark:bg-amber-900/20">
        <div class="flex items-center gap-3 text-amber-800 dark:text-amber-200">
          <Icon name="exclamationTriangle" size="md" />
          <span class="text-sm font-medium">{{ t('tokenRewards.disabled') }}</span>
        </div>
      </div>

      <div v-if="!loading && status && status.config.enabled" class="rounded-lg border border-sky-200 bg-sky-50 p-4 text-sky-800 dark:border-sky-800/60 dark:bg-sky-900/20 dark:text-sky-200">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="md" class="mt-0.5 shrink-0" />
          <div class="space-y-1 text-sm">
            <p class="font-semibold">{{ t('tokenRewards.cycleFreezeTitle') }}</p>
            <p>{{ t('tokenRewards.cycleFreezeDescription') }}</p>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('tokenRewards.title') }}</h2>
          <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadStatus">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>

        <div v-if="loading" class="flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>

        <div v-else-if="!status || status.tiers.length === 0" class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('tokenRewards.noTiers') }}
        </div>

        <div v-else class="space-y-6 p-6">
          <div class="space-y-4">
            <div class="flex flex-wrap items-end justify-between gap-3">
              <div>
                <p class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ t('tokenRewards.overallProgress') }}</p>
                <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
                  {{ formatTokens(currentTokens) }} / {{ formatTokens(maxRequiredTokens) }}
                </p>
              </div>
              <div v-if="nextLockedTier" class="text-sm text-gray-500 dark:text-dark-400">
                {{ t('tokenRewards.nextTier') }}:
                <span class="font-medium text-gray-900 dark:text-white">{{ formatTierTokens(nextLockedTier.tier.tokens, nextLockedTier.tier.token_unit) }}</span>
              </div>
            </div>

            <div class="relative pb-14 pt-6">
              <div class="h-3 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div
                  class="h-full rounded-full bg-primary-500 transition-all"
                  :style="{ width: `${overallProgressPercent}%` }"
                />
              </div>

              <div
                v-for="item in tierProgressItems"
                :key="item.tier.id"
                class="absolute top-3"
                :class="markerPositionClass(item.position)"
                :style="{ left: `${item.position}%` }"
              >
                <div
                  class="mx-auto flex h-9 w-9 items-center justify-center rounded-full border-2 bg-white shadow-sm dark:bg-dark-900"
                  :class="markerClass(item.status)"
                >
                  <Icon :name="item.status === 'claimed' ? 'check' : item.status === 'claimable' ? 'gift' : 'lock'" size="sm" />
                </div>
                <div class="mt-2 w-32 text-xs leading-5" :class="markerLabelClass(item.position)">
                  <p class="font-medium text-gray-900 dark:text-white">{{ formatTierTokens(item.tier.tokens, item.tier.token_unit) }}</p>
                  <p class="text-gray-500 dark:text-dark-400">${{ item.tier.reward_balance.toFixed(2) }}</p>
                </div>
              </div>
            </div>
          </div>

          <div class="divide-y divide-gray-100 rounded-lg border border-gray-100 dark:divide-dark-700 dark:border-dark-700">
            <div v-for="item in status.tiers" :key="item.tier.id" class="flex flex-col gap-3 p-4 lg:flex-row lg:items-center lg:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-3">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ formatTierTokens(item.tier.tokens, item.tier.token_unit) }}
                  </h3>
                  <span class="badge" :class="statusClass(item.status)">
                    {{ statusLabel(item.status) }}
                  </span>
                </div>
                <div class="mt-1 flex flex-wrap gap-x-5 gap-y-1 text-sm text-gray-500 dark:text-dark-400">
                  <span>{{ t('tokenRewards.reward') }}: ${{ item.tier.reward_balance.toFixed(2) }}</span>
                  <span v-if="item.claimed_at">{{ t('tokenRewards.claimedAt') }}: {{ formatDateTime(item.claimed_at) }}</span>
                </div>
              </div>
              <button
                class="btn btn-primary min-w-32"
                :disabled="item.status !== 'claimable' || claimingTierId === item.tier.id"
                @click="claimTier(item.tier.id)"
              >
                <Icon
                  :name="item.status === 'claimed' ? 'checkCircle' : 'gift'"
                  size="md"
                  class="mr-1"
                  :class="claimingTierId === item.tier.id ? 'animate-pulse' : ''"
                />
                {{ claimButtonText(item.status, item.tier.id) }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('tokenRewards.claimHistory') }}</h2>
          <button class="btn btn-secondary" :disabled="claimsLoading" :title="t('common.refresh')" @click="loadClaims">
            <Icon name="refresh" size="md" :class="claimsLoading ? 'animate-spin' : ''" />
          </button>
        </div>

        <div v-if="claimsLoading" class="flex items-center justify-center py-12">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>
        <div v-else-if="claims.length === 0" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('tokenRewards.noClaimHistory') }}
        </div>
        <div v-else>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.historyTier') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.historyCycle') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.historyRequired') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.historySnapshot') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.reward') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('tokenRewards.claimedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="claim in claims" :key="claim.id">
                  <td class="whitespace-nowrap px-6 py-4 font-mono text-sm text-gray-900 dark:text-white">{{ claim.tier_id }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">
                    {{ claim.cycle_type === 'monthly' ? t('tokenRewards.monthly') : t('tokenRewards.weekly') }}
                  </td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatTokens(claim.required_tokens, claim.token_unit) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatTokens(claim.token_snapshot) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">${{ claim.reward_balance.toFixed(2) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatDateTime(claim.claimed_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 px-6 py-4 text-sm dark:border-dark-700">
            <span class="text-gray-500 dark:text-dark-400">
              {{ t('tokenRewards.pagination', { page: claimsPage, pages: claimsPages, total: claimsTotal }) }}
            </span>
            <div class="flex gap-2">
              <button class="btn btn-secondary" :disabled="claimsPage <= 1 || claimsLoading" @click="changeClaimsPage(claimsPage - 1)">
                <Icon name="chevronLeft" size="md" />
              </button>
              <button class="btn btn-secondary" :disabled="claimsPage >= claimsPages || claimsLoading" @click="changeClaimsPage(claimsPage + 1)">
                <Icon name="chevronRight" size="md" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import { useAppStore, useAuthStore } from '@/stores'
import tokenRewardsAPI, { type TokenRewardClaim, type TokenRewardTokenUnit, type TokenRewardStatus, type TokenRewardTier, type TokenRewardTierState } from '@/api/tokenRewards'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const status = ref<TokenRewardStatus | null>(null)
const loading = ref(false)
const claims = ref<TokenRewardClaim[]>([])
const claimsLoading = ref(false)
const claimsPage = ref(1)
const claimsPageSize = 10
const claimsPages = ref(1)
const claimsTotal = ref(0)
const claimingTierId = ref<string | null>(null)
const progressEdgePaddingPercent = 6

const currentTokens = computed(() => status.value?.current_tokens || 0)
const tierProgressItems = computed(() => {
  const max = maxRequiredTokens.value
  return (status.value?.tiers || []).map((item) => {
    const required = requiredTokenCount(item.tier)
    const rawPosition = max > 0 ? Math.min(100, Math.max(0, (required / max) * 100)) : 0
    return {
      ...item,
      required,
      position: visualMarkerPosition(rawPosition)
    }
  })
})
const maxRequiredTokens = computed(() => {
  return Math.max(0, ...(status.value?.tiers || []).map((item) => requiredTokenCount(item.tier)))
})
const overallProgressPercent = computed(() => {
  return visualProgressPercent(currentTokens.value, tierProgressItems.value)
})
const nextLockedTier = computed(() => {
  return (status.value?.tiers || []).find((item) => item.status === 'locked') || null
})

const unitFactors: Record<TokenRewardTokenUnit, number> = {
  raw: 1,
  K: 1_000,
  M: 1_000_000,
  B: 1_000_000_000,
  T: 1_000_000_000_000
}

function autoTokenUnit(value: number): TokenRewardTokenUnit {
  if (value >= unitFactors.T) return 'T'
  if (value >= unitFactors.B) return 'B'
  if (value >= unitFactors.M) return 'M'
  if (value >= unitFactors.K) return 'K'
  return 'raw'
}

function formatTokens(value: number, preferredUnit?: TokenRewardTokenUnit): string {
  const unit = preferredUnit || autoTokenUnit(value)
  if (unit === 'raw') return `${new Intl.NumberFormat().format(value)} Tokens`
  const scaled = value / unitFactors[unit]
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(scaled)}${unit} Tokens`
}

function formatTierTokens(value: number, unit: TokenRewardTokenUnit): string {
  if (unit === 'raw') return `${new Intl.NumberFormat().format(value)} Tokens`
  return `${new Intl.NumberFormat().format(value)}${unit} Tokens`
}

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString()
}

function requiredTokenCount(tier: TokenRewardTier): number {
  return tier.tokens * unitFactors[tier.token_unit || 'raw']
}

function visualMarkerPosition(rawPosition: number): number {
  if (rawPosition <= 0) return 0
  if (rawPosition >= 100) return 100
  return progressEdgePaddingPercent + rawPosition * ((100 - progressEdgePaddingPercent) / 100)
}

function visualProgressPercent(tokens: number, items: Array<{ required: number; position: number }>): number {
  if (items.length === 0 || tokens <= 0) return 0

  let previousRequired = 0
  let previousPosition = 0

  for (const item of items) {
    if (tokens < item.required) {
      const intervalTokens = item.required - previousRequired
      if (intervalTokens <= 0) return item.position
      const intervalProgress = (tokens - previousRequired) / intervalTokens
      return Math.min(100, Math.max(0, previousPosition + (item.position - previousPosition) * intervalProgress))
    }

    previousRequired = item.required
    previousPosition = item.position
  }

  return 100
}

function markerPositionClass(position: number): string {
  if (position <= 4) return 'translate-x-0'
  if (position >= 96) return '-translate-x-full'
  return '-translate-x-1/2'
}

function markerLabelClass(position: number): string {
  if (position <= 4) return 'text-left'
  if (position >= 96) return 'text-right'
  return 'text-center'
}

function markerClass(state: TokenRewardTierState): string {
  if (state === 'claimed') return 'border-emerald-500 text-emerald-600 dark:text-emerald-400'
  if (state === 'claimable') return 'border-amber-500 text-amber-600 dark:text-amber-400'
  return 'border-gray-300 text-gray-400 dark:border-dark-600 dark:text-dark-400'
}

function statusLabel(state: TokenRewardTierState): string {
  return t(`tokenRewards.status.${state}`)
}

function statusClass(state: TokenRewardTierState): string {
  if (state === 'claimed') return 'badge-success'
  if (state === 'claimable') return 'badge-warning'
  return 'badge-gray'
}

function claimButtonText(state: TokenRewardTierState, tierId: string): string {
  if (claimingTierId.value === tierId) return t('tokenRewards.claiming')
  if (state === 'claimed') return t('tokenRewards.claimed')
  if (state === 'claimable') return t('tokenRewards.claim')
  return t('tokenRewards.locked')
}

async function loadStatus() {
  loading.value = true
  try {
    status.value = await tokenRewardsAPI.getStatus()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('tokenRewards.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadClaims() {
  claimsLoading.value = true
  try {
    const result = await tokenRewardsAPI.listClaims({ page: claimsPage.value, page_size: claimsPageSize })
    claims.value = result.items || []
    claimsPage.value = result.page || 1
    claimsPages.value = result.pages || 1
    claimsTotal.value = result.total || 0
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('tokenRewards.claimHistoryLoadFailed')))
  } finally {
    claimsLoading.value = false
  }
}

function changeClaimsPage(page: number) {
  claimsPage.value = Math.max(1, Math.min(page, claimsPages.value))
  loadClaims()
}

async function claimTier(tierId: string) {
  claimingTierId.value = tierId
  try {
    const result = await tokenRewardsAPI.claim(tierId)
    appStore.showSuccess(t('tokenRewards.claimSuccess', { amount: result.claim.reward_balance.toFixed(2) }))
    claimsPage.value = 1
    await Promise.all([loadStatus(), loadClaims(), authStore.refreshUser().catch(() => undefined)])
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('tokenRewards.claimFailed')))
  } finally {
    claimingTierId.value = null
  }
}

onMounted(() => {
  loadStatus()
  loadClaims()
})
</script>
