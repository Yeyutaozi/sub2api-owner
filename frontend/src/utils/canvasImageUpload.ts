const OPTIMIZE_MIN_BYTES = 1024 * 1024
const MAX_IMAGE_DIMENSION = 2048
const JPEG_QUALITY = 0.88

export interface OptimizedCanvasImage {
  body: File | Blob
  filename: string
  optimized: boolean
}

export function canvasImageUploadDimensions(width: number, height: number) {
  const safeWidth = Math.max(1, Math.round(width))
  const safeHeight = Math.max(1, Math.round(height))
  const scale = Math.min(1, MAX_IMAGE_DIMENSION / Math.max(safeWidth, safeHeight))
  return {
    width: Math.max(1, Math.round(safeWidth * scale)),
    height: Math.max(1, Math.round(safeHeight * scale)),
  }
}

export function canvasImageUploadFilename(filename: string) {
  const clean = String(filename || 'image').trim() || 'image'
  const stem = clean.replace(/\.[^.]*$/, '') || 'image'
  return `${stem}.jpg`
}

function isOptimizableImage(file: File | Blob, filename: string) {
  const mime = String(file.type || '').toLowerCase()
  if (['image/jpeg', 'image/png', 'image/webp'].includes(mime)) return true
  return /\.(?:jpe?g|png|webp)$/i.test(filename)
}

function canvasToJPEG(canvas: HTMLCanvasElement) {
  return new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/jpeg', JPEG_QUALITY)
  })
}

export async function optimizeCanvasImageUpload(
  file: File | Blob,
  filename: string,
): Promise<OptimizedCanvasImage> {
  const original = { body: file, filename, optimized: false }
  if (file.size < OPTIMIZE_MIN_BYTES || !isOptimizableImage(file, filename)) return original
  if (typeof document === 'undefined' || typeof createImageBitmap !== 'function') return original

  let bitmap: ImageBitmap | undefined
  try {
    bitmap = await createImageBitmap(file)
    const size = canvasImageUploadDimensions(bitmap.width, bitmap.height)
    const canvas = document.createElement('canvas')
    canvas.width = size.width
    canvas.height = size.height
    const context = canvas.getContext('2d')
    if (!context) return original

    // JPEG is broadly accepted by video/image upstreams. White avoids turning
    // transparent PNG areas black during conversion.
    context.fillStyle = '#fff'
    context.fillRect(0, 0, size.width, size.height)
    context.drawImage(bitmap, 0, 0, size.width, size.height)
    const compressed = await canvasToJPEG(canvas)
    if (!compressed || compressed.size >= file.size * 0.9) return original
    return {
      body: compressed,
      filename: canvasImageUploadFilename(filename),
      optimized: true,
    }
  } catch {
    return original
  } finally {
    bitmap?.close()
  }
}
