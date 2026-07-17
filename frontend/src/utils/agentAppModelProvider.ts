import type { GroupPlatform } from '@/types'

const providers = new Set<GroupPlatform>(['openai', 'anthropic', 'gemini', 'antigravity', 'grok'])

export function normalizeAgentAppProvider(value: unknown): GroupPlatform | '' {
  return typeof value === 'string' && providers.has(value.trim().toLowerCase() as GroupPlatform)
    ? value.trim().toLowerCase() as GroupPlatform
    : ''
}

export function inferAgentAppProvider(model: string): GroupPlatform | '' {
  const normalized = model.trim().toLowerCase()
  if (!normalized) return ''
  if (normalized.startsWith('grok') || normalized.includes('grok-') || normalized.includes('xai')) return 'grok'
  if (normalized.startsWith('gpt-') || normalized.startsWith('o1') || normalized.startsWith('o3') || normalized.startsWith('o4') || normalized.includes('openai')) return 'openai'
  if (normalized.startsWith('claude') || normalized.includes('anthropic')) return 'anthropic'
  if (normalized.startsWith('gemini') || normalized.includes('gemini')) return 'gemini'
  if (normalized.includes('antigravity')) return 'antigravity'
  return ''
}

export function agentAppProviderLabel(platform: string): string {
  const labels: Record<string, string> = {
    openai: 'OpenAI',
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    antigravity: 'Antigravity',
    grok: 'Grok'
  }
  return labels[platform] || platform
}
