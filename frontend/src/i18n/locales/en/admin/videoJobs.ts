export default {
  videoJobs: {
    title: 'Video Jobs',
    description: 'Inspect prompts, reference materials, results, and user/group for all video generation jobs. Search by job ID, sync upstream status, and force-kill stuck tasks.',
    searchPlaceholder: 'Search job ID / upstream ID / user / model',
    jobIdPlaceholder: 'Exact job ID',
    modelPlaceholder: 'Model name',
    filters: {
      allStatus: 'All statuses',
      unsettledOnly: 'Unsettled only',
      status: 'Status',
      model: 'Model'
    },
    columns: {
      jobId: 'Job ID',
      userGroup: 'User / Group',
      model: 'Model',
      status: 'Status',
      refund: 'Refund',
      prompt: 'Prompt',
      createdAt: 'Created',
      actions: 'Actions'
    },
    status: {
      queued: 'Queued',
      running: 'Generating',
      succeeded: 'Succeeded',
      failed: 'Failed',
      cancelled: 'Cancelled',
      unknown: 'Unknown'
    },
    refund: {
      pending: 'Pending',
      applied: 'Refunded',
      error: 'Refund error',
      not_required: 'Not required',
      empty: '-'
    },
    actions: {
      refresh: 'Refresh',
      detail: 'Details',
      sync: 'Sync upstream',
      kill: 'Kill task',
      openResult: 'Open result'
    },
    empty: {
      title: 'No video jobs',
      description: 'No video generation jobs match the current filters'
    },
    detail: {
      title: 'Job details',
      basic: 'Basics',
      prompt: 'Prompt',
      materials: 'Reference materials',
      result: 'Result',
      settlement: 'Settlement',
      noPrompt: 'No prompt',
      noMaterials: 'No reference materials',
      noResult: 'No result yet (job not succeeded)',
      resultPath: 'Result path',
      lastError: 'Last error',
      snapshotJson: 'Request snapshot',
      fields: {
        jobId: 'Job ID',
        upstreamJobId: 'Upstream job ID',
        user: 'User',
        group: 'Group',
        apiKey: 'API Key',
        accountId: 'Account ID',
        model: 'Model',
        fallbackModel: 'Fallback model',
        fallbackStatus: 'Fallback status',
        taskStatus: 'Task status',
        refundStatus: 'Refund status',
        settlementAttempts: 'Settlement attempts',
        refundAttempts: 'Refund attempts',
        resolution: 'Resolution',
        duration: 'Duration (s)',
        aspectRatio: 'Aspect ratio',
        createdAt: 'Created at',
        updatedAt: 'Updated at',
        settledAt: 'Settled at',
        refundedAt: 'Refunded at',
        lastPolledAt: 'Last polled',
        nextPollAt: 'Next poll'
      }
    },
    material: {
      startFrame: 'Start frame',
      endFrame: 'End frame',
      image: 'Image refs',
      video: 'Video refs',
      audio: 'Audio refs'
    },
    messages: {
      loadFailed: 'Failed to load video jobs',
      syncSuccess: 'Synced upstream status',
      syncFailed: 'Sync failed',
      killSuccess: 'Task killed; refund settlement attempted',
      killFailed: 'Failed to kill task',
      killConfirmTitle: 'Kill this task?',
      killConfirmMessage: 'Best-effort cancel upstream, mark local status as cancelled, and trigger refund settlement. Already settled jobs will not be refunded again.',
      copyOk: 'Copied',
      copyFail: 'Copy failed'
    }
  }
}
