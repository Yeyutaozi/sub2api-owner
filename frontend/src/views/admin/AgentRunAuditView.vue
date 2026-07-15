<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <Select
              v-model="filters.app_id"
              :options="appFilterOptions"
              searchable
              class="w-full sm:w-64"
              @change="handleFilterChange"
            />
            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              class="w-full sm:w-44"
              @change="handleFilterChange"
            />
          </div>

          <button class="btn btn-secondary" :disabled="loading" title="刷新" @click="loadRuns">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="runs" :loading="loading">
          <template #cell-id="{ row }">
            <span class="font-medium text-gray-900 dark:text-white">#{{ row.id }}</span>
          </template>

          <template #cell-app_id="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ appName(row.app_id) }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">应用 #{{ row.app_id }} · 版本 #{{ row.app_version_id }}</span>
            </div>
          </template>

          <template #cell-user_id="{ row }">
            <div class="flex flex-col text-sm text-gray-700 dark:text-gray-300">
              <span>用户 #{{ row.user_id }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">API Key #{{ row.api_key_id }}</span>
            </div>
          </template>

          <template #cell-worker_host_id="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ row.worker_host_id ? `#${row.worker_host_id}` : '-' }}
            </span>
          </template>

          <template #cell-status="{ row }">
            <span :class="['badge', statusBadgeClass(row.status)]">{{ statusLabel(row.status) }}</span>
          </template>

          <template #cell-duration_ms="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDurationMs(row.duration_ms) }}</span>
          </template>

          <template #cell-created_at="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>

          <template #empty>
            <EmptyState title="暂无使用记录" description="当前筛选条件下没有应用运行记录" />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { AgentApp } from '@/types'
import type { Column } from '@/components/common/types'
import type { AdminAgentRunAudit } from '@/api/admin/agentRuns'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import agentAppsAPI from '@/api/admin/agentApps'
import agentRunsAPI from '@/api/admin/agentRuns'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const loading = ref(false)
const runs = ref<AdminAgentRunAudit[]>([])
const apps = ref<AgentApp[]>([])
const filters = reactive<{ app_id: number | ''; status: string }>({
  app_id: '',
  status: ''
})
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1
})

const columns: Column[] = [
  { key: 'id', label: '运行编号' },
  { key: 'app_id', label: '应用' },
  { key: 'user_id', label: '调用用户' },
  { key: 'worker_host_id', label: 'Worker' },
  { key: 'status', label: '状态' },
  { key: 'duration_ms', label: '耗时' },
  { key: 'created_at', label: '提交时间' }
]

const statusFilterOptions = [
  { label: '全部运行状态', value: '' },
  { label: '排队中', value: 'queued' },
  { label: '运行中', value: 'running' },
  { label: '成功', value: 'succeeded' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
  { label: '超时', value: 'timeout' }
]

const appFilterOptions = computed(() => [
  { label: '全部应用', value: '' },
  ...apps.value.map(app => ({ label: `${app.name} (#${app.id})`, value: app.id }))
])

const appNameMap = computed(() => new Map(apps.value.map(app => [app.id, app.name])))

async function loadApps() {
  try {
    const firstPage = await agentAppsAPI.list(1, 100, { sort_by: 'id', sort_order: 'desc' })
    if (firstPage.pages <= 1) {
      apps.value = firstPage.items
      return
    }
    const remainingPages = await Promise.all(
      Array.from({ length: firstPage.pages - 1 }, (_, index) =>
        agentAppsAPI.list(index + 2, 100, { sort_by: 'id', sort_order: 'desc' })
      )
    )
    apps.value = [firstPage, ...remainingPages].flatMap(result => result.items)
  } catch {
    apps.value = []
  }
}

async function loadRuns() {
  loading.value = true
  try {
    const result = await agentRunsAPI.list(pagination.page, pagination.page_size, {
      app_id: typeof filters.app_id === 'number' ? filters.app_id : undefined,
      status: filters.status || undefined,
      sort_by: 'created_at',
      sort_order: 'desc'
    })
    runs.value = result.items
    Object.assign(pagination, {
      page: result.page,
      page_size: result.page_size,
      total: result.total,
      pages: result.pages
    })
  } catch (error: any) {
    runs.value = []
    appStore.showError(error?.message || '加载应用使用记录失败')
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  pagination.page = 1
  void loadRuns()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadRuns()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadRuns()
}

function appName(appID: number) {
  return appNameMap.value.get(appID) || `应用 #${appID}`
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    queued: '排队中',
    running: '运行中',
    succeeded: '成功',
    failed: '失败',
    canceled: '已取消',
    timeout: '超时'
  }
  return labels[status] || status
}

function statusBadgeClass(status: string) {
  if (status === 'succeeded') return 'badge-success'
  if (status === 'failed' || status === 'timeout') return 'badge-danger'
  if (status === 'running') return 'badge-primary'
  if (status === 'queued') return 'badge-warning'
  return 'badge-gray'
}

function formatDurationMs(value?: number) {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value < 1000) return `${Math.max(0, Math.round(value))} ms`
  if (value < 60_000) return `${(value / 1000).toFixed(1)} 秒`
  return `${Math.floor(value / 60_000)} 分 ${Math.round((value % 60_000) / 1000)} 秒`
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  void Promise.all([loadApps(), loadRuns()])
})
</script>
