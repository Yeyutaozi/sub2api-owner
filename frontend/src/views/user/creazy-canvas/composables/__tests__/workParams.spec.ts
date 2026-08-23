import { describe, expect, it } from 'vitest'
import { isReusableMediaUrl, sanitizeReusableUrlList } from '../workParams'

describe('canvas work media reuse', () => {
  it('rejects expiring public-media URLs in relative and absolute forms', () => {
    expect(isReusableMediaUrl('/v1/videos/public-media/sdpub_expired')).toBe(false)
    expect(isReusableMediaUrl('https://tkcreazy.top/v1/videos/public-media/sdpub_expired?download=1')).toBe(false)
    expect(
      sanitizeReusableUrlList([
        '/v1/videos/public-media/sdpub_expired',
        'https://cdn.example/reference.png',
      ]),
    ).toEqual(['https://cdn.example/reference.png'])
  })

  it('keeps stable signed local-media and regular remote URLs reusable', () => {
    expect(
      isReusableMediaUrl(
        '/api/v1/local-media?key=users%2F7%2Freference.png&expires=1787500000&signature=stable',
      ),
    ).toBe(true)
    expect(isReusableMediaUrl('https://cdn.example/reference.png')).toBe(true)
  })
})
