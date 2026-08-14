export default {
  generationJobs: {
    title: 'Generation Jobs',
    video: 'Video jobs',
    image: 'Image jobs'
  },
  imageJobs: {
    title: 'Image Jobs',
    description: 'Audit prompts, references, parameters, and results across users, and terminate abnormal image jobs that are still active.',
    searchPlaceholder: 'Task ID / user / model / prompt',
    filters: {
      allStatus: 'All statuses',
      allGateways: 'All execution modes',
      activeOnly: 'Active only'
    },
    columns: {
      taskId: 'Task ID',
      userGroup: 'User / Group',
      model: 'Model / Mode',
      status: 'Status',
      prompt: 'Prompt',
      createdAt: 'Created',
      actions: 'Actions'
    },
    status: {
      created: 'Created',
      queued: 'Queued',
      running: 'Running',
      succeeded: 'Succeeded',
      failed: 'Failed',
      canceled: 'Terminated',
      expired: 'Expired',
      unknown: 'Unknown'
    },
    gateway: {
      async: 'Async task',
      sync: 'Synchronous request'
    },
    scope: {
      async: 'Cancel execution and lock local status',
      local: 'Terminate local record and reject late callbacks'
    },
    actions: {
      refresh: 'Refresh',
      query: 'Search',
      detail: 'Audit',
      preview: 'Preview',
      loadPreview: 'Load result',
      terminate: 'Terminate',
      close: 'Close'
    },
    empty: {
      title: 'No image jobs',
      description: 'No image generation jobs match the current filters'
    },
    detail: {
      title: 'Image job audit',
      basic: 'Basics',
      prompt: 'Prompt',
      references: 'Reference images',
      result: 'Generated result',
      error: 'Error',
      params: 'Full request parameters',
      loadingPreview: 'Loading generated image…',
      noResult: 'No result is available for preview',
      fields: {
        taskId: 'Task ID',
        status: 'Status',
        user: 'User',
        group: 'Group',
        apiKey: 'API Key',
        model: 'Model',
        gateway: 'Execution mode',
        terminationScope: 'Termination scope',
        size: 'Size / Ratio / Quality',
        createdAt: 'Created at',
        updatedAt: 'Updated at',
        mimeType: 'Content type'
      }
    },
    messages: {
      loadFailed: 'Failed to load image jobs',
      previewFailed: 'Failed to load generated result',
      terminateTitle: 'Terminate this image job?',
      terminateAsync: 'The running async image execution will be canceled and its status will be locked as terminated.',
      terminateLocal: 'The synchronous API has no upstream cancel operation. The local job will be terminated and late callbacks rejected, but the upstream request may still complete externally.',
      terminateSuccess: 'Image job terminated',
      terminateFailed: 'Failed to terminate image job'
    }
  }
}
