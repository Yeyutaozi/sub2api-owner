import type { CreazyWork } from '@/api/creazyCanvas'

const ACTIVE_WORK_STATUSES = new Set([
  'created',
  'queued',
  'running',
  'pending',
  'processing',
  'settling',
  'in_progress',
  'inprogress',
  'generating',
  'working',
  'submitted',
])

export function normalizeWorkStatus(status?: string): string {
  return String(status || '').trim().toLowerCase()
}

export function isActiveWorkStatus(status?: string): boolean {
  return ACTIVE_WORK_STATUSES.has(normalizeWorkStatus(status))
}

export function isExpiredWork(work: CreazyWork, now = Date.now()): boolean {
  if (normalizeWorkStatus(work.status) === 'expired') return true
  if (!work.expires_at) return false
  const expiresAt = Date.parse(work.expires_at)
  return Number.isFinite(expiresAt) && expiresAt < now
}

export function canDeleteWork(work?: CreazyWork | null, now = Date.now()): boolean {
  if (!work?.id) return false
  return isExpiredWork(work, now) || !isActiveWorkStatus(work.status)
}
