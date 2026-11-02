import { reactive, watchSyncEffect } from 'vue'
import type { AccountPlatform, GroupPlatform } from '@/types'
import { listPlatforms } from './platformCatalog'

// type 别名（而非 interface）以便赋给 Select 组件的 Record<string, unknown>[] 选项类型。
export type PlatformOption<T extends string = string> = {
  value: T
  label: string
}

/**
 * Concrete upstream platforms supported by accounts and request routing.
 * Derived from the platform catalog (backend platform list, see platformCatalog.ts),
 * so newly registered providers show up in every selector without frontend
 * changes. Reactive so that tests replacing the catalog see the update.
 */
export const CONCRETE_PLATFORM_OPTIONS: PlatformOption<AccountPlatform>[] = reactive([])

/** Platforms that can own a group. */
export const GROUP_PLATFORM_OPTIONS: PlatformOption<GroupPlatform>[] = reactive([])

const VIDEO_GROUP_PLATFORM_OPTIONS: PlatformOption<GroupPlatform>[] = [
  { value: 'seedance', label: 'Seedance' },
  { value: 'ltx', label: 'LTX' },
  { value: 'happyhorse', label: 'HappyHorse' },
  { value: 'grokimagine', label: 'Grok Imagine' }
]

watchSyncEffect(() => {
  const concrete = listPlatforms().map(spec => ({ value: spec.id, label: spec.display_name }))
  CONCRETE_PLATFORM_OPTIONS.splice(0, CONCRETE_PLATFORM_OPTIONS.length, ...concrete)
  const groupPlatforms = [...concrete]
  for (const option of VIDEO_GROUP_PLATFORM_OPTIONS) {
    if (!groupPlatforms.some(existing => existing.value === option.value)) {
      groupPlatforms.push(option)
    }
  }
  GROUP_PLATFORM_OPTIONS.splice(0, GROUP_PLATFORM_OPTIONS.length, ...groupPlatforms, {
    value: 'composite',
    label: 'Composite'
  })
})
