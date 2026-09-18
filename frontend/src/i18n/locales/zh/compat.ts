export default {
  admin: {
    accounts: { messages: { accountCreated: '账号已创建' } },
    groups: {
      creazyCanvas: { allow: '允许 Creazy 画布', hint: '允许此分组的 Key 使用 Creazy 画布。', title: 'Creazy 画布' },
      modelsList: { title: '模型列表', hint: '选择此分组对外展示的模型。', selectedSummary: '已选择 {selected} / {total}', selectAll: '全选', invertSelection: '反选', loading: '正在加载模型…', empty: '暂无可用模型' },
    },
  },
  keys: {
    providerLabel: '供应商',
    bulkEdit: { apply: '应用', clearSelection: '清除选择', failureHint: '部分 Key 更新失败。', hint: '批量编辑选中的 Key。', invalidExpiration: '有效期无效。', invalidLimit: '限制无效。', ipHint: '可选 IP 限制。', limitHint: '留空表示不限。', partialFailure: '部分更新失败。', selectKey: '选择 Key', selectedCount: '已选择 {count} 个', success: 'Key 已更新', title: '批量编辑' },
    useKeyModal: {
      codexModelCatalog: { title: 'Codex 模型目录', description: '下载模型目录。', download: '下载', errorDescription: '无法加载模型目录。', fetch: '获取目录', modelsCount: '{count} 个模型', retry: '重试' },
      composite: { description: '复合供应商', codexDescription: '复合 Codex 目录', codexNote: '模型由分组路由。' },
      deepseek: { description: 'DeepSeek 供应商', codexDescription: 'DeepSeek Codex 目录', codexNote: '使用已配置的 DeepSeek 账号。' },
      minimax: { description: 'MiniMax 供应商', codexDescription: 'MiniMax Codex 目录', codexNote: '使用已配置的 MiniMax 账号。' },
      routedCodex: { description: '路由 Codex 目录', note: '模型由所选分组路由。' },
    },
  },
  modelPlaza: { table: { cacheReadShort: '缓存读取', cacheWriteShort: '缓存写入', marginalBadge: '边际', maxReasoningMultiplierBadge: '最大推理', maxReasoningMultiplierHint: '最大推理倍率', tierHint: '价格档位', tierHintMarginal: '边际档位', timePricingRateHint: '分时定价', timePricingRowHintPeak: '高峰时段', timePricingWeekdays: '工作日' } },
  monitorCommon: { providers: { minimax: 'MiniMax', opencode_go: 'OpenCode' } },
  redeem: { historyLoadFailed: '兑换历史加载失败', userRefreshFailed: '用户刷新失败' },
  usage: { allCompactionTypes: '全部压缩类型', compactionFilter: '压缩', compactionOnly: '仅压缩', nativeCompactionV2: '原生压缩 v2' },
}
