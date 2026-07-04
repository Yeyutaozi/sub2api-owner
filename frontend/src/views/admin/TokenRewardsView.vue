<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div class="card p-6">
        <div class="grid gap-6 md:grid-cols-2">
          <label class="flex items-center justify-between rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <span>
              <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenRewards.enabled') }}</span>
              <span class="mt-1 block text-sm text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.enabledState', { state: form.enabled ? t('common.enabled') : t('common.disabled') }) }}</span>
            </span>
            <input v-model="form.enabled" type="checkbox" class="h-5 w-5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </label>

          <div>
            <label class="input-label">{{ t('admin.tokenRewards.cycle') }}</label>
            <div class="mt-2 grid grid-cols-2 gap-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
              <button
                type="button"
                class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
                :class="form.cycle === 'weekly' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700' : 'text-gray-600 dark:text-dark-300'"
                @click="form.cycle = 'weekly'"
              >
                {{ t('admin.tokenRewards.weekly') }}
              </button>
              <button
                type="button"
                class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
                :class="form.cycle === 'monthly' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700' : 'text-gray-600 dark:text-dark-300'"
                @click="form.cycle = 'monthly'"
              >
                {{ t('admin.tokenRewards.monthly') }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="rounded-lg border border-sky-200 bg-sky-50 p-4 text-sky-800 dark:border-sky-800/60 dark:bg-sky-900/20 dark:text-sky-200">
        <div class="flex gap-3">
          <Icon name="infoCircle" size="md" class="mt-0.5 shrink-0" />
          <div class="space-y-1 text-sm">
            <p class="font-semibold">{{ t('admin.tokenRewards.cycleFreezeTitle') }}</p>
            <p>{{ t('admin.tokenRewards.cycleFreezeDescription') }}</p>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenRewards.tiers') }}</h2>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadConfig">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="addTier">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.tokenRewards.addTier') }}
            </button>
          </div>
        </div>

        <div v-if="loading" class="flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>

        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <div v-if="form.tiers.length === 0" class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.tokenRewards.noTiers') }}
          </div>

          <div v-for="(tier, index) in form.tiers" :key="tier.localId" class="grid gap-4 p-6 md:grid-cols-[1fr_1fr_9rem_1fr_auto] md:items-end">
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.tierId') }}</label>
              <input v-model.trim="tier.id" class="input mt-1 font-mono" :placeholder="`tier_${index + 1}`" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.requiredTokens') }}</label>
              <input
                :value="displayTokenValue(tier)"
                type="number"
                min="0"
                step="1"
                class="input mt-1"
                @input="updateTierTokens(tier, $event)"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.tokenUnit') }}</label>
              <select v-model="tier.token_unit" class="input mt-1">
                <option v-for="unit in tokenUnits" :key="unit.value" :value="unit.value">
                  {{ unit.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.rewardBalance') }}</label>
              <input v-model.number="tier.reward_balance" type="number" min="0.01" step="0.01" class="input mt-1" />
            </div>
            <button class="btn btn-danger md:mb-0" :title="t('common.delete')" @click="removeTier(index)">
              <Icon name="trash" size="md" />
            </button>
          </div>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" :disabled="saving" @click="loadConfig">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="saveConfig">
          <Icon name="check" size="md" class="mr-1" />
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>

      <div class="card overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenRewards.claimHistory') }}</h2>
          <button class="btn btn-secondary" :disabled="claimsLoading" :title="t('common.refresh')" @click="loadClaims">
            <Icon name="refresh" size="md" :class="claimsLoading ? 'animate-spin' : ''" />
          </button>
        </div>

        <div class="space-y-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_9rem_10rem_10rem_auto_auto] md:items-end">
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.filterUser') }}</label>
              <input
                v-model.trim="claimFilters.search"
                type="search"
                class="input mt-1"
                :placeholder="t('admin.tokenRewards.filterUserPlaceholder')"
                @keyup.enter="applyClaimFilters"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.filterTier') }}</label>
              <input
                v-model.trim="claimFilters.tier_id"
                type="search"
                class="input mt-1 font-mono"
                :placeholder="t('admin.tokenRewards.filterTierPlaceholder')"
                @keyup.enter="applyClaimFilters"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.historyCycle') }}</label>
              <select v-model="claimFilters.cycle_type" class="input mt-1">
                <option value="">{{ t('common.all') }}</option>
                <option value="weekly">{{ t('admin.tokenRewards.weekly') }}</option>
                <option value="monthly">{{ t('admin.tokenRewards.monthly') }}</option>
              </select>
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.claimedFrom') }}</label>
              <input v-model="claimFilters.claimed_from" type="date" class="input mt-1" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.tokenRewards.claimedTo') }}</label>
              <input v-model="claimFilters.claimed_to" type="date" class="input mt-1" />
            </div>
            <button class="btn btn-secondary" :disabled="claimsLoading" @click="resetClaimFilters">
              {{ t('common.reset') }}
            </button>
            <button class="btn btn-primary" :disabled="claimsLoading" @click="applyClaimFilters">
              {{ t('admin.tokenRewards.applyFilters') }}
            </button>
          </div>

          <div class="grid divide-y divide-gray-100 rounded-md bg-gray-50 dark:divide-dark-700 dark:bg-dark-800 md:grid-cols-4 md:divide-x md:divide-y-0">
            <div class="px-4 py-3">
              <p class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.statClaims') }}</p>
              <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatNumber(claimStats.total_claims) }}</p>
            </div>
            <div class="px-4 py-3">
              <p class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.statUsers') }}</p>
              <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatNumber(claimStats.unique_users) }}</p>
            </div>
            <div class="px-4 py-3">
              <p class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.statReward') }}</p>
              <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatMoney(claimStats.total_reward_balance) }}</p>
            </div>
            <div class="px-4 py-3">
              <p class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.statTokens') }}</p>
              <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatTokens(claimStats.total_token_snapshot) }}</p>
            </div>
          </div>
        </div>

        <div v-if="claimsLoading" class="flex items-center justify-center py-12">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>
        <div v-else-if="claims.length === 0" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.tokenRewards.noClaimHistory') }}
        </div>
        <div v-else>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.user') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.historyTier') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.historyCycle') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.historyRequired') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.historySnapshot') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.reward') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.tokenRewards.claimedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="claim in claims" :key="claim.id">
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-900 dark:text-white">
                    <div class="font-medium">{{ claim.user_email || '-' }}</div>
                    <div class="text-xs text-gray-500 dark:text-dark-400">ID: {{ claim.user_id }}</div>
                  </td>
                  <td class="whitespace-nowrap px-6 py-4 font-mono text-sm text-gray-900 dark:text-white">{{ claim.tier_id }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">
                    {{ claim.cycle_type === 'monthly' ? t('admin.tokenRewards.monthly') : t('admin.tokenRewards.weekly') }}
                  </td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatTokens(claim.required_tokens, claim.token_unit) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatTokens(claim.token_snapshot) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">{{ formatMoney(claim.reward_balance) }}</td>
                  <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatDateTime(claim.claimed_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="claimsPagination.total > 0"
            :page="claimsPagination.page"
            :total="claimsPagination.total"
            :page-size="claimsPagination.page_size"
            @update:page="handleClaimsPageChange"
            @update:pageSize="handleClaimsPageSizeChange"
          />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { Icon } from '@/components/icons'
import { useAppStore } from '@/stores'
import adminTokenRewardsAPI from '@/api/admin/tokenRewards'
import type { AdminTokenRewardClaim, AdminTokenRewardClaimQuery, AdminTokenRewardClaimStats } from '@/api/admin/tokenRewards'
import type { TokenRewardConfig, TokenRewardCycleType, TokenRewardTokenUnit } from '@/api/tokenRewards'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { extractApiErrorMessage } from '@/utils/apiError'

interface TierForm {
  localId: string
  id: string
  tokens: number
  token_unit: TokenRewardTokenUnit
  reward_balance: number
}

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const claims = ref<AdminTokenRewardClaim[]>([])
const claimsLoading = ref(false)
const claimsPagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 1
})
const claimFilters = reactive({
  search: '',
  tier_id: '',
  cycle_type: '' as '' | TokenRewardCycleType,
  claimed_from: '',
  claimed_to: ''
})
const defaultClaimStats = (): AdminTokenRewardClaimStats => ({
  total_claims: 0,
  unique_users: 0,
  total_reward_balance: 0,
  total_token_snapshot: 0
})
const claimStats = reactive<AdminTokenRewardClaimStats>(defaultClaimStats())
const tokenUnits: Array<{ value: TokenRewardTokenUnit; label: string }> = [
  { value: 'raw', label: t('admin.tokenRewards.units.raw') },
  { value: 'K', label: t('admin.tokenRewards.units.K') },
  { value: 'M', label: t('admin.tokenRewards.units.M') },
  { value: 'B', label: t('admin.tokenRewards.units.B') },
  { value: 'T', label: t('admin.tokenRewards.units.T') }
]
const unitFactors: Record<TokenRewardTokenUnit, number> = {
  raw: 1,
  K: 1_000,
  M: 1_000_000,
  B: 1_000_000_000,
  T: 1_000_000_000_000
}

const form = reactive<{
  enabled: boolean
  cycle: TokenRewardCycleType
  tiers: TierForm[]
}>({
  enabled: false,
  cycle: 'weekly',
  tiers: []
})

function displayTokenValue(tier: TierForm): number {
  return Number(tier.tokens || 0)
}

function updateTierTokens(tier: TierForm, event: Event) {
  const parsed = Number((event.target as HTMLInputElement).value)
  tier.tokens = Number.isFinite(parsed) ? Math.floor(parsed) : 0
}

function toForm(config: TokenRewardConfig) {
  form.enabled = config.enabled
  form.cycle = config.cycle || 'weekly'
  form.tiers = (config.tiers || []).map((tier, index) => ({
    localId: `${tier.id || 'tier'}_${index}_${Date.now()}`,
    id: tier.id,
    tokens: tier.tokens,
    token_unit: tier.token_unit || 'raw',
    reward_balance: tier.reward_balance
  }))
}

function toPayload(): TokenRewardConfig {
  return {
    enabled: form.enabled,
    cycle: form.cycle,
    tiers: form.tiers.map((tier, index) => ({
      id: tier.id.trim() || `tier_${index + 1}`,
      tokens: Number(tier.tokens),
      token_unit: tier.token_unit || 'raw',
      reward_balance: Number(tier.reward_balance)
    }))
  }
}

function requiredTokenCount(tier: Pick<TierForm, 'tokens' | 'token_unit'>): number {
  return Number(tier.tokens || 0) * unitFactors[tier.token_unit || 'raw']
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

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value || 0)
}

function formatMoney(value: number): string {
  return `$${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value || 0)}`
}

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString()
}

function validate(config: TokenRewardConfig): string | null {
  const ids = new Set<string>()
  const thresholds = new Set<number>()
  for (const tier of config.tiers) {
    if (!/^[A-Za-z0-9_-]{1,64}$/.test(tier.id)) return t('admin.tokenRewards.invalidTierId')
    if (ids.has(tier.id)) return t('admin.tokenRewards.duplicateTierId')
    ids.add(tier.id)
    if (!Number.isInteger(tier.tokens) || tier.tokens <= 0) return t('admin.tokenRewards.invalidTokens')
    if (!['raw', 'K', 'M', 'B', 'T'].includes(tier.token_unit)) return t('admin.tokenRewards.invalidTokenUnit')
    const requiredTokens = requiredTokenCount(tier)
    if (thresholds.has(requiredTokens)) return t('admin.tokenRewards.duplicateThreshold')
    thresholds.add(requiredTokens)
    if (!Number.isFinite(tier.reward_balance) || tier.reward_balance <= 0) return t('admin.tokenRewards.invalidReward')
  }
  return null
}

function addTier() {
  const next = form.tiers.length + 1
  form.tiers.push({
    localId: `new_${Date.now()}_${next}`,
    id: `tier_${next}`,
    tokens: next,
    token_unit: 'M',
    reward_balance: next
  })
}

function removeTier(index: number) {
  form.tiers.splice(index, 1)
}

async function loadConfig() {
  loading.value = true
  try {
    toForm(await adminTokenRewardsAPI.getConfig())
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.tokenRewards.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  const payload = toPayload()
  const validationError = validate(payload)
  if (validationError) {
    appStore.showError(validationError)
    return
  }
  saving.value = true
  try {
    toForm(await adminTokenRewardsAPI.updateConfig(payload))
    appStore.showSuccess(t('admin.tokenRewards.saveSuccess'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.tokenRewards.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function loadClaims() {
  claimsLoading.value = true
  try {
    const result = await adminTokenRewardsAPI.listClaims(buildClaimQuery())
    claims.value = result.items || []
    claimsPagination.page = result.page || 1
    claimsPagination.page_size = result.page_size || claimsPagination.page_size
    claimsPagination.pages = result.pages || 1
    claimsPagination.total = result.total || 0
    Object.assign(claimStats, result.stats || defaultClaimStats())
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.tokenRewards.claimHistoryLoadFailed')))
  } finally {
    claimsLoading.value = false
  }
}

function buildClaimQuery(): AdminTokenRewardClaimQuery {
  const params: AdminTokenRewardClaimQuery = {
    page: claimsPagination.page,
    page_size: claimsPagination.page_size
  }
  if (claimFilters.search.trim()) params.search = claimFilters.search.trim()
  if (claimFilters.tier_id.trim()) params.tier_id = claimFilters.tier_id.trim()
  if (claimFilters.cycle_type) params.cycle_type = claimFilters.cycle_type
  if (claimFilters.claimed_from) params.claimed_from = claimFilters.claimed_from
  if (claimFilters.claimed_to) params.claimed_to = claimFilters.claimed_to
  return params
}

function applyClaimFilters() {
  claimsPagination.page = 1
  loadClaims()
}

function resetClaimFilters() {
  claimFilters.search = ''
  claimFilters.tier_id = ''
  claimFilters.cycle_type = ''
  claimFilters.claimed_from = ''
  claimFilters.claimed_to = ''
  applyClaimFilters()
}

function handleClaimsPageChange(page: number) {
  claimsPagination.page = page
  loadClaims()
}

function handleClaimsPageSizeChange(pageSize: number) {
  claimsPagination.page_size = pageSize
  claimsPagination.page = 1
  loadClaims()
}

onMounted(() => {
  loadConfig()
  loadClaims()
})
</script>
