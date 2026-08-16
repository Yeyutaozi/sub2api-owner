import { describe, expect, it } from 'vitest'
import type { CreazyWork } from '@/api/creazyCanvas'
import {
  canDeleteWork,
  isActiveWorkStatus,
  isExpiredWork,
} from '../workLifecycle'

const NOW = Date.parse('2026-08-15T12:00:00Z')

function work(overrides: Partial<CreazyWork> = {}): CreazyWork {
  return {
    id: 1,
    api_key_id: 1,
    kind: 'image',
    public_model: 'gpt-image-2',
    status: 'running',
    prompt: 'test',
    ...overrides,
  }
}

describe('work lifecycle', () => {
  it('keeps an unexpired active task protected from deletion', () => {
    const item = work({ expires_at: '2026-08-15T13:00:00Z' })

    expect(isActiveWorkStatus(item.status)).toBe(true)
    expect(isExpiredWork(item, NOW)).toBe(false)
    expect(canDeleteWork(item, NOW)).toBe(false)
  })

  it('allows deletion when an active-looking task has expired', () => {
    const item = work({ expires_at: '2026-08-15T11:00:00Z' })

    expect(isActiveWorkStatus(item.status)).toBe(true)
    expect(isExpiredWork(item, NOW)).toBe(true)
    expect(canDeleteWork(item, NOW)).toBe(true)
  })

  it('allows terminal statuses and ignores invalid expiration timestamps', () => {
    expect(canDeleteWork(work({ status: 'expired' }), NOW)).toBe(true)
    expect(canDeleteWork(work({ status: 'succeeded' }), NOW)).toBe(true)
    expect(canDeleteWork(work({ expires_at: 'not-a-date' }), NOW)).toBe(false)
  })
})
