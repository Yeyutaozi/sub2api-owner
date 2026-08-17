<template>
  <section ref="shellEl" class="wf-shell" aria-label="AI 生成工作流画布">
    <header class="wf-commandbar">
      <div class="wf-document">
        <span class="wf-document__mark" aria-hidden="true"></span>
        <input
          v-model="documentName"
          class="wf-document__name"
          maxlength="120"
          aria-label="工作流名称"
          @blur="saveDocument(true)"
        />
        <span class="wf-save-state" :class="`wf-save-state--${saveTone}`">
          <i></i>{{ saveLabel }}
        </span>
      </div>
      <div class="wf-commandbar__actions">
        <div class="wf-edit-group" aria-label="编辑工具">
          <button type="button" class="wf-icon-button" title="撤销" :disabled="!canUndo || graphEditingLocked" @click="undoEditor">
            <Icon name="undo" size="sm" />
          </button>
          <button type="button" class="wf-icon-button" title="重做" :disabled="!canRedo || graphEditingLocked" @click="redoEditor">
            <Icon name="redo" size="sm" />
          </button>
          <button type="button" class="wf-icon-button" title="搜索并添加节点" @click="openCommandPalette()">
            <Icon name="search" size="sm" />
          </button>
          <button type="button" class="wf-icon-button" title="自动排版" :disabled="nodes.length < 2" @click="autoLayout">
            <Icon name="layout" size="sm" />
          </button>
        </div>
        <button
          type="button"
          class="wf-icon-button"
          title="立即保存"
          aria-label="立即保存"
          :disabled="saving || saveState === 'conflict'"
          @click="saveDocument(true)"
        >
          <Icon name="cloud" size="sm" />
        </button>
        <div class="wf-run-group">
          <button
            type="button"
            class="btn btn-primary btn-sm wf-run-button"
            :disabled="!generationNodes.length || workflowRun.active"
            @click="prepareRun('workflow')"
          >
            <Icon name="play" size="sm" class="mr-1.5" />
            {{ workflowRun.active ? '运行中' : '运行工作流' }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm wf-run-menu-button"
            :disabled="!apiReady || workflowRun.active"
            title="更多运行方式"
            aria-label="更多运行方式"
            :aria-expanded="runMenuOpen"
            @click="runMenuOpen = !runMenuOpen"
          >
            <Icon name="chevronDown" size="xs" />
          </button>
          <div v-if="runMenuOpen" class="wf-run-menu" role="menu">
            <button type="button" role="menuitem" :disabled="!canRunSelected" @click="prepareRun('node')">
              <Icon name="play" size="sm" /><span><strong>运行所选节点</strong><small>仅执行当前生成节点</small></span>
            </button>
            <button type="button" role="menuitem" :disabled="!canRunSelected" @click="prepareRun('downstream')">
              <Icon name="arrowRight" size="sm" /><span><strong>从这里运行</strong><small>按依赖执行全部下游</small></span>
            </button>
            <button type="button" role="menuitem" :disabled="!generationNodes.length" @click="prepareRun('workflow')">
              <Icon name="workflow" size="sm" /><span><strong>运行完整工作流</strong><small>校验并执行全部分支</small></span>
            </button>
          </div>
        </div>
      </div>
    </header>

    <div v-if="saveState === 'conflict'" class="wf-conflict-bar" role="alert">
      <span><Icon name="exclamationTriangle" size="sm" />{{ saveError }}</span>
      <div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="reloadCloudDocument">
          加载云端版本
        </button>
        <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="saveConflictAsCopy">
          另存本地副本
        </button>
      </div>
    </div>

    <div
      class="wf-workspace"
      :class="{ 'wf-workspace--inspecting': selectedNode, 'wf-workspace--loading': loading }"
      :aria-busy="loading"
    >
      <div v-if="loading" class="wf-loading-state" role="status">
        <Icon name="refresh" size="sm" class="animate-spin" />正在载入工作流
      </div>
      <aside class="wf-palette" aria-label="节点工具">
        <div class="wf-palette__heading">
          <span>{{ paletteTab === 'nodes' ? '节点库' : paletteTab === 'assets' ? '资产库' : '工作流模板' }}</span>
          <small>{{ paletteTab === 'nodes' ? '拖入画布或点击添加' : paletteTab === 'assets' ? '搜索并复用当前画布资产' : '一键插入常用生成链路' }}</small>
        </div>
        <div class="wf-palette-tabs" role="tablist" aria-label="资源面板">
          <button type="button" :class="{ active: paletteTab === 'nodes' }" @click="paletteTab = 'nodes'">节点</button>
          <button type="button" :class="{ active: paletteTab === 'assets' }" @click="paletteTab = 'assets'">资产</button>
          <button type="button" :class="{ active: paletteTab === 'templates' }" @click="paletteTab = 'templates'">模板</button>
        </div>
        <template v-if="paletteTab === 'nodes'">
        <button type="button" class="wf-tool wf-tool--asset" title="上传资产" :disabled="!apiReady" @click="openAssetPicker">
          <span class="wf-tool__icon"><Icon name="upload" size="sm" /></span>
          <span><strong>资产</strong><small>图片、视频、音频</small></span>
        </button>
        <button
          type="button"
          class="wf-tool wf-tool--prompt"
          title="添加提示词节点"
          draggable="true"
          @dragstart="onPaletteDragStart($event, 'prompt')"
          @dragend="onPaletteDragEnd"
          @click="addNode('prompt')"
        >
          <span class="wf-tool__icon"><Icon name="document" size="sm" /></span>
          <span><strong>提示词</strong><small>描述画面与镜头</small></span>
        </button>
        <button
          type="button"
          class="wf-tool wf-tool--image"
          title="添加图片生成节点"
          draggable="true"
          :disabled="!imageModels.length"
          @dragstart="onPaletteDragStart($event, 'image')"
          @dragend="onPaletteDragEnd"
          @click="addNode('image')"
        >
          <span class="wf-tool__icon"><Icon name="sparkles" size="sm" /></span>
          <span><strong>图片生成</strong><small>文字或参考图生图</small></span>
        </button>
        <button
          type="button"
          class="wf-tool wf-tool--video"
          title="添加视频生成节点"
          draggable="true"
          :disabled="!videoModels.length"
          @dragstart="onPaletteDragStart($event, 'video')"
          @dragend="onPaletteDragEnd"
          @click="addNode('video')"
        >
          <span class="wf-tool__icon"><Icon name="play" size="sm" /></span>
          <span><strong>视频生成</strong><small>图片、视频或文字驱动</small></span>
        </button>
        </template>
        <template v-else-if="paletteTab === 'assets'">
          <label class="wf-asset-search">
            <Icon name="search" size="xs" />
            <input v-model="assetSearch" placeholder="搜索资产" />
          </label>
          <button type="button" class="wf-upload-card" :disabled="!apiReady" @click="openAssetPicker">
            <Icon name="upload" size="sm" /><span><strong>上传新资产</strong><small>图片、视频或音频</small></span>
          </button>
          <div v-if="filteredAssetNodes.length" class="wf-asset-list">
            <button v-for="asset in filteredAssetNodes" :key="asset.id" type="button" @click="focusLibraryAsset(asset.id)">
              <span class="wf-asset-list__preview">
                <img v-if="asset.data.mediaKind === 'image' && mediaPreviewUrl(asset.data)" :src="mediaPreviewUrl(asset.data)" alt="" />
                <Icon v-else :name="asset.data.mediaKind === 'audio' ? 'sync' : 'play'" size="sm" />
              </span>
              <span><strong>{{ asset.data.name || asset.data.title }}</strong><small>{{ mediaKindLabel(asset.data.mediaKind) }} · {{ statusLabel(asset.data.status) }}</small></span>
            </button>
          </div>
          <p v-else class="wf-palette-empty">当前画布还没有可复用资产</p>
        </template>
        <template v-else>
          <button type="button" class="wf-template-card" @click="insertWorkflowTemplate('text-image')">
            <span><Icon name="sparkles" size="sm" /></span><strong>文生图</strong><small>提示词 → 图片生成</small>
          </button>
          <button type="button" class="wf-template-card" @click="insertWorkflowTemplate('image-video')">
            <span><Icon name="play" size="sm" /></span><strong>图生视频</strong><small>资产 + 提示词 → 视频</small>
          </button>
          <button type="button" class="wf-template-card" @click="insertWorkflowTemplate('storyboard')">
            <span><Icon name="workflow" size="sm" /></span><strong>分镜生成链</strong><small>提示词 → 生图 → 生视频</small>
          </button>
        </template>
        <div class="wf-palette__divider"></div>
        <a class="wf-task-link" href="/creazy-canvas/works" title="查看全部任务" aria-label="查看全部任务">
          <Icon name="inbox" size="sm" />
        </a>
      </aside>

      <div
        ref="stageEl"
        class="wf-stage"
        :class="{ 'wf-stage--dragging': draggingFile }"
        @dragenter.prevent="onStageDragEnter"
        @dragover.prevent="onStageDragOver"
        @dragleave="onStageDragLeave"
        @drop.prevent="onStageDrop"
        @pointermove="onStagePointerMove"
      >
        <VueFlow
          id="creazy-workflow"
          v-model:nodes="nodes"
          v-model:edges="edges"
          class="wf-flow"
          :min-zoom="0.35"
          :max-zoom="1.8"
          :snap-to-grid="true"
          :snap-grid="[16, 16]"
          :delete-key-code="null"
          :selection-key-code="true"
          :multi-selection-key-code="['Control', 'Meta']"
          pan-activation-key-code="Space"
          :pan-on-drag="[1]"
          :selection-mode="SelectionMode.Partial"
          :nodes-draggable="!graphEditingLocked"
          :nodes-connectable="!graphEditingLocked"
          :edges-updatable="!graphEditingLocked"
          :edge-updater-radius="18"
          :elevate-nodes-on-select="true"
          :elevate-edges-on-select="true"
          :is-valid-connection="isValidConnection"
          :default-edge-options="defaultEdgeOptions"
          @connect="onConnect"
          @node-click="onNodeClick"
          @node-context-menu="onNodeContextMenu"
          @edge-context-menu="onEdgeContextMenu"
          @pane-context-menu="onPaneContextMenu"
          @selection-context-menu="onSelectionContextMenu"
          @node-drag-start="onNodeDragStart"
          @node-drag-stop="onNodeDragStop"
          @edge-update="onEdgeUpdate"
          @pane-click="onPaneClick"
          @viewport-change-end="onViewportChangeEnd"
        >
          <Background pattern-color="#d9dee4" :gap="28" :size="1" />
          <MiniMap
            v-if="nodes.length > 5"
            class="wf-minimap"
            :pannable="true"
            :zoomable="true"
            :node-color="miniMapNodeColor"
            mask-color="rgba(226, 232, 240, 0.72)"
          />
          <Controls class="wf-controls" position="bottom-left" />

          <template #edge-signal="edgeProps">
            <WorkflowSignalEdge v-bind="edgeProps" />
          </template>

          <template #node-workflow="slotProps">
            <article
              class="wf-node"
              :class="[
                `wf-node--${slotProps.data.kind}`,
                `wf-node--status-${slotProps.data.status || 'idle'}`,
                { 'wf-node--selected': slotProps.selected },
              ]"
              @pointerdown.capture="onNodePointerDown($event, slotProps.id)"
            >
              <template v-for="port in inputPorts(slotProps.data)" :key="port.id">
                <Handle
                  :id="port.id"
                  type="target"
                  :position="Position.Left"
                  :connectable="slotProps.data.kind !== 'result'"
                  class="wf-handle wf-handle--target"
                  :class="`wf-handle--${port.signal}`"
                  :style="{ top: port.position, '--port-color': port.color }"
                  :title="`${port.label}输入`"
                  :aria-label="`${port.label}输入端口`"
                />
                <span
                  class="wf-port-label wf-port-label--target"
                  :style="{ top: port.position, '--port-color': port.color }"
                >{{ port.label }}</span>
              </template>
              <header class="wf-node__head wf-node-drag">
                <span class="wf-node__type-icon"><Icon :name="nodeIcon(slotProps.data)" size="sm" /></span>
                <div>
                  <strong>{{ slotProps.data.title }}</strong>
                  <small>{{ nodeTypeLabel(slotProps.data) }}</small>
                </div>
                <button
                  v-if="slotProps.data.kind === 'image' || slotProps.data.kind === 'video'"
                  type="button"
                  class="wf-node__quick-run nodrag"
                  :title="slotProps.data.status === 'failed' ? '重新运行节点' : '运行节点'"
                  :aria-label="slotProps.data.status === 'failed' ? '重新运行节点' : '运行节点'"
                  :disabled="slotProps.data.status === 'running' || activeNodeRuns.has(slotProps.id) || workflowRun.active || !apiReady"
                  @pointerdown.stop
                  @click.stop="runNode(slotProps.id)"
                >
                  <Icon
                    :name="slotProps.data.status === 'running' ? 'refresh' : 'play'"
                    size="xs"
                    :class="{ 'animate-spin': slotProps.data.status === 'running' }"
                  />
                </button>
                <span class="wf-node__status" :title="statusLabel(slotProps.data.status)">
                  <i></i>{{ statusShortLabel(slotProps.data.status) }}
                </span>
              </header>

              <div class="wf-node__body">
                <template v-if="slotProps.data.kind === 'asset' || slotProps.data.kind === 'result'">
                  <div class="wf-node__media" :class="`wf-node__media--${slotProps.data.mediaKind || 'image'}`">
                    <img
                      v-if="slotProps.data.mediaKind === 'image' && mediaPreviewUrl(slotProps.data)"
                      :src="mediaPreviewUrl(slotProps.data)"
                      alt=""
                    />
                    <video
                      v-else-if="slotProps.data.mediaKind === 'video' && playableMediaUrl(slotProps.data)"
                      :src="playableMediaUrl(slotProps.data)"
                      muted
                      playsinline
                    />
                    <span v-else class="wf-node__media-placeholder">
                      <Icon :name="slotProps.data.mediaKind === 'audio' ? 'sync' : 'play'" size="lg" />
                      {{ mediaKindLabel(slotProps.data.mediaKind) }}
                    </span>
                  </div>
                  <p class="wf-node__file">{{ slotProps.data.name || '生成结果' }}</p>
                  <div class="wf-node__media-meta">
                    <small v-if="slotProps.data.workId" class="wf-node__meta">任务 #{{ slotProps.data.workId }}</small>
                    <span v-if="slotProps.data.kind === 'result' && slotProps.data.runCount" class="wf-node__version">V{{ slotProps.data.runCount }}</span>
                  </div>
                  <button
                    v-if="slotProps.data.kind === 'result' && slotProps.data.sourceNodeId"
                    type="button"
                    class="wf-node__source-link nodrag"
                    @pointerdown.stop
                    @click.stop="focusLibraryAsset(slotProps.data.sourceNodeId)"
                  ><Icon name="workflow" size="xs" />定位生成节点</button>
                </template>

                <template v-else-if="slotProps.data.kind === 'prompt'">
                  <p class="wf-node__prompt" :class="{ 'wf-node__prompt--empty': !slotProps.data.prompt }">
                    {{ slotProps.data.prompt || '在右侧填写提示词' }}
                  </p>
                </template>

                <template v-else-if="slotProps.data.kind === 'image'">
                  <div class="wf-node__prompt-box" :class="{ 'wf-node__prompt-box--empty': !resolvedPromptForNode(slotProps.id) }">
                    <span>提示词</span>
                    <template v-if="hasIncomingPrompt(slotProps.id)">
                      <p>{{ resolvedPromptForNode(slotProps.id) }}</p>
                      <button type="button" class="nodrag" @pointerdown.stop @click.stop="focusIncomingPrompt(slotProps.id)">编辑上游提示词</button>
                    </template>
                    <template v-else>
                      <textarea
                        class="nodrag"
                        rows="3"
                        :value="slotProps.data.prompt"
                        placeholder="直接输入提示词，或创建独立提示词节点"
                        @pointerdown.stop
                        @input="patchNode(slotProps.id, { prompt: inputValue($event) })"
                      ></textarea>
                    </template>
                    <button
                      v-if="!hasIncomingPrompt(slotProps.id)"
                      type="button"
                      class="nodrag"
                      @pointerdown.stop
                      @click.stop="addPromptForNode(slotProps.id)"
                    ><Icon name="plus" size="xs" />{{ slotProps.data.prompt ? '拆分为提示词节点' : '添加提示词节点' }}</button>
                  </div>
                  <p class="wf-node__model">{{ slotProps.data.model || '选择图片模型' }}</p>
                  <div class="wf-node__chips">
                    <span>{{ slotProps.data.qualityTier || '1K' }}</span>
                    <span>{{ slotProps.data.aspectRatio || '1:1' }}</span>
                    <span>{{ incomingMediaCount(slotProps.id) }} 个素材</span>
                    <span v-for="label in nodeCapabilityLabels(slotProps.data)" :key="label" class="wf-node__capability">{{ label }}</span>
                  </div>
                </template>

                <template v-else-if="slotProps.data.kind === 'video'">
                  <div class="wf-node__prompt-box" :class="{ 'wf-node__prompt-box--empty': !resolvedPromptForNode(slotProps.id) }">
                    <span>提示词</span>
                    <template v-if="hasIncomingPrompt(slotProps.id)">
                      <p>{{ resolvedPromptForNode(slotProps.id) }}</p>
                      <button type="button" class="nodrag" @pointerdown.stop @click.stop="focusIncomingPrompt(slotProps.id)">编辑上游提示词</button>
                    </template>
                    <template v-else>
                      <textarea
                        class="nodrag"
                        rows="3"
                        :value="slotProps.data.prompt"
                        placeholder="直接输入提示词，或创建独立提示词节点"
                        @pointerdown.stop
                        @input="patchNode(slotProps.id, { prompt: inputValue($event) })"
                      ></textarea>
                    </template>
                    <button
                      v-if="!hasIncomingPrompt(slotProps.id)"
                      type="button"
                      class="nodrag"
                      @pointerdown.stop
                      @click.stop="addPromptForNode(slotProps.id)"
                    ><Icon name="plus" size="xs" />{{ slotProps.data.prompt ? '拆分为提示词节点' : '添加提示词节点' }}</button>
                  </div>
                  <p class="wf-node__model">{{ slotProps.data.model || '选择视频模型' }}</p>
                  <div class="wf-node__chips">
                    <span>{{ slotProps.data.resolution || '720p' }}</span>
                    <span>{{ slotProps.data.duration || 5 }}s</span>
                    <span>{{ slotProps.data.aspectRatio || '16:9' }}</span>
                    <span>{{ incomingMediaCount(slotProps.id) }} 个素材</span>
                    <span v-for="label in nodeCapabilityLabels(slotProps.data)" :key="label" class="wf-node__capability">{{ label }}</span>
                  </div>
                </template>

                <p v-if="slotProps.data.error" class="wf-node__error">{{ slotProps.data.error }}</p>
              </div>

              <footer v-if="slotProps.data.kind === 'image' || slotProps.data.kind === 'video'" class="wf-node__footer">
                <span><i></i>{{ incomingCount(slotProps.id) }} 路输入</span>
                <span class="wf-node__runtime">
                  <b>{{ estimatedNodeCostById(slotProps.id) == null ? '价格待定' : '$' + formatCost(estimatedNodeCostById(slotProps.id) || 0) }}</b>
                  <b v-if="slotProps.data.lastRunDurationMs">{{ formatRunDuration(slotProps.data.lastRunDurationMs) }}</b>
                  <b v-if="slotProps.data.runCount">V{{ slotProps.data.runCount }}</b>
                </span>
                <button
                  type="button"
                  class="nodrag"
                  :disabled="slotProps.data.status === 'running' || activeNodeRuns.has(slotProps.id) || workflowRun.active || !apiReady"
                  @pointerdown.stop
                  @click.stop="runNode(slotProps.id)"
                ><Icon :name="slotProps.data.status === 'running' ? 'refresh' : 'play'" size="xs" :class="{ 'animate-spin': slotProps.data.status === 'running' }" />{{ slotProps.data.status === 'running' ? '生成中' : '运行节点' }}</button>
              </footer>

              <Handle
                id="output"
                type="source"
                :position="Position.Right"
                class="wf-handle wf-handle--source"
                :class="`wf-handle--${outputPort(slotProps.data).signal}`"
                :style="{ '--port-color': outputPort(slotProps.data).color }"
                :title="`${outputPort(slotProps.data).label}输出`"
                :aria-label="`${outputPort(slotProps.data).label}输出端口`"
              />
              <span
                class="wf-port-label wf-port-label--source"
                :style="{ '--port-color': outputPort(slotProps.data).color }"
              >{{ outputPort(slotProps.data).label }}</span>
            </article>
          </template>
        </VueFlow>

        <div v-if="selectedNodes.length > 1 || selectedEdges.length" class="wf-selection-toolbar" role="toolbar" aria-label="所选元素操作">
          <span>已选 {{ selectedNodes.length }} 个节点<span v-if="selectedEdges.length"> · {{ selectedEdges.length }} 条连线</span></span>
          <button type="button" title="复制所选节点" :disabled="!selectedNodes.length" @click="copySelection">
            <Icon name="copy" size="sm" />
          </button>
          <button type="button" title="创建副本" :disabled="!selectedNodes.length" @click="duplicateSelection">
            <Icon name="duplicate" size="sm" />
          </button>
          <button type="button" title="聚焦所选内容" @click="fitSelection">
            <Icon name="grid" size="sm" />
          </button>
          <button type="button" class="danger" title="删除所选内容" @click="deleteSelection">
            <Icon name="trash" size="sm" />
          </button>
        </div>

        <div v-if="workflowRun.active || workflowRun.finished" class="wf-run-progress" aria-live="polite">
          <div>
            <span>{{ workflowRun.finished ? '执行完成' : '工作流执行中' }}</span>
            <strong>{{ workflowRun.completed }}/{{ workflowRun.total }}</strong>
          </div>
          <div class="wf-run-progress__bar"><i :style="{ width: workflowRunProgress + '%' }"></i></div>
          <small>{{ workflowRun.message }}</small>
          <button v-if="workflowRun.active" type="button" @click="stopWorkflowRun">停止后续节点</button>
          <button v-else type="button" aria-label="关闭执行状态" title="关闭" @click="dismissWorkflowRun"><Icon name="x" size="xs" /></button>
        </div>

        <div
          v-if="contextMenu"
          class="wf-context-menu"
          :style="contextMenuStyle"
          role="menu"
          @click.stop
        >
          <template v-if="contextMenu.kind === 'pane'">
            <button type="button" role="menuitem" @click="addNodeFromContext('prompt')"><Icon name="document" size="sm" />添加提示词</button>
            <button type="button" role="menuitem" :disabled="!imageModels.length" @click="addNodeFromContext('image')"><Icon name="sparkles" size="sm" />添加图片生成</button>
            <button type="button" role="menuitem" :disabled="!videoModels.length" @click="addNodeFromContext('video')"><Icon name="play" size="sm" />添加视频生成</button>
            <div></div>
            <button type="button" role="menuitem" :disabled="!clipboardFragment" @click="pasteSelection(contextMenu.flowPosition)"><Icon name="clipboard" size="sm" />粘贴</button>
            <button type="button" role="menuitem" :disabled="nodes.length < 2" @click="autoLayout"><Icon name="layout" size="sm" />自动排版</button>
          </template>
          <template v-else-if="contextMenu.kind === 'edge'">
            <button type="button" role="menuitem" class="danger" @click="deleteContextEdge"><Icon name="trash" size="sm" />删除连接</button>
          </template>
          <template v-else>
            <button v-if="contextNodeCanRun" type="button" role="menuitem" @click="prepareRunForContext('node')"><Icon name="play" size="sm" />运行节点</button>
            <button v-if="contextNodeCanRun" type="button" role="menuitem" @click="prepareRunForContext('downstream')"><Icon name="arrowRight" size="sm" />从这里运行</button>
            <div v-if="contextNodeCanRun"></div>
            <button type="button" role="menuitem" @click="copySelection"><Icon name="copy" size="sm" />复制</button>
            <button type="button" role="menuitem" @click="duplicateSelection"><Icon name="duplicate" size="sm" />创建副本</button>
            <button type="button" role="menuitem" @click="fitSelection"><Icon name="grid" size="sm" />聚焦所选内容</button>
            <div></div>
            <button type="button" role="menuitem" class="danger" @click="deleteSelection"><Icon name="trash" size="sm" />删除</button>
          </template>
        </div>

        <div v-if="commandPaletteOpen" class="wf-command-overlay" @pointerdown.self="closeCommandPalette">
          <section class="wf-command-palette" role="dialog" aria-modal="true" aria-label="搜索节点与命令">
            <label>
              <Icon name="search" size="sm" />
              <input
                ref="commandInput"
                v-model="commandQuery"
                type="search"
                placeholder="搜索节点或操作"
                autocomplete="off"
                @keydown.enter.prevent="runFirstCommand"
                @keydown.esc.prevent="closeCommandPalette"
              />
            </label>
            <div role="listbox">
              <button
                v-for="command in filteredCommands"
                :key="command.id"
                type="button"
                role="option"
                :disabled="command.disabled"
                @click="runCommand(command)"
              >
                <span><Icon :name="command.icon" size="sm" /></span>
                <span><strong>{{ command.label }}</strong><small>{{ command.description }}</small></span>
              </button>
              <p v-if="!filteredCommands.length">没有匹配的节点或操作</p>
            </div>
          </section>
        </div>

        <div v-if="runConfirmation" class="wf-dialog-backdrop" @pointerdown.self="cancelRunConfirmation">
          <section class="wf-run-dialog" role="dialog" aria-modal="true" aria-labelledby="wf-run-title">
            <header>
              <div><span>执行预检</span><h2 id="wf-run-title">{{ runConfirmation.title }}</h2></div>
              <button type="button" class="wf-icon-button" title="关闭" @click="cancelRunConfirmation"><Icon name="x" size="sm" /></button>
            </header>
            <div class="wf-run-summary">
              <div><span>待执行</span><strong>{{ runConfirmation.nodeIds.length }} 个节点</strong></div>
              <div><span>预计费用</span><strong>{{ runConfirmation.cost == null ? '以实际扣费为准' : '$' + formatCost(runConfirmation.cost) }}</strong></div>
              <div><span>并行批次</span><strong>{{ runConfirmation.layers.length }}</strong></div>
            </div>
            <label class="wf-run-reuse">
              <input v-model="reuseCompletedOutputs" type="checkbox" @change="refreshRunConfirmation" />
              <span><strong>复用已完成结果</strong><small>跳过已有输出的上游节点，只运行受影响部分</small></span>
            </label>
            <div v-if="runConfirmation.issues.length" class="wf-run-issues">
              <button v-for="issue in runConfirmation.issues" :key="issue.key" type="button" @click="focusIssue(issue)">
                <Icon :name="issue.blocking ? 'exclamationCircle' : 'infoCircle'" size="sm" />
                <span><strong>{{ issue.title }}</strong><small>{{ issue.message }}</small></span>
              </button>
            </div>
            <div v-else class="wf-run-ready"><Icon name="checkCircle" size="sm" />图结构与节点参数检查通过</div>
            <div class="wf-run-plan">
              <span v-for="nodeId in runConfirmation.nodeIds" :key="nodeId">{{ nodeTitle(nodeId) }}</span>
            </div>
            <footer>
              <button type="button" class="btn btn-secondary" @click="cancelRunConfirmation">取消</button>
              <button type="button" class="btn btn-primary" :disabled="runConfirmation.issues.some((issue) => issue.blocking)" @click="confirmWorkflowRun">
                <Icon name="play" size="sm" class="mr-1.5" />开始运行
              </button>
            </footer>
          </section>
        </div>

        <div v-if="draggingFile" class="wf-drop-overlay">
          <Icon name="upload" size="xl" />
          <strong>{{ draggingNodeKind ? '松开以添加节点' : '松开以上传到画布' }}</strong>
          <span>{{ draggingNodeKind ? '节点会放置在当前落点' : '支持图片、视频和音频' }}</span>
        </div>
        <div v-if="connectionNotice" class="wf-toast">{{ connectionNotice }}</div>
      </div>

      <aside v-if="selectedNode" class="wf-inspector" aria-label="节点参数">
        <template v-if="selectedNode && selectedNode.data">
          <div class="wf-inspector__head">
            <div>
              <span>节点参数 · {{ incomingCount(selectedNode.id) }} 输入</span>
              <strong>{{ selectedNode.data.title }}</strong>
            </div>
            <div class="wf-inspector__tools">
              <button type="button" class="wf-icon-button" title="删除节点" aria-label="删除节点" @click="deleteSelectedNode">
                <Icon name="trash" size="sm" />
              </button>
              <button type="button" class="wf-icon-button" title="关闭参数栏" aria-label="关闭参数栏" @click="clearElementSelection">
                <Icon name="x" size="sm" />
              </button>
            </div>
          </div>

          <div class="wf-inspector__body">
          <label class="wf-field">
            <span>名称</span>
            <input :value="selectedNode.data.title" maxlength="80" @input="updateSelected({ title: inputValue($event) })" />
          </label>

          <template v-if="selectedNode.data.kind === 'prompt'">
            <label class="wf-field wf-field--grow">
              <span>提示词</span>
              <textarea
                rows="10"
                :value="selectedNode.data.prompt"
                placeholder="描述画面、镜头、风格或动作"
                @input="updateSelected({ prompt: inputValue($event) })"
              ></textarea>
            </label>
          </template>

          <template v-else-if="selectedNode.data.kind === 'image'">
            <label class="wf-field">
              <span>图片模型</span>
              <select :value="selectedNode.data.model" @change="onImageModelChange(inputValue($event))">
                <option v-for="model in imageModels" :key="model.id" :value="model.id">{{ model.name || model.id }}</option>
              </select>
            </label>
            <label class="wf-field wf-field--grow">
              <span>节点提示词</span>
              <textarea
                rows="5"
                :value="selectedNode.data.prompt"
                placeholder="可留空，自动使用上游提示词节点"
                @input="updateSelected({ prompt: inputValue($event) })"
              ></textarea>
            </label>
            <div class="wf-field">
              <span>画质</span>
              <div class="wf-segments">
                <button
                  v-for="tier in selectedImageQualityOptions"
                  :key="tier"
                  type="button"
                  :class="{ active: selectedNode.data.qualityTier === tier }"
                  @click="updateSelectedImageQuality(tier)"
                >{{ tier }}</button>
              </div>
            </div>
            <label class="wf-field">
              <span>画面比例</span>
              <select :value="selectedNode.data.aspectRatio" @change="updateSelectedImageAspect(inputValue($event))">
                <option v-for="ratio in selectedImageAspectOptions" :key="ratio" :value="ratio">{{ ratio }}</option>
              </select>
            </label>
            <div class="wf-resolution-preview">
              <span>实际尺寸</span><strong>{{ selectedNode.data.size || '自动' }}</strong>
            </div>
          </template>

          <template v-else-if="selectedNode.data.kind === 'video'">
            <label class="wf-field">
              <span>视频模型</span>
              <select :value="selectedNode.data.model" @change="onVideoModelChange(inputValue($event))">
                <option v-for="model in videoModels" :key="model.id" :value="model.id">{{ model.name || model.id }}</option>
              </select>
            </label>
            <label class="wf-field wf-field--grow">
              <span>节点提示词</span>
              <textarea
                rows="5"
                :value="selectedNode.data.prompt"
                placeholder="可留空，自动使用上游提示词节点"
                @input="updateSelected({ prompt: inputValue($event) })"
              ></textarea>
            </label>
            <div class="wf-field-grid">
              <label class="wf-field">
                <span>分辨率</span>
                <select :value="selectedNode.data.resolution" @change="updateSelected({ resolution: inputValue($event) })">
                  <option v-for="item in selectedVideoResolutionOptions" :key="item" :value="item">{{ item }}</option>
                </select>
              </label>
              <label class="wf-field">
                <span>时长</span>
                <select :value="selectedNode.data.duration" @change="updateSelected({ duration: Number(inputValue($event)) })">
                  <option v-for="item in selectedVideoDurationOptions" :key="item" :value="item">{{ item }} 秒</option>
                </select>
              </label>
            </div>
            <label class="wf-field">
              <span>画面比例</span>
              <select :value="selectedNode.data.aspectRatio" @change="updateSelected({ aspectRatio: inputValue($event) })">
                <option v-for="item in selectedVideoAspectOptions" :key="item" :value="item">{{ item }}</option>
              </select>
            </label>
            <label v-if="selectedVideoModel?.allow_generated_audio" class="wf-toggle">
              <input
                type="checkbox"
                :checked="selectedNode.data.generateAudio"
                @change="updateSelected({ generateAudio: checkedValue($event) })"
              />
              <span>生成原生音频</span>
            </label>
          </template>

          <template v-else>
            <dl class="wf-asset-details">
              <div><dt>类型</dt><dd>{{ mediaKindLabel(selectedNode.data.mediaKind) }}</dd></div>
              <div v-if="selectedNode.data.mimeType"><dt>格式</dt><dd>{{ selectedNode.data.mimeType }}</dd></div>
              <div v-if="selectedNode.data.workId"><dt>任务</dt><dd>#{{ selectedNode.data.workId }}</dd></div>
              <div><dt>状态</dt><dd>{{ statusLabel(selectedNode.data.status) }}</dd></div>
            </dl>
            <button
              v-if="selectedNode.data.mediaKind === 'video' && selectedNode.data.workId && !playableMediaUrl(selectedNode.data)"
              type="button"
              class="btn btn-secondary btn-sm wf-inspector__wide-action"
              @click="loadSelectedVideo"
            >
              <Icon name="play" size="sm" class="mr-1.5" />加载视频预览
            </button>
          </template>

          <p v-if="selectedNode.data.error" class="wf-inspector__error">{{ selectedNode.data.error }}</p>
          </div>

          <div
            v-if="selectedNode.data.kind === 'image' || selectedNode.data.kind === 'video'"
            class="wf-inspector__actions"
          >
          <button
            type="button"
            class="btn btn-primary wf-inspector__run"
            :disabled="selectedNode.data.status === 'running' || activeNodeRuns.has(selectedNode.id) || workflowRun.active || !apiReady"
            @click="runNode(selectedNode.id)"
          >
            <Icon name="play" size="sm" class="mr-2" />
            {{ selectedNode.data.status === 'failed' ? '重新运行节点' : '运行节点' }}
          </button>
          </div>
        </template>

      </aside>
    </div>

    <input
      ref="assetInput"
      type="file"
      accept="image/*,video/*,audio/*"
      multiple
      class="hidden"
      @change="onAssetInput"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  Handle,
  MarkerType,
  Position,
  SelectionMode,
  VueFlow,
  useVueFlow,
  type Connection,
  type Edge,
  type EdgeMouseEvent,
  type EdgeUpdateEvent,
  type Node,
  type NodeDragEvent,
  type NodeMouseEvent,
  type ViewportTransform,
  type XYPosition,
} from '@vue-flow/core'
import dagre from '@dagrejs/dagre'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'
import Icon from '@/components/icons/Icon.vue'
import WorkflowSignalEdge from './WorkflowSignalEdge.vue'
import {
  createDocument,
  createWork,
  generateImage,
  generateVideo,
  getDocument,
  getImageTask,
  getVideoContentURL,
  getVideoJob,
  getWork,
  getWorkContentBlob,
  listDocuments,
  updateDocument,
  updateWork,
  uploadVideoAsset,
  type CreazyCanvasCatalog,
  type CreazyCanvasDocument,
  type CreazyCanvasGraph,
  type CreazyCanvasImageModel,
  type CreazyCanvasVideoModel,
  type CreazyWork,
  type ImageGenerationResponse,
  type VideoJob,
} from '@/api/creazyCanvas'
import { buildImageWorkParams, buildVideoWorkParams } from './composables/workParams'
import { useWorkflowHistory, type WorkflowCommand } from './composables/useWorkflowHistory'
import {
  downstreamNodeIds,
  internalEdges,
  topologicalOrder,
  wouldCreateCycle,
} from './composables/workflowGraph'

type WorkflowKind = 'asset' | 'prompt' | 'image' | 'video' | 'result'
type PaletteNodeKind = 'prompt' | 'image' | 'video'
type MediaKind = 'image' | 'video' | 'audio'
type NodeStatus = 'idle' | 'uploading' | 'running' | 'succeeded' | 'failed' | 'canceled'
type SignalKind = 'prompt' | 'image' | 'video' | 'audio' | 'data'

interface WorkflowPort {
  id: string
  label: string
  signal: SignalKind
  color: string
  position: string
}

interface WorkflowNodeData {
  kind: WorkflowKind
  title: string
  status: NodeStatus
  prompt?: string
  model?: string
  size?: string
  qualityTier?: string
  aspectRatio?: string
  resolution?: string
  duration?: number
  generateAudio?: boolean
  mediaKind?: MediaKind
  mediaUrl?: string
  previewUrl?: string
  playableUrl?: string
  outputUrl?: string
  name?: string
  mimeType?: string
  durationSeconds?: number
  workId?: number
  sourceNodeId?: string
  gatewayRemoteId?: string
  upstreamReady?: boolean
  error?: string
  lastRunDurationMs?: number
  runCount?: number
}

type WorkflowNode = Node<WorkflowNodeData, Record<string, never>, 'workflow'> & {
  data: WorkflowNodeData
  selected?: boolean
}
type WorkflowEdge = Edge & { selected?: boolean }

type RunScope = 'node' | 'downstream' | 'workflow'
type PaletteTab = 'nodes' | 'assets' | 'templates'
type WorkflowTemplateKind = 'text-image' | 'image-video' | 'storyboard'
type CommandIcon = 'document' | 'sparkles' | 'play' | 'upload' | 'layout' | 'grid' | 'workflow' | 'clipboard'

interface CommandPaletteItem {
  id: string
  label: string
  description: string
  keywords: string
  icon: CommandIcon
  disabled?: boolean
  action: () => void | Promise<void>
}

interface WorkflowClipboardFragment {
  type: 'creazy-workflow-fragment'
  version: 1
  nodes: Array<{ sourceId: string; position: XYPosition; data: WorkflowNodeData }>
  edges: Array<{ source: string; target: string }>
}

interface LocalWorkflowDraft {
  documentId: number
  baseRevision: number
  updatedAt: number
  dirty: boolean
  name?: string
  graph?: CreazyCanvasGraph
}

interface ContextMenuState {
  kind: 'pane' | 'node' | 'edge'
  x: number
  y: number
  nodeId?: string
  edgeId?: string
  flowPosition?: XYPosition
}

interface RunIssue {
  key: string
  nodeId?: string
  title: string
  message: string
  blocking: boolean
}

interface RunConfirmation {
  scope: RunScope
  sourceNodeId?: string
  title: string
  nodeIds: string[]
  layers: string[][]
  issues: RunIssue[]
  cost: number | null
}

const props = defineProps<{
  apiKeyId: number
  apiKeySecret: string
  catalog: CreazyCanvasCatalog | null
}>()

const emit = defineEmits<{
  workCreated: [workId: number]
}>()

const DEFAULT_IMAGE_RATIOS = ['1:1', '3:2', '2:3', '4:3', '3:4', '5:4', '4:5', '16:9', '9:16', '2:1', '1:2', '21:9', '9:21']
const DEFAULT_VIDEO_RATIOS = ['16:9', '9:16', '1:1']
const DEFAULT_RESOLUTIONS = ['480p', '720p', '1080p']
const DEFAULT_DURATIONS = [5, 10]
const LOCAL_DRAFT_PREFIX = 'creazy-workflow-draft-v1'
const SIGNAL_META: Record<SignalKind, { color: string; label: string; dashed?: boolean }> = {
  prompt: { color: '#a16207', label: '提示词', dashed: true },
  image: { color: '#2563eb', label: '图片' },
  video: { color: '#7c3aed', label: '视频' },
  audio: { color: '#0f766e', label: '音频' },
  data: { color: '#52606d', label: '数据' },
}

const nodes = ref<WorkflowNode[]>([])
const edges = ref<WorkflowEdge[]>([])
const selectedNodeId = ref('')
const documentId = ref(0)
const documentRevision = ref(0)
const documentName = ref('我的工作流')
const savedViewport = ref<ViewportTransform>({ x: 0, y: 0, zoom: 0.9 })
const loading = ref(true)
const saving = ref(false)
const saveState = ref<'loading' | 'saved' | 'dirty' | 'saving' | 'local' | 'conflict' | 'error'>('loading')
const saveError = ref('')
const draggingFile = ref(false)
const draggingNodeKind = ref<PaletteNodeKind | ''>('')
const paletteTab = ref<PaletteTab>('nodes')
const assetSearch = ref('')
const connectionNotice = ref('')
const assetInput = ref<HTMLInputElement | null>(null)
const shellEl = ref<HTMLElement | null>(null)
const stageEl = ref<HTMLElement | null>(null)
const commandInput = ref<HTMLInputElement | null>(null)
const commandPaletteOpen = ref(false)
const commandQuery = ref('')
const commandAnchor = ref<XYPosition | null>(null)
const contextMenu = ref<ContextMenuState | null>(null)
const clipboardFragment = ref<WorkflowClipboardFragment | null>(null)
const lastCanvasPointer = ref<XYPosition | null>(null)
const runMenuOpen = ref(false)
const runConfirmation = ref<RunConfirmation | null>(null)
const conflictDraft = ref<LocalWorkflowDraft | null>(null)
const reuseCompletedOutputs = ref(true)
const workflowRun = ref({
  id: '',
  active: false,
  finished: false,
  stopRequested: false,
  completed: 0,
  total: 0,
  message: '',
})
const localObjectUrls = new Set<string>()
const materializedMediaByWork = new Map<number, string>()
const activeRemotePolls = new Set<number>()
const foregroundWorkIds = new Set<number>()
const activeNodeRuns = new Set<string>()
const activeNodeRunCount = ref(0)
let saveTimer: ReturnType<typeof setTimeout> | null = null
let statusTimer: ReturnType<typeof setInterval> | null = null
let noticeTimer: ReturnType<typeof setTimeout> | null = null
let saveInFlight: Promise<void> | null = null
let saveRequested = false
let hydrating = true
let disposed = false
let refreshingStatuses = false
let lastSavedSnapshot = ''
let dragStartPositions: Record<string, XYPosition> | null = null
let modifierSelectionSnapshot: Set<string> | null = null
let pendingNodeEdit: {
  nodeId: string
  before: Partial<WorkflowNodeData>
  after: Partial<WorkflowNodeData>
  timer: ReturnType<typeof setTimeout> | null
} | null = null

const { fitView, screenToFlowCoordinate, setViewport } = useVueFlow('creazy-workflow')
const editorHistory = useWorkflowHistory()
const { canUndo, canRedo } = editorHistory

const imageModels = computed(() => props.catalog?.image_models || [])
const videoModels = computed(() => props.catalog?.video_models || [])
const apiReady = computed(() => Boolean(props.apiKeyId && props.apiKeySecret))
const selectedNodes = computed(() => nodes.value.filter((node) => Boolean(node.selected)))
const selectedEdges = computed(() => edges.value.filter((edge) => Boolean(edge.selected)))
const selectedNode = computed(() => selectedNodes.value.length === 1 ? selectedNodes.value[0] : null)
const generationNodes = computed(() => nodes.value.filter((node) => node.data.kind === 'image' || node.data.kind === 'video'))
const filteredAssetNodes = computed(() => {
  const query = assetSearch.value.trim().toLowerCase()
  return nodes.value.filter((node) => {
    if (node.data.kind !== 'asset' && node.data.kind !== 'result') return false
    if (!query) return true
    return [node.data.name, node.data.title, node.data.mediaKind].some((value) => String(value || '').toLowerCase().includes(query))
  })
})
const graphEditingLocked = computed(() => workflowRun.value.active || activeNodeRunCount.value > 0)
const canRunSelected = computed(() => {
  const data = selectedNode.value?.data
  const locallyRunning = activeNodeRunCount.value > 0 && Boolean(selectedNode.value && activeNodeRuns.has(selectedNode.value.id))
  return Boolean(
    apiReady.value && data && selectedNode.value &&
    (data.kind === 'image' || data.kind === 'video') &&
    data.status !== 'running' && !locallyRunning,
  )
})
const workflowRunProgress = computed(() => workflowRun.value.total
  ? Math.round((workflowRun.value.completed / workflowRun.value.total) * 100)
  : 0)
const contextNodeCanRun = computed(() => {
  const nodeId = contextMenu.value?.nodeId
  const node = nodes.value.find((item) => item.id === nodeId)
  return Boolean(node && (node.data.kind === 'image' || node.data.kind === 'video'))
})
const contextMenuStyle = computed(() => {
  const menu = contextMenu.value
  const stage = stageEl.value
  if (!menu || !stage) return {}
  const width = 214
  const height = menu.kind === 'pane' ? 258 : menu.kind === 'edge' ? 48 : 260
  return {
    left: `${Math.max(8, Math.min(menu.x, stage.clientWidth - width - 8))}px`,
    top: `${Math.max(8, Math.min(menu.y, stage.clientHeight - height - 8))}px`,
  }
})
const selectedImageModel = computed(() => {
  if (selectedNode.value?.data.kind !== 'image') return null
  return imageModels.value.find((item) => item.id === selectedNode.value?.data.model) || null
})
const selectedVideoModel = computed(() => {
  if (selectedNode.value?.data.kind !== 'video') return null
  return videoModels.value.find((item) => item.id === selectedNode.value?.data.model) || null
})
const selectedImageQualityOptions = computed(() => {
  const tiers = selectedImageModel.value?.quality_tiers?.filter(Boolean) || []
  return tiers.length ? tiers : ['1K', '2K', '4K']
})
const selectedImageAspectOptions = computed(() => {
  const ratios = selectedImageModel.value?.aspect_ratios?.filter(Boolean) || []
  return ratios.length ? ratios : DEFAULT_IMAGE_RATIOS
})
const selectedVideoResolutionOptions = computed(() => {
  const model = selectedVideoModel.value
  const values = model?.resolutions?.length ? model.resolutions : model?.allowed_resolutions
  return values?.length ? values : DEFAULT_RESOLUTIONS
})
const selectedVideoDurationOptions = computed(() => {
  const model = selectedVideoModel.value
  const values = model?.durations?.length ? model.durations : model?.allowed_durations
  return values?.length ? values : DEFAULT_DURATIONS
})
const selectedVideoAspectOptions = computed(() => {
  const model = selectedVideoModel.value
  const values = model?.aspect_ratios?.length ? model.aspect_ratios : model?.allowed_aspect_ratios
  return values?.length ? values : DEFAULT_VIDEO_RATIOS
})
const commandItems = computed<CommandPaletteItem[]>(() => [
  {
    id: 'add-prompt',
    label: '添加提示词',
    description: '文本输入节点',
    keywords: 'prompt text 提示词 文本',
    icon: 'document',
    action: () => addNode('prompt', commandAnchor.value || nextPosition()),
  },
  {
    id: 'add-image',
    label: '添加图片生成',
    description: '图片模型与参考图',
    keywords: 'image 图片 生图',
    icon: 'sparkles',
    disabled: !imageModels.value.length,
    action: () => addNode('image', commandAnchor.value || nextPosition()),
  },
  {
    id: 'add-video',
    label: '添加视频生成',
    description: '视频模型与媒体输入',
    keywords: 'video 视频 生视频',
    icon: 'play',
    disabled: !videoModels.value.length,
    action: () => addNode('video', commandAnchor.value || nextPosition()),
  },
  {
    id: 'upload-asset',
    label: '上传资产',
    description: '图片、视频或音频',
    keywords: 'asset upload 素材 资产 上传',
    icon: 'upload',
    disabled: !apiReady.value,
    action: openAssetPicker,
  },
  {
    id: 'paste',
    label: '粘贴节点',
    description: '在画布当前位置创建副本',
    keywords: 'paste clipboard 粘贴 剪贴板',
    icon: 'clipboard',
    disabled: !clipboardFragment.value,
    action: () => pasteSelection(commandAnchor.value || undefined),
  },
  {
    id: 'layout',
    label: '自动排版',
    description: '按依赖从左到右整理节点',
    keywords: 'layout arrange organize 自动 排版 整理',
    icon: 'layout',
    disabled: nodes.value.length < 2,
    action: autoLayout,
  },
  {
    id: 'fit',
    label: '适应全部内容',
    description: '将完整工作流放入视野',
    keywords: 'fit zoom 聚焦 适应',
    icon: 'grid',
    disabled: !nodes.value.length,
    action: fitCanvas,
  },
  {
    id: 'run-workflow',
    label: '运行完整工作流',
    description: '先预检依赖与预计费用',
    keywords: 'run workflow 执行 运行 工作流',
    icon: 'workflow',
    disabled: !apiReady.value || !generationNodes.value.length,
    action: () => prepareRun('workflow'),
  },
])
const filteredCommands = computed(() => {
  const query = commandQuery.value.trim().toLowerCase()
  if (!query) return commandItems.value
  return commandItems.value.filter((command) => `${command.label} ${command.description} ${command.keywords}`.toLowerCase().includes(query))
})

const saveLabel = computed(() => {
  if (saveState.value === 'loading') return '正在载入'
  if (saveState.value === 'saving') return '正在保存'
  if (saveState.value === 'dirty') return '有未保存修改'
  if (saveState.value === 'local') return '已保存本地草稿'
  if (saveState.value === 'conflict') return '检测到版本冲突'
  if (saveState.value === 'error') return saveError.value || '保存失败'
  return '已自动保存'
})
const saveTone = computed(() => {
  if (saveState.value === 'saved') return 'ok'
  if (saveState.value === 'error' || saveState.value === 'conflict') return 'bad'
  if (saveState.value === 'dirty' || saveState.value === 'local') return 'warn'
  return 'muted'
})

const defaultEdgeOptions = {
  type: 'signal',
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: SIGNAL_META.data.color,
    width: 12,
    height: 12,
  },
  interactionWidth: 24,
}

function uid(prefix: string): string {
  const random = globalThis.crypto?.randomUUID?.() || Math.random().toString(36).slice(2)
  return `${prefix}-${random}`
}

function cloneValue<T>(value: T): T {
  if (typeof structuredClone === 'function') {
    try {
      return structuredClone(value)
    } catch {
      // Vue Flow enriches nodes with reactive internals that are not cloneable.
    }
  }
  return JSON.parse(JSON.stringify(value)) as T
}

function historyCommand(label: string, redo: () => void | Promise<void>, undo: () => void | Promise<void>): WorkflowCommand {
  return {
    label,
    redo: async () => {
      await redo()
      scheduleSave()
    },
    undo: async () => {
      await undo()
      scheduleSave()
    },
  }
}

async function executeEditorCommand(command: WorkflowCommand) {
  await flushPendingNodeEdit()
  await editorHistory.execute(command)
}

async function recordEditorCommand(command: WorkflowCommand) {
  await flushPendingNodeEdit()
  await editorHistory.record(command)
  scheduleSave()
}

function selectOnlyNode(nodeId: string) {
  nodes.value = nodes.value.map((node) => ({ ...node, selected: node.id === nodeId }))
  edges.value = edges.value.map((edge) => ({ ...edge, selected: false }))
  selectedNodeId.value = nodeId
}

function selectOnlyNodes(nodeIds: Iterable<string>) {
  const ids = new Set(nodeIds)
  nodes.value = nodes.value.map((node) => ({ ...node, selected: ids.has(node.id) }))
  edges.value = edges.value.map((edge) => ({ ...edge, selected: false }))
  selectedNodeId.value = ids.size === 1 ? [...ids][0] || '' : ''
}

function clearElementSelection() {
  nodes.value = nodes.value.map((node) => node.selected ? { ...node, selected: false } : node)
  edges.value = edges.value.map((edge) => edge.selected ? { ...edge, selected: false } : edge)
  selectedNodeId.value = ''
}

function starterNodes(): WorkflowNode[] {
  const promptId = uid('prompt')
  const imageId = uid('image')
  const videoId = uid('video')
  return [
    createNode('prompt', { x: 80, y: 190 }, promptId),
    createNode('image', { x: 500, y: 120 }, imageId),
    createNode('video', { x: 940, y: 120 }, videoId),
  ]
}

function starterGraph(): { nodes: WorkflowNode[]; edges: WorkflowEdge[]; viewport: ViewportTransform } {
  const initialNodes = starterNodes()
  return {
    nodes: initialNodes,
    edges: [
      makeEdge(initialNodes[0].id, initialNodes[1].id, initialNodes),
      makeEdge(initialNodes[1].id, initialNodes[2].id, initialNodes),
    ],
    viewport: { x: 0, y: 0, zoom: 0.82 },
  }
}

function createNode(kind: WorkflowKind, position: XYPosition, id = uid(kind)): WorkflowNode {
  const imageModel = imageModels.value[0]
  const videoModel = videoModels.value[0]
  const base: WorkflowNodeData = { kind, title: '', status: 'idle' }
  if (kind === 'prompt') {
    Object.assign(base, { title: '提示词', prompt: '' })
  } else if (kind === 'image') {
    const tier = imageModel?.quality_tiers?.[0] || '1K'
    const ratio = imageModel?.aspect_ratios?.[0] || '1:1'
    Object.assign(base, {
      title: '图片生成',
      model: imageModel?.id || '',
      prompt: '',
      qualityTier: tier,
      aspectRatio: ratio,
      size: resolveImageSize(imageModel, tier, ratio),
    })
  } else if (kind === 'video') {
    Object.assign(base, {
      title: '视频生成',
      model: videoModel?.id || '',
      prompt: '',
      resolution: videoModel?.default_resolution || videoModel?.resolutions?.[0] || '720p',
      duration: videoModel?.default_duration || videoModel?.durations?.[0] || 5,
      aspectRatio: videoModel?.aspect_ratios?.[0] || '16:9',
      generateAudio: Boolean(videoModel?.allow_generated_audio || videoModel?.force_generated_audio),
    })
  }
  return {
    id,
    type: 'workflow',
    position,
    data: base,
    dragHandle: '.wf-node-drag',
    ariaLabel: base.title,
  }
}

function nextPosition(): XYPosition {
  const count = nodes.value.length
  return { x: 120 + (count % 3) * 420, y: 120 + Math.floor(count / 3) * 310 }
}

function addNode(kind: PaletteNodeKind, position = nextPosition()) {
  if (!ensureGraphEditable()) return
  const node = createNode(kind, position)
  node.selected = true
  void executeEditorCommand(historyCommand(
    `添加${node.data.title}`,
    () => {
      if (!nodes.value.some((item) => item.id === node.id)) nodes.value = [...nodes.value, cloneValue(node)]
      selectOnlyNode(node.id)
    },
    () => {
      nodes.value = nodes.value.filter((item) => item.id !== node.id)
      edges.value = edges.value.filter((edge) => edge.source !== node.id && edge.target !== node.id)
      selectedNodeId.value = ''
    },
  )).then(() => {
    void nextTick().then(() => fitView({ nodes: [node.id], padding: 0.8, duration: 280 })).catch(() => undefined)
  }).catch(() => undefined)
}

function addPromptForNode(targetId: string) {
  if (!ensureGraphEditable()) return
  const target = nodes.value.find((node) => node.id === targetId)
  if (!target || (target.data.kind !== 'image' && target.data.kind !== 'video')) return
  const promptNode = createNode('prompt', {
    x: target.position.x - 380,
    y: target.position.y + 20,
  })
  const inlinePrompt = String(target.data.prompt || '')
  promptNode.data.prompt = inlinePrompt
  promptNode.selected = true
  const edge = makeEdge(promptNode.id, target.id, [...nodes.value, promptNode])
  void executeEditorCommand(historyCommand(
    '添加并连接提示词',
    () => {
      if (!nodes.value.some((node) => node.id === promptNode.id)) nodes.value = [...nodes.value, cloneValue(promptNode)]
      if (!edges.value.some((item) => item.id === edge.id)) edges.value = [...edges.value, cloneValue(edge)]
      if (inlinePrompt) patchNode(target.id, { prompt: '' })
      selectOnlyNode(promptNode.id)
    },
    () => {
      edges.value = edges.value.filter((item) => item.id !== edge.id)
      nodes.value = nodes.value.filter((node) => node.id !== promptNode.id)
      if (inlinePrompt) patchNode(target.id, { prompt: inlinePrompt })
      selectedNodeId.value = ''
    },
  )).then(() => {
    void nextTick().then(() => fitView({ nodes: [promptNode.id, target.id], padding: 0.42, duration: 320 })).catch(() => undefined)
  }).catch(() => undefined)
}

function focusLibraryAsset(nodeId: string) {
  selectOnlyNode(nodeId)
  void nextTick().then(() => fitView({ nodes: [nodeId], padding: 0.75, duration: 280, minZoom: 0.65, maxZoom: 1.1 })).catch(() => undefined)
}

function insertWorkflowTemplate(kind: WorkflowTemplateKind) {
  if (!ensureGraphEditable()) return
  const anchor = lastCanvasPointer.value || nextPosition()
  const created: WorkflowNode[] = []
  const links: WorkflowEdge[] = []
  const prompt = createNode('prompt', { x: anchor.x, y: anchor.y + 60 })
  prompt.data.title = kind === 'storyboard' ? '分镜提示词' : '提示词'
  created.push(prompt)

  if (kind === 'text-image') {
    const image = createNode('image', { x: anchor.x + 420, y: anchor.y })
    created.push(image)
    links.push(makeEdge(prompt.id, image.id, created))
  } else if (kind === 'image-video') {
    const image = createNode('image', { x: anchor.x + 420, y: anchor.y })
    const video = createNode('video', { x: anchor.x + 860, y: anchor.y })
    created.push(image, video)
    links.push(makeEdge(prompt.id, image.id, created), makeEdge(prompt.id, video.id, created), makeEdge(image.id, video.id, created))
  } else {
    const imageA = createNode('image', { x: anchor.x + 420, y: anchor.y - 170 })
    const imageB = createNode('image', { x: anchor.x + 420, y: anchor.y + 210 })
    imageA.data.title = '关键帧 A'
    imageB.data.title = '关键帧 B'
    const video = createNode('video', { x: anchor.x + 860, y: anchor.y })
    created.push(imageA, imageB, video)
    links.push(
      makeEdge(prompt.id, imageA.id, created),
      makeEdge(prompt.id, imageB.id, created),
      makeEdge(prompt.id, video.id, created),
      makeEdge(imageA.id, video.id, created),
      makeEdge(imageB.id, video.id, created),
    )
  }

  const nodeIds = created.map((node) => node.id)
  void executeEditorCommand(historyCommand(
    '插入工作流模板',
    () => {
      const existing = new Set(nodes.value.map((node) => node.id))
      nodes.value = [...nodes.value, ...created.filter((node) => !existing.has(node.id)).map((node) => cloneValue(node))]
      const existingEdges = new Set(edges.value.map((edge) => edge.id))
      edges.value = [...edges.value, ...links.filter((edge) => !existingEdges.has(edge.id)).map((edge) => cloneValue(edge))]
      selectOnlyNodes(nodeIds)
    },
    () => {
      nodes.value = nodes.value.filter((node) => !nodeIds.includes(node.id))
      edges.value = edges.value.filter((edge) => !nodeIds.includes(edge.source) && !nodeIds.includes(edge.target))
      selectedNodeId.value = ''
    },
  )).then(() => {
    paletteTab.value = 'nodes'
    void nextTick().then(() => fitView({ nodes: nodeIds, padding: 0.25, duration: 360, minZoom: 0.45, maxZoom: 0.95 })).catch(() => undefined)
  }).catch(() => undefined)
}

function signalKindForNode(data?: WorkflowNodeData): SignalKind {
  if (!data) return 'data'
  if (data.kind === 'prompt') return 'prompt'
  if (data.kind === 'image') return 'image'
  if (data.kind === 'video') return 'video'
  if (data.mediaKind === 'image' || data.mediaKind === 'video' || data.mediaKind === 'audio') return data.mediaKind
  return 'data'
}

function outputPort(data: WorkflowNodeData): WorkflowPort {
  const signal = signalKindForNode(data)
  const meta = SIGNAL_META[signal]
  return { id: 'output', label: meta.label, signal, color: meta.color, position: '50%' }
}

function inputPorts(data: WorkflowNodeData): WorkflowPort[] {
  if (data.kind === 'image') {
    const model = imageModels.value.find((item) => item.id === data.model)
    const ports: WorkflowPort[] = [
      { id: 'prompt', label: '提示词', signal: 'prompt', color: SIGNAL_META.prompt.color, position: '34%' },
    ]
    if (model?.supports_reference !== false || model?.require_reference || Number(model?.max_reference_images || 0) > 0) {
      ports.push({
        id: 'reference-image',
        label: model?.require_reference ? '必需参考图' : Number(model?.max_reference_images || 0) > 0 ? `参考图 ×${model?.max_reference_images}` : '参考图',
        signal: 'image', color: SIGNAL_META.image.color, position: '68%',
      })
    }
    return ports
  }
  if (data.kind === 'video') {
    const model = videoModels.value.find((item) => item.id === data.model)
    const visualLabel = model?.require_start_frame
      ? '必需首帧'
      : model?.allow_start_frame && model?.allow_end_frame
        ? '首帧 / 尾帧'
        : Number(model?.max_image_references || 0) > 0
          ? `参考图 ×${model?.max_image_references}`
          : '视觉素材'
    const ports: WorkflowPort[] = [
      { id: 'prompt', label: '提示词', signal: 'prompt', color: SIGNAL_META.prompt.color, position: '28%' },
      { id: 'visual', label: visualLabel, signal: 'image', color: SIGNAL_META.image.color, position: '58%' },
    ]
    if (Number(model?.max_audio_references || 0) > 0) {
      ports.push({ id: 'audio', label: `音频 ×${model?.max_audio_references}`, signal: 'audio', color: SIGNAL_META.audio.color, position: '80%' })
    }
    return ports
  }
  if (data.kind === 'result') {
    const signal = signalKindForNode(data)
    return [{ id: 'result', label: '结果', signal, color: SIGNAL_META[signal].color, position: '50%' }]
  }
  return []
}

function nodeCapabilityLabels(data: WorkflowNodeData): string[] {
  if (data.kind === 'image') {
    const model = imageModels.value.find((item) => item.id === data.model)
    if (!model) return []
    const labels: string[] = []
    if (model.require_reference) labels.push('需参考图')
    else if (model.supports_reference !== false) labels.push('支持参考图')
    if (Number(model.max_n || 1) > 1) labels.push(`最多 ${model.max_n} 张`)
    return labels
  }
  if (data.kind === 'video') {
    const model = videoModels.value.find((item) => item.id === data.model)
    if (!model) return []
    const labels: string[] = []
    if (model.require_start_frame) labels.push('需首帧')
    else if (model.allow_start_frame) labels.push('支持首帧')
    if (model.allow_end_frame) labels.push('支持尾帧')
    if (Number(model.max_video_references || 0) > 0) labels.push('视频参考')
    if (model.allow_generated_audio || model.force_generated_audio) labels.push('原生音频')
    return labels.slice(0, 3)
  }
  return []
}

function targetHandleFor(source?: WorkflowNodeData, target?: WorkflowNodeData): string | undefined {
  if (!source || !target) return undefined
  if (target.kind === 'result') return 'result'
  if (target.kind === 'image') return source.kind === 'prompt' ? 'prompt' : 'reference-image'
  if (target.kind === 'video') {
    if (source.kind === 'prompt') return 'prompt'
    return signalKindForNode(source) === 'audio' ? 'audio' : 'visual'
  }
  return undefined
}

function edgeLabelFor(source?: WorkflowNodeData, target?: WorkflowNodeData): string {
  const handle = targetHandleFor(source, target)
  if (handle === 'reference-image') return '参考图'
  if (handle === 'visual') return '视觉参考'
  if (handle === 'audio') return '音频'
  if (handle === 'prompt') return '提示词'
  if (handle === 'result') return '生成结果'
  return SIGNAL_META[signalKindForNode(source)].label
}

function makeEdge(source: string, target: string, nodeList = nodes.value, id = uid('edge')): WorkflowEdge {
  const sourceNode = nodeList.find((node) => node.id === source)
  const targetNode = nodeList.find((node) => node.id === target)
  const signal = signalKindForNode(sourceNode?.data)
  const meta = SIGNAL_META[signal]
  return {
    id,
    source,
    target,
    sourceHandle: 'output',
    targetHandle: targetHandleFor(sourceNode?.data, targetNode?.data),
    type: 'signal',
    markerEnd: {
      type: MarkerType.ArrowClosed,
      color: meta.color,
      width: 12,
      height: 12,
    },
    interactionWidth: 24,
    data: {
      signal,
      color: meta.color,
      dashed: Boolean(meta.dashed),
      label: edgeLabelFor(sourceNode?.data, targetNode?.data),
    },
    ariaLabel: `${edgeLabelFor(sourceNode?.data, targetNode?.data)}连接`,
  }
}

function canReceiveManualInput(data: WorkflowNodeData): boolean {
  return data.kind === 'image' || data.kind === 'video'
}

function nodeDataConnectionCompatible(source: WorkflowNodeData, target: WorkflowNodeData): boolean {
  if (target.kind === 'result') return source.kind === 'image' || source.kind === 'video'
  if (!canReceiveManualInput(target)) return false
  if (target.kind === 'image') {
    if (source.kind === 'prompt') return true
    return resolvedMediaKind(source) === 'image'
  }
  return source.kind === 'prompt' || Boolean(resolvedMediaKind(source)) || source.kind === 'image'
}

function validateConnection(connection: Connection, ignoreEdgeId?: string): boolean {
  if (!connection.source || !connection.target || connection.source === connection.target) return false
  const source = nodes.value.find((node) => node.id === connection.source)
  const target = nodes.value.find((node) => node.id === connection.target)
  if (!source?.data || !target?.data) return false
  if (edges.value.some((edge) => edge.id !== ignoreEdgeId && edge.source === connection.source && edge.target === connection.target)) return false
  if (wouldCreateCycle(nodes.value, edges.value, String(connection.source), String(connection.target), ignoreEdgeId)) return false
  const expectedHandle = targetHandleFor(source.data, target.data)
  if (connection.sourceHandle && connection.sourceHandle !== 'output') return false
  if (connection.targetHandle && connection.targetHandle !== expectedHandle) return false
  return nodeDataConnectionCompatible(source.data, target.data)
}

function isValidConnection(connection: Connection): boolean {
  // Vue Flow also calls this validator for edges that already exist in the
  // store. Ignore that edge's own id so duplicate and cycle checks only
  // compare it with the rest of the graph.
  const edgeId = 'id' in connection && connection.id ? String(connection.id) : undefined
  return validateConnection(connection, edgeId)
}

function onConnect(connection: Connection) {
  if (!ensureGraphEditable()) return
  if (!validateConnection(connection)) {
    const cyclic = Boolean(connection.source && connection.target && wouldCreateCycle(
      nodes.value,
      edges.value,
      String(connection.source),
      String(connection.target),
    ))
    showConnectionNotice(cyclic ? '连接会形成循环，工作流必须保持无环' : '端口类型不兼容或连接已经存在')
    return
  }
  const edge = makeEdge(String(connection.source), String(connection.target))
  void executeEditorCommand(historyCommand(
    '添加连接',
    () => {
      if (!edges.value.some((item) => item.id === edge.id)) edges.value = [...edges.value, cloneValue(edge)]
    },
    () => {
      edges.value = edges.value.filter((item) => item.id !== edge.id)
    },
  ))
}

function showConnectionNotice(message: string) {
  connectionNotice.value = message
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => {
    connectionNotice.value = ''
    noticeTimer = null
  }, 2600)
}

function onNodeClick(event: NodeMouseEvent) {
  closeContextMenu()
  runMenuOpen.value = false
  if (!event.node?.id) return
  const mouseEvent = event.event
  if (modifierSelectionSnapshot && (mouseEvent.ctrlKey || mouseEvent.metaKey)) {
    const ids = new Set(modifierSelectionSnapshot)
    if (ids.has(event.node.id)) ids.delete(event.node.id)
    else ids.add(event.node.id)
    selectOnlyNodes(ids)
    modifierSelectionSnapshot = null
    return
  }
  modifierSelectionSnapshot = null
  selectedNodeId.value = event.node.id
  void nextTick(() => {
    if (selectedNodes.value.length !== 1) selectedNodeId.value = ''
  })
}

function onNodePointerDown(event: PointerEvent, nodeId: string) {
  const target = event.target as HTMLElement | null
  if (target?.closest('button, input, textarea, select, a, [contenteditable="true"]')) {
    modifierSelectionSnapshot = null
    return
  }
  if (!event.ctrlKey && !event.metaKey) {
    modifierSelectionSnapshot = null
    return
  }
  modifierSelectionSnapshot = new Set(selectedNodes.value.map((node) => node.id))
  if (!modifierSelectionSnapshot.size) {
    const current = nodes.value.find((node) => node.id === nodeId)
    if (current?.selected) modifierSelectionSnapshot.add(nodeId)
  }
}

function eventPoint(event: MouseEvent | TouchEvent): { clientX: number; clientY: number } {
  if (event instanceof MouseEvent) return { clientX: event.clientX, clientY: event.clientY }
  const touch = event.touches[0] || event.changedTouches[0]
  return { clientX: touch?.clientX || 0, clientY: touch?.clientY || 0 }
}

function contextCoordinates(event: MouseEvent | TouchEvent) {
  const point = eventPoint(event)
  const rect = stageEl.value?.getBoundingClientRect()
  return {
    x: point.clientX - (rect?.left || 0),
    y: point.clientY - (rect?.top || 0),
    flowPosition: screenToFlowCoordinate({ x: point.clientX, y: point.clientY }),
  }
}

function onNodeContextMenu(payload: NodeMouseEvent) {
  payload.event.preventDefault()
  const nodeId = payload.node.id
  if (!nodes.value.find((node) => node.id === nodeId)?.selected) selectOnlyNode(nodeId)
  const point = contextCoordinates(payload.event)
  contextMenu.value = { kind: 'node', nodeId, ...point }
  selectedNodeId.value = selectedNodes.value.length === 1 ? nodeId : ''
}

function onEdgeContextMenu(payload: EdgeMouseEvent) {
  payload.event.preventDefault()
  const edgeId = payload.edge.id
  nodes.value = nodes.value.map((node) => ({ ...node, selected: false }))
  edges.value = edges.value.map((edge) => ({ ...edge, selected: edge.id === edgeId }))
  selectedNodeId.value = ''
  const point = contextCoordinates(payload.event)
  contextMenu.value = { kind: 'edge', edgeId, ...point }
}

function onPaneContextMenu(event: MouseEvent) {
  event.preventDefault()
  const point = contextCoordinates(event)
  contextMenu.value = { kind: 'pane', ...point }
}

function onSelectionContextMenu(payload: { event: MouseEvent }) {
  payload.event.preventDefault()
  const point = contextCoordinates(payload.event)
  const nodeId = selectedNodes.value[0]?.id
  contextMenu.value = { kind: nodeId ? 'node' : 'pane', nodeId, ...point }
}

function onPaneClick() {
  clearElementSelection()
  closeContextMenu()
  closeCommandPalette()
  runMenuOpen.value = false
}

function ensureGraphEditable(): boolean {
  if (!graphEditingLocked.value) return true
  showConnectionNotice('任务运行期间已锁定画布结构与参数')
  return false
}

function closeContextMenu() {
  contextMenu.value = null
}

function onStagePointerMove(event: PointerEvent) {
  lastCanvasPointer.value = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
}

function deleteSelectedNode() {
  if (selectedNodeId.value && !selectedNodes.value.some((node) => node.id === selectedNodeId.value)) {
    selectOnlyNode(selectedNodeId.value)
  }
  deleteSelection()
}

function deleteSelection() {
  closeContextMenu()
  if (!ensureGraphEditable()) return
  const nodeIds = new Set(selectedNodes.value.map((node) => node.id))
  const explicitEdgeIds = new Set(selectedEdges.value.map((edge) => edge.id))
  if (!nodeIds.size && !explicitEdgeIds.size) return
  if (selectedNodes.value.some((node) => node.data.status === 'running')) {
    showConnectionNotice('运行中的节点不能删除，请先终止对应任务')
    return
  }
  const removedNodes = cloneValue(nodes.value.filter((node) => nodeIds.has(node.id)))
  const removedEdges = cloneValue(edges.value.filter((edge) => (
    explicitEdgeIds.has(edge.id) || nodeIds.has(edge.source) || nodeIds.has(edge.target)
  )))
  const removedEdgeIds = new Set(removedEdges.map((edge) => edge.id))
  void executeEditorCommand(historyCommand(
    nodeIds.size ? `删除 ${nodeIds.size} 个节点` : `删除 ${removedEdges.length} 条连接`,
    () => {
      nodes.value = nodes.value.filter((node) => !nodeIds.has(node.id))
      edges.value = edges.value.filter((edge) => !removedEdgeIds.has(edge.id))
      selectedNodeId.value = ''
    },
    () => {
      const existingNodeIds = new Set(nodes.value.map((node) => node.id))
      const restoredNodes = removedNodes.filter((node) => !existingNodeIds.has(node.id))
      nodes.value = [...nodes.value, ...cloneValue(restoredNodes)]
      const existingEdgeIds = new Set(edges.value.map((edge) => edge.id))
      edges.value = [...edges.value, ...cloneValue(removedEdges.filter((edge) => !existingEdgeIds.has(edge.id)))]
      selectOnlyNodes(restoredNodes.map((node) => node.id))
    },
  ))
}

function deleteContextEdge() {
  if (!ensureGraphEditable()) return
  const edgeId = contextMenu.value?.edgeId
  const edge = edges.value.find((item) => item.id === edgeId)
  if (!edge) return
  const snapshot = cloneValue(edge)
  closeContextMenu()
  void executeEditorCommand(historyCommand(
    '删除连接',
    () => {
      edges.value = edges.value.filter((item) => item.id !== snapshot.id)
    },
    () => {
      if (!edges.value.some((item) => item.id === snapshot.id)) edges.value = [...edges.value, cloneValue(snapshot)]
    },
  ))
}

function onNodeDragStart(event: NodeDragEvent) {
  dragStartPositions = Object.fromEntries(event.nodes.map((node) => [node.id, { ...node.position }]))
  closeContextMenu()
}

function applyNodePositions(positions: Record<string, XYPosition>) {
  nodes.value = nodes.value.map((node) => positions[node.id]
    ? { ...node, position: { ...positions[node.id] } }
    : node)
}

function onNodeDragStop(event: NodeDragEvent) {
  if (!dragStartPositions) return
  const before = dragStartPositions
  dragStartPositions = null
  const after = Object.fromEntries(event.nodes.map((node) => [node.id, { ...node.position }]))
  const changed = Object.keys(after).some((id) => before[id]
    && (before[id].x !== after[id].x || before[id].y !== after[id].y))
  if (!changed) return
  void recordEditorCommand(historyCommand(
    `移动 ${Object.keys(after).length} 个节点`,
    () => applyNodePositions(after),
    () => applyNodePositions(before),
  ))
}

function onEdgeUpdate(payload: EdgeUpdateEvent) {
  if (!ensureGraphEditable()) return
  const previous = edges.value.find((edge) => edge.id === payload.edge.id)
  if (!previous || !payload.connection.source || !payload.connection.target) return
  if (!validateConnection(payload.connection, previous.id)) {
    const cyclic = wouldCreateCycle(
      nodes.value,
      edges.value,
      String(payload.connection.source),
      String(payload.connection.target),
      previous.id,
    )
    showConnectionNotice(cyclic ? '重连会形成循环，已保留原连接' : '端口类型不兼容，已保留原连接')
    return
  }
  const before = cloneValue(previous)
  const after = makeEdge(
    String(payload.connection.source),
    String(payload.connection.target),
    nodes.value,
    previous.id,
  )
  after.selected = true
  void executeEditorCommand(historyCommand(
    '重连节点',
    () => {
      edges.value = edges.value.map((edge) => edge.id === after.id ? cloneValue(after) : edge)
    },
    () => {
      edges.value = edges.value.map((edge) => edge.id === before.id ? cloneValue(before) : edge)
    },
  ))
}

function updateSelected(patch: Partial<WorkflowNodeData>) {
  if (!ensureGraphEditable()) return
  const nodeId = selectedNode.value?.id || selectedNodeId.value
  const node = nodes.value.find((item) => item.id === nodeId)
  if (!node) return
  if (pendingNodeEdit && pendingNodeEdit.nodeId !== nodeId) void flushPendingNodeEdit()
  if (!pendingNodeEdit) {
    pendingNodeEdit = { nodeId, before: {}, after: {}, timer: null }
  }
  for (const key of Object.keys(patch) as Array<keyof WorkflowNodeData>) {
    if (!(key in pendingNodeEdit.before)) pendingNodeEdit.before[key] = cloneValue(node.data[key]) as never
    pendingNodeEdit.after[key] = cloneValue(patch[key]) as never
  }
  patchNode(nodeId, patch)
  if (pendingNodeEdit.timer) clearTimeout(pendingNodeEdit.timer)
  pendingNodeEdit.timer = setTimeout(() => void flushPendingNodeEdit(), 650)
}

async function flushPendingNodeEdit() {
  const edit = pendingNodeEdit
  pendingNodeEdit = null
  if (!edit) return
  if (edit.timer) clearTimeout(edit.timer)
  const changed = Object.keys(edit.after).some((key) => !Object.is(
    edit.before[key as keyof WorkflowNodeData],
    edit.after[key as keyof WorkflowNodeData],
  ))
  if (!changed) return
  await editorHistory.record(historyCommand(
    '修改节点参数',
    () => patchNode(edit.nodeId, cloneValue(edit.after)),
    () => patchNode(edit.nodeId, cloneValue(edit.before)),
  ))
}

function patchNode(id: string, patch: Partial<WorkflowNodeData>) {
  if (disposed) return
  const index = nodes.value.findIndex((node) => node.id === id)
  if (index < 0) return
  const current = nodes.value[index]
  if (Object.entries(patch).every(([key, value]) => Object.is(current.data[key as keyof WorkflowNodeData], value))) return
  const next = [...nodes.value]
  next[index] = { ...current, data: { ...current.data, ...patch } }
  nodes.value = next
  scheduleSave()
}

function inputValue(event: Event): string {
  return String((event.target as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement)?.value || '')
}

function checkedValue(event: Event): boolean {
  return Boolean((event.target as HTMLInputElement)?.checked)
}

function incomingNodes(nodeId: string): WorkflowNode[] {
  const ids = edges.value.filter((edge) => edge.target === nodeId).map((edge) => edge.source)
  return ids.map((id) => nodes.value.find((node) => node.id === id)).filter(Boolean) as WorkflowNode[]
}

function incomingCount(nodeId: string): number {
  return edges.value.filter((edge) => edge.target === nodeId).length
}

function resolvedPrompt(node: WorkflowNode): string {
  const upstream = incomingNodes(node.id)
    .filter((item) => item.data.kind === 'prompt')
    .map((item) => String(item.data.prompt || '').trim())
    .filter(Boolean)
  const own = String(node.data.prompt || '').trim()
  return [...upstream, own].filter(Boolean).join('\n')
}

function resolvedPromptForNode(nodeId: string): string {
  const node = nodes.value.find((item) => item.id === nodeId)
  return node ? resolvedPrompt(node) : ''
}

function incomingPromptNode(nodeId: string): WorkflowNode | undefined {
  return incomingNodes(nodeId).find((node) => node.data.kind === 'prompt')
}

function hasIncomingPrompt(nodeId: string): boolean {
  return Boolean(incomingPromptNode(nodeId))
}

function focusIncomingPrompt(nodeId: string) {
  const prompt = incomingPromptNode(nodeId)
  if (prompt) focusLibraryAsset(prompt.id)
}

function incomingMediaCount(nodeId: string): number {
  return incomingNodes(nodeId).filter((node) => Boolean(resolvedMediaKind(node.data))).length
}

function resolvedMediaKind(data: WorkflowNodeData): MediaKind | '' {
  if (data.kind === 'asset' || data.kind === 'result') return data.mediaKind || ''
  if (data.kind === 'image' && data.outputUrl) return 'image'
  if (data.kind === 'video' && data.outputUrl) return 'video'
  return ''
}

function resolvedMediaUrl(data: WorkflowNodeData): string {
  return String(data.mediaUrl || data.outputUrl || '')
}

function incomingMedia(nodeId: string): Array<{ node: WorkflowNode; kind: MediaKind; url: string }> {
  return incomingNodes(nodeId)
    .map((node) => ({ node, kind: resolvedMediaKind(node.data), url: resolvedMediaUrl(node.data) }))
    .filter((item): item is { node: WorkflowNode; kind: MediaKind; url: string } => Boolean(item.kind && item.url))
}

function mediaFilename(kind: MediaKind, mimeType: string): string {
  const normalized = String(mimeType || '').toLowerCase()
  const extension = normalized.includes('webp')
    ? 'webp'
    : normalized.includes('jpeg') || normalized.includes('jpg')
      ? 'jpg'
      : normalized.includes('gif')
        ? 'gif'
        : normalized.includes('webm')
          ? 'webm'
          : normalized.includes('wav')
            ? 'wav'
            : normalized.includes('mpeg')
              ? kind === 'audio' ? 'mp3' : 'mp4'
              : kind === 'image' ? 'png' : kind === 'video' ? 'mp4' : 'bin'
  return `canvas-${kind}-${Date.now()}.${extension}`
}

async function readMediaBlob(source: string, workId?: number): Promise<Blob> {
  const candidates = [source]
  if (workId) {
    try {
      const contentUrl = await getWorkContentBlob(workId)
      if (contentUrl.startsWith('blob:')) localObjectUrls.add(contentUrl)
      if (contentUrl && !candidates.includes(contentUrl)) candidates.push(contentUrl)
    } catch {
      // The original generation URL may still be available even if the work proxy is not.
    }
  }
  let lastError: unknown
  for (const candidate of candidates) {
    try {
      const response = await fetch(candidate, { cache: 'no-store' })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const blob = await response.blob()
      if (!blob.size) throw new Error('媒体内容为空')
      return blob
    } catch (error) {
      lastError = error
    }
  }
  throw lastError || new Error('无法读取媒体内容')
}

function patchMaterializedSource(node: WorkflowNode, mediaUrl: string) {
  if (node.data.kind === 'asset' || node.data.kind === 'result') {
    patchNode(node.id, { mediaUrl, previewUrl: mediaUrl, upstreamReady: true })
    return
  }
  patchNode(node.id, { outputUrl: mediaUrl, upstreamReady: true })
}

async function prepareVideoInputMedia(
  items: Array<{ node: WorkflowNode; kind: MediaKind; url: string }>,
): Promise<Array<{ node: WorkflowNode; kind: MediaKind; url: string }>> {
  const prepared: Array<{ node: WorkflowNode; kind: MediaKind; url: string }> = []
  let preparationError = ''
  for (const item of items) {
    const workId = Number(item.node.data.workId || 0)
    if (!workId || item.node.data.upstreamReady) {
      prepared.push(item)
      continue
    }
    const cached = materializedMediaByWork.get(workId)
    if (cached) {
      patchMaterializedSource(item.node, cached)
      prepared.push({ ...item, url: cached })
      continue
    }
    try {
      const blob = await readMediaBlob(item.url, workId)
      const uploaded = await uploadVideoAsset(
        props.apiKeySecret,
        blob,
        mediaFilename(item.kind, blob.type),
        item.kind,
      )
      const mediaUrl = String(uploaded.media_url || uploaded.url || '')
      if (!mediaUrl) throw new Error('媒体转存接口没有返回地址')
      materializedMediaByWork.set(workId, mediaUrl)
      patchMaterializedSource(item.node, mediaUrl)
      prepared.push({ ...item, url: mediaUrl })
    } catch (error) {
      preparationError ||= `无法准备${mediaKindLabel(item.kind)}输入：${errorMessage(error)}`
    }
  }
  if (items.length && !prepared.length && preparationError) throw new Error(preparationError)
  return prepared
}

function nodeIcon(data: WorkflowNodeData): 'upload' | 'document' | 'sparkles' | 'play' | 'checkCircle' {
  if (data.kind === 'asset') return 'upload'
  if (data.kind === 'prompt') return 'document'
  if (data.kind === 'image') return 'sparkles'
  if (data.kind === 'video') return 'play'
  return 'checkCircle'
}

function nodeTypeLabel(data: WorkflowNodeData): string {
  if (data.kind === 'asset') return 'ASSET'
  if (data.kind === 'prompt') return 'PROMPT'
  if (data.kind === 'image') return 'IMAGE'
  if (data.kind === 'video') return 'VIDEO'
  return 'OUTPUT'
}

function statusLabel(status?: NodeStatus): string {
  if (status === 'uploading') return '正在上传'
  if (status === 'running') return '正在生成'
  if (status === 'succeeded') return '已完成'
  if (status === 'failed') return '失败'
  if (status === 'canceled') return '已终止'
  return '待运行'
}

function statusShortLabel(status?: NodeStatus): string {
  if (status === 'uploading') return '上传'
  if (status === 'running') return '运行'
  if (status === 'succeeded') return '完成'
  if (status === 'failed') return '失败'
  if (status === 'canceled') return '终止'
  return '就绪'
}

function mediaKindLabel(kind?: MediaKind): string {
  if (kind === 'video') return '视频'
  if (kind === 'audio') return '音频'
  return '图片'
}

function mediaPreviewUrl(data: WorkflowNodeData): string {
  return String(data.previewUrl || data.mediaUrl || data.outputUrl || '')
}

function playableMediaUrl(data: WorkflowNodeData): string {
  const playable = String(data.playableUrl || '')
  if (playable) return playable
  const media = String(data.mediaUrl || data.outputUrl || '')
  return /^https?:\/\//i.test(media) ? media : ''
}

function miniMapNodeColor(node: Node): string {
  const kind = (node.data as WorkflowNodeData | undefined)?.kind
  if (kind === 'asset') return '#0f766e'
  if (kind === 'prompt') return '#b45309'
  if (kind === 'image') return '#2563eb'
  if (kind === 'video') return '#e05252'
  return '#059669'
}

function openAssetPicker() {
  if (!apiReady.value) {
    showConnectionNotice('请先选择可用的 API Key')
    return
  }
  assetInput.value?.click()
}

async function onAssetInput(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  await uploadFiles(files)
}

function onStageDragLeave(event: DragEvent) {
  const current = event.currentTarget as HTMLElement | null
  if (current && event.relatedTarget instanceof globalThis.Node && current.contains(event.relatedTarget)) return
  draggingNodeKind.value = ''
  draggingFile.value = false
}

function onStageDragEnter(event: DragEvent) {
  const kind = event.dataTransfer?.getData('application/x-creazy-node') as PaletteNodeKind | ''
  draggingNodeKind.value = kind === 'prompt' || kind === 'image' || kind === 'video' ? kind : ''
  draggingFile.value = true
}

function onStageDragOver(event: DragEvent) {
  const kind = event.dataTransfer?.getData('application/x-creazy-node') as PaletteNodeKind | ''
  if (kind === 'prompt' || kind === 'image' || kind === 'video') draggingNodeKind.value = kind
  draggingFile.value = true
}

async function onStageDrop(event: DragEvent) {
  const transferredKind = event.dataTransfer?.getData('application/x-creazy-node') as PaletteNodeKind | ''
  const paletteKind = transferredKind || draggingNodeKind.value
  const position = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
  if (paletteKind === 'prompt' || paletteKind === 'image' || paletteKind === 'video') {
    draggingNodeKind.value = ''
    draggingFile.value = false
    addNode(paletteKind, position)
    return
  }
  draggingNodeKind.value = ''
  draggingFile.value = false
  const files = Array.from(event.dataTransfer?.files || [])
  if (!files.length) return
  await uploadFiles(files, position)
}

async function uploadFiles(files: File[], origin?: XYPosition) {
  if (!ensureGraphEditable()) return
  if (!apiReady.value) {
    showConnectionNotice('请先选择可用的 API Key')
    return
  }
  for (let index = 0; index < files.length; index += 1) {
    const file = files[index]
    const mediaKind = file.type.startsWith('video/') ? 'video' : file.type.startsWith('audio/') ? 'audio' : 'image'
    const localUrl = URL.createObjectURL(file)
    localObjectUrls.add(localUrl)
    const position = origin
      ? { x: origin.x + index * 36, y: origin.y + index * 36 }
      : nextPosition()
    const node = createNode('asset', position)
    node.data = {
      kind: 'asset',
      title: file.name,
      name: file.name,
      status: 'uploading',
      mediaKind,
      mediaUrl: '',
      previewUrl: localUrl,
      playableUrl: mediaKind === 'video' || mediaKind === 'audio' ? localUrl : '',
      mimeType: file.type,
      durationSeconds: mediaKind === 'image' ? undefined : await readMediaDuration(file),
    }
    await executeEditorCommand(historyCommand(
      '添加上传资产',
      () => {
        if (!nodes.value.some((item) => item.id === node.id)) nodes.value = [...nodes.value, cloneValue(node)]
        selectOnlyNode(node.id)
      },
      () => {
        nodes.value = nodes.value.filter((item) => item.id !== node.id)
        edges.value = edges.value.filter((edge) => edge.source !== node.id && edge.target !== node.id)
      },
    ))
    try {
      const uploaded = await uploadVideoAsset(props.apiKeySecret, file, file.name, mediaKind)
      const mediaUrl = String(uploaded.media_url || uploaded.url || '')
      if (!mediaUrl) throw new Error('上传接口没有返回媒体地址')
      Object.assign(node.data, { status: 'succeeded' as const, mediaUrl, error: '' })
      patchNode(node.id, { status: 'succeeded', mediaUrl, error: '' })
    } catch (error) {
      const message = errorMessage(error)
      Object.assign(node.data, { status: 'failed' as const, error: message })
      patchNode(node.id, { status: 'failed', error: message })
    }
  }
}

async function readMediaDuration(file: File): Promise<number> {
  const url = URL.createObjectURL(file)
  try {
    const element = document.createElement(file.type.startsWith('audio/') ? 'audio' : 'video')
    element.preload = 'metadata'
    element.src = url
    await new Promise<void>((resolve, reject) => {
      element.onloadedmetadata = () => resolve()
      element.onerror = () => reject(new Error('metadata'))
    })
    return Number.isFinite(element.duration) ? Math.round(element.duration * 100) / 100 : 1
  } catch {
    return 1
  } finally {
    URL.revokeObjectURL(url)
  }
}

function onPaletteDragStart(event: DragEvent, kind: PaletteNodeKind) {
  event.dataTransfer?.setData('application/x-creazy-node', kind)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copy'
  draggingNodeKind.value = kind
  draggingFile.value = true
}

function onPaletteDragEnd() {
  draggingNodeKind.value = ''
  draggingFile.value = false
}

async function runNode(nodeId: string): Promise<boolean> {
  const node = nodes.value.find((item) => item.id === nodeId)
  if (!node || (node.data.kind !== 'image' && node.data.kind !== 'video')) return false
  if (node.data.status === 'running' || activeNodeRuns.has(nodeId)) {
    showConnectionNotice('该节点已有任务正在运行')
    return false
  }
  if (!apiReady.value) {
    patchNode(nodeId, { status: 'failed', error: '请先选择可用的 API Key' })
    return false
  }
  activeNodeRuns.add(nodeId)
  activeNodeRunCount.value = activeNodeRuns.size
  try {
    if (node.data.kind === 'image') return await runImageNode(node)
    return await runVideoNode(node)
  } finally {
    activeNodeRuns.delete(nodeId)
    activeNodeRunCount.value = activeNodeRuns.size
  }
}

async function runImageNode(node: WorkflowNode): Promise<boolean> {
  const prompt = resolvedPrompt(node)
  if (!prompt) {
    patchNode(node.id, { status: 'failed', error: '请填写提示词或连接提示词节点' })
    return false
  }
  const model = imageModels.value.find((item) => item.id === node.data.model) || imageModels.value[0]
  if (!model) {
    patchNode(node.id, { status: 'failed', error: '当前 API Key 没有可用图片模型' })
    return false
  }
  const refs = incomingMedia(node.id).filter((item) => item.kind === 'image').map((item) => item.url)
  const maxRefs = Number(model.max_reference_images || 0)
  const usableRefs = maxRefs > 0 ? refs.slice(0, maxRefs) : refs
  if (model.require_reference && !usableRefs.length) {
    patchNode(node.id, { status: 'failed', error: '该模型需要连接至少一张参考图' })
    return false
  }
  const runStartedAt = Date.now()
  const size = node.data.size || resolveImageSize(model, node.data.qualityTier || '1K', node.data.aspectRatio || '1:1')
  patchNode(node.id, { status: 'running', error: '', model: model.id, size })

  let workId = 0
  let gatewayType = model.async ? 'image_task' : 'image_sync'
  let remoteId = ''
  try {
    const work = await createWork({
      api_key_id: props.apiKeyId,
      kind: 'image',
      status: 'running',
      public_model: model.id,
      prompt,
      params: {
        ...buildImageWorkParams({
          size,
          qualityTier: node.data.qualityTier,
          aspectRatio: node.data.aspectRatio,
          refs: usableRefs,
        }),
        canvas_document_id: documentId.value || undefined,
        canvas_node_id: node.id,
        input_node_ids: incomingNodes(node.id).map((item) => item.id),
      },
      gateway_type: gatewayType,
    })
    workId = work.id
    foregroundWorkIds.add(workId)
    emit('workCreated', work.id)
    patchNode(node.id, { workId })
    if (disposed) {
      await updateWork(workId, { status: 'canceled', error_message: '页面在提交前关闭' }).catch(() => undefined)
      return false
    }

    const payload: Record<string, unknown> = { model: model.id, prompt, size, n: 1 }
    if (/^[124]K$/i.test(size)) payload.aspect_ratio = node.data.aspectRatio || '1:1'
    if (usableRefs.length) {
      payload.images = usableRefs.map((url) => ({ image_url: url }))
      payload.reference_images = usableRefs.map((url) => ({ image_url: url }))
    }

    let response: any
    let usedAsync = Boolean(model.async)
    try {
      response = await generateImage(props.apiKeySecret, payload as any, {
        async: usedAsync,
        edit: usableRefs.length > 0,
        workId,
      })
    } catch (error: any) {
      const status = Number(error?.status || 0)
      const message = String(error?.message || '')
      if (!usedAsync || !(status === 404 || /async.*not enabled|model not found|unknown model/i.test(message))) throw error
      usedAsync = false
      gatewayType = 'image_sync'
      response = await generateImage(props.apiKeySecret, payload as any, {
        async: false,
        edit: usableRefs.length > 0,
        workId,
      })
    }
    remoteId = String(response.task_id || response.id || '')
    if (remoteId) {
      gatewayType = 'image_task'
      await updateWork(workId, { gateway_type: gatewayType, gateway_remote_id: remoteId, status: 'running' })
      patchNode(node.id, { gatewayRemoteId: remoteId })
    }
    let urls = extractImageUrls(response)
    if (!urls.length && remoteId) {
      for (let index = 0; index < 90 && !disposed; index += 1) {
        await delay(2000)
        if (index % 5 === 0 && await workWasCanceled(workId)) {
          patchNode(node.id, { status: 'canceled', error: '任务已被管理员终止' })
          return false
        }
        response = await getImageTask(props.apiKeySecret, remoteId)
        urls = extractImageUrls(response)
        if (urls.length || isTerminalImageStatus(response.status)) break
      }
    }
    if (disposed && !urls.length) return false
    const outputUrl = String(urls[0] || '')
    if (!outputUrl) throw new Error(`图片生成失败${response?.status ? `：${response.status}` : ''}`)
    await updateWork(workId, {
      status: 'succeeded',
      public_model: model.id,
      prompt,
      gateway_type: gatewayType,
      gateway_remote_id: remoteId,
      preview_url: outputUrl,
      object_url: outputUrl,
      mime_type: 'image/png',
      error_message: '',
      params: {
        ...buildImageWorkParams({
          size,
          qualityTier: node.data.qualityTier,
          aspectRatio: node.data.aspectRatio,
          refs: usableRefs,
          resultUrls: urls,
        }),
        canvas_document_id: documentId.value || undefined,
        canvas_node_id: node.id,
        input_node_ids: incomingNodes(node.id).map((item) => item.id),
      },
    })
    if (disposed) return true
    const liveNode = nodes.value.find((item) => item.id === node.id)
    if (!liveNode || liveNode.data.workId !== workId) return true
    patchNode(liveNode.id, {
      status: 'succeeded', outputUrl, workId, gatewayRemoteId: remoteId, error: '',
      lastRunDurationMs: Date.now() - runStartedAt,
      runCount: Number(liveNode.data.runCount || 0) + 1,
    })
    addResultNode(liveNode, { mediaKind: 'image', mediaUrl: outputUrl, previewUrl: outputUrl, workId, name: `${liveNode.data.title} 结果` })
    await saveDocument(false)
    return true
  } catch (error) {
    if (disposed) return false
    if (workId && await workWasCanceled(workId)) {
      patchNode(node.id, { status: 'canceled', error: '任务已被管理员终止', workId, gatewayRemoteId: remoteId || undefined })
      return false
    }
    const message = errorMessage(error)
    patchNode(node.id, { status: 'failed', error: message, workId: workId || undefined, gatewayRemoteId: remoteId || undefined, lastRunDurationMs: Date.now() - runStartedAt })
    if (workId) {
      await updateWork(workId, { status: 'failed', gateway_type: gatewayType, gateway_remote_id: remoteId, error_message: message }).catch(() => undefined)
    }
    return false
  } finally {
    if (workId) foregroundWorkIds.delete(workId)
  }
}

async function runVideoNode(node: WorkflowNode): Promise<boolean> {
  const prompt = resolvedPrompt(node)
  if (!prompt) {
    patchNode(node.id, { status: 'failed', error: '请填写提示词或连接提示词节点' })
    return false
  }
  const model = videoModels.value.find((item) => item.id === node.data.model) || videoModels.value[0]
  if (!model) {
    patchNode(node.id, { status: 'failed', error: '当前 API Key 没有可用视频模型' })
    return false
  }
  const runStartedAt = Date.now()
  patchNode(node.id, { status: 'running', error: '', model: model.id })
  let media: Array<{ node: WorkflowNode; kind: MediaKind; url: string }>
  try {
    media = await prepareVideoInputMedia(incomingMedia(node.id))
  } catch (error) {
    patchNode(node.id, { status: 'failed', error: errorMessage(error) })
    return false
  }
  if (disposed) return false
  const images = media.filter((item) => item.kind === 'image')
  const videos = media.filter((item) => item.kind === 'video')
  const audios = media.filter((item) => item.kind === 'audio')
  const allowStart = Boolean(model.allow_start_frame || model.require_start_frame)
  const startFrame = allowStart ? images[0]?.url || '' : ''
  let imageRefs = allowStart ? images.slice(1) : images
  if (model.frames_exclusive_with_refs && startFrame) imageRefs = []
  if (model.require_start_frame && !startFrame) {
    patchNode(node.id, { status: 'failed', error: '该模型需要连接一张图片作为首帧' })
    return false
  }
  if (model.max_image_references) imageRefs = imageRefs.slice(0, model.max_image_references)
  const videoRefs = model.max_video_references ? videos.slice(0, model.max_video_references) : videos
  const audioRefs = model.max_audio_references ? audios.slice(0, model.max_audio_references) : audios
  if (model.audio_requires_image_refs && audioRefs.length && !imageRefs.length) {
    patchNode(node.id, { status: 'failed', error: '该模型使用参考音频时还需要参考图' })
    return false
  }

  const resolution = node.data.resolution || model.default_resolution || '720p'
  const duration = Number(node.data.duration || model.default_duration || 5)
  const aspectRatio = node.data.aspectRatio || model.aspect_ratios?.[0] || '16:9'
  const generateAudio = Boolean(model.force_generated_audio || audioRefs.length || (model.allow_generated_audio && node.data.generateAudio))
  patchNode(node.id, { status: 'running', error: '', model: model.id, resolution, duration, aspectRatio, generateAudio })

  let workId = 0
  let remoteId = ''
  try {
    const baseParams = buildVideoWorkParams({
      resolution,
      duration,
      aspectRatio,
      generateAudio,
      startFrame,
      refImages: imageRefs.map((item) => item.url),
      refVideos: videoRefs.map((item) => item.url),
      refAudios: audioRefs.map((item) => item.url),
      extra: {
        canvas_document_id: documentId.value || undefined,
        canvas_node_id: node.id,
        input_node_ids: incomingNodes(node.id).map((item) => item.id),
      },
    })
    const work = await createWork({
      api_key_id: props.apiKeyId,
      kind: 'video',
      status: 'running',
      public_model: model.id,
      prompt,
      params: baseParams,
      gateway_type: 'video_job',
    })
    workId = work.id
    foregroundWorkIds.add(workId)
    emit('workCreated', work.id)
    patchNode(node.id, { workId })
    if (disposed) {
      await updateWork(workId, { status: 'canceled', error_message: '页面在提交前关闭' }).catch(() => undefined)
      return false
    }

    const payload: Record<string, unknown> = { model: model.id, prompt, resolution, duration, aspect_ratio: aspectRatio }
    if (generateAudio) payload.audio = true
    if (startFrame) payload.start_frame_url = startFrame
    const guidances: Record<string, unknown> = {}
    if (imageRefs.length) {
      guidances.image_reference = imageRefs.map((item, index) => ({
        image: { url: item.url, type: 'UPLOADED' },
        strength: 'MID',
        order: index,
      }))
    }
    if (videoRefs.length) {
      guidances.video_reference_base = videoRefs.map((item) => ({
        video: { url: item.url, type: 'UPLOADED', duration_seconds: item.node.data.durationSeconds || 1 },
      }))
    }
    if (audioRefs.length) {
      guidances.audio_reference = audioRefs.map((item) => ({
        audio: { url: item.url, type: 'UPLOADED', duration_seconds: item.node.data.durationSeconds || 1 },
      }))
    }
    if (Object.keys(guidances).length) payload.guidances = guidances

    let job: any = await generateVideo(props.apiKeySecret, payload as any, { workId })
    remoteId = String(job.id || job.job_id || '')
    if (remoteId) {
      await updateWork(workId, { status: 'running', gateway_type: 'video_job', gateway_remote_id: remoteId })
      patchNode(node.id, { gatewayRemoteId: remoteId })
      for (let index = 0; index < 150 && !disposed; index += 1) {
        if (isTerminalVideoStatus(job.status)) break
        await delay(3000)
        if (index % 4 === 0 && await workWasCanceled(workId)) {
          patchNode(node.id, { status: 'canceled', error: '任务已被管理员终止' })
          return false
        }
        job = await getVideoJob(props.apiKeySecret, remoteId)
      }
    }
    if (disposed && !isSuccessfulVideoStatus(job.status)) return false
    if (!isSuccessfulVideoStatus(job.status)) {
      const detail = typeof job.error === 'string' ? job.error : job.error?.message
      throw new Error(detail || `视频生成失败：${job.status || 'unknown'}`)
    }
    const persistentUrl = extractVideoUrl(job) || (remoteId ? `/v1/videos/jobs/${remoteId}/content` : '')
    if (!persistentUrl) throw new Error('视频生成成功但没有返回内容地址')
    await updateWork(workId, {
      status: 'succeeded',
      public_model: model.id,
      prompt,
      params: { ...baseParams, result_urls: [persistentUrl], poster_url: startFrame || undefined },
      gateway_type: 'video_job',
      gateway_remote_id: remoteId,
      preview_url: startFrame || persistentUrl,
      object_url: persistentUrl,
      mime_type: 'video/mp4',
      error_message: '',
    })
    if (disposed) return true
    let playableUrl = ''
    try {
      playableUrl = remoteId ? await getVideoContentURL(props.apiKeySecret, remoteId) : ''
      if (playableUrl.startsWith('blob:')) localObjectUrls.add(playableUrl)
    } catch {
      playableUrl = /^https?:\/\//i.test(persistentUrl) ? persistentUrl : ''
    }
    const liveNode = nodes.value.find((item) => item.id === node.id)
    if (!liveNode || liveNode.data.workId !== workId) return true
    patchNode(liveNode.id, {
      status: 'succeeded', outputUrl: persistentUrl, playableUrl, workId, gatewayRemoteId: remoteId, error: '',
      lastRunDurationMs: Date.now() - runStartedAt,
      runCount: Number(liveNode.data.runCount || 0) + 1,
    })
    addResultNode(liveNode, {
      mediaKind: 'video',
      mediaUrl: persistentUrl,
      playableUrl,
      previewUrl: startFrame,
      workId,
      name: `${liveNode.data.title} 结果`,
    })
    await saveDocument(false)
    return true
  } catch (error) {
    if (disposed) return false
    if (workId && await workWasCanceled(workId)) {
      patchNode(node.id, { status: 'canceled', error: '任务已被管理员终止', workId, gatewayRemoteId: remoteId || undefined })
      return false
    }
    const message = errorMessage(error)
    patchNode(node.id, { status: 'failed', error: message, workId: workId || undefined, gatewayRemoteId: remoteId || undefined, lastRunDurationMs: Date.now() - runStartedAt })
    if (workId) {
      await updateWork(workId, { status: 'failed', gateway_type: 'video_job', gateway_remote_id: remoteId, error_message: message }).catch(() => undefined)
    }
    return false
  } finally {
    if (workId) foregroundWorkIds.delete(workId)
  }
}

function addResultNode(
  source: WorkflowNode,
  result: Partial<WorkflowNodeData> & { mediaKind: MediaKind; mediaUrl: string },
  select = true,
) {
  if (disposed || !nodes.value.some((node) => node.id === source.id)) return
  const existing = nodes.value.find((item) => item.data.kind === 'result' && item.data.workId === result.workId)
  if (existing) {
    patchNode(existing.id, {
      title: result.name || existing.data.title,
      name: result.name || existing.data.name,
      status: 'succeeded',
      mediaKind: result.mediaKind,
      mediaUrl: result.mediaUrl,
      previewUrl: result.previewUrl,
      playableUrl: result.playableUrl,
      sourceNodeId: source.id,
      runCount: result.runCount || source.data.runCount,
      error: '',
    })
    if (existing.id !== source.id && !edges.value.some((edge) => edge.source === source.id && edge.target === existing.id)) {
      edges.value.push(makeEdge(source.id, existing.id))
      scheduleSave()
    }
    if (select) selectOnlyNode(existing.id)
    return
  }
  const node = createNode('result', { x: source.position.x + 440, y: source.position.y + 188 })
  node.data = {
    kind: 'result',
    title: result.name || '生成结果',
    name: result.name || '生成结果',
    status: 'succeeded',
    mediaKind: result.mediaKind,
    mediaUrl: result.mediaUrl,
    previewUrl: result.previewUrl,
    playableUrl: result.playableUrl,
    workId: result.workId,
    sourceNodeId: source.id,
    runCount: result.runCount || source.data.runCount,
  }
  nodes.value.push(node)
  edges.value.push(makeEdge(source.id, node.id))
  if (select) selectOnlyNode(node.id)
  scheduleSave()
}

function extractImageUrls(payload: any): string[] {
  const urls: string[] = []
  if (Array.isArray(payload?.data)) {
    payload.data.forEach((item: any) => {
      if (item?.url) urls.push(String(item.url))
      else if (item?.b64_json) urls.push(`data:image/png;base64,${item.b64_json}`)
    })
  }
  if (payload?.url) urls.push(String(payload.url))
  if (payload?.result_url) urls.push(String(payload.result_url))
  if (Array.isArray(payload?.result_urls)) urls.push(...payload.result_urls.map(String))
  return [...new Set(urls.filter(Boolean))]
}

function extractVideoUrl(job: any): string {
  return String(
    job?.video_url || job?.content_url || job?.download_url || job?.url ||
    job?.result?.url || job?.result?.content_url || job?.result?.video_url ||
    job?.result?.data?.[0]?.url || job?.result?.data?.[0]?.mp4_url || '',
  )
}

function isTerminalImageStatus(status?: string): boolean {
  return ['succeeded', 'completed', 'success', 'failed', 'error', 'cancelled', 'canceled'].includes(String(status || '').toLowerCase())
}

function isTerminalVideoStatus(status?: string): boolean {
  return ['succeeded', 'completed', 'success', 'finished', 'done', 'complete', 'failed', 'error', 'cancelled', 'canceled'].includes(String(status || '').toLowerCase())
}

function isSuccessfulVideoStatus(status?: string): boolean {
  return ['succeeded', 'completed', 'success', 'finished', 'done', 'complete'].includes(String(status || '').toLowerCase())
}

function remoteFailureMessage(payload: any, fallback: string): string {
  if (typeof payload?.error === 'string') return payload.error
  return String(payload?.error?.message || payload?.message || fallback)
}

function workResultUrls(work: CreazyWork): string[] {
  const values = Array.isArray(work.params?.result_urls) ? work.params.result_urls.map(String) : []
  return [...new Set([String(work.object_url || ''), ...values, String(work.preview_url || '')].filter(Boolean))]
}

async function completeRestoredImageWork(
  node: WorkflowNode,
  work: CreazyWork,
  task: ImageGenerationResponse,
  remoteId: string,
) {
  const urls = extractImageUrls(task)
  const outputUrl = String(urls[0] || '')
  if (!outputUrl) throw new Error('图片任务已完成，但没有返回结果地址')
  await updateWork(work.id, {
    status: 'succeeded',
    gateway_type: work.gateway_type,
    gateway_remote_id: remoteId,
    object_url: outputUrl,
    preview_url: outputUrl,
    mime_type: work.mime_type || 'image/png',
    error_message: '',
    params: { ...(work.params || {}), result_urls: urls },
  })
  const currentNode = nodes.value.find((item) => item.id === node.id)
  if (disposed || !currentNode || currentNode.data.workId !== work.id) return
  patchNode(currentNode.id, {
    status: 'succeeded',
    outputUrl,
    workId: work.id,
    gatewayRemoteId: remoteId,
    error: '',
  })
  addResultNode(currentNode, {
    mediaKind: 'image',
    mediaUrl: outputUrl,
    previewUrl: outputUrl,
    workId: work.id,
    name: `${node.data.title} 结果`,
  }, false)
}

async function completeRestoredVideoWork(node: WorkflowNode, work: CreazyWork, job: VideoJob, remoteId: string) {
  const persistentUrl = extractVideoUrl(job) || (remoteId ? `/v1/videos/jobs/${remoteId}/content` : '')
  if (!persistentUrl) throw new Error('视频任务已完成，但没有返回结果地址')
  const posterUrl = String(work.params?.poster_url || work.params?.start_frame || '')
  await updateWork(work.id, {
    status: 'succeeded',
    gateway_type: work.gateway_type,
    gateway_remote_id: remoteId,
    object_url: persistentUrl,
    preview_url: posterUrl || persistentUrl,
    mime_type: work.mime_type || 'video/mp4',
    error_message: '',
    params: { ...(work.params || {}), result_urls: [persistentUrl], poster_url: posterUrl || undefined },
  })
  const currentNode = nodes.value.find((item) => item.id === node.id)
  if (disposed || !currentNode || currentNode.data.workId !== work.id) return
  let playableUrl = ''
  try {
    playableUrl = remoteId ? await getVideoContentURL(props.apiKeySecret, remoteId) : ''
    if (playableUrl.startsWith('blob:')) localObjectUrls.add(playableUrl)
  } catch {
    playableUrl = /^https?:\/\//i.test(persistentUrl) ? persistentUrl : ''
  }
  patchNode(currentNode.id, {
    status: 'succeeded',
    outputUrl: persistentUrl,
    playableUrl,
    workId: work.id,
    gatewayRemoteId: remoteId,
    error: '',
  })
  addResultNode(currentNode, {
    mediaKind: 'video',
    mediaUrl: persistentUrl,
    playableUrl,
    previewUrl: posterUrl,
    workId: work.id,
    name: `${node.data.title} 结果`,
  }, false)
}

async function resumeRemoteWork(node: WorkflowNode, work: CreazyWork) {
  const remoteId = String(work.gateway_remote_id || node.data.gatewayRemoteId || '')
  if (
    !remoteId || !apiReady.value || work.api_key_id !== props.apiKeyId ||
    foregroundWorkIds.has(work.id) || activeRemotePolls.has(work.id)
  ) return
  activeRemotePolls.add(work.id)
  try {
    if (node.data.kind === 'image' && work.gateway_type === 'image_task') {
      const task = await getImageTask(props.apiKeySecret, remoteId)
      if (!isTerminalImageStatus(task.status)) return
      const liveNode = nodes.value.find((item) => item.id === node.id)
      if (disposed || !liveNode || liveNode.data.workId !== work.id) return
      if (['cancelled', 'canceled'].includes(String(task.status || '').toLowerCase())) {
        patchNode(liveNode.id, { status: 'canceled', error: '任务已终止' })
        await updateWork(work.id, { status: 'canceled', error_message: '任务已终止' })
        return
      }
      const urls = extractImageUrls(task)
      if (!urls.length) {
        const message = remoteFailureMessage(task, `图片生成失败：${task.status || 'unknown'}`)
        patchNode(liveNode.id, { status: 'failed', error: message })
        await updateWork(work.id, { status: 'failed', error_message: message })
        return
      }
      if (await workWasCanceled(work.id)) {
        patchNode(liveNode.id, { status: 'canceled', error: '任务已被管理员终止' })
        return
      }
      await completeRestoredImageWork(liveNode, work, task, remoteId)
      return
    }
    if (node.data.kind === 'video' && work.gateway_type === 'video_job') {
      const job = await getVideoJob(props.apiKeySecret, remoteId)
      if (!isTerminalVideoStatus(job.status)) return
      const liveNode = nodes.value.find((item) => item.id === node.id)
      if (disposed || !liveNode || liveNode.data.workId !== work.id) return
      if (['cancelled', 'canceled'].includes(String(job.status || '').toLowerCase())) {
        patchNode(liveNode.id, { status: 'canceled', error: '任务已终止' })
        await updateWork(work.id, { status: 'canceled', error_message: '任务已终止' })
        return
      }
      if (!isSuccessfulVideoStatus(job.status)) {
        const message = remoteFailureMessage(job, `视频生成失败：${job.status || 'unknown'}`)
        patchNode(liveNode.id, { status: 'failed', error: message })
        await updateWork(work.id, { status: 'failed', error_message: message })
        return
      }
      if (await workWasCanceled(work.id)) {
        patchNode(liveNode.id, { status: 'canceled', error: '任务已被管理员终止' })
        return
      }
      await completeRestoredVideoWork(liveNode, work, job, remoteId)
    }
  } catch (error) {
    const liveNode = nodes.value.find((item) => item.id === node.id)
    if (disposed || !liveNode || liveNode.data.workId !== work.id) return
    if (await workWasCanceled(work.id)) {
      patchNode(liveNode.id, { status: 'canceled', error: '任务已被管理员终止' })
    } else {
      patchNode(liveNode.id, { status: 'running', error: `状态同步暂时失败，将自动重试：${errorMessage(error)}` })
    }
  } finally {
    activeRemotePolls.delete(work.id)
  }
}

async function restoreSucceededWork(node: WorkflowNode, work: CreazyWork): Promise<boolean> {
  const currentBeforeRestore = nodes.value.find((item) => item.id === node.id)
  if (disposed || !currentBeforeRestore || currentBeforeRestore.data.workId !== work.id) return false
  const urls = workResultUrls(work)
  const outputUrl = String(urls[0] || '')
  if (!outputUrl) return false
  let playableUrl = node.data.playableUrl || ''
  if (work.kind === 'video' && !playableUrl && work.gateway_remote_id && apiReady.value && work.api_key_id === props.apiKeyId) {
    try {
      playableUrl = await getVideoContentURL(props.apiKeySecret, work.gateway_remote_id)
      if (playableUrl.startsWith('blob:')) localObjectUrls.add(playableUrl)
    } catch {
      playableUrl = /^https?:\/\//i.test(outputUrl) ? outputUrl : ''
    }
  }
  const currentNode = nodes.value.find((item) => item.id === node.id)
  if (disposed || !currentNode || currentNode.data.workId !== work.id) return false
  patchNode(currentNode.id, { status: 'succeeded', outputUrl, playableUrl, error: '' })
  addResultNode(currentNode, {
    mediaKind: work.kind === 'video' ? 'video' : 'image',
    mediaUrl: outputUrl,
    playableUrl,
    previewUrl: String(work.preview_url || ''),
    workId: work.id,
    name: `${currentNode.data.title} 结果`,
  }, false)
  return true
}

async function workWasCanceled(workId: number): Promise<boolean> {
  if (!workId) return false
  try {
    const work = await getWork(workId)
    return String(work.status || '').toLowerCase() === 'canceled'
  } catch {
    return false
  }
}

function errorMessage(error: unknown): string {
  const value = error as any
  return String(value?.response?.data?.detail || value?.response?.data?.message || value?.message || '操作失败')
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function onImageModelChange(modelId: string) {
  const model = imageModels.value.find((item) => item.id === modelId)
  const tier = model?.quality_tiers?.[0] || '1K'
  const ratio = model?.aspect_ratios?.[0] || '1:1'
  updateSelected({ model: modelId, qualityTier: tier, aspectRatio: ratio, size: resolveImageSize(model, tier, ratio) })
}

function updateSelectedImageQuality(tier: string) {
  const ratio = selectedNode.value?.data.aspectRatio || '1:1'
  updateSelected({ qualityTier: tier, size: resolveImageSize(selectedImageModel.value || undefined, tier, ratio) })
}

function updateSelectedImageAspect(ratio: string) {
  const tier = selectedNode.value?.data.qualityTier || '1K'
  updateSelected({ aspectRatio: ratio, size: resolveImageSize(selectedImageModel.value || undefined, tier, ratio) })
}

function resolveImageSize(model: CreazyCanvasImageModel | undefined, tier: string, ratio: string): string {
  const sizes = model?.sizes?.length ? model.sizes : [tier]
  const candidates = sizes.filter((size) => classifyImageQualityTier(size) === tier)
  const match = String(ratio).match(/^(\d+):(\d+)$/)
  if (match) {
    const rw = Number(match[1])
    const rh = Number(match[2])
    const exact = candidates.find((size) => {
      const dims = parseImageSize(size)
      return dims && Math.abs(dims.width / dims.height - rw / rh) < 0.01
    })
    if (exact) return exact
    if (model?.allow_custom_size && rw > 0 && rh > 0) {
      const maxEdge = tier === '4K' ? 3840 : tier === '2K' ? 2048 : 1536
      const pixelBudget = tier === '4K' ? 8_294_400 : tier === '2K' ? 4_194_304 : 1_572_864
      const idealWidth = Math.sqrt((pixelBudget * rw) / rh)
      const idealHeight = Math.sqrt((pixelBudget * rh) / rw)
      const scale = Math.min(1, maxEdge / Math.max(idealWidth, idealHeight))
      const align16 = (value: number) => Math.max(16, Math.round(value / 16) * 16)
      return `${align16(idealWidth * scale)}x${align16(idealHeight * scale)}`
    }
  }
  return candidates[0] || sizes[0] || tier
}

function parseImageSize(size: string): { width: number; height: number } | null {
  const match = String(size || '').trim().match(/^(\d{2,5})\s*[xX×]\s*(\d{2,5})$/)
  if (!match) return null
  const width = Number(match[1])
  const height = Number(match[2])
  return width > 0 && height > 0 ? { width, height } : null
}

function classifyImageQualityTier(size: string): string {
  const normalized = String(size || '').trim().toUpperCase()
  if (normalized === '1K' || normalized === '2K' || normalized === '4K') return normalized
  const dims = parseImageSize(size)
  if (!dims) return '1K'
  const maxEdge = Math.max(dims.width, dims.height)
  if (maxEdge <= 1024) return '1K'
  if (maxEdge <= 2048) return '2K'
  return '4K'
}

function onVideoModelChange(modelId: string) {
  const model = videoModels.value.find((item) => item.id === modelId)
  updateSelected({
    model: modelId,
    resolution: model?.default_resolution || model?.resolutions?.[0] || '720p',
    duration: model?.default_duration || model?.durations?.[0] || 5,
    aspectRatio: model?.aspect_ratios?.[0] || '16:9',
    generateAudio: Boolean(model?.allow_generated_audio || model?.force_generated_audio),
  })
}

async function loadSelectedVideo() {
  const node = selectedNode.value
  if (!node?.data.workId) return
  try {
    const url = await getWorkContentBlob(node.data.workId)
    if (url.startsWith('blob:')) localObjectUrls.add(url)
    patchNode(node.id, { playableUrl: url, error: '' })
  } catch (error) {
    patchNode(node.id, { error: errorMessage(error) })
  }
}

function onViewportChangeEnd(viewport: ViewportTransform) {
  savedViewport.value = { x: viewport.x, y: viewport.y, zoom: viewport.zoom }
  scheduleSave()
}

async function revealSelectedNodeBesideInspector() {
  if (!selectedNode.value || window.matchMedia('(max-width: 900px)').matches) return
  await nextTick()
  const stage = stageEl.value
  const inspector = stage?.parentElement?.querySelector<HTMLElement>('.wf-inspector')
  const selected = stage?.querySelector<HTMLElement>('.vue-flow__node.selected')
  if (!stage || !inspector || !selected) return

  const stageRect = stage.getBoundingClientRect()
  const inspectorRect = inspector.getBoundingClientRect()
  const selectedRect = selected.getBoundingClientRect()
  const rightOverlap = selectedRect.right - (inspectorRect.left - 24)
  const leftOverlap = (stageRect.left + 24) - selectedRect.left
  const shiftX = rightOverlap > 0 ? -rightOverlap : leftOverlap > 0 ? leftOverlap : 0
  if (Math.abs(shiftX) < 1) return

  const nextViewport = {
    ...savedViewport.value,
    x: savedViewport.value.x + shiftX,
  }
  savedViewport.value = nextViewport
  await setViewport(nextViewport, {
    duration: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 160,
  })
}

async function fitCanvas() {
  await fitView({ padding: 0.18, duration: 320, minZoom: 0.45, maxZoom: 1.05 })
}

async function undoEditor() {
  if (!ensureGraphEditable()) return
  await flushPendingNodeEdit()
  await editorHistory.undo()
}

async function redoEditor() {
  if (!ensureGraphEditable()) return
  await flushPendingNodeEdit()
  await editorHistory.redo()
}

function selectAllElements() {
  nodes.value = nodes.value.map((node) => ({ ...node, selected: true }))
  edges.value = edges.value.map((edge) => ({ ...edge, selected: true }))
  selectedNodeId.value = nodes.value.length === 1 ? nodes.value[0]?.id || '' : ''
  closeContextMenu()
}

async function fitSelection() {
  const ids = selectedNodes.value.map((node) => node.id)
  closeContextMenu()
  if (!ids.length) return fitCanvas()
  await fitView({ nodes: ids, padding: 0.28, duration: 280, minZoom: 0.5, maxZoom: 1.15 })
}

function resetCopiedNodeData(source: WorkflowNodeData): WorkflowNodeData {
  const data = cloneValue(source)
  if (data.kind === 'result') {
    data.kind = 'asset'
    data.title = data.name || `${data.title || '生成结果'}副本`
    data.status = 'succeeded'
    data.mediaUrl = data.mediaUrl || data.outputUrl || ''
    data.previewUrl = data.previewUrl || data.outputUrl || data.mediaUrl || ''
  } else if (data.kind === 'image' || data.kind === 'video') {
    data.status = 'idle'
    data.outputUrl = ''
    data.previewUrl = ''
    data.playableUrl = ''
  }
  delete data.workId
  delete data.gatewayRemoteId
  delete data.error
  return data
}

function buildClipboardFragment(): WorkflowClipboardFragment | null {
  const sourceNodes = selectedNodes.value
  if (!sourceNodes.length) return null
  const minX = Math.min(...sourceNodes.map((node) => node.position.x))
  const minY = Math.min(...sourceNodes.map((node) => node.position.y))
  const nodeIds = sourceNodes.map((node) => node.id)
  return {
    type: 'creazy-workflow-fragment',
    version: 1,
    nodes: sourceNodes.map((node) => ({
      sourceId: node.id,
      position: { x: node.position.x - minX, y: node.position.y - minY },
      data: resetCopiedNodeData(node.data),
    })),
    edges: internalEdges(nodeIds, edges.value).map((edge) => ({ source: edge.source, target: edge.target })),
  }
}

async function copySelection() {
  closeContextMenu()
  const fragment = buildClipboardFragment()
  if (!fragment) return
  clipboardFragment.value = fragment
  try {
    await navigator.clipboard?.writeText(JSON.stringify(fragment))
  } catch {
    // The in-memory fragment remains available when clipboard permission is unavailable.
  }
  showConnectionNotice(`已复制 ${fragment.nodes.length} 个节点`)
}

function isWorkflowFragment(value: unknown): value is WorkflowClipboardFragment {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<WorkflowClipboardFragment>
  if (
    candidate.type !== 'creazy-workflow-fragment' || candidate.version !== 1 ||
    !Array.isArray(candidate.nodes) || !Array.isArray(candidate.edges) ||
    candidate.nodes.length > 200 || candidate.edges.length > 400
  ) return false
  const allowedKinds = new Set<WorkflowKind>(['asset', 'prompt', 'image', 'video', 'result'])
  const sourceIds = new Set<string>()
  for (const item of candidate.nodes as unknown[]) {
    if (!item || typeof item !== 'object') return false
    const node = item as Record<string, unknown>
    const position = node.position as Record<string, unknown> | undefined
    const data = node.data as Record<string, unknown> | undefined
    if (
      typeof node.sourceId !== 'string' || !node.sourceId || node.sourceId.length > 200 || sourceIds.has(node.sourceId) ||
      !position || typeof position.x !== 'number' || typeof position.y !== 'number' ||
      !Number.isFinite(position.x) || !Number.isFinite(position.y) ||
      Math.abs(position.x) > 10_000_000 || Math.abs(position.y) > 10_000_000 ||
      !data || !allowedKinds.has(data.kind as WorkflowKind)
    ) return false
    sourceIds.add(node.sourceId)
  }
  return (candidate.edges as unknown[]).every((item) => {
    if (!item || typeof item !== 'object') return false
    const edge = item as Record<string, unknown>
    return typeof edge.source === 'string' && typeof edge.target === 'string' &&
      edge.source !== edge.target && sourceIds.has(edge.source) && sourceIds.has(edge.target)
  })
}

async function readClipboardFragment(): Promise<WorkflowClipboardFragment | null> {
  if (clipboardFragment.value) return cloneValue(clipboardFragment.value)
  try {
    const raw = await navigator.clipboard?.readText()
    if (!raw || raw.length > 1_000_000) return null
    const parsed: unknown = JSON.parse(raw)
    return isWorkflowFragment(parsed) ? parsed : null
  } catch {
    return null
  }
}

function viewportCenterPosition(): XYPosition {
  const rect = stageEl.value?.getBoundingClientRect()
  if (!rect) return nextPosition()
  return screenToFlowCoordinate({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
}

async function pasteSelection(anchor?: XYPosition) {
  if (!ensureGraphEditable()) return
  const fragment = await readClipboardFragment()
  closeContextMenu()
  if (!fragment?.nodes.length) {
    showConnectionNotice('剪贴板中没有可粘贴的画布节点')
    return
  }
  const origin = anchor || lastCanvasPointer.value || viewportCenterPosition()
  const idMap = new Map<string, string>()
  const pastedNodes = fragment.nodes.map((item) => {
    const id = uid(item.data.kind || 'node')
    idMap.set(item.sourceId, id)
    return {
      id,
      type: 'workflow' as const,
      position: { x: origin.x + item.position.x, y: origin.y + item.position.y },
      data: resetCopiedNodeData(item.data),
      dragHandle: '.wf-node-drag',
      ariaLabel: item.data.title,
      selected: true,
    } satisfies WorkflowNode
  })
  const pastedNodeById = new Map(pastedNodes.map((node) => [node.id, node]))
  const pastedEdges: WorkflowEdge[] = []
  const graphNodes = [...nodes.value, ...pastedNodes]
  for (const edge of fragment.edges) {
    const source = idMap.get(edge.source)
    const target = idMap.get(edge.target)
    if (!source || !target || source === target) continue
    const sourceNode = pastedNodeById.get(source)
    const targetNode = pastedNodeById.get(target)
    if (!sourceNode || !targetNode || !nodeDataConnectionCompatible(sourceNode.data, targetNode.data)) continue
    if (pastedEdges.some((item) => item.source === source && item.target === target)) continue
    if (wouldCreateCycle(graphNodes, [...edges.value, ...pastedEdges], source, target)) continue
    pastedEdges.push(makeEdge(source, target, pastedNodes))
  }
  const nodeIds = new Set(pastedNodes.map((node) => node.id))
  const edgeIds = new Set(pastedEdges.map((edge) => edge.id))
  await executeEditorCommand(historyCommand(
    `粘贴 ${pastedNodes.length} 个节点`,
    () => {
      nodes.value = [...nodes.value.filter((node) => !nodeIds.has(node.id)), ...cloneValue(pastedNodes)]
      edges.value = [...edges.value.filter((edge) => !edgeIds.has(edge.id)), ...cloneValue(pastedEdges)]
      selectOnlyNodes(nodeIds)
    },
    () => {
      nodes.value = nodes.value.filter((node) => !nodeIds.has(node.id))
      edges.value = edges.value.filter((edge) => !edgeIds.has(edge.id))
      selectedNodeId.value = ''
    },
  ))
  await nextTick()
  await fitView({ nodes: [...nodeIds], padding: 0.45, duration: 240, maxZoom: 1.1 })
}

async function duplicateSelection() {
  const source = selectedNodes.value
  if (!source.length) return
  await copySelection()
  const minX = Math.min(...source.map((node) => node.position.x))
  const minY = Math.min(...source.map((node) => node.position.y))
  await pasteSelection({ x: minX + 32, y: minY + 32 })
}

function openCommandPalette(anchor?: XYPosition) {
  closeContextMenu()
  runMenuOpen.value = false
  commandAnchor.value = anchor || lastCanvasPointer.value || viewportCenterPosition()
  commandQuery.value = ''
  commandPaletteOpen.value = true
  void nextTick(() => commandInput.value?.focus())
}

function closeCommandPalette() {
  commandPaletteOpen.value = false
  commandQuery.value = ''
}

function runCommand(command: CommandPaletteItem) {
  if (command.disabled) return
  closeCommandPalette()
  void command.action()
}

function runFirstCommand() {
  const command = filteredCommands.value.find((item) => !item.disabled)
  if (command) runCommand(command)
}

function addNodeFromContext(kind: PaletteNodeKind) {
  const position = contextMenu.value?.flowPosition || nextPosition()
  closeContextMenu()
  addNode(kind, position)
}

function prepareRunForContext(scope: RunScope) {
  const nodeId = contextMenu.value?.nodeId
  closeContextMenu()
  if (!nodeId) return
  selectOnlyNode(nodeId)
  prepareRun(scope, nodeId)
}

function autoLayout() {
  if (!ensureGraphEditable()) return
  closeContextMenu()
  if (nodes.value.length < 2) return
  const topology = topologicalOrder(nodes.value.map((node) => node.id), edges.value)
  if (topology.hasCycle) {
    showConnectionNotice('检测到循环连接，修复后才能自动排版')
    return
  }
  const graph = new dagre.graphlib.Graph()
  graph.setGraph({ rankdir: 'LR', ranksep: 104, nodesep: 52, marginx: 56, marginy: 56 })
  graph.setDefaultEdgeLabel(() => ({}))
  for (const node of nodes.value) {
    const wrapper = Array.from(stageEl.value?.querySelectorAll<HTMLElement>('.vue-flow__node') || [])
      .find((element) => element.dataset.id === node.id)
    const element = wrapper?.querySelector<HTMLElement>('.wf-node')
    const fallbackHeight = node.data.kind === 'asset' || node.data.kind === 'result' ? 214 : 116
    graph.setNode(node.id, {
      width: element?.offsetWidth || 320,
      height: element?.offsetHeight || fallbackHeight,
    })
  }
  for (const edge of edges.value) graph.setEdge(edge.source, edge.target)
  dagre.layout(graph)
  const before = Object.fromEntries(nodes.value.map((node) => [node.id, { ...node.position }]))
  const after = Object.fromEntries(nodes.value.map((node) => {
    const point = graph.node(node.id) as { x: number; y: number; width: number; height: number }
    return [node.id, { x: Math.round(point.x - point.width / 2), y: Math.round(point.y - point.height / 2) }]
  }))
  void executeEditorCommand(historyCommand(
    '自动排版',
    () => applyNodePositions(after),
    () => applyNodePositions(before),
  )).then(() => nextTick()).then(() => fitCanvas()).catch(() => undefined)
}

function pickConfiguredPrice(prices: Record<string, number | null | undefined> | undefined, keys: string[]): number | null {
  if (!prices) return null
  for (const key of keys) {
    const exact = prices[key]
    if (exact != null && Number.isFinite(Number(exact))) return Number(exact)
    const entry = Object.entries(prices).find(([candidate, value]) => candidate.toLowerCase() === key.toLowerCase() && value != null)
    if (entry?.[1] != null && Number.isFinite(Number(entry[1]))) return Number(entry[1])
  }
  return null
}

function estimatedNodeCost(node: WorkflowNode): number | null {
  if (node.data.kind === 'image') {
    const model = imageModels.value.find((item) => item.id === node.data.model)
    if (!model) return null
    return pickConfiguredPrice(model.prices, [node.data.qualityTier || '', node.data.size || '', '1K'])
  }
  if (node.data.kind === 'video') {
    const model = videoModels.value.find((item) => item.id === node.data.model)
    if (!model) return null
    const unit = pickConfiguredPrice(model.prices, [node.data.resolution || '', '720p', '1080p'])
    if (unit == null) return null
    const billingUnit = String(model.billing_unit || 'per_second').toLowerCase()
    return ['per_request', 'per-request', 'request'].includes(billingUnit)
      ? unit
      : unit * Math.max(1, Number(node.data.duration || 1))
  }
  return 0
}

function estimatedNodeCostById(nodeId: string): number | null {
  const node = nodes.value.find((item) => item.id === nodeId)
  return node ? estimatedNodeCost(node) : null
}

function formatRunDuration(value: number): string {
  if (value < 1000) return `${Math.max(1, Math.round(value))}ms`
  const seconds = value / 1000
  return seconds < 60 ? `${seconds.toFixed(seconds < 10 ? 1 : 0)}s` : `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`
}

function formatCost(value: number): string {
  if (value >= 1) return value.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
  return value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '') || '0'
}

function nodeTitle(nodeId: string): string {
  return nodes.value.find((node) => node.id === nodeId)?.data.title || '未知节点'
}

function buildRunLayers(nodeIds: string[]): string[][] {
  const remaining = new Set(nodeIds)
  const layers: string[][] = []
  while (remaining.size) {
    const ready = [...remaining].filter((nodeId) => !edges.value.some((edge) => (
      edge.target === nodeId && remaining.has(edge.source)
    )))
    if (!ready.length) return []
    layers.push(ready)
    ready.forEach((nodeId) => remaining.delete(nodeId))
  }
  return layers
}

function candidateRunNodeIds(scope: RunScope, sourceNodeId?: string): string[] {
  const generationIds = new Set(generationNodes.value.map((node) => node.id))
  if (scope === 'workflow') return generationNodes.value.map((node) => node.id)
  const source = sourceNodeId || selectedNode.value?.id || selectedNodeId.value
  if (!source || !generationIds.has(source)) return []
  if (scope === 'node') return [source]
  const downstream = downstreamNodeIds(source, edges.value)
  return [source, ...downstream].filter((nodeId) => generationIds.has(nodeId))
}

function buildRunConfirmation(scope: RunScope, sourceNodeId?: string): RunConfirmation {
  const candidates = candidateRunNodeIds(scope, sourceNodeId)
  const nodeIds = candidates.filter((nodeId) => {
    if (!reuseCompletedOutputs.value || scope === 'node') return true
    const node = nodes.value.find((item) => item.id === nodeId)
    return !(node?.data.status === 'succeeded' && node.data.outputUrl)
  })
  const issues: RunIssue[] = []
  const graphOrder = topologicalOrder(nodes.value.map((node) => node.id), edges.value)
  if (graphOrder.hasCycle) {
    issues.push({
      key: 'cycle',
      nodeId: graphOrder.unresolvedNodeIds[0],
      title: '工作流存在循环',
      message: '删除循环连接后再运行',
      blocking: true,
    })
  }
  if (!apiReady.value) {
    issues.push({ key: 'api', title: 'API Key 不可用', message: '请选择允许画布生成的 API Key', blocking: true })
  }
  if (!nodeIds.length) {
    issues.push({
      key: 'empty',
      title: candidates.length ? '没有需要重新运行的节点' : '没有可执行节点',
      message: candidates.length ? '关闭“复用已完成结果”可强制重新生成' : '请选择生成节点或添加图片/视频节点',
      blocking: true,
    })
  }
  for (const nodeId of nodeIds) {
    const node = nodes.value.find((item) => item.id === nodeId)
    if (!node) continue
    const model = node.data.kind === 'image'
      ? imageModels.value.find((item) => item.id === node.data.model)
      : videoModels.value.find((item) => item.id === node.data.model)
    if (!model) {
      issues.push({ key: `model:${nodeId}`, nodeId, title: `${node.data.title}缺少模型`, message: '在右侧参数面板选择可用模型', blocking: true })
    }
    if (!resolvedPrompt(node)) {
      issues.push({ key: `prompt:${nodeId}`, nodeId, title: `${node.data.title}缺少提示词`, message: '填写节点提示词或连接提示词节点', blocking: true })
    }
    if (node.data.status === 'running') {
      issues.push({ key: `running:${nodeId}`, nodeId, title: `${node.data.title}正在运行`, message: '等待当前任务结束后再执行', blocking: true })
    }
    if (node.data.kind === 'image') {
      const imageModel = model as CreazyCanvasImageModel | undefined
      if (imageModel?.require_reference && !incomingMedia(node.id).some((item) => item.kind === 'image')) {
        issues.push({ key: `reference:${nodeId}`, nodeId, title: `${node.data.title}缺少参考图`, message: '该模型要求至少连接一张图片', blocking: true })
      }
    } else {
      const videoModel = model as CreazyCanvasVideoModel | undefined
      if (videoModel?.require_start_frame && !incomingMedia(node.id).some((item) => item.kind === 'image')) {
        issues.push({ key: `frame:${nodeId}`, nodeId, title: `${node.data.title}缺少首帧`, message: '该模型要求连接图片输入', blocking: true })
      }
    }
  }
  const costs = nodeIds.map((nodeId) => {
    const node = nodes.value.find((item) => item.id === nodeId)
    return node ? estimatedNodeCost(node) : null
  })
  const cost = costs.some((value) => value == null)
    ? null
    : costs.reduce<number>((sum, value) => sum + Number(value || 0), 0)
  const title = scope === 'node' ? '运行所选节点' : scope === 'downstream' ? '从所选节点运行' : '运行完整工作流'
  return { scope, sourceNodeId, title, nodeIds, layers: buildRunLayers(nodeIds), issues, cost }
}

function prepareRun(scope: RunScope, sourceNodeId?: string) {
  runMenuOpen.value = false
  closeContextMenu()
  closeCommandPalette()
  runConfirmation.value = buildRunConfirmation(scope, sourceNodeId)
}

function refreshRunConfirmation() {
  const current = runConfirmation.value
  if (current) runConfirmation.value = buildRunConfirmation(current.scope, current.sourceNodeId)
}

function cancelRunConfirmation() {
  runConfirmation.value = null
}

async function focusIssue(issue: RunIssue) {
  if (!issue.nodeId) return
  selectOnlyNode(issue.nodeId)
  await nextTick()
  await fitView({ nodes: [issue.nodeId], padding: 0.8, duration: 260, maxZoom: 1.05 })
}

async function executeWorkflowPlan(plan: RunConfirmation) {
  workflowRun.value = {
    id: `run-${Date.now()}`,
    active: true,
    finished: false,
    stopRequested: false,
    completed: 0,
    total: plan.nodeIds.length,
    message: '正在准备第一批节点',
  }
  const unavailable = new Set<string>()
  for (const layer of plan.layers) {
    if (workflowRun.value.stopRequested) break
    const runnable = layer.filter((nodeId) => !edges.value.some((edge) => edge.target === nodeId && unavailable.has(edge.source)))
    const skipped = layer.filter((nodeId) => !runnable.includes(nodeId))
    skipped.forEach((nodeId) => unavailable.add(nodeId))
    workflowRun.value.message = runnable.length > 1
      ? `并行运行 ${runnable.length} 个节点`
      : runnable.length === 1 ? `正在运行 ${nodeTitle(runnable[0])}` : '跳过依赖失败的节点'
    const results = await Promise.all(runnable.map(async (nodeId) => ({ nodeId, ok: await runNode(nodeId) })))
    results.filter((result) => !result.ok).forEach((result) => unavailable.add(result.nodeId))
    workflowRun.value.completed += layer.length
  }
  workflowRun.value.active = false
  workflowRun.value.finished = true
  if (workflowRun.value.stopRequested) {
    workflowRun.value.message = `已停止，完成 ${workflowRun.value.completed}/${workflowRun.value.total}`
  } else if (unavailable.size) {
    workflowRun.value.message = `执行结束，${unavailable.size} 个节点失败或被跳过`
  } else {
    workflowRun.value.message = '所有计划节点执行完成'
  }
}

function confirmWorkflowRun() {
  const pending = runConfirmation.value
  if (!pending) return
  const plan = buildRunConfirmation(pending.scope, pending.sourceNodeId)
  if (plan.issues.some((issue) => issue.blocking)) {
    runConfirmation.value = plan
    return
  }
  runConfirmation.value = null
  void executeWorkflowPlan(plan)
}

function stopWorkflowRun() {
  workflowRun.value.stopRequested = true
  workflowRun.value.message = '当前批次完成后停止，不再提交后续节点'
}

function dismissWorkflowRun() {
  workflowRun.value.finished = false
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

function onWorkspaceKeydown(event: KeyboardEvent) {
  const shell = shellEl.value
  const target = event.target
  if (!shell || !(target instanceof Node) || !shell.contains(target)) return
  if (event.key === 'Escape') {
    closeContextMenu()
    closeCommandPalette()
    runMenuOpen.value = false
    if (runConfirmation.value) cancelRunConfirmation()
    return
  }
  if (isEditableTarget(target)) return
  const modifier = event.ctrlKey || event.metaKey
  const key = event.key.toLowerCase()
  if (modifier && key === 'z') {
    event.preventDefault()
    if (event.shiftKey) void redoEditor()
    else void undoEditor()
  } else if (modifier && key === 'y') {
    event.preventDefault()
    void redoEditor()
  } else if (modifier && key === 'a') {
    event.preventDefault()
    selectAllElements()
  } else if (modifier && key === 'c') {
    event.preventDefault()
    void copySelection()
  } else if (modifier && key === 'v') {
    event.preventDefault()
    void pasteSelection()
  } else if (modifier && key === 'd') {
    event.preventDefault()
    void duplicateSelection()
  } else if (modifier && key === '0') {
    event.preventDefault()
    void fitCanvas()
  } else if (event.key === 'Delete' || event.key === 'Backspace') {
    event.preventDefault()
    deleteSelection()
  } else if (event.key === '/') {
    event.preventDefault()
    openCommandPalette()
  }
}

function graphForPersistence(): CreazyCanvasGraph {
  return {
    nodes: nodes.value.map((node) => ({
      id: node.id,
      type: 'workflow',
      position: { x: node.position.x, y: node.position.y },
      dragHandle: '.wf-node-drag',
      data: sanitizeNodeData(node.data),
    })),
    edges: edges.value.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle || undefined,
      targetHandle: edge.targetHandle || undefined,
      type: 'signal',
      markerEnd: MarkerType.ArrowClosed,
    })),
    viewport: { ...savedViewport.value },
  }
}

function sanitizeNodeData(data: WorkflowNodeData): WorkflowNodeData {
  const copy = { ...data }
  if (copy.previewUrl?.startsWith('blob:')) copy.previewUrl = copy.mediaUrl || copy.outputUrl || ''
  if (copy.playableUrl?.startsWith('blob:')) copy.playableUrl = ''
  return copy
}

function serializeDocumentSnapshot(graph: CreazyCanvasGraph, name = documentName.value): string {
  return JSON.stringify({ name, graph })
}

function graphFromDocument(graph?: CreazyCanvasGraph): { nodes: WorkflowNode[]; edges: WorkflowEdge[]; viewport: ViewportTransform } {
  if (!graph || !Array.isArray(graph.nodes) || !Array.isArray(graph.edges)) return starterGraph()
  const restoredNodes = graph.nodes
    .filter((item: any) => item && item.id && item.position && item.data)
    .map((item: any) => ({
      id: String(item.id),
      type: 'workflow' as const,
      position: { x: Number(item.position.x || 0), y: Number(item.position.y || 0) },
      data: { status: 'idle', title: '节点', ...item.data } as WorkflowNodeData,
      dragHandle: '.wf-node-drag',
    })) as WorkflowNode[]
  const restoredNodeIds = new Set(restoredNodes.map((node) => node.id))
  const restoredEdges = graph.edges
    .filter((item: any) => item && item.source && item.target && restoredNodeIds.has(String(item.source)) && restoredNodeIds.has(String(item.target)) && item.source !== item.target)
    .map((item: any) => makeEdge(
      String(item.source),
      String(item.target),
      restoredNodes,
      String(item.id || uid('edge')),
    )) as WorkflowEdge[]
  const viewport = graph.viewport || {}
  const restoredZoom = Number(viewport.zoom || 0.9)
  return {
    nodes: restoredNodes.length ? restoredNodes : starterNodes(),
    edges: restoredEdges,
    viewport: {
      x: Number(viewport.x || 0),
      y: Number(viewport.y || 0),
      zoom: Math.min(1.4, Math.max(0.6, Number.isFinite(restoredZoom) ? restoredZoom : 0.9)),
    },
  }
}

function localDraftKey(): string {
  return `${LOCAL_DRAFT_PREFIX}:${props.apiKeyId || 'default'}`
}

function writeLocalDraft() {
  try {
    const draft: LocalWorkflowDraft = {
      documentId: documentId.value,
      baseRevision: documentRevision.value,
      updatedAt: Date.now(),
      dirty: true,
      name: documentName.value,
      graph: graphForPersistence(),
    }
    localStorage.setItem(localDraftKey(), JSON.stringify(draft))
  } catch {
    // Browser storage is a best-effort safety net; server persistence remains authoritative.
  }
}

function clearLocalDraft() {
  try {
    localStorage.removeItem(localDraftKey())
  } catch {
    // Local storage cleanup is best effort after the server confirms the snapshot.
  }
}

function readLocalDraft(): LocalWorkflowDraft | null {
  try {
    const raw = localStorage.getItem(localDraftKey())
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<LocalWorkflowDraft>
    if (!parsed || typeof parsed !== 'object' || !parsed.graph) return null
    return {
      documentId: Number(parsed.documentId || 0),
      baseRevision: Number(parsed.baseRevision || 0),
      updatedAt: Number(parsed.updatedAt || 0),
      dirty: parsed.dirty !== false,
      name: typeof parsed.name === 'string' ? parsed.name : undefined,
      graph: parsed.graph,
    }
  } catch {
    return null
  }
}

function scheduleSave() {
  if (disposed || hydrating || loading.value) return
  if (documentId.value) {
    const snapshot = serializeDocumentSnapshot(graphForPersistence())
    if (snapshot === lastSavedSnapshot) {
      if (saveState.value !== 'conflict') {
        saveState.value = 'saved'
        saveError.value = ''
      }
      return
    }
  }
  writeLocalDraft()
  saveRequested = true
  if (saveState.value === 'conflict') return
  saveState.value = 'dirty'
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    saveTimer = null
    void saveDocument(false)
  }, 700)
}

async function saveDocument(_manual: boolean) {
  if (disposed || hydrating || loading.value || saveState.value === 'conflict') return
  saveRequested = true
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (saveInFlight) return saveInFlight
  saveInFlight = (async () => {
    saving.value = true
    try {
      while (saveRequested && !disposed) {
        saveRequested = false
        saveState.value = 'saving'
        saveError.value = ''
        writeLocalDraft()
        const graph = graphForPersistence()
        const snapshot = serializeDocumentSnapshot(graph)
        if (documentId.value && snapshot === lastSavedSnapshot) {
          saveState.value = 'saved'
          conflictDraft.value = null
          clearLocalDraft()
          continue
        }
        try {
          let document: CreazyCanvasDocument
          if (!documentId.value) {
            document = await createDocument({ name: documentName.value, graph })
          } else {
            document = await updateDocument(documentId.value, {
              name: documentName.value,
              graph,
              expected_revision: documentRevision.value,
            })
          }
          documentId.value = document.id
          documentRevision.value = document.revision
          lastSavedSnapshot = snapshot
          if (serializeDocumentSnapshot(graphForPersistence()) === snapshot) {
            saveState.value = 'saved'
            conflictDraft.value = null
            clearLocalDraft()
          } else {
            saveState.value = 'dirty'
            saveRequested = true
            writeLocalDraft()
          }
        } catch (error: any) {
          const status = Number(error?.response?.status || error?.status || 0)
          if (status === 409 && documentId.value) {
            saveState.value = 'conflict'
            saveError.value = '画布已在其他位置更新，请选择云端版本或另存本地副本'
            conflictDraft.value = readLocalDraft()
          } else {
            saveState.value = 'local'
            saveError.value = errorMessage(error)
          }
          saveRequested = false
        }
      }
    } finally {
      saving.value = false
      saveInFlight = null
    }
  })()
  return saveInFlight
}

async function reloadCloudDocument() {
  if (saving.value) return
  saveRequested = false
  conflictDraft.value = null
  clearLocalDraft()
  await loadDocument()
}

async function saveConflictAsCopy() {
  if (saving.value) return
  saving.value = true
  const draft = conflictDraft.value
  const graph = draft?.graph || graphForPersistence()
  const sourceName = draft?.name || documentName.value
  const copyName = `${sourceName.replace(/\s+副本$/, '')} 副本`
  try {
    const document = await createDocument({ name: copyName, graph })
    documentId.value = document.id
    documentRevision.value = document.revision
    documentName.value = document.name || copyName
    if (draft?.graph) {
      const restored = graphFromDocument(draft.graph)
      nodes.value = restored.nodes
      edges.value = restored.edges
      savedViewport.value = restored.viewport
      await editorHistory.clear()
      await nextTick()
      await setViewport(restored.viewport, { duration: 0 })
    }
    lastSavedSnapshot = serializeDocumentSnapshot(graph, documentName.value)
    saveState.value = 'saved'
    saveError.value = ''
    conflictDraft.value = null
    clearLocalDraft()
  } catch (error) {
    saveState.value = 'local'
    saveError.value = errorMessage(error)
    writeLocalDraft()
  } finally {
    saving.value = false
  }
}

async function loadDocument() {
  loading.value = true
  saveState.value = 'loading'
  hydrating = true
  conflictDraft.value = null
  const local = readLocalDraft()
  let loadedGraph: CreazyCanvasGraph | undefined
  let restoredLocalDraft = false
  try {
    const documents = await listDocuments()
    if (documents.length) {
      const document = await getDocument(documents[0].id)
      const remoteName = document.name || '我的工作流'
      documentId.value = document.id
      documentRevision.value = document.revision
      documentName.value = remoteName
      loadedGraph = document.graph
      if (
        local?.dirty && local.graph && local.documentId === document.id &&
        local.baseRevision === document.revision
      ) {
        documentName.value = local.name || remoteName
        loadedGraph = local.graph
        restoredLocalDraft = true
        saveState.value = 'dirty'
      } else if (local?.dirty && local.graph) {
        conflictDraft.value = local
        saveState.value = 'conflict'
        saveError.value = '云端版本已变化，本地未保存草稿仍可另存为副本'
      } else {
        saveState.value = 'saved'
        clearLocalDraft()
      }
    } else {
      const initial = local?.dirty && local.graph
        ? { graph: local.graph, name: local.name || documentName.value }
        : { graph: starterGraph(), name: documentName.value }
      const created = await createDocument({
        name: initial.name,
        graph: 'nodes' in initial.graph
          ? { nodes: initial.graph.nodes as any, edges: initial.graph.edges as any, viewport: initial.graph.viewport }
          : initial.graph,
      })
      documentId.value = created.id
      documentRevision.value = created.revision
      documentName.value = created.name || initial.name
      loadedGraph = created.graph
      saveState.value = 'saved'
      clearLocalDraft()
    }
  } catch (error) {
    if (local?.name) documentName.value = local.name
    loadedGraph = local?.graph
    if (local) {
      documentId.value = local.documentId
      documentRevision.value = local.baseRevision
      restoredLocalDraft = true
    }
    saveState.value = 'local'
    saveError.value = errorMessage(error)
  }
  const restored = graphFromDocument(loadedGraph)
  nodes.value = restored.nodes
  edges.value = restored.edges
  savedViewport.value = restored.viewport
  await editorHistory.clear()
  await nextTick()
  if (restored.nodes.length) {
    await setViewport(restored.viewport, { duration: 0 })
  } else {
    await fitCanvas()
  }
  lastSavedSnapshot = restoredLocalDraft ? '' : serializeDocumentSnapshot(graphForPersistence())
  loading.value = false
  hydrating = false
  if (restoredLocalDraft && saveState.value === 'dirty') scheduleSave()
  void refreshNodeWorkStatuses()
}

async function refreshNodeWorkStatuses() {
  if (refreshingStatuses) return
  refreshingStatuses = true
  const tracked = nodes.value.filter(
    (node) => node.data.workId && (node.data.kind === 'image' || node.data.kind === 'video'),
  )
  try {
    for (const node of tracked) {
      try {
        const trackedWorkId = Number(node.data.workId)
        const work = await getWork(trackedWorkId)
        const liveNode = nodes.value.find((item) => item.id === node.id)
        if (disposed || !liveNode || liveNode.data.workId !== trackedWorkId || work.id !== trackedWorkId) continue
        const status = String(work.status || '').toLowerCase()
        if (status === 'canceled') {
          patchNode(liveNode.id, { status: 'canceled', error: work.error_message || '任务已终止' })
        } else if (status === 'failed') {
          patchNode(liveNode.id, { status: 'failed', error: work.error_message || '生成失败' })
        } else if (status === 'succeeded') {
          const restored = await restoreSucceededWork(liveNode, work)
          if (!restored && work.gateway_remote_id) await resumeRemoteWork(liveNode, work)
        } else if (status === 'running' || status === 'queued' || status === 'created') {
          patchNode(liveNode.id, {
            status: 'running',
            gatewayRemoteId: String(work.gateway_remote_id || liveNode.data.gatewayRemoteId || ''),
          })
          await resumeRemoteWork(liveNode, work)
        }
      } catch {
        // Keep the locally persisted state when the work record is temporarily unavailable.
      }
    }
  } finally {
    refreshingStatuses = false
  }
}

watch(
  () => [props.catalog, props.apiKeyId] as const,
  () => {
    nodes.value = nodes.value.map((node) => {
      if (node.data.kind === 'image' && imageModels.value.length) {
        const model = imageModels.value.find((item) => item.id === node.data.model) || imageModels.value[0]
        const tier = model.quality_tiers?.includes(node.data.qualityTier || '')
          ? String(node.data.qualityTier)
          : model.quality_tiers?.[0] || '1K'
        const ratio = model.aspect_ratios?.includes(node.data.aspectRatio || '')
          ? String(node.data.aspectRatio)
          : model.aspect_ratios?.[0] || '1:1'
        return {
          ...node,
          data: {
            ...node.data,
            model: model.id,
            qualityTier: tier,
            aspectRatio: ratio,
            size: resolveImageSize(model, tier, ratio),
          },
        }
      }
      if (node.data.kind === 'video' && videoModels.value.length) {
        const model = videoModels.value.find((item) => item.id === node.data.model) || videoModels.value[0]
        const resolution = (model.resolutions || model.allowed_resolutions || []).includes(node.data.resolution || '')
          ? String(node.data.resolution)
          : model.default_resolution || model.resolutions?.[0] || model.allowed_resolutions?.[0] || '720p'
        const durationOptions = model.durations || model.allowed_durations || []
        const duration = durationOptions.includes(Number(node.data.duration))
          ? Number(node.data.duration)
          : model.default_duration || durationOptions[0] || 5
        const aspectOptions = model.aspect_ratios || model.allowed_aspect_ratios || []
        const aspectRatio = aspectOptions.includes(node.data.aspectRatio || '')
          ? String(node.data.aspectRatio)
          : aspectOptions[0] || '16:9'
        return {
          ...node,
          data: {
            ...node.data,
            model: model.id,
            resolution,
            duration,
            aspectRatio,
            generateAudio: Boolean(model.allow_generated_audio || model.force_generated_audio),
          },
        }
      }
      return node
    })
  },
)

watch([nodes, edges, documentName], () => scheduleSave(), { deep: true })

watch(
  () => selectedNodes.value.map((node) => node.id).join('|'),
  () => {
    selectedNodeId.value = selectedNodes.value.length === 1 ? selectedNodes.value[0]?.id || '' : ''
  },
)

watch(
  () => selectedNode.value?.id || '',
  (nodeId) => {
    if (nodeId) void revealSelectedNodeBesideInspector()
  },
)

onMounted(async () => {
  document.addEventListener('keydown', onWorkspaceKeydown)
  await loadDocument()
  statusTimer = setInterval(() => void refreshNodeWorkStatuses(), 6000)
})

onBeforeUnmount(() => {
  const needsLocalDraft = serializeDocumentSnapshot(graphForPersistence()) !== lastSavedSnapshot || saving.value
  disposed = true
  document.removeEventListener('keydown', onWorkspaceKeydown)
  void flushPendingNodeEdit()
  if (needsLocalDraft) writeLocalDraft()
  else clearLocalDraft()
  if (saveTimer) clearTimeout(saveTimer)
  if (statusTimer) clearInterval(statusTimer)
  if (noticeTimer) clearTimeout(noticeTimer)
  if (pendingNodeEdit?.timer) clearTimeout(pendingNodeEdit.timer)
  localObjectUrls.forEach((url) => URL.revokeObjectURL(url))
  localObjectUrls.clear()
})
</script>

<style scoped>
.wf-shell {
  --wf-ink: #20262e;
  --wf-muted: #687380;
  --wf-line: #d9dee5;
  --wf-paper: #f8f9fa;
  --wf-stage: #f2f4f6;
  --wf-asset: #0f766e;
  --wf-prompt: #a16207;
  --wf-image: #2563eb;
  --wf-video: #7c3aed;
  --wf-result: #059669;
  height: calc(100dvh - 114px);
  min-height: 560px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  color: var(--wf-ink);
  background: #fbfcfd;
  font-family: "IBM Plex Sans", "Segoe UI", "PingFang SC", "Microsoft YaHei UI", system-ui, sans-serif;
}

.wf-commandbar {
  min-height: 48px;
  flex: 0 0 auto;
  padding: 6px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid var(--wf-line);
  background: #fbfcfd;
}

.wf-conflict-bar {
  min-height: 44px;
  padding: 7px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid #f1b8ad;
  background: #fff7f5;
  color: #9f2f20;
  font-size: 12px;
}

.wf-conflict-bar > span,
.wf-conflict-bar > div {
  display: flex;
  align-items: center;
  gap: 8px;
}

.wf-document,
.wf-commandbar__actions,
.wf-edit-group,
.wf-run-group,
.wf-document__mark,
.wf-save-state,
.wf-tool,
.wf-task-link,
.wf-node__head,
.wf-node__chips,
.wf-inspector__head,
.wf-toggle,
.wf-resolution-preview {
  display: flex;
  align-items: center;
}

.wf-document {
  min-width: 0;
  gap: 9px;
}

.wf-document__mark {
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  justify-content: center;
  border: 1px solid #94a3b8;
  background: #111827;
}

.wf-document__mark::after {
  content: "";
  width: 9px;
  height: 9px;
  border: 2px solid #67e8f9;
  border-left-color: #f59e0b;
  transform: rotate(45deg);
}

.wf-document__name {
  width: min(32vw, 360px);
  min-width: 120px;
  border: 1px solid transparent;
  border-radius: 5px;
  padding: 6px 8px;
  background: transparent;
  color: var(--wf-ink);
  font-size: 14px;
  font-weight: 700;
}

.wf-document__name:hover,
.wf-document__name:focus {
  outline: none;
  border-color: #cbd5e1;
  background: #f8fafc;
}

.wf-save-state {
  gap: 6px;
  color: var(--wf-muted);
  font-size: 11px;
  white-space: nowrap;
}

.wf-save-state i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #94a3b8;
}

.wf-save-state--ok i { background: var(--wf-result); }
.wf-save-state--warn i { background: #d97706; }
.wf-save-state--bad { color: #b91c1c; }
.wf-save-state--bad i { background: #dc2626; }

.wf-commandbar__actions { gap: 8px; }

.wf-edit-group {
  height: 32px;
  overflow: hidden;
  border: 1px solid #d7dde4;
  border-radius: 5px;
  background: #fff;
}

.wf-edit-group .wf-icon-button {
  border: 0;
  border-right: 1px solid #e1e5ea;
  border-radius: 0;
}

.wf-edit-group .wf-icon-button:last-child { border-right: 0; }

.wf-icon-button {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid #d7dee7;
  border-radius: 5px;
  background: #fff;
  color: #475569;
}

.wf-icon-button:hover { border-color: #94a3b8; color: #111827; }
.wf-icon-button:disabled { color: #b8c0ca; cursor: not-allowed; background: #f6f7f8; }
.wf-icon-button:focus-visible,
.wf-tool:focus-visible,
.wf-node button:focus-visible { outline: 2px solid #2563eb; outline-offset: 2px; }

.wf-run-group { position: relative; align-items: stretch; }
.wf-run-button { min-width: 142px; border-radius: 5px 0 0 5px; }
.wf-run-menu-button { width: 32px; padding: 0; justify-content: center; border-left: 1px solid rgba(255, 255, 255, 0.3); border-radius: 0 5px 5px 0; }
.wf-run-menu {
  position: absolute;
  top: 38px;
  right: 0;
  z-index: 90;
  width: 250px;
  padding: 5px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 18px 38px -20px rgba(15, 23, 42, 0.62);
}
.wf-run-menu button {
  width: 100%;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 9px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: #334155;
  text-align: left;
}
.wf-run-menu button:hover:not(:disabled) { background: #f1f5f9; }
.wf-run-menu button:disabled { opacity: 0.45; cursor: not-allowed; }
.wf-run-menu button > span { min-width: 0; display: grid; gap: 2px; }
.wf-run-menu strong { font-size: 11px; }
.wf-run-menu small { color: #84909d; font-size: 10px; }

.wf-workspace {
  position: relative;
  min-height: 0;
  flex: 1 1 auto;
  display: grid;
  grid-template-columns: 188px minmax(520px, 1fr);
  overflow: hidden;
}

.wf-workspace--loading > :not(.wf-loading-state) {
  pointer-events: none;
  opacity: 0.58;
}

.wf-loading-state {
  position: absolute;
  inset: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: rgba(248, 250, 252, 0.76);
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.wf-workspace--inspecting { grid-template-columns: 188px minmax(520px, 1fr); }

.wf-palette,
.wf-inspector {
  min-width: 0;
  overflow-y: auto;
  background: #fbfcfd;
}

.wf-palette {
  padding: 14px 10px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  border-right: 1px solid var(--wf-line);
}

.wf-palette__heading { display: grid; gap: 3px; margin: 2px 6px 12px; text-align: left; }
.wf-palette__heading small { color: #94a3b8; font-size: 9px; line-height: 1.35; }
.wf-palette-tabs { display: grid; grid-template-columns: repeat(3, 1fr); gap: 3px; margin-bottom: 10px; padding: 3px; border: 1px solid #dce3ea; border-radius: 5px; background: #f1f4f7; }
.wf-palette-tabs button { min-height: 27px; border: 0; border-radius: 3px; background: transparent; color: #718096; font-size: 9px; font-weight: 700; }
.wf-palette-tabs button.active { background: #fff; color: #1f2937; box-shadow: 0 1px 3px rgba(15, 23, 42, 0.12); }

.wf-palette__heading span,
.wf-inspector__head span {
  color: #778391;
  font-size: 9px;
  font-weight: 800;
  text-transform: uppercase;
}

.wf-tool {
  width: 100%;
  min-height: 58px;
  margin-bottom: 6px;
  padding: 8px;
  justify-content: flex-start;
  gap: 9px;
  text-align: left;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: #1f2937;
}

.wf-tool:hover:not(:disabled) { border-color: #c7d2df; background: #fff; box-shadow: 0 5px 14px -11px rgba(15, 23, 42, 0.55); transform: translateY(-1px); }
.wf-tool:disabled { opacity: 0.45; cursor: not-allowed; }
.wf-tool > span:last-child { min-width: 0; display: grid; gap: 3px; }
.wf-tool strong { color: #1f2937; font-size: 11px; line-height: 1.1; white-space: nowrap; }
.wf-tool small { overflow: hidden; color: #84909d; font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.wf-tool__icon { width: 34px; height: 34px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 5px; color: #fff; }
.wf-tool--asset .wf-tool__icon { background: var(--wf-asset); }
.wf-tool--prompt .wf-tool__icon { background: var(--wf-prompt); }
.wf-tool--image .wf-tool__icon { background: var(--wf-image); }
.wf-tool--video .wf-tool__icon { background: var(--wf-video); }
.wf-asset-search { min-height: 34px; display: flex; align-items: center; gap: 6px; margin-bottom: 8px; padding: 0 8px; border: 1px solid #d5dde6; border-radius: 4px; background: #fff; color: #84909d; }
.wf-asset-search input { min-width: 0; flex: 1; border: 0; outline: 0; background: transparent; color: #1f2937; font-size: 10px; }
.wf-upload-card,
.wf-template-card { width: 100%; min-height: 56px; display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 9px; margin-bottom: 7px; padding: 8px; border: 1px dashed #b8c5d3; border-radius: 5px; background: #fff; color: #475569; text-align: left; }
.wf-upload-card > span,
.wf-template-card { align-content: center; }
.wf-upload-card > span { min-width: 0; display: grid; gap: 2px; }
.wf-upload-card strong,
.wf-template-card strong { color: #273444; font-size: 10px; }
.wf-upload-card small,
.wf-template-card small { color: #84909d; font-size: 9px; }
.wf-upload-card:hover:not(:disabled),
.wf-template-card:hover { border-color: #7d91a8; background: #f8fafc; }
.wf-template-card { grid-template-columns: 30px minmax(0, 1fr); grid-template-rows: auto auto; border-style: solid; }
.wf-template-card > span { grid-row: 1 / 3; width: 30px; height: 30px; display: grid; place-items: center; background: #edf3fa; color: #2563eb; }
.wf-template-card strong,
.wf-template-card small { grid-column: 2; }
.wf-asset-list { min-height: 0; display: grid; align-content: start; gap: 5px; overflow-y: auto; }
.wf-asset-list > button { min-width: 0; display: grid; grid-template-columns: 42px minmax(0, 1fr); align-items: center; gap: 8px; padding: 5px; border: 1px solid transparent; border-radius: 4px; background: transparent; text-align: left; }
.wf-asset-list > button:hover { border-color: #d3dce6; background: #fff; }
.wf-asset-list__preview { width: 42px; aspect-ratio: 1; display: grid; place-items: center; overflow: hidden; background: #e8edf3; color: #64748b; }
.wf-asset-list__preview img { width: 100%; height: 100%; object-fit: cover; }
.wf-asset-list > button > span:last-child { min-width: 0; display: grid; gap: 3px; }
.wf-asset-list strong,
.wf-asset-list small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wf-asset-list strong { color: #334155; font-size: 9px; }
.wf-asset-list small { color: #84909d; font-size: 8px; }
.wf-palette-empty { margin: 20px 8px; color: #94a3b8; font-size: 9px; line-height: 1.5; text-align: center; }

.wf-palette__divider { width: 100%; height: 1px; margin: 8px 0; background: var(--wf-line); }
.wf-task-link { width: 100%; height: 38px; margin-top: auto; justify-content: center; border-radius: 5px; color: #687584; text-decoration: none; }
.wf-task-link:hover { background: #eef2f6; color: #1d4ed8; }

.wf-stage {
  position: relative;
  min-width: 0;
  overflow: hidden;
  background: var(--wf-stage);
  box-shadow: inset 0 0 0 1px rgba(84, 96, 110, 0.04);
}
.wf-stage--dragging { box-shadow: inset 0 0 0 2px #0f766e; }
.wf-flow { width: 100%; height: 100%; }

.wf-node {
  position: relative;
  width: 320px;
  overflow: visible;
  border: 1px solid #cfd6de;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 4px 12px -9px rgba(32, 38, 46, 0.42);
}
.wf-node--prompt { width: 300px; }
.wf-node--asset,
.wf-node--result { width: 360px; }

.wf-node--asset { --node-color: var(--wf-asset); }
.wf-node--prompt { --node-color: var(--wf-prompt); }
.wf-node--image { --node-color: var(--wf-image); }
.wf-node--video { --node-color: var(--wf-video); }
.wf-node--result { --node-color: var(--wf-result); }
.wf-node--selected {
  border-color: #8fb2ef;
  outline: 2px solid #2563eb;
  outline-offset: 1px;
  box-shadow: 0 10px 22px -16px rgba(37, 99, 235, 0.55);
}
.wf-node--status-running { border-color: #91b1e8; }
.wf-node--status-failed { border-color: #ef9a9a; }

.wf-node__head {
  min-height: 44px;
  padding: 7px 9px 7px 10px;
  gap: 8px;
  cursor: grab;
  border-bottom: 1px solid #e2e8f0;
}

.wf-node__head:active { cursor: grabbing; }
.wf-node__type-icon { width: 27px; height: 27px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 4px; background: color-mix(in srgb, var(--node-color) 10%, white); color: var(--node-color); }
.wf-node__head > div { min-width: 0; display: grid; gap: 1px; }
.wf-node__head strong { max-width: 178px; overflow: hidden; color: #20262e; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.wf-node__head small { color: #84909d; font-size: 10px; }
.wf-node__quick-run {
  width: 26px;
  height: 26px;
  margin-left: auto;
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  border: 1px solid #cfd7e0;
  border-radius: 4px;
  background: #fff;
  color: #475569;
  opacity: 0;
  pointer-events: none;
  transition: opacity 120ms ease, color 120ms ease, border-color 120ms ease;
}
.wf-node:hover .wf-node__quick-run,
.wf-node--selected .wf-node__quick-run,
.wf-node:focus-within .wf-node__quick-run { opacity: 1; pointer-events: auto; }
.wf-node__quick-run:hover:not(:disabled) { border-color: #8aa9df; color: #1d4ed8; }
.wf-node__quick-run:disabled { color: #a8b1bc; background: #f5f7f9; }
.wf-node__status { margin-left: 2px; display: inline-flex; align-items: center; gap: 4px; color: #687380; font-size: 9px; white-space: nowrap; }
.wf-node__head > div + .wf-node__status { margin-left: auto; }
.wf-node__status i { width: 5px; height: 5px; border-radius: 50%; background: #94a3b8; }
.wf-node--status-idle .wf-node__status { display: none; }
.wf-node--status-running .wf-node__status i { background: #2563eb; animation: wf-pulse 1.1s infinite; }
.wf-node--status-succeeded .wf-node__status i { background: #059669; }
.wf-node--status-failed .wf-node__status i { background: #dc2626; }
.wf-node--status-canceled .wf-node__status i { background: #d97706; }

.wf-node__body { min-height: 82px; padding: 12px; }
.wf-node--image .wf-node__body,
.wf-node--video .wf-node__body { min-height: 176px; }
.wf-node__prompt { max-height: 72px; margin: 0; overflow: hidden; color: #3e4955; font-size: 11px; line-height: 1.5; white-space: pre-wrap; }
.wf-node__prompt--empty { color: #94a3b8; }
.wf-node__prompt-box { position: relative; min-height: 82px; margin-bottom: 12px; padding: 9px 10px; border: 1px solid #dce3ea; border-left: 3px solid var(--wf-prompt); border-radius: 4px; background: #fbfcfd; }
.wf-node__prompt-box > span { display: block; margin-bottom: 5px; color: #7c8795; font-size: 9px; font-weight: 800; text-transform: uppercase; }
.wf-node__prompt-box p { max-height: 52px; margin: 0; overflow: hidden; color: #334155; font-size: 11px; line-height: 1.45; white-space: pre-wrap; }
.wf-node__prompt-box textarea { width: 100%; min-height: 54px; padding: 0; resize: none; border: 0; outline: 0; background: transparent; color: #334155; font: inherit; font-size: 11px; line-height: 1.45; }
.wf-node__prompt-box--empty { display: grid; align-content: center; justify-items: start; border-style: dashed; background: #fffdf8; }
.wf-node__prompt-box--empty > span { display: none; }
.wf-node__prompt-box--empty p { margin-bottom: 7px; color: #9a6a26; }
.wf-node__prompt-box button { min-height: 26px; display: inline-flex; align-items: center; gap: 5px; padding: 0 8px; border: 1px solid #d6b46f; border-radius: 4px; background: #fff; color: #8a5a12; font-size: 10px; font-weight: 700; }
.wf-node__prompt-box button:hover { border-color: #a16207; background: #fffbeb; }
.wf-node__model { margin: 0 0 10px; overflow: hidden; color: #303a45; font-family: "IBM Plex Mono", "Cascadia Mono", ui-monospace, monospace; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.wf-node__chips { gap: 5px; flex-wrap: wrap; }
.wf-node__chips span { padding: 3px 5px; border: 1px solid #dce3ea; border-radius: 3px; background: #f8fafc; color: #64748b; font-size: 9px; }
.wf-node__chips .wf-node__capability { border-color: color-mix(in srgb, var(--node-color) 25%, #dce3ea); background: color-mix(in srgb, var(--node-color) 6%, white); color: color-mix(in srgb, var(--node-color) 72%, #475569); }
.wf-node__error { margin: 8px 0 0; max-height: 45px; overflow: hidden; color: #b91c1c; font-size: 9px; line-height: 1.45; }
.wf-node__footer { min-height: 42px; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 7px 10px; border-top: 1px solid #e2e8f0; background: #f8fafc; }
.wf-node__footer > span { display: inline-flex; align-items: center; gap: 6px; color: #718096; font-size: 9px; }
.wf-node__footer > span i { width: 6px; height: 6px; border-radius: 50%; background: var(--node-color); }
.wf-node__footer .wf-node__runtime { min-width: 0; flex: 1; justify-content: flex-end; gap: 7px; }
.wf-node__runtime b { color: #64748b; font-family: "IBM Plex Mono", "Cascadia Mono", ui-monospace, monospace; font-size: 8px; font-weight: 600; }
.wf-node__footer button { min-height: 28px; display: inline-flex; align-items: center; gap: 6px; padding: 0 10px; border: 1px solid color-mix(in srgb, var(--node-color) 55%, #cbd5e1); border-radius: 4px; background: #fff; color: color-mix(in srgb, var(--node-color) 82%, #1f2937); font-size: 10px; font-weight: 700; }
.wf-node__footer button:hover:not(:disabled) { background: color-mix(in srgb, var(--node-color) 7%, white); border-color: var(--node-color); }
.wf-node__footer button:disabled { opacity: 0.48; cursor: not-allowed; }

.wf-node__media { width: 100%; aspect-ratio: 16 / 9; display: grid; place-items: center; overflow: hidden; background: #111827; }
.wf-node__media img,
.wf-node__media video { width: 100%; height: 100%; object-fit: cover; }
.wf-node__media--audio { aspect-ratio: 3 / 1; background: #ecfeff; color: #0f766e; }
.wf-node__media-placeholder { display: grid; justify-items: center; gap: 5px; color: #94a3b8; font-size: 9px; }
.wf-node__file { margin: 7px 0 0; overflow: hidden; color: #334155; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.wf-node__meta { color: #94a3b8; font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 8px; }
.wf-node__media-meta { min-height: 18px; display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.wf-node__version { padding: 2px 5px; border: 1px solid #bbf7d0; border-radius: 3px; background: #f0fdf4; color: #047857; font-family: "IBM Plex Mono", ui-monospace, monospace; font-size: 8px; font-weight: 700; }
.wf-node__source-link { min-height: 25px; display: inline-flex; align-items: center; gap: 5px; margin-top: 5px; padding: 0; border: 0; background: transparent; color: #52667a; font-size: 9px; font-weight: 700; }
.wf-node__source-link:hover { color: #1d4ed8; }

.wf-handle {
  width: 10px;
  height: 10px;
  z-index: 8;
  border: 2px solid #fff;
  background: var(--port-color, var(--node-color));
  box-shadow: 0 0 0 1px var(--port-color, var(--node-color));
}
.wf-handle--target { left: -7px; }
.wf-handle--source { right: -7px; top: 50%; }

.wf-port-label {
  position: absolute;
  z-index: 7;
  height: 18px;
  display: inline-flex;
  align-items: center;
  padding: 0 5px;
  border: 1px solid color-mix(in srgb, var(--port-color) 34%, #d7dde4);
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.96);
  color: color-mix(in srgb, var(--port-color) 78%, #20262e);
  font-family: "IBM Plex Mono", "Cascadia Mono", ui-monospace, monospace;
  font-size: 8px;
  font-weight: 700;
  line-height: 1;
  pointer-events: none;
  white-space: nowrap;
  opacity: 0;
  visibility: hidden;
  transition: opacity 120ms ease, visibility 120ms ease;
}

.wf-node:hover .wf-port-label,
.wf-node--selected .wf-port-label,
.wf-node:focus-within .wf-port-label { opacity: 1; visibility: visible; }
.wf-port-label--target { left: -9px; transform: translate(-100%, -50%); }
.wf-port-label--source { right: -9px; top: 50%; transform: translate(100%, -50%); }

.wf-drop-overlay {
  position: absolute;
  inset: 18px;
  z-index: 20;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 8px;
  border: 2px dashed #0f766e;
  background: rgba(236, 254, 255, 0.92);
  color: #0f766e;
  pointer-events: none;
}

.wf-drop-overlay strong { font-size: 15px; }
.wf-drop-overlay span { color: #64748b; font-size: 11px; }
.wf-toast { position: absolute; left: 50%; bottom: 72px; z-index: 80; transform: translateX(-50%); padding: 8px 12px; border: 1px solid #334155; border-radius: 5px; background: #172033; color: #fff; font-size: 11px; box-shadow: 0 12px 24px -16px rgba(15, 23, 42, 0.75); }

.wf-selection-toolbar {
  position: absolute;
  left: 50%;
  bottom: 18px;
  z-index: 45;
  min-height: 38px;
  max-width: calc(100% - 220px);
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 3px;
  transform: translateX(-50%);
  border: 1px solid #bfc8d3;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.97);
  box-shadow: 0 12px 28px -18px rgba(15, 23, 42, 0.68);
}
.wf-selection-toolbar > span { padding: 0 8px; color: #475569; font-size: 10px; white-space: nowrap; }
.wf-selection-toolbar button {
  width: 30px;
  height: 30px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: #475569;
}
.wf-selection-toolbar button:hover:not(:disabled) { background: #edf2f7; color: #111827; }
.wf-selection-toolbar button:disabled { color: #b8c0ca; }
.wf-selection-toolbar button.danger { color: #b91c1c; }
.wf-selection-toolbar button.danger:hover { background: #fef2f2; }

.wf-run-progress {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 48;
  width: min(280px, calc(100% - 24px));
  padding: 10px 11px;
  border: 1px solid #b8c4d1;
  border-left: 4px solid #2563eb;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 14px 30px -20px rgba(15, 23, 42, 0.7);
}
.wf-run-progress > div:first-child { display: flex; justify-content: space-between; gap: 12px; color: #334155; font-size: 10px; }
.wf-run-progress > div:first-child span { font-weight: 700; }
.wf-run-progress > div:first-child strong { font-family: "IBM Plex Mono", ui-monospace, monospace; }
.wf-run-progress__bar { height: 3px; margin: 8px 0 6px; overflow: hidden; background: #e2e8f0; }
.wf-run-progress__bar i { height: 100%; display: block; background: #2563eb; transition: width 180ms ease; }
.wf-run-progress small { display: block; padding-right: 78px; overflow: hidden; color: #64748b; font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.wf-run-progress > button { position: absolute; right: 9px; bottom: 8px; border: 0; background: transparent; color: #475569; font-size: 9px; }
.wf-run-progress > button:hover { color: #b91c1c; }

.wf-context-menu {
  position: absolute;
  z-index: 100;
  width: 206px;
  padding: 5px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 18px 38px -20px rgba(15, 23, 42, 0.68);
}
.wf-context-menu > div { height: 1px; margin: 4px 3px; background: #e2e8f0; }
.wf-context-menu button {
  width: 100%;
  min-height: 34px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 6px 8px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: #334155;
  font-size: 11px;
  text-align: left;
}
.wf-context-menu button:hover:not(:disabled) { background: #f1f5f9; }
.wf-context-menu button:disabled { color: #b1bac5; cursor: not-allowed; }
.wf-context-menu button.danger { color: #b91c1c; }
.wf-context-menu button.danger:hover { background: #fef2f2; }

.wf-command-overlay,
.wf-dialog-backdrop {
  position: absolute;
  inset: 0;
  z-index: 110;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: 54px 16px 16px;
  background: rgba(30, 41, 59, 0.18);
  backdrop-filter: blur(1px);
}
.wf-command-palette {
  width: min(540px, 100%);
  overflow: hidden;
  border: 1px solid #b9c3cf;
  border-radius: 7px;
  background: #fff;
  box-shadow: 0 28px 72px -32px rgba(15, 23, 42, 0.72);
}
.wf-command-palette > label { height: 50px; display: flex; align-items: center; gap: 10px; padding: 0 14px; border-bottom: 1px solid #dbe1e8; color: #64748b; }
.wf-command-palette input { min-width: 0; flex: 1; border: 0; outline: none; background: transparent; color: #111827; font-size: 14px; }
.wf-command-palette > div { max-height: min(440px, calc(100vh - 220px)); overflow-y: auto; padding: 6px; }
.wf-command-palette > div > button { width: 100%; min-height: 52px; display: flex; align-items: center; gap: 11px; padding: 7px 9px; border: 0; border-radius: 4px; background: transparent; text-align: left; }
.wf-command-palette > div > button:hover:not(:disabled),
.wf-command-palette > div > button:focus-visible { outline: none; background: #eef3f8; }
.wf-command-palette > div > button:disabled { opacity: 0.42; cursor: not-allowed; }
.wf-command-palette > div > button > span:first-child { width: 30px; height: 30px; display: grid; place-items: center; flex: 0 0 auto; background: #edf2f7; color: #334155; }
.wf-command-palette > div > button > span:last-child { min-width: 0; display: grid; gap: 2px; }
.wf-command-palette strong { color: #1f2937; font-size: 11px; }
.wf-command-palette small { color: #84909d; font-size: 10px; }
.wf-command-palette p { margin: 24px 12px; color: #84909d; font-size: 11px; text-align: center; }

.wf-dialog-backdrop { z-index: 120; align-items: center; padding-block: 18px; background: rgba(15, 23, 42, 0.28); }
.wf-run-dialog {
  width: min(620px, 100%);
  max-height: calc(100% - 12px);
  overflow-y: auto;
  border: 1px solid #b9c3cf;
  border-radius: 7px;
  background: #fff;
  box-shadow: 0 30px 80px -34px rgba(15, 23, 42, 0.8);
}
.wf-run-dialog > header { min-height: 62px; display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 11px 14px; border-bottom: 1px solid #dbe1e8; }
.wf-run-dialog > header div { display: grid; gap: 2px; }
.wf-run-dialog > header span { color: #64748b; font-size: 9px; font-weight: 800; text-transform: uppercase; }
.wf-run-dialog h2 { margin: 0; color: #18212b; font-size: 16px; letter-spacing: 0; }
.wf-run-summary { display: grid; grid-template-columns: repeat(3, 1fr); border-bottom: 1px solid #dbe1e8; }
.wf-run-summary > div { min-width: 0; display: grid; gap: 3px; padding: 11px 14px; border-right: 1px solid #e2e8f0; }
.wf-run-summary > div:last-child { border-right: 0; }
.wf-run-summary span { color: #718096; font-size: 9px; }
.wf-run-summary strong { overflow: hidden; color: #253142; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.wf-run-reuse { min-height: 58px; display: flex; align-items: center; gap: 10px; padding: 10px 14px; border-bottom: 1px solid #e2e8f0; }
.wf-run-reuse input { width: 16px; height: 16px; accent-color: #2563eb; }
.wf-run-reuse > span { display: grid; gap: 2px; }
.wf-run-reuse strong { color: #334155; font-size: 11px; }
.wf-run-reuse small { color: #84909d; font-size: 9px; }
.wf-run-issues { display: grid; gap: 5px; padding: 10px 14px 4px; }
.wf-run-issues button { display: flex; align-items: flex-start; gap: 9px; padding: 8px; border: 1px solid #fed7aa; border-radius: 4px; background: #fff7ed; color: #9a3412; text-align: left; }
.wf-run-issues button > span { display: grid; gap: 2px; }
.wf-run-issues strong { font-size: 10px; }
.wf-run-issues small { color: #b45309; font-size: 9px; }
.wf-run-ready { margin: 11px 14px 4px; display: flex; align-items: center; gap: 8px; padding: 9px; border-left: 3px solid #059669; background: #ecfdf5; color: #047857; font-size: 10px; }
.wf-run-plan { max-height: 112px; overflow-y: auto; display: flex; flex-wrap: wrap; align-content: flex-start; gap: 5px; padding: 10px 14px 14px; }
.wf-run-plan span { padding: 4px 6px; border: 1px solid #d5dde6; border-radius: 3px; background: #f8fafc; color: #52606d; font-size: 9px; }
.wf-run-dialog > footer { display: flex; justify-content: flex-end; gap: 8px; padding: 11px 14px; border-top: 1px solid #dbe1e8; background: #f8fafc; }

:deep(.vue-flow__selection) { border: 1px solid #2563eb; background: rgba(37, 99, 235, 0.08); }
:deep(.vue-flow__edge.selected .vue-flow__edge-path) { filter: drop-shadow(0 0 2px rgba(37, 99, 235, 0.42)); }

.wf-inspector {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 36;
  width: 320px;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  overflow: hidden;
  border-left: 1px solid var(--wf-line);
  background: #fbfcfd;
  box-shadow: -12px 0 28px -24px rgba(15, 23, 42, 0.5);
  animation: wf-inspector-in 150ms ease-out;
}

.wf-inspector__head {
  min-height: 62px;
  justify-content: space-between;
  gap: 10px;
  padding: 12px 14px;
  border-bottom: 1px solid #e1e6eb;
  background: #fff;
}
.wf-inspector__head > div { min-width: 0; display: grid; gap: 3px; }
.wf-inspector__head strong { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.wf-inspector__tools { display: flex; align-items: center; gap: 5px; }
.wf-inspector__tools .wf-icon-button { width: 30px; height: 30px; }
.wf-inspector__body { min-height: 0; overflow-y: auto; padding: 14px; }
.wf-inspector__actions { padding: 11px 14px; border-top: 1px solid #dde3e9; background: #f6f8fa; }
.wf-field { display: grid; gap: 6px; margin-bottom: 14px; }
.wf-field > span,
.wf-toggle span { color: #475569; font-size: 11px; font-weight: 700; }
.wf-field input,
.wf-field textarea,
.wf-field select {
  width: 100%;
  border: 1px solid #cfd8e3;
  border-radius: 5px;
  background: #fff;
  color: #111827;
  font-size: 12px;
}
.wf-field input,
.wf-field select { height: 36px; padding: 0 9px; }
.wf-field textarea { min-height: 86px; padding: 8px 9px; resize: vertical; line-height: 1.5; }
.wf-field input:focus,
.wf-field textarea:focus,
.wf-field select:focus { outline: 2px solid rgba(37, 99, 235, 0.18); border-color: #2563eb; }
.wf-field--grow textarea { min-height: 110px; }
.wf-field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 9px; }
.wf-segments { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; gap: 5px; }
.wf-segments button { min-height: 32px; border: 1px solid #cfd8e3; border-radius: 4px; background: #fff; color: #64748b; font-size: 10px; }
.wf-segments button.active { border-color: #2563eb; background: #eff6ff; color: #1d4ed8; font-weight: 700; }
.wf-toggle { min-height: 36px; gap: 8px; margin-bottom: 14px; }
.wf-toggle input { width: 16px; height: 16px; accent-color: #2563eb; }
.wf-resolution-preview { justify-content: space-between; margin: -2px 0 15px; padding: 8px 9px; border-left: 3px solid #2563eb; background: #eef4ff; }
.wf-resolution-preview span { color: #64748b; font-size: 10px; }
.wf-resolution-preview strong { color: #1d4ed8; font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 11px; }

.wf-asset-details { margin: 0 0 15px; }
.wf-asset-details div { display: flex; justify-content: space-between; gap: 12px; padding: 8px 0; border-bottom: 1px solid #e2e8f0; }
.wf-asset-details dt { color: #64748b; font-size: 10px; }
.wf-asset-details dd { margin: 0; overflow: hidden; color: #1f2937; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.wf-inspector__wide-action { width: 100%; justify-content: center; margin-bottom: 14px; }
.wf-inspector__error { margin: 12px 0 0; padding: 8px; border-left: 3px solid #dc2626; background: #fef2f2; color: #b91c1c; font-size: 10px; line-height: 1.5; }
.wf-inspector__run { width: 100%; margin: 0; justify-content: center; }
:deep(.vue-flow__connection-path) { stroke: #52606d; stroke-width: 2; stroke-dasharray: 6 5; vector-effect: non-scaling-stroke; }
:deep(.vue-flow__edge-interaction) { stroke-width: 24; }
:deep(.wf-minimap) { right: 10px; bottom: 10px; width: 136px; height: 88px; border: 1px solid #cbd2da; border-radius: 4px; background: rgba(250, 251, 252, 0.96); box-shadow: 0 6px 20px rgba(32, 38, 46, 0.08); }
:deep(.wf-controls) { left: 10px; bottom: 10px; overflow: hidden; border: 1px solid #cbd2da; border-radius: 4px; box-shadow: 0 5px 16px rgba(32, 38, 46, 0.08); }
:deep(.wf-controls button) { width: 28px; height: 28px; border-bottom-color: #e2e8f0; background: #fff; }
.wf-workspace--inspecting .wf-run-progress { right: 332px; }
.wf-workspace--inspecting :deep(.wf-minimap) { right: 332px; }

@keyframes wf-inspector-in {
  from { opacity: 0; transform: translateX(8px); }
  to { opacity: 1; transform: translateX(0); }
}

@keyframes wf-pulse {
  0%, 100% { opacity: 0.35; transform: scale(0.85); }
  50% { opacity: 1; transform: scale(1.25); }
}

@media (max-width: 1180px) {
  .wf-workspace,
  .wf-workspace--inspecting { grid-template-columns: 150px minmax(440px, 1fr); }
  .wf-palette { padding-inline: 5px; }
  .wf-tool { padding-inline: 7px; }
  .wf-tool small { display: none; }
  .wf-inspector { width: 300px; }
  .wf-workspace--inspecting .wf-run-progress { right: 312px; }
  .wf-workspace--inspecting :deep(.wf-minimap) { right: 312px; }
}

@media (max-width: 900px) {
  .wf-shell { height: auto; min-height: auto; overflow: visible; }
  .wf-commandbar { align-items: flex-start; flex-wrap: wrap; }
  .wf-conflict-bar { align-items: flex-start; flex-direction: column; }
  .wf-conflict-bar > div { width: 100%; justify-content: flex-end; }
  .wf-document { width: 100%; }
  .wf-document__name { width: min(60vw, 360px); }
  .wf-commandbar__actions { width: 100%; justify-content: flex-end; }
  .wf-workspace,
  .wf-workspace--inspecting { height: auto; min-height: 0; flex: none; overflow: visible; display: grid; grid-template-columns: 1fr; grid-template-rows: auto minmax(560px, 68vh) auto; }
  .wf-palette { display: flex; flex-flow: row wrap; align-items: stretch; gap: 7px; overflow: visible; border-right: 0; border-bottom: 1px solid var(--wf-line); }
  .wf-palette__heading,
  .wf-palette__divider,
  .wf-task-link { display: none; }
  .wf-palette-tabs { flex: 1 0 100%; margin-bottom: 0; }
  .wf-tool { width: auto; min-height: 46px; margin: 0; flex: 1 1 132px; flex-direction: row; justify-content: flex-start; text-align: left; }
  .wf-asset-search,
  .wf-upload-card,
  .wf-asset-list,
  .wf-palette-empty { flex: 1 0 100%; }
  .wf-asset-list { max-height: 156px; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .wf-template-card { width: auto; margin-bottom: 0; flex: 1 1 180px; }
  .wf-inspector { position: relative; inset: auto; z-index: auto; width: auto; min-height: 360px; border-left: 0; border-top: 1px solid var(--wf-line); box-shadow: none; }
  .wf-workspace--inspecting .wf-run-progress { right: 10px; }
  .wf-workspace--inspecting :deep(.wf-minimap) { right: 10px; }
  .wf-selection-toolbar { max-width: calc(100% - 28px); }
  .wf-run-progress { top: 10px; right: 10px; }
}

@media (max-width: 560px) {
  .wf-save-state { display: none; }
  .wf-commandbar__actions > .wf-icon-button,
  .wf-commandbar__actions .btn-secondary { display: none; }
  .wf-commandbar__actions { align-items: center; }
  .wf-edit-group .wf-icon-button { width: 29px; }
  .wf-edit-group .wf-icon-button:last-child { display: none; }
  .wf-run-group { min-width: 0; flex: 1; }
  .wf-run-button { min-width: 0; flex: 1; width: auto; }
  .wf-palette { gap: 5px; }
  .wf-tool { min-width: 104px; justify-content: flex-start; padding-inline: 7px; gap: 5px; }
  .wf-tool strong { font-size: 10px; white-space: nowrap; }
  .wf-asset-list { grid-template-columns: 1fr; }
  .wf-template-card { flex-basis: 100%; }
  .wf-selection-toolbar { bottom: 10px; }
  .wf-selection-toolbar > span { max-width: 124px; overflow: hidden; text-overflow: ellipsis; }
  .wf-command-overlay { padding: 24px 10px 10px; }
  .wf-command-palette > div { max-height: min(420px, calc(100vh - 190px)); }
  .wf-dialog-backdrop { padding: 10px; }
  .wf-run-summary { grid-template-columns: 1fr; }
  .wf-run-summary > div { grid-template-columns: 90px 1fr; align-items: center; border-right: 0; border-bottom: 1px solid #e2e8f0; }
  .wf-run-summary > div:last-child { border-bottom: 0; }
  .wf-run-menu { width: min(250px, calc(100vw - 24px)); }
  :deep(.wf-minimap) { display: none; }
}

@media (prefers-reduced-motion: reduce) {
  .wf-tool,
  .wf-node__status i,
  .wf-inspector { transition: none; animation: none; }
  .wf-workspace { transition: none; }
}
</style>
