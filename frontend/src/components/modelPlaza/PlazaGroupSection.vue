<template>
  <section
    class="desk-panel overflow-hidden"
    :class="[platformBorderStrongClass(group.platform)]"
  >
    <!-- 分组头部:名称/平台/倍率徽章/专属/订阅徽章 + 描述 -->
    <header class="desk-panel-head !items-start px-5 py-4">
      <div class="flex flex-wrap items-center gap-2">
        <GroupBadge
          :name="group.name"
          :platform="group.platform as GroupPlatform"
          :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
          :rate-multiplier="group.rate_multiplier"
          :user-rate-multiplier="group.user_rate_multiplier ?? null"
          :peak-rate-enabled="group.peak_rate_enabled"
          :peak-start="group.peak_start"
          :peak-end="group.peak_end"
          :peak-rate-multiplier="group.peak_rate_multiplier"
          always-show-rate
        />
        <span
          v-if="group.is_exclusive"
          class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
        >
          <Icon name="shield" size="xs" class="h-3 w-3" />
          {{ t('modelPlaza.badges.exclusive') }}
        </span>
        <span
          v-if="group.subscription_type === 'subscription'"
          class="inline-flex items-center rounded-md bg-signal-50 px-2 py-0.5 text-xs font-medium text-signal-700 dark:bg-signal-900/20 dark:text-signal-300"
        >
          {{ t('modelPlaza.badges.subscription') }}
        </span>
      </div>
      <p v-if="group.description" class="mt-2 text-sm text-gray-500 dark:text-dark-400">
        {{ group.description }}
      </p>
      <p
        v-if="peakNote"
        class="mt-1.5 inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400"
      >
        <Icon name="clock" size="xs" class="h-3 w-3" />
        {{ peakNote }}
      </p>
      <p
        class="mp-group-ttft mt-2 inline-flex flex-wrap items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs"
        :data-tone="ttftTone"
        :title="ttftTitle"
        data-testid="plaza-group-ttft"
      >
        <Icon name="bolt" size="xs" class="h-3.5 w-3.5" />
        <span class="font-semibold">{{ t('modelPlaza.detail.avgFirstToken') }}</span>
        <span class="mp-group-ttft__value font-mono tabular-nums font-extrabold tracking-tight">{{ formattedFirstToken }}</span>
        <span v-if="ttftTone !== 'unknown'" class="mp-group-ttft__grade">{{ ttftGradeLabel }}</span>
        <span class="mp-group-ttft__note">{{ group.ttft_disclaimer || t('modelPlaza.detail.ttftDisclaimer') }}</span>
      </p>
    </header>

    <!-- 模型价格表 -->
    <div class="px-5">
      <PlazaModelPricingTable
        v-if="group.models.length > 0"
        :models="group.models"
        :platform="group.platform"
        :rate-multiplier="group.rate_multiplier"
        :user-rate-multiplier="group.user_rate_multiplier ?? null"
      />
      <p v-else class="py-4 text-center text-sm text-gray-400 dark:text-dark-500">
        {{ t('modelPlaza.detail.noModels') }}
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { platformBorderStrongClass } from '@/utils/platformColors'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { firstTokenSeverity, type LatencySeverity } from '@/utils/latencyHealth'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  group: ModelPlazaGroup
}>()

const { t } = useI18n()
const appStore = useAppStore()

const peakNote = computed(() => {
  if (!hasPeakRate(props.group)) return ''
  const window = formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
  return t('modelPlaza.detail.peakNote', {
    window,
    multiplier: props.group.peak_rate_multiplier
  })
})

const formattedFirstToken = computed(() => {
  const ms = Number(props.group.avg_first_token_ms || 0)
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  if (ms >= 1000) return (ms / 1000).toFixed(1) + 's'
  return Math.round(ms) + 'ms'
})

type TtftTone = LatencySeverity | 'unknown'

const ttftTone = computed<TtftTone>(() => {
  const ms = Number(props.group.avg_first_token_ms || 0)
  if (!Number.isFinite(ms) || ms <= 0) return 'unknown'
  return firstTokenSeverity(ms)
})

const ttftGradeLabel = computed(() => {
  if (ttftTone.value === 'unknown') return ''
  return t(`modelPlaza.detail.ttftGrade.${ttftTone.value}`)
})

const ttftTitle = computed(() => {
  const base = props.group.ttft_disclaimer || t('modelPlaza.detail.ttftDisclaimer')
  if (ttftTone.value === 'unknown') return base
  return `${t('modelPlaza.detail.avgFirstToken')}: ${formattedFirstToken.value} (${ttftGradeLabel.value}) · ${base}`
})
</script>

<style scoped>
.mp-group-ttft {
  border-color: #cbd5e1;
  background: #f8fafc;
  color: #475569;
}
.mp-group-ttft__value {
  font-size: 13px;
}
.mp-group-ttft__grade {
  font-size: 10px;
  font-weight: 800;
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 12%, transparent);
}
.mp-group-ttft__note {
  font-size: 11px;
  opacity: 0.72;
}
.mp-group-ttft[data-tone='good'] {
  color: #047857;
  border-color: #6ee7b7;
  background: linear-gradient(180deg, #ecfdf5, #d1fae5);
}
.mp-group-ttft[data-tone='warn'] {
  color: #b45309;
  border-color: #fcd34d;
  background: linear-gradient(180deg, #fffbeb, #fef3c7);
}
.mp-group-ttft[data-tone='slow'] {
  color: #c2410c;
  border-color: #fdba74;
  background: linear-gradient(180deg, #fff7ed, #ffedd5);
}
.mp-group-ttft[data-tone='critical'] {
  color: #b91c1c;
  border-color: #fca5a5;
  background: linear-gradient(180deg, #fef2f2, #fee2e2);
}
.mp-group-ttft[data-tone='unknown'] {
  color: #64748b;
  border-color: #e2e8f0;
  background: #f8fafc;
}
:global(.dark) .mp-group-ttft[data-tone='good'] {
  color: #6ee7b7;
  border-color: rgba(52, 211, 153, 0.45);
  background: linear-gradient(180deg, rgba(6, 78, 59, 0.45), rgba(6, 95, 70, 0.22));
}
:global(.dark) .mp-group-ttft[data-tone='warn'] {
  color: #fbbf24;
  border-color: rgba(251, 191, 36, 0.4);
  background: linear-gradient(180deg, rgba(120, 53, 15, 0.4), rgba(146, 64, 14, 0.2));
}
:global(.dark) .mp-group-ttft[data-tone='slow'] {
  color: #fb923c;
  border-color: rgba(251, 146, 60, 0.4);
  background: linear-gradient(180deg, rgba(124, 45, 18, 0.4), rgba(154, 52, 18, 0.2));
}
:global(.dark) .mp-group-ttft[data-tone='critical'] {
  color: #fca5a5;
  border-color: rgba(248, 113, 113, 0.45);
  background: linear-gradient(180deg, rgba(127, 29, 29, 0.45), rgba(153, 27, 27, 0.22));
}
:global(.dark) .mp-group-ttft[data-tone='unknown'] {
  color: #94a3b8;
  border-color: #334155;
  background: #0f172a;
}
</style>
