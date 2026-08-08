import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('AccountTableFilters video platforms', () => {
  it('allows administrators to filter Seedance, LTX, and HappyHorse accounts', () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: { platform: '', type: '', status: '', privacy_mode: '', group: '' },
        groups: [],
      },
      global: {
        stubs: {
          SearchInput: true,
          Select: {
            props: ['options'],
            template: '<div>{{ options.map((option) => option.label).join(",") }}</div>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('Seedance')
    expect(wrapper.text()).toContain('LTX')
    expect(wrapper.text()).toContain('HappyHorse')
    expect(wrapper.text()).toContain('MiniMax')
  })
})
