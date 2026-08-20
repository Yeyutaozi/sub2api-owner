import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  canvasImageUploadDimensions,
  canvasImageUploadFilename,
  optimizeCanvasImageUpload,
} from '../canvasImageUpload'

describe('canvasImageUpload', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('caps the longest edge without upscaling', () => {
    expect(canvasImageUploadDimensions(4032, 3024)).toEqual({ width: 2048, height: 1536 })
    expect(canvasImageUploadDimensions(800, 600)).toEqual({ width: 800, height: 600 })
  })

  it('uses a JPEG filename accepted by direct-file upstreams', () => {
    expect(canvasImageUploadFilename('person.png')).toBe('person.jpg')
    expect(canvasImageUploadFilename('portrait')).toBe('portrait.jpg')
  })

  it('leaves small images unchanged', async () => {
    const file = new File([new Uint8Array(1024)], 'small.png', { type: 'image/png' })
    const result = await optimizeCanvasImageUpload(file, file.name)

    expect(result).toEqual({ body: file, filename: 'small.png', optimized: false })
  })

  it('leaves non-image uploads unchanged', async () => {
    const file = new File([new Uint8Array(2 * 1024 * 1024)], 'clip.mp4', { type: 'video/mp4' })
    const result = await optimizeCanvasImageUpload(file, file.name)

    expect(result).toEqual({ body: file, filename: 'clip.mp4', optimized: false })
  })

  it('compresses a large image before upload', async () => {
    const file = new File([new Uint8Array(2 * 1024 * 1024)], 'person.png', { type: 'image/png' })
    const compressed = new Blob([new Uint8Array(256 * 1024)], { type: 'image/jpeg' })
    const close = vi.fn()
    vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ width: 4032, height: 3024, close }))

    const context = { fillStyle: '', fillRect: vi.fn(), drawImage: vi.fn() }
    const canvas = {
      width: 0,
      height: 0,
      getContext: vi.fn().mockReturnValue(context),
      toBlob: vi.fn((callback: BlobCallback) => callback(compressed)),
    }
    vi.spyOn(document, 'createElement').mockReturnValue(canvas as unknown as HTMLCanvasElement)

    const result = await optimizeCanvasImageUpload(file, file.name)

    expect(result).toEqual({ body: compressed, filename: 'person.jpg', optimized: true })
    expect(canvas.width).toBe(2048)
    expect(canvas.height).toBe(1536)
    expect(context.drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 2048, 1536)
    expect(close).toHaveBeenCalledOnce()
  })
})
