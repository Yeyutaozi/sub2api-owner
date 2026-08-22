import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import {
  buildModelMappingObject,
  getModelsByPlatform,
  getPresetMappingsByPlatform,
  getSeedanceModelsByVideoProvider,
  splitModelMappingObject
} from '../useModelWhitelist'
import { getSeedanceVideoProviderBaseUrl } from '@/utils/videoAccountProviders'

describe('useModelWhitelist', () => {
  it('keeps GLM defaults limited to supported text and embedding models', () => {
    const glmModels = getModelsByPlatform('glm')

    expect(glmModels).toContain('glm-5.2')
    expect(glmModels).toContain('embedding-3')
    expect(glmModels).not.toContain('cogview-4')
    expect(glmModels).not.toContain('glm-4v')
    expect(glmModels).not.toContain('cogvideo')

    expect(getModelsByPlatform('zhipu')).toContain('cogvideo')
  })

  it('uses public Seedance model IDs while keeping legacy alias mappings', () => {
    expect(getModelsByPlatform('seedance')).toEqual([
      'seedance-2.0',
      'seedance-2.0-fast',
      'seedance-2.0-mini',
      'seedance-2.5',
      'sd-2.5-ff',
      'sd-2.0-933-art',
      'sd2-mx933',
      'sd2-mx933-fast',
      'sd-2.0-mx933',
      'sd-2.5-mx',
      'sd-2.0-900-720p',
      'seedance-2.5-c1-03',
      'sd2.0-933',
      'sd2-933-25'
    ])
    expect(getModelsByPlatform('seedance')).not.toContain('B-SD2.0-933')
    expect(getModelsByPlatform('seedance')).not.toContain('B-SD2.0-F-933')
    expect(
      getPresetMappingsByPlatform('seedance').map(({ from, to }) => ({ from, to }))
    ).toEqual([
      { from: 'sd2-mx933', to: 'sd2-mx933' },
      { from: 'sd2-mx933-fast', to: 'sd2-mx933-fast' },
      { from: 'sd-2.0-mx933', to: 'sd-2.0-mx933' },
      { from: 'sd-2.5-mx', to: 'sd-2.5-mx' },
      { from: 'sd-2.0-900-720p', to: 'seedance2.0-900-3' },
      { from: 'seedance-2.5-c1-03', to: 'seedance-2.5-c1' },
      { from: 'sd-2.5-ff', to: 'seedance-2-5' },
      { from: 'sd2-933-25', to: '47' },
      { from: 'doubao-seedance-2-0-pro', to: 'seedance-2.0' },
      { from: 'doubao-seedance-2-0-fast', to: 'seedance-2.0-fast' }
    ])
    expect(getModelsByPlatform('ltx')).toEqual(['ltx-2.3-pro', 'ltx-2.3-fast'])
    expect(getModelsByPlatform('happyhorse')).toEqual(['happy-horse-1.1'])
    expect(getSeedanceModelsByVideoProvider('fflink')).toEqual([
      'seedance-2.0',
      'seedance-2.0-fast',
      'seedance-2.0-mini',
      'seedance-2.5',
      'sd-2.5-ff',
      'sd-2.0-933-art'
    ])
    expect(getSeedanceModelsByVideoProvider('huiqu')).toEqual([
      'sd2-mx933',
      'sd2-mx933-fast'
    ])
    expect(getSeedanceModelsByVideoProvider('ximei')).toEqual([
      'sd-2.0-mx933',
      'sd-2.5-mx'
    ])
    expect(getSeedanceModelsByVideoProvider('weijin')).toEqual([
      'sd-2.0-900-720p'
    ])
    expect(getSeedanceModelsByVideoProvider('globalaiopc')).toEqual([
      'seedance-2.5-c1-03'
    ])
    expect(getSeedanceModelsByVideoProvider('openvideo')).toEqual(['sd2.0-933'])
    expect(getSeedanceModelsByVideoProvider('zhi168')).toEqual(['sd2-933-25'])
    expect(getSeedanceModelsByVideoProvider('tianyue')).toEqual([
      'B-SD2.0-933',
      'B-SD2.0-F-933'
    ])
    expect(getSeedanceVideoProviderBaseUrl('fflink')).toBe('https://api.fflink.top')
    expect(getSeedanceVideoProviderBaseUrl('huiqu')).toBe('https://api.bjhuiqu.net')
    expect(getSeedanceVideoProviderBaseUrl('ximei')).toBe(
      'https://liantongyidong.ximeiedu.org'
    )
    expect(getSeedanceVideoProviderBaseUrl('weijin')).toBe('https://www.weijinapi.top')
    expect(getSeedanceVideoProviderBaseUrl('globalaiopc')).toBe(
      'https://zcbservice.aizfw.cn/kyyReactApiServer'
    )
    expect(getSeedanceVideoProviderBaseUrl('openvideo')).toBe(
      'https://www.openvideo.top/api/v1'
    )
    expect(getSeedanceVideoProviderBaseUrl('zhi168')).toBe(
      'https://www.zhi168.it.com/api'
    )
    expect(getSeedanceVideoProviderBaseUrl('tianyue')).toBe(
      'http://192.220.23.225:3000'
    )
  })

  it('always maps the public Weijin 900 ID to its dedicated upstream model', () => {
    expect(buildModelMappingObject('whitelist', ['sd-2.0-900-720p'], [])).toEqual({
      'sd-2.0-900-720p': 'seedance2.0-900-3'
    })
    expect(buildModelMappingObject('mapping', [], [
      { from: 'sd-2.0-900-720p', to: 'sd-2.0-900-720p' }
    ])).toEqual({
      'sd-2.0-900-720p': 'seedance2.0-900-3'
    })
  })

  it('always maps the GlobalAiOpc public model to the upstream C1 model', () => {
    expect(buildModelMappingObject('whitelist', ['seedance-2.5-c1-03'], [])).toEqual({
      'seedance-2.5-c1-03': 'seedance-2.5-c1'
    })
    expect(buildModelMappingObject('mapping', [], [
      { from: 'seedance-2.5-c1-03', to: 'seedance-2.5-c1-03' }
    ])).toEqual({
      'seedance-2.5-c1-03': 'seedance-2.5-c1'
    })
  })

  it('openai 模型列表包含 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('codex-auto-review')
    expect(models).toContain('gpt-5.6')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it('xAI 模型列表包含 Grok 4.5 官方模型和别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-4.5-latest')
    expect(models).toContain('grok-build-latest')
  })

  it('combined 模式支持 Grok 4.5 官方别名映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['grok-4.5'],
      [
        { from: 'grok-latest', to: 'grok-4.5' },
        { from: 'grok-4.5-latest', to: 'grok-4.5' },
        { from: 'grok-build-latest', to: 'grok-4.5' }
      ]
    )

    expect(mapping).toEqual({
      'grok-4.5': 'grok-4.5',
      'grok-latest': 'grok-4.5',
      'grok-4.5-latest': 'grok-4.5',
      'grok-build-latest': 'grok-4.5'
    })
  })

  it('grok 模型列表包含 Composer 默认项和兼容别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-composer-2.5-fast')
    expect(models).toContain('grok-composer')
    expect(models).toContain('composer-2.5')
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('antigravity 模型列表包含 Gemini 3.1 Pro 通用别名', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-pro')
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })
})
