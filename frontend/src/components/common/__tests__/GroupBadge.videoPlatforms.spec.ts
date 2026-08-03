import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupBadge video platforms', () => {
  it.each([
    ['ltx', 'bg-cyan-50'],
    ['happyhorse', 'bg-amber-50'],
  ] as const)('uses a dedicated %s group style', (platform, expectedClass) => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: `${platform} group`,
        platform,
        rateMultiplier: 1,
      },
    })

    expect(wrapper.classes()).toContain(expectedClass)
  })
})
