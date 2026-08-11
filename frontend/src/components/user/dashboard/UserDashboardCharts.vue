<template>
  <div class="space-y-6">
    <!-- Date Range Filter -->
    <div class="instrument-panel">
      <div class="instrument-panel__body">
      <p class="instrument-panel__title">RANGE · GRANULARITY</p>
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('dashboard.timeRange') }}:</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
          {{ t('common.refresh') }}
        </button>
        <div class="ml-auto flex items-center gap-2">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('dashboard.granularity') }}:</span>
          <div class="w-28">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
      </div>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="desk-chart-grid grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Model Distribution Chart -->
      <div class="instrument-panel chart-desk relative overflow-hidden">
        <div class="instrument-panel__body p-4">
        <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
          <LoadingSpinner size="md" />
        </div>
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.modelDistribution') }}</h3>
        <div class="flex flex-col items-center gap-4 sm:flex-row sm:items-center sm:gap-6">
          <div class="chart-canvas-box h-48 w-48 shrink-0">
            <Doughnut v-if="modelData" :data="modelData" :options="doughnutOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('dashboard.noDataAvailable') }}</div>
          </div>
          <div class="chart-legend-scroll max-h-48 w-full min-w-0 flex-1 overflow-auto">
            <table class="chart-legend-table w-full text-xs">
              <thead>
                <tr class="text-gray-500 dark:text-gray-400">
                  <th class="pb-2 pr-3 text-left whitespace-nowrap">{{ t('dashboard.model') }}</th>
                  <th class="pb-2 pl-3 text-right whitespace-nowrap">{{ t('dashboard.requests') }}</th>
                  <th class="pb-2 pl-3 text-right whitespace-nowrap">{{ t('dashboard.tokens') }}</th>
                  <th class="pb-2 pl-3 text-right whitespace-nowrap">{{ t('dashboard.actual') }}</th>
                  <th class="pb-2 pl-3 text-right whitespace-nowrap">{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="model in models" :key="model.model" class="border-t border-gray-100 dark:border-dark-700">
                  <td class="max-w-[120px] truncate py-1.5 pr-3 font-medium text-gray-900 dark:text-white" :title="model.model">{{ model.model }}</td>
                  <td class="py-1.5 pl-3 text-right whitespace-nowrap text-gray-600 dark:text-gray-400">{{ formatNumber(model.requests) }}</td>
                  <td class="py-1.5 pl-3 text-right whitespace-nowrap text-gray-600 dark:text-gray-400">{{ formatTokens(model.total_tokens) }}</td>
                  <td class="py-1.5 pl-3 text-right whitespace-nowrap text-green-600 dark:text-green-400">${{ formatCost(model.actual_cost) }}</td>
                  <td class="py-1.5 pl-3 text-right whitespace-nowrap text-gray-400 dark:text-gray-500">${{ formatCost(model.cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        </div>
      </div>

      <!-- Token Usage Trend Chart -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import { Doughnut } from 'vue-chartjs'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler } from 'chart.js'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler)

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()

const toPieNumber = (value: unknown): number => {
  const n = Number(value)
  return Number.isFinite(n) && n > 0 ? n : 0
}

// Prefer tokens for pie slices; when all tokens are 0 (e.g. pure image/video jobs), fall back to request counts.
const pieValues = (models: ModelStat[]): number[] => {
  const tokenVals = models.map((m) => toPieNumber(m.total_tokens))
  if (tokenVals.some((v) => v > 0)) return tokenVals
  return models.map((m) => toPieNumber(m.requests))
}

const modelData = computed(() => {
  if (!props.models?.length) return null
  const data = pieValues(props.models)
  if (!data.some((v) => v > 0)) return null
  return {
    labels: props.models.map((m: ModelStat) => m.model),
    datasets: [{
      data,
      backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'],
      borderWidth: 0
    }]
  }
})

const doughnutOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const value = Number(context.parsed) || 0
          const total = (context.dataset.data as number[]).reduce((a, b) => a + (Number(b) || 0), 0)
          const pct = total > 0 ? ((value / total) * 100).toFixed(1) : '0.0'
          return `${context.label}: ${formatTokens(value)} (${pct}%)`
        }
      }
    }
  }
}
</script>
