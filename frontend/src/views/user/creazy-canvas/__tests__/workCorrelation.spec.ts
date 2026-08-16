import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const canvasSource = readFileSync(resolve(here, '../CreazyCanvasView.vue'), 'utf8')
const workflowSource = readFileSync(resolve(here, '../CreazyWorkflowCanvas.vue'), 'utf8')

function functionSource(source: string, startMarker: string, endMarker: string): string {
  const start = source.indexOf(startMarker)
  const end = source.indexOf(endMarker, start)
  expect(start).toBeGreaterThanOrEqual(0)
  expect(end).toBeGreaterThan(start)
  return source.slice(start, end)
}

describe('Creazy Canvas work correlation', () => {
  it('uses a native playback URL for video previews instead of downloading the full blob', () => {
    const preview = functionSource(
      canvasSource,
      'async function performLoadWorkPreview(work: CreazyWork)',
      'async function loadWorkPreview(work: CreazyWork)',
    )

    const videoBranchStart = preview.indexOf('if (!isImageWork(work))')
    const imageBranchStart = preview.indexOf('let url = workStaticMediaUrl(work)', videoBranchStart)
    expect(videoBranchStart).toBeGreaterThanOrEqual(0)
    expect(imageBranchStart).toBeGreaterThan(videoBranchStart)
    const videoBranch = preview.slice(videoBranchStart, imageBranchStart)
    expect(videoBranch).toContain('getWorkPlaybackURL(work.id)')
    expect(videoBranch).not.toContain('getWorkContentBlob(work.id)')
    expect(canvasSource).toContain('preload="metadata"')
    expect(canvasSource).not.toContain('workCoverVideoSrc')

    const resumedVideo = functionSource(
      canvasSource,
      'async function resumeOrphanedVideoWorks()',
      'async function runVideoLifecycle(',
    )
    const completedVideo = functionSource(
      canvasSource,
      'async function runVideoLifecycle(',
      'function taskBoardPageSize()',
    )
    expect(resumedVideo).toContain('getWorkPlaybackURL(workId)')
    expect(completedVideo).toContain('getWorkPlaybackURL(savedId)')
    expect(resumedVideo).not.toContain('getVideoContentURL(')
    expect(completedVideo).not.toContain('getVideoContentURL(')
  })

  it('persists the regular video work before submitting the gateway request', () => {
    const source = functionSource(
      canvasSource,
      'async function generateVideo()',
      'async function resumeOrphanedVideoWorks()',
    )
    const persistIndex = source.indexOf('const running = await persistWork({')
    const gatewayIndex = source.indexOf('gatewayGenerateVideo(apiKey, snapshot.payload, { workId: runningWorkId })')

    expect(persistIndex).toBeGreaterThanOrEqual(0)
    expect(gatewayIndex).toBeGreaterThan(persistIndex)
    expect(source).toContain("if (!running?.id) throw new Error(t('creazyCanvas.errors.saveFailed'))")
  })

  it('passes the persisted work id through every regular and workflow generation path', () => {
    const regularImage = functionSource(
      canvasSource,
      'async function runImageLifecycle(',
      'function extractVideoUrl(',
    )
    const workflowImage = functionSource(
      workflowSource,
      'async function runImageNode(',
      'async function runVideoNode(',
    )
    const workflowVideo = functionSource(
      workflowSource,
      'async function runVideoNode(',
      'function addResultNode(',
    )

    expect(regularImage.match(/workId: runningWorkId \|\| undefined/g)).toHaveLength(2)
    expect(
      workflowImage.match(
        /generateImage\(props\.apiKeySecret, payload as any, \{[\s\S]*?workId,\s*\}\)/g,
      ),
    ).toHaveLength(2)
    expect(workflowVideo).toContain('generateVideo(props.apiKeySecret, payload as any, { workId })')
  })
})
