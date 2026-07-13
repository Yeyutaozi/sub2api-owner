<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-72">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
              <input
                v-model="searchQuery"
                type="text"
                placeholder="搜索 Worker Host"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
            <Select v-model="filters.status" :options="statusFilterOptions" class="w-full sm:w-40" @change="loadHosts" />
          </div>

          <div class="flex flex-wrap items-center justify-end gap-2">
            <button class="btn btn-secondary" :disabled="loading" title="刷新" @click="loadHosts">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreateDialog">
              <Icon name="plus" size="md" class="mr-2" />
              新建 Worker Host
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="hosts"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="id"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ row.protocol }}</span>
            </div>
          </template>

          <template #cell-base_url="{ row }">
            <code class="code text-xs">{{ row.base_url }}</code>
          </template>

          <template #cell-route="{ row }">
            <div class="flex flex-col gap-1 text-xs">
              <span><span class="text-gray-400">运行</span> <code class="code">{{ row.run_path }}</code></span>
              <span><span class="text-gray-400">健康</span> <code class="code">{{ row.health_path }}</code></span>
            </div>
          </template>

          <template #cell-status="{ row }">
            <div class="flex flex-col gap-1">
              <span :class="['badge', statusBadgeClass(row.status)]">{{ statusLabel(row.status) }}</span>
              <span :class="['badge', healthBadgeClass(row.last_health_status)]">{{ healthLabel(row.last_health_status) }}</span>
            </div>
          </template>

          <template #cell-limits="{ row }">
            <div class="flex flex-col text-xs text-gray-600 dark:text-gray-300">
              <span>并发 {{ row.max_concurrency }}</span>
              <span>超时 {{ row.timeout_seconds }}s</span>
            </div>
          </template>

          <template #cell-last_checked_at="{ row }">
            <div class="flex flex-col text-xs">
              <span class="text-gray-700 dark:text-gray-200">{{ row.last_checked_at ? formatDateTime(row.last_checked_at) : '-' }}</span>
              <span v-if="row.last_health_latency_ms" class="text-gray-500">{{ row.last_health_latency_ms }}ms</span>
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-emerald-50 hover:text-emerald-600 disabled:opacity-50 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400"
                :disabled="checkingIds.has(row.id)"
                @click="handleHealthCheck(row)"
              >
                <Icon name="checkCircle" size="sm" :class="checkingIds.has(row.id) ? 'animate-spin' : ''" />
                <span class="text-xs">检查</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                @click="openEditDialog(row)"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">编辑</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                @click="confirmDelete(row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">删除</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              title="暂无 Worker Host"
              description="先登记已部署的 Worker 服务，再在应用版本中绑定具体运行路径"
              action-text="新建 Worker Host"
              @action="openCreateDialog"
            />
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

    <BaseDialog :show="showDialog" :title="editingHost ? '编辑 Worker Host' : '新建 Worker Host'" width="wide" @close="closeDialog">
      <form id="worker-host-form" class="space-y-4" @submit.prevent="handleSubmit">
        <div class="rounded-lg border border-primary-100 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-900/50 dark:bg-primary-900/20 dark:text-primary-200">
          <div class="font-medium">这里只登记 Worker 服务地址，不配置用户模型 API Key</div>
          <p class="mt-1 text-xs leading-5">
            用户模型 API Key 会在用户运行应用时从平台内已有 Key 中选择；Worker 只能通过 Sub2API Model Proxy 调模型，不会拿到用户 Key 明文。
          </p>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">Worker 名称 <span class="text-red-500">*</span></label>
            <input v-model="form.name" class="input" required placeholder="local-worker / image-worker-hk-1" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">仅用于后台识别这台 Worker 服务。</p>
          </div>
          <div>
            <label class="input-label">状态</label>
            <Select v-model="form.status" :options="statusEditOptions" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">禁用后不会再把新任务派发到这台 Worker。</p>
          </div>
        </div>

        <div>
          <label class="input-label">Worker 服务地址 <span class="text-red-500">*</span></label>
          <input v-model="form.base_url" class="input" required placeholder="http://127.0.0.1:8091 或 http://Worker服务器IP:8091" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">填 Worker 服务的根地址，不要填具体应用路径。具体应用路径在应用版本里配置。</p>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label class="input-label">Worker 协议版本</label>
            <input v-model="form.protocol" class="input" placeholder="sub2api-worker-v1" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">默认保持不变。</p>
          </div>
          <div>
            <label class="input-label">服务鉴权方式</label>
            <Select v-model="form.auth_type" :options="authTypeOptions" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Sub2API 会用每次运行的 Run Token 自动签名请求 Worker。</p>
          </div>
          <div>
            <label class="input-label">模型调用凭证</label>
            <div class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">
              Worker 不接收用户 API Key；用户运行应用时选择平台内 Key，模型调用统一走 Sub2API Model Proxy。
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label class="input-label">健康检查路径（Host 级）</label>
            <input v-model="form.health_path" class="input" placeholder="/health" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">健康检查会请求：服务地址 + 该路径。</p>
          </div>
          <div>
            <label class="input-label">默认运行路径（兜底）</label>
            <input v-model="form.run_path" class="input" placeholder="/runs" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">应用版本通常会单独配置运行路径。</p>
          </div>
          <div>
            <label class="input-label">取消路径（可选）</label>
            <input v-model="form.cancel_path" class="input" placeholder="/cancel" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Worker 支持取消任务时再填写。</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">最大并发任务数</label>
            <input v-model.number="form.max_concurrency" type="number" min="1" class="input" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">限制 Sub2API 同时派给这台 Worker 的任务数量。</p>
          </div>
          <div>
            <label class="input-label">任务超时（秒）</label>
            <input v-model.number="form.timeout_seconds" type="number" min="1" class="input" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">超过该时间未完成，运行会被标记为超时。</p>
          </div>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeDialog">取消</button>
        <button type="submit" form="worker-host-form" class="btn btn-primary" :disabled="submitting">
          {{ submitting ? '保存中...' : '保存' }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="deleteTarget !== null"
      title="删除 Worker Host"
      :message="deleteTarget ? `确认删除 ${deleteTarget.name}？已绑定的应用版本不会被自动改写。` : ''"
      type="danger"
      confirm-text="删除"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { Column } from '@/components/common/types'
import type { AgentWorkerHost, CreateAgentWorkerHostRequest, PaginatedResponse } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import agentWorkerHostsAPI from '@/api/admin/agentWorkerHosts'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const toast = {
  success: (message: string) => appStore.showSuccess(message),
  error: (message: string) => appStore.showError(message),
  warning: (message: string) => appStore.showWarning(message)
}

const loading = ref(false)
const submitting = ref(false)
const hosts = ref<AgentWorkerHost[]>([])
const checkingIds = ref<Set<number>>(new Set())
const searchQuery = ref('')
const filters = reactive({
  status: ''
})
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1
})
const sortState = reactive({
  sort_by: 'id',
  sort_order: 'desc' as 'asc' | 'desc'
})

const showDialog = ref(false)
const editingHost = ref<AgentWorkerHost | null>(null)
const deleteTarget = ref<AgentWorkerHost | null>(null)

const form = reactive<CreateAgentWorkerHostRequest>({
  name: '',
  base_url: '',
  protocol: 'sub2api-worker-v1',
  auth_type: 'hmac_run_token',
  secret_ref: '',
  health_path: '/health',
  run_path: '/runs',
  cancel_path: '',
  max_concurrency: 1,
  timeout_seconds: 600,
  status: 'active'
})

const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
  { label: '异常', value: 'unhealthy' }
]

const statusEditOptions = [
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
  { label: '异常', value: 'unhealthy' }
]

const authTypeOptions = [
  { label: 'HMAC + Run Token', value: 'hmac_run_token' }
]

const columns: Column[] = [
  { key: 'name', label: '名称', sortable: true },
  { key: 'base_url', label: '服务地址' },
  { key: 'route', label: '路径' },
  { key: 'status', label: '状态', sortable: true },
  { key: 'limits', label: '并发/超时' },
  { key: 'last_checked_at', label: '最近检查', sortable: true },
  { key: 'actions', label: '操作' }
]

let searchTimer: number | null = null

async function loadHosts() {
  loading.value = true
  try {
    const data: PaginatedResponse<AgentWorkerHost> = await agentWorkerHostsAPI.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      }
    )
    hosts.value = data.items
    pagination.total = data.total
    pagination.page = data.page
    pagination.page_size = data.page_size
    pagination.pages = data.pages
  } catch (error: any) {
    toast.error(error?.message || '加载 Worker Host 失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  if (searchTimer) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    pagination.page = 1
    loadHosts()
  }, 300)
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  loadHosts()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadHosts()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadHosts()
}

function resetForm() {
  Object.assign(form, {
    name: '',
    base_url: '',
    protocol: 'sub2api-worker-v1',
    auth_type: 'hmac_run_token',
    secret_ref: '',
    health_path: '/health',
    run_path: '/runs',
    cancel_path: '',
    max_concurrency: 1,
    timeout_seconds: 600,
    status: 'active'
  })
}

function openCreateDialog() {
  editingHost.value = null
  resetForm()
  showDialog.value = true
}

function openEditDialog(host: AgentWorkerHost) {
  editingHost.value = host
  Object.assign(form, {
    name: host.name,
    base_url: host.base_url,
    protocol: host.protocol,
    auth_type: host.auth_type,
    secret_ref: host.secret_ref || '',
    health_path: host.health_path,
    run_path: host.run_path,
    cancel_path: host.cancel_path || '',
    max_concurrency: host.max_concurrency,
    timeout_seconds: host.timeout_seconds,
    status: host.status
  })
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingHost.value = null
}

async function handleSubmit() {
  submitting.value = true
  try {
    const payload = { ...form }
    if (editingHost.value) {
      await agentWorkerHostsAPI.update(editingHost.value.id, payload)
      toast.success('Worker Host 已更新')
    } else {
      await agentWorkerHostsAPI.create(payload)
      toast.success('Worker Host 已创建')
    }
    closeDialog()
    await loadHosts()
  } catch (error: any) {
    toast.error(error?.message || '保存 Worker Host 失败')
  } finally {
    submitting.value = false
  }
}

async function handleHealthCheck(host: AgentWorkerHost) {
  checkingIds.value.add(host.id)
  checkingIds.value = new Set(checkingIds.value)
  try {
    const result = await agentWorkerHostsAPI.healthCheck(host.id)
    toast[result.success ? 'success' : 'warning'](result.message || (result.success ? '健康检查通过' : '健康检查失败'))
    await loadHosts()
  } catch (error: any) {
    toast.error(error?.message || '健康检查失败')
  } finally {
    checkingIds.value.delete(host.id)
    checkingIds.value = new Set(checkingIds.value)
  }
}

function confirmDelete(host: AgentWorkerHost) {
  deleteTarget.value = host
}

async function handleDelete() {
  if (!deleteTarget.value) return
  try {
    await agentWorkerHostsAPI.delete(deleteTarget.value.id)
    toast.success('Worker Host 已删除')
    deleteTarget.value = null
    await loadHosts()
  } catch (error: any) {
    toast.error(error?.message || '删除 Worker Host 失败')
  }
}

function statusLabel(status: string) {
  return status === 'active' ? '启用' : status === 'disabled' ? '禁用' : status === 'unhealthy' ? '异常' : status
}

function statusBadgeClass(status: string) {
  return status === 'active' ? 'badge-success' : status === 'disabled' ? 'badge-gray' : 'badge-danger'
}

function healthLabel(status: string) {
  return status === 'healthy' ? '健康' : status === 'unhealthy' ? '不健康' : '未检查'
}

function healthBadgeClass(status: string) {
  return status === 'healthy' ? 'badge-success' : status === 'unhealthy' ? 'badge-danger' : 'badge-gray'
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(loadHosts)
</script>
