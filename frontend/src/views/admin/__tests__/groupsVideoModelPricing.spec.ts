import { describe, expect, it } from 'vitest'

import {
  DEFAULT_SEEDANCE_VIDEO_MODELS,
  createVideoModelPricesForm,
  serializeVideoModelPrices,
  supportedResolutionsForVideoModel,
  videoModelPriceFamilyRows
} from '../groupsVideoModelPricing'

describe('Grok video model pricing form', () => {
  it('provides editable rows for both canonical Grok video families', () => {
    const form = createVideoModelPricesForm()

    expect(videoModelPriceFamilyRows(form).map(({ key }) => key)).toEqual([
      'grok-imagine-video',
      'grok-imagine-video-1.5'
    ])
    expect(form['grok-imagine-video']['480p']).toBeNull()
    expect(form['grok-imagine-video-1.5']['1080p']).toBeNull()
  })

  it('serializes only finite non-negative prices and preserves future families', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
    form['grok-imagine-video']['480p'] = 0.05
    form['grok-imagine-video']['720p'] = ''
    form['grok-imagine-video-1.5']['1080p'] = -1

    expect(serializeVideoModelPrices(form)).toEqual({
      'grok-imagine-video': { '480p': 0.05 },
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
  })

  it('round-trips unknown model families so editing does not discard them', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '480p': 0.2 }
    })

    expect(videoModelPriceFamilyRows(form).map(({ key }) => key)).toContain(
      'grok-imagine-video-2'
    )
    expect(serializeVideoModelPrices(form)).toMatchObject({
      'grok-imagine-video-2': { '480p': 0.2 }
    })
  })
})

describe('Seedance video model pricing catalog', () => {
  it('keeps the fixed-resolution FFLink models next to their model families', () => {
    expect(DEFAULT_SEEDANCE_VIDEO_MODELS).toEqual([
      'seedance-2.0',
      'seedance2.0-480p',
      'seedance-2.0-fast',
      'seedance-2.0-mini',
      'seedance2.0-mini-720p',
      'seedance-2.5',
      'sd2-mx933',
      'sd2-mx933-fast',
      'sd-2.0-mx933',
      'sd-2.0-900-720p',
      'seedance-2.5-c1-03',
      'sd-2.5-ff',
      'sd-2.0-933-art',
      'sd2-933-25',
      'sd-2.5-mx',
      'seedance2.0-one-face-reference-480p',
      'seedance2.0-one-face-reference-720p'
    ])
  })

  it('exposes only the resolution supported by each fixed-resolution model', () => {
    expect(supportedResolutionsForVideoModel('seedance', 'seedance2.0-480p')).toEqual(['480p'])
    expect(supportedResolutionsForVideoModel('seedance', 'seedance2.0-mini-720p')).toEqual(['720p'])
  })
})
