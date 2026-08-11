import { describe, expect, it } from 'vitest'
import { buildPlazaModelCards, normalizeModelKey } from '../plazaCatalog'
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
