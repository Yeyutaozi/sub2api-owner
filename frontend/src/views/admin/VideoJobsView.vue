<template>
  <AppLayout>
    <TablePageLayout>
      <template #intro>
        <div class="desk-board__intro-copy">
          <p class="page-kicker">VIDEO JOBS</p>
          <h2 class="desk-board__title">{{ t('admin.videoJobs.title') }}</h2>
          <p class="desk-board__sub">{{ t('admin.videoJobs.description') }}</p>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex-1 sm:max-w-72">
              <input
                v-model="filters.search"
                type="text"
                class="input"
                :placeholder="t('admin.videoJobs.searchPlaceholder')"
                @keyup.enter="handleFilterChange"
              />
            </div>
            <div class="w-full sm:w-56">
              <input
                v-model="filters.job_id"
                type="text"
                class="input"
                :placeholder="t('admin.videoJobs.jobIdPlaceholder')"
                @keyup.enter="handleFilterChange"
              />
            </div>
            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              class="w-full sm:w-40"
              @change="handleFilterChange"
            />
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="filters.unsettled_only"
                type="checkbox"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                @change="handleFilterChange"
              />
              {{ t('admin.videoJobs.filters.unsettledOnly') }}
            </label>
            <div class="ml-auto flex items-center gap-2">
              <button class="btn btn-secondary" :disabled="loading" :title="t('admin.videoJobs.actions.refresh')" @click="loadJobs">
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button class="btn btn-primary" :disabled="loading" @click="handleFilterChange">
                {{ t('admin.videoJobs.actions.refresh') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="jobs" :loading="loading">
          <template #cell-job_id="{ row }">
            <div class="flex min-w-0 flex-col">
              <button class="truncate text-left font-medium text-primary-600 hover:underline dark:text-primary-400" @click="openDetail(row)">
                {{ row.job_id }}
              </button>
              <span v-if="row.upstream_job_id && row.upstream_job_id !== row.job_id" class="truncate text-xs text-gray-500 dark:text-gray-400">
                up: {{ row.upstream_job_id }}
              </span>
            </div>
          </template>

          <template #cell-userGroup="{ row }">
            <div class="flex min-w-0 flex-col text-sm text-gray-700 dark:text-gray-300">
              <span class="truncate font-medium">{{ row.username || row.user_email || ('#' + row.user_id) }}</span>
              <span v-if="row.username && row.user_email" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ row.user_email }}</span>
              <span class="truncate text-xs text-gray-500 dark:text-gray-400">
                {{ row.group_name || ('group #' + row.group_id) }} · {{ row.api_key_name || ('key #' + row.api_key_id) }}
              </span>
            </div>
          </template>

          <template #cell-model="{ row }">
            <div class="flex min-w-0 flex-col text-sm">
              <span class="truncate font-medium text-gray-900 dark:text-white">{{ row.model }}</span>
              <span v-if="row.fallback_model" class="truncate text-xs text-gray-500 dark:text-gray-400">fb: {{ row.fallback_model }}</span>
            </div>
          </template>

          <template #cell-task_status="{ row }">
            <div class="flex flex-col gap-1">
              <span :class="['badge', statusBadgeClass(row.task_status)]">{{ statusLabel(row.task_status) }}</span>
              <span v-if="!row.settled_at" class="text-[11px] text-amber-600 dark:text-amber-400">unsettled</span>
            </div>
          </template>

          <template #cell-refund_status="{ row }">
            <span :class="['badge', refundBadgeClass(row.refund_status)]">{{ refundLabel(row.refund_status) }}</span>
          </template>

          <template #cell-prompt="{ row }">
            <span class="line-clamp-2 max-w-xs text-sm text-gray-700 dark:text-gray-300" :title="row.prompt || ''">
              {{ row.prompt || '-' }}
            </span>
          </template>

          <template #cell-created_at="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex flex-wrap items-center gap-1">
              <button class="btn btn-secondary btn-sm" @click="openDetail(row)">{{ t('admin.videoJobs.actions.detail') }}</button>
              <button class="btn btn-secondary btn-sm" :disabled="actionLoadingId === row.job_id" @click="syncJob(row)">
                {{ t('admin.videoJobs.actions.sync') }}
              </button>
              <button
                class="btn btn-danger btn-sm"
                :disabled="!!row.settled_at || actionLoadingId === row.job_id"
                @click="confirmKill(row)"
              >
                {{ t('admin.videoJobs.actions.kill') }}
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState :title="t('admin.videoJobs.empty.title')" :description="t('admin.videoJobs.empty.description')" />
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

    <BaseDialog :show="!!detail" :title="t('admin.videoJobs.detail.title')" width="extra-wide" @close="detail = null">
      <div v-if="detail" class="space-y-5">
        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.basic') }}</h4>
          <div class="grid gap-2 text-sm sm:grid-cols-2">
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.jobId') }}:</span> <button class="font-mono text-primary-600" @click="copyText(detail.job_id)">{{ detail.job_id }}</button></div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.upstreamJobId') }}:</span> <span class="font-mono">{{ detail.upstream_job_id || '-' }}</span></div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.user') }}:</span> {{ detail.username || detail.user_email || ('#' + detail.user_id) }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.group') }}:</span> {{ detail.group_name || ('#' + detail.group_id) }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.apiKey') }}:</span> {{ detail.api_key_name || ('#' + detail.api_key_id) }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.accountId') }}:</span> {{ detail.account_id }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.model') }}:</span> {{ detail.model }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.fallbackModel') }}:</span> {{ detail.fallback_model || '-' }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.taskStatus') }}:</span> <span :class="['badge', statusBadgeClass(detail.task_status)]">{{ statusLabel(detail.task_status) }}</span></div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.refundStatus') }}:</span> <span :class="['badge', refundBadgeClass(detail.refund_status)]">{{ refundLabel(detail.refund_status) }}</span></div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.resolution') }}:</span> {{ snapshotField(detail, 'resolution') }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.duration') }}:</span> {{ snapshotField(detail, 'duration_seconds') }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.aspectRatio') }}:</span> {{ snapshotField(detail, 'aspect_ratio') }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.createdAt') }}:</span> {{ formatDateTime(detail.created_at) }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.settledAt') }}:</span> {{ detail.settled_at ? formatDateTime(detail.settled_at) : '-' }}</div>
            <div><span class="text-gray-500">{{ t('admin.videoJobs.detail.fields.lastPolledAt') }}:</span> {{ detail.last_polled_at ? formatDateTime(detail.last_polled_at) : '-' }}</div>
          </div>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.prompt') }}</h4>
          <pre class="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-3 text-sm text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ detail.prompt || t('admin.videoJobs.detail.noPrompt') }}</pre>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.materials') }}</h4>
          <div v-if="materialItems(detail).length === 0" class="text-sm text-gray-500">{{ t('admin.videoJobs.detail.noMaterials') }}</div>
          <div v-else class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <div v-for="item in materialItems(detail)" :key="item.key" class="rounded-lg border border-gray-200 p-2 dark:border-dark-600">
              <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">{{ item.label }}</div>
              <a v-if="item.url" :href="item.url" target="_blank" rel="noopener noreferrer" class="block">
                <img v-if="item.kind === 'image'" :src="item.url" alt="" class="h-28 w-full rounded object-cover bg-gray-100" @error="onImgError" />
                <video v-else-if="item.kind === 'video'" :src="item.url" class="h-28 w-full rounded bg-black object-cover" controls muted />
                <audio v-else-if="item.kind === 'audio'" :src="item.url" class="w-full" controls />
                <span v-else class="break-all text-xs text-primary-600">{{ item.url }}</span>
              </a>
              <div v-else class="text-xs text-gray-400">-</div>
            </div>
          </div>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.result') }}</h4>
          <div v-if="detail.result_path" class="space-y-2">
            <div class="text-sm"><span class="text-gray-500">{{ t('admin.videoJobs.detail.resultPath') }}:</span> <code>{{ detail.result_path }}</code></div>
            <a class="btn btn-primary btn-sm" :href="detail.result_path" target="_blank" rel="noopener noreferrer">{{ t('admin.videoJobs.actions.openResult') }}</a>
          </div>
          <div v-else class="text-sm text-gray-500">{{ t('admin.videoJobs.detail.noResult') }}</div>
        </section>

        <section v-if="detail.last_error" class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.lastError') }}</h4>
          <pre class="max-h-40 overflow-auto whitespace-pre-wrap rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ detail.last_error }}</pre>
        </section>

        <section class="space-y-2">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.videoJobs.detail.snapshotJson') }}</h4>
          <pre class="max-h-56 overflow-auto rounded-lg bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ prettySnapshot(detail) }}</pre>
        </section>
      </div>

      <template #footer>
        <div class="flex flex-wrap justify-end gap-2">
          <button class="btn btn-secondary" @click="detail = null">Close</button>
          <button v-if="detail" class="btn btn-secondary" :disabled="actionLoadingId === detail.job_id" @click="syncJob(detail)">
            {{ t('admin.videoJobs.actions.sync') }}
          </button>
          <button
            v-if="detail && !detail.settled_at"
            class="btn btn-danger"
            :disabled="actionLoadingId === detail.job_id"
            @click="confirmKill(detail)"
          >
            {{ t('admin.videoJobs.actions.kill') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!killTarget"
      :title="t('admin.videoJobs.messages.killConfirmTitle')"
      :message="t('admin.videoJobs.messages.killConfirmMessage')"
      danger
      @confirm="doKill"
      @cancel="killTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
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
import { videoJobsApi, type AdminVideoJob } from '@/api/admin/videoJobs'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const actionLoadingId = ref('')
const jobs = ref<AdminVideoJob[]>([])
const detail = ref<AdminVideoJob | null>(null)
const killTarget = ref<AdminVideoJob | null>(null)

const filters = reactive({
  search: '',
  job_id: '',
  status: '',
  unsettled_only: false
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1
})

const columns = computed<Column[]>(() => [
  { key: 'job_id', label: t('admin.videoJobs.columns.jobId') },
  { key: 'userGroup', label: t('admin.videoJobs.columns.userGroup') },
  { key: 'model', label: t('admin.videoJobs.columns.model') },
  { key: 'task_status', label: t('admin.videoJobs.columns.status') },
  { key: 'refund_status', label: t('admin.videoJobs.columns.refund') },
  { key: 'prompt', label: t('admin.videoJobs.columns.prompt') },
  { key: 'created_at', label: t('admin.videoJobs.columns.createdAt') },
  { key: 'actions', label: t('admin.videoJobs.columns.actions') }
])

const statusFilterOptions = computed(() => [
  { label: t('admin.videoJobs.filters.allStatus'), value: '' },
  { label: t('admin.videoJobs.status.queued'), value: 'queued' },
  { label: t('admin.videoJobs.status.running'), value: 'running' },
  { label: t('admin.videoJobs.status.succeeded'), value: 'succeeded' },
  { label: t('admin.videoJobs.status.failed'), value: 'failed' },
  { label: t('admin.videoJobs.status.cancelled'), value: 'cancelled' },
  { label: t('admin.videoJobs.status.unknown'), value: 'unknown' }
])

async function loadJobs() {
  loading.value = true
  try {
    const result = await videoJobsApi.list({
      page: pagination.page,
      page_size: pagination.page_size,
      search: filters.search.trim() || undefined,
      job_id: filters.job_id.trim() || undefined,
      status: filters.status || undefined,
      unsettled_only: filters.unsettled_only || undefined
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
    appStore.showError(error?.message || t('admin.videoJobs.messages.loadFailed'))
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

function openDetail(row: AdminVideoJob) {
  detail.value = row
}

async function syncJob(row: AdminVideoJob) {
  actionLoadingId.value = row.job_id
  try {
    const updated = await videoJobsApi.sync(row.job_id)
    replaceJob(updated)
    if (detail.value?.job_id === updated.job_id) detail.value = updated
    appStore.showSuccess(t('admin.videoJobs.messages.syncSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.videoJobs.messages.syncFailed'))
  } finally {
    actionLoadingId.value = ''
  }
}

function confirmKill(row: AdminVideoJob) {
  killTarget.value = row
}

async function doKill() {
  const row = killTarget.value
  killTarget.value = null
  if (!row) return
  actionLoadingId.value = row.job_id
  try {
    const updated = await videoJobsApi.kill(row.job_id)
    replaceJob(updated)
    if (detail.value?.job_id === updated.job_id) detail.value = updated
    appStore.showSuccess(t('admin.videoJobs.messages.killSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.videoJobs.messages.killFailed'))
  } finally {
    actionLoadingId.value = ''
  }
}

function replaceJob(updated: AdminVideoJob) {
  const idx = jobs.value.findIndex((j) => j.job_id === updated.job_id)
  if (idx >= 0) jobs.value[idx] = updated
}

function statusLabel(status: string) {
  const key = status || 'unknown'
  const map: Record<string, string> = {
    queued: t('admin.videoJobs.status.queued'),
    running: t('admin.videoJobs.status.running'),
    succeeded: t('admin.videoJobs.status.succeeded'),
    failed: t('admin.videoJobs.status.failed'),
    cancelled: t('admin.videoJobs.status.cancelled'),
    canceled: t('admin.videoJobs.status.cancelled'),
    unknown: t('admin.videoJobs.status.unknown')
  }
  return map[key] || status
}

function statusBadgeClass(status: string) {
  if (status === 'succeeded') return 'badge-success'
  if (status === 'failed') return 'badge-danger'
  if (status === 'running') return 'badge-primary'
  if (status === 'queued') return 'badge-warning'
  if (status === 'cancelled' || status === 'canceled') return 'badge-gray'
  return 'badge-gray'
}

function refundLabel(status: string) {
  if (!status) return t('admin.videoJobs.refund.empty')
  const map: Record<string, string> = {
    pending: t('admin.videoJobs.refund.pending'),
    applied: t('admin.videoJobs.refund.applied'),
    error: t('admin.videoJobs.refund.error'),
    not_required: t('admin.videoJobs.refund.not_required')
  }
  return map[status] || status
}

function refundBadgeClass(status: string) {
  if (status === 'applied' || status === 'not_required') return 'badge-success'
  if (status === 'error') return 'badge-danger'
  if (status === 'pending') return 'badge-warning'
  return 'badge-gray'
}

function formatDateTime(value?: string | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString(undefined, { hour12: false })
}

function snapshotField(job: AdminVideoJob, key: string) {
  const snap = job.request_snapshot || {}
  const val = snap[key]
  if (val == null || val === '') return '-'
  return String(val)
}

function materialItems(job: AdminVideoJob) {
  const snap = job.request_snapshot || {}
  const items: Array<{ key: string; label: string; url: string; kind: 'image' | 'video' | 'audio' | 'link' }> = []
  const start = String(snap.start_frame_url || '')
  const end = String(snap.end_frame_url || '')
  if (start) items.push({ key: 'start', label: t('admin.videoJobs.material.startFrame'), url: start, kind: 'image' })
  if (end) items.push({ key: 'end', label: t('admin.videoJobs.material.endFrame'), url: end, kind: 'image' })
  const images = Array.isArray(snap.image_references) ? snap.image_references : []
  images.forEach((ref: any, i: number) => {
    const url = String(ref?.url || ref || '')
    if (url) items.push({ key: 'img-' + i, label: t('admin.videoJobs.material.image') + ' #' + (i + 1), url, kind: 'image' })
  })
  const videos = Array.isArray(snap.video_references) ? snap.video_references : []
  videos.forEach((ref: any, i: number) => {
    const url = String(ref?.url || ref || '')
    if (url) items.push({ key: 'vid-' + i, label: t('admin.videoJobs.material.video') + ' #' + (i + 1), url, kind: 'video' })
  })
  const audios = Array.isArray(snap.audio_references) ? snap.audio_references : []
  audios.forEach((ref: any, i: number) => {
    const url = String(ref?.url || ref || '')
    if (url) items.push({ key: 'aud-' + i, label: t('admin.videoJobs.material.audio') + ' #' + (i + 1), url, kind: 'audio' })
  })
  return items
}

function prettySnapshot(job: AdminVideoJob) {
  try {
    return JSON.stringify(job.request_snapshot || {}, null, 2)
  } catch {
    return '{}'
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('admin.videoJobs.messages.copyOk'))
  } catch {
    appStore.showError(t('admin.videoJobs.messages.copyFail'))
  }
}

function onImgError(e: Event) {
  const el = e.target as HTMLImageElement | null
  if (el) el.style.display = 'none'
}

onMounted(() => {
  void loadJobs()
})
</script>
