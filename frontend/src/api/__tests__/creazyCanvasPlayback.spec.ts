import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiGet = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { get: apiGet },
  buildApiUrl: (path: string) => `https://app.example.test/api${path}`,
  buildGatewayUrl: (path: string) => `https://app.example.test${path}`,
}))

import { getWorkPlaybackURL } from '@/api/creazyCanvas'

describe('Creazy Canvas playback URL', () => {
  beforeEach(() => {
    apiGet.mockReset()
  })

  it('requests a work-bound playback URL and resolves relative stream paths', async () => {
    apiGet.mockResolvedValue({
      data: {
        work_id: 41,
        source: 'playback',
        url: '/creazy-canvas/works/41/playback?token=abc',
      },
    })

    const result = await getWorkPlaybackURL(41)

    expect(apiGet).toHaveBeenCalledWith('/creazy-canvas/works/41/playback-url')
    expect(result.url).toBe('https://app.example.test/api/creazy-canvas/works/41/playback?token=abc')
    expect(result.source).toBe('playback')
  })
})
