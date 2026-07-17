import { describe, expect, it } from 'vitest'

import {
  agentAppProviderLabel,
  inferAgentAppProvider,
  normalizeAgentAppProvider
} from '../agentAppModelProvider'

describe('agent app model provider', () => {
  it('preserves an explicit Grok provider', () => {
    expect(normalizeAgentAppProvider('grok')).toBe('grok')
    expect(agentAppProviderLabel('grok')).toBe('Grok')
  })

  it('infers Grok video models instead of leaving the policy unrestricted', () => {
    expect(inferAgentAppProvider('grok-imagine-video')).toBe('grok')
    expect(inferAgentAppProvider('grok-imagine-video-1.5')).toBe('grok')
  })

  it('keeps existing provider inference behavior', () => {
    expect(inferAgentAppProvider('gpt-4.1-mini')).toBe('openai')
    expect(inferAgentAppProvider('claude-sonnet-4')).toBe('anthropic')
    expect(inferAgentAppProvider('gemini-2.5-pro')).toBe('gemini')
  })
})
