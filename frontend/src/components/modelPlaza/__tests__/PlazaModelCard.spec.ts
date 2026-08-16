import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { VideoBillingUnit } from '@/types'
import type {
  ModelPlazaGroup,
  ModelPlazaResponse,
  PlazaModel,
} from '@/api/modelPlaza'
import PlazaModelCard from '../PlazaModelCard.vue'
import { buildPlazaModelCards } from '../plazaCatalog'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function videoGroup(
  id: number,
  rate: number,
  billingUnit?: VideoBillingUnit,
): ModelPlazaGroup {
  const model: PlazaModel = {
    name: 'video-model',
    platform: 'seedance',
    kind: 'video',
    pricing: null,
    official_pricing: null,
    video_billing_unit: billingUnit,
    video_resolutions: ['720p'],
    video_prices: { '720p': 0.08 },
  }
  return {
    id,
    name: `Group ${id}`,
    description: '',
    platform: 'seedance',
    subscription_type: 'standard',
    rate_multiplier: rate,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    avg_first_token_ms: 100,
    ttft_disclaimer: '',
    models: [model],
  }
}

function cardFor(groups: ModelPlazaGroup[]) {
  const response: ModelPlazaResponse = {
    description: '',
    groups,
  }
  const [card] = buildPlazaModelCards(response)
  return card
}

describe('PlazaModelCard video billing unit', () => {
  it('shows each offer unit independently and does not rank incomparable mixed units', () => {
    const wrapper = mount(PlazaModelCard, {
      props: {
        card: cardFor([
          videoGroup(1, 0.8, 'per_second'),
          videoGroup(2, 1, 'per_request'),
        ]),
      },
      global: { stubs: { PlazaVendorMark: true } },
    })

    expect(
      wrapper
        .findAll('[data-testid="plaza-offer-video-billing-unit"]')
        .map((node) => node.text()),
    ).toEqual([
      'modelPlaza.table.perSecond',
      'modelPlaza.table.perRequest',
    ])
    expect(wrapper.find('.mp-pill--best').exists()).toBe(false)
  })

  it('defaults a missing effective unit to per-second display', () => {
    const wrapper = mount(PlazaModelCard, {
      props: { card: cardFor([videoGroup(1, 1)]) },
      global: { stubs: { PlazaVendorMark: true } },
    })

    expect(wrapper.text()).toContain('modelPlaza.table.videoBillingPerSecondHint')
    expect(wrapper.text()).not.toContain('modelPlaza.table.videoBillingPerClipHint')
  })
})
