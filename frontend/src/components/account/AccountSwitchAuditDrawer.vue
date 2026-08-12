<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { listSwitchAudit, type AccountSwitchAuditEvent } from '@/api/admin/accounts'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  show: boolean
}>()
const emit = defineEmits<{
  (e: 'close'): void
}>()

const appStore = useAppStore()
const loading = ref(false)
const items = ref<AccountSwitchAuditEvent[]>([])
const disclaimer = ref('')
const retentionHours = ref(24)
const filters = ref({
  user_id: '' as string,
  group_id: '' as string,
  account_id: '' as string,
  reason: '' as string,
  limit: 100
})

async function load() {
  loading.value = true
  try {
    const params: Record<string, number | string> = { limit: filters.value.limit }
    if (filters.value.user_id) params.user_id = Number(filters.value.user_id)
    if (filters.value.group_id) params.group_id = Number(filters.value.group_id)
    if (filters.value.account_id) params.account_id = Number(filters.value.account_id)
    if (filters.value.reason) params.reason = filters.value.reason
    const res = await listSwitchAudit(params as any)
    items.value = res.items || []
    disclaimer.value = res.disclaimer || ''
    retentionHours.value = res.retention_hours || 24
  } catch (e: any) {
    appStore.showError(e?.message || '加载切号审计失败')
  } finally {
    loading.value = false
  }
}

function formatTime(v?: string) {
  if (!v) return '-'
  try {
    return new Date(v).toLocaleString()
  } catch {
    return v
  }
}

function ttftClass(ms?: number | null) {
  const v = Number(ms || 0)
  if (!v || v <= 0) return ''
  if (v < 1500) return 'ttft-good'
  if (v < 5000) return 'ttft-mid'
  return 'ttft-bad'
}

function reasonLabel(r?: string) {
  const raw = (r || '').trim()
  const map: Record<string, string> = {
    ttft: '首字绝对阈值',
    ttft_relative: '首字相对偏慢',
    ttft_explore: '首字探索切号(同伴未采样)',
    error_rate: '错误率',
    concurrency_full: '并发已满',
    safe_rate: '安全倍率',
    upstream_failover: '上游失败切号',
    upstream_429: '上游 429',
    upstream_500: '上游 500',
    upstream_502: '上游 502',
    upstream_503: '上游 503',
    rate_limited: '上游限流',
    no_available_account: '无可用账号',
    switch_exhausted: '切换次数耗尽'
  }
  if (map[raw]) return map[raw]
  if (raw.startsWith('upstream_')) {
    const code = raw.slice('upstream_'.length)
    return `上游 ${code}`
  }
  if (raw.includes(':')) {
    return raw
      .split(':')
      .map((part) => map[part] || part)
      .join(' · ')
  }
  return raw || '-'
}

function isFailoverEvent(ev: AccountSwitchAuditEvent) {
  return ev.layer === 'failover' || ev.event_type === 'failover_switch'
}

function toAccountLabel(ev: AccountSwitchAuditEvent) {
  if (ev.to_account_id && ev.to_account_id > 0) {
    return `#${ev.to_account_id} ${ev.to_account_name || ''}`.trim()
  }
  return isFailoverEvent(ev) ? '未选出目标账号' : '-'
}

function routeThresholdText(ev: AccountSwitchAuditEvent) {
  if (isFailoverEvent(ev)) {
    return ev.to_account_id && ev.to_account_id > 0
      ? '上游失败后重选账号 · 完整上下文转发'
      : '上游失败后重选失败 · 完整上下文仍会转发'
  }
  const ttft = ev.threshold_ttft_ms != null ? Math.round(ev.threshold_ttft_ms) : null
  const rel = ev.relative_ratio != null && ev.relative_ratio > 0 ? ev.relative_ratio : null
  if (ttft == null && rel == null) return '粘性逃逸'
  const parts: string[] = []
  if (ttft != null) parts.push(`阈值 TTFT ${ttft}ms`)
  if (rel != null) parts.push(`相对 ${rel}x`)
  return parts.join(' · ')
}

watch(
  () => props.show,
  (v) => {
    if (v) load()
  }
)

onMounted(() => {
  if (props.show) load()
})
</script>

<template>
  <div v-if="show" class="switch-audit-overlay" @click.self="emit('close')">
    <div class="switch-audit-panel">
      <header class="switch-audit-header">
        <div>
          <p class="kicker">SWITCH AUDIT · {{ retentionHours }}H</p>
          <h3>自动切号审计</h3>
          <p class="sub">{{ disclaimer || '触发自动切换时记录用户、选号过程与评分依据（进程内保留 24 小时）' }}</p>
        </div>
        <button type="button" class="btn btn-secondary" @click="emit('close')">关闭</button>
      </header>

      <div class="switch-audit-filters">
        <input v-model="filters.user_id" class="input" placeholder="用户 ID" />
        <input v-model="filters.group_id" class="input" placeholder="分组 ID" />
        <input v-model="filters.account_id" class="input" placeholder="账号 ID" />
        <select v-model="filters.reason" class="input">
          <option value="">全部原因</option>
          <option value="ttft">首字绝对阈值</option>
          <option value="ttft_relative">首字相对偏慢</option>
          <option value="error_rate">错误率</option>
          <option value="concurrency_full">并发已满</option>
          <option value="safe_rate">安全倍率</option>
          <option value="upstream_500">上游 500</option>
          <option value="upstream_429">上游 429</option>
          <option value="upstream_502">上游 502</option>
          <option value="upstream_503">上游 503</option>
          <option value="upstream_failover">上游失败切号</option>
        </select>
        <button type="button" class="btn btn-primary" :disabled="loading" @click="load">刷新</button>
      </div>

      <div v-if="loading" class="switch-audit-empty">加载中…</div>
      <div v-else-if="!items.length" class="switch-audit-empty">近 {{ retentionHours }} 小时暂无自动切号记录（仅记录 sticky 逃逸与上游失败切号；同账号重试/未触发逃逸不会出现）</div>

      <div v-else class="switch-audit-list">
        <article v-for="ev in items" :key="ev.id" class="switch-audit-card">
          <div class="card-top">
            <div class="badges">
              <span class="badge reason">{{ reasonLabel(ev.reason) }}</span>
              <span class="badge layer">{{ ev.layer || ev.event_type }}</span>
              <span v-if="ev.context_preserved" class="badge ok">上下文已保留</span>
            </div>
            <time>{{ formatTime(ev.at) }}</time>
          </div>

          <div class="meta-grid">
            <div><label>用户</label><span>{{ ev.user_id || '-' }}</span></div>
            <div><label>分组</label><span>{{ ev.group_id || '-' }}</span></div>
            <div><label>模型</label><span>{{ ev.model || '-' }}</span></div>
            <div><label>平台</label><span>{{ ev.platform || '-' }}</span></div>
            <div><label>请求</label><span class="mono">{{ ev.request_id || '-' }}</span></div>
            <div><label>会话</label><span class="mono">{{ ev.session_hash_short || '-' }}</span></div>
          </div>

          <div class="route-line">
            <div class="route-node from">
              <small>FROM</small>
              <strong>#{{ ev.from_account_id || '-' }} {{ ev.from_account_name || '' }}</strong>
              <span v-if="ev.has_from_ttft">TTFT {{ Math.round(ev.from_ttft_ms || 0) }}ms · err {{ ((ev.from_error_rate || 0) * 100).toFixed(1) }}%</span>
              <span v-else-if="isFailoverEvent(ev)">上游失败源账号</span>
            </div>
            <div class="route-arrow">→</div>
            <div class="route-node to" :class="{ missing: !ev.to_account_id }">
              <small>TO</small>
              <strong>{{ toAccountLabel(ev) }}</strong>
              <span>{{ routeThresholdText(ev) }}</span>
            </div>
          </div>

          <details v-if="ev.candidates?.length" class="candidates">
            <summary>候选评分（{{ ev.candidates.length }}）</summary>
            <table>
              <thead>
                <tr>
                  <th>账号</th>
                  <th>分数</th>
                  <th>TTFT</th>
                  <th>错误率</th>
                  <th>负载</th>
                  <th>等待</th>
                  <th>倍率</th>
                  <th>标记</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="c in ev.candidates"
                  :key="c.account_id"
                  :class="{ selected: c.selected, escaped: c.escaped }"
                >
                  <td>#{{ c.account_id }} {{ c.account_name || '' }}</td>
                  <td>
                    <span :title="c.has_ttft ? '实测首字反比分' : '无首字样本（非完美分，仅表示可探索）'">
                      {{ c.score.toFixed(1) }}
                    </span>
                  </td>
                  <td>
                    <span v-if="c.has_ttft" :class="ttftClass(c.ttft_ms)">{{ Math.round(c.ttft_ms || 0) }}ms</span>
                    <span v-else class="ttft-unmeasured" title="该账号近期无首字样本，评分仅为探索占位">未采样</span>
                  </td>
                  <td>{{ (c.error_rate * 100).toFixed(1) }}%</td>
                  <td>{{ c.load_rate.toFixed(1) }}%</td>
                  <td>{{ c.waiting_count }}</td>
                  <td>{{ c.rate_multiplier != null ? c.rate_multiplier.toFixed(2) : '-' }}</td>
                  <td>
                    <span v-if="c.escaped">逃逸</span>
                    <span v-else-if="c.selected">选中</span>
                    <span v-else>-</span>
                  </td>
                </tr>
              </tbody>
            </table>
            <p v-if="ev.score_weights" class="weights">
              权重 P{{ ev.score_weights.priority }} / L{{ ev.score_weights.load }} / Q{{ ev.score_weights.queue }} /
              E{{ ev.score_weights.error_rate }} / T{{ ev.score_weights.ttft }}
            </p>
          </details>
          <p v-if="ev.note" class="note">{{ ev.note }}</p>
        </article>
      </div>
    </div>
  </div>
</template>

<style scoped>
.switch-audit-overlay {
  position: fixed;
  inset: 0;
  z-index: 10050;
  background: rgba(8, 12, 18, 0.55);
  display: flex;
  justify-content: flex-end;
  backdrop-filter: blur(2px);
}
.switch-audit-panel {
  width: min(920px, 100vw);
  height: 100%;
  background: linear-gradient(180deg, #f7fafc 0%, #eef3f7 100%);
  color: #122033;
  box-shadow: -18px 0 40px rgba(8, 16, 28, 0.2);
  display: flex;
  flex-direction: column;
  padding: 20px;
  overflow: hidden;
}
.dark .switch-audit-panel {
  background: linear-gradient(180deg, #121820 0%, #0d1218 100%);
  color: #e7eef6;
}
.switch-audit-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 14px;
}
.kicker {
  font-size: 11px;
  letter-spacing: 0.12em;
  color: #0f8f72;
  font-weight: 700;
  margin: 0 0 4px;
}
.switch-audit-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.sub {
  margin: 6px 0 0;
  font-size: 12px;
  opacity: 0.72;
  max-width: 640px;
}
.switch-audit-filters {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}
.switch-audit-empty {
  padding: 40px 12px;
  text-align: center;
  opacity: 0.7;
}
.switch-audit-list {
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-right: 4px;
}
.switch-audit-card {
  border: 1px solid rgba(15, 40, 60, 0.1);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.86);
  padding: 14px;
  box-shadow: 0 8px 20px rgba(16, 32, 48, 0.05);
}
.dark .switch-audit-card {
  background: rgba(22, 30, 40, 0.9);
  border-color: rgba(255, 255, 255, 0.08);
}
.card-top {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  margin-bottom: 10px;
}
.badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.badge {
  font-size: 11px;
  border-radius: 999px;
  padding: 2px 8px;
  font-weight: 600;
}
.badge.reason {
  background: #fff4d6;
  color: #8a5a00;
}
.badge.layer {
  background: #e7f0ff;
  color: #1d4f91;
}
.badge.ok {
  background: #dbf7ee;
  color: #0b6b52;
}
.meta-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}
.meta-grid label {
  display: block;
  font-size: 11px;
  opacity: 0.55;
}
.meta-grid span {
  font-size: 13px;
  font-weight: 600;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px !important;
}
.route-line {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 10px;
  align-items: center;
  margin-bottom: 10px;
}
.route-node {
  border-radius: 12px;
  padding: 10px;
  background: #f3f7fb;
}
.dark .route-node {
  background: rgba(255, 255, 255, 0.04);
}
.route-node.from {
  border-left: 3px solid #d97706;
}
.route-node.to.missing {
  border-color: rgba(220, 100, 60, 0.35);
  background: rgba(255, 244, 236, 0.9);
}
.dark .route-node.to.missing {
  background: rgba(80, 40, 20, 0.35);
}
.route-node.to {
  border-left: 3px solid #0f8f72;
}
.route-node small {
  display: block;
  font-size: 10px;
  letter-spacing: 0.08em;
  opacity: 0.55;
}
.route-node strong {
  display: block;
  margin: 2px 0;
}
.route-node span {
  font-size: 12px;
  opacity: 0.75;
}
.route-arrow {
  font-size: 18px;
  opacity: 0.5;
}
.candidates summary {
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}
.candidates table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.candidates th,
.candidates td {
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  padding: 6px 4px;
  text-align: left;
}
.dark .candidates th,
.dark .candidates td {
  border-bottom-color: rgba(255, 255, 255, 0.08);
}
.candidates tr.selected td {
  color: #0b6b52;
  font-weight: 700;
}
.candidates tr.escaped td {
  color: #b45309;
}
.weights,
.note {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.7;
}
@media (max-width: 768px) {
  .switch-audit-filters,
  .meta-grid {
    grid-template-columns: 1fr 1fr;
  }
  .route-line {
    grid-template-columns: 1fr;
  }
  .route-arrow {
    display: none;
  }
}

.ttft-unmeasured {
  color: var(--text-secondary, #94a3b8);
  font-size: 12px;
}
.ttft-good { color: #16a34a; font-weight: 600; }
.ttft-mid { color: #ca8a04; font-weight: 600; }
.ttft-bad { color: #dc2626; font-weight: 700; }
.route-node.to.missing strong {
  color: #dc2626;
}
</style>
