<template>
  <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
    <div class="flex items-start justify-between gap-3">
      <div>
        <div class="text-xs font-medium text-primary-600 dark:text-primary-400">用户侧预览</div>
        <h3 class="mt-1 text-base font-semibold text-gray-950 dark:text-white">{{ name || '未命名应用' }}</h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ description || '暂无应用说明' }}</p>
      </div>
      <span class="badge badge-success">已发布后可见</span>
    </div>

    <div class="mt-4 grid gap-4 lg:grid-cols-2">
      <section>
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">运行输入</h4>
        <div v-if="inputFields.length" class="mt-2 space-y-2">
          <div v-for="field in inputFields" :key="field.name" class="rounded border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-2 text-sm">
              <span class="font-medium text-gray-800 dark:text-gray-100">{{ field.label || field.name }}</span>
              <span class="text-xs text-gray-500">{{ field.required ? '必填' : '可选' }} · {{ inputTypeLabel(field.type) }}</span>
            </div>
          </div>
        </div>
        <p v-else class="mt-2 text-sm text-gray-500">未配置输入项</p>
      </section>

      <section>
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">模型授权</h4>
        <div class="mt-2 rounded border border-gray-200 bg-white px-3 py-3 text-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-gray-700 dark:text-gray-200">需要 {{ requiredModelCount }} 项必需模型授权，{{ optionalModelCount }} 项按需授权。</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">系统会优先选择符合厂商和分组要求的低倍率 Key，用户可以手动更改。</p>
        </div>
      </section>
    </div>

    <section class="mt-4">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">最终结果</h4>
      <div class="mt-2 flex flex-wrap gap-2">
        <span v-for="field in outputFields" :key="field.name" :class="['badge', field.primary ? 'badge-primary' : 'badge-gray']">
          {{ field.label || field.name }} · {{ outputTypeLabel(field.type) }}{{ field.primary ? ' · 主要结果' : '' }}
        </span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  name?: string
  description?: string
  inputFields: Array<{ name: string; label: string; type: string; required: boolean }>
  modelRoles: Array<{ required: boolean }>
  outputFields: Array<{ name: string; label: string; type: string; primary: boolean }>
}>()

const requiredModelCount = computed(() => props.modelRoles.filter(role => role.required !== false).length)
const optionalModelCount = computed(() => props.modelRoles.filter(role => role.required === false).length)

function inputTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    text: '单行文本', textarea: '多行文本', select: '选择项', image: '图片', file: '文件',
    audio: '音频', video: '视频', number: '数字', boolean: '开关', date: '日期'
  }
  return labels[type] || type
}

function outputTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    text: '文本', number: '数字', boolean: '是/否', list: '列表', table: '表格', object: '结构化信息'
  }
  return labels[type] || type
}
</script>
