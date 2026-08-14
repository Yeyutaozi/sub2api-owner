<template>
  <AppLayout>
    <TablePageLayout>
      <template #intro>
        <div class="flex w-full flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="desk-board__intro-copy">
            <p class="page-kicker">GENERATION JOBS</p>
            <h2 class="desk-board__title">{{ t('admin.imageJobs.title') }}</h2>
            <p class="desk-board__sub">{{ t('admin.imageJobs.description') }}</p>
          </div>
          <nav class="inline-flex w-fit border-b border-gray-200 dark:border-dark-600" :aria-label="t('admin.generationJobs.title')">
            <RouterLink to="/admin/video-jobs" class="task-tab">{{ t('admin.generationJobs.video') }}</RouterLink>
            <RouterLink to="/admin/image-jobs" class="task-tab task-tab--active">{{ t('admin.generationJobs.image') }}</RouterLink>
          </nav>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="min-w-56 flex-1 sm:max-w-80">
            <input
              v-model="filters.search"
              type="text"
              class="input"
              :placeholder="t('admin.imageJobs.searchPlaceholder')"
              @keyup.enter="handleFilterChange"
            />
          </div>
          <Select v-model="filters.status" :options="statusOptions" class="w-full sm:w-40" @change="handleFilterChange" />
          <Select v-model="filters.gateway_type" :options="gatewayOptions" class="w-full sm:w-44" @change="handleFilterChange" />
          <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              v-model="filters.active_only"
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="handleFilterChange"
            />
            {{ t('admin.imageJobs.filters.activeOnly') }}
          </label>
          <div class="ml-auto flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="loading" :title="t('admin.imageJobs.actions.refresh')" @click="loadJobs">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" :disabled="loading" @click="handleFilterChange">
              {{ t('admin.imageJobs.actions.query') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="jobs" :loading="loading" row-key="id">
          <template #cell-task_id="{ row }">
            <button class="block max-w-48 truncate text-left font-mono text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" @click="openDetail(row)">
              {{ row.task_id }}
            </button>
          </template>

          <template #cell-userGroup="{ row }">
            <div class="flex min-w-0 flex-col text-sm text-gray-700 dark:text-gray-300">
              <span class="truncate font-medium">{{ row.username || row.user_email || ('#' + row.user_id) }}</span>
              <span v-if="row.username && row.user_email" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ row.user_email }}</span>
              <span class="truncate text-xs text-gray-500 dark:text-gray-400">
                {{ row.group_name || '-' }} · {{ row.api_key_name || ('key #' + row.api_key_id) }}
              </span>
            </div>
          </template>

          <template #cell-model="{ row }">
            <div class="flex min-w-0 flex-col text-sm">
              <span class="truncate font-medium text-gray-900 dark:text-white">{{ row.model || '-' }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ gatewayLabel(row.gateway_type) }}</span>
            </div>
          </template>

          <template #cell-status="{ row }">
            <span :class="['badge', statusBadgeClass(row.status)]">{{ statusLabel(row.status) }}</span>
          </template>

          <template #cell-prompt="{ row }">
            <span class="line-clamp-2 max-w-xs text-sm text-gray-700 dark:text-gray-300" :title="row.prompt || ''">
              {{ row.prompt || '-' }}
            </span>
          </template>

          <template #cell-created_at="{ row }">
            <span class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex flex-wrap items-center gap-1">
              <button class="btn btn-secondary btn-sm" @click="openDetail(row)">{{ t('admin.imageJobs.actions.detail') }}</button>
              <button v-if="hasResult(row)" class="btn btn-secondary btn-sm" :disabled="previewLoadingId === row.id" @click="openPreview(row)">
                {{ t('admin.imageJobs.actions.preview') }}
              </button>
              <button v-if="row.can_terminate" class="btn btn-danger btn-sm" :disabled="actionLoadingId === row.id" @click="terminateTarget = row">
                {{ t('admin.imageJobs.actions.terminate') }}
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState :title="t('admin.imageJobs.empty.title')" :description="t('admin.imageJobs.empty.description')" />
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

    <BaseDialog :show="!!detail" :title="t('admin.imageJobs.detail.title')" width="extra-wide" @close="closeDetail">
      <div v-if="detail" class="space-y-5">
        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.basic') }}</h4>
          <div class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.taskId') }}:</span> <code class="break-all">{{ detail.task_id }}</code></div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.status') }}:</span> <span :class="['badge', statusBadgeClass(detail.status)]">{{ statusLabel(detail.status) }}</span></div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.user') }}:</span> {{ detail.username || detail.user_email || ('#' + detail.user_id) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.group') }}:</span> {{ detail.group_name || '-' }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.apiKey') }}:</span> {{ detail.api_key_name || ('#' + detail.api_key_id) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.model') }}:</span> {{ detail.model || '-' }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.gateway') }}:</span> {{ gatewayLabel(detail.gateway_type) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.terminationScope') }}:</span> {{ scopeLabel(detail.termination_scope) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.size') }}:</span> {{ paramSummary(detail) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.createdAt') }}:</span> {{ formatDateTime(detail.created_at) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.updatedAt') }}:</span> {{ formatDateTime(detail.updated_at) }}</div>
            <div><span class="text-gray-500">{{ t('admin.imageJobs.detail.fields.mimeType') }}:</span> {{ detail.mime_type || '-' }}</div>
          </div>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.prompt') }}</h4>
          <pre class="max-h-44 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-3 text-sm text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ detail.prompt || '-' }}</pre>
        </section>

        <section v-if="referenceImages(detail).length" class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.references') }}</h4>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <a v-for="(url, index) in referenceImages(detail)" :key="url + index" :href="url" target="_blank" rel="noopener noreferrer" class="block border border-gray-200 p-1 dark:border-dark-600">
              <img :src="url" alt="" class="aspect-square w-full object-cover" />
            </a>
          </div>
        </section>

        <section class="space-y-2">
          <div class="flex items-center justify-between gap-3">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.result') }}</h4>
            <button v-if="hasResult(detail) && !previewURL" class="btn btn-secondary btn-sm" :disabled="previewLoadingId === detail.id" @click="loadPreview(detail)">
              {{ t('admin.imageJobs.actions.loadPreview') }}
            </button>
          </div>
          <div v-if="previewLoadingId === detail.id" class="flex h-48 items-center justify-center text-sm text-gray-500">{{ t('admin.imageJobs.detail.loadingPreview') }}</div>
          <a v-else-if="previewURL" :href="previewURL" target="_blank" rel="noopener noreferrer" class="block w-fit max-w-full">
            <img :src="previewURL" alt="" class="max-h-[28rem] max-w-full border border-gray-200 object-contain dark:border-dark-600" />
          </a>
          <p v-else class="text-sm text-gray-500">{{ t('admin.imageJobs.detail.noResult') }}</p>
        </section>

        <section v-if="detail.error_message" class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.error') }}</h4>
          <pre class="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ detail.error_message }}</pre>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.imageJobs.detail.params') }}</h4>
          <pre class="max-h-64 overflow-auto rounded bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ prettyParams(detail) }}</pre>
        </section>
      </div>

      <template #footer>
        <div class="flex flex-wrap justify-end gap-2">
          <button class="btn btn-secondary" @click="closeDetail">{{ t('admin.imageJobs.actions.close') }}</button>
          <button v-if="detail?.can_terminate" class="btn btn-danger" :disabled="actionLoadingId === detail.id" @click="terminateTarget = detail">
            {{ t('admin.imageJobs.actions.terminate') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!terminateTarget"
      :title="t('admin.imageJobs.messages.terminateTitle')"
      :message="terminateMessage"
      danger
      @confirm="terminateJob"
      @cancel="terminateTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { imageJobsApi, type AdminImageJob } from '@/api/admin/imageJobs'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const actionLoadingId = ref<number | null>(null)
const previewLoadingId = ref<number | null>(null)
const jobs = ref<AdminImageJob[]>([])
const detail = ref<AdminImageJob | null>(null)
const terminateTarget = ref<AdminImageJob | null>(null)
const previewURL = ref('')

const filters = reactive({ search: '', status: '', gateway_type: '', active_only: false })
const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })

const columns = computed<Column[]>(() => [
  { key: 'task_id', label: t('admin.imageJobs.columns.taskId') },
  { key: 'userGroup', label: t('admin.imageJobs.columns.userGroup') },
  { key: 'model', label: t('admin.imageJobs.columns.model') },
  { key: 'status', label: t('admin.imageJobs.columns.status') },
  { key: 'prompt', label: t('admin.imageJobs.columns.prompt') },
  { key: 'created_at', label: t('admin.imageJobs.columns.createdAt') },
  { key: 'actions', label: t('admin.imageJobs.columns.actions') }
])

const statusOptions = computed(() => [
  { label: t('admin.imageJobs.filters.allStatus'), value: '' },
  ...['created', 'queued', 'running', 'succeeded', 'failed', 'canceled', 'expired'].map((value) => ({ label: statusLabel(value), value }))
])

const gatewayOptions = computed(() => [
  { label: t('admin.imageJobs.filters.allGateways'), value: '' },
  { label: t('admin.imageJobs.gateway.async'), value: 'image_task' },
  { label: t('admin.imageJobs.gateway.sync'), value: 'image_sync' }
])

const terminateMessage = computed(() => {
  if (terminateTarget.value?.termination_scope === 'async_execution') return t('admin.imageJobs.messages.terminateAsync')
  return t('admin.imageJobs.messages.terminateLocal')
})

async function loadJobs() {
  loading.value = true
  try {
    const result = await imageJobsApi.list({
      page: pagination.page,
      page_size: pagination.page_size,
      search: filters.search.trim() || undefined,
      status: filters.status || undefined,
      gateway_type: filters.gateway_type || undefined,
      active_only: filters.active_only || undefined
    })
    jobs.value = result?.items || []
    Object.assign(pagination, {
      page: result?.page || pagination.page,
      page_size: result?.page_size || pagination.page_size,
      total: result?.total || 0,
      pages: result?.pages || 1
    })
  } catch (error: any) {
    jobs.value = []
    appStore.showError(error?.message || t('admin.imageJobs.messages.loadFailed'))
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  pagination.page = 1
  void loadJobs()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadJobs()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadJobs()
}

async function openDetail(row: AdminImageJob) {
  clearPreview()
  detail.value = row
  try {
    const fresh = await imageJobsApi.get(row.id)
    detail.value = fresh
    replaceJob(fresh)
  } catch {
    // The list record still provides a useful audit view when refresh fails.
  }
}

async function openPreview(row: AdminImageJob) {
  await openDetail(row)
  if (detail.value) await loadPreview(detail.value)
}

async function loadPreview(row: AdminImageJob) {
  clearPreview()
  const directURL = directResultURL(row)
  if (directURL) {
    previewURL.value = directURL
    return
  }
  previewLoadingId.value = row.id
  try {
    previewURL.value = await imageJobsApi.getContentURL(row.id)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageJobs.messages.previewFailed'))
  } finally {
    previewLoadingId.value = null
  }
}

async function terminateJob() {
  const row = terminateTarget.value
  terminateTarget.value = null
  if (!row) return
  actionLoadingId.value = row.id
  try {
    const updated = await imageJobsApi.terminate(row.id)
    replaceJob(updated)
    if (detail.value?.id === updated.id) detail.value = updated
    appStore.showSuccess(t('admin.imageJobs.messages.terminateSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.imageJobs.messages.terminateFailed'))
  } finally {
    actionLoadingId.value = null
  }
}

function replaceJob(updated: AdminImageJob) {
  const index = jobs.value.findIndex((item) => item.id === updated.id)
  if (index >= 0) jobs.value[index] = updated
}

function closeDetail() {
  detail.value = null
  clearPreview()
}

function clearPreview() {
  if (previewURL.value.startsWith('blob:')) URL.revokeObjectURL(previewURL.value)
  previewURL.value = ''
}

function hasResult(row: AdminImageJob) {
  return row.status === 'succeeded' || !!directResultURL(row)
}

function directResultURL(row: AdminImageJob) {
  const resultURLs = Array.isArray(row.params?.result_urls) ? row.params.result_urls : []
  const candidates = [row.object_url, row.preview_url, ...resultURLs]
  for (const candidate of candidates) {
    const url = String(candidate || '').trim()
    if (/^https?:\/\//i.test(url) || /^data:image\/(?:png|jpe?g|webp|gif);/i.test(url)) return url
  }
  return ''
}

function statusLabel(status: string) {
  const normalized = status === 'cancelled' ? 'canceled' : status || 'unknown'
  return t(`admin.imageJobs.status.${normalized}`)
}

function statusBadgeClass(status: string) {
  if (status === 'succeeded') return 'badge-success'
  if (status === 'failed') return 'badge-danger'
  if (status === 'running') return 'badge-primary'
  if (status === 'queued' || status === 'created') return 'badge-warning'
  return 'badge-gray'
}

function gatewayLabel(gateway: string) {
  if (gateway === 'image_task') return t('admin.imageJobs.gateway.async')
  if (gateway === 'image_sync') return t('admin.imageJobs.gateway.sync')
  return gateway || '-'
}

function scopeLabel(scope: string) {
  return scope === 'async_execution' ? t('admin.imageJobs.scope.async') : t('admin.imageJobs.scope.local')
}

function paramSummary(row: AdminImageJob) {
  const size = String(row.params?.size || row.params?.resolution || '-')
  const ratio = String(row.params?.aspect_ratio || '')
  const quality = String(row.params?.quality || row.params?.quality_tier || '')
  return [size, ratio, quality].filter(Boolean).join(' · ')
}

function referenceImages(row: AdminImageJob): string[] {
  const params = row.params || {}
  const candidates = [params.reference_images, params.references, params.input_images, params.images]
  const urls: string[] = []
  for (const candidate of candidates) {
    if (!Array.isArray(candidate)) continue
    for (const item of candidate) {
      const url = typeof item === 'string' ? item : String((item as any)?.url || (item as any)?.image_url || '')
      if ((url.startsWith('https://') || url.startsWith('http://') || url.startsWith('data:image/')) && !urls.includes(url)) urls.push(url)
    }
  }
  return urls.slice(0, 8)
}

function prettyParams(row: AdminImageJob) {
  return JSON.stringify(row.params || {}, null, 2)
}

function formatDateTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

onMounted(() => void loadJobs())
onUnmounted(clearPreview)
</script>

<style scoped>
.task-tab {
  @apply border-b-2 border-transparent px-4 py-2 text-sm font-medium text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white;
}

.task-tab--active {
  @apply border-primary-600 text-primary-700 dark:border-primary-400 dark:text-primary-300;
}
</style>
