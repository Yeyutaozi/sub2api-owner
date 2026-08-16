export default {
  videoJobs: {
    title: 'Video Jobs',
    description: 'Inspect prompts, reference materials, results, and user/group for all video generation jobs. Search by platform task ID or user, sync upstream status, force-fail or kill stuck tasks.',
    searchPlaceholder: 'Platform task ID / user email / username / user ID / model',
    jobIdPlaceholder: 'Exact platform task ID',
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
      createdAt: 'Time / duration',
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
    timing: {
      running: 'Running',
      elapsed: 'Elapsed'
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
      kill: 'Kill (cancel)',
      forceFail: 'Force fail',
      batchSync: 'Batch sync',
      batchKill: 'Batch kill',
      batchForceFail: 'Batch force fail',
      clearSelection: 'Clear selection',
      openResult: 'Open result'
    },

    batch: {
      selectedCount: '{count} selected',
      unsettledCount: '{count} unsettled'
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
        executionDuration: 'Execution time',
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
      syncStillPending: 'Queried upstream; task is still not terminal',
      syncFailed: 'Sync failed',
      killSuccess: 'Task killed; refund settlement attempted',
      killFailed: 'Failed to kill task',
      killConfirmTitle: 'Kill this task?',
      killConfirmMessage: 'Best-effort cancel upstream, mark local status as cancelled, and trigger refund settlement. Already settled jobs will not be refunded again.',
      forceFailSuccess: 'Task marked failed; refund settlement attempted',
      forceFailFailed: 'Failed to force-fail task',
      forceFailConfirmTitle: 'Force fail this task?',
      forceFailConfirmMessage: 'Best-effort cancel upstream, mark local status as failed, and trigger refund settlement. Already settled jobs will not be refunded again.',

      batchSyncSuccess: 'Batch sync done: {ok} succeeded, {fail} failed',
      batchSyncPartial: 'Batch sync partial: {ok} succeeded, {fail} failed',
      batchKillSuccess: 'Batch kill done: {ok} succeeded, {fail} failed',
      batchKillPartial: 'Batch kill partial: {ok} succeeded, {fail} failed',
      batchForceFailSuccess: 'Batch force-fail done: {ok} succeeded, {fail} failed',
      batchForceFailPartial: 'Batch force-fail partial: {ok} succeeded, {fail} failed',
      batchNoUnsettled: 'No unsettled jobs in selection',
      batchKillConfirmTitle: 'Batch kill selected tasks?',
      batchKillConfirmMessage: 'Will terminate {count} unsettled task(s), mark cancelled, and attempt refund settlement.',
      batchForceFailConfirmTitle: 'Batch force-fail selected tasks?',
      batchForceFailConfirmMessage: 'Will mark {count} unsettled task(s) as failed and attempt refund settlement.',

      copyOk: 'Copied',
      copyFail: 'Copy failed'
    }
  }
}
