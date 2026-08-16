import { describe, expect, it } from 'vitest'
import {
  buildPlazaModelCards,
  normalizeModelKey,
  paidTokenUnit,
  resolveOfferTokenBase,
  resolvePlazaVideoBillingUnit,
} from '../plazaCatalog'
import type { ModelPlazaResponse } from '@/api/modelPlaza'

function group(
  id: number,
  name: string,
  platform: string,
  rate: number,
  models: Array<{ name: string; platform?: string; kind?: string }>
) {
  return {
    id,
    name,
    platform,
    description: '',
    subscription_type: 'payg',
    rate_multiplier: rate,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    avg_first_token_ms: 120 + id,
    ttft_disclaimer: 'based on recent',
    models: models.map((m) => ({
      name: m.name,
      platform: m.platform || platform,
      kind: m.kind || 'chat',
      pricing: { billing_mode: 'token' as const }
    }))
  }
}

describe('buildPlazaModelCards', () => {
  it('merges same model across groups into one card with multiple offers', () => {
    const response = {
      groups: [
        group(1, 'Alpha', 'openai', 1, [{ name: 'gpt-5.6', kind: 'chat' }]),
        group(2, 'Beta', 'OpenAI', 1.2, [{ name: 'GPT-5.6', kind: 'chat' }])
      ]
    } as unknown as ModelPlazaResponse

    const cards = buildPlazaModelCards(response)
    const gpt = cards.filter((c) => normalizeModelKey(c.name) === 'gpt-5.6')
    expect(gpt).toHaveLength(1)
    expect(gpt[0].offerCount).toBe(2)
    expect(gpt[0].offers.map((o) => o.groupName).sort()).toEqual(['Alpha', 'Beta'])
    expect(gpt[0].minRate).toBe(1)
    expect(gpt[0].maxRate).toBe(1.2)
  })

  it('dedupes same model repeated under one group', () => {
    const response = {
      groups: [
        group(1, 'Alpha', 'openai', 1, [
          { name: 'gpt-5.6' },
          { name: 'gpt-5.6' }
        ])
      ]
    } as unknown as ModelPlazaResponse

    const cards = buildPlazaModelCards(response)
    expect(cards).toHaveLength(1)
    expect(cards[0].offerCount).toBe(1)
  })

  it('keeps different kinds separate', () => {
    const response = {
      groups: [
        group(1, 'Alpha', 'openai', 1, [
          { name: 'dall-e-3', kind: 'image' },
          { name: 'dall-e-3', kind: 'chat' }
        ])
      ]
    } as unknown as ModelPlazaResponse

    const cards = buildPlazaModelCards(response)
    expect(cards).toHaveLength(2)
  })
})

describe('video billing units', () => {
  it('uses the model unit and safely defaults missing catalog data to per second', () => {
    expect(resolvePlazaVideoBillingUnit({ video_billing_unit: 'per_request' })).toBe('per_request')
    expect(resolvePlazaVideoBillingUnit({ video_billing_unit: 'per_second' })).toBe('per_second')
    expect(resolvePlazaVideoBillingUnit({})).toBe('per_second')
  })
})


describe('token price fallback', () => {
  it('uses sibling channel base when offer pricing is null', () => {
    const offer = {
      effectiveRate: 0.1,
      model: {
        name: 'gpt-5.4',
        platform: 'openai',
        kind: 'chat',
        pricing: null,
        official_pricing: null
      }
    }
    const sibling = {
      billing_mode: 'token' as const,
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    }
    expect(resolveOfferTokenBase(offer as any, 'input_price', { siblingPricing: sibling })).toBe(5e-6)
    // 0.1x => $0.5 / $3 per M
    expect(paidTokenUnit(offer as any, 'input_price', { siblingPricing: sibling })).toBeCloseTo(5e-7)
    expect(paidTokenUnit(offer as any, 'output_price', { siblingPricing: sibling })).toBeCloseTo(3e-6)
  })

  it('falls back to official pricing when no channel or sibling pricing', () => {
    const offer = {
      effectiveRate: 0.1,
      model: {
        name: 'gpt-5.4',
        platform: 'openai',
        kind: 'chat',
        pricing: null,
        official_pricing: null
      }
    }
    const official = {
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null
    }
    expect(paidTokenUnit(offer as any, 'input_price', { official })).toBeCloseTo(5e-7)
  })
})
