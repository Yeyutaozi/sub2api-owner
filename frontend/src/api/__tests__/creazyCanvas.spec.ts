// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CREAZY_CANVAS_WORK_ID_HEADER,
  generateImage,
  generateVideo,
} from '@/api/creazyCanvas'

describe('Creazy Canvas image gateway', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends local reference images to edits as official multipart files', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const first = new File(['first'], 'first.png', { type: 'image/png' })
    const second = new File(['second'], 'second.webp', { type: 'image/webp' })
    await generateImage(
      'test-key',
      {
        model: 'gpt-image-2',
        prompt: 'edit the references',
        size: '1024x1024',
        n: 1,
        images: [{ image_url: 'https://should-not-be-forwarded.example/image.png' }],
      },
      { edit: true, imageFiles: [first, second], workId: 17 },
    )

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v1/images/edits')
    expect(init.body).toBeInstanceOf(FormData)
    expect(new Headers(init.headers).has('Content-Type')).toBe(false)
    expect(new Headers(init.headers).get(CREAZY_CANVAS_WORK_ID_HEADER)).toBe('17')

    const form = init.body as FormData
    expect(form.get('model')).toBe('gpt-image-2')
    expect(form.get('prompt')).toBe('edit the references')
    expect(form.get('images')).toBeNull()
    expect(form.getAll('image[]')).toHaveLength(2)
  })

  it('keeps prompt-only generation requests as JSON', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await generateImage(
      'test-key',
      { model: 'gpt-image-2', prompt: 'draw a city' },
      { workId: 42 },
    )

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v1/images/generations')
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json')
    expect(new Headers(init.headers).get(CREAZY_CANVAS_WORK_ID_HEADER)).toBe('42')
    expect(JSON.parse(String(init.body))).toEqual({ model: 'gpt-image-2', prompt: 'draw a city' })
  })

  it('adds the work correlation id to video generation requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: 'job-1', status: 'submitted' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await generateVideo(
      'test-key',
      { model: 'seedance-2.0', prompt: 'animate the city' },
      { workId: 99 },
    )

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v1/videos/generations')
    expect(new Headers(init.headers).get(CREAZY_CANVAS_WORK_ID_HEADER)).toBe('99')
  })

  it.each([0, -1, 1.5, Number.NaN])('does not send an invalid work id (%s)', async (workId) => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await generateImage(
      'test-key',
      { model: 'gpt-image-2', prompt: 'draw a city' },
      { workId },
    )

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(new Headers(init.headers).has(CREAZY_CANVAS_WORK_ID_HEADER)).toBe(false)
  })
})
