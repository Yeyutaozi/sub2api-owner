<template>
  <BaseDialog
    :show="show"
    :title="isVideoPricingGroup ? t('admin.groups.userPricingTitle') : t('admin.groups.rateMultipliersTitle')"
    width="wide"
    @close="handleClose"
  >
    <div v-if="group" class="space-y-4">
      <!-- 分组信息 -->
      <div class="flex flex-wrap items-center gap-3 rounded-lg bg-gray-50 px-4 py-2.5 text-sm dark:bg-dark-700">
        <span class="inline-flex items-center gap-1.5" :class="platformColorClass">
          <PlatformIcon :platform="group.platform" size="sm" />
          {{ t('admin.groups.platforms.' + group.platform) }}
        </span>
        <span class="text-gray-400">|</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
        <span class="text-gray-400">|</span>
        <span class="text-gray-600 dark:text-gray-400">
          {{ t('admin.groups.columns.rateMultiplier') }}: {{ group.rate_multiplier }}x
        </span>
      </div>

      <div
        v-if="isVideoPricingGroup"
        class="space-y-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-xs leading-5 text-blue-800 dark:border-blue-900/60 dark:bg-blue-900/20 dark:text-blue-300"
      >
        <p class="font-medium">{{ t('admin.groups.videoPriceOverrides.priorityHint') }}</p>
        <p>{{ t('admin.groups.videoPriceOverrides.workflowHint') }}</p>
      </div>

      <div
        v-if="isVideoPricingGroup"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      >
        <div class="mb-2 flex items-center justify-between gap-2">
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.groups.videoPriceOverrides.groupDefaultsTitle') }}
          </h4>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ videoPriceUnitLabel }}</span>
        </div>
        <div v-if="groupDefaultPriceRows.length === 0" class="text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.groups.videoPriceOverrides.groupDefaultsEmpty') }}
        </div>
        <div v-else class="max-h-36 space-y-1 overflow-y-auto text-xs">
          <div
            v-for="row in groupDefaultPriceRows"
            :key="row.model"
            class="grid grid-cols-[minmax(160px,1.2fr)_minmax(0,2fr)] gap-2 rounded bg-gray-50 px-2 py-1.5 dark:bg-dark-700/60"
          >
            <span class="font-mono text-gray-700 dark:text-gray-300">{{ row.model }}</span>
            <span class="text-gray-600 dark:text-gray-400">{{ row.pricesText }}</span>
          </div>
        </div>
      </div>

      <!-- 操作区 -->
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <!-- 添加用户 -->
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ isVideoPricingGroup ? t('admin.groups.videoPriceOverrides.addUser') : t('admin.groups.addUserRate') }}
        </h4>
        <p v-if="isVideoPricingGroup" class="mb-2 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.videoPriceOverrides.addUserHint') }}
        </p>
        <div class="flex items-end gap-2">
          <div class="relative flex-1">
            <input
              v-model="searchQuery"
              type="text"
              autocomplete="off"
              class="input w-full"
              :placeholder="t('admin.groups.searchUserPlaceholder')"
              @input="handleSearchUsers"
              @focus="showDropdown = true"
              @keydown.enter.prevent="handleAddLocal"
            />
            <div
              v-if="showDropdown && searchResults.length > 0"
              class="absolute left-0 right-0 top-full z-10 mt-1 max-h-48 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-500 dark:bg-dark-700"
            >
              <button
                v-for="user in searchResults"
                :key="user.id"
                type="button"
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
                @click="selectUser(user)"
              >
                <span class="text-gray-400">#{{ user.id }}</span>
                <span class="text-gray-900 dark:text-white">{{ user.username || user.email }}</span>
                <span v-if="user.username" class="text-xs text-gray-400">{{ user.email }}</span>
              </button>
            </div>
          </div>
          <div v-if="!isVideoPricingGroup" class="w-28">
            <input
              v-model.number="newRate"
              type="number"
              step="0.001"
              min="0"
              autocomplete="off"
              class="hide-spinner input w-full"
              placeholder="1.0"
            />
          </div>
          <details v-else class="w-36">
            <summary class="mb-1 cursor-pointer select-none text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.groups.videoPriceOverrides.optionalRate') }}
            </summary>
            <input
              v-model.number="newRate"
              type="number"
              step="0.001"
              min="0"
              autocomplete="off"
              class="hide-spinner input w-full"
              :placeholder="t('admin.groups.videoPriceOverrides.optionalRatePlaceholder')"
            />
          </details>
          <button
            type="button"
            class="btn btn-primary shrink-0"
            :disabled="!canAddUser"
            @click="handleAddLocal"
          >
            {{ isVideoPricingGroup ? t('admin.groups.videoPriceOverrides.addAndConfigure') : t('common.add') }}
          </button>
        </div>

        <!-- 批量调整 + 全部清空 -->
        <div v-if="localEntries.length > 0 && !isVideoPricingGroup" class="mt-3 flex items-center gap-3 border-t border-gray-100 pt-3 dark:border-dark-600">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.batchAdjust') }}</span>
          <div class="flex items-center gap-1.5">
            <span class="text-xs text-gray-400">×</span>
            <input
              v-model.number="batchFactor"
              type="number"
              step="0.1"
              min="0"
              autocomplete="off"
              class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
              placeholder="0.5"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm shrink-0 px-2.5 py-1 text-xs"
              :disabled="!batchFactor || batchFactor <= 0"
              @click="applyBatchFactor"
            >
              {{ t('admin.groups.applyMultiplier') }}
            </button>
          </div>
          <div class="ml-auto">
            <button
              type="button"
              class="rounded-lg border border-red-200 bg-red-50 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-100 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/40"
              @click="clearAllLocal"
            >
              {{ t('admin.groups.clearAll') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="loading" class="flex justify-center py-6">
        <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <div
        v-if="isVideoPricingGroup && localEntries.length > 0"
        class="flex items-center justify-end rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600"
      >
        <button
          type="button"
          class="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
          @click="clearAllLocal"
        >
          {{ t('admin.groups.clearAll') }}
        </button>
      </div>

      <!-- 已设置的用户列表 -->
      <div v-else>
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ isVideoPricingGroup ? t('admin.groups.videoPriceOverrides.configuredUsers') : t('admin.groups.rateMultipliers') }} ({{ localEntries.length }})
        </h4>

        <div v-if="localEntries.length === 0" class="py-6 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ isVideoPricingGroup ? t('admin.groups.videoPriceOverrides.empty') : t('admin.groups.noRateMultipliers') }}
        </div>

        <div v-else>
          <!-- 表格 -->
          <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <div class="max-h-[420px] overflow-auto">
              <table class="w-full min-w-max text-sm">
                <thead class="sticky top-0 z-[1]">
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-700">
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userEmail') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">ID</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userName') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userNotes') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userStatus') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.rateMultiplier') }}</th>
                    <th v-if="showFinalRate" class="px-3 py-2 text-left text-xs font-medium text-primary-600 dark:text-primary-400">{{ t('admin.groups.finalRate') }}</th>
                    <th v-if="isVideoPricingGroup" class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.groups.videoPriceOverrides.column') }}
                    </th>
                    <th class="w-10 px-2 py-2"></th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                  <template v-for="entry in paginatedLocalEntries" :key="entry.user_id">
                  <tr class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ entry.user_email }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-400 dark:text-gray-500">{{ entry.user_id }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-900 dark:text-white">{{ entry.user_name || '-' }}</td>
                    <td class="max-w-[160px] truncate px-3 py-2 text-gray-500 dark:text-gray-400" :title="entry.user_notes">{{ entry.user_notes || '-' }}</td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <span
                        :class="[
                          'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
                          entry.user_status === 'active'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
                        ]"
                      >
                        {{ entry.user_status }}
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <input
                        type="number"
                        step="0.001"
                        min="0.001"
                        autocomplete="off"
                        :value="entry.rate_multiplier ?? ''"
                        :placeholder="String(props.group?.rate_multiplier ?? 1)"
                        class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm font-medium transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
                        @change="updateLocalRate(entry.user_id, ($event.target as HTMLInputElement).value)"
                      />
                    </td>
                    <td v-if="showFinalRate" class="whitespace-nowrap px-3 py-2 font-medium text-primary-600 dark:text-primary-400">
                      {{ computeFinalRate(entry.rate_multiplier) }}
                    </td>
                    <td v-if="isVideoPricingGroup" class="whitespace-nowrap px-3 py-2">
                      <button
                        type="button"
                        class="rounded-md border border-primary-200 bg-primary-50 px-2.5 py-1 text-xs font-semibold text-primary-700 transition-colors hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-900/30 dark:text-primary-300 dark:hover:bg-primary-900/50"
                        @click="toggleVideoPrices(entry.user_id)"
                      >
                        {{
                          expandedVideoPriceUserID === entry.user_id
                            ? t('admin.groups.videoPriceOverrides.collapse')
                            : t('admin.groups.videoPriceOverrides.configure')
                        }}
                        <span class="ml-1 font-normal opacity-80">
                          {{
                            countVideoPriceOverrides(entry) > 0
                              ? t('admin.groups.videoPriceOverrides.overrideCount', {
                                  count: countVideoPriceOverrides(entry),
                                })
                              : t('admin.groups.videoPriceOverrides.notSetYet')
                          }}
                        </span>
                      </button>
                    </td>
                    <td class="px-2 py-2">
                      <button
                        type="button"
                        class="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        @click="removeLocal(entry.user_id)"
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </td>
                  </tr>
                  <tr v-if="isVideoPricingGroup && expandedVideoPriceUserID === entry.user_id">
                    <td :colspan="showFinalRate ? 9 : 8" class="bg-gray-50 px-4 py-3 dark:bg-dark-800/70">
                      <div class="mb-3 flex flex-wrap items-start justify-between gap-2">
                        <div>
                          <div class="text-sm font-medium text-gray-800 dark:text-gray-200">
                            {{ t('admin.groups.videoPriceOverrides.userTitle', { user: entry.user_email }) }}
                          </div>
                          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                            {{
                              t('admin.groups.videoPriceOverrides.inheritHint', {
                                unit: videoPriceUnitLabel,
                              })
                            }}
                          </p>
                          <p class="mt-1 text-xs font-medium text-amber-600 dark:text-amber-400">
                            {{ t('admin.groups.videoPriceOverrides.fillThenSave') }}
                          </p>
                        </div>
                        <button
                          v-if="countVideoPriceOverrides(entry) > 0"
                          type="button"
                          class="rounded px-2 py-1 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                          @click="clearEntryVideoPrices(entry)"
                        >
                          {{ t('admin.groups.videoPriceOverrides.clearUser') }}
                        </button>
                      </div>
                      <div class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-600 dark:border-dark-600">
                        <div
                          v-for="model in availableVideoModels"
                          :key="model"
                          class="grid gap-3 py-3 lg:grid-cols-[minmax(180px,0.8fr)_minmax(0,2fr)] lg:items-end"
                        >
                          <div class="font-mono text-xs font-medium text-gray-700 dark:text-gray-300">
                            {{ model }}
                          </div>
                          <div class="grid gap-2" :style="videoPriceGridStyle(model)">
                            <label
                              v-for="resolution in supportedVideoResolutions(model)"
                              :key="resolution"
                              class="block"
                            >
                              <span class="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                                {{ resolution }} ({{ videoPriceUnitLabel }})
                              </span>
                              <input
                                type="number"
                                inputmode="decimal"
                                step="0.0001"
                                min="0"
                                autocomplete="off"
                                class="hide-spinner input w-full"
                                :value="getEntryVideoPrice(entry, model, resolution)"
                                :placeholder="groupVideoPricePlaceholder(model, resolution)"
                                @change="setEntryVideoPrice(entry, model, resolution, ($event.target as HTMLInputElement).value)"
                              />
                            </label>
                          </div>
                        </div>
                      </div>
                    </td>
                  </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </div>

          <!-- 分页 -->
          <Pagination
            :total="localEntries.length"
            :page="currentPage"
            :page-size="pageSize"
            @update:page="currentPage = $event"
            @update:pageSize="handlePageSizeChange"
          />
        </div>
      </div>

      <!-- 底部操作栏 -->
      <div class="flex items-center gap-3 border-t border-gray-200 pt-4 dark:border-dark-600">
        <!-- 左侧：未保存提示 + 撤销 -->
        <template v-if="isDirty">
          <span class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.groups.unsavedChanges') }}</span>
          <button
            type="button"
            class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            @click="handleCancel"
          >
            {{ t('admin.groups.revertChanges') }}
          </button>
        </template>
        <!-- 右侧：关闭 / 保存 -->
        <div class="ml-auto flex items-center gap-3">
          <button type="button" class="btn btn-sm px-4 py-1.5" @click="handleClose">
            {{ t('common.close') }}
          </button>
          <button
            v-if="isDirty"
            type="button"
            class="btn btn-primary btn-sm px-4 py-1.5"
            :disabled="saving"
            @click="handleSave"
          >
            <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </BaseDialog>

</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { GroupRateMultiplierEntry } from '@/api/admin/groups'
import type { AdminGroup, AdminUser, VideoModelPrices, VideoModelPrice } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import {
  normalizeVideoBillingUnitForPlatform,
  normalizeVideoModelPricesForPlatform,
  supportsVideoModelPricingPlatform,
  videoModelsForPricingPlatform,
  supportedResolutionsForVideoModel,
  type VideoModelPriceResolution,
} from '@/views/admin/groupsVideoModelPricing'

interface LocalEntry extends GroupRateMultiplierEntry {}

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const serverEntries = ref<GroupRateMultiplierEntry[]>([])
const localEntries = ref<LocalEntry[]>([])
const searchQuery = ref('')
const searchResults = ref<AdminUser[]>([])
const showDropdown = ref(false)
const selectedUser = ref<AdminUser | null>(null)
const newRate = ref<number | null>(null)
const currentPage = ref(1)
const pageSize = ref(10)
const batchFactor = ref<number | null>(null)
const expandedVideoPriceUserID = ref<number | null>(null)

let searchTimeout: ReturnType<typeof setTimeout>

const platformColorClass = computed(() => {
  switch (props.group?.platform) {
    case 'anthropic': return 'text-orange-700 dark:text-orange-400'
    case 'openai': return 'text-emerald-700 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-700 dark:text-purple-400'
    case 'seedance': return 'text-rose-700 dark:text-rose-400'
    case 'ltx': return 'text-cyan-700 dark:text-cyan-300'
    case 'happyhorse': return 'text-amber-700 dark:text-amber-300'
    case 'minimax': return 'text-fuchsia-700 dark:text-fuchsia-300'
    case 'grokimagine': return 'text-violet-700 dark:text-violet-300'
    default: return 'text-blue-700 dark:text-blue-400'
  }
})

const isVideoPricingGroup = computed(() =>
  supportsVideoModelPricingPlatform(props.group?.platform ?? '')
)

const videoBillingUnit = computed(() =>
  normalizeVideoBillingUnitForPlatform(
    props.group?.platform ?? '',
    props.group?.video_billing_unit,
  )
)

const videoPriceUnitLabel = computed(() =>
  t(
    `admin.groups.videoPricing.${
      videoBillingUnit.value === 'per_request'
        ? 'priceUnitPerRequest'
        : 'priceUnitPerSecond'
    }`,
  )
)

const videoPricePeriodLabel = computed(() =>
  t(
    `admin.groups.videoPricing.${
      videoBillingUnit.value === 'per_request'
        ? 'pricePeriodPerRequest'
        : 'pricePeriodPerSecond'
    }`,
  )
)

const groupVideoModelPrices = computed(() =>
  normalizeVideoModelPricesForPlatform(
    props.group?.platform ?? '',
    props.group?.video_model_prices,
  ),
)

const availableVideoModels = computed(() => [
  ...new Set([
    ...videoModelsForPricingPlatform(props.group?.platform ?? 'seedance'),
    ...Object.keys(groupVideoModelPrices.value),
    ...localEntries.value.flatMap(entry => Object.keys(entry.video_model_prices ?? {})),
  ]),
])

const groupDefaultPriceRows = computed(() => {
  return availableVideoModels.value
    .map((model) => {
      const card = groupVideoModelPrices.value[model] || {}
      const parts = supportedVideoResolutions(model)
        .map((resolution) => {
          const price = card[resolution as keyof VideoModelPrice]
          if (price == null) return null
          return `${resolution}=${price}`
        })
        .filter(Boolean)
      if (parts.length === 0) return null
      return { model, pricesText: parts.join(' · ') }
    })
    .filter((row): row is { model: string; pricesText: string } => row != null)
})

const canAddUser = computed(() => {
  if (selectedUser.value) {
    return isVideoPricingGroup.value || !!newRate.value
  }
  if (searchResults.value.length === 1) {
    return isVideoPricingGroup.value || !!newRate.value
  }
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return false
  const exact = searchResults.value.some(
    (user) => user.email?.toLowerCase() === q || user.username?.toLowerCase() === q,
  )
  return exact && (isVideoPricingGroup.value || !!newRate.value)
})

const supportedVideoResolutions = (model: string): readonly VideoModelPriceResolution[] =>
  supportedResolutionsForVideoModel(props.group?.platform ?? 'seedance', model)

const videoPriceGridStyle = (model: string) => ({
  gridTemplateColumns: `repeat(${supportedVideoResolutions(model).length}, minmax(0, 1fr))`,
})

const cloneVideoModelPrices = (prices: VideoModelPrices | undefined): VideoModelPrices => {
  return normalizeVideoModelPricesForPlatform(props.group?.platform ?? '', prices)
}

const hasVideoPrices = (entry: GroupRateMultiplierEntry): boolean =>
  Object.values(entry.video_model_prices ?? {}).some((price) =>
    Object.values(price ?? {}).some((value) => value != null),
  )

const videoPricesFingerprint = (prices: VideoModelPrices | undefined): string =>
  JSON.stringify(
    Object.keys(prices ?? {})
      .sort()
      .reduce<Record<string, VideoModelPrice>>((result, model) => {
        const price = prices?.[model] ?? {}
        result[model] = Object.keys(price)
          .sort()
          .reduce<VideoModelPrice>((card, resolution) => {
            const value = price[resolution as keyof VideoModelPrice]
            if (value != null) card[resolution as keyof VideoModelPrice] = value
            return card
          }, {})
        return result
      }, {}),
  )

// 是否显示"最终倍率"预览列
const showFinalRate = computed(() => {
  return batchFactor.value != null && batchFactor.value > 0 && batchFactor.value !== 1
})

// 计算最终倍率预览
const computeFinalRate = (rate: number | null | undefined) => {
  const base = rate ?? props.group?.rate_multiplier ?? 1
  if (!batchFactor.value) return base
  return parseFloat((base * batchFactor.value).toFixed(6))
}

// 检测是否有未保存的修改
const isDirty = computed(() => {
  if (localEntries.value.length !== serverEntries.value.length) return true
  const serverMap = new Map(serverEntries.value.map(e => [e.user_id, e]))
  return localEntries.value.some((entry) => {
    const serverEntry = serverMap.get(entry.user_id)
    return !serverEntry
      || (serverEntry.rate_multiplier ?? null) !== (entry.rate_multiplier ?? null)
      || videoPricesFingerprint(serverEntry.video_model_prices) !== videoPricesFingerprint(entry.video_model_prices)
  })
})

const paginatedLocalEntries = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return localEntries.value.slice(start, start + pageSize.value)
})

const cloneEntries = (entries: GroupRateMultiplierEntry[]): LocalEntry[] => {
  return entries.map(e => ({
    ...e,
    video_model_prices: cloneVideoModelPrices(e.video_model_prices),
  }))
}

const loadEntries = async () => {
  if (!props.group) return
  loading.value = true
  try {
    const raw = await adminAPI.groups.getGroupRateMultipliers(props.group.id)
    // RPM 在另一个弹窗管理；视频分组还要保留只设置了专属视频单价的用户。
    serverEntries.value = cloneEntries(raw.filter(e =>
      e.rate_multiplier != null || (isVideoPricingGroup.value && hasVideoPrices(e)),
    ))
    localEntries.value = cloneEntries(serverEntries.value)
    // 视频分组打开时默认展开第一位用户的模型单价表，避免只看见倍率列。
    if (isVideoPricingGroup.value && localEntries.value.length > 0) {
      expandedVideoPriceUserID.value = localEntries.value[0].user_id
    } else {
      expandedVideoPriceUserID.value = null
    }
    adjustPage()
  } catch (error) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading group rate multipliers:', error)
  } finally {
    loading.value = false
  }
}

const adjustPage = () => {
  const totalPages = Math.max(1, Math.ceil(localEntries.value.length / pageSize.value))
  if (currentPage.value > totalPages) {
    currentPage.value = totalPages
  }
}

watch(() => props.show, (val) => {
  if (val && props.group) {
    currentPage.value = 1
    batchFactor.value = null
    searchQuery.value = ''
    searchResults.value = []
    selectedUser.value = null
    newRate.value = null
    expandedVideoPriceUserID.value = null
    loadEntries()
  }
})

const handlePageSizeChange = (newSize: number) => {
  pageSize.value = newSize
  currentPage.value = 1
}

const handleSearchUsers = () => {
  clearTimeout(searchTimeout)
  selectedUser.value = null
  if (!searchQuery.value.trim()) {
    searchResults.value = []
    showDropdown.value = false
    return
  }
  searchTimeout = setTimeout(async () => {
    try {
      const res = await adminAPI.users.list(1, 10, { search: searchQuery.value.trim() })
      searchResults.value = res.items
      showDropdown.value = true
    } catch {
      searchResults.value = []
    }
  }, 300)
}

const selectUser = (user: AdminUser) => {
  selectedUser.value = user
  searchQuery.value = user.email
  showDropdown.value = false
  searchResults.value = []
}

// 本地添加（或覆盖已有用户）
const resolveUserToAdd = (): AdminUser | null => {
  if (selectedUser.value) return selectedUser.value
  if (searchResults.value.length === 1) return searchResults.value[0]
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return null
  return (
    searchResults.value.find(
      (user) => user.email?.toLowerCase() === q || user.username?.toLowerCase() === q,
    ) || null
  )
}

const handleAddLocal = () => {
  const user = resolveUserToAdd()
  if (!user || (!isVideoPricingGroup.value && !newRate.value)) {
    if (isVideoPricingGroup.value) {
      appStore.showError(t('admin.groups.videoPriceOverrides.selectUserFirst'))
    }
    return
  }
  selectedUser.value = user
  const idx = localEntries.value.findIndex(e => e.user_id === user.id)
  const existing = idx >= 0 ? localEntries.value[idx] : null
  const entry: LocalEntry = {
    user_id: user.id,
    user_name: user.username || '',
    user_email: user.email,
    user_notes: user.notes || '',
    user_status: user.status || 'active',
    // 视频分组：只有显式填写才写倍率，默认专注模型单价覆盖。
    rate_multiplier: newRate.value ?? existing?.rate_multiplier ?? null,
    rpm_override: existing?.rpm_override ?? null,
    video_model_prices: cloneVideoModelPrices(existing?.video_model_prices),
  }
  if (idx >= 0) {
    localEntries.value[idx] = entry
  } else {
    localEntries.value.push(entry)
  }
  searchQuery.value = ''
  selectedUser.value = null
  newRate.value = null
  showDropdown.value = false
  searchResults.value = []
  if (isVideoPricingGroup.value) {
    expandedVideoPriceUserID.value = user.id
    const index = localEntries.value.findIndex(e => e.user_id === user.id)
    if (index >= 0) {
      currentPage.value = Math.floor(index / pageSize.value) + 1
    }
    appStore.showSuccess(t('admin.groups.videoPriceOverrides.addedConfigureHint'))
  }
  adjustPage()
}

// 本地修改倍率
const updateLocalRate = (userId: number, value: string) => {
  const entry = localEntries.value.find(e => e.user_id === userId)
  if (!entry) return
  if (value.trim() === '') {
    entry.rate_multiplier = null
    return
  }
  const num = parseFloat(value)
  if (isNaN(num)) return
  entry.rate_multiplier = num
}

// 本地删除
const removeLocal = (userId: number) => {
  localEntries.value = localEntries.value.filter(e => e.user_id !== userId)
  if (expandedVideoPriceUserID.value === userId) expandedVideoPriceUserID.value = null
  adjustPage()
}

const toggleVideoPrices = (userId: number) => {
  expandedVideoPriceUserID.value = expandedVideoPriceUserID.value === userId ? null : userId
}

const countVideoPriceOverrides = (entry: GroupRateMultiplierEntry): number =>
  Object.values(entry.video_model_prices ?? {}).reduce(
    (count, price) => count + Object.values(price ?? {}).filter(value => value != null).length,
    0,
  )

const getEntryVideoPrice = (
  entry: GroupRateMultiplierEntry,
  model: string,
  resolution: VideoModelPriceResolution,
): number | '' => entry.video_model_prices?.[model]?.[resolution] ?? ''

const groupVideoPricePlaceholder = (
  model: string,
  resolution: VideoModelPriceResolution,
): string => {
  const groupPrice = groupVideoModelPrices.value[model]?.[resolution]
  return groupPrice == null
    ? t('admin.groups.videoPriceOverrides.notConfigured')
    : t('admin.groups.videoPriceOverrides.inheritPrice', {
        price: groupPrice,
        unit: videoPricePeriodLabel.value,
      })
}

const setEntryVideoPrice = (
  entry: LocalEntry,
  model: string,
  resolution: VideoModelPriceResolution,
  rawValue: string,
) => {
  const value = rawValue.trim()
  entry.video_model_prices ??= {}
  entry.video_model_prices[model] ??= {}
  if (value === '') {
    delete entry.video_model_prices[model][resolution]
    if (Object.values(entry.video_model_prices[model]).every(item => item == null)) {
      delete entry.video_model_prices[model]
    }
    return
  }
  const price = Number(value)
  if (!Number.isFinite(price) || price < 0) {
    appStore.showError(t('admin.groups.videoPriceOverrides.invalidPrice'))
    return
  }
  entry.video_model_prices[model][resolution] = price
}

const clearEntryVideoPrices = (entry: LocalEntry) => {
  entry.video_model_prices = {}
}

// 批量乘数应用到本地
const applyBatchFactor = () => {
  if (!batchFactor.value || batchFactor.value <= 0) return
  for (const entry of localEntries.value) {
    if (entry.rate_multiplier != null) {
      entry.rate_multiplier = parseFloat((entry.rate_multiplier * batchFactor.value).toFixed(6))
    }
  }
  batchFactor.value = null
}

// 本地清空
const clearAllLocal = () => {
  localEntries.value = []
}

// 取消：恢复到服务器数据
const handleCancel = () => {
  localEntries.value = cloneEntries(serverEntries.value)
  batchFactor.value = null
  expandedVideoPriceUserID.value = null
  adjustPage()
}

// 保存倍率和专属视频单价；RPM override 由独立弹窗管理。
const handleSave = async () => {
  if (!props.group) return
  saving.value = true
  try {
    const entries = localEntries.value
      .filter(e => e.rate_multiplier != null)
      .map(e => ({
        user_id: e.user_id,
        rate_multiplier: e.rate_multiplier as number
      }))
    const requests: Promise<unknown>[] = [
      adminAPI.groups.batchSetGroupRateMultipliers(props.group.id, entries),
    ]
    if (isVideoPricingGroup.value) {
      const videoEntries = localEntries.value
        .filter(hasVideoPrices)
        .map(e => ({
          user_id: e.user_id,
          video_model_prices: cloneVideoModelPrices(e.video_model_prices),
        }))
      requests.push(adminAPI.groups.batchSetGroupVideoModelPrices(props.group.id, videoEntries))
    }
    await Promise.all(requests)
    appStore.showSuccess(t(isVideoPricingGroup.value ? 'admin.groups.userPricingSaved' : 'admin.groups.rateSaved'))
    emit('success')
    emit('close')
  } catch (error) {
    appStore.showError(t('admin.groups.failedToSave'))
    console.error('Error saving per-user billing:', error)
  } finally {
    saving.value = false
  }
}

// 关闭时如果有未保存修改，先恢复
const handleClose = () => {
  if (isDirty.value) {
    localEntries.value = cloneEntries(serverEntries.value)
  }
  expandedVideoPriceUserID.value = null
  emit('close')
}

// 点击外部关闭下拉
const handleClickOutside = () => {
  showDropdown.value = false
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', handleClickOutside)
}
</script>

<style scoped>
.hide-spinner::-webkit-outer-spin-button,
.hide-spinner::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.hide-spinner {
  -moz-appearance: textfield;
}
</style>
