<template>
  <g
    class="wf-signal-edge"
    :class="{
      'wf-signal-edge--active': active,
      'wf-signal-edge--selected': selected,
      'wf-signal-edge--dashed': data?.dashed,
    }"
  >
    <path :d="edgePath" class="wf-signal-edge__rail" />
    <BaseEdge
      :id="id"
      :path="edgePath"
      :marker-start="markerStart"
      :marker-end="markerEnd"
      :interaction-width="interactionWidth || 24"
      class="wf-signal-edge__path"
      :style="pathStyle"
    />

    <EdgeLabelRenderer>
      <div
        v-if="showLabel"
        class="wf-signal-edge__label nodrag nopan"
        :style="labelPosition"
      >
        {{ data?.label || '数据' }}
      </div>
    </EdgeLabelRenderer>
  </g>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  useVueFlow,
  type EdgeProps,
} from '@vue-flow/core'

interface SignalEdgeData {
  color?: string
  dashed?: boolean
  label?: string
  signal?: string
}

const props = defineProps<EdgeProps<SignalEdgeData>>()
const { viewport } = useVueFlow()

const pathData = computed(() =>
  getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
    borderRadius: 11,
    offset: 20,
  }),
)

const edgePath = computed(() => pathData.value[0])
const labelX = computed(() => pathData.value[1])
const labelY = computed(() => pathData.value[2])
const color = computed(() => props.data?.color || '#52606d')
const active = computed(
  () => props.sourceNode?.data?.status === 'running' || props.targetNode?.data?.status === 'running',
)
const showLabel = computed(() => Boolean(props.selected || active.value || viewport.value.zoom >= 1.15))
const pathStyle = computed<CSSProperties>(() => ({
  '--edge-color': color.value,
  stroke: props.selected || active.value ? color.value : '#7b8794',
  strokeWidth: props.selected ? 2.3 : active.value ? 2 : 1.55,
  strokeDasharray: props.data?.dashed ? '7 5' : undefined,
} as CSSProperties))
const labelPosition = computed<CSSProperties>(() => ({
  transform: `translate(-50%, -50%) translate(${labelX.value}px, ${labelY.value}px)`,
}))
</script>

<style scoped>
.wf-signal-edge__rail {
  fill: none;
  stroke: rgba(255, 255, 255, 0.68);
  stroke-width: 4;
  vector-effect: non-scaling-stroke;
}

.wf-signal-edge__path {
  fill: none;
  opacity: 0.62;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
  transition: opacity 160ms ease, stroke-width 160ms ease;
}

.wf-signal-edge--selected .wf-signal-edge__path,
.wf-signal-edge--active .wf-signal-edge__path {
  opacity: 1;
}

.wf-signal-edge--active .wf-signal-edge__path {
  stroke-dasharray: 9 6;
  animation: wf-signal-flow 850ms linear infinite;
}

.wf-signal-edge__label {
  position: absolute;
  z-index: 4;
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 6px;
  border: 1px solid #d8dee6;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.94);
  color: #495463;
  font-family: "IBM Plex Mono", "Cascadia Mono", ui-monospace, monospace;
  font-size: 8px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1;
  pointer-events: none;
  white-space: nowrap;
}

@keyframes wf-signal-flow {
  to { stroke-dashoffset: -30; }
}

@media (prefers-reduced-motion: reduce) {
  .wf-signal-edge--active .wf-signal-edge__path { animation: none; }
}
</style>
