// Compatibility keys retained while the official locale tree is being
// reorganized. These keys cover the merged account-center/admin surfaces.
export default {
  admin: {
    accounts: { messages: { accountCreated: 'Account created' } },
    groups: {
      creazyCanvas: { allow: 'Allow Creazy Canvas', hint: 'Allow keys in this group to use Creazy Canvas.', title: 'Creazy Canvas' },
      modelsList: { title: 'Model list', hint: 'Choose models exposed by this group.', selectedSummary: '{selected} / {total} selected', selectAll: 'Select all', invertSelection: 'Invert selection', loading: 'Loading models…', empty: 'No models available' },
    },
  },
  keys: {
    providerLabel: 'Provider',
    bulkEdit: { apply: 'Apply', clearSelection: 'Clear selection', failureHint: 'Some keys could not be updated.', hint: 'Edit selected keys.', invalidExpiration: 'Invalid expiration.', invalidLimit: 'Invalid limit.', ipHint: 'Optional IP restriction.', limitHint: 'Leave empty for unlimited.', partialFailure: 'Some updates failed.', selectKey: 'Select key', selectedCount: '{count} selected', success: 'Keys updated', title: 'Bulk edit' },
    useKeyModal: {
      codexModelCatalog: { title: 'Codex model catalog', description: 'Download the model catalog.', download: 'Download', errorDescription: 'Unable to load catalog.', fetch: 'Fetch catalog', modelsCount: '{count} models', retry: 'Retry' },
      composite: { description: 'Composite provider', codexDescription: 'Composite Codex catalog', codexNote: 'Models are routed by the group.' },
      deepseek: { description: 'DeepSeek provider', codexDescription: 'DeepSeek Codex catalog', codexNote: 'Uses the configured DeepSeek account.' },
      minimax: { description: 'MiniMax provider', codexDescription: 'MiniMax Codex catalog', codexNote: 'Uses the configured MiniMax account.' },
      routedCodex: { description: 'Routed Codex catalog', note: 'Models are routed by the selected group.' },
    },
  },
  modelPlaza: { table: { cacheReadShort: 'Cache read', cacheWriteShort: 'Cache write', marginalBadge: 'Marginal', maxReasoningMultiplierBadge: 'Max reasoning', maxReasoningMultiplierHint: 'Maximum reasoning multiplier', tierHint: 'Pricing tier', tierHintMarginal: 'Marginal tier', timePricingRateHint: 'Time-based pricing', timePricingRowHintPeak: 'Peak period', timePricingWeekdays: 'Weekdays' } },
  monitorCommon: { providers: { minimax: 'MiniMax', opencode_go: 'OpenCode' } },
  redeem: { historyLoadFailed: 'Failed to load redemption history', userRefreshFailed: 'Failed to refresh user' },
  usage: { allCompactionTypes: 'All compaction types', compactionFilter: 'Compaction', compactionOnly: 'Compaction only', nativeCompactionV2: 'Native compaction v2' },
}
