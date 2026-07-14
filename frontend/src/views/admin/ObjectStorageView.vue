<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">对象存储</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            管理应用中心的图片、视频、文档等运行产物。保存后立即生效，无需重启服务。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button class="btn btn-secondary" :disabled="loading" @click="loadConfig">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button class="btn btn-secondary" :disabled="submitting || validating" @click="handleValidate">
            <Icon name="checkCircle" size="md" />
            {{ validating ? '测试中...' : '测试连接' }}
          </button>
          <button class="btn btn-primary" :disabled="submitting || !canSave" @click="handleSave">
            <Icon name="check" size="md" />
            保存并切换
          </button>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <form class="space-y-4" @submit.prevent="handleSave">
          <div v-if="form.runtime_error" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300">
            当前配置不可用：{{ runtimeErrorMessage }}
          </div>
          <div v-if="form.enabled && !form.encryption_key_configured" class="rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm text-yellow-800 dark:border-yellow-900/50 dark:bg-yellow-900/20 dark:text-yellow-200">
            必须先在 Sub2API 配置中固定 <code>totp.encryption_key</code> 并重启一次服务，才能安全保存对象存储凭证。
          </div>
          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">基础配置</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">选择云厂商和 bucket，新上传的产物会立即使用当前配置。</p>
              </div>
              <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                启用
              </label>
            </div>

            <div class="mt-5 grid gap-4 md:grid-cols-2">
              <label class="space-y-1.5">
                <span class="form-label">云厂商</span>
                <Select v-model="form.provider" :options="providerOptions" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">Region</span>
                <input v-model.trim="form.region" class="input" placeholder="例如 ap-hongkong / auto" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">Bucket</span>
                <input v-model.trim="form.bucket" class="input" placeholder="产物所在 bucket" />
              </label>
              <label v-if="form.provider === 'r2'" class="space-y-1.5">
                <span class="form-label">Account ID</span>
                <input v-model.trim="form.account_id" class="input" placeholder="Cloudflare Account ID" />
              </label>
              <label class="space-y-1.5 md:col-span-2">
                <span class="form-label">自定义 Endpoint</span>
                <input v-model.trim="form.endpoint" class="input" placeholder="留空则按厂商和 Region 自动生成" />
              </label>
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">访问凭证</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">Secret Access Key 会加密保存。切换厂商、Endpoint 或 Access Key ID 时必须重新填写。</p>
            <div class="mt-5 grid gap-4 md:grid-cols-2">
              <label class="space-y-1.5">
                <span class="form-label">Access Key ID</span>
                <input v-model.trim="form.access_key_id" class="input" autocomplete="off" placeholder="对象存储访问 ID" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">Secret Access Key</span>
                <input v-model="form.secret_access_key" type="password" class="input" autocomplete="new-password" :placeholder="secretPlaceholder" />
              </label>
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">访问方式</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">大多数 S3 兼容服务保持默认即可，MinIO 通常需要路径样式。</p>
            <div class="mt-5 grid gap-4 md:grid-cols-2">
              <label class="space-y-1.5 md:col-span-2">
                <span class="form-label">Public Base URL</span>
                <input v-model.trim="form.public_base_url" class="input" placeholder="可选，自定义公开访问域名" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">对象前缀</span>
                <input v-model.trim="form.prefix" class="input" placeholder="例如 agent-artifacts" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">下载链接有效期（秒）</span>
                <input v-model.number="form.download_url_ttl_seconds" type="number" min="60" class="input" />
              </label>
              <details class="md:col-span-2 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-700">
                <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">高级兼容设置</summary>
                <div class="mt-3 grid gap-3 sm:grid-cols-3">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                    <input v-model="form.force_path_style" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                    强制 Path Style
                  </label>
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                    <input v-model="form.virtual_host_style" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                    Virtual Host Style
                  </label>
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                    <input v-model="form.disable_checksum" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                    禁用 Checksum
                  </label>
                </div>
              </details>
            </div>
          </section>

          <div class="rounded-lg border border-blue-100 bg-blue-50 px-4 py-3 text-sm leading-6 text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/20 dark:text-blue-200">
            “测试连接”会在目标 Bucket 中上传、生成下载签名并删除一个小型测试文件。切换配置不会迁移旧文件，历史产物仍从原厂商读取。
          </div>

          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">产物策略</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">这里是全局默认值；应用版本单独设置的保留时间和文件上限优先。运行记录不会随对象文件清理。</p>
            <div class="mt-5 grid gap-4 md:grid-cols-3">
              <label class="space-y-1.5">
                <span class="form-label">单文件上限（MB）</span>
                <input v-model.number="maxUploadMB" type="number" min="1" class="input" />
              </label>
              <label class="space-y-1.5">
                <span class="form-label">产物保留天数</span>
                <input v-model.number="form.retention_days" type="number" min="0" class="input" />
              </label>
              <label class="mt-7 inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="form.cleanup_expired_artifacts_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                自动清理过期对象
              </label>
            </div>
          </section>
        </form>

        <aside class="space-y-4">
          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">当前状态</h2>
            <dl class="mt-4 space-y-3 text-sm">
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">启用状态</dt>
                <dd :class="['badge', form.enabled ? 'badge-success' : 'badge-gray']">{{ form.enabled ? '已启用' : '未启用' }}</dd>
              </div>
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">平台加密密钥</dt>
                <dd :class="['badge', form.encryption_key_configured ? 'badge-success' : 'badge-danger']">
                  {{ form.encryption_key_configured ? '已固定' : '未固定' }}
                </dd>
              </div>
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">配置来源</dt>
                <dd class="text-gray-900 dark:text-white">{{ sourceLabel }}</dd>
              </div>
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">密钥状态</dt>
                <dd :class="['badge', form.secret_access_key_configured && !secretNeedsReentry ? 'badge-success' : secretNeedsReentry ? 'badge-danger' : 'badge-warning']">
                  {{ secretNeedsReentry ? '需重新填写' : form.secret_access_key_configured ? '已保存' : '未保存' }}
                </dd>
              </div>
            </dl>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">解析结果</h2>
            <div class="mt-4 space-y-3 text-sm text-gray-600 dark:text-gray-300">
              <div>
                <div class="text-xs text-gray-500 dark:text-gray-400">Endpoint</div>
                <code class="mt-1 block break-all rounded bg-gray-50 px-2 py-1 text-xs dark:bg-dark-900">{{ form.resolved_endpoint || form.endpoint || '-' }}</code>
              </div>
              <div>
                <div class="text-xs text-gray-500 dark:text-gray-400">对象位置</div>
                <code class="mt-1 block break-all rounded bg-gray-50 px-2 py-1 text-xs dark:bg-dark-900">{{ objectLocationPreview }}</code>
              </div>
            </div>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import objectStorageAPI, { type AgentArtifactStorageConfig } from '@/api/admin/objectStorage'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const loading = ref(false)
const submitting = ref(false)
const validating = ref(false)

const form = reactive<AgentArtifactStorageConfig>({
  enabled: false,
  provider: 'cos',
  account_id: '',
  endpoint: '',
  region: 'ap-hongkong',
  bucket: '',
  access_key_id: '',
  secret_access_key: '',
  secret_access_key_configured: false,
  prefix: 'agent-artifacts',
  public_base_url: '',
  force_path_style: false,
  virtual_host_style: false,
  disable_checksum: false,
  max_upload_bytes: 512 * 1024 * 1024,
  download_url_ttl_seconds: 3600,
  retention_days: 30,
  cleanup_expired_artifacts_enabled: true,
  encryption_key_configured: false,
  runtime_error: '',
  source: '',
  resolved_endpoint: ''
})

const providerOptions = [
  { label: '腾讯云 COS', value: 'cos' },
  { label: '阿里云 OSS', value: 'oss' },
  { label: 'Cloudflare R2', value: 'r2' },
  { label: 'AWS S3 / S3 兼容', value: 's3' },
  { label: '华为云 OBS', value: 'obs' },
  { label: '火山引擎 TOS', value: 'tos' },
  { label: 'Wasabi', value: 'wasabi' },
  { label: 'Backblaze B2', value: 'b2' },
  { label: 'DigitalOcean Spaces', value: 'spaces' },
  { label: '百度云 BOS', value: 'bos' },
  { label: 'MinIO', value: 'minio' },
  { label: '自定义 S3 兼容', value: 'custom' }
]

const maxUploadMB = computed({
  get: () => Math.max(1, Math.round((form.max_upload_bytes || 0) / 1024 / 1024)),
  set: (value: number) => {
    form.max_upload_bytes = Math.max(1, Number(value) || 1) * 1024 * 1024
  }
})

const savedCredentialIdentity = ref('')
const credentialIdentity = computed(() => [form.provider, form.account_id, form.endpoint, form.access_key_id].map(value => String(value || '').trim()).join('|'))
const credentialIdentityChanged = computed(() => Boolean(savedCredentialIdentity.value && savedCredentialIdentity.value !== credentialIdentity.value))
const secretNeedsReentry = computed(() => String(form.runtime_error || '').includes('AGENT_ARTIFACT_STORAGE_SECRET_DECRYPT_FAILED'))
const runtimeErrorMessage = computed(() => secretNeedsReentry.value
  ? '旧凭据无法使用当前加密密钥解密，请重新填写 Secret Access Key，测试通过后保存。'
  : form.runtime_error)
const secretPlaceholder = computed(() => credentialIdentityChanged.value || secretNeedsReentry.value
  ? '配置已切换，必须重新填写'
  : form.secret_access_key_configured ? '不填则保留原密钥' : '首次启用必须填写')
const canSave = computed(() => {
  const hasEnteredSecret = Boolean(form.secret_access_key)
  if (secretNeedsReentry.value && !hasEnteredSecret) return false
  return !form.enabled || (
    Boolean(form.encryption_key_configured) &&
    ((Boolean(form.secret_access_key_configured) && !secretNeedsReentry.value) || hasEnteredSecret) &&
    (!credentialIdentityChanged.value || hasEnteredSecret)
  )
})
const sourceLabel = computed(() => ({ database: '后台配置', config: '配置文件', default: '默认值', preview: '预览' }[form.source || ''] || '-') )
const objectLocationPreview = computed(() => {
  const prefix = form.prefix ? `${form.prefix.replace(/^\/+|\/+$/g, '')}/` : ''
  return `${form.provider || '-'}://${form.bucket || '-'}/${prefix}runs/...`
})

function applyConfig(config: AgentArtifactStorageConfig) {
  Object.assign(form, {
    enabled: Boolean(config.enabled),
    provider: config.provider || 'cos',
    account_id: config.account_id || '',
    endpoint: config.endpoint || '',
    region: config.region || 'ap-hongkong',
    bucket: config.bucket || '',
    access_key_id: config.access_key_id || '',
    secret_access_key: '',
    secret_access_key_configured: Boolean(config.secret_access_key_configured),
    prefix: config.prefix || 'agent-artifacts',
    public_base_url: config.public_base_url || '',
    force_path_style: Boolean(config.force_path_style),
    virtual_host_style: Boolean(config.virtual_host_style),
    disable_checksum: Boolean(config.disable_checksum),
    max_upload_bytes: config.max_upload_bytes || 512 * 1024 * 1024,
    download_url_ttl_seconds: config.download_url_ttl_seconds || 3600,
    retention_days: config.retention_days ?? 30,
    cleanup_expired_artifacts_enabled: Boolean(config.cleanup_expired_artifacts_enabled),
    encryption_key_configured: Boolean(config.encryption_key_configured),
    runtime_error: config.runtime_error || '',
    source: config.source || '',
    resolved_endpoint: config.resolved_endpoint || ''
  })
  savedCredentialIdentity.value = credentialIdentity.value
}

async function loadConfig() {
  loading.value = true
  try {
    applyConfig(await objectStorageAPI.getConfig())
  } catch (error: any) {
    appStore.showError(error?.message || '加载对象存储配置失败')
  } finally {
    loading.value = false
  }
}

function payload(): AgentArtifactStorageConfig {
  return { ...form }
}

function connectionValidationError(): string | null {
  if (!String(form.bucket || '').trim()) return '请先填写 Bucket'
  if (!String(form.access_key_id || '').trim()) return '请先填写 Access Key ID'

  const publicBaseURL = String(form.public_base_url || '').trim()
  if (publicBaseURL && !/^https?:\/\//i.test(publicBaseURL)) {
    return 'Public Base URL 必须是完整的 http:// 或 https:// 地址；私有 COS 请留空'
  }

  const canReuseSavedSecret = Boolean(form.secret_access_key_configured) && !credentialIdentityChanged.value && !secretNeedsReentry.value
  if (!String(form.secret_access_key || '').trim() && !canReuseSavedSecret) {
    return '请先填写 Secret Access Key'
  }

  const provider = String(form.provider || '').trim()
  if (provider === 'r2' && !String(form.account_id || '').trim() && !String(form.endpoint || '').trim()) {
    return 'Cloudflare R2 需要填写 Account ID 或自定义 Endpoint'
  }
  if ((provider === 'minio' || provider === 'custom') && !String(form.endpoint || '').trim()) {
    return '当前存储类型需要填写 Endpoint'
  }
  return null
}

async function handleValidate() {
  const validationError = connectionValidationError()
  if (validationError) {
    appStore.showError(validationError)
    return
  }

  validating.value = true
  try {
    const result = await objectStorageAPI.validateConfig({ ...payload(), enabled: true })
    if (!result.valid) throw new Error('对象存储未通过连接测试')
    appStore.showSuccess('连接测试通过，上传、签名下载和删除均正常')
  } catch (error: any) {
    appStore.showError(error?.message || '配置校验失败')
  } finally {
    validating.value = false
  }
}

async function handleSave() {
  submitting.value = true
  try {
    const config = await objectStorageAPI.updateConfig(payload())
    applyConfig(config)
    appStore.showSuccess('对象存储配置已保存，新的应用产物会立即使用当前配置')
  } catch (error: any) {
    appStore.showError(error?.message || '保存对象存储配置失败')
  } finally {
    submitting.value = false
  }
}

onMounted(loadConfig)
</script>
