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
                placeholder="搜索应用"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
            <Select v-model="filters.status" :options="statusFilterOptions" class="w-full sm:w-40" @change="loadApps" />
            <Select v-model="filters.app_type" :options="typeFilterOptions" class="w-full sm:w-40" @change="loadApps" />
          </div>

          <div class="flex flex-wrap items-center justify-end gap-2">
            <button class="btn btn-secondary" :disabled="loading" title="刷新" @click="loadApps">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openAppDialog">
              <Icon name="plus" size="md" class="mr-2" />
              发布应用
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="apps"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="id"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <code class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ row.slug }}</code>
            </div>
          </template>

          <template #cell-app_type="{ row }">
            <span class="badge badge-primary">{{ appTypeLabel(row.app_type) }}</span>
          </template>

          <template #cell-status="{ row }">
            <span :class="['badge', appStatusBadgeClass(row.status)]">{{ appStatusLabel(row.status) }}</span>
          </template>

          <template #cell-visibility="{ row }">
            <span class="badge badge-gray">{{ row.visibility === 'public' ? '所有用户可用' : '不对用户开放' }}</span>
          </template>

          <template #cell-created_at="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                @click="openVersionDialog(row)"
              >
                <Icon name="plus" size="sm" />
                <span class="text-xs">版本</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                @click="openVersions(row)"
              >
                <Icon name="document" size="sm" />
                <span class="text-xs">查看</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                @click="openEditAppDialog(row)"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">编辑</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                @click="deleteApp(row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">删除</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              title="暂无应用"
              description="先创建应用版本配置，再绑定已部署的 Worker 服务和具体运行路径"
              action-text="新建应用"
              @action="openAppDialog"
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

    <BaseDialog :show="showAppDialog" title="发布应用" width="full" @close="showAppDialog = false">
      <form id="agent-app-form" class="space-y-4" @submit.prevent="handleCreateApp">
        <div class="rounded-lg border border-primary-100 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-900/50 dark:bg-primary-900/20 dark:text-primary-200">
          <div class="font-medium">发布的是应用入口和运行策略，不是在这里上传或编写 Worker 代码</div>
          <p class="mt-1 text-xs leading-5">
            Worker 代码需要先独立部署；这里负责绑定 Worker Host、运行路径、用户输入表单、模型能力和结果保存策略。用户运行时只能选择平台内已有 API Key。
          </p>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-gray-800 dark:text-gray-100">应用定义</span>
            <span class="badge badge-gray">展示与检索</span>
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label class="input-label">应用名称 <span class="text-red-500">*</span></label>
              <input v-model="appForm.name" required class="input" placeholder="商品图工作流" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">展示给用户看的应用名称。</p>
            </div>
            <div>
              <label class="input-label">应用标识 <span class="text-red-500">*</span></label>
              <input v-model="appForm.slug" required class="input" placeholder="product-image-workflow" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">用于 URL 和接口识别，只使用小写字母、数字、中划线或下划线。</p>
            </div>
          </div>

          <div class="mt-4">
            <label class="input-label">描述</label>
            <textarea v-model="appForm.description" rows="2" class="input" placeholder="面向用户展示的应用能力概述"></textarea>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">简要说明这个应用能帮用户完成什么。</p>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
            <div>
              <label class="input-label">类型</label>
              <Select v-model="appForm.app_type" :options="typeEditOptions" />
            </div>
            <div>
              <label class="input-label">可见性</label>
              <Select v-model="appForm.visibility" :options="visibilityOptions" />
            </div>
            <div>
              <label class="input-label">状态</label>
              <Select v-model="appForm.status" :options="statusEditOptions" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">发布状态会影响用户是否能看到和运行。</p>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label class="input-label">分类</label>
              <input v-model="appForm.category" class="input" placeholder="图像 / 文档 / 视频 / 自动化" />
            </div>
            <div>
              <label class="input-label">应用图标</label>
              <div class="flex items-start gap-3">
                <div class="flex h-14 w-14 flex-shrink-0 items-center justify-center overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                  <img v-if="appForm.icon_url" :src="appIconPreviewURL || appForm.icon_url" alt="" class="h-full w-full object-cover" />
                  <Icon v-else name="document" size="lg" class="text-gray-400 dark:text-gray-500" />
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <button type="button" class="btn btn-secondary btn-sm" :disabled="iconUploading" @click="triggerIconUpload">
                      <Icon name="upload" size="sm" class="mr-1.5" />
                      {{ iconUploading ? '上传中...' : '上传图片' }}
                    </button>
                    <button v-if="appForm.icon_url" type="button" class="btn btn-secondary btn-sm" :disabled="iconUploading" @click="clearAppIcon">
                      <Icon name="trash" size="sm" class="mr-1.5" />
                      移除
                    </button>
                  </div>
                  <input ref="iconInputRef" type="file" accept="image/png,image/jpeg,image/webp" class="hidden" @change="handleIconSelected" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">上传 PNG、JPG 或 WebP，最大 2MB。图片会进入对象存储，DB 只保存 URL。</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">运行版本</span>
              <span class="badge badge-primary">Worker</span>
              <span class="badge badge-gray">可热插拔</span>
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('grokVideo')">Grok 生视频模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('productMarketing')">商品营销包模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('academicPaper')">Word 论文模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('text')">文本模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('image')">文生图 / 图生图模板</button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
            <div>
              <label class="input-label">版本号 <span class="text-red-500">*</span></label>
              <input v-model="publishVersionForm.version" required class="input" placeholder="v1.0.0" />
            </div>
            <div>
              <label class="input-label">运行类型</label>
              <Select v-model="publishVersionForm.runtime_type" :options="runtimeTypeOptions" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前阶段主要使用 Worker。</p>
            </div>
            <div>
              <label class="input-label">版本状态</label>
              <Select v-model="publishVersionForm.status" :options="statusEditOptions" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">选择已发布后，新用户运行会使用该版本。</p>
            </div>
            <div>
              <label class="input-label">Worker Host <span class="text-red-500">*</span></label>
              <Select v-model="publishVersionForm.worker_host_id" :options="workerHostOptions" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">选择已登记的 Worker 服务地址。</p>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
            <div>
              <label class="input-label">应用运行路径 <span class="text-red-500">*</span></label>
              <input v-model="publishVersionForm.worker_route" required class="input" placeholder="/runs 或 /workflow/runs" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">这是 Worker 内某个应用的入口，实际请求地址 = Worker Host 服务地址 + 该路径。</p>
            </div>
            <div>
              <label class="input-label">应用健康路径（可选）</label>
              <input v-model="publishVersionForm.worker_health_route" class="input" placeholder="/health" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不填时使用 Worker Host 的健康检查路径。</p>
            </div>
            <div>
              <label class="input-label">变更说明</label>
              <input v-model="publishVersionForm.changelog" class="input" placeholder="首次发布 / 新增生图能力 / 调整模型策略" />
            </div>
          </div>

          <details class="mt-4 rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/60">
            <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">部署备注（可选）</summary>
            <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">代码版本引用</label>
                <input v-model="publishVersionForm.source_ref" class="input" placeholder="git tag / commit / runner 名称" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只做审计记录，不会从这里拉代码。</p>
              </div>
              <div>
                <label class="input-label">镜像版本引用</label>
                <input v-model="publishVersionForm.image_ref" class="input" placeholder="registry.example.com/worker:v1" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只做部署版本记录，不会在 Sub2API 主进程里运行镜像。</p>
              </div>
            </div>
          </details>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">输入项</span>
              <span class="badge badge-gray">表单</span>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="addInputField(publishVersionForm)">添加输入项</button>
          </div>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">定义用户运行应用时看到的表单。图片/文件类型代表输入资产，运行时应传资产引用，不传二进制到数据库。</p>
          <div class="space-y-3">
            <div
              v-for="(field, index) in publishVersionForm.input_fields"
              :key="`publish-input-${index}`"
              class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-[1fr_1fr_150px_90px_110px_44px]"
            >
              <input v-model.trim="field.name" class="input" placeholder="字段名，如 product_name" />
              <input v-model.trim="field.label" class="input" placeholder="用户看到的名称，如 商品名称" />
              <Select v-model="field.type" :options="inputTypeOptions" />
              <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input v-model="field.required" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                必填
              </label>
              <input v-model.trim="field.asset_role" class="input" placeholder="资产角色，如 reference" />
              <button type="button" class="btn btn-secondary btn-icon" title="删除" @click="removeInputField(publishVersionForm, index)">
                <Icon name="trash" size="sm" />
              </button>
              <div v-if="field.type === 'select'" class="md:col-span-6">
                <div class="mb-2 flex items-center justify-between gap-3">
                  <label class="input-label mb-0">下拉选项</label>
                  <button type="button" class="btn btn-secondary btn-sm" @click="addSelectOption(field)">添加选项</button>
                </div>
                <div class="space-y-2">
                  <div
                    v-for="(option, optionIndex) in selectOptionsForField(field)"
                    :key="`publish-input-${index}-option-${optionIndex}`"
                    class="grid grid-cols-1 gap-2 md:grid-cols-[1fr_1fr_44px]"
                  >
                    <input v-model.trim="option.label" class="input" placeholder="用户看到的中文，如 文生视频" @input="syncSelectOptionString(field)" />
                    <input v-model.trim="option.value" class="input" placeholder="提交给 Worker 的值，如 text_to_video" @input="syncSelectOptionString(field)" />
                    <button type="button" class="btn btn-secondary btn-icon" title="删除选项" @click="removeSelectOption(field, optionIndex)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">左边是用户看到的文字，右边是提交给 Worker 的真实值；例如“文生视频 / text_to_video”。</p>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">用户可见结果</span>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">定义 Worker 输出字段如何展示；列表、表格和结构化信息无需用户阅读 JSON。</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="addOutputField(publishVersionForm)">添加结果字段</button>
          </div>
          <div class="space-y-3">
            <div v-for="(field, index) in publishVersionForm.output_fields" :key="`publish-output-${index}`" class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-[1fr_1fr_160px_100px_44px]">
              <input v-model.trim="field.name" class="input" placeholder="输出字段，如 result" />
              <input v-model.trim="field.label" class="input" placeholder="用户看到的名称" />
              <Select v-model="field.type" :options="outputTypeOptions" />
              <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input type="radio" name="publish-primary-output" :checked="field.primary" @change="setPrimaryOutputField(publishVersionForm, index)" />
                主要结果
              </label>
              <button type="button" class="btn btn-secondary btn-icon" title="删除" @click="removeOutputField(publishVersionForm, index)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">模型能力绑定</span>
              <span class="badge badge-gray">模型绑定</span>
            </div>
              <button type="button" class="btn btn-secondary btn-sm" @click="addModelRole(publishVersionForm)">添加能力</button>
            </div>
            <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">这里配置 Worker 需要调用哪些模型能力，以及每个能力允许用户从哪个 Key 分组里选择。管理员不填写用户 Key。</p>
            <div class="space-y-3">
              <div
                v-for="(role, index) in publishVersionForm.model_roles"
                :key="`publish-role-${index}`"
                class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-2"
              >
                <div>
                  <label class="input-label">能力编号（Worker 用）</label>
                  <input v-model.trim="role.node_id" class="input" placeholder="prompt_rewrite" />
                </div>
                <div>
                  <label class="input-label">调用用途</label>
                  <input v-model.trim="role.role" class="input" placeholder="generate / rewrite / vision" />
                </div>
                <div>
                  <label class="input-label">能力</label>
                  <Select v-model="role.capability" :options="capabilityOptions" />
                </div>
                <div>
                  <label class="input-label">模型厂商 <span class="text-red-500">*</span></label>
                  <Select v-model="role.provider" :options="providerOptions" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">用户运行时只能选择该厂商分组下的 Key。</p>
                </div>
                <div>
                  <label class="input-label">模型名</label>
                  <input v-model.trim="role.model" class="input" placeholder="填平台可调用模型" />
                </div>
                <div>
                  <label class="input-label">允许的 Key 分组</label>
                  <Select v-model="role.model_group_id" :options="modelGroupOptionsForRole(role)" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只显示所选厂商下的分组；不限分组表示该厂商下任意可用 Key。</p>
                </div>
                <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                  <input v-model="role.required" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                  运行前必须选择 Key
                </label>
                <div class="flex items-end md:justify-end">
                  <button type="button" class="btn btn-secondary btn-icon" title="删除该模型角色" @click="removeModelRole(publishVersionForm, index)">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">能力与产物</span>
              <span class="badge badge-gray">发布设置</span>
            </div>
            <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">能力用于判断应用需要 text/image/video 等模型能力；产物策略控制结果保存到对象存储的类型和保留时间。</p>
            <div v-if="publishVersionForm.capabilities" class="grid grid-cols-2 gap-3 md:grid-cols-3">
              <label v-for="item in capabilityOptions" :key="`publish-cap-${item.value}`" class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input
                  v-model="publishVersionForm.capabilities[item.value]"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                {{ item.label }}
              </label>
            </div>
            <div v-if="publishVersionForm.artifact_policy" class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">结果保留天数</label>
                <input v-model.number="publishVersionForm.artifact_policy.retention_days" type="number" min="0" class="input" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">0 表示长期保留；大于 0 才会在开启清理任务后到期清理。</p>
              </div>
              <div>
                <label class="input-label">单文件上限 MB</label>
                <input v-model.number="publishVersionForm.artifact_policy.max_file_mb" type="number" min="1" class="input" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">限制 Worker 上传到对象存储的单个产物大小。</p>
              </div>
            </div>
            <div v-if="publishVersionForm.artifact_policy?.allowed_types" class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-3">
              <label v-for="item in artifactTypeOptions" :key="`publish-artifact-${item.value}`" class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input
                  v-model="publishVersionForm.artifact_policy.allowed_types[item.value]"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                {{ item.label }}
              </label>
            </div>
          </div>
        </div>
        <details class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">预览用户看到的页面</summary>
          <div class="mt-3">
            <AgentAppVersionPreview
              :name="appForm.name"
              :description="appForm.description"
              :input-fields="publishVersionForm.input_fields"
              :model-roles="publishVersionForm.model_roles"
              :output-fields="publishVersionForm.output_fields"
            />
          </div>
        </details>
      </form>

      <template #footer>
        <button class="btn btn-secondary" type="button" @click="showAppDialog = false">取消</button>
        <button class="btn btn-primary" type="submit" form="agent-app-form" :disabled="submitting">
          {{ submitting ? '发布中...' : '发布应用' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog :show="showEditAppDialog" title="编辑应用" width="wide" @close="closeEditAppDialog">
      <form id="agent-app-edit-form" class="space-y-4" @submit.prevent="saveEditedApp">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">应用名称</label>
            <input v-model.trim="editAppForm.name" required class="input" />
          </div>
          <div>
            <label class="input-label">应用标识</label>
            <input v-model.trim="editAppForm.slug" required class="input" />
          </div>
        </div>
        <div>
          <label class="input-label">描述</label>
          <textarea v-model="editAppForm.description" rows="3" class="input" />
        </div>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label class="input-label">类型</label>
            <Select v-model="editAppForm.app_type" :options="typeEditOptions" />
          </div>
          <div>
            <label class="input-label">可见性</label>
            <Select v-model="editAppForm.visibility" :options="visibilityOptions" />
          </div>
          <div>
            <label class="input-label">状态</label>
            <Select v-model="editAppForm.status" :options="statusEditOptions" />
          </div>
        </div>
        <div>
          <label class="input-label">分类</label>
          <input v-model.trim="editAppForm.category" class="input" />
        </div>
        <div>
          <label class="input-label">应用图标</label>
          <div class="flex items-center gap-3">
            <div class="flex h-14 w-14 items-center justify-center overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
              <img v-if="editIconPreviewURL" :src="editIconPreviewURL" alt="" class="h-full w-full object-cover" />
              <Icon v-else name="document" size="lg" class="text-gray-400" />
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="editIconUploading" @click="triggerEditIconUpload">
              <Icon name="upload" size="sm" class="mr-1.5" />
              {{ editIconUploading ? '上传中...' : '上传新图标' }}
            </button>
            <input ref="editIconInputRef" type="file" accept="image/png,image/jpeg,image/webp" class="hidden" @change="handleEditIconSelected" />
          </div>
        </div>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeEditAppDialog">取消</button>
        <button type="submit" form="agent-app-edit-form" class="btn btn-primary" :disabled="submitting">保存修改</button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showVersionDialog"
      :title="selectedApp ? `新增版本：${selectedApp.name}` : '新增版本'"
      width="extra-wide"
      @close="closeVersionDialog"
    >
      <form id="agent-version-form" class="space-y-4" @submit.prevent="handleCreateVersion">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label class="input-label">版本号 <span class="text-red-500">*</span></label>
            <input v-model="versionForm.version" required class="input" placeholder="v1.0.0" />
          </div>
          <div>
            <label class="input-label">运行类型</label>
            <Select v-model="versionForm.runtime_type" :options="runtimeTypeOptions" />
          </div>
          <div>
            <label class="input-label">状态</label>
            <Select v-model="versionForm.status" :options="statusEditOptions" />
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-1 text-sm font-medium text-gray-800 dark:text-gray-100">Worker 绑定</div>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">选择已部署的 Worker Host，再填写该版本对应的应用入口路径。</p>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div>
              <label class="input-label">Worker Host <span class="text-red-500">*</span></label>
              <Select v-model="versionForm.worker_host_id" :options="workerHostOptions" />
            </div>
            <div>
              <label class="input-label">应用运行路径 <span class="text-red-500">*</span></label>
            <input v-model="versionForm.worker_route" required class="input" placeholder="/runs 或 /workflow/runs" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">实际请求地址 = Worker Host 服务地址 + 该路径。</p>
            </div>
            <div>
              <label class="input-label">应用健康路径（可选）</label>
              <input v-model="versionForm.worker_health_route" class="input" placeholder="/image/v1/health" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">不填时使用 Worker Host 的健康检查路径。</p>
            </div>
          </div>
        </div>

        <details class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/60">
          <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">部署备注（可选）</summary>
          <div class="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label class="input-label">镜像版本引用</label>
              <input v-model="versionForm.image_ref" class="input" placeholder="registry.example.com/image-worker:v1" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只做审计记录，不会在 Sub2API 主进程里启动镜像。</p>
            </div>
            <div>
              <label class="input-label">代码版本引用</label>
              <input v-model="versionForm.source_ref" class="input" placeholder="git tag / commit" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">记录这个版本对应哪份 Worker 代码。</p>
            </div>
          </div>
        </details>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">输入项</span>
              <span class="badge badge-gray">表单</span>
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('text', versionForm)">文本模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('image', versionForm)">文生图 / 图生图模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('grokVideo', versionForm)">Grok 生视频模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('productMarketing', versionForm)">商品营销包模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="applyVersionTemplate('academicPaper', versionForm)">Word 论文模板</button>
              <button type="button" class="btn btn-secondary btn-sm" @click="addInputField(versionForm)">添加输入项</button>
            </div>
          </div>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">定义用户运行时填写的表单，图片/文件输入后续会以对象存储资产引用传给 Worker。</p>
          <div class="space-y-3">
            <div
              v-for="(field, index) in versionForm.input_fields"
              :key="`version-input-${index}`"
              class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-[1fr_1fr_150px_90px_110px_44px]"
            >
              <input v-model.trim="field.name" class="input" placeholder="字段名，如 product_name" />
              <input v-model.trim="field.label" class="input" placeholder="用户看到的名称，如 商品名称" />
              <Select v-model="field.type" :options="inputTypeOptions" />
              <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input v-model="field.required" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                必填
              </label>
              <input v-model.trim="field.asset_role" class="input" placeholder="资产角色，如 reference" />
              <button type="button" class="btn btn-secondary btn-icon" title="删除" @click="removeInputField(versionForm, index)">
                <Icon name="trash" size="sm" />
              </button>
              <div v-if="field.type === 'select'" class="md:col-span-6">
                <div class="mb-2 flex items-center justify-between gap-3">
                  <label class="input-label mb-0">下拉选项</label>
                  <button type="button" class="btn btn-secondary btn-sm" @click="addSelectOption(field)">添加选项</button>
                </div>
                <div class="space-y-2">
                  <div
                    v-for="(option, optionIndex) in selectOptionsForField(field)"
                    :key="`version-input-${index}-option-${optionIndex}`"
                    class="grid grid-cols-1 gap-2 md:grid-cols-[1fr_1fr_44px]"
                  >
                    <input v-model.trim="option.label" class="input" placeholder="用户看到的中文，如 文生视频" @input="syncSelectOptionString(field)" />
                    <input v-model.trim="option.value" class="input" placeholder="提交给 Worker 的值，如 text_to_video" @input="syncSelectOptionString(field)" />
                    <button type="button" class="btn btn-secondary btn-icon" title="删除选项" @click="removeSelectOption(field, optionIndex)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">左边是用户看到的文字，右边是提交给 Worker 的真实值；例如“文生视频 / text_to_video”。</p>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">用户可见结果</span>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">新版本会继承当前结果结构，可以按 Worker 实际输出继续调整。</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="addOutputField(versionForm)">添加结果字段</button>
          </div>
          <div class="space-y-3">
            <div v-for="(field, index) in versionForm.output_fields" :key="`version-output-${index}`" class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-[1fr_1fr_160px_100px_44px]">
              <input v-model.trim="field.name" class="input" placeholder="输出字段，如 result" />
              <input v-model.trim="field.label" class="input" placeholder="用户看到的名称" />
              <Select v-model="field.type" :options="outputTypeOptions" />
              <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input type="radio" name="version-primary-output" :checked="field.primary" @change="setPrimaryOutputField(versionForm, index)" />
                主要结果
              </label>
              <button type="button" class="btn btn-secondary btn-icon" title="删除" @click="removeOutputField(versionForm, index)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-medium text-gray-800 dark:text-gray-100">模型能力绑定</span>
                <span class="badge badge-gray">模型绑定</span>
              </div>
              <button type="button" class="btn btn-secondary btn-sm" @click="addModelRole(versionForm)">添加能力</button>
            </div>
            <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">这里只配置 Worker 需要调用哪些模型能力，以及每个能力允许用户从哪个 Key 分组里选择。管理员不填写用户 Key。</p>
            <div class="space-y-3">
              <div
                v-for="(role, index) in versionForm.model_roles"
                :key="`version-role-${index}`"
                class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-3 dark:border-dark-700 md:grid-cols-2"
              >
                <div>
                  <label class="input-label">能力编号（Worker 用）</label>
                  <input v-model.trim="role.node_id" class="input" placeholder="prompt_rewrite" />
                </div>
                <div>
                  <label class="input-label">调用用途</label>
                  <input v-model.trim="role.role" class="input" placeholder="generate / rewrite / vision" />
                </div>
                <div>
                  <label class="input-label">能力</label>
                  <Select v-model="role.capability" :options="capabilityOptions" />
                </div>
                <div>
                  <label class="input-label">模型厂商 <span class="text-red-500">*</span></label>
                  <Select v-model="role.provider" :options="providerOptions" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">用户运行时只能选择该厂商分组下的 Key。</p>
                </div>
                <div>
                  <label class="input-label">模型名</label>
                  <input v-model.trim="role.model" class="input" placeholder="填平台可调用模型" />
                </div>
                <div>
                  <label class="input-label">允许的 Key 分组</label>
                  <Select v-model="role.model_group_id" :options="modelGroupOptionsForRole(role)" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只显示所选厂商下的分组；不限分组表示该厂商下任意可用 Key。</p>
                </div>
                <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                  <input v-model="role.required" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                  运行前必须选择 Key
                </label>
                <div class="flex items-end md:justify-end">
                  <button type="button" class="btn btn-secondary btn-icon" title="删除该模型角色" @click="removeModelRole(versionForm, index)">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-800 dark:text-gray-100">能力与产物</span>
              <span class="badge badge-gray">发布设置</span>
            </div>
            <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">声明应用需要哪些模型能力，以及运行结果允许保存哪些对象存储产物。</p>
            <div v-if="versionForm.capabilities" class="grid grid-cols-2 gap-3 md:grid-cols-3">
              <label v-for="item in capabilityOptions" :key="`version-cap-${item.value}`" class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input
                  v-model="versionForm.capabilities[item.value]"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                {{ item.label }}
              </label>
            </div>
            <div v-if="versionForm.artifact_policy" class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">结果保留天数</label>
                <input v-model.number="versionForm.artifact_policy.retention_days" type="number" min="0" class="input" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">0 表示长期保留；大于 0 才会在开启清理任务后到期清理。</p>
              </div>
              <div>
                <label class="input-label">单文件上限 MB</label>
                <input v-model.number="versionForm.artifact_policy.max_file_mb" type="number" min="1" class="input" />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">限制 Worker 上传到对象存储的单个产物大小。</p>
              </div>
            </div>
            <div v-if="versionForm.artifact_policy?.allowed_types" class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-3">
              <label v-for="item in artifactTypeOptions" :key="`version-artifact-${item.value}`" class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input
                  v-model="versionForm.artifact_policy.allowed_types[item.value]"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                {{ item.label }}
              </label>
            </div>
          </div>
        </div>

        <div>
          <label class="input-label">变更说明</label>
          <textarea v-model="versionForm.changelog" rows="2" class="input" placeholder="本版本的能力说明或部署备注"></textarea>
        </div>
        <details class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">预览新版本的用户界面</summary>
          <div class="mt-3">
            <AgentAppVersionPreview
              :name="selectedApp?.name"
              :description="selectedApp?.description"
              :input-fields="versionForm.input_fields"
              :model-roles="versionForm.model_roles"
              :output-fields="versionForm.output_fields"
            />
          </div>
        </details>
      </form>

      <template #footer>
        <button class="btn btn-secondary" type="button" @click="closeVersionDialog">取消</button>
        <button class="btn btn-primary" type="submit" form="agent-version-form" :disabled="submitting">
          {{ submitting ? '保存中...' : '保存版本' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showVersionsDialog"
      :title="selectedApp ? `版本列表：${selectedApp.name}` : '版本列表'"
      width="extra-wide"
      @close="showVersionsDialog = false"
    >
      <div v-if="versionsLoading" class="py-8 text-center text-sm text-gray-500">加载中...</div>
      <div v-else-if="versions.length === 0" class="py-8 text-center text-sm text-gray-500">暂无版本</div>
      <div v-else class="space-y-3">
        <div
          v-for="version in versions"
          :key="version.id"
          class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
        >
          <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ version.version }}</span>
                <span :class="['badge', appStatusBadgeClass(version.status)]">{{ appStatusLabel(version.status) }}</span>
                <span class="badge badge-gray">{{ runtimeTypeLabel(version.runtime_type) }}</span>
              </div>
              <div class="mt-2 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 md:grid-cols-2">
                <span>Host：{{ version.worker_host?.name || '-' }}</span>
                <span>Route：<code class="code">{{ version.worker_route || '-' }}</code></span>
                <span>镜像：{{ version.image_ref || '-' }}</span>
                <span>源码：{{ version.source_ref || '-' }}</span>
              </div>
              <p v-if="version.changelog" class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ version.changelog }}</p>
              <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-3">
                <div class="rounded border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900">
                  <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">用户输入</div>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in versionInputSummary(version)"
                      :key="item"
                      class="badge badge-gray"
                    >
                      {{ item }}
                    </span>
                  </div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900">
                  <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">模型用量</div>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in versionModelSummary(version)"
                      :key="item"
                      class="badge badge-primary"
                    >
                      {{ item }}
                    </span>
                  </div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900">
                  <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">结果保存</div>
                  <div class="text-sm text-gray-700 dark:text-gray-300">{{ versionArtifactSummary(version) }}</div>
                  <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ versionCapabilitySummary(version) }}</div>
                </div>
              </div>
              <details class="mt-3 rounded border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900">
                <summary class="cursor-pointer text-xs font-medium text-gray-500 dark:text-gray-400">查看原始版本配置</summary>
                <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
                  <pre class="max-h-36 overflow-auto rounded bg-white p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ formatJSON(version.input_schema_json) }}</pre>
                  <pre class="max-h-36 overflow-auto rounded bg-white p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ formatJSON(version.node_model_policy_json) }}</pre>
                  <pre class="max-h-36 overflow-auto rounded bg-white p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ formatJSON(version.capabilities_json) }}</pre>
                  <pre class="max-h-36 overflow-auto rounded bg-white p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ formatJSON(version.artifact_policy_json) }}</pre>
                </div>
              </details>
            </div>
            <div class="flex flex-shrink-0 flex-col items-end gap-2">
              <span class="text-xs text-gray-500">{{ formatDateTime(version.created_at) }}</span>
              <div class="flex flex-wrap justify-end gap-2">
                <button
                  v-if="selectedApp?.published_version_id !== version.id"
                  type="button"
                  class="btn btn-primary btn-sm"
                  :disabled="versionActionLoadingId === version.id"
                  @click="publishAppVersion(version)"
                >
                  发布为当前版本
                </button>
                <button
                  v-if="version.status !== 'disabled'"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="versionActionLoadingId === version.id"
                  @click="setAppVersionStatus(version, 'disabled')"
                >
                  禁用版本
                </button>
                <button
                  v-else
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="versionActionLoadingId === version.id"
                  @click="setAppVersionStatus(version, 'draft')"
                >
                  恢复草稿
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { Column } from '@/components/common/types'
import type { AdminGroup, AgentApp, AgentAppVersion, AgentWorkerHost, CreateAgentAppRequest, GroupPlatform, PaginatedResponse } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AgentAppVersionPreview from '@/components/agent/AgentAppVersionPreview.vue'
import agentAppsAPI from '@/api/admin/agentApps'
import agentWorkerHostsAPI from '@/api/admin/agentWorkerHosts'
import groupsAPI from '@/api/admin/groups'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const toast = {
  success: (message: string) => appStore.showSuccess(message),
  error: (message: string) => appStore.showError(message)
}

const loading = ref(false)
const submitting = ref(false)
const iconUploading = ref(false)
const iconInputRef = ref<HTMLInputElement | null>(null)
const appIconPreviewURL = ref('')
const apps = ref<AgentApp[]>([])
const searchQuery = ref('')
const filters = reactive({
  status: '',
  app_type: ''
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

const showAppDialog = ref(false)
const showEditAppDialog = ref(false)
const showVersionDialog = ref(false)
const showVersionsDialog = ref(false)
const selectedApp = ref<AgentApp | null>(null)
const workerHosts = ref<AgentWorkerHost[]>([])
const modelGroups = ref<AdminGroup[]>([])
const versions = ref<AgentAppVersion[]>([])
const versionsLoading = ref(false)
const versionActionLoadingId = ref<number | null>(null)
const editingApp = ref<AgentApp | null>(null)
const editIconUploading = ref(false)
const editIconInputRef = ref<HTMLInputElement | null>(null)
const editIconPreviewURL = ref('')

type InputFieldType = 'text' | 'textarea' | 'select' | 'image' | 'file' | 'audio' | 'video' | 'number' | 'boolean' | 'date'
type OutputFieldType = 'text' | 'number' | 'boolean' | 'list' | 'table' | 'object'
type CapabilityKey = 'text' | 'image' | 'video' | 'audio' | 'vision' | 'file' | 'tool'
type ArtifactTypeKey = 'json' | 'image' | 'video' | 'audio' | 'file' | 'log'

const capabilityKeys: CapabilityKey[] = ['text', 'image', 'video', 'audio', 'vision', 'file', 'tool']
const artifactTypeKeys: ArtifactTypeKey[] = ['json', 'image', 'video', 'audio', 'file', 'log']

interface InputFieldForm {
  name: string
  label: string
  type: InputFieldType
  required: boolean
  asset_role: string
  options: string
  select_options?: SelectOptionForm[]
}

interface SelectOptionForm {
  label: string
  value: string
}

interface ModelRoleForm {
  node_id: string
  role: string
  capability: CapabilityKey
  provider: GroupPlatform | ''
  model: string
  model_group_id: number | ''
  required: boolean
}

interface OutputFieldForm {
  name: string
  label: string
  type: OutputFieldType
  primary: boolean
}

interface ArtifactPolicyForm {
  retention_days: number
  max_file_mb: number
  allowed_types: Record<ArtifactTypeKey, boolean>
}

const appForm = reactive<CreateAgentAppRequest>({
  name: '',
  slug: '',
  description: '',
  icon_url: '',
  category: '',
  app_type: 'agent',
  visibility: 'public',
  status: 'published'
})

const editAppForm = reactive<CreateAgentAppRequest>({
  name: '',
  slug: '',
  description: '',
  icon_url: '',
  category: '',
  app_type: 'agent',
  visibility: 'public',
  status: 'published'
})

const publishVersionForm = reactive({
  version: 'v1.0.0',
  status: 'published',
  runtime_type: 'worker',
  worker_host_id: '' as number | '',
  worker_route: '',
  worker_health_route: '/health',
  image_ref: '',
  source_ref: '',
  input_fields: [] as InputFieldForm[],
  output_fields: defaultOutputFields(),
  model_roles: [] as ModelRoleForm[],
  capabilities: emptyCapabilities(),
  artifact_policy: defaultArtifactPolicyForm(),
  changelog: ''
})

const versionForm = reactive({
  version: '',
  status: 'draft',
  runtime_type: 'worker',
  worker_host_id: '' as number | '',
  worker_route: '',
  worker_health_route: '',
  image_ref: '',
  source_ref: '',
  input_fields: [] as InputFieldForm[],
  output_fields: defaultOutputFields(),
  model_roles: [] as ModelRoleForm[],
  capabilities: emptyCapabilities(),
  artifact_policy: defaultArtifactPolicyForm(),
  changelog: ''
})

const columns: Column[] = [
  { key: 'name', label: '应用', sortable: true },
  { key: 'app_type', label: '类型', sortable: true },
  { key: 'status', label: '状态', sortable: true },
  { key: 'visibility', label: '可见性' },
  { key: 'created_at', label: '创建时间', sortable: true },
  { key: 'actions', label: '操作' }
]

const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '草稿', value: 'draft' },
  { label: '已发布', value: 'published' },
  { label: '禁用', value: 'disabled' },
  { label: '归档', value: 'archived' }
]

const typeFilterOptions = [
  { label: '全部类型', value: '' },
  { label: '智能体', value: 'agent' },
  { label: '工作流', value: 'workflow' },
  { label: '提示词', value: 'prompt' },
  { label: '外部 Worker', value: 'external' }
]

const typeEditOptions = typeFilterOptions.filter(item => item.value !== '')
const statusEditOptions = statusFilterOptions.filter(item => item.value !== '')
const visibilityOptions = [
  { label: '不对用户开放', value: 'private' },
  { label: '所有用户可用', value: 'public' }
]
const runtimeTypeOptions = [
  { label: 'Worker', value: 'worker' }
]

const inputTypeOptions = [
  { label: '单行文本', value: 'text' },
  { label: '多行文本', value: 'textarea' },
  { label: '下拉选择', value: 'select' },
  { label: '图片上传', value: 'image' },
  { label: '文件上传', value: 'file' },
  { label: '音频上传', value: 'audio' },
  { label: '视频上传', value: 'video' },
  { label: '数字', value: 'number' },
  { label: '开关', value: 'boolean' },
  { label: '日期', value: 'date' }
]

const outputTypeOptions = [
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '是/否', value: 'boolean' },
  { label: '列表', value: 'list' },
  { label: '表格', value: 'table' },
  { label: '结构化信息', value: 'object' }
]

const capabilityOptions: Array<{ label: string; value: CapabilityKey }> = [
  { label: '文本', value: 'text' },
  { label: '图片', value: 'image' },
  { label: '视频', value: 'video' },
  { label: '音频', value: 'audio' },
  { label: '视觉理解', value: 'vision' },
  { label: '文件', value: 'file' },
  { label: '工具调用', value: 'tool' }
]

const providerOptions: Array<{ label: string; value: GroupPlatform | '' }> = [
  { label: '请选择模型厂商', value: '' },
  { label: 'OpenAI', value: 'openai' },
  { label: 'Anthropic', value: 'anthropic' },
  { label: 'Gemini', value: 'gemini' },
  { label: 'Antigravity', value: 'antigravity' },
  { label: 'Grok', value: 'grok' }
]

const artifactTypeOptions: Array<{ label: string; value: ArtifactTypeKey }> = [
  { label: 'JSON', value: 'json' },
  { label: '图片', value: 'image' },
  { label: '视频', value: 'video' },
  { label: '音频', value: 'audio' },
  { label: '文件', value: 'file' },
  { label: '日志', value: 'log' }
]

const workerHostOptions = computed(() => [
  { label: '请选择 Worker Host', value: '' },
  ...workerHosts.value.map(host => ({
    label: `${host.name} (${host.base_url})`,
    value: host.id
  }))
])

function modelGroupOptionsForRole(role: ModelRoleForm) {
  const provider = role.provider
  const groups = provider
    ? modelGroups.value.filter(group => group.platform === provider)
    : modelGroups.value
  return [
    { label: provider ? `不限${providerLabel(provider)}分组` : '请先选择模型厂商', value: '' },
    ...groups.map(group => ({
      label: `${group.name} / ${providerLabel(group.platform)} (#${group.id})`,
      value: group.id
    }))
  ]
}

let searchTimer: number | null = null

async function loadApps() {
  loading.value = true
  try {
    const data: PaginatedResponse<AgentApp> = await agentAppsAPI.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        app_type: filters.app_type || undefined,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      }
    )
    apps.value = data.items
    pagination.total = data.total
    pagination.page = data.page
    pagination.page_size = data.page_size
    pagination.pages = data.pages
  } catch (error: any) {
    toast.error(error?.message || '加载应用失败')
  } finally {
    loading.value = false
  }
}

async function loadWorkerHosts() {
  try {
    workerHosts.value = await agentWorkerHostsAPI.getAll('active')
  } catch (error: any) {
    toast.error(error?.message || '加载 Worker Host 失败')
  }
}

async function loadModelGroups() {
  try {
    modelGroups.value = await groupsAPI.getAll()
  } catch (error: any) {
    toast.error(error?.message || '加载模型分组失败')
  }
}

function handleSearch() {
  if (searchTimer) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    pagination.page = 1
    loadApps()
  }, 300)
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  loadApps()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadApps()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadApps()
}

function openAppDialog() {
  Object.assign(appForm, {
    name: '',
    slug: '',
    description: '',
    icon_url: '',
    category: '',
    app_type: 'agent',
    visibility: 'public',
    status: 'published'
  })
  resetPublishVersionForm()
  appIconPreviewURL.value = ''
  clearIconFileInput()
  Promise.all([loadWorkerHosts(), loadModelGroups()])
  showAppDialog.value = true
}

function openEditAppDialog(app: AgentApp) {
  editingApp.value = app
  Object.assign(editAppForm, {
    name: app.name,
    slug: app.slug,
    description: app.description || '',
    icon_url: app.icon_url || '',
    category: app.category || '',
    app_type: app.app_type,
    visibility: app.visibility,
    status: app.status
  })
  editIconPreviewURL.value = app.icon_url?.startsWith('http') ? app.icon_url : ''
  showEditAppDialog.value = true
  if (app.icon_url && !editIconPreviewURL.value) void loadEditAppIconPreview(app)
}

async function loadEditAppIconPreview(app: AgentApp) {
  try {
    const result = await agentAppsAPI.getIconURL(app.id)
    if (editingApp.value?.id === app.id) editIconPreviewURL.value = result.url
  } catch (error: any) {
    if (editingApp.value?.id === app.id) toast.error(error?.message || '加载应用图标失败')
  }
}

function closeEditAppDialog() {
  showEditAppDialog.value = false
  editingApp.value = null
  editIconPreviewURL.value = ''
}

function triggerEditIconUpload() {
  editIconInputRef.value?.click()
}

async function handleEditIconSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
    toast.error('应用图标只支持 PNG、JPG 或 WebP')
    return
  }
  if (file.size > 2 * 1024 * 1024) {
    toast.error('应用图标不能超过 2MB')
    return
  }
  editIconUploading.value = true
  try {
    const result = await agentAppsAPI.uploadIcon(file)
    editAppForm.icon_url = result.url
    editIconPreviewURL.value = result.preview_url || result.url
  } catch (error: any) {
    toast.error(error?.message || '上传应用图标失败')
  } finally {
    editIconUploading.value = false
  }
}

async function saveEditedApp() {
  if (!editingApp.value) return
  submitting.value = true
  try {
    await agentAppsAPI.update(editingApp.value.id, { ...editAppForm })
    toast.success('应用资料已更新')
    closeEditAppDialog()
    await loadApps()
  } catch (error: any) {
    toast.error(error?.message || '更新应用失败')
  } finally {
    submitting.value = false
  }
}

async function deleteApp(app: AgentApp) {
  if (!window.confirm(`删除“${app.name}”后用户将无法继续使用，历史运行记录仍会保留。确认删除吗？`)) return
  try {
    await agentAppsAPI.remove(app.id)
    toast.success('应用已删除')
    await loadApps()
  } catch (error: any) {
    toast.error(error?.message || '删除应用失败')
  }
}

function triggerIconUpload() {
  if (iconUploading.value) return
  iconInputRef.value?.click()
}

async function handleIconSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  const supportedTypes = ['image/png', 'image/jpeg', 'image/webp']
  if (!supportedTypes.includes(file.type)) {
    toast.error('应用图标只支持 PNG、JPG 或 WebP')
    clearIconFileInput()
    return
  }
  if (file.size > 2 * 1024 * 1024) {
    toast.error('应用图标不能超过 2MB')
    clearIconFileInput()
    return
  }

  iconUploading.value = true
  try {
    const result = await agentAppsAPI.uploadIcon(file)
    appForm.icon_url = result.url
    appIconPreviewURL.value = result.preview_url || result.url
    toast.success('应用图标已上传')
  } catch (error: any) {
    toast.error(error?.message || '上传应用图标失败，请检查对象存储配置')
  } finally {
    iconUploading.value = false
    clearIconFileInput()
  }
}

function clearAppIcon() {
  appForm.icon_url = ''
  appIconPreviewURL.value = ''
  clearIconFileInput()
}

function clearIconFileInput() {
  if (iconInputRef.value) {
    iconInputRef.value.value = ''
  }
}

async function handleCreateApp() {
  ensureVersionFormDefaults(publishVersionForm)
  const versionPayload = buildVersionPayload(publishVersionForm)
  if (!versionPayload) return

  submitting.value = true
  try {
    await agentAppsAPI.createWithVersion({
      app: { ...appForm },
      version: versionPayload
    })
    toast.success('应用已发布')
    showAppDialog.value = false
    await loadApps()
  } catch (error: any) {
    toast.error(error?.message || '发布应用失败')
  } finally {
    submitting.value = false
  }
}

async function openVersionDialog(app: AgentApp) {
  selectedApp.value = app
  const [, , versions] = await Promise.all([
    loadWorkerHosts(),
    loadModelGroups(),
    agentAppsAPI.listVersions(app.id)
  ])
  const source = versions.find(item => item.id === app.published_version_id) || versions[0]
  if (source) {
    applyVersionToForm(source, versionForm)
  } else {
    Object.assign(versionForm, {
      version: 'v1.0.0',
      status: 'published',
      runtime_type: 'worker',
      worker_host_id: '',
      worker_route: '/image/runs',
      worker_health_route: '/health',
      image_ref: '',
      source_ref: 'sub2api-app-worker:image',
      input_fields: defaultImageInputFields(),
      output_fields: defaultOutputFields(),
      model_roles: defaultImageModelRoles(),
      capabilities: defaultImageCapabilities(),
      artifact_policy: defaultArtifactPolicyForm(),
      changelog: '发布新版本'
    })
  }
  ensureVersionFormDefaults(versionForm)
  showVersionDialog.value = true
}

function applyVersionToForm(version: AgentAppVersion, target: VersionFormLike) {
  Object.assign(target, {
    version: nextVersionName(version.version),
    status: 'published',
    runtime_type: 'worker',
    worker_host_id: version.worker_host_id || '',
    worker_route: version.worker_route || '/runs',
    worker_health_route: version.worker_health_route || '/health',
    image_ref: version.image_ref || '',
    source_ref: version.source_ref || '',
    input_fields: inputFieldsFromSchema(version.input_schema_json),
    output_fields: outputFieldsFromSchema(version.output_schema_json),
    model_roles: modelRolesFromPolicy(version.node_model_policy_json),
    capabilities: capabilitiesFromJSON(version.capabilities_json),
    artifact_policy: artifactPolicyFromJSON(version.artifact_policy_json),
    changelog: `基于 ${version.version} 发布新版本`
  })
}

function nextVersionName(value: string): string {
  const match = String(value || '').trim().match(/^v?(\d+)\.(\d+)\.(\d+)$/i)
  if (!match) return ''
  return `v${match[1]}.${match[2]}.${Number(match[3]) + 1}`
}

function inputFieldsFromSchema(schema: Record<string, unknown>): InputFieldForm[] {
  const properties = schema?.properties && typeof schema.properties === 'object'
    ? schema.properties as Record<string, unknown>
    : {}
  const required = new Set(Array.isArray(schema?.required) ? schema.required.filter((item): item is string => typeof item === 'string') : [])
  return Object.entries(properties).map(([name, raw]) => {
    const field = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
    const declaredKind = String(field['x-input-kind'] || '')
    const type: InputFieldType = ['text', 'textarea', 'select', 'image', 'file', 'audio', 'video', 'number', 'boolean', 'date'].includes(declaredKind)
      ? declaredKind as InputFieldType
      : field.type === 'number' ? 'number' : 'text'
    const rawOptions = Array.isArray(field['x-options']) ? field['x-options'] : Array.isArray(field.enum) ? field.enum : []
    const selectOptions = rawOptions
      .map((item): SelectOptionForm | null => {
        if (item && typeof item === 'object') {
          const option = item as Record<string, unknown>
          const value = String(option.value ?? '').trim()
          const label = String(option.label ?? value).trim()
          return value ? { label: label || value, value } : null
        }
        const value = String(item ?? '').trim()
        return value ? { label: value, value } : null
      })
      .filter((item): item is SelectOptionForm => item !== null)
    const options = serializeSelectOptions(selectOptions)
    return {
      name,
      label: String(field.title || name),
      type,
      required: required.has(name),
      asset_role: String(field['x-asset-role'] || ''),
      options,
      select_options: selectOptions
    }
  })
}

function outputFieldsFromSchema(schema: Record<string, unknown>): OutputFieldForm[] {
  const properties = schema?.properties && typeof schema.properties === 'object'
    ? schema.properties as Record<string, unknown>
    : {}
  const primary = String(schema?.['x-primary-field'] || '')
  const fields = Object.entries(properties)
    .filter(([name]) => name !== 'artifact' && name !== 'artifacts')
    .map(([name, raw]) => {
      const field = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
      let type: OutputFieldType = 'object'
      if (field['x-display-kind'] === 'table') type = 'table'
      else if (field.type === 'string') type = 'text'
      else if (field.type === 'number' || field.type === 'integer') type = 'number'
      else if (field.type === 'boolean') type = 'boolean'
      else if (field.type === 'array') type = 'list'
      return { name, label: String(field.title || name), type, primary: name === primary }
    })
  if (!fields.length) return defaultOutputFields()
  if (!fields.some(field => field.primary)) fields[0].primary = true
  return fields
}

function modelRolesFromPolicy(policy: Record<string, unknown>): ModelRoleForm[] {
  return Object.entries(policy || {}).map(([policyKey, raw]) => {
    const item = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
    const parts = policyKey.split('.')
    const provider = normalizeProvider(item.provider || item.platform)
    return {
      node_id: String(item.node_id || parts[0] || ''),
      role: String(item.role || parts.slice(1).join('.') || 'generate'),
      capability: capabilityKeys.includes(item.capability as CapabilityKey) ? item.capability as CapabilityKey : 'text',
      provider,
      model: String(item.model || ''),
      model_group_id: normalizeModelGroupIDForProvider(item.model_group_id, provider),
      required: item.optional !== true
    }
  })
}

function capabilitiesFromJSON(raw: Record<string, unknown>): Record<CapabilityKey, boolean> {
  const source = raw || {}
  const listed = Array.isArray(source.capabilities) ? source.capabilities.map(String) : []
  return capabilityMap(capabilityKeys.filter(key => source[key] === true || listed.includes(key)))
}

function artifactPolicyFromJSON(raw: Record<string, unknown>): ArtifactPolicyForm {
  const allowed = Array.isArray(raw?.allowed_types) ? raw.allowed_types.map(String) : []
  return {
    retention_days: Math.max(0, Number(raw?.retention_days) || 0),
    max_file_mb: Math.max(1, Number(raw?.max_file_mb) || 100),
    allowed_types: artifactTypeKeys.reduce((result, key) => {
      result[key] = allowed.length === 0 || allowed.includes(key)
      return result
    }, {} as Record<ArtifactTypeKey, boolean>)
  }
}

function closeVersionDialog() {
  showVersionDialog.value = false
  selectedApp.value = null
}

async function handleCreateVersion() {
  if (!selectedApp.value) return
  ensureVersionFormDefaults(versionForm)
  const versionPayload = buildVersionPayload(versionForm)
  if (!versionPayload) return

  submitting.value = true
  try {
    await agentAppsAPI.createVersion(selectedApp.value.id, versionPayload)
    toast.success('应用版本已保存')
    closeVersionDialog()
  } catch (error: any) {
    toast.error(error?.message || '保存应用版本失败')
  } finally {
    submitting.value = false
  }
}

type VersionFormLike = {
  version: string
  status: string
  runtime_type: string
  worker_host_id: number | ''
  worker_route: string
  worker_health_route: string
  image_ref: string
  source_ref: string
  changelog: string
  input_fields: InputFieldForm[]
  output_fields: OutputFieldForm[]
  model_roles: ModelRoleForm[]
  capabilities: Record<CapabilityKey, boolean>
  artifact_policy: ArtifactPolicyForm
}

function buildVersionPayload(form: VersionFormLike) {
  if (form.runtime_type === 'worker' && typeof form.worker_host_id !== 'number') {
    toast.error('Worker 运行类型必须选择 Worker Host')
    return null
  }

  const inputFields = normalizeInputFields(form.input_fields)
  if (inputFields.length === 0) {
    toast.error('至少需要一个输入项')
    return null
  }
  const emptySelectField = inputFields.find(field => field.type === 'select' && normalizedSelectOptions(field).length === 0)
  if (emptySelectField) {
    toast.error(`${emptySelectField.label || emptySelectField.name} 是下拉选择，请先填写选项`)
    return null
  }
  if (!form.output_fields.some(field => field.name.trim())) {
    toast.error('至少需要定义一个用户可见的结果字段')
    return null
  }
  const modelRoles = normalizeModelRoles(form.model_roles)
  if (modelRoles.length === 0) {
    toast.error('至少需要一个模型能力绑定')
    return null
  }
  const missingProviderRole = modelRoles.find(role => !role.provider)
  if (missingProviderRole) {
    toast.error(`${missingProviderRole.node_id}.${missingProviderRole.role} 需要选择模型厂商`)
    return null
  }
  const missingModelRole = modelRoles.find(role => !role.model)
  if (missingModelRole) {
    toast.error(`${missingModelRole.node_id}.${missingModelRole.role} 需要填写模型`)
    return null
  }

  return {
    version: form.version,
    status: form.status,
    runtime_type: form.runtime_type,
    worker_host_id: typeof form.worker_host_id === 'number' ? form.worker_host_id : undefined,
    worker_route: form.worker_route,
    worker_health_route: form.worker_health_route,
    image_ref: form.image_ref,
    source_ref: form.source_ref,
    input_schema_json: buildInputSchema(inputFields),
    output_schema_json: buildOutputSchema(form),
    capabilities_json: buildCapabilities(form.capabilities),
    default_model_config_json: buildDefaultModelConfig(modelRoles),
    node_model_policy_json: buildNodeModelPolicy(modelRoles),
    artifact_policy_json: buildArtifactPolicy(form.artifact_policy),
    changelog: form.changelog
  }
}

function resetPublishVersionForm() {
  Object.assign(publishVersionForm, {
    version: 'v1.0.0',
    status: 'published',
    runtime_type: 'worker',
    worker_host_id: '',
    worker_route: '/image/runs',
    worker_health_route: '/health',
    image_ref: '',
    source_ref: 'sub2api-app-worker:image',
    input_fields: defaultImageInputFields(),
    output_fields: defaultOutputFields(),
    model_roles: defaultImageModelRoles(),
    capabilities: defaultImageCapabilities(),
    artifact_policy: defaultArtifactPolicyForm(),
    changelog: '首次发布'
  })
  ensureVersionFormDefaults(publishVersionForm)
}

function applyVersionTemplate(type: 'text' | 'image' | 'grokVideo' | 'productMarketing' | 'academicPaper', target: VersionFormLike = publishVersionForm) {
  if (type === 'text') {
    Object.assign(target, {
      worker_route: '/runs',
      source_ref: 'sub2api-app-worker:text',
      input_fields: [
        { name: 'prompt', label: '提示词', type: 'textarea', required: true, asset_role: '', options: '', select_options: [] }
      ],
      output_fields: defaultOutputFields(),
      model_roles: [
        { node_id: 'text', role: 'generate', capability: 'text', provider: 'openai', model: 'gpt-5.5', model_group_id: '', required: true }
      ],
      capabilities: capabilityMap(['text']),
      artifact_policy: defaultArtifactPolicyForm()
    })
    ensureVersionFormDefaults(target)
    return
  }

  if (type === 'grokVideo') {
    Object.assign(target, {
      worker_route: '/grok-video/runs',
      source_ref: 'sub2api-app-worker:grok-video',
      input_fields: defaultGrokVideoInputFields(),
      output_fields: defaultVideoOutputFields(),
      model_roles: defaultGrokVideoModelRoles(),
      capabilities: capabilityMap(['video', 'image', 'file']),
      artifact_policy: defaultGrokVideoArtifactPolicyForm()
    })
    ensureVersionFormDefaults(target)
    return
  }

  if (type === 'productMarketing') {
    Object.assign(target, {
      worker_route: '/product-marketing/runs',
      source_ref: 'sub2api-app-worker:product-marketing',
      input_fields: defaultProductMarketingInputFields(),
      output_fields: defaultProductMarketingOutputFields(),
      model_roles: defaultProductMarketingModelRoles(),
      capabilities: capabilityMap(['text', 'vision', 'image', 'file']),
      artifact_policy: defaultArtifactPolicyForm()
    })
    ensureVersionFormDefaults(target)
    return
  }

  if (type === 'academicPaper') {
    Object.assign(target, {
      worker_route: '/academic-paper/runs',
      source_ref: 'sub2api-app-worker:academic-paper',
      input_fields: defaultAcademicPaperInputFields(),
      output_fields: defaultAcademicPaperOutputFields(),
      model_roles: defaultAcademicPaperModelRoles(),
      capabilities: capabilityMap(['text', 'file']),
      artifact_policy: defaultAcademicPaperArtifactPolicyForm()
    })
    ensureVersionFormDefaults(target)
    return
  }

  Object.assign(target, {
    worker_route: '/image/runs',
    source_ref: 'sub2api-app-worker:image',
    input_fields: defaultImageInputFields(),
    output_fields: defaultOutputFields(),
    model_roles: defaultImageModelRoles(),
    capabilities: defaultImageCapabilities(),
    artifact_policy: defaultArtifactPolicyForm()
  })
  ensureVersionFormDefaults(target)
}

function defaultImageCapabilities() {
  return capabilityMap(['image'])
}

function defaultImageInputFields(): InputFieldForm[] {
  return [
    { name: 'prompt', label: '提示词', type: 'textarea', required: true, asset_role: '', options: '', select_options: [] },
    { name: 'reference_image', label: '参考图片（可选）', type: 'image', required: false, asset_role: 'reference', options: '', select_options: [] }
  ]
}

function defaultGrokVideoInputFields(): InputFieldForm[] {
  const modeOptions = [
    { label: '文生视频', value: 'text_to_video' },
    { label: '图生视频', value: 'image_to_video' },
    { label: '多参考图生视频', value: 'reference_to_video' },
    { label: '视频编辑', value: 'edit_video' },
    { label: '视频续写', value: 'extend_video' }
  ]
  const durationOptions = [
    { label: '5 秒', value: '5' },
    { label: '6 秒', value: '6' },
    { label: '8 秒', value: '8' },
    { label: '10 秒', value: '10' }
  ]
  const resolutionOptions = [
    { label: '720p', value: '720p' }
  ]
  const aspectRatioOptions = [
    { label: '横屏 16:9', value: '16:9' },
    { label: '竖屏 9:16', value: '9:16' },
    { label: '方形 1:1', value: '1:1' }
  ]
  return [
    {
      name: 'mode',
      label: '生成模式',
      type: 'select',
      required: true,
      asset_role: '',
      options: serializeSelectOptions(modeOptions),
      select_options: modeOptions
    },
    { name: 'prompt', label: '提示词 / 画面描述', type: 'textarea', required: true, asset_role: '', options: '', select_options: [] },
    {
      name: 'duration',
      label: '视频时长',
      type: 'select',
      required: false,
      asset_role: '',
      options: serializeSelectOptions(durationOptions),
      select_options: durationOptions
    },
    {
      name: 'resolution',
      label: '视频清晰度',
      type: 'select',
      required: false,
      asset_role: '',
      options: serializeSelectOptions(resolutionOptions),
      select_options: resolutionOptions
    },
    {
      name: 'aspect_ratio',
      label: '画面比例',
      type: 'select',
      required: false,
      asset_role: '',
      options: serializeSelectOptions(aspectRatioOptions),
      select_options: aspectRatioOptions
    },
    { name: 'source_image', label: '起始图片 / 人物图片', type: 'image', required: false, asset_role: 'source', options: '', select_options: [] },
    { name: 'reference_images', label: '参考图片', type: 'image', required: false, asset_role: 'reference', options: '', select_options: [] },
    { name: 'source_video', label: '原始视频', type: 'video', required: false, asset_role: 'source', options: '', select_options: [] }
  ]
}

function defaultProductMarketingInputFields(): InputFieldForm[] {
  const platformOptions = [
    { label: '淘宝 / 天猫', value: 'taobao' },
    { label: '京东', value: 'jd' },
    { label: '抖音', value: 'douyin' },
    { label: '小红书', value: 'xiaohongshu' },
    { label: 'Amazon', value: 'amazon' },
    { label: '独立站', value: 'independent_store' }
  ]
  const styleOptions = [
    { label: '干净通勤', value: 'clean_urban' },
    { label: '高级商业', value: 'premium_commercial' },
    { label: '自然生活方式', value: 'natural_lifestyle' },
    { label: '社媒醒目', value: 'social_bold' },
    { label: '极简棚拍', value: 'minimal_studio' }
  ]
  const outputCountOptions = [1, 2, 3, 4].map(value => ({ label: `${value} 张`, value: String(value) }))
  return [
    { name: 'product_name', label: '商品名称', type: 'text', required: true, asset_role: '', options: '', select_options: [] },
    { name: 'selling_points', label: '商品卖点', type: 'textarea', required: true, asset_role: '', options: '', select_options: [] },
    { name: 'target_audience', label: '目标人群', type: 'text', required: false, asset_role: '', options: '', select_options: [] },
    { name: 'platform', label: '投放平台', type: 'select', required: true, asset_role: '', options: serializeSelectOptions(platformOptions), select_options: platformOptions },
    { name: 'visual_style', label: '视觉风格', type: 'select', required: true, asset_role: '', options: serializeSelectOptions(styleOptions), select_options: styleOptions },
    { name: 'language', label: '文案语言', type: 'text', required: false, asset_role: '', options: '', select_options: [] },
    { name: 'campaign_goal', label: '营销目标', type: 'text', required: false, asset_role: '', options: '', select_options: [] },
    { name: 'output_count', label: '生成图片数量', type: 'select', required: true, asset_role: '', options: serializeSelectOptions(outputCountOptions), select_options: outputCountOptions },
    { name: 'product_images', label: '商品参考图（可多选）', type: 'image', required: false, asset_role: 'reference', options: '', select_options: [] },
    { name: 'additional_requirements', label: '补充要求', type: 'textarea', required: false, asset_role: '', options: '', select_options: [] }
  ]
}

function academicPaperInputField(
  name: string,
  label: string,
  type: InputFieldType = 'text',
  required = false,
  assetRole = ''
): InputFieldForm {
  return { name, label, type, required, asset_role: assetRole, options: '', select_options: [] }
}

function academicPaperSelectField(
  name: string,
  label: string,
  options: SelectOptionForm[],
  required = false
): InputFieldForm {
  const selectOptions = options.map(option => ({ ...option }))
  return {
    name,
    label,
    type: 'select',
    required,
    asset_role: '',
    options: serializeSelectOptions(selectOptions),
    select_options: selectOptions
  }
}

function defaultAcademicPaperInputFields(): InputFieldForm[] {
  const paperTypeOptions = [
    { label: '课程论文', value: 'course_paper' },
    { label: '本科毕业论文', value: 'undergraduate_thesis' },
    { label: '硕士学位论文', value: 'master_thesis' },
    { label: '博士学位论文', value: 'doctoral_dissertation' },
    { label: '期刊论文', value: 'journal_article' },
    { label: '文献综述', value: 'literature_review' },
    { label: '研究报告', value: 'research_report' },
    { label: '案例分析', value: 'case_study' },
    { label: '开题报告', value: 'research_proposal' },
    { label: '其他', value: 'other' }
  ]
  const educationLevelOptions = [
    { label: '专科', value: 'college' },
    { label: '本科', value: 'undergraduate' },
    { label: '硕士', value: 'master' },
    { label: '博士', value: 'doctoral' },
    { label: '职业教育', value: 'vocational' },
    { label: '其他', value: 'other' }
  ]
  const languageOptions = [
    { label: '简体中文', value: 'zh-CN' },
    { label: '繁体中文', value: 'zh-TW' },
    { label: '英语', value: 'en' },
    { label: '中英双语', value: 'zh-en' },
    { label: '其他语言', value: 'other' }
  ]
  const writingStyleOptions = [
    { label: '严谨学术', value: 'academic' },
    { label: '清晰规范', value: 'formal' },
    { label: '实证研究', value: 'empirical' },
    { label: '理论分析', value: 'theoretical' },
    { label: '文献综述', value: 'literature_review' },
    { label: '案例研究', value: 'case_study' },
    { label: '自定义', value: 'custom' }
  ]
  const citationStyleOptions = [
    { label: 'GB/T 7714-2015 顺序编码制', value: 'gbt7714_numeric' },
    { label: 'GB/T 7714-2015 著者-出版年制', value: 'gbt7714_author_year' },
    { label: 'APA 第 7 版', value: 'apa7' },
    { label: 'MLA 第 9 版', value: 'mla9' },
    { label: 'Chicago 著者-日期制', value: 'chicago_author_date' },
    { label: 'Chicago 脚注制', value: 'chicago_notes' },
    { label: 'IEEE', value: 'ieee' },
    { label: 'Vancouver', value: 'vancouver' },
    { label: 'Harvard', value: 'harvard' },
    { label: '自定义', value: 'custom' }
  ]
  const formatPresetOptions = [
    { label: '通用中文论文（推荐）', value: 'standard_cn_academic' },
    { label: '本科毕业论文', value: 'undergraduate_thesis' },
    { label: '硕士 / 博士学位论文', value: 'graduate_thesis' },
    { label: '中文期刊论文', value: 'cn_journal' },
    { label: '英文 APA 论文', value: 'apa_english' },
    { label: '严格按上传模板', value: 'uploaded_template' },
    { label: '完全自定义', value: 'custom' }
  ]
  const pageSizeOptions = [
    { label: 'A4（210 × 297 mm）', value: 'A4' },
    { label: 'A3（297 × 420 mm）', value: 'A3' },
    { label: 'B5（176 × 250 mm）', value: 'B5' },
    { label: 'Letter（216 × 279 mm）', value: 'LETTER' },
    { label: 'Legal（216 × 356 mm）', value: 'LEGAL' },
    { label: '自定义尺寸', value: 'CUSTOM' }
  ]
  const orientationOptions = [
    { label: '纵向', value: 'portrait' },
    { label: '横向', value: 'landscape' }
  ]
  const eastAsiaFontOptions = [
    { label: '宋体', value: '宋体' },
    { label: '黑体', value: '黑体' },
    { label: '仿宋', value: '仿宋' },
    { label: '楷体', value: '楷体' },
    { label: '微软雅黑', value: '微软雅黑' },
    { label: '等线', value: '等线' },
    { label: '华文中宋', value: '华文中宋' },
    { label: '华文宋体', value: '华文宋体' },
    { label: '华文仿宋', value: '华文仿宋' },
    { label: '华文楷体', value: '华文楷体' },
    { label: '方正小标宋简体', value: '方正小标宋简体' },
    { label: '方正仿宋简体', value: '方正仿宋简体' },
    { label: '方正楷体简体', value: '方正楷体简体' },
    { label: '思源宋体', value: '思源宋体' },
    { label: '思源黑体', value: '思源黑体' }
  ]
  const latinFontOptions = [
    { label: 'Times New Roman', value: 'Times New Roman' },
    { label: 'Arial', value: 'Arial' },
    { label: 'Calibri', value: 'Calibri' },
    { label: 'Cambria', value: 'Cambria' },
    { label: 'Georgia', value: 'Georgia' },
    { label: 'Garamond', value: 'Garamond' },
    { label: 'Helvetica', value: 'Helvetica' },
    { label: 'Verdana', value: 'Verdana' },
    { label: 'Tahoma', value: 'Tahoma' },
    { label: 'Courier New', value: 'Courier New' },
    { label: 'Book Antiqua', value: 'Book Antiqua' },
    { label: 'Palatino Linotype', value: 'Palatino Linotype' }
  ]
  const fontSizeOptions = [
    { label: '初号（42 pt）', value: '42' },
    { label: '小初（36 pt）', value: '36' },
    { label: '一号（26 pt）', value: '26' },
    { label: '小一（24 pt）', value: '24' },
    { label: '二号（22 pt）', value: '22' },
    { label: '小二（18 pt）', value: '18' },
    { label: '三号（16 pt）', value: '16' },
    { label: '小三（15 pt）', value: '15' },
    { label: '四号（14 pt）', value: '14' },
    { label: '小四（12 pt）', value: '12' },
    { label: '五号（10.5 pt）', value: '10.5' },
    { label: '小五（9 pt）', value: '9' },
    { label: '六号（7.5 pt）', value: '7.5' },
    { label: '小六（6.5 pt）', value: '6.5' },
    { label: '七号（5.5 pt）', value: '5.5' },
    { label: '八号（5 pt）', value: '5' },
    { label: '11 pt', value: '11' },
    { label: '13 pt', value: '13' },
    { label: '20 pt', value: '20' },
    { label: '28 pt', value: '28' },
    { label: '32 pt', value: '32' }
  ]
  const alignmentOptions = [
    { label: '左对齐', value: 'left' },
    { label: '居中', value: 'center' },
    { label: '右对齐', value: 'right' },
    { label: '两端对齐', value: 'justify' },
    { label: '分散对齐', value: 'distribute' }
  ]
  const lineSpacingModeOptions = [
    { label: '单倍行距', value: 'single' },
    { label: '多倍行距（配合数值）', value: 'multiple' },
    { label: '固定值（pt）', value: 'exact' },
    { label: '最小值（pt）', value: 'at_least' }
  ]
  const headingNumberingStyleOptions = [
    { label: '中文章节：第一章 / 第一节', value: 'chinese_chapter' },
    { label: '阿拉伯数字：1 / 1.1 / 1.1.1', value: 'decimal' },
    { label: '中文序号：一、/（一）/ 1.', value: 'chinese_outline' },
    { label: '不编号', value: 'none' },
    { label: '自定义格式', value: 'custom' }
  ]
  const pageNumberPositionOptions = [
    { label: '页脚', value: 'footer' },
    { label: '页眉', value: 'header' }
  ]
  const pageNumberFormatOptions = [
    { label: '阿拉伯数字：1, 2, 3', value: 'decimal' },
    { label: '大写罗马数字：I, II, III', value: 'upperRoman' },
    { label: '小写罗马数字：i, ii, iii', value: 'lowerRoman' }
  ]

  const styleFields = (
    prefix: string,
    label: string,
    settings: { paragraph?: boolean; pagination?: boolean } = {}
  ): InputFieldForm[] => {
    const fields = [
      academicPaperSelectField(`${prefix}_east_asia_font`, `${label}｜中文字体`, eastAsiaFontOptions),
      academicPaperSelectField(`${prefix}_latin_font`, `${label}｜英文字体`, latinFontOptions),
      academicPaperSelectField(`${prefix}_size_pt`, `${label}｜字号`, fontSizeOptions),
      academicPaperInputField(`${prefix}_bold`, `${label}｜加粗`, 'boolean'),
      academicPaperInputField(`${prefix}_italic`, `${label}｜斜体`, 'boolean'),
      academicPaperSelectField(`${prefix}_alignment`, `${label}｜对齐方式`, alignmentOptions),
      academicPaperSelectField(`${prefix}_line_spacing_mode`, `${label}｜行距类型`, lineSpacingModeOptions),
      academicPaperInputField(`${prefix}_line_spacing_value`, `${label}｜行距值`, 'number'),
      academicPaperInputField(`${prefix}_space_before_pt`, `${label}｜段前（pt）`, 'number'),
      academicPaperInputField(`${prefix}_space_after_pt`, `${label}｜段后（pt）`, 'number')
    ]
    if (settings.paragraph) {
      fields.push(
        academicPaperInputField(`${prefix}_first_line_indent_chars`, `${label}｜首行缩进（字符）`, 'number'),
        academicPaperInputField(`${prefix}_first_line_indent_cm`, `${label}｜首行缩进（cm）`, 'number')
      )
    }
    if (settings.pagination) {
      fields.push(
        academicPaperInputField(`${prefix}_keep_with_next`, `${label}｜与下段同页`, 'boolean'),
        academicPaperInputField(`${prefix}_page_break_before`, `${label}｜段前分页`, 'boolean')
      )
    }
    return fields
  }

  return [
    academicPaperInputField('topic', '论文主题 / 研究方向', 'textarea', true),
    academicPaperInputField('paper_title', '指定论文题目（可选）'),
    academicPaperSelectField('paper_type', '论文类型', paperTypeOptions, true),
    academicPaperInputField('discipline', '学科 / 专业', 'text', true),
    academicPaperSelectField('education_level', '学历层次', educationLevelOptions, true),
    academicPaperSelectField('language', '写作语言', languageOptions, true),
    academicPaperInputField('word_count', '目标字数（建议 1000–50000）', 'number', true),
    academicPaperInputField('outline_requirements', '目录结构 / 章节层级要求', 'textarea'),
    academicPaperInputField('writing_requirements', '写作要求', 'textarea'),
    academicPaperSelectField('writing_style', '写作风格', writingStyleOptions),
    academicPaperInputField('research_method', '研究方法（可填写多个）', 'textarea'),
    academicPaperInputField('additional_requirements', '其他补充要求', 'textarea'),
    academicPaperInputField('abstract_enabled', '生成摘要', 'boolean'),
    academicPaperInputField('abstract_requirements', '摘要要求', 'textarea'),
    academicPaperInputField('keywords_enabled', '生成关键词', 'boolean'),
    academicPaperInputField('keywords_count', '关键词数量', 'number'),
    academicPaperInputField('keywords_requirements', '关键词要求', 'textarea'),
    academicPaperSelectField('citation_style', '引用格式', citationStyleOptions),
    academicPaperInputField('citation_requirements', '引文与脚注要求', 'textarea'),
    academicPaperInputField('reference_requirements', '参考文献数量 / 年限 / 来源要求', 'textarea'),
    academicPaperInputField('reference_materials', '参考资料（可多文件）', 'file', false, 'reference'),
    academicPaperInputField('template_file', '学校 / 期刊 Word 模板（可选）', 'file', false, 'template'),

    academicPaperSelectField('page_format_preset', '格式预设', formatPresetOptions, true),
    academicPaperSelectField('page_size', '纸张大小', pageSizeOptions),
    academicPaperSelectField('page_orientation', '纸张方向', orientationOptions),
    academicPaperInputField('page_width_mm', '自定义纸张宽度（mm）', 'number'),
    academicPaperInputField('page_height_mm', '自定义纸张高度（mm）', 'number'),
    academicPaperInputField('page_margin_top_mm', '上边距（mm）', 'number'),
    academicPaperInputField('page_margin_bottom_mm', '下边距（mm）', 'number'),
    academicPaperInputField('page_margin_left_mm', '左边距（mm）', 'number'),
    academicPaperInputField('page_margin_right_mm', '右边距（mm）', 'number'),
    academicPaperInputField('page_gutter_mm', '装订线（mm）', 'number'),

    academicPaperInputField('cover_enabled', '生成封面', 'boolean'),
    academicPaperInputField('cover_school', '学校名称'),
    academicPaperInputField('cover_department', '院系名称'),
    academicPaperInputField('cover_major', '专业名称'),
    academicPaperInputField('cover_author', '作者姓名'),
    academicPaperInputField('cover_student_id', '学号'),
    academicPaperInputField('cover_supervisor', '指导教师'),
    academicPaperInputField('cover_submission_date', '提交日期'),
    academicPaperInputField('cover_requirements', '封面补充要求', 'textarea'),

    ...styleFields('title', '论文标题', { pagination: true }),

    academicPaperInputField('toc_enabled', '生成自动目录', 'boolean'),
    academicPaperInputField('toc_title', '目录标题'),
    academicPaperInputField('toc_levels', '目录显示级数（1–5）', 'number'),
    academicPaperInputField('toc_page_break_before', '目录前分页', 'boolean'),
    academicPaperInputField('toc_page_break_after', '目录后分页', 'boolean'),
    academicPaperInputField('heading_numbering_enabled', '启用标题自动编号', 'boolean'),
    academicPaperSelectField('heading_numbering_style', '标题编号样式', headingNumberingStyleOptions),
    academicPaperInputField('heading1_number_format', '一级标题编号格式（如 第%1章）'),
    academicPaperInputField('heading2_number_format', '二级标题编号格式（如 %1.%2）'),
    academicPaperInputField('heading3_number_format', '三级标题编号格式（如 %1.%2.%3）'),
    academicPaperInputField('heading4_number_format', '四级标题编号格式'),
    academicPaperInputField('heading5_number_format', '五级标题编号格式'),
    academicPaperInputField('heading_numbering_separator', '标题编号与标题间隔符'),

    ...styleFields('heading1', '一级标题', { pagination: true }),
    ...styleFields('heading2', '二级标题', { pagination: true }),
    ...styleFields('heading3', '三级标题', { pagination: true }),
    ...styleFields('heading4', '四级标题', { pagination: true }),
    ...styleFields('heading5', '五级标题', { pagination: true }),
    ...styleFields('body', '正文', { paragraph: true }),
    ...styleFields('abstract', '摘要', { paragraph: true }),
    ...styleFields('keywords', '关键词', { paragraph: true }),

    academicPaperInputField('references_enabled', '生成参考文献', 'boolean'),
    academicPaperInputField('references_title', '参考文献标题'),
    academicPaperInputField('references_hanging_indent_cm', '悬挂缩进（cm）', 'number'),
    ...styleFields('references', '参考文献', { paragraph: true, pagination: true }),

    academicPaperInputField('acknowledgements_enabled', '生成致谢', 'boolean'),
    academicPaperInputField('acknowledgements_title', '致谢标题'),
    academicPaperInputField('acknowledgements_requirements', '致谢内容要求', 'textarea'),
    ...styleFields('acknowledgements', '致谢', { paragraph: true, pagination: true }),

    academicPaperInputField('appendix_enabled', '生成附录', 'boolean'),
    academicPaperInputField('appendix_title', '附录标题'),
    academicPaperInputField('appendix_requirements', '附录内容要求', 'textarea'),
    ...styleFields('appendix', '附录', { paragraph: true, pagination: true }),

    academicPaperInputField('header_enabled', '启用页眉', 'boolean'),
    academicPaperInputField('header_text', '页眉文字'),
    academicPaperSelectField('header_alignment', '页眉对齐方式', alignmentOptions),
    academicPaperSelectField('header_east_asia_font', '页眉中文字体', eastAsiaFontOptions),
    academicPaperSelectField('header_latin_font', '页眉英文字体', latinFontOptions),
    academicPaperSelectField('header_size_pt', '页眉字号', fontSizeOptions),
    academicPaperInputField('header_different_first_page', '首页不同页眉', 'boolean'),
    academicPaperInputField('header_distance_cm', '页眉距边界（cm）', 'number'),

    academicPaperInputField('footer_enabled', '启用页脚', 'boolean'),
    academicPaperInputField('footer_text', '页脚文字'),
    academicPaperSelectField('footer_alignment', '页脚对齐方式', alignmentOptions),
    academicPaperSelectField('footer_east_asia_font', '页脚中文字体', eastAsiaFontOptions),
    academicPaperSelectField('footer_latin_font', '页脚英文字体', latinFontOptions),
    academicPaperSelectField('footer_size_pt', '页脚字号', fontSizeOptions),
    academicPaperInputField('footer_different_first_page', '首页不同页脚', 'boolean'),
    academicPaperInputField('footer_distance_cm', '页脚距边界（cm）', 'number'),

    academicPaperInputField('page_number_enabled', '显示页码', 'boolean'),
    academicPaperSelectField('page_number_position', '页码位置', pageNumberPositionOptions),
    academicPaperSelectField('page_number_alignment', '页码对齐方式', alignmentOptions),
    academicPaperInputField('page_number_start', '起始页码', 'number'),
    academicPaperSelectField('page_number_format', '页码格式', pageNumberFormatOptions),

    academicPaperInputField('pagination_title_page_break_after', '封面后分页', 'boolean'),
    academicPaperInputField('pagination_toc_page_break_after', '目录后分页', 'boolean'),
    academicPaperInputField('pagination_abstract_page_break_after', '摘要后分页', 'boolean'),
    academicPaperInputField('pagination_chapter_page_break_before', '一级章节前分页', 'boolean'),
    academicPaperInputField('pagination_keep_paragraphs_together', '尽量保持段落同页', 'boolean')
  ]
}

function defaultOutputFields(): OutputFieldForm[] {
  return [
    { name: 'result', label: '最终结果', type: 'text', primary: true },
    { name: 'prompt', label: '实际使用的提示词', type: 'text', primary: false }
  ]
}

function defaultVideoOutputFields(): OutputFieldForm[] {
  return [
    { name: 'result', label: '最终结果', type: 'text', primary: true },
    { name: 'artifact', label: '视频文件', type: 'object', primary: false },
    { name: 'generation_mode', label: '生成模式', type: 'text', primary: false }
  ]
}

function defaultProductMarketingOutputFields(): OutputFieldForm[] {
  return [
    { name: 'result', label: '营销方向', type: 'text', primary: true },
    { name: 'marketing_plan', label: '营销方案', type: 'object', primary: false },
    { name: 'image_count', label: '图片数量', type: 'number', primary: false }
  ]
}

function defaultAcademicPaperOutputFields(): OutputFieldForm[] {
  return [
    { name: 'result', label: '论文生成结果', type: 'text', primary: true },
    { name: 'document', label: 'Word 论文文件', type: 'object', primary: false },
    { name: 'word_count', label: '实际字数', type: 'number', primary: false },
    { name: 'quality_report', label: '质量检查报告', type: 'object', primary: false }
  ]
}

function defaultImageModelRoles(): ModelRoleForm[] {
  return [
    { node_id: 'image_generation', role: 'generate', capability: 'image', provider: 'openai', model: 'gpt-image-2', model_group_id: '', required: true }
  ]
}

function defaultGrokVideoModelRoles(): ModelRoleForm[] {
  return [
    { node_id: 'video', role: 'generate', capability: 'video', provider: 'grok', model: 'grok-imagine-video', model_group_id: '', required: true },
    { node_id: 'video', role: 'image_to_video', capability: 'video', provider: 'grok', model: 'grok-imagine-video-1.5', model_group_id: '', required: true }
  ]
}

function defaultProductMarketingModelRoles(): ModelRoleForm[] {
  return [
    { node_id: 'marketing', role: 'analyze', capability: 'vision', provider: 'openai', model: 'gpt-5.5', model_group_id: '', required: true },
    { node_id: 'image', role: 'generate', capability: 'image', provider: 'openai', model: 'gpt-image-2', model_group_id: '', required: true }
  ]
}

function defaultAcademicPaperModelRoles(): ModelRoleForm[] {
  return [
    { node_id: 'academic_paper', role: 'plan', capability: 'text', provider: 'openai', model: 'gpt-5.5', model_group_id: '', required: true },
    { node_id: 'academic_paper', role: 'write', capability: 'text', provider: 'openai', model: 'gpt-5.5', model_group_id: '', required: true }
  ]
}

function emptyCapabilities(): Record<CapabilityKey, boolean> {
  return capabilityMap([])
}

function capabilityMap(enabled: CapabilityKey[]): Record<CapabilityKey, boolean> {
  const map = {} as Record<CapabilityKey, boolean>
  for (const key of capabilityKeys) {
    map[key] = enabled.includes(key)
  }
  return map
}

function defaultArtifactPolicyForm(): ArtifactPolicyForm {
  return {
    retention_days: 0,
    max_file_mb: 100,
    allowed_types: {
      json: true,
      image: true,
      video: false,
      audio: false,
      file: true,
      log: true
    }
  }
}

function defaultGrokVideoArtifactPolicyForm(): ArtifactPolicyForm {
  return {
    retention_days: 0,
    max_file_mb: 512,
    allowed_types: {
      json: true,
      image: true,
      video: true,
      audio: false,
      file: true,
      log: true
    }
  }
}

function defaultAcademicPaperArtifactPolicyForm(): ArtifactPolicyForm {
  return {
    retention_days: 0,
    max_file_mb: 200,
    allowed_types: {
      json: true,
      image: true,
      video: false,
      audio: false,
      file: true,
      log: true
    }
  }
}

function ensureVersionFormDefaults(form: VersionFormLike) {
  if (!Array.isArray(form.input_fields)) {
    form.input_fields = []
  }
  if (!Array.isArray(form.model_roles)) {
    form.model_roles = []
  }
  if (!Array.isArray(form.output_fields) || form.output_fields.length === 0) {
    form.output_fields = defaultOutputFields()
  }
  form.model_roles = form.model_roles.map(role => ({
    ...role,
    provider: normalizeProvider(role.provider),
    model_group_id: normalizeModelGroupIDForProvider(role.model_group_id, normalizeProvider(role.provider)),
    required: role.required !== false
  }))
  form.input_fields = form.input_fields.map(field => ({
    ...field,
    options: serializeSelectOptions(normalizedSelectOptions(field)),
    select_options: normalizedSelectOptions(field)
  }))
  form.capabilities = {
    ...emptyCapabilities(),
    ...(form.capabilities || {})
  }
  const policy = form.artifact_policy || defaultArtifactPolicyForm()
  const retentionDays = Number(policy.retention_days)
  const maxFileMB = Number(policy.max_file_mb)
  form.artifact_policy = {
    retention_days: Number.isFinite(retentionDays) && retentionDays >= 0 ? retentionDays : 0,
    max_file_mb: Number.isFinite(maxFileMB) && maxFileMB > 0 ? maxFileMB : 100,
    allowed_types: {
      ...defaultArtifactPolicyForm().allowed_types,
      ...(policy.allowed_types || {})
    }
  }
  for (const key of artifactTypeKeys) {
    form.artifact_policy.allowed_types[key] = Boolean(form.artifact_policy.allowed_types[key])
  }
}

function addOutputField(form: VersionFormLike) {
  form.output_fields.push({ name: '', label: '', type: 'text', primary: false })
}

function removeOutputField(form: VersionFormLike, index: number) {
  const removedPrimary = form.output_fields[index]?.primary
  form.output_fields.splice(index, 1)
  if (removedPrimary && form.output_fields.length) form.output_fields[0].primary = true
}

function setPrimaryOutputField(form: VersionFormLike, index: number) {
  form.output_fields.forEach((field, currentIndex) => { field.primary = currentIndex === index })
}

function addInputField(form: VersionFormLike) {
  ensureVersionFormDefaults(form)
  form.input_fields.push({
    name: '',
    label: '',
    type: 'text',
    required: false,
    asset_role: '',
    options: '',
    select_options: []
  })
}

function removeInputField(form: VersionFormLike, index: number) {
  ensureVersionFormDefaults(form)
  form.input_fields.splice(index, 1)
}

function addModelRole(form: VersionFormLike) {
  ensureVersionFormDefaults(form)
  form.model_roles.push({
    node_id: 'text',
    role: 'generate',
    capability: 'text',
    provider: 'openai',
    model: '',
    model_group_id: '',
    required: true
  })
}

function removeModelRole(form: VersionFormLike, index: number) {
  ensureVersionFormDefaults(form)
  form.model_roles.splice(index, 1)
}

function normalizeInputFields(fields: InputFieldForm[]): InputFieldForm[] {
  return fields
    .map(field => {
      const selectOptions = normalizedSelectOptions(field)
      return {
        name: field.name.trim(),
        label: field.label.trim(),
        type: field.type,
        required: field.required,
        asset_role: field.asset_role.trim(),
        options: serializeSelectOptions(selectOptions),
        select_options: selectOptions
      }
    })
    .filter(field => field.name !== '')
}

function normalizeModelRoles(roles: ModelRoleForm[]): ModelRoleForm[] {
  return roles
    .map(role => ({
      node_id: role.node_id.trim(),
      role: role.role.trim(),
      capability: role.capability,
      provider: normalizeProvider(role.provider),
      model: role.model.trim(),
      model_group_id: normalizeModelGroupIDForProvider(role.model_group_id, normalizeProvider(role.provider)),
      required: role.required !== false
    }))
    .filter(role => role.node_id !== '' && role.role !== '')
}

function normalizeProvider(value: unknown): GroupPlatform | '' {
  return value === 'openai' || value === 'anthropic' || value === 'gemini' || value === 'antigravity' || value === 'grok'
    ? value
    : ''
}

function normalizeModelGroupID(value: unknown): number | '' {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return value
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const numeric = Number(value)
    if (Number.isFinite(numeric) && numeric > 0) {
      return numeric
    }
  }
  return ''
}

function normalizeModelGroupIDForProvider(value: unknown, provider: GroupPlatform | ''): number | '' {
  const groupID = normalizeModelGroupID(value)
  if (groupID === '' || provider === '') return groupID
  const group = modelGroups.value.find(item => item.id === groupID)
  return group?.platform === provider ? groupID : ''
}

function providerLabel(provider: string): string {
  const labels: Record<string, string> = {
    openai: 'OpenAI',
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    antigravity: 'Antigravity',
    grok: 'Grok'
  }
  return labels[provider] || provider
}

function buildInputSchema(fields: InputFieldForm[]) {
  const required = fields.filter(field => field.required).map(field => field.name)
  const properties: Record<string, Record<string, unknown>> = {}
  for (const field of fields) {
    properties[field.name] = inputFieldToSchema(field)
  }
  return {
    type: 'object',
    required,
    properties
  }
}

function inputFieldToSchema(field: InputFieldForm): Record<string, unknown> {
  if (field.type === 'number') {
    return { type: 'number', title: field.label || field.name }
  }
  if (field.type === 'select') {
    const options = normalizedSelectOptions(field)
    return {
      type: 'string',
      title: field.label || field.name,
      enum: options.map(option => option.value),
      'x-input-kind': 'select',
      'x-options': options
    }
  }
  if (field.type === 'boolean') {
    return { type: 'boolean', title: field.label || field.name, 'x-input-kind': 'boolean' }
  }
  if (field.type === 'date') {
    return { type: 'string', format: 'date', title: field.label || field.name, 'x-input-kind': 'date' }
  }
  if (['image', 'file', 'audio', 'video'].includes(field.type)) {
    const schema: Record<string, unknown> = {
      type: 'array',
      title: field.label || field.name,
      items: { type: 'string' },
      'x-input-kind': field.type
    }
    if (field.asset_role) {
      schema['x-asset-role'] = field.asset_role
    }
    return schema
  }
  return {
    type: 'string',
    title: field.label || field.name,
    'x-input-kind': field.type
  }
}

function parseSelectOptionForms(value: string): SelectOptionForm[] {
  return String(value || '')
    .split(/[\n,，]/)
    .map((item) => {
      const raw = item.trim()
      if (!raw) return null
      const separatorIndex = raw.indexOf('=')
      if (separatorIndex >= 0) {
        const label = raw.slice(0, separatorIndex).trim()
        const optionValue = raw.slice(separatorIndex + 1).trim()
        if (optionValue) return { label: label || optionValue, value: optionValue }
        if (label) return { label, value: label }
        return null
      }
      return { label: raw, value: raw }
    })
    .filter((item): item is SelectOptionForm => item !== null)
}

function normalizedSelectOptions(field: InputFieldForm): SelectOptionForm[] {
  const source = Array.isArray(field.select_options) && field.select_options.length > 0
    ? field.select_options
    : parseSelectOptionForms(field.options)
  const seen = new Set<string>()
  const normalized: SelectOptionForm[] = []
  for (const item of source) {
    const value = String(item.value || item.label || '').trim()
    const label = String(item.label || value).trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    normalized.push({ label: label || value, value })
  }
  return normalized
}

function serializeSelectOptions(options: SelectOptionForm[]): string {
  return options
    .map((option) => {
      const value = String(option.value || option.label || '').trim()
      const label = String(option.label || value).trim()
      if (!value) return ''
      return label && label !== value ? `${label}=${value}` : value
    })
    .filter(Boolean)
    .join('\n')
}

function selectOptionsForField(field: InputFieldForm): SelectOptionForm[] {
  if (!Array.isArray(field.select_options)) {
    field.select_options = parseSelectOptionForms(field.options)
  }
  return field.select_options
}

function syncSelectOptionString(field: InputFieldForm) {
  field.options = serializeSelectOptions(selectOptionsForField(field))
}

function addSelectOption(field: InputFieldForm) {
  const options = selectOptionsForField(field)
  options.push({ label: '', value: '' })
  syncSelectOptionString(field)
}

function removeSelectOption(field: InputFieldForm, index: number) {
  const options = selectOptionsForField(field)
  options.splice(index, 1)
  syncSelectOptionString(field)
}

function buildOutputSchema(form: VersionFormLike) {
  const fields = form.output_fields
    .map(field => ({ ...field, name: field.name.trim(), label: field.label.trim() }))
    .filter(field => field.name)
  const properties: Record<string, Record<string, unknown>> = {}
  for (const field of fields) {
    if (field.type === 'text') properties[field.name] = { type: 'string', title: field.label || field.name }
    else if (field.type === 'number') properties[field.name] = { type: 'number', title: field.label || field.name }
    else if (field.type === 'boolean') properties[field.name] = { type: 'boolean', title: field.label || field.name }
    else if (field.type === 'list') properties[field.name] = { type: 'array', title: field.label || field.name, items: {} }
    else if (field.type === 'table') properties[field.name] = { type: 'array', title: field.label || field.name, items: { type: 'object' }, 'x-display-kind': 'table' }
    else properties[field.name] = { type: 'object', title: field.label || field.name }
  }
  properties.artifacts = { type: 'array', title: '结果产物', items: { type: 'object' } }
  return {
    type: 'object',
    'x-primary-field': fields.find(field => field.primary)?.name || fields[0]?.name || 'result',
    properties,
    'x-app-capabilities': buildCapabilities(form.capabilities)
  }
}

function buildCapabilities(capabilities: Record<CapabilityKey, boolean>) {
  const enabled = Object.entries(capabilities)
    .filter(([, value]) => value)
    .map(([key]) => key)
  return {
    text: capabilities.text,
    image: capabilities.image,
    video: capabilities.video,
    audio: capabilities.audio,
    vision: capabilities.vision,
    file_input_refs: capabilities.file,
    tool: capabilities.tool,
    artifact_upload: true,
    capabilities: enabled
  }
}

function buildDefaultModelConfig(roles: ModelRoleForm[]) {
  const config: Record<string, { model: string; provider?: string; platform?: string; model_group_id?: number }> = {}
  for (const role of roles) {
    if (role.model && !config[role.capability]) {
      config[role.capability] = {
        ...(role.provider ? { provider: role.provider, platform: role.provider } : {}),
        model: role.model,
        ...(typeof role.model_group_id === 'number' ? { model_group_id: role.model_group_id } : {})
      }
    }
  }
  return config
}

function buildNodeModelPolicy(roles: ModelRoleForm[]) {
  const policy: Record<string, Record<string, unknown>> = {}
  for (const role of roles) {
    const key = `${role.node_id}.${role.role}`
    policy[key] = {
      node_id: role.node_id,
      role: role.role,
      capability: role.capability,
      provider: role.provider || undefined,
      platform: role.provider || undefined,
      model: role.model || undefined,
      model_group_id: typeof role.model_group_id === 'number' ? role.model_group_id : undefined,
      required: role.required !== false,
      optional: role.required === false
    }
  }
  return policy
}

function buildArtifactPolicy(policy: ArtifactPolicyForm) {
  const normalizedPolicy = policy || defaultArtifactPolicyForm()
  return {
    store: 'object_storage',
    db_mode: 'reference_only',
    retention_days: normalizedPolicy.retention_days,
    max_file_mb: normalizedPolicy.max_file_mb,
    allowed_types: Object.entries(normalizedPolicy.allowed_types || {})
      .filter(([, enabled]) => enabled)
      .map(([type]) => type)
  }
}

async function openVersions(app: AgentApp) {
  selectedApp.value = app
  versionsLoading.value = true
  showVersionsDialog.value = true
  try {
    await refreshVersionDialog()
  } catch (error: any) {
    toast.error(error?.message || '加载版本失败')
    versions.value = []
  } finally {
    versionsLoading.value = false
  }
}

async function refreshVersionDialog() {
  if (!selectedApp.value) return
  const appId = selectedApp.value.id
  const [app, items] = await Promise.all([
    agentAppsAPI.getById(appId),
    agentAppsAPI.listVersions(appId)
  ])
  selectedApp.value = app
  versions.value = items
}

async function publishAppVersion(version: AgentAppVersion) {
  if (!selectedApp.value || versionActionLoadingId.value === version.id) return
  versionActionLoadingId.value = version.id
  try {
    await agentAppsAPI.publishVersion(selectedApp.value.id, version.id)
    toast.success('当前版本已切换')
    await Promise.all([refreshVersionDialog(), loadApps()])
  } catch (error: any) {
    toast.error(error?.message || '发布版本失败')
  } finally {
    versionActionLoadingId.value = null
  }
}

async function setAppVersionStatus(version: AgentAppVersion, status: string) {
  if (!selectedApp.value || versionActionLoadingId.value === version.id) return
  if (status === 'disabled' && !window.confirm('禁用后用户不能再运行该版本，确认继续吗？')) return
  versionActionLoadingId.value = version.id
  try {
    await agentAppsAPI.updateVersionStatus(selectedApp.value.id, version.id, status)
    toast.success(status === 'disabled' ? '版本已禁用' : '版本已恢复为草稿')
    await Promise.all([refreshVersionDialog(), loadApps()])
  } catch (error: any) {
    toast.error(error?.message || '更新版本状态失败')
  } finally {
    versionActionLoadingId.value = null
  }
}

function appTypeLabel(type: string) {
  return type === 'agent' ? '智能体' : type === 'workflow' ? '工作流' : type === 'prompt' ? '提示词' : '外部 Worker'
}

function appStatusLabel(status: string) {
  return status === 'draft' ? '草稿' : status === 'published' ? '已发布' : status === 'disabled' ? '禁用' : status === 'archived' ? '归档' : status
}

function appStatusBadgeClass(status: string) {
  return status === 'published' ? 'badge-success' : status === 'draft' ? 'badge-gray' : status === 'disabled' ? 'badge-danger' : 'badge-warning'
}

function runtimeTypeLabel(type: string) {
  return type === 'worker' ? 'Worker' : type === 'prompt' ? '提示词' : '内部执行'
}

function versionInputSummary(version: AgentAppVersion): string[] {
  const schema = version.input_schema_json || {}
  const properties = schema.properties && typeof schema.properties === 'object'
    ? schema.properties as Record<string, unknown>
    : {}
  const required = Array.isArray(schema.required)
    ? new Set(schema.required.filter((item): item is string => typeof item === 'string'))
    : new Set<string>()
  const items = Object.entries(properties).map(([name, value]) => {
    const record = value && typeof value === 'object' ? value as Record<string, unknown> : {}
    const label = stringValue(record.title) || name
    const type = inputKindLabel(stringValue(record['x-input-kind']) || stringValue(record.type))
    return `${label}${required.has(name) ? '*' : ''} · ${type}`
  })
  return items.length ? items : ['未定义输入']
}

function versionModelSummary(version: AgentAppVersion): string[] {
  const policy = version.node_model_policy_json || {}
  const items = Object.entries(policy).map(([key, value]) => {
    const record = value && typeof value === 'object' ? value as Record<string, unknown> : {}
    const capability = capabilityDisplayName(stringValue(record.capability) || 'model')
    const role = modelRoleDisplayName(stringValue(record.role))
    const model = stringValue(record.model)
    return [capability, role, model].filter(Boolean).join(' · ') || key
  })
  return items.length ? items : ['未定义模型规则']
}

function versionArtifactSummary(version: AgentAppVersion): string {
  const policy = version.artifact_policy_json || {}
  const retentionDays = Number(policy.retention_days || 0)
  const maxFileMB = Number(policy.max_file_mb || 0)
  const allowedTypes = Array.isArray(policy.allowed_types)
    ? policy.allowed_types.filter((item): item is string => typeof item === 'string')
    : []
  const retention = retentionDays > 0 ? `保留 ${retentionDays} 天` : '长期保留'
  const maxFile = maxFileMB > 0 ? `单文件 ${maxFileMB}MB` : '不限单文件大小'
  const types = allowedTypes.length ? allowedTypes.map(artifactTypeDisplayName).join('、') : '未限制类型'
  return `${retention} · ${maxFile} · ${types}`
}

function versionCapabilitySummary(version: AgentAppVersion): string {
  const capabilities = version.capabilities_json || {}
  const list = Array.isArray(capabilities.capabilities)
    ? capabilities.capabilities.filter((item): item is string => typeof item === 'string')
    : Object.entries(capabilities)
        .filter(([, enabled]) => enabled === true)
        .map(([key]) => key)
  return list.length ? `能力：${list.map(capabilityDisplayName).join('、')}` : '未声明能力'
}

function inputKindLabel(kind: string): string {
  const labels: Record<string, string> = {
    string: '文本',
    text: '文本',
    textarea: '长文本',
    number: '数字',
    integer: '数字',
    image: '图片',
    file: '文件',
    array: '文件'
  }
  return labels[kind] || kind || '输入'
}

function capabilityDisplayName(capability: string): string {
  const labels: Record<string, string> = {
    text: '文本',
    image: '图片',
    video: '视频',
    audio: '音频',
    vision: '视觉',
    file: '文件',
    tool: '工具',
    model: '模型',
    file_input_refs: '文件'
  }
  return labels[capability] || capability
}

function modelRoleDisplayName(role: string): string {
  const labels: Record<string, string> = {
    generate: '生成',
    rewrite: '改写',
    summarize: '总结',
    caption: '说明',
    extract: '提取',
    classify: '分类'
  }
  return labels[role] || role
}

function artifactTypeDisplayName(type: string): string {
  const labels: Record<string, string> = {
    json: '结构化结果',
    image: '图片',
    video: '视频',
    audio: '音频',
    file: '文件',
    log: '日志'
  }
  return labels[type] || type
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function formatJSON(value: Record<string, unknown> | undefined) {
  if (!value || Object.keys(value).length === 0) return '{}'
  return JSON.stringify(value, null, 2)
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(loadApps)
</script>
