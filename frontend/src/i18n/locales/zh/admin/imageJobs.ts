export default {
  generationJobs: {
    title: '生成任务',
    video: '视频任务',
    image: '图片任务'
  },
  imageJobs: {
    title: '图片任务',
    description: '跨用户审计生图提示词、参考图、参数与结果，并终止仍在执行的异常任务。',
    searchPlaceholder: '任务 ID / 用户 / 模型 / 提示词',
    filters: {
      allStatus: '全部状态',
      allGateways: '全部执行方式',
      activeOnly: '仅进行中'
    },
    columns: {
      taskId: '任务 ID',
      userGroup: '用户 / 分组',
      model: '模型 / 执行方式',
      status: '状态',
      prompt: '提示词',
      createdAt: '创建时间',
      actions: '操作'
    },
    status: {
      created: '已创建',
      queued: '排队中',
      running: '生成中',
      succeeded: '已完成',
      failed: '失败',
      canceled: '已终止',
      expired: '已过期',
      unknown: '未知'
    },
    gateway: {
      async: '异步任务',
      sync: '同步请求'
    },
    scope: {
      async: '取消实际执行并锁定本地状态',
      local: '终止本地记录并拦截迟到回调'
    },
    actions: {
      refresh: '刷新',
      query: '查询',
      detail: '审计',
      preview: '预览',
      loadPreview: '加载结果',
      terminate: '终止',
      close: '关闭'
    },
    empty: {
      title: '暂无图片任务',
      description: '当前筛选条件下没有生图任务'
    },
    detail: {
      title: '图片任务审计',
      basic: '基础信息',
      prompt: '提示词',
      references: '参考图',
      result: '生成结果',
      error: '错误信息',
      params: '完整请求参数',
      loadingPreview: '正在加载结果图…',
      noResult: '当前没有可预览的结果',
      fields: {
        taskId: '任务 ID',
        status: '状态',
        user: '用户',
        group: '分组',
        apiKey: 'API Key',
        model: '模型',
        gateway: '执行方式',
        terminationScope: '终止范围',
        size: '尺寸 / 比例 / 画质',
        createdAt: '创建时间',
        updatedAt: '更新时间',
        mimeType: '内容类型'
      }
    },
    messages: {
      loadFailed: '加载图片任务失败',
      previewFailed: '加载生成结果失败',
      terminateTitle: '终止这个图片任务？',
      terminateAsync: '系统会取消仍在运行的异步生图执行，并将任务状态锁定为已终止。',
      terminateLocal: '同步接口没有上游取消能力。系统会把本地任务标记为已终止并拒绝迟到回调，但上游请求仍可能在外部完成。',
      terminateSuccess: '图片任务已终止',
      terminateFailed: '终止图片任务失败'
    }
  }
}
