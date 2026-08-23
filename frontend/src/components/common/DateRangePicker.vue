<template>
  <div class="relative" ref="containerRef">
    <button
      type="button"
      ref="triggerRef"
      @click="toggle"
      :class="['date-picker-trigger', isOpen && 'date-picker-trigger-open']"
    >
      <span class="date-picker-icon">
        <Icon name="calendar" size="sm" />
      </span>
      <span class="date-picker-value">
        {{ displayValue }}
      </span>
      <span class="date-picker-chevron">
        <Icon
          name="chevronDown"
          size="sm"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <!-- Teleport to body to escape instrument-panel / desk-panel overflow clipping -->
    <Teleport to="body">
      <Transition name="date-picker-dropdown">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="date-picker-dropdown-portal"
          :style="dropdownStyle"
          @click.stop
          @mousedown.stop
        >
          <!-- Quick presets -->
          <div class="date-picker-presets">
            <button
              v-for="preset in presets"
              :key="preset.value"
              type="button"
              @click="selectPreset(preset)"
              :class="['date-picker-preset', isPresetActive(preset) && 'date-picker-preset-active']"
            >
              {{ t(preset.labelKey) }}
            </button>
          </div>

          <div class="date-picker-divider"></div>

          <!-- Custom date range inputs -->
          <div class="date-picker-custom">
            <div class="date-picker-field">
              <label class="date-picker-label">{{ t('dates.startDate') }}</label>
              <input
                type="date"
                v-model="localStartDate"
                :max="localEndDate || tomorrow"
                class="date-picker-input"
                @change="onDateChange"
              />
            </div>
            <div class="date-picker-separator">
              <Icon name="arrowRight" size="sm" class="text-gray-400" />
            </div>
            <div class="date-picker-field">
              <label class="date-picker-label">{{ t('dates.endDate') }}</label>
              <input
                type="date"
                v-model="localEndDate"
                :min="localStartDate"
                :max="tomorrow"
                class="date-picker-input"
                @change="onDateChange"
              />
            </div>
          </div>

          <!-- Apply button -->
          <div class="date-picker-actions">
            <button type="button" @click="apply" class="date-picker-apply">
              {{ t('dates.apply') }}
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface DatePreset {
  labelKey: string
  value: string
  getRange: () => { start: string; end: string }
}

interface Props {
  startDate: string
  endDate: string
}

interface Emits {
  (e: 'update:startDate', value: string): void
  (e: 'update:endDate', value: string): void
  (e: 'change', range: { startDate: string; endDate: string; preset: string | null }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t, locale } = useI18n()

const isOpen = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<'bottom' | 'top'>('bottom')
const triggerRect = ref<DOMRect | null>(null)
const dropdownViewportPadding = 8
const dropdownMinimumWidth = 320

const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const activePreset = ref<string | null>('last24Hours')

const today = computed(() => {
  // Use local timezone to avoid UTC timezone issues
  const now = new Date()
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
})

// Tomorrow's date - used for max date to handle timezone differences
// When user is in a timezone behind the server, "today" on server might be "tomorrow" locally
const tomorrow = computed(() => {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return formatDateToString(d)
})

// Helper function to format date to YYYY-MM-DD using local timezone
const formatDateToString = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const presets: DatePreset[] = [
  {
    labelKey: 'dates.today',
    value: 'today',
    getRange: () => {
      const t = today.value
      return { start: t, end: t }
    }
  },
  {
    labelKey: 'dates.yesterday',
    value: 'yesterday',
    getRange: () => {
      const d = new Date()
      d.setDate(d.getDate() - 1)
      const yesterday = formatDateToString(d)
      return { start: yesterday, end: yesterday }
    }
  },
  {
    labelKey: 'dates.last24Hours',
    value: 'last24Hours',
    getRange: () => {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
      return {
        start: formatDateToString(start),
        end: formatDateToString(end)
      }
    }
  },
  {
    labelKey: 'dates.last7Days',
    value: '7days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 6)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last14Days',
    value: '14days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 13)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: '30days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 29)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 1))
      return { start, end: today.value }
    }
  },
  {
    labelKey: 'dates.lastMonth',
    value: 'lastMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth() - 1, 1))
      const end = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 0))
      return { start, end }
    }
  }
]

const displayValue = computed(() => {
  if (activePreset.value) {
    const preset = presets.find((p) => p.value === activePreset.value)
    if (preset) return t(preset.labelKey)
  }

  if (localStartDate.value && localEndDate.value) {
    if (localStartDate.value === localEndDate.value) {
      return formatDate(localStartDate.value)
    }
    return `${formatDate(localStartDate.value)} - ${formatDate(localEndDate.value)}`
  }

  return t('dates.selectDateRange')
})

const formatDate = (dateStr: string): string => {
  const date = new Date(dateStr + 'T00:00:00')
  const dateLocale = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  return date.toLocaleDateString(dateLocale, { month: 'short', day: 'numeric' })
}

const isPresetActive = (preset: DatePreset): boolean => {
  return activePreset.value === preset.value
}

const selectPreset = (preset: DatePreset) => {
  const range = preset.getRange()
  localStartDate.value = range.start
  localEndDate.value = range.end
  activePreset.value = preset.value
}

const onDateChange = () => {
  // Check if current dates match any preset
  activePreset.value = null
  for (const preset of presets) {
    const range = preset.getRange()
    if (range.start === localStartDate.value && range.end === localEndDate.value) {
      activePreset.value = preset.value
      break
    }
  }
}

const updateTriggerRect = () => {
  if (triggerRef.value) {
    triggerRect.value = triggerRef.value.getBoundingClientRect()
  } else if (containerRef.value) {
    triggerRect.value = containerRef.value.getBoundingClientRect()
  }
}

const calculateDropdownPosition = () => {
  updateTriggerRect()
  nextTick(() => {
    if (!dropdownRef.value || !triggerRect.value) return
    const dropdownHeight = dropdownRef.value.offsetHeight || 280
    const spaceBelow = window.innerHeight - triggerRect.value.bottom
    const spaceAbove = triggerRect.value.top

    // Prefer below when trigger is in view; only flip above when clearly better
    if (spaceBelow < dropdownHeight + 8 && spaceAbove > spaceBelow && spaceAbove > 120) {
      dropdownPosition.value = 'top'
    } else {
      dropdownPosition.value = 'bottom'
    }
  })
}

const dropdownStyle = computed(() => {
  if (!triggerRect.value) return {}

  const rect = triggerRect.value
  const pad = dropdownViewportPadding
  const viewportRight = Math.max(pad, window.innerWidth - pad)
  const preferredWidth = Math.max(dropdownMinimumWidth, rect.width)
  let left = rect.left
  // Prefer left-align to trigger; flip left if would overflow right edge
  if (left + preferredWidth > viewportRight) {
    left = Math.max(pad, viewportRight - preferredWidth)
  }
  left = Math.min(Math.max(pad, left), viewportRight)
  const availableWidth = Math.max(0, viewportRight - left)
  const minWidth = Math.min(preferredWidth, availableWidth)

  // Keep dropdown fully inside the viewport vertically
  const estimatedHeight = dropdownRef.value?.offsetHeight || 280
  let top: number
  if (dropdownPosition.value === 'top') {
    top = rect.top - estimatedHeight - 4
  } else {
    top = rect.bottom + 4
  }
  const maxTop = Math.max(pad, window.innerHeight - estimatedHeight - pad)
  top = Math.min(Math.max(pad, top), maxTop)

  const style: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    top: `${top}px`,
    minWidth: `${minWidth}px`,
    maxWidth: `${availableWidth}px`,
    zIndex: '100000020'
  }

  return style
})

const onViewportChange = () => {
  if (!isOpen.value) return
  updateTriggerRect()
  calculateDropdownPosition()
}

const toggle = () => {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    // Keep trigger visible so fixed positioning is meaningful
    triggerRef.value?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    updateTriggerRect()
    nextTick(() => {
      calculateDropdownPosition()
      updateTriggerRect()
    })
  }
}

const apply = () => {
  emit('update:startDate', localStartDate.value)
  emit('update:endDate', localEndDate.value)
  emit('change', {
    startDate: localStartDate.value,
    endDate: localEndDate.value,
    preset: activePreset.value
  })
  isOpen.value = false
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (containerRef.value?.contains(target)) return
  if (dropdownRef.value?.contains(target)) return
  isOpen.value = false
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    isOpen.value = false
  }
}

// Sync local state with props
watch(
  () => props.startDate,
  (val) => {
    localStartDate.value = val
    onDateChange()
  }
)

watch(
  () => props.endDate,
  (val) => {
    localEndDate.value = val
    onDateChange()
  }
)

watch(isOpen, (open) => {
  if (open) {
    updateTriggerRect()
    calculateDropdownPosition()
    window.addEventListener('scroll', onViewportChange, { capture: true, passive: true })
    window.addEventListener('resize', onViewportChange)
  } else {
    window.removeEventListener('scroll', onViewportChange, { capture: true })
    window.removeEventListener('resize', onViewportChange)
  }
})

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
  // Initialize active preset detection
  onDateChange()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
  window.removeEventListener('scroll', onViewportChange, { capture: true })
  window.removeEventListener('resize', onViewportChange)
})
</script>

<style scoped>
.date-picker-trigger {
  @apply flex items-center gap-2;
  @apply rounded-lg px-3 py-2 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-700 dark:text-gray-300;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.date-picker-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}

.date-picker-icon {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-value {
  @apply font-medium;
}

.date-picker-chevron {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-dropdown {
  @apply absolute left-0 z-[100] mt-2;
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
  @apply min-w-[320px];
}

.date-picker-presets {
  @apply grid grid-cols-2 gap-1 p-2;
}

.date-picker-preset {
  @apply rounded-md px-3 py-1.5 text-xs font-medium;
  @apply text-gray-600 dark:text-gray-400;
  @apply hover:bg-gray-100 dark:hover:bg-dark-700;
  @apply transition-colors duration-150;
}

.date-picker-preset-active {
  @apply bg-primary-100 dark:bg-primary-900/30;
  @apply text-primary-700 dark:text-primary-300;
}

.date-picker-divider {
  @apply border-t border-gray-100 dark:border-dark-700;
}

.date-picker-custom {
  @apply flex items-end gap-2 p-3;
}

.date-picker-field {
  @apply flex-1;
}

.date-picker-label {
  @apply mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400;
}

.date-picker-input {
  @apply w-full rounded-md px-2 py-1.5 text-sm;
  @apply bg-gray-50 dark:bg-dark-700;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
}

.date-picker-input::-webkit-calendar-picker-indicator {
  @apply cursor-pointer opacity-60 hover:opacity-100;
  filter: invert(0.5);
}

.dark .date-picker-input::-webkit-calendar-picker-indicator {
  filter: none;
}

.date-picker-separator {
  @apply flex items-center justify-center pb-1;
}

.date-picker-actions {
  @apply flex justify-end p-2 pt-0;
}

.date-picker-apply {
  @apply rounded-lg px-4 py-1.5 text-sm font-medium;
  @apply bg-primary-600 text-white;
  @apply hover:bg-primary-700;
  @apply transition-colors duration-150;
}

/* Dropdown animation */
.date-picker-dropdown-enter-active,
.date-picker-dropdown-leave-active {
  transition: all 0.2s ease;
}

.date-picker-dropdown-enter-from,
.date-picker-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>

<!-- Non-scoped styles for teleported dropdown (outside component root) -->
<style>
.date-picker-dropdown-portal {
  background-color: #ffffff;
  border-radius: 0.75rem;
  border: 1px solid #e5e7eb;
  box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1);
  overflow: hidden;
  min-width: 320px;
}

.dark .date-picker-dropdown-portal {
  background-color: #1f2937;
  border-color: #374151;
  box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.3), 0 4px 6px -4px rgb(0 0 0 / 0.3);
}

.date-picker-dropdown-portal .date-picker-presets {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.25rem;
  padding: 0.5rem;
}

.date-picker-dropdown-portal .date-picker-preset {
  border-radius: 0.375rem;
  padding: 0.375rem 0.75rem;
  font-size: 0.75rem;
  font-weight: 500;
  color: #4b5563;
  transition: background-color 0.15s, color 0.15s;
  text-align: left;
  background: transparent;
  border: none;
  cursor: pointer;
}

.dark .date-picker-dropdown-portal .date-picker-preset {
  color: #9ca3af;
}

.date-picker-dropdown-portal .date-picker-preset:hover {
  background-color: #f3f4f6;
}

.dark .date-picker-dropdown-portal .date-picker-preset:hover {
  background-color: #374151;
}

.date-picker-dropdown-portal .date-picker-preset-active {
  background-color: #dbeafe;
  color: #1d4ed8;
}

.dark .date-picker-dropdown-portal .date-picker-preset-active {
  background-color: rgb(30 58 138 / 0.3);
  color: #93c5fd;
}

.date-picker-dropdown-portal .date-picker-divider {
  border-top: 1px solid #f3f4f6;
}

.dark .date-picker-dropdown-portal .date-picker-divider {
  border-top-color: #374151;
}

.date-picker-dropdown-portal .date-picker-custom {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
  padding: 0.75rem;
}

.date-picker-dropdown-portal .date-picker-field {
  flex: 1 1 0%;
}

.date-picker-dropdown-portal .date-picker-label {
  display: block;
  margin-bottom: 0.25rem;
  font-size: 0.75rem;
  font-weight: 500;
  color: #6b7280;
}

.dark .date-picker-dropdown-portal .date-picker-label {
  color: #9ca3af;
}

.date-picker-dropdown-portal .date-picker-input {
  width: 100%;
  border-radius: 0.375rem;
  padding: 0.375rem 0.5rem;
  font-size: 0.875rem;
  background-color: #f9fafb;
  border: 1px solid #e5e7eb;
  color: #111827;
}

.dark .date-picker-dropdown-portal .date-picker-input {
  background-color: #374151;
  border-color: #4b5563;
  color: #f3f4f6;
}

.date-picker-dropdown-portal .date-picker-input:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgb(59 130 246 / 0.3);
}

.date-picker-dropdown-portal .date-picker-input::-webkit-calendar-picker-indicator {
  cursor: pointer;
  opacity: 0.6;
  filter: invert(0.5);
}

.date-picker-dropdown-portal .date-picker-input::-webkit-calendar-picker-indicator:hover {
  opacity: 1;
}

.dark .date-picker-dropdown-portal .date-picker-input::-webkit-calendar-picker-indicator {
  filter: invert(0.7);
}

.date-picker-dropdown-portal .date-picker-separator {
  display: flex;
  align-items: center;
  justify-content: center;
  padding-bottom: 0.25rem;
}

.date-picker-dropdown-portal .date-picker-actions {
  display: flex;
  justify-content: flex-end;
  padding: 0 0.5rem 0.5rem;
}

.date-picker-dropdown-portal .date-picker-apply {
  border-radius: 0.5rem;
  padding: 0.375rem 1rem;
  font-size: 0.875rem;
  font-weight: 500;
  background-color: #2563eb;
  color: #ffffff;
  border: none;
  cursor: pointer;
  transition: background-color 0.15s;
}

.date-picker-dropdown-portal .date-picker-apply:hover {
  background-color: #1d4ed8;
}
</style>
