export default {
  videoJobs: {
    title: '视频任务',
    description: '查看所有视频生成任务的提示词、参考素材、结果与用户/分组信息；支持按平台任务 ID 或用户搜索，同步上游状态，强制失败或结束任务。',
    searchPlaceholder: '平台任务 ID / 用户邮箱 / 用户名 / 用户ID / 模型',
    jobIdPlaceholder: '精确平台任务 ID',
    modelPlaceholder: '模型名',
    filters: {
      allStatus: '全部状态',
      unsettledOnly: '仅未结算',
      status: '任务状态',
      model: '模型'
    },
    columns: {
      jobId: '任务 ID',
      userGroup: '用户 / 分组',
      model: '模型',
      status: '状态',
      refund: '退款',
      prompt: '提示词',
      createdAt: '创建时间',
      actions: '操作'
    },
    status: {
      queued: '排队中',
      running: '生成中',
      succeeded: '成功',
      failed: '失败',
      cancelled: '已取消',
      unknown: '未知'
    },
    refund: {
      pending: '待退款',
      applied: '已退款',
      error: '退款失败',
      not_required: '无需退款',
      empty: '-'
    },
    actions: {
      refresh: '刷新',
      detail: '详情',
      sync: '同步上游',
      kill: '结束(取消)',
      forceFail: '强制失败',
      batchSync: '批量同步上游',
      batchKill: '批量终止',
      batchForceFail: '批量强制失败',
      clearSelection: '清除选择',
      openResult: '打开结果'
    },

    batch: {
      selectedCount: '已选 {count} 项',
      unsettledCount: '其中未结算 {count}'
    },
    empty: {
      title: '暂无视频任务',
      description: '当前筛选条件下没有视频生成任务'
    },
    detail: {
      title: '任务详情',
      basic: '基础信息',
      prompt: '提示词',
      materials: '参考素材',
      result: '生成结果',
      settlement: '结算信息',
      noPrompt: '无提示词',
      noMaterials: '无参考素材',
      noResult: '尚无结果（任务未成功）',
      resultPath: '结果路径',
      lastError: '最近错误',
      snapshotJson: '请求快照',
      fields: {
        jobId: '任务 ID',
        upstreamJobId: '上游任务 ID',
        user: '用户',
        group: '分组',
        apiKey: 'API Key',
        accountId: '账号 ID',
        model: '模型',
        fallbackModel: '回退模型',
        fallbackStatus: '回退状态',
        taskStatus: '任务状态',
        refundStatus: '退款状态',
        settlementAttempts: '结算次数',
        refundAttempts: '退款次数',
        resolution: '分辨率',
        duration: '时长(秒)',
        aspectRatio: '宽高比',
        createdAt: '创建时间',
        updatedAt: '更新时间',
        settledAt: '结算时间',
        refundedAt: '退款时间',
        lastPolledAt: '最近轮询',
        nextPollAt: '下次轮询'
      }
    },
    material: {
      startFrame: '首帧',
      endFrame: '尾帧',
      image: '图片参考',
      video: '视频参考',
      audio: '音频参考'
    },
    messages: {
      loadFailed: '加载视频任务失败',
      syncSuccess: '已同步上游状态',
      syncFailed: '同步失败',
      killSuccess: '任务已结束并尝试退款结算',
      killFailed: '结束任务失败',
      killConfirmTitle: '确认结束任务？',
      killConfirmMessage: '将尽量取消上游任务，并把本地状态标记为已取消，同时触发退款结算。已结算任务不会重复退款。',
      forceFailSuccess: '任务已标记失败并尝试退款结算',
      forceFailFailed: '强制失败操作失败',
      forceFailConfirmTitle: '确认强制失败？',
      forceFailConfirmMessage: '将尽量取消上游任务，并把本地状态标记为失败，同时触发退款结算。已结算任务不会重复退款。',

      batchSyncSuccess: '批量同步完成：成功 {ok}，失败 {fail}',
      batchSyncPartial: '批量同步部分失败：成功 {ok}，失败 {fail}',
      batchKillSuccess: '批量终止完成：成功 {ok}，失败 {fail}',
      batchKillPartial: '批量终止部分失败：成功 {ok}，失败 {fail}',
      batchForceFailSuccess: '批量强制失败完成：成功 {ok}，失败 {fail}',
      batchForceFailPartial: '批量强制失败部分失败：成功 {ok}，失败 {fail}',
      batchNoUnsettled: '所选任务中没有未结算项',
      batchKillConfirmTitle: '批量终止所选任务？',
      batchKillConfirmMessage: '将终止 {count} 个未结算任务，标记为已取消，并尝试退款结算。',
      batchForceFailConfirmTitle: '批量强制失败所选任务？',
      batchForceFailConfirmMessage: '将把 {count} 个未结算任务标记为失败，并尝试退款结算。',

      copyOk: '已复制',
      copyFail: '复制失败'
    }
  }
}
