<template>
  <span v-if="isPrimitive" class="whitespace-pre-wrap break-words">{{ primitiveText }}</span>

  <div v-else-if="isTable" class="overflow-x-auto rounded border border-gray-200 dark:border-dark-700">
    <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-700">
      <thead class="bg-gray-50 dark:bg-dark-800">
        <tr>
          <th v-for="column in tableColumns" :key="column" class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">
            {{ humanize(column) }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900/40">
        <tr v-for="(row, index) in value" :key="index">
          <td v-for="column in tableColumns" :key="column" class="max-w-xs px-3 py-2 align-top text-gray-800 dark:text-gray-200">
            {{ compactValue(row?.[column]) }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <ul v-else-if="Array.isArray(value)" class="space-y-2">
    <li v-for="(item, index) in value" :key="index" class="flex gap-2">
      <span class="mt-2 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary-400" />
      <StructuredValue :value="item" :depth="depth + 1" />
    </li>
  </ul>

  <dl v-else-if="isObject" class="grid gap-2 sm:grid-cols-2">
    <div v-for="([key, item]) in objectEntries" :key="key" class="rounded border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/60">
      <dt class="text-xs text-gray-500 dark:text-gray-400">{{ humanize(key) }}</dt>
      <dd class="mt-1 text-sm text-gray-900 dark:text-gray-100">
        <StructuredValue v-if="depth < 3" :value="item" :depth="depth + 1" />
        <span v-else>{{ compactValue(item) }}</span>
      </dd>
    </div>
  </dl>

  <span v-else>-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

defineOptions({ name: 'StructuredValue' })

const props = withDefaults(defineProps<{
  value: unknown
  depth?: number
}>(), {
  depth: 0
})

const isPrimitive = computed(() => props.value == null || ['string', 'number', 'boolean'].includes(typeof props.value))
const primitiveText = computed(() => props.value == null ? '-' : String(props.value))
const isObject = computed(() => Boolean(props.value && typeof props.value === 'object' && !Array.isArray(props.value)))
const objectEntries = computed(() => isObject.value ? Object.entries(props.value as Record<string, unknown>) : [])
const isTable = computed(() => Array.isArray(props.value) && props.value.length > 0 && props.value.every(item => item && typeof item === 'object' && !Array.isArray(item)))
const tableColumns = computed(() => {
  if (!isTable.value) return []
  const columns = new Set<string>()
  for (const row of props.value as Array<Record<string, unknown>>) {
    Object.keys(row).forEach(key => columns.add(key))
  }
  return Array.from(columns).slice(0, 12)
})

function humanize(value: string): string {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, character => character.toUpperCase())
}

function compactValue(value: unknown): string {
  if (value == null) return '-'
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return value.map(compactValue).join('、')
  if (typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>)
      .slice(0, 6)
      .map(([key, item]) => `${humanize(key)}：${compactValue(item)}`)
      .join('；')
  }
  return String(value)
}
</script>
