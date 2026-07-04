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
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import { useAppStore } from '@/stores'
import adminTokenRewardsAPI from '@/api/admin/tokenRewards'
import type { TokenRewardConfig, TokenRewardCycleType, TokenRewardTokenUnit } from '@/api/tokenRewards'
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

onMounted(loadConfig)
</script>
