import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'

const apiMocks = vi.hoisted(() => ({
  getGroupRateMultipliers: vi.fn(),
  batchSetGroupRateMultipliers: vi.fn(),
  batchSetGroupVideoModelPrices: vi.fn(),
  listUsers: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      getGroupRateMultipliers: apiMocks.getGroupRateMultipliers,
      batchSetGroupRateMultipliers: apiMocks.batchSetGroupRateMultipliers,
      batchSetGroupVideoModelPrices: apiMocks.batchSetGroupVideoModelPrices,
    },
    users: { list: apiMocks.listUsers },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const baseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /></div>',
}

const makeGroup = (
  platform: 'seedance' | 'ltx' | 'happyhorse',
  videoModelPrices: AdminGroup['video_model_prices'],
): AdminGroup => ({
  id: 17,
  name: `${platform} group`,
  platform,
  rate_multiplier: 1,
  video_model_prices: videoModelPrices,
} as AdminGroup)

const mountAndOpen = async (group: AdminGroup) => {
  const wrapper = mount(GroupRateMultipliersModal, {
    props: { show: false, group },
    global: {
      stubs: {
        BaseDialog: baseDialogStub,
        Pagination: true,
        Icon: true,
        PlatformIcon: true,
      },
    },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.batchSetGroupRateMultipliers.mockResolvedValue({ message: 'ok' })
  apiMocks.batchSetGroupVideoModelPrices.mockResolvedValue({ message: 'ok' })
  apiMocks.listUsers.mockResolvedValue({ items: [] })
})

describe('GroupRateMultipliersModal per-user video pricing', () => {
  it('submits an LTX user override by model, resolution, and generated second', async () => {
    apiMocks.getGroupRateMultipliers.mockResolvedValue([
      {
        user_id: 9,
        user_name: 'Test User',
        user_email: 'test@example.com',
        user_notes: '',
        user_status: 'active',
        rate_multiplier: null,
        video_model_prices: { 'ltx-2.3-pro': { '1440p': 0.2 } },
      },
    ])
    const wrapper = await mountAndOpen(makeGroup('ltx', {
      'ltx-2.3-pro': { '1080p': 0.1, '1440p': 0.2, '2160p': 0.3 },
    }))

    const configure = wrapper.findAll('button').find(button =>
      button.text().includes('admin.groups.videoPriceOverrides.configure'),
    )
    expect(configure).toBeTruthy()
    await configure!.trigger('click')

    expect(wrapper.text()).toContain('ltx-2.3-pro')
    const price2160 = wrapper.findAll('label').find(label => label.text().includes('2160p'))
    expect(price2160).toBeTruthy()
    const input = price2160!.find('input')
    await input.setValue('0.45')
    await input.trigger('change')

    const save = wrapper.findAll('button').find(button => button.text() === 'common.save')
    expect(save).toBeTruthy()
    await save!.trigger('click')
    await flushPromises()

    expect(apiMocks.batchSetGroupVideoModelPrices).toHaveBeenCalledWith(17, [
      {
        user_id: 9,
        video_model_prices: {
          'ltx-2.3-pro': { '1440p': 0.2, '2160p': 0.45 },
        },
      },
    ])
  })

  it('shows only HappyHorse-supported resolutions for a dedicated user price', async () => {
    apiMocks.getGroupRateMultipliers.mockResolvedValue([
      {
        user_id: 10,
        user_name: 'Horse User',
        user_email: 'horse@example.com',
        user_notes: '',
        user_status: 'active',
        rate_multiplier: null,
        video_model_prices: { 'legacy-horse-alias': { '720p': 0.12 } },
      },
    ])
    const wrapper = await mountAndOpen(makeGroup('happyhorse', {
      'happy-horse-1.1': { '720p': 0.12, '1080p': 0.18 },
    }))

    const configure = wrapper.findAll('button').find(button =>
      button.text().includes('admin.groups.videoPriceOverrides.configure'),
    )
    await configure!.trigger('click')

    expect(wrapper.text()).toContain('happy-horse-1.1')
    expect(wrapper.text()).toContain('legacy-horse-alias')
    const labels = wrapper.findAll('label').map(label => label.text())
    expect(labels.some(label => label.includes('720p'))).toBe(true)
    expect(labels.some(label => label.includes('1080p'))).toBe(true)
    expect(labels.some(label => label.includes('480p'))).toBe(false)
    expect(labels.some(label => label.includes('1440p'))).toBe(false)
  })

  it('normalizes legacy MX933 group and user prices to public model IDs before saving', async () => {
    apiMocks.getGroupRateMultipliers.mockResolvedValue([
      {
        user_id: 11,
        user_name: 'Seedance User',
        user_email: 'seedance@example.com',
        user_notes: '',
        user_status: 'active',
        rate_multiplier: null,
        video_model_prices: {
          'sd2-mx933-720-1s': { '480p': 0.03, '720p': 0.05 },
          'sd2-mx933': { '480p': 0.04, '720p': 0.06 },
          'sd2-mx933-720-fast-1s': { '720p': 0.02 },
        },
      },
    ])
    const wrapper = await mountAndOpen(makeGroup('seedance', {
      'sd2-mx933-720-1s': { '480p': 0.03, '720p': 0.05 },
      'sd2-mx933-fast': { '480p': 0.04, '720p': 0.06 },
    }))

    const configure = wrapper.findAll('button').find(button =>
      button.text().includes('admin.groups.videoPriceOverrides.configure'),
    )
    await configure!.trigger('click')

    expect(wrapper.text()).toContain('sd2-mx933')
    expect(wrapper.text()).toContain('sd2-mx933-fast')
    expect(wrapper.text()).not.toContain('sd2-mx933-720-1s')
    expect(wrapper.text()).not.toContain('sd2-mx933-720-fast-1s')

    const legacyFastPrice = wrapper.findAll('input').find(input =>
      (input.element as HTMLInputElement).value === '0.02',
    )
    await legacyFastPrice!.setValue('0.025')
    await legacyFastPrice!.trigger('change')

    const save = wrapper.findAll('button').find(button => button.text() === 'common.save')
    await save!.trigger('click')
    await flushPromises()

    expect(apiMocks.batchSetGroupVideoModelPrices).toHaveBeenCalledWith(17, [
      {
        user_id: 11,
        video_model_prices: {
          'sd2-mx933': { '480p': 0.04, '720p': 0.06 },
          'sd2-mx933-fast': { '720p': 0.025 },
        },
      },
    ])
  })

  it('shows Seedance per-request units for per-user price overrides', async () => {
    apiMocks.getGroupRateMultipliers.mockResolvedValue([
      {
        user_id: 12,
        user_name: 'Request User',
        user_email: 'request@example.com',
        user_notes: '',
        user_status: 'active',
        rate_multiplier: null,
        video_model_prices: { 'sd-2.0-mx933': { '480p': 0.5 } },
      },
    ])
    const group = makeGroup('seedance', {
      'sd-2.0-mx933': { '480p': 0.4, '720p': 0.6 },
    })
    group.video_billing_unit = 'per_request'

    const wrapper = await mountAndOpen(group)
    const configure = wrapper.findAll('button').find(button =>
      button.text().includes('admin.groups.videoPriceOverrides.configure'),
    )
    await configure!.trigger('click')

    expect(wrapper.text()).toContain('admin.groups.videoPricing.priceUnitPerRequest')
    expect(wrapper.text()).not.toContain('admin.groups.videoPricing.priceUnitPerSecond')
  })
})
