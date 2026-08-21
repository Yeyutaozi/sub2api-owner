// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CREAZY_CANVAS_WORK_ID_HEADER,
  generateImage,
  getImageTask,
  resolveOpenAIImageSize,
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
      { async: true, edit: true, imageFiles: [first, second], workId: 17 },
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
    expect(form.getAll('image')).toHaveLength(2)
    expect(form.get('image[]')).toBeNull()
    expect(form.get('async')).toBe('true')
    expect(form.get('quality')).toBeNull()
    expect(form.get('response_format')).toBe('url')
    expect(form.get('output_format')).toBe('jpeg')
    const headers = new Headers(init.headers)
    expect(headers.get('X-Request-ID')).toBeTruthy()
    expect(headers.get('Idempotency-Key')).toBe(headers.get('X-Request-ID'))
  })

  it('uses the plugin async JSON contract on the standard generations endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ task_id: 'imgtask_1', query_path: '/v1/image/tasks/imgtask_1' }), {
        status: 202,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await generateImage(
      'test-key',
      { model: 'gpt-image-2', prompt: 'draw a city', size: '3840x2160', quality: 'high' },
      { async: true },
    )

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/v1/images/generations')
    expect(url).not.toContain('/async')
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-image-2',
      prompt: 'draw a city',
      size: '3840x2160',
      n: 1,
      response_format: 'url',
      output_format: 'jpeg',
      async: true,
    })
  })

  it('normalizes URL edit references to the plugin image field', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ task_id: 'imgtask_2' }), { status: 202 }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await generateImage(
      'test-key',
      {
        model: 'gpt-image-2',
        prompt: 'edit',
        images: [{ image_url: 'https://example.test/a.png' }, { image_url: 'https://example.test/b.png' }],
      },
      { async: true, edit: true },
    )

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    const body = JSON.parse(String(init.body))
    expect(body.image).toEqual(['https://example.test/a.png', 'https://example.test/b.png'])
    expect(body.images).toBeUndefined()
    expect(body.reference_images).toBeUndefined()
  })

  it('polls the plugin query_path and accepts the nested result response', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ status: 'completed', result: { data: [{ url: 'https://example.test/out.jpg' }] } }), {
        status: 200,
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await getImageTask('test-key', 'imgtask_3', '/v1/image/tasks/imgtask_3')

    expect(String(fetchMock.mock.calls[0][0])).toContain('/v1/image/tasks/imgtask_3')
    expect(result.result).toEqual({ data: [{ url: 'https://example.test/out.jpg' }] })
  })

  it('uses the plugin 4K size table exactly', () => {
    expect(resolveOpenAIImageSize('4K', '1:1')).toBe('2880x2880')
    expect(resolveOpenAIImageSize('4K', '4:3')).toBe('3312x2480')
    expect(resolveOpenAIImageSize('4K', '16:9')).toBe('3840x2160')
    expect(resolveOpenAIImageSize('4K', '5:4')).toBeUndefined()
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
