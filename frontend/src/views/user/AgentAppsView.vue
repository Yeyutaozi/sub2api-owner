<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
            <Icon name="grid" size="sm" />
            <span>应用中心</span>
          </div>
          <h1 class="mt-1 text-2xl font-semibold tracking-normal text-gray-950 dark:text-white">选择一个应用开始工作</h1>
        </div>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div class="relative w-full sm:w-80">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
            <input
              v-model="searchQuery"
              class="input pl-10"
              placeholder="搜索应用名称或分类"
              @keyup.enter="loadApps"
            />
          </div>
          <div class="flex gap-3 sm:contents">
            <Select v-model="typeFilter" :options="typeOptions" class="min-w-0 flex-1 sm:w-40 sm:flex-none" />
            <button class="btn btn-secondary flex-shrink-0" :disabled="loadingApps || loadingRuns" title="刷新" @click="refreshWorkspace">
              <Icon name="refresh" size="md" :class="loadingApps || loadingRuns ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </div>

      <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(440px,520px)]">
        <main :class="['min-w-0 space-y-5 xl:order-1', selectedApp ? 'order-2' : 'order-1']">
          <section class="card p-4">
            <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">可用应用</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ appPagination.total }} 个已发布应用</p>
              </div>
              <div v-if="selectedApp" class="hidden items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:bg-dark-800 dark:text-gray-300 sm:flex">
                <span class="h-2 w-2 rounded-full bg-primary-500" />
                <span class="max-w-[220px] truncate">{{ selectedApp.name }}</span>
              </div>
            </div>

            <div v-if="loadingApps" class="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-3">
              <div v-for="i in 6" :key="i" class="h-40 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800" />
            </div>
            <div v-else-if="apps.length === 0" class="flex min-h-[240px] items-center justify-center rounded-lg border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
              暂无可用应用
            </div>
            <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-3">
              <button
                v-for="app in apps"
                :key="app.id"
                type="button"
                :class="[
                  'group flex min-h-[168px] flex-col justify-between rounded-lg border p-4 text-left transition-all',
                  selectedApp?.id === app.id
                    ? 'border-primary-500 bg-primary-50 shadow-sm ring-1 ring-primary-200 dark:border-primary-500 dark:bg-primary-900/20 dark:ring-primary-900'
                    : 'border-gray-200 bg-white hover:border-primary-300 hover:shadow-sm dark:border-dark-700 dark:bg-dark-900/40 dark:hover:border-primary-600'
                ]"
                @click="selectApp(app)"
              >
                <div class="flex items-start gap-3">
                  <img
                    v-if="app.icon_url"
                    :src="appDisplayIconURL(app)"
                    :alt="app.name"
                    class="h-12 w-12 flex-shrink-0 rounded-lg border border-gray-200 object-cover dark:border-dark-700"
                  />
                  <div
                    v-else
                    :class="['flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-lg text-sm font-semibold', appTypeToneClass(app.app_type)]"
                  >
                    {{ appInitials(app) }}
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-start justify-between gap-2">
                      <h3 class="line-clamp-2 text-sm font-semibold leading-5 text-gray-950 dark:text-white">{{ app.name }}</h3>
                      <Icon name="chevronRight" size="sm" class="mt-0.5 flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500" />
                    </div>
                    <div class="mt-2 flex flex-wrap items-center gap-1.5">
                      <span :class="['badge', appTypeBadgeClass(app.app_type)]">{{ appTypeLabel(app.app_type) }}</span>
                      <span v-if="app.category" class="badge badge-gray">{{ app.category }}</span>
                    </div>
                  </div>
                </div>
                <p class="mt-4 line-clamp-2 min-h-[40px] text-sm leading-5 text-gray-500 dark:text-gray-400">
                  {{ app.description || '暂无描述' }}
                </p>
                <div class="mt-4 flex items-center justify-between text-xs text-gray-400 dark:text-gray-500">
                  <span>{{ appVersionLabel(app) }}</span>
                  <span v-if="selectedApp?.id === app.id" class="font-medium text-primary-600 dark:text-primary-400">当前选择</span>
                </div>
              </button>
            </div>

            <Pagination
              v-if="appPagination.total > appPagination.page_size"
              class="mt-4"
              :page="appPagination.page"
              :total="appPagination.total"
              :page-size="appPagination.page_size"
              :show-page-size-selector="false"
              @update:page="handleAppPageChange"
              @update:page-size="handleAppPageSizeChange"
            />
          </section>

          <section v-if="selectedRun" ref="runResultSection" class="card scroll-mt-24 overflow-hidden">
            <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="text-base font-semibold text-gray-950 dark:text-white">运行结果</h2>
                  <span :class="['badge', runStatusBadgeClass(selectedRun.status)]">{{ runStatusLabel(selectedRun.status) }}</span>
                  <span v-if="selectedRunPolling" class="inline-flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
                    <Icon name="refresh" size="sm" class="animate-spin" />
                    自动刷新中
                  </span>
                </div>
                <p class="mt-1 truncate text-sm text-gray-500 dark:text-gray-400">
                  #{{ selectedRun.id }} · {{ formatDateTime(selectedRun.created_at) }}
                </p>
              </div>
              <button
                v-if="canCancelRun(selectedRun)"
                type="button"
                class="btn btn-danger btn-sm"
                :disabled="cancelingRunId === selectedRun.id"
                title="取消运行"
                @click="cancelSelectedRun"
              >
                <Icon name="x" size="sm" class="mr-1" />
                {{ cancelingRunId === selectedRun.id ? '取消中...' : '停止' }}
              </button>
            </div>

            <div class="p-5">
              <section v-if="selectedRunInputItems.length || selectedRunInputAssets.length" class="mb-5 border-b border-gray-200 pb-5 dark:border-dark-700">
                <div class="mb-3 flex items-center gap-2">
                  <Icon name="document" size="sm" class="text-gray-500 dark:text-gray-400" />
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">本次输入</h3>
                </div>

                <div v-if="selectedRunInputItems.length" class="grid grid-cols-1 gap-x-6 gap-y-4 lg:grid-cols-2">
                  <div v-for="item in selectedRunInputItems" :key="item.key" class="min-w-0 border-l-2 border-gray-200 pl-3 dark:border-dark-700">
                    <div class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</div>
                    <div class="mt-1 break-words text-sm text-gray-900 dark:text-gray-100">
                      <StructuredValue :value="item.value" />
                    </div>
                  </div>
                </div>

                <div v-if="selectedRunInputAssets.length" class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
                  <div
                    v-for="asset in selectedRunInputAssets"
                    :key="asset.id"
                    class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40"
                  >
                    <div v-if="isImageInputAsset(asset) && inputAssetPreviewURL(asset)" class="bg-gray-50 dark:bg-dark-800">
                      <img :src="inputAssetPreviewURL(asset)" :alt="asset.name" class="max-h-80 w-full object-contain" />
                    </div>
                    <div v-else-if="isVideoInputAsset(asset) && inputAssetPreviewURL(asset)" class="bg-black">
                      <video :src="inputAssetPreviewURL(asset)" controls class="max-h-80 w-full" />
                    </div>
                    <div v-else-if="isAudioInputAsset(asset) && inputAssetPreviewURL(asset)" class="bg-gray-50 p-4 dark:bg-dark-800">
                      <audio :src="inputAssetPreviewURL(asset)" controls class="w-full" />
                    </div>
                    <div class="flex items-center justify-between gap-3 px-3 py-3 text-sm">
                      <span class="min-w-0">
                        <span class="block text-xs text-gray-500 dark:text-gray-400">{{ inputAssetFieldLabel(asset) }}</span>
                        <span class="mt-1 block truncate text-gray-800 dark:text-gray-100">{{ asset.name }}</span>
                        <span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ inputAssetDescription(asset) }}</span>
                      </span>
                      <button type="button" class="btn btn-secondary btn-sm flex-shrink-0" title="下载输入文件" @click="downloadInputAsset(asset.id)">
                        <Icon name="download" size="sm" />
                      </button>
                    </div>
                  </div>
                </div>
              </section>

              <div
                v-if="runPrimaryText(selectedRun)"
                :class="isBundledResultApp ? '' : 'rounded-lg bg-gray-50 p-4 dark:bg-dark-900/60'"
              >
                <div class="mb-2 flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  <Icon name="sparkles" size="sm" />
                  <span>{{ isTerminalRunStatus(selectedRun.status) ? '最终结果' : '实时输出' }}</span>
                </div>
                <div
                  class="whitespace-pre-wrap text-sm leading-6 text-gray-900 dark:text-gray-100"
                  :class="isBundledResultApp ? 'rounded-lg bg-gray-50 p-4 dark:bg-dark-900/60' : ''"
                >{{ runPrimaryText(selectedRun) }}</div>
                <div
                  v-if="isBundledResultApp && runResultArtifacts(selectedRun).length"
                  class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2"
                >
                  <div
                    v-for="artifact in runResultArtifacts(selectedRun)"
                    :key="artifact.id"
                    :class="isWordArtifact(artifact) ? 'lg:col-span-2' : ''"
                    class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40"
                  >
                    <div v-if="isImageArtifact(artifact) && artifactPreviewURL(artifact)" class="bg-gray-50 dark:bg-dark-800">
                      <img :src="artifactPreviewURL(artifact)" :alt="artifact.name" class="max-h-96 w-full object-contain" />
                    </div>
                    <div v-else-if="isVideoArtifact(artifact) && artifactPreviewURL(artifact)" class="bg-black">
                      <video :src="artifactPreviewURL(artifact)" controls class="max-h-96 w-full" />
                    </div>
                    <div class="flex items-center justify-between gap-3 px-3 py-3 text-sm">
                      <span class="min-w-0">
                        <span class="block truncate text-gray-800 dark:text-gray-100">{{ artifact.name }}</span>
                        <span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ artifactDescription(artifact) }}</span>
                      </span>
                      <button type="button" class="btn btn-secondary btn-sm flex-shrink-0" :title="isWordArtifact(artifact) ? '下载 Word 论文' : '下载结果'" @click="downloadArtifact(artifact.id)">
                        <Icon name="download" size="sm" />
                        <span v-if="isWordArtifact(artifact)" class="ml-1">下载 Word</span>
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              <div v-else-if="selectedRun.status === 'succeeded' && !runResultArtifacts(selectedRun).length" class="rounded-lg bg-green-50 p-4 text-sm text-green-700 dark:bg-green-900/20 dark:text-green-300">
                运行已完成，暂无可直接展示的结果。
              </div>
              <div v-else-if="selectedRun.status === 'running' || selectedRun.status === 'queued'" class="rounded-lg bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
                {{ selectedRun.status === 'queued' ? '正在排队' : '正在运行' }}
              </div>

              <div v-if="runReadableItems(selectedRun).length" class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
                <div
                  v-for="item in runReadableItems(selectedRun)"
                  :key="item.label"
                  class="rounded-lg border border-gray-200 px-3 py-3 dark:border-dark-700"
                >
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</div>
                  <div class="mt-1 break-words text-sm text-gray-900 dark:text-gray-100">
                    <StructuredValue :value="item.value" />
                  </div>
                </div>
              </div>

              <div v-if="runUsageLogs.length" class="mt-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">本次模型消耗</h3>
                  <button type="button" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="router.push('/usage')">
                    查看全部使用记录
                  </button>
                </div>
                <div class="mt-3 grid grid-cols-3 gap-2 text-center text-xs">
                  <div class="rounded bg-gray-50 px-2 py-2 dark:bg-dark-900/60">
                    <div class="text-gray-500 dark:text-gray-400">模型请求</div>
                    <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ runUsageLogs.length }}</div>
                  </div>
                  <div class="rounded bg-gray-50 px-2 py-2 dark:bg-dark-900/60">
                    <div class="text-gray-500 dark:text-gray-400">Token</div>
                    <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ runUsageTotalTokens }}</div>
                  </div>
                  <div class="rounded bg-gray-50 px-2 py-2 dark:bg-dark-900/60">
                    <div class="text-gray-500 dark:text-gray-400">实际扣费</div>
                    <div class="mt-1 font-semibold text-gray-900 dark:text-white">${{ runUsageActualCost }}</div>
                  </div>
                </div>
                <div class="mt-3 space-y-2">
                  <div v-for="log in runUsageLogs" :key="log.id" class="flex items-center justify-between gap-3 text-xs text-gray-600 dark:text-gray-300">
                    <span class="min-w-0 truncate">{{ log.model }} · {{ log.agent_node_role || log.agent_node_id || '模型调用' }}</span>
                    <span class="flex-shrink-0">{{ usageLogMeasure(log) }} · ${{ Number(log.actual_cost || 0).toFixed(6) }}</span>
                  </div>
                </div>
              </div>

              <div v-if="selectedRun.error_message" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
                <div>{{ runErrorMessage(selectedRun) }}</div>
                <div v-if="selectedRun.error_code" class="mt-1 text-xs opacity-75">错误编号：{{ selectedRun.error_code }}</div>
              </div>

              <div v-if="!isBundledResultApp && runResultArtifacts(selectedRun).length" class="mt-5">
                <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">最终结果</h3>
                <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
                  <div
                    v-for="artifact in runResultArtifacts(selectedRun)"
                    :key="artifact.id"
                    class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/40"
                  >
                    <div v-if="isImageArtifact(artifact) && artifactPreviewURL(artifact)" class="bg-gray-50 dark:bg-dark-800">
                      <img :src="artifactPreviewURL(artifact)" :alt="artifact.name" class="max-h-80 w-full object-contain" />
                    </div>
                    <div v-else-if="isVideoArtifact(artifact) && artifactPreviewURL(artifact)" class="bg-black">
                      <video :src="artifactPreviewURL(artifact)" controls class="max-h-80 w-full" />
                    </div>
                    <div v-else-if="isAudioArtifact(artifact) && artifactPreviewURL(artifact)" class="bg-gray-50 p-4 dark:bg-dark-800">
                      <audio :src="artifactPreviewURL(artifact)" controls class="w-full" />
                    </div>
                    <div class="flex items-center justify-between gap-3 px-3 py-3 text-sm">
                      <span class="flex min-w-0 items-center gap-3">
                        <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-gray-300">
                          <Icon :name="artifactIconName(artifact)" size="sm" />
                        </span>
                        <span class="min-w-0">
                          <span class="block truncate text-gray-800 dark:text-gray-100">{{ artifact.name }}</span>
                          <span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ artifactDescription(artifact) }}</span>
                        </span>
                      </span>
                      <button type="button" class="btn btn-secondary btn-sm flex-shrink-0" @click="downloadArtifact(artifact.id)">
                        <Icon name="download" size="sm" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <details class="mt-5 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
                <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">查看运行过程</summary>
                <div class="mt-3 space-y-4">
                  <div>
                    <div class="mb-2 flex items-center justify-between gap-2">
                      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">处理进度</h4>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="loadingRunEvents"
                        title="刷新运行事件"
                        @click="loadRunEvents()"
                      >
                        <Icon name="refresh" size="sm" :class="loadingRunEvents ? 'animate-spin' : ''" />
                      </button>
                    </div>
                    <div v-if="loadingRunEvents" class="space-y-2">
                      <div v-for="i in 3" :key="i" class="h-10 animate-pulse rounded bg-gray-50 dark:bg-dark-800" />
                    </div>
                    <div v-else-if="runEvents.length === 0" class="text-sm text-gray-500 dark:text-gray-400">暂无事件</div>
                    <div v-else class="space-y-3">
                      <div v-for="event in runEvents" :key="event.id" class="grid grid-cols-[auto_1fr] gap-3">
                        <div class="pt-1">
                          <span :class="['block h-2.5 w-2.5 rounded-full', runEventDotClass(event)]" />
                        </div>
                        <div class="min-w-0 border-b border-gray-200 pb-3 last:border-b-0 last:pb-0 dark:border-dark-700">
                          <div class="flex flex-wrap items-center gap-2">
                            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ runEventLabel(event) }}</span>
                            <span v-if="event.status" :class="['badge', runStatusBadgeClass(event.status)]">{{ runStatusLabel(event.status) }}</span>
                            <span v-if="typeof event.progress === 'number'" class="text-xs text-gray-500 dark:text-gray-400">{{ formatPercent(event.progress) }}</span>
                          </div>
                          <p v-if="runEventMessage(event)" class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ runEventMessage(event) }}</p>
                          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(event.created_at) }}</p>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div v-if="runLogArtifacts(selectedRun).length">
                    <h4 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">日志文件</h4>
                    <div class="space-y-2">
                      <div
                        v-for="artifact in runLogArtifacts(selectedRun)"
                        :key="artifact.id"
                        class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-900/60"
                      >
                        <span class="min-w-0">
                          <span class="block truncate text-gray-800 dark:text-gray-100">{{ artifact.name }}</span>
                          <span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ artifactDescription(artifact) }}</span>
                        </span>
                        <button type="button" class="btn btn-secondary btn-sm flex-shrink-0" @click="downloadArtifact(artifact.id)">
                          <Icon name="download" size="sm" />
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </details>
            </div>
          </section>

          <section class="card overflow-hidden">
            <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <div class="min-w-0">
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">运行历史</h2>
                <p class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ selectedApp ? `${selectedApp.name} 的最近运行` : '全部应用的最近运行' }}
                </p>
              </div>
              <button class="btn btn-secondary btn-sm flex-shrink-0" :disabled="loadingRuns" title="刷新" @click="loadRuns()">
                <Icon name="refresh" size="sm" :class="loadingRuns ? 'animate-spin' : ''" />
              </button>
            </div>
            <div class="max-h-[420px] overflow-y-auto p-3">
              <div v-if="loadingRuns" class="grid grid-cols-1 gap-2 md:grid-cols-2">
                <div v-for="i in 4" :key="i" class="h-16 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800" />
              </div>
              <div v-else-if="runs.length === 0" class="flex min-h-[140px] items-center justify-center text-sm text-gray-500 dark:text-gray-400">
                暂无运行记录
              </div>
              <div v-else class="grid grid-cols-1 gap-2 md:grid-cols-2">
                <button
                  v-for="run in runs"
                  :key="run.id"
                  type="button"
                  :class="[
                    'flex min-h-[68px] w-full items-center justify-between gap-3 rounded-lg border px-3 py-3 text-left transition-colors',
                    selectedRun?.id === run.id
                      ? 'border-primary-500 bg-primary-50 dark:border-primary-500 dark:bg-primary-900/20'
                      : 'border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800'
                  ]"
                  @click="selectRun(run)"
                >
                  <span class="min-w-0">
                    <span class="flex items-center gap-2">
                      <span :class="['h-2 w-2 rounded-full', runStatusDotClass(run.status)]" />
                      <span class="truncate text-sm font-medium text-gray-900 dark:text-white">运行 #{{ run.id }}</span>
                    </span>
                    <span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(run.created_at) }}</span>
                  </span>
                  <span :class="['badge flex-shrink-0', runStatusBadgeClass(run.status)]">{{ runStatusLabel(run.status) }}</span>
                </button>
              </div>
            </div>
            <Pagination
              v-if="runPagination.total > runPagination.page_size"
              :page="runPagination.page"
              :total="runPagination.total"
              :page-size="runPagination.page_size"
              :show-page-size-selector="false"
              @update:page="handleRunPageChange"
              @update:page-size="handleRunPageSizeChange"
            />
          </section>
        </main>

        <aside :class="['space-y-4 xl:order-2 xl:sticky xl:top-20 xl:self-start', selectedApp ? 'order-1' : 'order-2']">
          <section class="card overflow-hidden">
            <div v-if="selectedApp" class="p-5">
              <div class="flex items-start gap-3">
                <img
                  v-if="selectedApp.icon_url"
                  :src="appDisplayIconURL(selectedApp)"
                  :alt="selectedApp.name"
                  class="h-14 w-14 flex-shrink-0 rounded-lg border border-gray-200 object-cover dark:border-dark-700"
                />
                <div
                  v-else
                  :class="['flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-lg text-base font-semibold', appTypeToneClass(selectedApp.app_type)]"
                >
                  {{ appInitials(selectedApp) }}
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span :class="['badge', appTypeBadgeClass(selectedApp.app_type)]">{{ appTypeLabel(selectedApp.app_type) }}</span>
                    <span class="badge badge-success">已发布</span>
                  </div>
                  <h2 class="mt-2 line-clamp-2 text-lg font-semibold leading-6 text-gray-950 dark:text-white">{{ selectedApp.name }}</h2>
                </div>
              </div>
              <p class="mt-4 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ selectedApp.description || '暂无描述' }}</p>

              <div class="mt-5 space-y-4 border-t border-gray-100 pt-5 dark:border-dark-700">
                <div v-if="!hasModelPolicies">
                  <label class="input-label">运行使用的 Key</label>
                  <Select v-model="selectedApiKeyId" :options="apiKeyOptions" searchable>
                    <template #selected="{ option }">
                      <span v-if="option?.apiKey" class="flex min-w-0 items-center gap-2">
                        <span class="truncate font-medium">{{ apiKeyOptionName(option) }}</span>
                        <span class="hidden truncate text-xs text-gray-500 dark:text-gray-400 sm:inline">
                          {{ option.groupLabel }} · {{ option.rateLabel }}
                        </span>
                      </span>
                      <span v-else>{{ option?.label || '选择 API Key' }}</span>
                    </template>
                    <template #option="{ option, selected }">
                      <div v-if="option?.apiKey" class="min-w-0 flex-1">
                        <div class="flex min-w-0 items-center gap-2">
                          <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ apiKeyOptionName(option) }}</span>
                          <span class="flex-shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-800 dark:text-gray-300">{{ option.maskLabel }}</span>
                        </div>
                        <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                          <span>{{ option.groupLabel }}</span>
                          <span>·</span>
                          <span>{{ option.platformLabel }}</span>
                          <span>·</span>
                          <span>{{ option.rateLabel }}</span>
                        </div>
                      </div>
                      <span v-else class="select-option-label">{{ option?.label }}</span>
                      <Icon v-if="selected" name="check" size="sm" class="flex-shrink-0 text-primary-500" :stroke-width="2" />
                    </template>
                  </Select>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    本次运行会使用这个 Key 走平台代理调用模型，并产生正常使用记录和扣费记录。
                  </p>
                  <div v-if="selectedDefaultApiKey" class="mt-2 grid grid-cols-1 gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs dark:border-dark-700 dark:bg-dark-900/60 sm:grid-cols-3">
                    <div>
                      <div class="text-gray-500 dark:text-gray-400">所属分组</div>
                      <div class="mt-1 truncate font-medium text-gray-800 dark:text-gray-100">{{ apiKeyGroupLabel(selectedDefaultApiKey) }}</div>
                    </div>
                    <div>
                      <div class="text-gray-500 dark:text-gray-400">平台</div>
                      <div class="mt-1 font-medium text-gray-800 dark:text-gray-100">{{ apiKeyPlatformLabel(selectedDefaultApiKey) }}</div>
                    </div>
                    <div>
                      <div class="text-gray-500 dark:text-gray-400">扣费倍率</div>
                      <div class="mt-1 font-medium text-gray-800 dark:text-gray-100">{{ apiKeyRateLabel(selectedDefaultApiKey) }}</div>
                    </div>
                  </div>
                </div>

                <div v-if="inputFields.length" class="space-y-4">
                  <component
                    :is="isAcademicPaperApp ? 'details' : 'div'"
                    v-for="group in inputFieldGroups"
                    :key="group.key"
                    :open="isAcademicPaperApp && group.defaultOpen"
                    :class="isAcademicPaperApp ? 'rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/30' : ''"
                  >
                    <summary
                      v-if="isAcademicPaperApp"
                      class="cursor-pointer select-none px-4 py-3 text-sm font-semibold text-gray-900 dark:text-white"
                    >
                      {{ group.label }}
                      <span class="ml-1 text-xs font-normal text-gray-500 dark:text-gray-400">{{ group.fields.length }} 项</span>
                    </summary>
                    <div :class="isAcademicPaperApp ? 'space-y-4 border-t border-gray-100 p-4 dark:border-dark-700' : 'space-y-4'">
                      <div
                        v-for="field in group.fields"
                        :key="field.name"
                      >
                    <label class="input-label">
                      {{ field.label }}
                      <span v-if="field.required" class="text-red-500">*</span>
                    </label>
                    <div v-if="isAcademicOutlineField(field)" class="space-y-3">
                      <textarea
                        v-model="outlineImportText"
                        rows="5"
                        class="input resize-y"
                        placeholder="1 绪论&#10;1.1 研究背景&#10;1.2 研究目的与意义&#10;2 研究设计"
                        @input="outlineImportError = ''"
                      />
                      <div class="flex flex-wrap items-center justify-between gap-2">
                        <span class="text-xs text-gray-500 dark:text-gray-400">
                          {{ outlineNodes.length }} 个标题 · 最高 {{ outlineMaxLevel }} 级
                        </span>
                        <button type="button" class="btn btn-secondary btn-sm" @click="importOutlineRequirements">
                          <Icon name="clipboard" size="sm" class="mr-1.5" />
                          解析为目录
                        </button>
                      </div>
                      <p v-if="outlineImportError" class="text-xs leading-5 text-red-600 dark:text-red-300">
                        {{ outlineImportError }}
                      </p>

                      <div class="border-y border-gray-200 dark:border-dark-700">
                        <div
                          v-for="(node, nodeIndex) in outlineNodes"
                          :key="node.id"
                          class="border-b border-gray-100 py-3 last:border-b-0 dark:border-dark-700/70"
                        >
                          <div
                            class="flex min-w-0 items-center gap-2"
                            :style="{ paddingLeft: `${(node.level - 1) * 12}px` }"
                          >
                            <span class="w-12 flex-shrink-0 text-right text-xs font-medium tabular-nums text-gray-500 dark:text-gray-400">
                              {{ outlineNumberLabels[nodeIndex] }}
                            </span>
                            <input
                              v-model="node.title"
                              class="input min-w-0 flex-1"
                              :placeholder="`${node.level} 级标题`"
                              @input="syncOutlineLegacyText"
                            />
                          </div>
                          <div class="mt-2 flex flex-wrap justify-end gap-1">
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm"
                              :disabled="node.level >= 5"
                              title="添加下级标题"
                              aria-label="添加下级标题"
                              @click="addOutlineChild(nodeIndex)"
                            >
                              <Icon name="plus" size="sm" />
                            </button>
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm"
                              :disabled="!canPromoteOutlineNode(nodeIndex)"
                              title="提升一级"
                              aria-label="提升一级"
                              @click="promoteOutlineNode(nodeIndex)"
                            >
                              <Icon name="chevronLeft" size="sm" />
                            </button>
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm"
                              :disabled="!canDemoteOutlineNode(nodeIndex)"
                              title="降低一级"
                              aria-label="降低一级"
                              @click="demoteOutlineNode(nodeIndex)"
                            >
                              <Icon name="chevronRight" size="sm" />
                            </button>
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm"
                              :disabled="!canMoveOutlineNodeUp(nodeIndex)"
                              title="上移"
                              aria-label="上移"
                              @click="moveOutlineNodeUp(nodeIndex)"
                            >
                              <Icon name="arrowUp" size="sm" />
                            </button>
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm"
                              :disabled="!canMoveOutlineNodeDown(nodeIndex)"
                              title="下移"
                              aria-label="下移"
                              @click="moveOutlineNodeDown(nodeIndex)"
                            >
                              <Icon name="arrowDown" size="sm" />
                            </button>
                            <button
                              type="button"
                              class="btn btn-secondary btn-icon btn-sm text-red-600 hover:text-red-700 dark:text-red-300"
                              title="删除标题"
                              aria-label="删除标题"
                              @click="removeOutlineNode(nodeIndex)"
                            >
                              <Icon name="trash" size="sm" />
                            </button>
                          </div>
                        </div>
                        <div v-if="outlineNodes.length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                          暂无目录标题
                        </div>
                      </div>

                      <div class="flex justify-end">
                        <button type="button" class="btn btn-secondary btn-sm" @click="addOutlineRoot">
                          <Icon name="plus" size="sm" class="mr-1.5" />
                          添加一级标题
                        </button>
                      </div>
                      <p v-if="outlineValidationMessage" class="text-xs leading-5 text-red-600 dark:text-red-300">
                        {{ outlineValidationMessage }}
                      </p>
                    </div>
                    <textarea
                      v-else-if="field.kind === 'textarea'"
                      v-model="inputValues[field.name]"
                      rows="5"
                      class="input resize-y"
                      :placeholder="inputPlaceholder(field)"
                    />
                    <input
                      v-else-if="field.kind === 'number'"
                      v-model="inputValues[field.name]"
                      type="number"
                      class="input"
                      :placeholder="inputPlaceholder(field)"
                    />
                    <Select
                      v-else-if="field.kind === 'select'"
                      v-model="inputValues[field.name]"
                      :options="inputSelectOptions(field)"
                    />
                    <Select
                      v-else-if="field.kind === 'boolean'"
                      v-model="inputValues[field.name]"
                      :options="inputBooleanOptions(field)"
                    />
                    <input
                      v-else-if="field.kind === 'date'"
                      v-model="inputValues[field.name]"
                      type="date"
                      class="input"
                    />
                    <div v-else-if="isAssetInputField(field)" class="space-y-2">
                      <label class="flex min-h-[116px] cursor-pointer flex-col items-center justify-center rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-5 text-center transition-colors hover:border-primary-400 hover:bg-primary-50/50 dark:border-dark-600 dark:bg-dark-900/60 dark:hover:border-primary-500 dark:hover:bg-primary-900/10">
                        <Icon name="upload" size="lg" class="text-gray-400 dark:text-gray-500" />
                        <span class="mt-2 text-sm font-medium text-gray-800 dark:text-gray-100">{{ assetInputActionLabel(field) }}</span>
                        <span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ field.required ? '必填' : '可选' }}</span>
                        <input
                          type="file"
                          class="sr-only"
                          :accept="inputFileAccept(field)"
                          multiple
                          @change="handleInputFilesSelected(field, $event)"
                        />
                      </label>
                      <div v-if="inputFiles[field.name]?.length" class="space-y-1">
                        <div
                          v-for="(file, fileIndex) in inputFiles[field.name]"
                          :key="`${file.name}-${file.size}-${file.lastModified}`"
                          class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-900 dark:text-gray-300"
                        >
                          <span class="flex min-w-0 items-center gap-3">
                            <img
                              v-if="field.kind === 'image'"
                              :src="inputFilePreviewURL(field.name, fileIndex, file)"
                              :alt="file.name"
                              class="h-12 w-12 flex-shrink-0 rounded object-cover"
                            />
                            <video
                              v-else-if="field.kind === 'video'"
                              :src="inputFilePreviewURL(field.name, fileIndex, file)"
                              class="h-16 w-24 flex-shrink-0 rounded bg-black object-contain"
                              controls
                              preload="metadata"
                            />
                            <audio
                              v-else-if="field.kind === 'audio'"
                              :src="inputFilePreviewURL(field.name, fileIndex, file)"
                              class="w-48 max-w-full flex-shrink-0"
                              controls
                              preload="metadata"
                            />
                            <span class="min-w-0">
                              <span class="block truncate">{{ file.name }}</span>
                              <span class="mt-1 block text-gray-400">{{ formatBytes(file.size) }}</span>
                            </span>
                          </span>
                          <button type="button" class="btn btn-secondary btn-icon btn-sm flex-shrink-0" title="移除" @click="removeInputFile(field.name, fileIndex)">
                            <Icon name="x" size="sm" />
                          </button>
                        </div>
                      </div>
                    </div>
                    <input
                      v-else
                      v-model="inputValues[field.name]"
                      class="input"
                      :placeholder="inputPlaceholder(field)"
                    />
                      </div>
                    </div>
                  </component>
                  </div>

                <div v-else>
                  <label class="input-label">需求说明</label>
                  <textarea
                    v-model="inputText"
                    rows="7"
                    class="input resize-y"
                    placeholder="输入你想处理的内容"
                  />
                </div>

                <section v-if="hasModelPolicies" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                  <div class="mb-3 flex flex-wrap items-start justify-between gap-3">
                    <div class="min-w-0">
                      <div class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                        <Icon name="key" size="sm" class="text-primary-500" />
                        <span>选择模型 Key</span>
                      </div>
                      <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                        这个应用会调用 {{ modelPolicyItems.length }} 个模型能力。每一项只显示符合管理员分组要求的 Key，并会按所选 Key 的分组倍率计费。
                      </p>
                    </div>
                    <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingKeys" @click="autoFillPolicyKeys">
                      <Icon name="refresh" size="sm" class="mr-1.5" />
                      自动选择
                    </button>
                  </div>

                  <div class="space-y-3">
                    <div
                      v-for="item in modelPolicyItems"
                      :key="item.policyKey"
                      class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/60"
                    >
                      <div class="mb-2 flex items-start justify-between gap-2">
                        <div class="min-w-0">
                          <div class="flex flex-wrap items-center gap-2">
                            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ policyUserLabel(item) }}</span>
                            <span v-if="item.provider" class="rounded bg-primary-50 px-1.5 py-0.5 text-[11px] text-primary-700 ring-1 ring-primary-100 dark:bg-primary-900/20 dark:text-primary-200 dark:ring-primary-800">
                              {{ providerLabel(item.provider) }}
                            </span>
                            <span v-if="item.model" class="rounded bg-white px-1.5 py-0.5 text-[11px] text-gray-500 ring-1 ring-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:ring-dark-700">
                              {{ item.model }}
                            </span>
                            <span v-if="item.optional" class="badge badge-gray">按需调用</span>
                          </div>
                          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">可选择：{{ policyGroupRequirementLabel(item) }}</div>
                          <div v-if="!item.optional && !hasPolicyKeyOption(item)" class="mt-1 text-xs text-red-600 dark:text-red-300">
                            你当前没有满足该厂商和分组要求的可用 Key，请先去 API 密钥页面创建或启用对应厂商分组的 Key。
                          </div>
                        </div>
                        <span v-if="!item.optional && !hasPolicyKeyOption(item)" class="badge badge-danger">无可用 Key</span>
                      </div>
                      <button
                        v-if="!item.optional && !hasPolicyKeyOption(item)"
                        type="button"
                        class="btn btn-secondary btn-sm mb-2"
                        @click="goToApiKeys"
                      >
                        去创建 API Key
                      </button>
                      <Select v-model="policyApiKeySelections[item.policyKey]" :options="apiKeyOptionsForPolicy(item)" searchable>
                        <template #selected="{ option }">
                          <span v-if="option?.apiKey" class="flex min-w-0 items-center gap-2">
                            <span class="truncate font-medium">{{ apiKeyOptionName(option) }}</span>
                            <span class="hidden truncate text-xs text-gray-500 dark:text-gray-400 sm:inline">
                              {{ option.groupLabel }} · {{ option.rateLabel }}
                            </span>
                          </span>
                          <span v-else>{{ option?.label || '选择 API Key' }}</span>
                        </template>
                        <template #option="{ option, selected }">
                          <div v-if="option?.apiKey" class="min-w-0 flex-1">
                            <div class="flex min-w-0 items-center gap-2">
                              <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ apiKeyOptionName(option) }}</span>
                              <span class="flex-shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-800 dark:text-gray-300">{{ option.maskLabel }}</span>
                            </div>
                            <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                              <span>{{ option.groupLabel }}</span>
                              <span>·</span>
                              <span>{{ option.platformLabel }}</span>
                              <span>·</span>
                              <span>{{ option.rateLabel }}</span>
                            </div>
                          </div>
                          <span v-else class="select-option-label">{{ option?.label }}</span>
                          <Icon v-if="selected" name="check" size="sm" class="flex-shrink-0 text-primary-500" :stroke-width="2" />
                        </template>
                      </Select>
                      <div
                        v-if="selectedPolicyApiKey(item)"
                        class="mt-2 grid grid-cols-1 gap-2 rounded-lg border border-white bg-white p-3 text-xs dark:border-dark-700 dark:bg-dark-800/80 sm:grid-cols-3"
                      >
                        <div>
                          <div class="text-gray-500 dark:text-gray-400">所属分组</div>
                          <div class="mt-1 truncate font-medium text-gray-800 dark:text-gray-100">{{ selectedPolicyApiKeyGroupLabel(item) }}</div>
                        </div>
                        <div>
                          <div class="text-gray-500 dark:text-gray-400">平台</div>
                          <div class="mt-1 font-medium text-gray-800 dark:text-gray-100">{{ selectedPolicyApiKeyPlatformLabel(item) }}</div>
                        </div>
                        <div>
                          <div class="text-gray-500 dark:text-gray-400">扣费倍率</div>
                          <div class="mt-1 font-medium text-gray-800 dark:text-gray-100">{{ selectedPolicyApiKeyRateLabel(item) }}</div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="mt-3 rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-xs leading-5 text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/20 dark:text-blue-200">
                    模型请求由 Sub2API 代理发出，Worker 只拿运行授权，不会拿到你的明文 Key。运行成功后可在平台使用记录里查看消耗。
                  </div>
                </section>

                <p v-if="inputError" class="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ inputError }}</p>
                <p v-else-if="missingPolicySelections.length" class="rounded-lg bg-yellow-50 px-3 py-2 text-sm text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300">
                  {{ missingPolicyMessage }}
                </p>
                <p v-else-if="missingRequiredInputs.length" class="rounded-lg bg-yellow-50 px-3 py-2 text-sm text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300">
                  请填写必填项：{{ missingRequiredInputs.map(field => field.label).join('、') }}
                </p>
                <p v-else-if="outlineValidationMessage" class="rounded-lg bg-yellow-50 px-3 py-2 text-sm text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300">
                  {{ outlineValidationMessage }}
                </p>

                <button class="btn btn-primary w-full justify-center" :disabled="submittingRun || !canRunWithInputs" @click="submitRun">
                  <Icon name="play" size="md" class="mr-2" />
                  {{ submittingRun ? (uploadProgressLabel || '提交中...') : '运行应用' }}
                </button>

              </div>
            </div>
            <div v-else class="flex min-h-[420px] flex-col items-center justify-center p-6 text-center">
              <div class="flex h-14 w-14 items-center justify-center rounded-lg bg-gray-100 text-gray-400 dark:bg-dark-800 dark:text-gray-500">
                <Icon name="sparkles" size="lg" />
              </div>
              <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">选择应用</h2>
              <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">左侧选择后即可运行</p>
            </div>
          </section>

        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import StructuredValue from '@/components/agent/StructuredValue.vue'
import agentAppsAPI from '@/api/agentApps'
import usageAPI from '@/api/usage'
import keysAPI from '@/api/keys'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AgentAppCatalog, AgentArtifact, AgentInputAsset, AgentRun, AgentRunEvent, ApiKey, GroupPlatform, PaginatedResponse, UsageLog } from '@/types'
import { agentAppProviderLabel, inferAgentAppProvider, normalizeAgentAppProvider } from '@/utils/agentAppModelProvider'

type ModelPolicyItem = {
  policyKey: string
  nodeId: string
  role: string
  capability: string
  provider: GroupPlatform | ''
  model: string
  modelGroupId: number | null
  optional: boolean
}

type InputFieldKind = 'text' | 'textarea' | 'number' | 'image' | 'file' | 'audio' | 'video' | 'select' | 'boolean' | 'date'

type InputFieldItem = {
  name: string
  label: string
  kind: InputFieldKind
  required: boolean
  assetRole: string
  accept: string
  options: Array<{ label: string; value: string }>
}

type InputFieldGroup = {
  key: string
  label: string
  fields: InputFieldItem[]
  defaultOpen: boolean
}

type OutlineNode = {
  id: string
  title: string
  level: number
}

type OutlineNodeTemplate = Omit<OutlineNode, 'id'>

type ApiKeyOptionItem = {
  label: string
  value: number | ''
  apiKey?: ApiKey
  groupLabel?: string
  platformLabel?: string
  rateLabel?: string
  maskLabel?: string
}

const appStore = useAppStore()
const router = useRouter()

const apps = ref<AgentAppCatalog[]>([])
const runs = ref<AgentRun[]>([])
const runEvents = ref<AgentRunEvent[]>([])
const runUsageLogs = ref<UsageLog[]>([])
const apiKeys = ref<ApiKey[]>([])
const userGroupRates = ref<Record<number, number>>({})
const selectedApp = ref<AgentAppCatalog | null>(null)
const isProductMarketingApp = computed(() => selectedApp.value?.slug === 'ai-product-marketing')
const isAcademicPaperApp = computed(() => {
  const version = selectedApp.value?.published_version
  const inputProperties = version?.input_schema_json?.properties
  const outputProperties = version?.output_schema_json?.properties
  const paperInputs = inputProperties && typeof inputProperties === 'object'
    ? inputProperties as Record<string, unknown>
    : {}
  const paperOutputs = outputProperties && typeof outputProperties === 'object'
    ? outputProperties as Record<string, unknown>
    : {}
  return selectedApp.value?.slug === 'ai-academic-paper' || (
    'topic' in paperInputs && 'word_count' in paperInputs && ('document' in paperOutputs || 'quality_report' in paperOutputs)
  )
})
const isBundledResultApp = computed(() => isProductMarketingApp.value || isAcademicPaperApp.value)
const selectedRun = ref<AgentRun | null>(null)
const runResultSection = ref<HTMLElement | null>(null)

const searchQuery = ref('')
const typeFilter = ref('')
const selectedApiKeyId = ref<number | ''>('')
const policyApiKeySelections = ref<Record<string, number | ''>>({})
const inputText = ref('')
const inputValues = ref<Record<string, string>>({})
const outlineNodes = ref<OutlineNode[]>([])
const outlineImportText = ref('')
const outlineImportError = ref('')
const inputFiles = ref<Record<string, File[]>>({})
const inputFilePreviewURLs = ref<Record<string, string>>({})
const uploadedInputAssetIDs = ref<Record<string, number>>({})
const artifactPreviewURLs = ref<Record<number, string>>({})
const artifactPreviewExpiresAt = ref<Record<number, number>>({})
const inputAssetPreviewURLs = ref<Record<number, string>>({})
const inputAssetPreviewExpiresAt = ref<Record<number, number>>({})
const appIconURLs = ref<Record<number, string>>({})
const appIconExpiresAt = ref<Record<number, number>>({})
const inputError = ref('')
const uploadProgressLabel = ref('')

const loadingApps = ref(false)
const loadingRuns = ref(false)
const loadingRunEvents = ref(false)
const loadingKeys = ref(false)
const submittingRun = ref(false)
const cancelingRunId = ref<number | null>(null)
const pollingRunId = ref<number | null>(null)
let runPollTimer: number | null = null
let runUsageRetryTimer: number | null = null
let runUsageRetryCount = 0

const appPagination = ref({ page: 1, page_size: 12, total: 0, pages: 1 })
const runPagination = ref({ page: 1, page_size: 10, total: 0, pages: 1 })

const typeOptions = [
  { label: '全部类型', value: '' },
  { label: '智能体', value: 'agent' },
  { label: '工作流', value: 'workflow' },
  { label: '提示词', value: 'prompt' },
  { label: '外部应用', value: 'external' }
]

const defaultAcademicOutline: OutlineNodeTemplate[] = [
  { title: '绪论', level: 1 },
  { title: '研究背景', level: 2 },
  { title: '研究目的与意义', level: 2 },
  { title: '文献综述与理论基础', level: 1 },
  { title: '国内外研究现状', level: 2 },
  { title: '核心概念与理论基础', level: 2 },
  { title: '研究设计', level: 1 },
  { title: '研究方法', level: 2 },
  { title: '数据来源与分析方法', level: 2 },
  { title: '研究结果与分析', level: 1 },
  { title: '结论与建议', level: 1 }
]

const apiKeyOptions = computed<ApiKeyOptionItem[]>(() => [
  { label: loadingKeys.value ? '加载中...' : '选择 API Key', value: '' },
  ...apiKeys.value
    .filter((key) => key.status === 'active')
    .map(apiKeyToOption)
])

const modelPolicyItems = computed<ModelPolicyItem[]>(() => {
  const raw = selectedApp.value?.published_version?.node_model_policy_json
  if (!raw || typeof raw !== 'object') return []
  return Object.entries(raw as Record<string, unknown>)
    .map(([policyKey, value]) => normalizeModelPolicyItem(policyKey, value))
    .filter((item): item is ModelPolicyItem => item !== null)
})

const inputFields = computed<InputFieldItem[]>(() => normalizeInputFields(selectedApp.value?.published_version?.input_schema_json)
  .filter(field => !(isAcademicPaperApp.value && field.name === 'outline_spec')))
const outlineNumberLabels = computed(() => buildOutlineNumberLabels(outlineNodes.value))
const outlineMaxLevel = computed(() => outlineNodes.value.length
  ? outlineNodes.value.reduce((maximum, node) => Math.max(maximum, node.level), 1)
  : 0)
const outlineImportPending = computed(() => isAcademicPaperApp.value &&
  outlineImportText.value.trim() !== outlineNodesToText(outlineNodes.value).trim())
const outlineValidationMessage = computed(() => {
  if (!isAcademicPaperApp.value) return ''
  if (outlineImportPending.value) return '目录文本已修改，请先解析为目录并确认预览'
  return validateOutlineNodes(outlineNodes.value)
})
const inputFieldGroups = computed<InputFieldGroup[]>(() => {
  if (!isAcademicPaperApp.value) {
    return [{ key: 'all', label: '', fields: inputFields.value, defaultOpen: true }]
  }
  const groups: InputFieldGroup[] = [
    { key: 'content', label: '论文内容与写作目标', fields: [], defaultOpen: true },
    { key: 'structure', label: '目录结构与章节要求', fields: [], defaultOpen: true },
    { key: 'sources', label: '引用规范与参考资料', fields: [], defaultOpen: true },
    { key: 'page', label: '页面、封面与目录', fields: [], defaultOpen: false },
    { key: 'title', label: '论文标题格式', fields: [], defaultOpen: false },
    { key: 'body', label: '正文格式', fields: [], defaultOpen: false },
    { key: 'abstract', label: '摘要与关键词', fields: [], defaultOpen: false },
    { key: 'heading1', label: '一级标题格式', fields: [], defaultOpen: false },
    { key: 'heading2', label: '二级标题格式', fields: [], defaultOpen: false },
    { key: 'heading3', label: '三级标题格式', fields: [], defaultOpen: false },
    { key: 'heading4', label: '四级标题格式', fields: [], defaultOpen: false },
    { key: 'heading5', label: '五级标题格式', fields: [], defaultOpen: false },
    { key: 'references', label: '参考文献格式', fields: [], defaultOpen: false },
    { key: 'backMatter', label: '致谢与附录', fields: [], defaultOpen: false },
    { key: 'headerFooter', label: '页眉、页脚与页码', fields: [], defaultOpen: false },
    { key: 'other', label: '其他设置', fields: [], defaultOpen: false }
  ]
  const byKey = new Map(groups.map(group => [group.key, group]))
  for (const field of inputFields.value) {
    const name = field.name.toLowerCase()
    let key = 'other'
    if (name.startsWith('heading1_')) key = 'heading1'
    else if (name.startsWith('heading2_')) key = 'heading2'
    else if (name.startsWith('heading3_')) key = 'heading3'
    else if (name.startsWith('heading4_')) key = 'heading4'
    else if (name.startsWith('heading5_')) key = 'heading5'
    else if (name.startsWith('title_')) key = 'title'
    else if (name.startsWith('body_')) key = 'body'
    else if (name.startsWith('abstract_') || name.startsWith('keyword')) key = 'abstract'
    else if (name.startsWith('references_')) key = 'references'
    else if (name.startsWith('acknowledgement') || name.startsWith('appendix_')) key = 'backMatter'
    else if (
      name.startsWith('page_') || name.startsWith('cover_') || name.startsWith('toc_') ||
      name.startsWith('pagination_') || name.startsWith('heading_numbering_') || name === 'format_preset'
    ) key = 'page'
    else if (name.startsWith('header_') || name.startsWith('footer_') || name.startsWith('page_number_')) key = 'headerFooter'
    else if (
      name.startsWith('citation_') || name.startsWith('reference_') || name === 'template_file'
    ) key = 'sources'
    else if (name.includes('outline') || name.includes('directory')) key = 'structure'
    else if (
      [
        'topic', 'paper_title', 'paper_type', 'discipline', 'education_level', 'language', 'word_count',
        'writing_requirements', 'writing_style', 'research_method', 'additional_requirements'
      ].includes(name)
    ) key = 'content'
    byKey.get(key)?.fields.push(field)
  }
  return groups.filter(group => group.fields.length > 0)
})

const missingPolicySelections = computed(() =>
  modelPolicyItems.value.filter((item) => !item.optional).filter((item) => {
    const selected = policyApiKeySelections.value[item.policyKey]
    return selected === '' || selected == null || !activeApiKeysForPolicy(item).some((key) => key.id === Number(selected))
  })
)

const hasModelPolicies = computed(() => modelPolicyItems.value.length > 0)
const runApiKeyId = computed<number | ''>(() => {
  if (!hasModelPolicies.value) return selectedApiKeyId.value
  for (const item of modelPolicyItems.value) {
    const selected = policyApiKeySelections.value[item.policyKey]
    if (selected !== '' && selected != null && activeApiKeysForPolicy(item).some((key) => key.id === Number(selected))) {
      return Number(selected)
    }
  }
  return ''
})
const canRun = computed(() => Boolean(selectedApp.value) && runApiKeyId.value !== '' && missingPolicySelections.value.length === 0)
const missingRequiredInputs = computed(() => inputFields.value.filter(field => {
  if (!field.required) return false
  if (isAssetInputField(field)) return !(inputFiles.value[field.name] || []).length
  return String(inputValues.value[field.name] ?? '').trim() === ''
}))
const inputsValid = computed(() => missingRequiredInputs.value.length === 0 && outlineValidationMessage.value === '')
const canRunWithInputs = computed(() => canRun.value && inputsValid.value)
const inputFormDirty = computed(() => {
  const valuesDirty = isAcademicPaperApp.value
    ? inputFields.value.some(field => !isAssetInputField(field) && String(inputValues.value[field.name] ?? '').trim() !== academicPaperInputDefault(field))
    : Object.values(inputValues.value).some(value => String(value || '').trim() !== '')
  return inputText.value.trim() !== '' || valuesDirty || outlineImportPending.value || Object.values(inputFiles.value).some(files => files.length > 0)
})
const maxInputFileBytes = computed(() => {
  const maxFileMB = Number(selectedApp.value?.published_version?.artifact_policy_json?.max_file_mb)
  return Number.isFinite(maxFileMB) && maxFileMB > 0 ? maxFileMB * 1024 * 1024 : 100 * 1024 * 1024
})
const runUsageTotalTokens = computed(() => runUsageLogs.value.reduce((total, log) => total + usageLogTokens(log), 0).toLocaleString())
const runUsageActualCost = computed(() => runUsageLogs.value.reduce((total, log) => total + Number(log.actual_cost || 0), 0).toFixed(6))
const selectedRunInputItems = computed(() => runInputItems(selectedRun.value))
const selectedRunInputAssets = computed(() => runInputAssets(selectedRun.value))

function usageLogTokens(log: UsageLog): number {
  return Number(log.input_tokens || 0) + Number(log.output_tokens || 0) + Number(log.cache_creation_tokens || 0) + Number(log.cache_read_tokens || 0)
}

function usageLogMeasure(log: UsageLog): string {
  const videoCount = Number(log.video_count || 0)
  if (videoCount > 0) {
    const parts = [`${videoCount} 个视频`]
    const duration = Number(log.video_duration_seconds || 0)
    if (duration > 0) parts.push(`${duration} 秒`)
    if (log.video_resolution) parts.push(log.video_resolution)
    return parts.join(' · ')
  }
  const imageCount = Number(log.image_count || 0)
  if (imageCount > 0) return `${imageCount} 张图片`
  const tokens = usageLogTokens(log)
  if (tokens > 0) return `${tokens} Token`
  if (String(log.media_type || '').toLowerCase().startsWith('audio')) return '音频调用'
  return '模型调用'
}
const selectedRunPolling = computed(() => pollingRunId.value != null && pollingRunId.value === selectedRun.value?.id)
const selectedDefaultApiKey = computed(() => apiKeys.value.find((key) => key.id === Number(selectedApiKeyId.value)) || null)
const missingPolicyMessage = computed(() => {
  if (!missingPolicySelections.value.length) return ''
  const withoutKey = missingPolicySelections.value.filter((item) => !hasPolicyKeyOption(item))
  const targets = (withoutKey.length ? withoutKey : missingPolicySelections.value)
    .slice(0, 3)
    .map((item) => `${policyUserLabel(item)}（${policyGroupRequirementLabel(item)}）`)
  const suffix = missingPolicySelections.value.length > 3 ? `等 ${missingPolicySelections.value.length} 项` : ''
  if (withoutKey.length) {
    return `当前账号缺少这些厂商/分组要求下的可用 Key：${targets.join('、')}${suffix ? `，${suffix}` : ''}。请先去 API 密钥页面创建或启用对应厂商分组的 Key。`
  }
  return `请先为这些模型能力选择 Key：${targets.join('、')}${suffix ? `，${suffix}` : ''}。`
})

function refreshWorkspace() {
  loadApps()
  loadRuns()
  if (selectedRun.value) {
    refreshSelectedRun(selectedRun.value.id, { silent: false })
  }
}

function goToApiKeys() {
  router.push('/keys')
}

watch(typeFilter, () => {
  appPagination.value.page = 1
  loadApps()
})

watch([modelPolicyItems, apiKeys], () => {
  syncPolicyApiKeySelections()
}, { immediate: true })

async function loadApps() {
  loadingApps.value = true
  try {
    const data: PaginatedResponse<AgentAppCatalog> = await agentAppsAPI.listApps(
      appPagination.value.page,
      appPagination.value.page_size,
      {
        search: searchQuery.value.trim() || undefined,
        app_type: typeFilter.value || undefined,
        sort_by: 'created_at',
        sort_order: 'desc'
      }
    )
    apps.value = data.items
    await ensureAppIconURLs(apps.value)
    appPagination.value = {
      page: data.page,
      page_size: data.page_size,
      total: data.total,
      pages: data.pages
    }
    if (!selectedApp.value && apps.value.length > 0) {
      await selectApp(apps.value[0])
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '应用列表加载失败'))
  } finally {
    loadingApps.value = false
  }
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    const [items, rates] = await Promise.all([
      loadAllActiveKeys(),
      userGroupsAPI.getUserGroupRates().catch(() => ({} as Record<number, number>))
    ])
    apiKeys.value = items
    userGroupRates.value = rates
    if (selectedApiKeyId.value === '') {
      selectedApiKeyId.value = recommendedApiKey(items)?.id || ''
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, 'API Key 加载失败'))
  } finally {
    loadingKeys.value = false
  }
}

async function loadAllActiveKeys(): Promise<ApiKey[]> {
  const items: ApiKey[] = []
  let page = 1
  let pages = 1
  do {
    const data = await keysAPI.list(page, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    items.push(...data.items)
    pages = data.pages
    page++
  } while (page <= pages && page <= 20)
  return items
}

async function loadRuns(options: { silent?: boolean } = {}) {
  if (!options.silent) loadingRuns.value = true
  try {
    const data = await agentAppsAPI.listRuns(runPagination.value.page, runPagination.value.page_size, {
      app_id: selectedApp.value?.id,
      sort_by: 'created_at',
      sort_order: 'desc'
    })
    runs.value = data.items
    runPagination.value = {
      page: data.page,
      page_size: data.page_size,
      total: data.total,
      pages: data.pages
    }
    if (selectedRun.value) {
      const updated = runs.value.find((run) => run.id === selectedRun.value?.id)
      if (updated) {
        const current = selectedRun.value
        selectedRun.value = {
          ...current,
          ...updated,
          artifacts: current.artifacts
        }
        syncRunPolling(selectedRun.value)
      }
      if (updated) await loadRunEvents(updated.id, { silent: options.silent })
    }
  } catch (err: unknown) {
    if (!options.silent) appStore.showError(extractApiErrorMessage(err, '运行记录加载失败'))
  } finally {
    if (!options.silent) loadingRuns.value = false
  }
}

async function selectApp(app: AgentAppCatalog) {
  if (selectedApp.value && selectedApp.value.id !== app.id && inputFormDirty.value && !window.confirm('切换应用会清空当前尚未提交的内容，确认继续吗？')) {
    return
  }
  try {
    selectedApp.value = await agentAppsAPI.getApp(app.id)
    await ensureAppIconURLs([selectedApp.value])
  } catch (err: unknown) {
    selectedApp.value = app
    appStore.showError(extractApiErrorMessage(err, '应用详情加载失败'))
  }
  selectedRun.value = null
  runEvents.value = []
  runUsageLogs.value = []
  stopRunUsageRetry()
  stopRunPolling()
  policyApiKeySelections.value = {}
  resetInputForm()
  syncPolicyApiKeySelections()
  runPagination.value.page = 1
  await loadRuns()
}

async function ensureAppIconURLs(items: AgentAppCatalog[]) {
  const now = Date.now()
  const targets = items.filter(app => app.icon_url && !app.icon_url.startsWith('http') && (
    !appIconURLs.value[app.id] || (appIconExpiresAt.value[app.id] || 0) < now + 30_000
  ))
  if (!targets.length) return
  const entries = await Promise.all(targets.map(async app => {
    try {
      const result = await agentAppsAPI.getAppIconURL(app.id)
      return [app.id, result.url, result.expires_at ? new Date(result.expires_at).getTime() : now + 5 * 60_000] as const
    } catch {
      return null
    }
  }))
  const next = { ...appIconURLs.value }
  const nextExpires = { ...appIconExpiresAt.value }
  for (const entry of entries) {
    if (entry) {
      next[entry[0]] = entry[1]
      nextExpires[entry[0]] = entry[2]
    }
  }
  appIconURLs.value = next
  appIconExpiresAt.value = nextExpires
}

function appDisplayIconURL(app: AgentAppCatalog): string {
  return appIconURLs.value[app.id] || app.icon_url || ''
}

async function selectRun(run: AgentRun) {
  stopRunUsageRetry()
  try {
    selectedRun.value = await agentAppsAPI.getRun(run.id)
    await ensureRunPreviewURLs(selectedRun.value)
    await loadRunEvents(run.id)
    await loadRunUsage(run.id)
    syncRunPolling(selectedRun.value)
  } catch (err: unknown) {
    selectedRun.value = run
    runEvents.value = []
    runUsageLogs.value = []
    syncRunPolling(run)
    appStore.showError(extractApiErrorMessage(err, '运行详情加载失败'))
  }
  await scrollToRunResult()
}

async function scrollToRunResult() {
  await nextTick()
  runResultSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

async function loadRunEvents(runId = selectedRun.value?.id, options: { silent?: boolean } = {}) {
  if (!runId) {
    runEvents.value = []
    return
  }
  if (!options.silent) loadingRunEvents.value = true
  try {
    const data = await agentAppsAPI.listRunEvents(runId, 1, 100)
    runEvents.value = data.items
  } catch (err: unknown) {
    runEvents.value = []
    if (!options.silent) appStore.showError(extractApiErrorMessage(err, '运行事件加载失败'))
  } finally {
    if (!options.silent) loadingRunEvents.value = false
  }
}

async function refreshSelectedRun(runId = selectedRun.value?.id, options: { silent?: boolean } = {}) {
  if (!runId) return
  try {
    const latest = await agentAppsAPI.getRun(runId)
    if (selectedRun.value?.id === runId || !selectedRun.value) {
      selectedRun.value = latest
    }
    await ensureRunPreviewURLs(latest)
    await loadRunEvents(runId, { silent: options.silent })
    if (isTerminalRunStatus(latest.status)) await loadRunUsage(runId, { silent: true })
    await loadRuns({ silent: true })
    syncRunPolling(latest)
  } catch (err: unknown) {
    if (!options.silent) appStore.showError(extractApiErrorMessage(err, '运行详情刷新失败'))
  }
}

async function loadRunUsage(runId: number, options: { silent?: boolean } = {}) {
  try {
    const items: UsageLog[] = []
    let page = 1
    let pages = 1
    do {
      const data = await usageAPI.listByAgentRun(runId, page, 100)
      items.push(...data.items)
      pages = Math.max(1, data.pages)
      page += 1
    } while (page <= pages && page <= 100)
    if (selectedRun.value?.id !== runId) return
    runUsageLogs.value = items
    if (items.length === 0 && isTerminalRunStatus(selectedRun.value?.status) && runUsageRetryCount < 3) {
      runUsageRetryCount++
      runUsageRetryTimer = window.setTimeout(() => loadRunUsage(runId, { silent: true }), 1500)
    } else if (items.length > 0) {
      stopRunUsageRetry()
    }
  } catch (err: unknown) {
    runUsageLogs.value = []
    if (!options.silent) appStore.showError(extractApiErrorMessage(err, '本次使用记录加载失败'))
  }
}

function stopRunUsageRetry() {
  if (runUsageRetryTimer) window.clearTimeout(runUsageRetryTimer)
  runUsageRetryTimer = null
  runUsageRetryCount = 0
}

function isTerminalRunStatus(status?: string): boolean {
  return status === 'succeeded' || status === 'failed' || status === 'canceled' || status === 'timeout'
}

function shouldPollRun(run: AgentRun | null): boolean {
  return Boolean(run && !isTerminalRunStatus(run.status))
}

function syncRunPolling(run: AgentRun | null) {
  if (!shouldPollRun(run)) {
    if (pollingRunId.value === run?.id) stopRunPolling()
    return
  }
  startRunPolling(run!.id)
}

function startRunPolling(runId = selectedRun.value?.id) {
  if (!runId) return
  if (pollingRunId.value === runId && runPollTimer) return
  stopRunPolling()
  pollingRunId.value = runId
  scheduleRunPoll(runId)
}

function scheduleRunPoll(runId: number) {
  runPollTimer = window.setTimeout(async () => {
    if (pollingRunId.value !== runId) return
    await refreshSelectedRun(runId, { silent: true })
    if (pollingRunId.value !== runId) return
    if (shouldPollRun(selectedRun.value)) {
      scheduleRunPoll(runId)
    } else {
      stopRunPolling()
    }
  }, 2000)
}

function stopRunPolling() {
  if (runPollTimer) {
    window.clearTimeout(runPollTimer)
    runPollTimer = null
  }
  pollingRunId.value = null
}

async function submitRun() {
  if (!selectedApp.value) return
  const billingApiKeyId = runApiKeyId.value
  if (billingApiKeyId === '') {
    inputError.value = hasModelPolicies.value ? '请先选择模型 Key' : '请先选择运行使用的 Key'
    return
  }
  if (missingPolicySelections.value.length > 0) {
    inputError.value = '请为每个模型能力选择可用 API Key'
    return
  }
  if (outlineValidationMessage.value) {
    inputError.value = outlineValidationMessage.value
    return
  }
  if (!inputsValid.value) {
    inputError.value = `请填写必填项：${missingRequiredInputs.value.map(field => field.label).join('、')}`
    return
  }
  submittingRun.value = true
  uploadProgressLabel.value = ''
  try {
    const payload = await buildRunInputPayload()
    if (!payload) return
    const { input, assetIds } = payload
    const run = await agentAppsAPI.createRun(selectedApp.value.id, {
      app_version_id: selectedApp.value.published_version?.id,
      api_key_id: Number(billingApiKeyId),
      api_key_bindings: buildRunAPIKeyBindings(),
      input,
      input_asset_ids: assetIds
    })
    selectedRun.value = run
    await ensureRunPreviewURLs(run)
    await scrollToRunResult()
    appStore.showSuccess('运行已提交')
    await loadRunEvents(run.id)
    await loadRuns()
    syncRunPolling(run)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '运行提交失败'))
  } finally {
    submittingRun.value = false
    uploadProgressLabel.value = ''
  }
}

async function downloadArtifact(artifactId: number) {
  try {
    const result = await agentAppsAPI.getArtifactDownloadURL(artifactId)
    const anchor = document.createElement('a')
    anchor.href = result.url
    anchor.target = '_blank'
    anchor.rel = 'noopener noreferrer'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '产物下载链接获取失败'))
  }
}

async function downloadInputAsset(inputAssetId: number) {
  try {
    const result = await agentAppsAPI.getInputAssetDownloadURL(inputAssetId)
    const anchor = document.createElement('a')
    anchor.href = result.url
    anchor.target = '_blank'
    anchor.rel = 'noopener noreferrer'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '输入文件下载链接获取失败'))
  }
}

async function ensureRunPreviewURLs(run: AgentRun | null) {
  await Promise.all([ensureArtifactPreviewURLs(run), ensureInputAssetPreviewURLs(run)])
}

async function ensureArtifactPreviewURLs(run: AgentRun | null) {
  const now = Date.now()
  const artifacts = (run?.artifacts || []).filter((artifact) => isPreviewableArtifact(artifact) && (
    !artifactPreviewURLs.value[artifact.id] || (artifactPreviewExpiresAt.value[artifact.id] || 0) < now + 30_000
  ))
  if (artifacts.length === 0) return
  const entries = await Promise.all(
    artifacts.map(async (artifact) => {
      try {
        const result = await agentAppsAPI.getArtifactDownloadURL(artifact.id)
        return [artifact.id, result.url, result.expires_at ? new Date(result.expires_at).getTime() : now + 5 * 60_000] as const
      } catch {
        return null
      }
    })
  )
  const next = { ...artifactPreviewURLs.value }
  const nextExpires = { ...artifactPreviewExpiresAt.value }
  for (const entry of entries) {
    if (entry) {
      next[entry[0]] = entry[1]
      nextExpires[entry[0]] = entry[2]
    }
  }
  artifactPreviewURLs.value = next
  artifactPreviewExpiresAt.value = nextExpires
}

async function ensureInputAssetPreviewURLs(run: AgentRun | null) {
  const now = Date.now()
  const assets = runInputAssets(run).filter((asset) => isPreviewableInputAsset(asset) && (
    !inputAssetPreviewURLs.value[asset.id] || (inputAssetPreviewExpiresAt.value[asset.id] || 0) < now + 30_000
  ))
  if (assets.length === 0) return
  const entries = await Promise.all(
    assets.map(async (asset) => {
      try {
        const result = await agentAppsAPI.getInputAssetDownloadURL(asset.id)
        return [asset.id, result.url, result.expires_at ? new Date(result.expires_at).getTime() : now + 5 * 60_000] as const
      } catch {
        return null
      }
    })
  )
  const next = { ...inputAssetPreviewURLs.value }
  const nextExpires = { ...inputAssetPreviewExpiresAt.value }
  for (const entry of entries) {
    if (entry) {
      next[entry[0]] = entry[1]
      nextExpires[entry[0]] = entry[2]
    }
  }
  inputAssetPreviewURLs.value = next
  inputAssetPreviewExpiresAt.value = nextExpires
}

function artifactPreviewURL(artifact: AgentArtifact): string {
  return artifactPreviewURLs.value[artifact.id] || ''
}

function inputAssetPreviewURL(asset: AgentInputAsset): string {
  return inputAssetPreviewURLs.value[asset.id] || ''
}

function runResultArtifacts(run: AgentRun | null): AgentArtifact[] {
  return (run?.artifacts || []).filter((artifact) => {
    const type = String(artifact.artifact_type || '').toLowerCase()
    return type !== 'log' && type !== 'input'
  })
}

function runLogArtifacts(run: AgentRun | null): AgentArtifact[] {
  return (run?.artifacts || []).filter((artifact) => String(artifact.artifact_type || '').toLowerCase() === 'log')
}

function isPreviewableArtifact(artifact: AgentArtifact): boolean {
  return isImageArtifact(artifact) || isVideoArtifact(artifact) || isAudioArtifact(artifact)
}

function isPreviewableInputAsset(asset: AgentInputAsset): boolean {
  return isImageInputAsset(asset) || isVideoInputAsset(asset) || isAudioInputAsset(asset)
}

function isImageInputAsset(asset: AgentInputAsset): boolean {
  return inputAssetMime(asset).startsWith('image/')
}

function isVideoInputAsset(asset: AgentInputAsset): boolean {
  return inputAssetMime(asset).startsWith('video/')
}

function isAudioInputAsset(asset: AgentInputAsset): boolean {
  return inputAssetMime(asset).startsWith('audio/')
}

function inputAssetMime(asset: AgentInputAsset): string {
  const direct = String(asset.mime_type || '').trim().toLowerCase()
  if (direct) return direct
  for (const key of ['mime_type', 'content_type', 'media_type']) {
    const value = asset.metadata_json?.[key]
    if (typeof value === 'string' && value.trim()) return value.trim().toLowerCase()
  }
  const extension = asset.name.toLowerCase().split('.').pop() || ''
  const byExtension: Record<string, string> = {
    apng: 'image/apng', avif: 'image/avif', gif: 'image/gif', jpeg: 'image/jpeg', jpg: 'image/jpeg', png: 'image/png', webp: 'image/webp',
    aac: 'audio/aac', flac: 'audio/flac', m4a: 'audio/mp4', mp3: 'audio/mpeg', ogg: 'audio/ogg', opus: 'audio/ogg', wav: 'audio/wav',
    avi: 'video/x-msvideo', m4v: 'video/mp4', mkv: 'video/x-matroska', mov: 'video/quicktime', mp4: 'video/mp4', webm: 'video/webm'
  }
  return byExtension[extension] || ''
}

function isImageArtifact(artifact: AgentArtifact): boolean {
  return artifactMime(artifact).startsWith('image/')
}

function isVideoArtifact(artifact: AgentArtifact): boolean {
  return artifactMime(artifact).startsWith('video/')
}

function isAudioArtifact(artifact: AgentArtifact): boolean {
  return artifactMime(artifact).startsWith('audio/')
}

function isWordArtifact(artifact: AgentArtifact): boolean {
  const mime = artifactMime(artifact)
  return mime === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' || artifact.name.toLowerCase().endsWith('.docx')
}

function artifactMime(artifact: AgentArtifact): string {
  const direct = String(artifact.mime_type || '').trim().toLowerCase()
  if (direct) return direct
  for (const key of ['mime_type', 'content_type', 'media_type']) {
    const value = artifact.metadata_json?.[key]
    if (typeof value === 'string' && value.trim()) return value.trim().toLowerCase()
  }
  const extension = artifact.name.toLowerCase().split('.').pop() || ''
  const byExtension: Record<string, string> = {
    apng: 'image/apng', avif: 'image/avif', gif: 'image/gif', jpeg: 'image/jpeg', jpg: 'image/jpeg', png: 'image/png', webp: 'image/webp',
    aac: 'audio/aac', flac: 'audio/flac', m4a: 'audio/mp4', mp3: 'audio/mpeg', ogg: 'audio/ogg', opus: 'audio/ogg', wav: 'audio/wav',
    avi: 'video/x-msvideo', m4v: 'video/mp4', mkv: 'video/x-matroska', mov: 'video/quicktime', mp4: 'video/mp4', webm: 'video/webm',
    docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  }
  return byExtension[extension] || ''
}

function artifactIconName(artifact: AgentArtifact): 'eye' | 'play' | 'document' {
  if (isImageArtifact(artifact)) return 'eye'
  if (isVideoArtifact(artifact) || isAudioArtifact(artifact)) return 'play'
  return 'document'
}

function canCancelRun(run: AgentRun | null): boolean {
  return run?.status === 'queued' || run?.status === 'running'
}

async function cancelSelectedRun() {
  const run = selectedRun.value
  if (!run || !canCancelRun(run) || cancelingRunId.value === run.id) return
  if (!window.confirm('停止后不会再发起后续模型请求；已经发送的请求可能仍会完成并产生费用。确认停止吗？')) return

  cancelingRunId.value = run.id
  try {
    selectedRun.value = await agentAppsAPI.cancelRun(run.id)
    await ensureRunPreviewURLs(selectedRun.value)
    appStore.showSuccess('已停止后续步骤，正在执行的请求可能仍会完成')
    await loadRunEvents(run.id)
    await loadRuns()
    stopRunPolling()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '取消运行失败'))
  } finally {
    cancelingRunId.value = null
  }
}

async function buildRunInputPayload(): Promise<{ input: Record<string, unknown>; assetIds: number[] } | null> {
  inputError.value = ''
  if (!inputFields.value.length) {
    const prompt = inputText.value.trim()
    return { input: prompt ? { prompt } : {}, assetIds: [] }
  }

  if (outlineValidationMessage.value) {
    inputError.value = outlineValidationMessage.value
    return null
  }

  for (const field of inputFields.value) {
    if (isAssetInputField(field)) {
      const files = inputFiles.value[field.name] || []
      if (field.required && files.length === 0) {
        inputError.value = `${field.label} 为必填`
        return null
      }
      const oversized = files.find(file => file.size > maxInputFileBytes.value)
      if (oversized) {
        inputError.value = `${oversized.name} 超过单文件 ${formatBytes(maxInputFileBytes.value)} 限制`
        return null
      }
      continue
    }
    const raw = String(inputValues.value[field.name] ?? '').trim()
    if (field.required && raw === '') {
      inputError.value = `${field.label} 为必填`
      return null
    }
    if (raw !== '' && field.kind === 'number' && !Number.isFinite(Number(raw))) {
      inputError.value = `${field.label} 不是有效数字`
      return null
    }
  }

  const input: Record<string, unknown> = {}
  const assetIds: number[] = []
  const totalFiles = inputFields.value.reduce((total, field) => total + (isAssetInputField(field) ? (inputFiles.value[field.name] || []).length : 0), 0)
  let uploadedCount = 0
  for (const field of inputFields.value) {
    if (isAssetInputField(field)) {
      const files = inputFiles.value[field.name] || []
      if (field.required && files.length === 0) {
        inputError.value = `${field.label} 为必填`
        return null
      }
      if (files.length === 0) {
        input[field.name] = []
        continue
      }
      const uploaded: AgentInputAsset[] = []
      for (const file of files) {
        const cacheKey = uploadedAssetCacheKey(field.name, file)
        const cachedID = uploadedInputAssetIDs.value[cacheKey]
        if (cachedID) {
          uploaded.push({ id: cachedID } as AgentInputAsset)
          uploadedCount++
          continue
        }
        const asset = await agentAppsAPI.uploadInputAsset(file, {
          app_id: selectedApp.value?.id,
          field_name: field.name,
          asset_type: field.kind,
          asset_role: field.assetRole || undefined,
          metadata: { field_label: field.label },
          onProgress: percent => {
            uploadProgressLabel.value = `上传文件 ${uploadedCount + 1}/${totalFiles} · ${percent}%`
          }
        })
        uploaded.push(asset)
        uploadedInputAssetIDs.value = { ...uploadedInputAssetIDs.value, [cacheKey]: asset.id }
        uploadedCount++
      }
      const ids = uploaded.map((asset) => asset.id)
      input[field.name] = ids
      assetIds.push(...ids)
      continue
    }

    const raw = String(inputValues.value[field.name] ?? '').trim()
    if (field.required && raw === '') {
      inputError.value = `${field.label} 为必填`
      return null
    }
    if (raw === '') {
      continue
    }
    if (field.kind === 'number') {
      const numeric = Number(raw)
      if (!Number.isFinite(numeric)) {
        inputError.value = `${field.label} 不是有效数字`
        return null
      }
      input[field.name] = numeric
    } else if (field.kind === 'boolean') {
      input[field.name] = raw === 'true'
    } else {
      input[field.name] = raw
    }
  }
  if (isAcademicPaperApp.value) {
    const legacyOutline = outlineNodesToText(outlineNodes.value)
    if (inputFields.value.some(field => field.name === 'outline_requirements')) {
      input.outline_requirements = legacyOutline
    }
    input.outline_spec = {
      version: 1,
      nodes: outlineNodes.value.map(node => ({
        id: node.id,
        title: node.title.trim(),
        level: node.level
      }))
    }
  }
  return { input, assetIds }
}

let outlineNodeSequence = 0

function isAcademicOutlineField(field: InputFieldItem): boolean {
  return isAcademicPaperApp.value && field.name === 'outline_requirements'
}

function nextOutlineNodeID(): string {
  outlineNodeSequence += 1
  return `outline-${Date.now().toString(36)}-${outlineNodeSequence.toString(36)}`
}

function createOutlineNode(template: OutlineNodeTemplate): OutlineNode {
  return {
    id: nextOutlineNodeID(),
    title: template.title,
    level: Math.min(5, Math.max(1, Math.trunc(template.level)))
  }
}

function createDefaultOutlineNodes(): OutlineNode[] {
  return defaultAcademicOutline.map(createOutlineNode)
}

function buildOutlineNumberLabels(nodes: Array<Pick<OutlineNode, 'level'>>): string[] {
  const counters = [0, 0, 0, 0, 0]
  return nodes.map((node) => {
    const level = Math.min(5, Math.max(1, Math.trunc(node.level)))
    counters[level - 1] += 1
    for (let index = level; index < counters.length; index++) counters[index] = 0
    return counters.slice(0, level).join('.')
  })
}

function outlineNodesToText(nodes: Array<Pick<OutlineNode, 'title' | 'level'>>): string {
  const numbers = buildOutlineNumberLabels(nodes)
  return nodes
    .map((node, index) => `${numbers[index]} ${node.title.trim()}`.trim())
    .join('\n')
}

function defaultAcademicOutlineText(): string {
  return outlineNodesToText(defaultAcademicOutline)
}

function validateOutlineNodes(nodes: OutlineNode[]): string {
  if (nodes.length === 0) return '请至少添加一个一级标题'
  const ids = new Set<string>()
  for (let index = 0; index < nodes.length; index++) {
    const node = nodes[index]
    if (!node.id || ids.has(node.id)) return `目录第 ${index + 1} 项的标识无效，请重新导入目录`
    ids.add(node.id)
    if (!Number.isInteger(node.level) || node.level < 1 || node.level > 5) {
      return `目录第 ${index + 1} 项的层级必须在 1 到 5 之间`
    }
    if (!node.title.trim()) return `目录第 ${index + 1} 项的标题不能为空`
    if (index === 0 && node.level !== 1) return '目录第 1 项必须是一级标题'
    if (index > 0 && node.level > nodes[index - 1].level + 1) {
      return `目录第 ${index + 1} 项不能从 ${nodes[index - 1].level} 级直接跳到 ${node.level} 级`
    }
  }
  return ''
}

function parseOutlineLine(rawLine: string): OutlineNodeTemplate | null {
  const leadingWhitespace = rawLine.match(/^[\t ]*/)?.[0] || ''
  const line = rawLine.trim()
  let match = line.match(/^(#{1,5})\s+(.+)$/)
  if (match) return { level: match[1].length, title: match[2].trim() }

  match = line.match(/^第[零〇一二三四五六七八九十百千万\d]+章(?:[\s、.．:：-]+)?(.+)$/)
  if (match) return { level: 1, title: match[1].trim() }

  match = line.match(/^第[零〇一二三四五六七八九十百千万\d]+节(?:[\s、.．:：-]+)?(.+)$/)
  if (match) return { level: 2, title: match[1].trim() }

  match = line.match(/^([一二三四五六七八九十百千万]+)[、.．]\s*(.+)$/)
  if (match) return { level: 1, title: match[2].trim() }

  match = line.match(/^[（(]([一二三四五六七八九十百千万]+)[)）]\s*(.+)$/)
  if (match) return { level: 2, title: match[2].trim() }

  match = line.match(/^[（(](\d+)[)）]\s*(.+)$/)
  if (match) return { level: 2, title: match[2].trim() }

  match = line.match(/^(\d+(?:\.\d+){0,4})(?:[.．、:：)）]\s*|\s+)(.+)$/)
  if (match) return { level: match[1].split('.').length, title: match[2].trim() }

  match = line.match(/^[-*+]\s+(.+)$/)
  if (match) {
    const indentationWidth = Array.from(leadingWhitespace).reduce((width, character) => width + (character === '\t' ? 2 : 1), 0)
    return { level: Math.min(5, Math.floor(indentationWidth / 2) + 1), title: match[1].trim() }
  }
  return null
}

function importOutlineRequirements() {
  const sourceLines = outlineImportText.value.replace(/\r\n?/g, '\n').split('\n')
  const parsed: OutlineNode[] = []
  let previousLevel = 0
  for (let sourceIndex = 0; sourceIndex < sourceLines.length; sourceIndex++) {
    const rawLine = sourceLines[sourceIndex]
    if (!rawLine.trim()) continue
    const template = parseOutlineLine(rawLine)
    if (!template || !template.title) {
      outlineImportError.value = `第 ${sourceIndex + 1} 行无法识别，请使用明确的标题编号或 Markdown 标题`
      return
    }
    if (parsed.length === 0 && template.level !== 1) {
      outlineImportError.value = `第 ${sourceIndex + 1} 行必须是一级标题`
      return
    }
    if (parsed.length > 0 && template.level > previousLevel + 1) {
      outlineImportError.value = `第 ${sourceIndex + 1} 行不能从 ${previousLevel} 级直接跳到 ${template.level} 级`
      return
    }
    parsed.push(createOutlineNode(template))
    previousLevel = template.level
  }
  if (parsed.length === 0) {
    outlineImportError.value = '请先输入需要解析的目录文本'
    return
  }
  outlineNodes.value = parsed
  outlineImportError.value = ''
  inputError.value = ''
  syncOutlineLegacyText()
  appStore.showSuccess(`已解析 ${parsed.length} 个标题，请确认目录预览`)
}

function syncOutlineLegacyText() {
  const legacyText = outlineNodesToText(outlineNodes.value)
  inputValues.value.outline_requirements = legacyText
  outlineImportText.value = legacyText
  outlineImportError.value = ''
}

function outlineSubtreeEnd(nodes: OutlineNode[], index: number): number {
  const level = nodes[index]?.level
  if (!level) return index
  let end = index + 1
  while (end < nodes.length && nodes[end].level > level) end += 1
  return end
}

function previousOutlineSiblingStart(nodes: OutlineNode[], index: number): number {
  const level = nodes[index]?.level
  if (!level || index <= 0) return -1
  let candidate = index - 1
  while (candidate >= 0 && nodes[candidate].level > level) candidate -= 1
  return candidate >= 0 && nodes[candidate].level === level ? candidate : -1
}

function canMoveOutlineNodeUp(index: number): boolean {
  return previousOutlineSiblingStart(outlineNodes.value, index) >= 0
}

function canMoveOutlineNodeDown(index: number): boolean {
  const nodes = outlineNodes.value
  const nextIndex = outlineSubtreeEnd(nodes, index)
  return nextIndex < nodes.length && nodes[nextIndex].level === nodes[index]?.level
}

function canPromoteOutlineNode(index: number): boolean {
  return (outlineNodes.value[index]?.level || 1) > 1
}

function canDemoteOutlineNode(index: number): boolean {
  const nodes = outlineNodes.value
  const previousSibling = previousOutlineSiblingStart(nodes, index)
  if (previousSibling < 0) return false
  const subtreeEnd = outlineSubtreeEnd(nodes, index)
  return nodes.slice(index, subtreeEnd).every(node => node.level < 5)
}

function addOutlineRoot() {
  outlineNodes.value = [...outlineNodes.value, createOutlineNode({ title: '', level: 1 })]
  syncOutlineLegacyText()
}

function addOutlineChild(index: number) {
  const parent = outlineNodes.value[index]
  if (!parent || parent.level >= 5) return
  const nodes = [...outlineNodes.value]
  const insertAt = outlineSubtreeEnd(nodes, index)
  nodes.splice(insertAt, 0, createOutlineNode({ title: '', level: parent.level + 1 }))
  outlineNodes.value = nodes
  syncOutlineLegacyText()
}

function promoteOutlineNode(index: number) {
  if (!canPromoteOutlineNode(index)) return
  const nodes = [...outlineNodes.value]
  const subtreeEnd = outlineSubtreeEnd(nodes, index)
  outlineNodes.value = nodes.map((node, nodeIndex) => nodeIndex >= index && nodeIndex < subtreeEnd
    ? { ...node, level: node.level - 1 }
    : node)
  syncOutlineLegacyText()
}

function demoteOutlineNode(index: number) {
  if (!canDemoteOutlineNode(index)) return
  const nodes = [...outlineNodes.value]
  const subtreeEnd = outlineSubtreeEnd(nodes, index)
  outlineNodes.value = nodes.map((node, nodeIndex) => nodeIndex >= index && nodeIndex < subtreeEnd
    ? { ...node, level: node.level + 1 }
    : node)
  syncOutlineLegacyText()
}

function moveOutlineNodeUp(index: number) {
  const nodes = [...outlineNodes.value]
  const previousStart = previousOutlineSiblingStart(nodes, index)
  if (previousStart < 0) return
  const currentEnd = outlineSubtreeEnd(nodes, index)
  const previousBlock = nodes.slice(previousStart, index)
  const currentBlock = nodes.slice(index, currentEnd)
  nodes.splice(previousStart, currentEnd - previousStart, ...currentBlock, ...previousBlock)
  outlineNodes.value = nodes
  syncOutlineLegacyText()
}

function moveOutlineNodeDown(index: number) {
  const nodes = [...outlineNodes.value]
  const currentEnd = outlineSubtreeEnd(nodes, index)
  if (currentEnd >= nodes.length || nodes[currentEnd].level !== nodes[index]?.level) return
  const nextEnd = outlineSubtreeEnd(nodes, currentEnd)
  const currentBlock = nodes.slice(index, currentEnd)
  const nextBlock = nodes.slice(currentEnd, nextEnd)
  nodes.splice(index, nextEnd - index, ...nextBlock, ...currentBlock)
  outlineNodes.value = nodes
  syncOutlineLegacyText()
}

function removeOutlineNode(index: number) {
  const nodes = [...outlineNodes.value]
  const subtreeEnd = outlineSubtreeEnd(nodes, index)
  if (subtreeEnd - index > 1 && !window.confirm('删除该标题会同时删除其下级标题，确认继续吗？')) return
  nodes.splice(index, subtreeEnd - index)
  outlineNodes.value = nodes
  syncOutlineLegacyText()
}

function handleInputFilesSelected(field: InputFieldItem, event: Event) {
  const target = event.target as HTMLInputElement
  const selected = Array.from(target.files || [])
  if (selected.length === 0) {
    target.value = ''
    return
  }
  const oversized = selected.find(file => file.size > maxInputFileBytes.value)
  if (oversized) {
    appStore.showError(`${oversized.name} 超过单文件 ${formatBytes(maxInputFileBytes.value)} 限制`)
    target.value = ''
    return
  }
  if (field.kind === 'image' && selected.some(file => file.type && !file.type.startsWith('image/'))) {
    appStore.showError('参考图片只允许选择图片文件')
    target.value = ''
    return
  }
  if (field.kind === 'audio' && selected.some(file => file.type && !file.type.startsWith('audio/'))) {
    appStore.showError('该输入项只允许选择音频文件')
    target.value = ''
    return
  }
  if (field.kind === 'video' && selected.some(file => file.type && !file.type.startsWith('video/'))) {
    appStore.showError('该输入项只允许选择视频文件')
    target.value = ''
    return
  }
  const existing = inputFiles.value[field.name] || []
  const mergedFiles = [...existing, ...selected]
  const dedupedFiles = Array.from(
    new Map(mergedFiles.map(file => [uploadedAssetCacheKey(field.name, file), file])).values()
  )
  if (dedupedFiles.length > 10) {
    appStore.showError('单个输入项最多选择 10 个文件')
    target.value = ''
    return
  }
  inputFiles.value = {
    ...inputFiles.value,
    [field.name]: dedupedFiles
  }
  target.value = ''
}

function inputFilePreviewURL(fieldName: string, index: number, file: File): string {
  const key = inputFilePreviewKey(fieldName, index, file)
  const existing = inputFilePreviewURLs.value[key]
  if (existing) return existing
  const url = URL.createObjectURL(file)
  inputFilePreviewURLs.value = {
    ...inputFilePreviewURLs.value,
    [key]: url
  }
  return url
}

function removeInputFile(fieldName: string, index: number) {
  const files = inputFiles.value[fieldName] || []
  const file = files[index]
  if (file) {
    const cacheKey = uploadedAssetCacheKey(fieldName, file)
    const nextAssets = { ...uploadedInputAssetIDs.value }
    delete nextAssets[cacheKey]
    uploadedInputAssetIDs.value = nextAssets
  }
  revokeInputFilePreviewURLs(fieldName)
  inputFiles.value = {
    ...inputFiles.value,
    [fieldName]: files.filter((_, currentIndex) => currentIndex !== index)
  }
}

function uploadedAssetCacheKey(fieldName: string, file: File): string {
  return `${selectedApp.value?.id || 0}:${fieldName}:${file.name}:${file.size}:${file.lastModified}`
}

function inputFilePreviewKey(fieldName: string, index: number, file: File): string {
  return `${fieldName}:${index}:${file.name}:${file.size}:${file.lastModified}`
}

function revokeInputFilePreviewURLs(fieldName?: string) {
  const next: Record<string, string> = {}
  for (const [key, url] of Object.entries(inputFilePreviewURLs.value)) {
    if (!fieldName || key.startsWith(`${fieldName}:`)) {
      URL.revokeObjectURL(url)
    } else {
      next[key] = url
    }
  }
  inputFilePreviewURLs.value = next
}

function resetInputForm() {
  revokeInputFilePreviewURLs()
  const values: Record<string, string> = {}
  const files: Record<string, File[]> = {}
  for (const field of inputFields.value) {
    if (isAssetInputField(field)) {
      files[field.name] = []
    } else {
      values[field.name] = isAcademicPaperApp.value ? academicPaperInputDefault(field) : ''
    }
  }
  inputValues.value = values
  outlineNodes.value = isAcademicPaperApp.value ? createDefaultOutlineNodes() : []
  outlineImportText.value = isAcademicPaperApp.value ? outlineNodesToText(outlineNodes.value) : ''
  outlineImportError.value = ''
  inputFiles.value = files
  inputText.value = ''
  inputError.value = ''
  uploadedInputAssetIDs.value = {}
}

function academicPaperInputDefault(field: InputFieldItem): string {
  const defaults: Record<string, string> = {
    paper_type: 'course_paper',
    education_level: 'undergraduate',
    language: 'zh-CN',
    word_count: '5000',
    outline_requirements: defaultAcademicOutlineText(),
    writing_style: 'academic',
    abstract_enabled: 'true',
    keywords_enabled: 'true',
    keywords_count: '5',
    citation_style: 'gbt7714_numeric',
    page_format_preset: 'standard_cn_academic',
    page_size: 'A4',
    page_orientation: 'portrait',
    cover_enabled: 'true',
    toc_enabled: 'true',
    toc_levels: '3',
    heading_numbering_enabled: 'true',
    heading_numbering_style: 'decimal',
    references_enabled: 'true',
    acknowledgements_enabled: 'false',
    appendix_enabled: 'false',
    header_enabled: 'false',
    footer_enabled: 'false',
    page_number_enabled: 'true',
    page_number_position: 'footer',
    page_number_alignment: 'center',
    page_number_start: '1',
    page_number_format: 'decimal',
    pagination_title_page_break_after: 'true',
    pagination_toc_page_break_after: 'true',
    pagination_abstract_page_break_after: 'true',
    pagination_chapter_page_break_before: 'true',
    pagination_keep_paragraphs_together: 'true'
  }
  const value = defaults[field.name] || ''
  if (field.kind === 'select' && value && !field.options.some(option => option.value === value)) return ''
  return value
}

function handleAppPageChange(page: number) {
  appPagination.value.page = page
  loadApps()
}

function handleAppPageSizeChange(pageSize: number) {
  appPagination.value.page = 1
  appPagination.value.page_size = pageSize
  loadApps()
}

function handleRunPageChange(page: number) {
  runPagination.value.page = page
  loadRuns()
}

function handleRunPageSizeChange(pageSize: number) {
  runPagination.value.page = 1
  runPagination.value.page_size = pageSize
  loadRuns()
}

function normalizeInputFields(schema: Record<string, unknown> | undefined): InputFieldItem[] {
  if (!schema || typeof schema !== 'object') return []
  const properties = schema.properties && typeof schema.properties === 'object'
    ? schema.properties as Record<string, unknown>
    : {}
  const required = Array.isArray(schema.required)
    ? new Set(schema.required.filter((item): item is string => typeof item === 'string'))
    : new Set<string>()
  return Object.entries(properties)
    .map(([name, value]) => normalizeInputField(name, value, required.has(name)))
    .filter((item): item is InputFieldItem => item !== null)
}

function normalizeInputField(name: string, value: unknown, required: boolean): InputFieldItem | null {
  const record = value && typeof value === 'object' ? value as Record<string, unknown> : {}
  const items = record.items && typeof record.items === 'object' && !Array.isArray(record.items)
    ? record.items as Record<string, unknown>
    : {}
  const mediaType = normalizeMediaAccept(
    record.contentMediaType,
    record.content_media_type,
    record.accept,
    items.contentMediaType,
    items.content_media_type,
    items.accept
  )
  const kind = normalizeInputKind(
    stringValue(record['x-input-kind']) || stringValue(record['x-input-type']) || stringValue(record['x-asset-type']) ||
      stringValue(items['x-input-kind']) || stringValue(items['x-input-type']) || stringValue(items['x-asset-type']),
    stringValue(record.type),
    Array.isArray(record.enum),
    stringValue(record.format) || stringValue(items.format),
    mediaType
  )
  if (!kind) return null
  return {
    name,
    label: stringValue(record.title) || name,
    kind,
    required,
    assetRole: stringValue(record['x-asset-role']),
    accept: mediaType,
    options: normalizeInputOptions(record)
  }
}

function normalizeInputKind(inputKind: string, schemaType: string, hasEnum = false, format = '', mediaType = ''): InputFieldKind | null {
  const normalizedInputKind = inputKind.toLowerCase()
  const normalizedSchemaType = schemaType.toLowerCase()
  const normalizedFormat = format.toLowerCase()
  if (['textarea', 'text', 'number', 'image', 'file', 'audio', 'video', 'select', 'boolean', 'date'].includes(normalizedInputKind)) {
    return normalizedInputKind as InputFieldKind
  }
  const mediaKind = mediaKindFromAccept(mediaType)
  if (mediaKind) return mediaKind
  if (normalizedFormat === 'binary' || normalizedFormat === 'data-url') return 'file'
  if (normalizedSchemaType === 'boolean') return 'boolean'
  if (normalizedSchemaType === 'number' || normalizedSchemaType === 'integer') return 'number'
  if (normalizedSchemaType === 'array') return 'file'
  if (normalizedSchemaType === 'string' && hasEnum) return 'select'
  if (normalizedSchemaType === 'string') return 'text'
  return null
}

function normalizeMediaAccept(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (Array.isArray(value)) {
      const items = value.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(Boolean)
      if (items.length) return items.join(',')
    }
  }
  return ''
}

function mediaKindFromAccept(value: string): 'image' | 'audio' | 'video' | null {
  const extensionKinds: Record<string, 'image' | 'audio' | 'video'> = {
    apng: 'image', avif: 'image', gif: 'image', jpeg: 'image', jpg: 'image', png: 'image', webp: 'image',
    aac: 'audio', flac: 'audio', m4a: 'audio', mp3: 'audio', ogg: 'audio', opus: 'audio', wav: 'audio',
    avi: 'video', m4v: 'video', mkv: 'video', mov: 'video', mp4: 'video', webm: 'video'
  }
  const kinds = new Set<'image' | 'audio' | 'video'>()
  for (const token of value.toLowerCase().split(',').map(item => item.trim()).filter(Boolean)) {
    if (token.startsWith('image/')) kinds.add('image')
    else if (token.startsWith('audio/')) kinds.add('audio')
    else if (token.startsWith('video/')) kinds.add('video')
    else if (token.startsWith('.') && extensionKinds[token.slice(1)]) kinds.add(extensionKinds[token.slice(1)])
  }
  return kinds.size === 1 ? [...kinds][0] : null
}

function normalizeInputOptions(record: Record<string, unknown>): Array<{ label: string; value: string }> {
  const enumValues = Array.isArray(record.enum) ? record.enum : []
  const xOptions = Array.isArray(record['x-options']) ? record['x-options'] : []
  const raw = xOptions.length > 0 ? xOptions : enumValues
  return raw
    .map((item) => {
      if (typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean') {
        const value = String(item)
        return { label: value, value }
      }
      if (item && typeof item === 'object') {
        const option = item as Record<string, unknown>
        const value = stringValue(option.value)
        const label = stringValue(option.label) || value
        if (value) return { label, value }
      }
      return null
    })
    .filter((item): item is { label: string; value: string } => item !== null)
}

function inputSelectOptions(field: InputFieldItem) {
  return [
    { label: field.required ? `请选择${field.label}` : `不选择${field.label}`, value: '' },
    ...field.options
  ]
}

function inputBooleanOptions(field: InputFieldItem) {
  return [
    { label: field.required ? `请选择${field.label}` : `不设置${field.label}`, value: '' },
    { label: '是', value: 'true' },
    { label: '否', value: 'false' }
  ]
}

function isAssetInputField(field: InputFieldItem): boolean {
  return field.kind === 'image' || field.kind === 'file' || field.kind === 'audio' || field.kind === 'video'
}

function assetInputActionLabel(field: InputFieldItem): string {
  const labels: Partial<Record<InputFieldKind, string>> = {
    image: '上传图片',
    audio: '上传音频',
    video: '上传视频',
    file: '上传文件'
  }
  return labels[field.kind] || '上传文件'
}

function inputFileAccept(field: InputFieldItem): string | undefined {
  if (field.accept) return field.accept
  if (field.kind === 'image') return 'image/*'
  if (field.kind === 'audio') return 'audio/*'
  if (field.kind === 'video') return 'video/*'
  return undefined
}

function inputPlaceholder(field: InputFieldItem): string {
  if (field.kind === 'textarea') return `输入${field.label}`
  if (field.kind === 'number') return '输入数字'
  return field.required ? `填写${field.label}` : '可选'
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

function normalizeModelPolicyItem(policyKey: string, value: unknown): ModelPolicyItem | null {
  const record = value && typeof value === 'object' ? value as Record<string, unknown> : {}
  const [fallbackNodeId, fallbackRole] = splitPolicyKey(policyKey)
  const model = stringValue(record.model)
  return {
    policyKey,
    nodeId: stringValue(record.node_id) || fallbackNodeId,
    role: stringValue(record.role) || fallbackRole,
    capability: stringValue(record.capability) || 'model',
    provider: normalizeAgentAppProvider(stringValue(record.provider) || stringValue(record.platform)) || inferAgentAppProvider(model),
    model,
    modelGroupId: numberValue(record.model_group_id),
    optional: record.optional === true
  }
}

function splitPolicyKey(policyKey: string): [string, string] {
  const parts = policyKey.split('.')
  if (parts.length >= 2) return [parts[0] || '', parts.slice(1).join('.') || '']
  return [policyKey, '']
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function numberValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    if (Number.isFinite(parsed) && parsed > 0) return parsed
  }
  return null
}

function activeApiKeysForPolicy(item: ModelPolicyItem): ApiKey[] {
  return apiKeys.value.filter((key) => (
    key.status === 'active' &&
    (item.provider === '' || key.group?.platform === item.provider) &&
    (item.modelGroupId == null || key.group_id === item.modelGroupId)
  ))
}

function apiKeyOptionsForPolicy(item: ModelPolicyItem): ApiKeyOptionItem[] {
  const options = activeApiKeysForPolicy(item)
  return [
    { label: loadingKeys.value ? '加载中...' : '选择 API Key', value: '' },
    ...options.map(apiKeyToOption)
  ]
}

function apiKeyToOption(key: ApiKey): ApiKeyOptionItem {
  const groupLabel = apiKeyGroupLabel(key)
  const platformLabel = apiKeyPlatformLabel(key)
  const rateLabel = apiKeyRateLabel(key)
  const maskLabel = maskKey(key.key)
  return {
    label: `${key.name} · ${groupLabel} · ${platformLabel} · ${rateLabel} (${maskLabel})`,
    value: key.id,
    apiKey: key,
    groupLabel,
    platformLabel,
    rateLabel,
    maskLabel
  }
}

function apiKeyOptionName(option: unknown): string {
  const apiKey = (option as { apiKey?: ApiKey } | null)?.apiKey
  return apiKey?.name || ''
}

function apiKeyGroupLabel(key: ApiKey): string {
  if (key.group?.name) return key.group.name
  if (key.group_id) return `分组 #${key.group_id}`
  return '未绑定分组'
}

function apiKeyPlatformLabel(key: ApiKey): string {
  const platform = key.group?.platform
  if (!platform) return '平台未配置'
  return providerLabel(platform)
}

function providerLabel(platform: string): string {
  return agentAppProviderLabel(platform)
}

function apiKeyRateLabel(key: ApiKey): string {
  const rate = apiKeyEffectiveRate(key)
  if (rate == null) return '倍率未配置'
  return `倍率 ${formatRateMultiplier(rate)}`
}

function apiKeyEffectiveRate(key: ApiKey): number | null {
  if (key.group_id != null && Object.prototype.hasOwnProperty.call(userGroupRates.value, key.group_id)) {
    const custom = Number(userGroupRates.value[key.group_id])
    if (Number.isFinite(custom)) return custom
  }
  const groupRate = Number(key.group?.rate_multiplier)
  if (Number.isFinite(groupRate)) return groupRate
  return null
}

function formatRateMultiplier(value: number): string {
  if (value === 0) return '0x'
  if (Number.isInteger(value)) return `${value}x`
  return `${Number(value.toFixed(4)).toString()}x`
}

function hasPolicyKeyOption(item: ModelPolicyItem): boolean {
  return activeApiKeysForPolicy(item).length > 0
}

function selectedPolicyApiKey(item: ModelPolicyItem): ApiKey | null {
  const selected = policyApiKeySelections.value[item.policyKey]
  if (selected === '' || selected == null) return null
  return apiKeys.value.find((key) => key.id === Number(selected)) || null
}

function selectedPolicyApiKeyGroupLabel(item: ModelPolicyItem): string {
  const key = selectedPolicyApiKey(item)
  return key ? apiKeyGroupLabel(key) : '-'
}

function selectedPolicyApiKeyPlatformLabel(item: ModelPolicyItem): string {
  const key = selectedPolicyApiKey(item)
  return key ? apiKeyPlatformLabel(key) : '-'
}

function selectedPolicyApiKeyRateLabel(item: ModelPolicyItem): string {
  const key = selectedPolicyApiKey(item)
  return key ? apiKeyRateLabel(key) : '-'
}

function syncPolicyApiKeySelections() {
  if (!hasModelPolicies.value) {
    policyApiKeySelections.value = {}
    return
  }
  const next: Record<string, number | ''> = {}
  for (const item of modelPolicyItems.value) {
    const available = activeApiKeysForPolicy(item)
    const current = policyApiKeySelections.value[item.policyKey]
    if (current !== '' && current != null && available.some((key) => key.id === Number(current))) {
      next[item.policyKey] = Number(current)
      continue
    }
    if (!item.optional && available.length > 0) {
      next[item.policyKey] = recommendedApiKey(available)?.id || ''
      continue
    }
    next[item.policyKey] = ''
  }
  policyApiKeySelections.value = next
}

function autoFillPolicyKeys() {
  const next: Record<string, number | ''> = {}
  for (const item of modelPolicyItems.value) {
    const available = activeApiKeysForPolicy(item)
    const current = policyApiKeySelections.value[item.policyKey]
    if (current !== '' && current != null && available.some((key) => key.id === Number(current))) {
      next[item.policyKey] = Number(current)
      continue
    }
    next[item.policyKey] = item.optional ? '' : recommendedApiKey(available)?.id || ''
  }
  policyApiKeySelections.value = next
}

function recommendedApiKey(keys: ApiKey[]): ApiKey | null {
  if (!keys.length) return null
  return [...keys].sort((left, right) => {
    const leftRate = apiKeyEffectiveRate(left)
    const rightRate = apiKeyEffectiveRate(right)
    if (leftRate == null && rightRate == null) return right.id - left.id
    if (leftRate == null) return 1
    if (rightRate == null) return -1
    if (leftRate !== rightRate) return leftRate - rightRate
    return right.id - left.id
  })[0]
}

function buildRunAPIKeyBindings() {
  return modelPolicyItems.value
    .map((item) => ({
      policy_key: item.policyKey,
      node_id: item.nodeId || undefined,
      role: item.role || undefined,
      api_key_id: Number(policyApiKeySelections.value[item.policyKey])
    }))
    .filter((item) => Number.isFinite(item.api_key_id) && item.api_key_id > 0)
}

function policyLabel(item: ModelPolicyItem): string {
  if (item.nodeId && item.role) return `${item.nodeId} / ${item.role}`
  return item.nodeId || item.role || item.policyKey
}

function policyUserLabel(item: ModelPolicyItem): string {
  const capability = capabilityLabel(item.capability)
  const role = item.role ? modelRoleLabel(item.role) : ''
  if (role && role !== item.role) return `${capability}：${role}`
  if (item.model) return `${capability}处理`
  return policyLabel(item)
}

function modelRoleLabel(role: string): string {
  const labels: Record<string, string> = {
    generate: '生成结果',
    rewrite: '改写内容',
    summarize: '总结内容',
    caption: '生成说明',
    vision: '理解图片',
    extract: '提取信息',
    classify: '分类判断'
  }
  return labels[role] || role
}

function capabilityLabel(capability: string): string {
  const labels: Record<string, string> = {
    text: '文本',
    image: '图像',
    vision: '视觉',
    file: '文件',
    video: '视频',
    model: '模型'
  }
  return labels[capability] || capability
}

function policyGroupRequirementLabel(item: ModelPolicyItem): string {
  const provider = item.provider ? providerLabel(item.provider) : '任意厂商'
  if (item.modelGroupId == null) {
    return item.provider ? `${provider} 厂商下任意可用 Key` : '任意厂商任意分组的可用 Key'
  }
  const key = apiKeys.value.find((apiKey) => apiKey.group_id === item.modelGroupId && apiKey.group?.name)
  const group = key?.group?.name ? `${key.group.name} 分组` : `分组 #${item.modelGroupId}`
  return `${provider} / ${group} 的 Key`
}

function appTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    agent: '智能体',
    workflow: '工作流',
    prompt: '提示词',
    external: '外部'
  }
  return labels[type] || type
}

function appTypeBadgeClass(type: string): string {
  switch (type) {
    case 'workflow':
      return 'badge-purple'
    case 'prompt':
      return 'badge-warning'
    case 'external':
      return 'badge-gray'
    case 'agent':
    default:
      return 'badge-primary'
  }
}

function appTypeToneClass(type: string): string {
  switch (type) {
    case 'workflow':
      return 'bg-sky-50 text-sky-700 ring-1 ring-sky-100 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-800'
    case 'prompt':
      return 'bg-amber-50 text-amber-700 ring-1 ring-amber-100 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-800'
    case 'external':
      return 'bg-gray-100 text-gray-700 ring-1 ring-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:ring-dark-700'
    case 'agent':
    default:
      return 'bg-primary-50 text-primary-700 ring-1 ring-primary-100 dark:bg-primary-900/20 dark:text-primary-300 dark:ring-primary-800'
  }
}

function appInitials(app: AgentAppCatalog): string {
  const name = (app.name || app.slug || '').trim()
  if (!name) return '应用'
  return Array.from(name.replace(/\s+/g, '')).slice(0, 2).join('')
}

function appVersionLabel(app: AgentAppCatalog): string {
  if (app.published_version?.version) return `版本 ${app.published_version.version}`
  if (app.published_version_id) return `版本 ${app.published_version_id}`
  return '已发布'
}

function runStatusLabel(status: string): string {
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

function runStatusBadgeClass(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'badge-success'
    case 'failed':
    case 'timeout':
      return 'badge-danger'
    case 'running':
      return 'badge-primary'
    case 'queued':
      return 'badge-warning'
    default:
      return 'badge-gray'
  }
}

function runStatusDotClass(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'bg-green-500'
    case 'failed':
    case 'timeout':
      return 'bg-red-500'
    case 'running':
      return 'bg-primary-500'
    case 'queued':
      return 'bg-yellow-500'
    case 'canceled':
      return 'bg-gray-400'
    default:
      return 'bg-gray-300'
  }
}

function runEventLabel(event: AgentRunEvent): string {
  const labels: Record<string, string> = {
    queued: '排队中',
    dispatching: '准备处理',
    running: '正在处理',
    worker_accepted: '开始处理',
    progress: '处理中',
    log: '记录进度',
    model_proxy: '正在调用模型',
    artifact: '正在保存结果',
    succeeded: '已完成',
    failed: '处理失败',
    canceled: '已停止',
    timeout: '处理超时'
  }
  return labels[event.event_type] || event.event_type
}

function runEventMessage(event: AgentRunEvent): string {
  const message = String(event.message || '').trim()
  if (!message || message === 'Worker completed') return ''
  const replacements: Record<string, string> = {
    'Worker accepted': '任务已开始处理',
    'Worker completed': '',
    'Model proxy request started': '模型请求已发起',
    'Model proxy request completed': '模型请求已完成',
    'Artifact uploaded': '结果已保存',
    'Preparing image prompt': '正在优化图片提示词',
    'Generating image': '正在生成图片',
    'Image generated': '图片已生成',
    'Run completed': '任务处理完成',
    'Calling model': '正在调用模型',
    'run queued': '任务已进入队列',
    'dispatching run to worker': '正在分配执行资源',
    'worker dispatch started': 'Worker 已开始执行'
  }
  return replacements[message] || message
}

function runErrorMessage(run: AgentRun): string {
  const byCode: Record<string, string> = {
    WORKER_HOST_MISSING: '应用尚未绑定可用的 Worker 服务，请联系管理员。',
    WORKER_HOST_UNAVAILABLE: 'Worker 服务暂时不可用，请稍后重试。',
    WORKER_HOST_INACTIVE: 'Worker 服务当前已停用，请联系管理员。',
    WORKER_URL_INVALID: '应用运行地址配置错误，请联系管理员。',
    WORKER_REQUEST_FAILED: '无法连接 Worker 服务，请稍后重试。',
    MODEL_PROXY_FAILED: '模型调用失败，请检查所选 Key 或稍后重试。',
    ARTIFACT_UPLOAD_FAILED: '结果保存失败，请稍后重试。',
    USER_CANCELED: '你已停止本次运行。'
  }
  return byCode[run.error_code || ''] || run.error_message || '运行失败，请稍后重试。'
}

function runEventDotClass(event: AgentRunEvent): string {
  if (event.event_type === 'succeeded' || event.status === 'succeeded') return 'bg-green-500'
  if (event.event_type === 'failed' || event.event_type === 'timeout' || event.status === 'failed' || event.status === 'timeout') return 'bg-red-500'
  if (event.event_type === 'canceled' || event.status === 'canceled') return 'bg-gray-400'
  if (event.event_type === 'model_proxy') return 'bg-blue-500'
  if (event.event_type === 'artifact') return 'bg-violet-500'
  return 'bg-primary-500'
}

function formatPercent(value: number): string {
  const normalized = value > 1 ? value / 100 : value
  return `${Math.round(Math.min(Math.max(normalized, 0), 1) * 100)}%`
}

function runPrimaryText(run: AgentRun): string {
  const output = readableOutputRecord(run)
  const schema = selectedApp.value?.published_version?.output_schema_json || {}
  const configuredPrimary = typeof schema['x-primary-field'] === 'string' ? String(schema['x-primary-field']) : ''
  const schemaProperties = schema.properties && typeof schema.properties === 'object'
    ? Object.keys(schema.properties as Record<string, unknown>)
    : []
  const terminalOnlyKeys = new Set(['message', 'description'])
  const keys = Array.from(new Set([configuredPrimary, 'result', 'answer', 'text', 'content', 'summary', 'message', 'description', ...schemaProperties].filter(Boolean)))
    .filter(key => isTerminalRunStatus(run.status) || !terminalOnlyKeys.has(key.toLowerCase()))
  for (const key of keys) {
    const value = output[key]
    if (typeof value === 'string' && value.trim() !== '' && value !== 'Worker completed') {
      return value.trim()
    }
  }
  const nested = run.output_summary_json?.output
  if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
    for (const key of keys) {
      const value = (nested as Record<string, unknown>)[key]
      if (typeof value === 'string' && value.trim() !== '' && value !== 'Worker completed') {
        return value.trim()
      }
    }
  }
  return ''
}

function runInputItems(run: AgentRun | null): Array<{ key: string; label: string; value: unknown }> {
  const input = run?.input_summary_json
  if (!input || typeof input !== 'object' || Array.isArray(input)) return []
  const assetFieldNames = new Set(runInputAssets(run).map(asset => String(asset.field_name || '').trim()).filter(Boolean))
  return Object.entries(input)
    .filter(([key, value]) => !isTechnicalInputKey(key) && !assetFieldNames.has(key) && value != null && value !== '')
    .slice(0, 20)
    .map(([key, value]) => ({
      key,
      label: inputFieldDisplayLabel(key),
      value
    }))
}

function runInputAssets(run: AgentRun | null): AgentInputAsset[] {
  const raw = run?.input_summary_json?.input_assets
  if (!Array.isArray(raw)) return []
  return raw.flatMap((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return []
    const value = item as Record<string, unknown>
    const id = Number(value.id || value.file_id || 0)
    if (!Number.isFinite(id) || id <= 0) return []
    const metadata = value.metadata_json && typeof value.metadata_json === 'object' && !Array.isArray(value.metadata_json)
      ? value.metadata_json as Record<string, unknown>
      : value.metadata && typeof value.metadata === 'object' && !Array.isArray(value.metadata)
        ? value.metadata as Record<string, unknown>
        : {}
    return [{
      id,
      run_id: Number(value.run_id || run?.id || 0) || undefined,
      user_id: Number(value.user_id || run?.user_id || 0),
      app_id: Number(value.app_id || run?.app_id || 0) || undefined,
      field_name: String(value.field_name || ''),
      asset_type: String(value.asset_type || 'file'),
      asset_role: String(value.asset_role || ''),
      name: String(value.name || `input-${id}`),
      mime_type: String(value.mime_type || ''),
      storage_provider: String(value.storage_provider || ''),
      bucket: String(value.bucket || '') || undefined,
      object_key: String(value.object_key || ''),
      object_url: String(value.object_url || ''),
      size_bytes: Number(value.size_bytes || 0),
      sha256: String(value.sha256 || '') || undefined,
      metadata_json: metadata,
      expires_at: String(value.expires_at || '') || undefined,
      created_at: String(value.created_at || run?.created_at || '')
    }]
  })
}

function isTechnicalInputKey(key: string): boolean {
  const normalized = key.toLowerCase()
  return normalized === 'input_assets' || normalized === 'input_asset_ids' || normalized === 'input_files' || normalized === 'outline_spec'
}

function inputFieldDisplayLabel(key: string): string {
  const properties = selectedApp.value?.published_version?.input_schema_json?.properties
  if (properties && typeof properties === 'object') {
    const field = (properties as Record<string, unknown>)[key]
    if (field && typeof field === 'object' && !Array.isArray(field)) {
      const title = String((field as Record<string, unknown>).title || '').trim()
      if (title) return title
    }
  }
  const labels: Record<string, string> = {
    prompt: '提示词',
    text: '输入内容',
    content: '输入内容',
    question: '问题',
    query: '问题',
    instruction: '指令',
    description: '描述',
    style: '风格',
    size: '尺寸',
    quality: '质量',
    background: '背景',
    output_format: '输出格式',
    input_fidelity: '参考图保真度'
  }
  return labels[key] || key.replace(/[_-]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase())
}

function runReadableItems(run: AgentRun): Array<{ label: string; value: unknown }> {
  const output = readableOutputRecord(run)
  return Object.entries(output)
    .filter(([key, value]) => !isTechnicalOutputKey(key) && value != null && !isPrimaryOutputKey(key))
    .slice(0, 12)
    .map(([key, value]) => ({
      label: outputFieldLabel(key),
      value
    }))
}

function readableOutputRecord(run: AgentRun): Record<string, unknown> {
  const raw = run.output_summary_json || {}
  const nested = raw.output
  if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
    return nested as Record<string, unknown>
  }
  return raw
}

function isTechnicalOutputKey(key: string): boolean {
  const normalized = key.toLowerCase()
  if (['result', 'answer', 'text', 'content', 'summary', 'message', 'description', 'prompt', 'kind', 'artifact', 'artifacts'].includes(normalized)) return true
  return (
    normalized.includes('api_key') ||
    normalized.includes('worker_run') ||
    normalized.includes('model_proxy') ||
    normalized.includes('metadata') ||
    normalized.includes('progress') ||
    normalized.endsWith('_id') ||
    normalized === 'id'
  )
}

function isPrimaryOutputKey(key: string): boolean {
  return ['result', 'answer', 'text', 'content', 'summary', 'message', 'description'].includes(key.toLowerCase())
}

function outputFieldLabel(key: string): string {
  const properties = selectedApp.value?.published_version?.output_schema_json?.properties
  if (properties && typeof properties === 'object') {
    const field = (properties as Record<string, unknown>)[key]
    if (field && typeof field === 'object') {
      const title = String((field as Record<string, unknown>).title || '').trim()
      if (title) return title
    }
  }
  const labels: Record<string, string> = {
    prompt: '输入内容',
    title: '标题',
    name: '名称',
    kind: '类型',
    status: '状态'
  }
  return labels[key] || key.replace(/[_-]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase())
}

function artifactDescription(artifact: AgentArtifact): string {
  const parts = [artifactTypeLabel(artifact), formatBytes(artifact.size_bytes)]
  if (artifact.expires_at) {
    parts.push(`保留至 ${formatDateTime(artifact.expires_at)}`)
  }
  return parts.filter(Boolean).join(' · ')
}

function inputAssetFieldLabel(asset: AgentInputAsset): string {
  const metadataLabel = String(asset.metadata_json?.field_label || '').trim()
  if (metadataLabel) return metadataLabel
  const key = String(asset.field_name || '').trim()
  if (key) return inputFieldDisplayLabel(key)
  if (asset.asset_role) return `输入文件 · ${asset.asset_role}`
  return '输入文件'
}

function inputAssetDescription(asset: AgentInputAsset): string {
  const parts = [inputAssetTypeLabel(asset), formatBytes(asset.size_bytes)]
  if (asset.expires_at) parts.push(`保留至 ${formatDateTime(asset.expires_at)}`)
  return parts.filter(Boolean).join(' · ')
}

function inputAssetTypeLabel(asset: AgentInputAsset): string {
  const mime = inputAssetMime(asset)
  if (mime.startsWith('image/')) return '参考图片'
  if (mime.startsWith('video/')) return '输入视频'
  if (mime.startsWith('audio/')) return '输入音频'
  return '输入文件'
}

function artifactTypeLabel(artifact: AgentArtifact): string {
  const mime = artifactMime(artifact)
  if (mime.startsWith('image/')) return '图片'
  if (mime.startsWith('video/')) return '视频'
  if (mime.startsWith('audio/')) return '音频'
  if (isWordArtifact(artifact)) return 'Word 论文文档'
  if (artifact.artifact_type === 'log') return '日志'
  if (artifact.artifact_type === 'preview') return '预览'
  return '文件'
}

function formatDateTime(value?: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function maskKey(key: string): string {
  if (!key) return '-'
  if (key.length <= 12) return key
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

onMounted(async () => {
  await Promise.all([loadKeys(), loadApps()])
})

onBeforeUnmount(() => {
  stopRunPolling()
  stopRunUsageRetry()
  revokeInputFilePreviewURLs()
})
</script>
