<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-normal text-gray-950 dark:text-white">
            {{ t('creazyCanvas.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('creazyCanvas.subtitle') }}
          </p>
        </div>
        <div class="w-full sm:w-96 space-y-2">
          <div>
            <label class="input-label">{{ t('creazyCanvas.key.label') }}</label>
            <select
              v-model.number="selectedKeyId"
              class="input"
              :disabled="loadingKeys"
              @change="onKeyChange"
            >
              <option :value="0">
                {{ loadingKeys ? t('creazyCanvas.key.loading') : t('creazyCanvas.key.placeholder') }}
              </option>
              <option v-for="item in keys" :key="item.id" :value="item.id">
                {{ keyLabel(item) }}
              </option>
            </select>
            <p v-if="!loadingKeys && keys.length === 0" class="input-hint text-amber-600 dark:text-amber-400">
              {{ t('creazyCanvas.key.empty') }}
            </p>
          </div>
          <div v-if="selectedKeyId" class="rounded-xl border border-gray-200 bg-gray-50/80 p-3 dark:border-dark-700 dark:bg-dark-900/40">
            <div class="flex flex-wrap items-center gap-2">
              <span
                class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold"
                :class="keyReadyChipClass"
              >
                <span class="h-1.5 w-1.5 rounded-full" :class="keyReadyDotClass" />
                {{ keyReadyLabel }}
              </span>
              <span class="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100">
                {{ selectedKeyLabel }}
              </span>
            </div>
            <p class="mt-1.5 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('creazyCanvas.key.selectOnlyHint') }}
            </p>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap gap-2 border-b border-gray-200 pb-2 dark:border-dark-700">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
          :class="
            activeTab === tab.id
              ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'
          "
          @click="switchTab(tab.id)"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- Image -->
      <section v-if="activeTab === 'image'" class="grid gap-5 lg:grid-cols-2">
        <div class="card space-y-4 p-5">
          <div>
            <label class="input-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <textarea
              v-model="imageForm.prompt"
              rows="5"
              class="input"
              :placeholder="t('creazyCanvas.form.promptPlaceholder')"
            />
          </div>
          <div class="rounded-xl border border-indigo-200/80 bg-gradient-to-br from-indigo-50/90 to-white p-3 dark:border-indigo-900/50 dark:from-indigo-950/30 dark:to-dark-900">
            <div class="mb-2 flex items-center gap-2">
              <span class="inline-flex h-6 items-center rounded-md bg-indigo-600 px-2 text-[11px] font-semibold uppercase tracking-wide text-white">
                {{ t('creazyCanvas.form.model') }}
              </span>
              <span v-if="imageForm.model" class="truncate text-xs font-medium text-indigo-700 dark:text-indigo-300">
                {{ selectedImageModel?.name || imageForm.model }}
              </span>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <select v-model="imageForm.model" class="input border-indigo-200 focus:border-indigo-400 dark:border-indigo-800" :disabled="loadingCatalog">
                  <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : '—' }}</option>
                  <option v-for="m in imageModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
                </select>
                <p
                  v-if="!loadingCatalog && selectedKeyId && imageModels.length === 0"
                  class="input-hint text-amber-600 dark:text-amber-400"
                >
                  {{ t('creazyCanvas.catalog.emptyImage') }}
                </p>
              </div>
              <div>
                <label class="input-label">{{ t('creazyCanvas.form.size') }}</label>
                <select v-model="imageForm.size" class="input">
                  <option v-for="s in imageSizeOptions" :key="s" :value="s">{{ s }}</option>
                </select>
              </div>
            </div>
          </div>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="submittingImage || !selectedKeyId || resolvingKeySecret || !hasKeySecret"
            @click="generateImage"
          >
            <Icon v-if="submittingImage" name="refresh" size="sm" class="mr-2 animate-spin" />
            {{ submittingImage ? t('creazyCanvas.form.submitting') : t('creazyCanvas.form.generate') }}
          </button>
          <p v-if="imageRunningCount" class="text-xs text-amber-600 dark:text-amber-400">
            {{ t('creazyCanvas.tasks.runningCount', { n: imageRunningCount }) }}
          </p>
          <p v-if="imageError" class="text-sm text-red-600 dark:text-red-400">{{ imageError }}</p>
          <p v-if="imageSaveMessage" class="text-sm text-green-600 dark:text-green-400">{{ imageSaveMessage }}</p>
        </div>

        <div class="card space-y-4 p-5">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="() => loadWorks()">
              {{ t('creazyCanvas.works.refresh') }}
            </button>
          </div>

          <div v-if="imageResultUrls.length" class="space-y-2 rounded-xl border border-emerald-200/70 bg-emerald-50/40 p-3 dark:border-emerald-900/40 dark:bg-emerald-950/20">
            <p class="text-xs font-semibold text-emerald-700 dark:text-emerald-300">{{ t('creazyCanvas.tasks.latestPreview') }}</p>
            <div class="grid gap-2 sm:grid-cols-2">
              <button
                v-for="(url, idx) in imageResultUrls"
                :key="'latest-img-' + idx"
                type="button"
                class="overflow-hidden rounded-lg border border-emerald-100 bg-white dark:border-emerald-900/40 dark:bg-dark-900"
                @click="openMediaPreview({ type: 'image', url })"
              >
                <img :src="url" alt="latest" class="max-h-40 w-full object-contain" />
              </button>
            </div>
          </div>

          <div v-if="!imageTaskWorks.length" class="flex min-h-[180px] items-center justify-center rounded-lg border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
            {{ t('creazyCanvas.tasks.empty') }}
          </div>
          <div v-else class="space-y-3">
            <article
              v-for="work in imageTaskWorks"
              :key="work.id"
              class="rounded-xl border p-3 shadow-sm transition-colors"
              :class="workCardClass(work)"
            >
              <div class="flex flex-wrap items-center gap-2">
                <span class="badge inline-flex items-center gap-1.5" :class="workStatusClass(work.status)">
                  <span class="h-1.5 w-1.5 rounded-full" :class="workStatusDotClass(work.status)" />
                  {{ workStatusLabel(work.status) }}
                </span>
                <span class="inline-flex max-w-full items-center gap-1 rounded-md border border-indigo-200 bg-indigo-50 px-2 py-0.5 text-[11px] font-semibold text-indigo-700 dark:border-indigo-800/60 dark:bg-indigo-950/40 dark:text-indigo-300">
                  {{ work.public_model || '—' }}
                </span>
                <span v-if="work.created_at" class="text-[11px] text-gray-500">{{ formatDateTime(work.created_at) }}</span>
              </div>
              <p class="mt-2 line-clamp-2 text-sm text-gray-800 dark:text-gray-100">{{ work.prompt || ('#' + work.id) }}</p>
              <p v-if="work.error_message" class="mt-1 line-clamp-2 text-xs text-red-600 dark:text-red-300">{{ work.error_message }}</p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-if="canPreviewWork(work)"
                  type="button"
                  class="btn btn-primary btn-sm"
                  :disabled="workPreviewLoading[String(work.id)]"
                  @click="openWorkPreview(work)"
                >
                  {{ workPreviewLoading[String(work.id)] ? t('creazyCanvas.works.previewLoading') : t('creazyCanvas.works.preview') }}
                </button>
                <button
                  v-if="canPreviewWork(work)"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="downloadingWorkId === String(work.id)"
                  @click="downloadWork(work)"
                >
                  {{ t('creazyCanvas.works.download') }}
                </button>
                <button type="button" class="btn btn-secondary btn-sm" @click="reuseWork(work)">
                  {{ t('creazyCanvas.works.reuse') }}
                </button>
              </div>
            </article>
          </div>
        </div>
      </section>

      <!-- Video -->
      <section v-else-if="activeTab === 'video'" class="grid gap-5 lg:grid-cols-2">
        <div class="card space-y-4 p-5">
          <div>
            <label class="input-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <textarea
              v-model="videoForm.prompt"
              rows="5"
              class="input"
              :placeholder="t('creazyCanvas.form.promptPlaceholder')"
            />
          </div>
          <div class="rounded-xl border border-violet-200/80 bg-gradient-to-br from-violet-50/90 to-white p-3 dark:border-violet-900/50 dark:from-violet-950/30 dark:to-dark-900">
            <div class="mb-2 flex items-center gap-2">
              <span class="inline-flex h-6 items-center rounded-md bg-violet-600 px-2 text-[11px] font-semibold uppercase tracking-wide text-white">
                {{ t('creazyCanvas.form.model') }}
              </span>
              <span v-if="videoForm.model" class="truncate text-xs font-medium text-violet-700 dark:text-violet-300">
                {{ selectedVideoModel?.name || videoForm.model }}
              </span>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <select v-model="videoForm.model" class="input border-violet-200 focus:border-violet-400 dark:border-violet-800" :disabled="loadingCatalog">
                  <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : '—' }}</option>
                  <option v-for="m in videoModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
                </select>
                <p
                  v-if="!loadingCatalog && selectedKeyId && videoModels.length === 0"
                  class="input-hint text-amber-600 dark:text-amber-400"
                >
                  {{ t('creazyCanvas.catalog.emptyVideo') }}
                </p>
              </div>
              <div>
                <label class="input-label">{{ t('creazyCanvas.form.resolution') }}</label>
                <select v-model="videoForm.resolution" class="input">
                  <option v-for="r in videoResolutionOptions" :key="r" :value="r">{{ r }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('creazyCanvas.form.duration') }}</label>
                <select v-model.number="videoForm.duration" class="input">
                  <option v-for="d in videoDurationOptions" :key="d" :value="d">{{ d }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('creazyCanvas.form.aspectRatio') }}</label>
                <select v-model="videoForm.aspectRatio" class="input">
                  <option v-for="a in videoAspectOptions" :key="a" :value="a">{{ a }}</option>
                </select>
              </div>
            </div>
          </div>

          <label
            v-if="mediaCaps.allowGeneratedAudio"
            class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"
            :title="mediaCaps.forceGeneratedAudio ? t('creazyCanvas.form.forceGeneratedAudio') : undefined"
          >
            <input
              v-model="videoForm.generateAudio"
              type="checkbox"
              class="rounded border-gray-300"
              :disabled="mediaCaps.forceGeneratedAudio"
            />
            {{ t('creazyCanvas.form.generateAudio') }}
            <span v-if="mediaCaps.forceGeneratedAudio" class="text-xs text-gray-500">
              ({{ t('creazyCanvas.form.forceGeneratedAudio') }})
            </span>
          </label>

          <div
            v-if="selectedVideoModel"
            class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs text-gray-600 dark:border-dark-700 dark:bg-dark-900/40 dark:text-gray-300"
          >
            <div class="font-medium text-gray-800 dark:text-gray-100">
              {{ t('creazyCanvas.form.mediaLimits') }}
            </div>
            <div class="mt-1">
              {{
                t('creazyCanvas.form.mediaLimitsDetail', {
                  images: mediaCaps.maxImages,
                  videos: mediaCaps.maxVideos,
                  audios: mediaCaps.maxAudios,
                  total: mediaCaps.maxTotal
                    ? t('creazyCanvas.form.mediaTotal', { total: mediaCaps.maxTotal })
                    : '',
                })
              }}
              <span v-if="mediaCaps.maxTotalImages > 0">
                {{ t('creazyCanvas.form.mediaTotalImages', { max: mediaCaps.maxTotalImages }) }}
              </span>
            </div>
            <p class="mt-1 text-gray-500 dark:text-gray-400">
              {{ t('creazyCanvas.form.optionalMediaHint') }}
            </p>
            <p v-if="mediaCaps.requireStartFrame" class="mt-1 text-amber-700 dark:text-amber-300">
              {{ t('creazyCanvas.form.constraintHintRequireStart') }}
            </p>
            <p v-if="mediaCaps.allowEndFrame" class="mt-1 text-amber-700 dark:text-amber-300">
              {{ t('creazyCanvas.form.constraintHintEndNeedsStart') }}
            </p>
            <p v-if="mediaCaps.framesExclusiveWithRefs" class="mt-1 text-amber-700 dark:text-amber-300">
              {{ t('creazyCanvas.form.constraintHintExclusiveModes') }}
            </p>
          </div>

          <div class="space-y-3">
            <div v-if="mediaCaps.allowStartFrame">
              <label class="input-label">{{ mediaCaps.requireStartFrame ? t('creazyCanvas.form.startFrameRequired') : t('creazyCanvas.form.startFrame') }}</label>
              <div class="flex flex-wrap items-center gap-2">
                <input ref="startFrameInput" type="file" accept="image/*" class="hidden" @change="onPickStartFrame" />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || frameUploadsBlocked"
                  :title="frameUploadsBlocked ? t('creazyCanvas.form.framesExclusiveWithRefs') : undefined"
                  @click="startFrameInput?.click()"
                >
                  {{ uploadingMedia === 'start' ? t('creazyCanvas.form.uploading') : t('creazyCanvas.form.upload') }}
                </button>
                <span v-if="startFrame?.name" class="text-xs text-gray-600 dark:text-gray-300">{{ startFrame.name }}</span>
                <button v-if="startFrame" type="button" class="btn btn-secondary btn-sm" @click="clearStartFrame">
                  {{ t('creazyCanvas.form.remove') }}
                </button>
              </div>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <input
                  v-model="startFrameUrlInput"
                  type="url"
                  class="input flex-1 min-w-[16rem] font-mono text-xs"
                  :placeholder="t('creazyCanvas.form.startFrameUrlPlaceholder')"
                  :disabled="!!uploadingMedia || !selectedKeyId"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || !startFrameUrlInput.trim()"
                  @click="applyStartFrameUrl"
                >
                  {{ t('creazyCanvas.form.applyUrl') }}
                </button>
              </div>
            </div>

            <div v-if="mediaCaps.allowEndFrame">
              <label class="input-label">{{ t('creazyCanvas.form.endFrame') }}</label>
              <div class="flex flex-wrap items-center gap-2">
                <input ref="endFrameInput" type="file" accept="image/*" class="hidden" @change="onPickEndFrame" />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || !startFrame || frameUploadsBlocked"
                  :title="
                    !startFrame
                      ? t('creazyCanvas.form.endNeedsStart')
                      : frameUploadsBlocked
                        ? t('creazyCanvas.form.framesExclusiveWithRefs')
                        : undefined
                  "
                  @click="endFrameInput?.click()"
                >
                  {{ uploadingMedia === 'end' ? t('creazyCanvas.form.uploading') : t('creazyCanvas.form.upload') }}
                </button>
                <span v-if="endFrame?.name" class="text-xs text-gray-600 dark:text-gray-300">{{ endFrame.name }}</span>
                <button v-if="endFrame" type="button" class="btn btn-secondary btn-sm" @click="endFrame = null">
                  {{ t('creazyCanvas.form.remove') }}
                </button>
              </div>
            </div>

            <div v-if="mediaCaps.maxImages > 0">
              <label class="input-label flex items-center justify-between gap-2">
                <span>{{ t('creazyCanvas.form.refImages') }} ({{ refImages.length }}/{{ mediaCaps.maxImages }})</span>
                <button
                  v-if="refImages.length"
                  type="button"
                  class="text-[11px] font-medium text-rose-600 hover:underline dark:text-rose-400"
                  :disabled="!!uploadingMedia"
                  @click="clearRefImages"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </label>
              <div class="flex flex-wrap items-center gap-2">
                <input
                  ref="refImageInput"
                  type="file"
                  accept="image/*"
                  multiple
                  class="hidden"
                  @change="onPickRefImage"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || refImages.length >= mediaCaps.maxImages || refUploadsBlocked"
                  :title="refUploadsBlocked ? t('creazyCanvas.form.refsExclusiveWithFrames') : undefined"
                  @click="refImageInput?.click()"
                >
                  {{
                    uploadingMedia === 'ref-image'
                      ? uploadProgressLabel || t('creazyCanvas.form.uploading')
                      : mediaCaps.maxImages >= 8
                        ? t('creazyCanvas.form.uploadMultiple')
                        : t('creazyCanvas.form.upload')
                  }}
                </button>
              </div>
              <ul
                v-if="refImages.length"
                class="mt-2 max-h-44 overflow-y-auto rounded-md border border-gray-200 bg-white/70 divide-y divide-gray-100 dark:border-dark-600 dark:bg-dark-900/50 dark:divide-dark-700"
              >
                <li
                  v-for="(item, idx) in refImages"
                  :key="'img-' + idx + '-' + item.media_url"
                  class="flex items-center gap-2 px-2 py-1.5 text-xs text-gray-700 dark:text-gray-200"
                >
                  <div
                    class="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded bg-violet-50 text-[10px] font-semibold text-violet-600 dark:bg-violet-950/40 dark:text-violet-300"
                  >
                    <img
                      v-if="item.preview_url"
                      :src="item.preview_url"
                      alt=""
                      class="h-full w-full object-cover"
                    />
                    <span v-else>IMG</span>
                  </div>
                  <span class="min-w-0 flex-1 truncate" :title="item.name">{{ item.name }}</span>
                  <span class="shrink-0 text-[10px] text-gray-400">#{{ idx + 1 }}</span>
                  <button type="button" class="btn btn-secondary btn-sm shrink-0 px-2 py-0.5" @click="removeRefImage(idx)">
                    {{ t('creazyCanvas.form.remove') }}
                  </button>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.maxVideos > 0">
              <label class="input-label flex items-center justify-between gap-2">
                <span>{{ t('creazyCanvas.form.refVideos') }} ({{ refVideos.length }}/{{ mediaCaps.maxVideos }})</span>
                <button
                  v-if="refVideos.length"
                  type="button"
                  class="text-[11px] font-medium text-rose-600 hover:underline dark:text-rose-400"
                  :disabled="!!uploadingMedia"
                  @click="clearRefVideos"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </label>
              <div class="flex flex-wrap items-center gap-2">
                <input
                  ref="refVideoInput"
                  type="file"
                  accept="video/*"
                  multiple
                  class="hidden"
                  @change="onPickRefVideo"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || refVideos.length >= mediaCaps.maxVideos"
                  @click="refVideoInput?.click()"
                >
                  {{
                    uploadingMedia === 'ref-video'
                      ? uploadProgressLabel || t('creazyCanvas.form.uploading')
                      : mediaCaps.maxVideos >= 5
                        ? t('creazyCanvas.form.uploadMultiple')
                        : t('creazyCanvas.form.upload')
                  }}
                </button>
              </div>
              <ul
                v-if="refVideos.length"
                class="mt-2 max-h-36 overflow-y-auto rounded-md border border-gray-200 bg-white/70 divide-y divide-gray-100 dark:border-dark-600 dark:bg-dark-900/50 dark:divide-dark-700"
              >
                <li
                  v-for="(item, idx) in refVideos"
                  :key="'vid-' + idx + '-' + item.media_url"
                  class="flex items-center gap-2 px-2 py-1.5 text-xs text-gray-700 dark:text-gray-200"
                >
                  <div
                    class="flex h-8 w-8 shrink-0 items-center justify-center rounded bg-sky-50 text-[10px] font-semibold text-sky-600 dark:bg-sky-950/40 dark:text-sky-300"
                  >
                    VID
                  </div>
                  <span class="min-w-0 flex-1 truncate" :title="item.name">
                    {{ item.name }}
                    <span class="text-gray-400">({{ item.duration_seconds }}s)</span>
                  </span>
                  <span class="shrink-0 text-[10px] text-gray-400">#{{ idx + 1 }}</span>
                  <button type="button" class="btn btn-secondary btn-sm shrink-0 px-2 py-0.5" @click="refVideos.splice(idx, 1)">
                    {{ t('creazyCanvas.form.remove') }}
                  </button>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.maxAudios > 0">
              <label class="input-label flex items-center justify-between gap-2">
                <span>{{ t('creazyCanvas.form.refAudios') }} ({{ refAudios.length }}/{{ mediaCaps.maxAudios }})</span>
                <button
                  v-if="refAudios.length"
                  type="button"
                  class="text-[11px] font-medium text-rose-600 hover:underline dark:text-rose-400"
                  :disabled="!!uploadingMedia"
                  @click="clearRefAudios"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </label>
              <div class="flex flex-wrap items-center gap-2">
                <input
                  ref="refAudioInput"
                  type="file"
                  accept="audio/*"
                  multiple
                  class="hidden"
                  @change="onPickRefAudio"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingMedia || !selectedKeyId || refAudios.length >= mediaCaps.maxAudios || refUploadsBlocked"
                  :title="refUploadsBlocked ? t('creazyCanvas.form.refsExclusiveWithFrames') : undefined"
                  @click="refAudioInput?.click()"
                >
                  {{
                    uploadingMedia === 'ref-audio'
                      ? uploadProgressLabel || t('creazyCanvas.form.uploading')
                      : mediaCaps.maxAudios >= 5
                        ? t('creazyCanvas.form.uploadMultiple')
                        : t('creazyCanvas.form.upload')
                  }}
                </button>
              </div>
              <ul
                v-if="refAudios.length"
                class="mt-2 max-h-36 overflow-y-auto rounded-md border border-gray-200 bg-white/70 divide-y divide-gray-100 dark:border-dark-600 dark:bg-dark-900/50 dark:divide-dark-700"
              >
                <li
                  v-for="(item, idx) in refAudios"
                  :key="'aud-' + idx + '-' + item.media_url"
                  class="flex items-center gap-2 px-2 py-1.5 text-xs text-gray-700 dark:text-gray-200"
                >
                  <div
                    class="flex h-8 w-8 shrink-0 items-center justify-center rounded bg-emerald-50 text-[10px] font-semibold text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-300"
                  >
                    AUD
                  </div>
                  <span class="min-w-0 flex-1 truncate" :title="item.name">
                    {{ item.name }}
                    <span class="text-gray-400">({{ item.duration_seconds }}s)</span>
                  </span>
                  <span class="shrink-0 text-[10px] text-gray-400">#{{ idx + 1 }}</span>
                  <button type="button" class="btn btn-secondary btn-sm shrink-0 px-2 py-0.5" @click="refAudios.splice(idx, 1)">
                    {{ t('creazyCanvas.form.remove') }}
                  </button>
                </li>
              </ul>
            </div>
          </div>

          <button
            type="button"
            class="btn btn-primary"
            :disabled="submittingVideo || !!uploadingMedia || !selectedKeyId || resolvingKeySecret || !hasKeySecret"
            @click="generateVideo"
          >
            <Icon v-if="submittingVideo" name="refresh" size="sm" class="mr-2 animate-spin" />
            {{ submittingVideo ? t('creazyCanvas.form.submitting') : t('creazyCanvas.form.generate') }}
          </button>
          <p v-if="videoRunningCount" class="text-xs text-amber-600 dark:text-amber-400">
            {{ t('creazyCanvas.tasks.runningCount', { n: videoRunningCount }) }}
          </p>
          <p v-if="videoStatus" class="text-sm text-gray-600 dark:text-gray-300">
            {{ t('creazyCanvas.result.status') }}: {{ videoStatus }}
          </p>
          <p v-if="videoError" class="text-sm text-red-600 dark:text-red-400">{{ videoError }}</p>
          <p v-if="videoSaveMessage" class="text-sm text-green-600 dark:text-green-400">{{ videoSaveMessage }}</p>
        </div>

        <div class="card space-y-4 p-5">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="() => loadWorks()">
              {{ t('creazyCanvas.works.refresh') }}
            </button>
          </div>

          <div v-if="videoResultUrl" class="space-y-2 rounded-xl border border-emerald-200/70 bg-emerald-50/40 p-3 dark:border-emerald-900/40 dark:bg-emerald-950/20">
            <p class="text-xs font-semibold text-emerald-700 dark:text-emerald-300">{{ t('creazyCanvas.tasks.latestPreview') }}</p>
            <div class="overflow-hidden rounded-lg border border-emerald-100 bg-black dark:border-emerald-900/40">
              <video
                :src="videoResultUrl"
                controls
                playsinline
                preload="metadata"
                :muted="false"
                class="max-h-64 w-full object-contain"
              />
            </div>
            <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.result.videoAudioHint') }}</p>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                @click="openMediaPreview({ type: 'video', url: videoResultUrl })"
              >
                {{ t('creazyCanvas.result.fullscreenPreview') }}
              </button>
              <a
                :href="videoResultUrl"
                :download="isBlobUrl(videoResultUrl) ? 'creazy-video.mp4' : undefined"
                target="_blank"
                rel="noopener"
                class="btn btn-secondary btn-sm"
              >
                {{ t('creazyCanvas.result.download') }}
              </a>
            </div>
          </div>

          <div v-if="!videoTaskWorks.length" class="flex min-h-[180px] items-center justify-center rounded-lg border border-dashed border-gray-200 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
            {{ t('creazyCanvas.tasks.empty') }}
          </div>
          <div v-else class="space-y-3">
            <article
              v-for="work in videoTaskWorks"
              :key="work.id"
              class="rounded-xl border p-3 shadow-sm transition-colors"
              :class="workCardClass(work)"
            >
              <div class="flex flex-wrap items-center gap-2">
                <span class="badge inline-flex items-center gap-1.5" :class="workStatusClass(work.status)">
                  <span class="h-1.5 w-1.5 rounded-full" :class="workStatusDotClass(work.status)" />
                  {{ workStatusLabel(work.status) }}
                </span>
                <span class="inline-flex max-w-full items-center gap-1 rounded-md border border-violet-200 bg-violet-50 px-2 py-0.5 text-[11px] font-semibold text-violet-700 dark:border-violet-800/60 dark:bg-violet-950/40 dark:text-violet-300">
                  {{ work.public_model || '—' }}
                </span>
                <span v-if="work.created_at" class="text-[11px] text-gray-500">{{ formatDateTime(work.created_at) }}</span>
              </div>
              <p class="mt-2 line-clamp-2 text-sm text-gray-800 dark:text-gray-100">{{ work.prompt || ('#' + work.id) }}</p>
              <p v-if="work.error_message" class="mt-1 line-clamp-2 text-xs text-red-600 dark:text-red-300">{{ work.error_message }}</p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-if="canPreviewWork(work)"
                  type="button"
                  class="btn btn-primary btn-sm"
                  :disabled="workPreviewLoading[String(work.id)]"
                  @click="openWorkPreview(work)"
                >
                  {{ workPreviewLoading[String(work.id)] ? t('creazyCanvas.works.previewLoading') : t('creazyCanvas.works.preview') }}
                </button>
                <button
                  v-if="canPreviewWork(work)"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="downloadingWorkId === String(work.id)"
                  @click="downloadWork(work)"
                >
                  {{ t('creazyCanvas.works.download') }}
                </button>
                <button type="button" class="btn btn-secondary btn-sm" @click="reuseWork(work)">
                  {{ t('creazyCanvas.works.reuse') }}
                </button>
              </div>
            </article>
          </div>
        </div>
      </section>

      <!-- Works -->
      <section v-else class="space-y-4">
        <div class="card overflow-hidden">
          <div class="border-b border-gray-100 bg-gradient-to-r from-primary-50/80 via-white to-white px-5 py-4 dark:border-dark-700 dark:from-primary-950/30 dark:via-dark-900 dark:to-dark-900">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div class="min-w-0">
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('creazyCanvas.works.title') }}
                </h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  <template v-if="!selectedKeyId">{{ t('creazyCanvas.works.selectKeyFirst') }}</template>
                  <template v-else>
                    {{ t('creazyCanvas.works.filteredByKey') }}
                    <span class="ml-1 inline-flex items-center rounded-md bg-primary-100 px-2 py-0.5 font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
                      {{ selectedKeyLabel }}
                    </span>
                  </template>
                </p>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <label class="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
                  <span>{{ t('creazyCanvas.works.filterKind') }}</span>
                  <select
                    v-model="worksFilterKind"
                    class="input min-w-[7rem] py-1 text-xs"
                    :disabled="!selectedKeyId || loadingWorks"
                    @change="() => loadWorks()"
                  >
                    <option value="">{{ t('creazyCanvas.works.filterAll') }}</option>
                    <option value="image">{{ t('creazyCanvas.works.image') }}</option>
                    <option value="video">{{ t('creazyCanvas.works.video') }}</option>
                  </select>
                </label>
                <label class="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
                  <span>{{ t('creazyCanvas.works.filterStatus') }}</span>
                  <select
                    v-model="worksFilterStatus"
                    class="input min-w-[7rem] py-1 text-xs"
                    :disabled="!selectedKeyId || loadingWorks"
                    @change="() => loadWorks()"
                  >
                    <option value="">{{ t('creazyCanvas.works.filterAll') }}</option>
                    <option value="succeeded">{{ t('creazyCanvas.works.statusLabels.succeeded') }}</option>
                    <option value="failed">{{ t('creazyCanvas.works.statusLabels.failed') }}</option>
                    <option value="queued">{{ t('creazyCanvas.works.statusLabels.queued') }}</option>
                    <option value="running">{{ t('creazyCanvas.works.statusLabels.running') }}</option>
                    <option value="created">{{ t('creazyCanvas.works.statusLabels.created') }}</option>
                    <option value="expired">{{ t('creazyCanvas.works.statusLabels.expired') }}</option>
                    <option value="canceled">{{ t('creazyCanvas.works.statusLabels.canceled') }}</option>
                  </select>
                </label>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!selectedKeyId || loadingWorks"
                  @click="() => loadWorks()"
                >
                  <Icon name="refresh" size="sm" class="mr-1.5" :class="loadingWorks ? 'animate-spin' : ''" />
                  {{ t('creazyCanvas.works.refresh') }}
                </button>
              </div>
            </div>

            <div v-if="selectedKeyId && worksStatusSummary.total > 0" class="mt-4 flex flex-wrap gap-2">
              <span
                v-for="item in worksStatusSummary.items"
                :key="item.key"
                class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium"
                :class="item.chipClass"
              >
                <span class="h-1.5 w-1.5 rounded-full" :class="item.dotClass" />
                {{ item.label }}
                <span class="tabular-nums opacity-80">{{ item.count }}</span>
              </span>
            </div>
          </div>

          <div class="p-5">
            <div
              v-if="worksNeedSecretBanner"
              class="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-xs leading-relaxed text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/40 dark:text-amber-200"
            >
              {{ t('creazyCanvas.works.needSecretBanner') }}
            </div>

            <div v-if="!selectedKeyId" class="flex min-h-[240px] flex-col items-center justify-center rounded-2xl border border-dashed border-gray-200 bg-gray-50/70 px-6 text-center dark:border-dark-700 dark:bg-dark-900/40">
              <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-100 text-primary-600 dark:bg-primary-900/40 dark:text-primary-300">
                <Icon name="key" size="md" />
              </div>
              <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('creazyCanvas.works.selectKeyFirst') }}</p>
              <p class="mt-1 max-w-md text-xs text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.works.selectKeyFirstHint') }}</p>
            </div>

            <div v-else-if="loadingWorks" class="space-y-3">
              <div v-for="i in 4" :key="i" class="h-28 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800" />
            </div>

            <div
              v-else-if="works.length === 0"
              class="flex min-h-[240px] flex-col items-center justify-center rounded-2xl border border-dashed border-gray-200 bg-gray-50/70 px-6 text-center dark:border-dark-700 dark:bg-dark-900/40"
            >
              <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('creazyCanvas.works.emptyForKey') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.works.emptyForKeyHint') }}</p>
            </div>

            <div v-else class="space-y-3">
              <article
                v-for="work in works"
                :key="String(work.id)"
                class="group relative overflow-hidden rounded-2xl border bg-white shadow-sm transition hover:shadow-md dark:bg-dark-900"
                :class="workCardClass(work)"
              >
                <div class="flex flex-col gap-4 p-4 sm:flex-row sm:items-stretch">
                  <!-- Cover -->
                  <div class="shrink-0">
                    <button
                      v-if="workCoverUrl(work)"
                      type="button"
                      class="relative h-28 w-full overflow-hidden rounded-xl border border-gray-200 bg-gray-50 sm:w-40 dark:border-dark-700 dark:bg-dark-800"
                      :title="t('creazyCanvas.works.preview')"
                      @click="openWorkPreview(work)"
                    >
                      <img
                        v-if="isImageWork(work) || workCoverIsImage(work)"
                        :src="workCoverUrl(work)"
                        alt=""
                        class="h-full w-full object-cover"
                      />
                      <video
                        v-else
                        :src="workCoverVideoSrc(work)"
                        muted
                        playsinline
                        preload="metadata"
                        class="h-full w-full object-cover"
                        @loadeddata="onCoverVideoLoaded"
                      />
                      <span
                        class="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/0 text-xs font-medium text-white opacity-0 transition group-hover:bg-black/35 group-hover:opacity-100"
                      >
                        {{ t('creazyCanvas.works.preview') }}
                      </span>
                    </button>
                    <button
                      v-else-if="canPreviewWork(work)"
                      type="button"
                      class="flex h-28 w-full flex-col items-center justify-center gap-1 rounded-xl border border-dashed px-2 text-center text-[11px] leading-tight sm:w-40"
                      :class="
                        workNeedsSecret(work)
                          ? 'border-amber-300 bg-amber-50/70 text-amber-700 dark:border-amber-700 dark:bg-amber-950/20 dark:text-amber-300'
                          : 'border-gray-300 bg-gray-50 text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400'
                      "
                      :disabled="workPreviewLoading[String(work.id)]"
                      @click="openWorkPreview(work)"
                    >
                      <template v-if="workPreviewLoading[String(work.id)]">
                        {{ t('creazyCanvas.works.previewLoading') }}
                      </template>
                      <template v-else-if="workNeedsSecret(work)">
                        <span class="font-semibold">{{ t('creazyCanvas.works.coverNeedSecret') }}</span>
                        <span class="opacity-80">{{ t('creazyCanvas.works.preview') }}</span>
                      </template>
                      <template v-else>
                        {{ t('creazyCanvas.works.preview') }}
                      </template>
                    </button>
                    <div
                      v-else
                      class="flex h-28 w-full flex-col items-center justify-center rounded-xl border border-gray-100 bg-gray-50 px-2 text-center text-[11px] text-gray-400 sm:w-40 dark:border-dark-700 dark:bg-dark-800"
                      :title="workCoverUnavailableReason(work)"
                    >
                      <span>{{ workCoverPlaceholder(work) }}</span>
                    </div>
                  </div>

                  <!-- Body -->
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="badge" :class="workKindClass(work.kind)">
                        {{ workTypeLabel(work.kind) }}
                      </span>
                      <span class="badge inline-flex items-center gap-1.5" :class="workStatusClass(work.status)">
                        <span class="h-1.5 w-1.5 rounded-full" :class="workStatusDotClass(work.status)" />
                        {{ workStatusLabel(work.status) }}
                      </span>
                      <span v-if="isExpired(work)" class="badge badge-danger">{{ t('creazyCanvas.works.expired') }}</span>
                    </div>

                    <p class="mt-2 line-clamp-2 text-sm font-medium leading-6 text-gray-900 dark:text-gray-50">
                      {{ work.prompt || work.public_model || `#${work.id}` }}
                    </p>

                    <div class="mt-3 flex flex-wrap items-center gap-2">
                      <span
                        class="inline-flex max-w-full items-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-2.5 py-1 text-xs font-semibold text-indigo-700 dark:border-indigo-800/60 dark:bg-indigo-950/40 dark:text-indigo-300"
                        :title="t('creazyCanvas.form.model')"
                      >
                        <span class="text-[10px] font-medium uppercase tracking-wide text-indigo-500 dark:text-indigo-400">
                          {{ t('creazyCanvas.form.model') }}
                        </span>
                        <span class="truncate">{{ work.public_model || '\u2014' }}</span>
                      </span>
                      <span
                        v-if="work.created_at"
                        class="inline-flex items-center rounded-lg bg-gray-100 px-2 py-1 text-[11px] text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                      >
                        {{ t('creazyCanvas.works.createdAt') }} · {{ formatDateTime(work.created_at) }}
                      </span>
                      <span
                        v-if="work.expires_at"
                        class="inline-flex items-center rounded-lg bg-gray-100 px-2 py-1 text-[11px] text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                      >
                        exp · {{ formatDateTime(work.expires_at) }}
                      </span>
                    </div>

                    <p
                      v-if="work.error_message"
                      class="mt-2 line-clamp-2 rounded-lg bg-red-50 px-2.5 py-1.5 text-xs text-red-600 dark:bg-red-950/30 dark:text-red-300"
                    >
                      {{ work.error_message }}
                    </p>
                  </div>

                  <!-- Actions -->
                  <div class="flex shrink-0 flex-wrap items-start gap-2 sm:flex-col sm:items-stretch">
                    <button
                      v-if="canPreviewWork(work)"
                      type="button"
                      class="btn btn-primary btn-sm sm:min-w-[6.5rem]"
                      :disabled="workPreviewLoading[String(work.id)]"
                      @click="openWorkPreview(work)"
                    >
                      {{
                        workPreviewLoading[String(work.id)]
                          ? t('creazyCanvas.works.previewLoading')
                          : t('creazyCanvas.works.preview')
                      }}
                    </button>
                    <button type="button" class="btn btn-secondary btn-sm sm:min-w-[6.5rem]" @click="reuseWork(work)">
                      {{ t('creazyCanvas.works.reuse') }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm sm:min-w-[6.5rem]"
                      :disabled="downloadingWorkId === String(work.id) || isExpired(work) || work.status === 'failed'"
                      @click="downloadWork(work)"
                    >
                      {{ t('creazyCanvas.works.download') }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm sm:min-w-[6.5rem]"
                      :disabled="deletingWorkId === String(work.id)"
                      @click="removeWork(work)"
                    >
                      {{ t('creazyCanvas.works.delete') }}
                    </button>
                  </div>
                </div>
              </article>
            </div>
          </div>
        </div>
      </section>
    </div>
    <!-- Media preview lightbox (image/video with sound) -->
    <Teleport to="body">
      <div
        v-if="mediaPreview"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
        role="dialog"
        aria-modal="true"
        @click.self="closeMediaPreview"
      >
        <button
          type="button"
          class="absolute right-4 top-4 rounded-full bg-white/10 px-3 py-1 text-sm text-white hover:bg-white/20"
          @click="closeMediaPreview"
        >
          {{ t('creazyCanvas.result.closePreview') }}
        </button>
        <img
          v-if="mediaPreview.type === 'image'"
          :src="mediaPreview.url"
          alt="preview"
          class="max-h-[90vh] max-w-[95vw] rounded-lg object-contain shadow-2xl"
          @click.stop
        />
        <video
          v-else
          :src="mediaPreview.url"
          controls
          autoplay
          playsinline
          preload="auto"
          :muted="false"
          class="max-h-[90vh] max-w-[95vw] rounded-lg bg-black shadow-2xl"
          @click.stop
        />
      </div>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  createWork,
  updateWork,
  deleteWork,
  generateImage as gatewayGenerateImage,
  generateVideo as gatewayGenerateVideo,
  getCatalog,
  getImageTask,
  getVideoContentURL,
  getVideoJob,
  getWorkDownloadURL,
  getWorkContentBlob,
  listKeys,
  listWorks,
  uploadVideoAsset,
  type CreateCreazyWorkRequest,
  type UpdateCreazyWorkRequest,
  type CreazyCanvasCatalog,
  type CreazyCanvasImageModel,
  type CreazyCanvasKey,
  type CreazyCanvasVideoModel,
  type CreazyWork,
} from '@/api/creazyCanvas'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

type TabId = 'image' | 'video' | 'works'
type MediaItem = { name: string; media_url: string; duration_seconds?: number; preview_url?: string }

const tabs = computed(() => [
  { id: 'image' as const, label: t('creazyCanvas.tabs.image') },
  { id: 'video' as const, label: t('creazyCanvas.tabs.video') },
  { id: 'works' as const, label: t('creazyCanvas.tabs.works') },
])

const activeTab = ref<TabId>('image')
const keys = ref<CreazyCanvasKey[]>([])
const selectedKeyId = ref(0)
const loadingKeys = ref(false)
const catalog = ref<CreazyCanvasCatalog | null>(null)
const loadingCatalog = ref(false)
/** Secret only in memory (hydrated from user /keys; never localStorage/sessionStorage) */
const keySecrets = reactive<Record<number, string>>({})
const resolvingKeySecret = ref(false)
const keySecretHydrated = ref(false)

const imageForm = reactive({
  prompt: '',
  model: '',
  size: '1024x1024',
})
const videoForm = reactive({
  prompt: '',
  model: '',
  resolution: '720p',
  duration: 5,
  aspectRatio: '16:9',
  generateAudio: true,
})

const startFrame = ref<MediaItem | null>(null)
const startFrameUrlInput = ref('')
const endFrame = ref<MediaItem | null>(null)
const refImages = ref<MediaItem[]>([])
const refVideos = ref<MediaItem[]>([])
const refAudios = ref<MediaItem[]>([])
const uploadingMedia = ref('')
const uploadProgressLabel = ref('')
const uploadProgress = ref({ done: 0, total: 0 })
const startFrameInput = ref<HTMLInputElement | null>(null)
const endFrameInput = ref<HTMLInputElement | null>(null)
const refImageInput = ref<HTMLInputElement | null>(null)
const refVideoInput = ref<HTMLInputElement | null>(null)
const refAudioInput = ref<HTMLInputElement | null>(null)

const submittingImage = ref(false)
const submittingVideo = ref(false)
const generatingImage = ref(false)
const generatingVideo = ref(false)
const activeImageJobs = ref(0)
const activeVideoJobs = ref(0)
const imageError = ref('')
const videoError = ref('')
const imageResultUrls = ref<string[]>([])
const videoResultUrl = ref('')
const videoStatus = ref('')
const videoJobId = ref('')
/** Non-blob URL kept for work persistence (blob: cannot be restored after refresh) */
const videoPersistUrl = ref('')
const imageSaveMessage = ref('')
const videoSaveMessage = ref('')
/** Fullscreen image/video preview */
const mediaPreview = ref<{ type: 'image' | 'video'; url: string } | null>(null)
/** Cached playable URLs for works list (may be blob:) */
const workPreviewUrls = reactive<Record<string, string>>({})
const workPreviewLoading = reactive<Record<string, boolean>>({})
const workPreviewBlobUrls = new Set<string>()

const works = ref<CreazyWork[]>([])
const loadingWorks = ref(false)
const worksFilterKind = ref('')
const worksFilterStatus = ref('')
const downloadingWorkId = ref('')
const deletingWorkId = ref('')
const lastImageTaskId = ref('')
const lastImageGatewayType = ref<'image_task' | 'image_sync' | ''>('')

let pollTimer: ReturnType<typeof setTimeout> | null = null
let worksPollTimer: ReturnType<typeof setTimeout> | null = null
let cancelled = false
const activeVideoWorkId = ref<number | null>(null)

const DEFAULT_IMAGE_SIZES = ['1024x1024', '1024x1792', '1792x1024', '1K', '2K', '4K']
const DEFAULT_VIDEO_RESOLUTIONS = ['480p', '720p', '1080p']
const DEFAULT_VIDEO_DURATIONS = [5, 10]
const DEFAULT_ASPECT_RATIOS = ['16:9', '9:16', '1:1']

const imageModels = computed<CreazyCanvasImageModel[]>(() => catalog.value?.image_models || [])
const videoModels = computed<CreazyCanvasVideoModel[]>(() => catalog.value?.video_models || [])
const selectedImageModel = computed(() => imageModels.value.find((m) => m.id === imageForm.model))
const selectedVideoModel = computed(() => videoModels.value.find((m) => m.id === videoForm.model))
const hasKeySecret = computed(() => {
  const id = selectedKeyId.value
  return Boolean(id && keySecrets[id])
})

const selectedKeyLabel = computed(() => {
  const id = selectedKeyId.value
  if (!id) return ''
  const item = keys.value.find((k) => Number(k.id) === Number(id))
  return item ? keyLabel(item) : `#${id}`
})

const keyReadyLabel = computed(() => {
  if (!selectedKeyId.value) return ''
  if (resolvingKeySecret.value) return t('creazyCanvas.key.resolving')
  if (hasKeySecret.value) return t('creazyCanvas.key.ready')
  return t('creazyCanvas.key.unavailable')
})

const keyReadyChipClass = computed(() => {
  if (resolvingKeySecret.value) {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300'
  }
  if (hasKeySecret.value) {
    return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300'
  }
  return 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300'
})

const keyReadyDotClass = computed(() => {
  if (resolvingKeySecret.value) return 'bg-amber-500 animate-pulse'
  if (hasKeySecret.value) return 'bg-emerald-500'
  return 'bg-red-500'
})

function isActiveWorkStatus(status?: string) {
  const s = String(status || '').toLowerCase()
  return s === 'created' || s === 'queued' || s === 'running' || s === 'pending' || s === 'processing'
}

function sortTaskWorks(list: CreazyWork[]) {
  return [...list].sort((a, b) => {
    const ar = isActiveWorkStatus(a.status) ? 0 : 1
    const br = isActiveWorkStatus(b.status) ? 0 : 1
    if (ar !== br) return ar - br
    return Number(b.id || 0) - Number(a.id || 0)
  })
}

const imageTaskWorks = computed(() => {
  return sortTaskWorks(
    works.value.filter((w) => (w.kind || '').toLowerCase() !== 'video'),
  ).slice(0, 30)
})

const videoTaskWorks = computed(() => {
  return sortTaskWorks(
    works.value.filter((w) => (w.kind || '').toLowerCase() === 'video'),
  ).slice(0, 30)
})

const imageRunningCount = computed(() => {
  return imageTaskWorks.value.filter((w) => isActiveWorkStatus(w.status)).length || activeImageJobs.value
})

const videoRunningCount = computed(() => {
  return videoTaskWorks.value.filter((w) => isActiveWorkStatus(w.status)).length || activeVideoJobs.value
})

const worksStatusSummary = computed(() => {
  const counts: Record<string, number> = {}
  for (const w of works.value) {
    const s = (w.status || 'unknown').toLowerCase()
    counts[s] = (counts[s] || 0) + 1
  }
  const order = ['succeeded', 'failed', 'running', 'queued', 'created', 'expired', 'canceled', 'cancelled']
  const seen = new Set<string>()
  const items: Array<{ key: string; label: string; count: number; chipClass: string; dotClass: string }> = []
  for (const key of order) {
    if (!counts[key]) continue
    seen.add(key)
    items.push({
      key,
      label: workStatusLabel(key),
      count: counts[key],
      chipClass: workStatusChipClass(key),
      dotClass: workStatusDotClass(key),
    })
  }
  for (const [key, count] of Object.entries(counts)) {
    if (seen.has(key)) continue
    items.push({
      key,
      label: workStatusLabel(key),
      count,
      chipClass: workStatusChipClass(key),
      dotClass: workStatusDotClass(key),
    })
  }
  return { total: works.value.length, items }
})

const worksNeedSecretBanner = computed(() => {
  if (!works.value.length) return false
  return works.value.some((w) => canPreviewWork(w) && workNeedsSecret(w) && !workCoverUrl(w))
})

const mediaCaps = computed(() => {
  const m = selectedVideoModel.value
  const modelId = String(m?.id || videoForm.model || '').toLowerCase()
  const platform = String(m?.platform || catalog.value?.platform || '').toLowerCase()
  // Prefer catalog flags; fallback for MiniMax H3 when backend has not been rebuilt yet.
  const h3Fallback =
    platform === 'minimax' ||
    modelId.includes('minimax-h3') ||
    modelId.includes('minimax_h3') ||
    modelId === 'minimax-h3' ||
    modelId.includes('h3-933')
  return {
    maxImages: Number(m?.max_image_references || 0),
    maxVideos: Number(m?.max_video_references || 0),
    maxAudios: Number(m?.max_audio_references || 0),
    maxTotal: Number(m?.max_total_media || 0),
    maxTotalImages: Number(m?.max_total_images || 0),
    allowStartFrame: Boolean(m?.allow_start_frame),
    requireStartFrame: Boolean(m?.require_start_frame),
    allowEndFrame: Boolean(m?.allow_end_frame),
    allowGeneratedAudio: Boolean(m?.allow_generated_audio),
    framesExclusiveWithRefs: Boolean(m?.frames_exclusive_with_refs) || h3Fallback,
    audioRequiresImageRefs: Boolean(m?.audio_requires_image_refs) || h3Fallback,
    forceGeneratedAudio: Boolean(m?.force_generated_audio) || h3Fallback,
    promptLimit: Number(m?.prompt_limit || 0),
  }
})

const hasStartOrEndFrame = computed(() => Boolean(startFrame.value || endFrame.value))
const hasExclusiveRefs = computed(
  () => refImages.value.length > 0 || refAudios.value.length > 0,
)
// H3-like models: frames mode and ref mode are mutually exclusive.
const frameUploadsBlocked = computed(
  () => mediaCaps.value.framesExclusiveWithRefs && hasExclusiveRefs.value,
)
const refUploadsBlocked = computed(
  () => mediaCaps.value.framesExclusiveWithRefs && hasStartOrEndFrame.value,
)


function isPlayableMediaUrl(url?: string | null): boolean {
  const u = String(url || '').trim()
  if (!u) return false
  const lower = u.toLowerCase()
  if (lower === 'b64' || lower === 'b64_json' || lower === 'null' || lower === 'undefined' || lower === '-') return false
  if (u.startsWith('blob:') || u.startsWith('data:')) {
    return u.startsWith('data:') ? u.includes(',') : true
  }
  if (u.startsWith('http://') || u.startsWith('https://') || u.startsWith('/')) return true
  return false
}

function sanitizeMediaUrl(url?: string | null): string {
  return isPlayableMediaUrl(url) ? String(url).trim() : ''
}

function syncFormModelsFromCatalog() {
  const images = catalog.value?.image_models || []
  const videos = catalog.value?.video_models || []
  if (images.length) {
    if (!images.some((m) => m.id === imageForm.model)) {
      imageForm.model = images[0].id
    }
  } else {
    imageForm.model = ''
  }
  if (videos.length) {
    if (!videos.some((m) => m.id === videoForm.model)) {
      videoForm.model = videos[0].id
    }
  } else {
    videoForm.model = ''
  }
  if (imageSizeOptions.value.length && !imageSizeOptions.value.includes(imageForm.size)) {
    imageForm.size = imageSizeOptions.value[0]
  }
  if (videoResolutionOptions.value.length && !videoResolutionOptions.value.includes(videoForm.resolution)) {
    videoForm.resolution = videoResolutionOptions.value[0]
  }
  if (videoDurationOptions.value.length && !videoDurationOptions.value.includes(videoForm.duration)) {
    videoForm.duration = Number(videoDurationOptions.value[0])
  }
  if (videoAspectOptions.value.length && !videoAspectOptions.value.includes(videoForm.aspectRatio)) {
    videoForm.aspectRatio = videoAspectOptions.value[0]
  }
}

const imageSizeOptions = computed(() => {
  const fromModel = selectedImageModel.value?.sizes
  return fromModel?.length ? fromModel : DEFAULT_IMAGE_SIZES
})

const videoResolutionOptions = computed(() => {
  const fromModel = selectedVideoModel.value?.resolutions
  return fromModel?.length ? fromModel : DEFAULT_VIDEO_RESOLUTIONS
})

const videoDurationOptions = computed(() => {
  const fromModel = selectedVideoModel.value?.durations
  return fromModel?.length ? fromModel : DEFAULT_VIDEO_DURATIONS
})

const videoAspectOptions = computed(() => {
  const fromModel = selectedVideoModel.value?.aspect_ratios
  return fromModel?.length ? fromModel : DEFAULT_ASPECT_RATIOS
})

function keyLabel(item: CreazyCanvasKey): string {
  const group = item.group_name || item.platform || ''
  return group ? `${item.name} · ${group}` : item.name
}

function resolveTabFromRoute(): TabId {
  const path = route.path
  if (path.endsWith('/video')) return 'video'
  if (path.endsWith('/works')) return 'works'
  return 'image'
}

function switchTab(tab: TabId) {
  activeTab.value = tab
  const target =
    tab === 'image' ? '/creazy-canvas/image' : tab === 'video' ? '/creazy-canvas/video' : '/creazy-canvas/works'
  if (route.path !== target) router.replace(target)
  void loadWorks({ quiet: tab !== 'works' })
}


async function hydrateKeySecretsFromPlatform() {
  resolvingKeySecret.value = true
  try {
    let page = 1
    const pageSize = 100
    const maxPages = 10
    while (page <= maxPages) {
      const res = await keysAPI.list(page, pageSize)
      const items = (res as any)?.items || (res as any)?.data || []
      for (const item of items as any[]) {
        const id = Number(item?.id || 0)
        const secret = String(item?.key || '').trim()
        if (id && secret) keySecrets[id] = secret
      }
      const total = Number((res as any)?.total || 0)
      if (!items.length || page * pageSize >= total) break
      page += 1
    }
    keySecretHydrated.value = true
  } catch (error) {
    console.warn('[creazy-canvas] hydrateKeySecretsFromPlatform failed', error)
  } finally {
    resolvingKeySecret.value = false
  }
}

async function ensureKeySecret(keyId?: number) {
  const id = Number(keyId || selectedKeyId.value || 0)
  if (!id) return ''
  if (keySecrets[id]) return keySecrets[id]
  resolvingKeySecret.value = true
  try {
    const item = await keysAPI.getById(id)
    const secret = String((item as any)?.key || '').trim()
    if (secret) {
      keySecrets[id] = secret
      return secret
    }
  } catch (error) {
    console.warn('[creazy-canvas] ensureKeySecret failed', error)
  } finally {
    resolvingKeySecret.value = false
  }
  return ''
}

function resolveApiKeySecret(keyId?: number): string {
  const id = Number(keyId || selectedKeyId.value || 0)
  const secret = (id && keySecrets[id]) || ''
  if (!secret) {
    throw new Error(t('creazyCanvas.errors.noKeySecret'))
  }
  return secret
}

function resolveWorkApiKeySecret(work: CreazyWork): string {
  const keyId = Number(work.api_key_id || selectedKeyId.value || 0)
  if (keyId && keySecrets[keyId]) return keySecrets[keyId]
  if (selectedKeyId.value && keySecrets[selectedKeyId.value]) return keySecrets[selectedKeyId.value]
  return ''
}

async function updateWorkRecord(
  workId: number,
  partial: UpdateCreazyWorkRequest,
): Promise<CreazyWork | null> {
  if (!workId) return null
  try {
    const payload: UpdateCreazyWorkRequest = {
      ...partial,
      preview_url:
        partial.preview_url !== undefined
          ? sanitizeMediaUrl(partial.preview_url) || undefined
          : undefined,
      object_url:
        partial.object_url !== undefined
          ? sanitizeMediaUrl(partial.object_url) || undefined
          : undefined,
    }
    // Keep empty string error_message when explicitly clearing.
    if (partial.error_message === '') payload.error_message = ''
    return await updateWork(workId, payload)
  } catch (error: any) {
    console.warn('[creazy-canvas] updateWorkRecord failed', error)
    return null
  }
}

async function persistWork(
  partial: Omit<CreateCreazyWorkRequest, 'api_key_id'> & { api_key_id?: number },
) {
  if (!selectedKeyId.value && !partial.api_key_id) return null
  try {
    const payload: CreateCreazyWorkRequest = {
      api_key_id: partial.api_key_id ?? selectedKeyId.value,
      kind: partial.kind,
      public_model: partial.public_model || '',
      prompt: partial.prompt || '',
      params: partial.params || {},
      gateway_type: partial.gateway_type,
      gateway_remote_id: partial.gateway_remote_id || '',
      status: partial.status || 'created',
      error_message: partial.error_message,
      preview_url: sanitizeMediaUrl(partial.preview_url) || undefined,
      object_url: sanitizeMediaUrl(partial.object_url) || undefined,
      mime_type: partial.mime_type,
      size_bytes: partial.size_bytes,
    }
    return await createWork(payload)
  } catch (error: any) {
    console.warn('[creazy-canvas] persistWork failed', error)
    return null
  }
}

function clearPoll() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

function clearWorksPoll() {
  if (worksPollTimer) {
    clearTimeout(worksPollTimer)
    worksPollTimer = null
  }
}

function scheduleWorksPoll() {
  clearWorksPoll()
  const hasActive = works.value.some((w) => isActiveWorkStatus(w.status))
  if (!hasActive && activeImageJobs.value <= 0 && activeVideoJobs.value <= 0) return
  worksPollTimer = setTimeout(() => {
    worksPollTimer = null
    if (cancelled) return
    void loadWorks({ quiet: true })
  }, 4000)
}

/** Independent sleep for concurrent background jobs (do not share pollTimer). */
function sleepMs(ms: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })
}


function formatDateTime(value?: string) {
  if (!value) return '—'
  try {
    return new Date(value).toLocaleString()
  } catch {
    return value
  }
}

function workTypeLabel(kind?: string) {
  const k = (kind || '').toLowerCase()
  return k === 'video' ? t('creazyCanvas.works.video') : t('creazyCanvas.works.image')
}

function workStatusLabel(status?: string) {
  const s = (status || '').toLowerCase()
  const key = `creazyCanvas.works.statusLabels.${s}`
  const labeled = t(key)
  return labeled === key ? status || '—' : labeled
}

/** §7.3 gateway error mapping — never surface upstream body/stack */
function mapGatewayError(error: any, fallback?: string): string {
  const status = Number(error?.status || error?.response?.status || 0)
  const raw = String(
    error?.message ||
      error?.response?.data?.error?.message ||
      error?.response?.data?.message ||
      error?.response?.data?.detail ||
      '',
  )
  const lower = raw.toLowerCase()
  const code = String(error?.code || error?.response?.data?.error?.code || error?.response?.data?.code || '').toLowerCase()
  const blob = `${lower} ${code}`

  if (
    blob.includes('moderation') ||
    blob.includes('content_policy') ||
    blob.includes('safety') ||
    blob.includes('nsfw') ||
    blob.includes('审核') ||
    blob.includes('违规')
  ) {
    return t('creazyCanvas.errors.contentModeration')
  }
  // Upstream "access forbidden" is account/channel side, not the user API key secret.
  if (
    blob.includes('upstream') &&
    (blob.includes('forbidden') || blob.includes('access denied') || status === 502 || status === 503)
  ) {
    return t('creazyCanvas.errors.serviceBusy')
  }
  if (status === 401 || status === 403 || blob.includes('invalid api key') || blob.includes('unauthorized') || (blob.includes('forbidden') && !blob.includes('upstream'))) {
    return t('creazyCanvas.errors.keyInvalid')
  }
  if (
    status === 402 ||
    blob.includes('insufficient') ||
    blob.includes('balance') ||
    blob.includes('quota') ||
    blob.includes('billing') ||
    blob.includes('余额')
  ) {
    return t('creazyCanvas.errors.insufficientBalance')
  }
  if (status === 404 || blob.includes('model not found') || blob.includes('model_not_found') || blob.includes('unknown model')) {
    return t('creazyCanvas.errors.modelUnavailable')
  }
  if (status === 429 || blob.includes('rate limit') || blob.includes('too many')) {
    return t('creazyCanvas.errors.rateLimited')
  }
  if (status >= 500 || blob.includes('timeout') || blob.includes('timed out') || blob.includes('gateway timeout')) {
    return t('creazyCanvas.errors.serviceBusy')
  }
  if (status === 400) {
    // keep short field-level hint if already localized/short; strip long raw bodies
    if (raw && raw.length <= 120 && !(raw.includes('{') || raw.includes('[')) && !blob.includes('stack') && !blob.includes('traceback')) {
      return raw
    }
    return t('creazyCanvas.errors.invalidParams')
  }

  // Known local validation messages (already i18n) pass through
  if (
    raw &&
    (raw === t('creazyCanvas.errors.noKeySecret') ||
      raw === t('creazyCanvas.errors.selectKey') ||
      raw === t('creazyCanvas.errors.selectModel') ||
      raw === t('creazyCanvas.errors.promptRequired') ||
      raw.startsWith(t('creazyCanvas.result.failed')) ||
      raw.includes(t('creazyCanvas.form.endNeedsStart')) ||
      raw.includes('不能超过') ||
      raw.includes('cannot exceed') ||
      raw.includes('reference audio') ||
      raw.includes('参考音频'))
  ) {
    return raw
  }

  if (raw && raw.length <= 120 && !(raw.includes('{') || raw.includes('[')) && !blob.includes('http') && !blob.includes('stack')) {
    return raw
  }
  return fallback || t('creazyCanvas.errors.generateFailed')
}

function workKindClass(kind?: string) {
  const k = (kind || '').toLowerCase()
  if (k === 'video') return 'badge-primary'
  if (k === 'image') return 'badge-purple'
  return 'badge-gray'
}

function workStatusClass(status?: string) {
  const s = (status || '').toLowerCase()
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) return 'badge-success'
  if (['failed', 'error'].includes(s)) return 'badge-danger'
  if (['expired'].includes(s)) return 'badge-danger'
  if (['running', 'pending', 'processing'].includes(s)) return 'badge-warning'
  if (['queued'].includes(s)) return 'badge-warning'
  if (['created'].includes(s)) return 'badge-primary'
  if (['canceled', 'cancelled'].includes(s)) return 'badge-gray'
  return 'badge-gray'
}

function workStatusDotClass(status?: string) {
  const s = (status || '').toLowerCase()
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) return 'bg-emerald-500'
  if (['failed', 'error', 'expired'].includes(s)) return 'bg-red-500'
  if (['running', 'pending', 'processing'].includes(s)) return 'bg-amber-500 animate-pulse'
  if (['queued'].includes(s)) return 'bg-amber-400'
  if (['created'].includes(s)) return 'bg-primary-500'
  if (['canceled', 'cancelled'].includes(s)) return 'bg-gray-400'
  return 'bg-gray-400'
}

function workStatusChipClass(status?: string) {
  const s = (status || '').toLowerCase()
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-950/30 dark:text-emerald-300'
  }
  if (['failed', 'error', 'expired'].includes(s)) {
    return 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300'
  }
  if (['running', 'pending', 'processing', 'queued'].includes(s)) {
    return 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300'
  }
  if (['created'].includes(s)) {
    return 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-900/50 dark:bg-primary-950/30 dark:text-primary-300'
  }
  return 'border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300'
}

function workCardClass(work: CreazyWork) {
  const s = (work.status || '').toLowerCase()
  if (isExpired(work) || s === 'expired') {
    return 'border-red-200/80 dark:border-red-900/40'
  }
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) {
    return 'border-emerald-200/90 dark:border-emerald-900/40'
  }
  if (['failed', 'error'].includes(s)) {
    return 'border-red-200/90 dark:border-red-900/40'
  }
  if (['running', 'pending', 'processing', 'queued'].includes(s)) {
    return 'border-amber-200/90 dark:border-amber-900/40'
  }
  if (['created'].includes(s)) {
    return 'border-primary-200/90 dark:border-primary-900/40'
  }
  return 'border-gray-200 dark:border-dark-700'
}

function isExpired(work: CreazyWork) {
  if ((work.status || '').toLowerCase() === 'expired') return true
  if (!work.expires_at) return false
  return new Date(work.expires_at).getTime() < Date.now()
}

function isBlobUrl(url?: string) {
  return Boolean(url && url.startsWith('blob:'))
}

function isImageWork(work: CreazyWork) {
  return (work.kind || '').toLowerCase() !== 'video'
}

function isSucceededWork(work: CreazyWork) {
  const s = (work.status || '').toLowerCase()
  return ['succeeded', 'completed', 'success', 'done'].includes(s)
}

function canPreviewWork(work: CreazyWork) {
  if (isExpired(work) || !isSucceededWork(work)) return false
  if (workStaticMediaUrl(work)) return true
  if (work.gateway_remote_id) return true
  // valid data:/http media only — ignore broken placeholders like "b64"
  return Boolean(sanitizeMediaUrl(work.preview_url) || sanitizeMediaUrl(work.object_url))
}

function workStaticMediaUrl(work: CreazyWork): string {
  const params = (work.params || {}) as Record<string, unknown>
  const resultUrls = Array.isArray(params.result_urls) ? (params.result_urls as string[]) : []
  const poster =
    typeof params.poster_url === 'string'
      ? params.poster_url
      : typeof params.start_frame_url === 'string'
        ? params.start_frame_url
        : ''
  const candidates = [poster, work.preview_url, work.object_url, ...resultUrls]
    .map((c) => sanitizeMediaUrl(typeof c === 'string' ? c : ''))
    .filter(Boolean) as string[]
  for (const c of candidates) {
    if (!needsAuthForMediaPlayback(c)) return c
  }
  // Prefer relative gateway path over invalid placeholders (never return bare "b64").
  return candidates.find((c) => c.startsWith('/') || c.startsWith('http')) || ''
}

function workPosterUrl(work: CreazyWork): string {
  const params = (work.params || {}) as Record<string, unknown>
  const poster = sanitizeMediaUrl(
    typeof params.poster_url === 'string'
      ? params.poster_url
      : typeof params.start_frame_url === 'string'
        ? params.start_frame_url
        : '',
  )
  if (poster && !needsAuthForMediaPlayback(poster)) return poster
  return ''
}

function workPreviewUrl(work: CreazyWork): string {
  return workPreviewUrls[String(work.id)] || ''
}

function workCoverVideoSrc(work: CreazyWork): string {
  const url = workCoverUrl(work)
  if (!url) return ''
  // Help browsers paint a poster frame for remote/blob videos in list cards.
  if (url.startsWith('blob:') || url.startsWith('data:')) return url
  if (url.includes('#t=')) return url
  return url + '#t=0.1'
}

function onCoverVideoLoaded(ev: Event) {
  const el = ev.target as HTMLVideoElement | null
  if (!el) return
  try {
    if (el.readyState >= 1 && el.currentTime < 0.05) {
      el.currentTime = 0.1
    }
  } catch {
    // ignore seek failures (some blobs disallow seek until more data)
  }
}

/** Cover prefers cached playable media, then public poster/static URL. */
function workCoverUrl(work: CreazyWork): string {
  return (
    workPreviewUrl(work) ||
    workPosterUrl(work) ||
    (() => {
      const staticUrl = workStaticMediaUrl(work)
      return staticUrl && !needsAuthForMediaPlayback(staticUrl) ? staticUrl : ''
    })()
  )
}

function workCoverIsImage(work: CreazyWork): boolean {
  if (isImageWork(work)) return true
  const cover = workCoverUrl(work)
  if (!cover) return false
  if (isBlobUrl(cover)) return false
  if (workPosterUrl(work) && cover === workPosterUrl(work)) return true
  return /\.(png|jpe?g|webp|gif|bmp)(\?|#|$)/i.test(cover)
}

function workNeedsSecret(work: CreazyWork): boolean {
  // V1 still requires secret for *generation*, but succeeded works are previewable
  // via JWT session content proxy — selecting the Key is enough for replay.
  void work
  return false
}

function workCoverPlaceholder(work: CreazyWork): string {
  if (isExpired(work)) return t('creazyCanvas.works.coverExpired')
  const s = (work.status || '').toLowerCase()
  if (['failed', 'canceled', 'cancelled'].includes(s)) return t('creazyCanvas.works.coverUnavailable')
  if (['created', 'queued', 'running'].includes(s)) return t('creazyCanvas.works.coverPending')
  return t('creazyCanvas.works.coverUnavailable')
}

function workCoverUnavailableReason(work: CreazyWork): string {
  return workCoverPlaceholder(work)
}


function needsAuthForMediaPlayback(url?: string): boolean {
  if (!url) return true
  if (!isPlayableMediaUrl(url)) return true
  if (isBlobUrl(url) || url.startsWith('data:')) return false
  if (url.startsWith('/')) return true
  try {
    const u = new URL(url, typeof window !== 'undefined' ? window.location.origin : 'http://localhost')
    const path = u.pathname || ''
    if (path.includes('/v1/videos/jobs/') && path.includes('/content')) return true
    if (path.startsWith('/v1/') || path.startsWith('/api/')) return true
    if (typeof window !== 'undefined' && u.origin === window.location.origin && path.startsWith('/v1/')) return true
  } catch {
    return true
  }
  return false
}

function revokeBlobUrl(url?: string) {
  if (!url || !isBlobUrl(url)) return
  try {
    URL.revokeObjectURL(url)
  } catch {
    // ignore
  }
}

function clearVideoResultPlayback() {
  revokeBlobUrl(videoResultUrl.value)
  videoResultUrl.value = ''
  videoPersistUrl.value = ''
}

function setVideoResultPlayback(playable: string, persist?: string) {
  if (videoResultUrl.value && videoResultUrl.value !== playable) {
    revokeBlobUrl(videoResultUrl.value)
  }
  videoResultUrl.value = playable
  if (persist && !isBlobUrl(persist)) {
    videoPersistUrl.value = persist
  } else if (!isBlobUrl(playable)) {
    videoPersistUrl.value = playable
  } else if (videoJobId.value) {
    videoPersistUrl.value = '/v1/videos/jobs/' + videoJobId.value + '/content'
  }
}

function openMediaPreview(item: { type: 'image' | 'video'; url: string }) {
  if (!item?.url) return
  mediaPreview.value = { type: item.type, url: item.url }
}

function closeMediaPreview() {
  mediaPreview.value = null
}

async function resolvePlayableVideoUrl(
  apiKey: string,
  jobId: string,
  extracted: string,
): Promise<{ playable: string; persist: string }> {
  const fallbackPersist = extracted || (jobId ? '/v1/videos/jobs/' + jobId + '/content' : '')
  if (jobId && (!extracted || needsAuthForMediaPlayback(extracted))) {
    try {
      const resolved = await getVideoContentURL(apiKey, jobId)
      if (resolved) {
        return {
          playable: resolved,
          persist:
            extracted && !isBlobUrl(extracted) && !needsAuthForMediaPlayback(extracted)
              ? extracted
              : fallbackPersist,
        }
      }
    } catch {
      // fall through
    }
  }
  if (extracted) {
    return { playable: extracted, persist: isBlobUrl(extracted) ? fallbackPersist : extracted }
  }
  return { playable: '', persist: fallbackPersist }
}

async function loadWorkPreview(work: CreazyWork): Promise<boolean> {
  const id = String(work.id)
  if (workPreviewUrls[id]) return true
  if (workPreviewLoading[id]) return false
  workPreviewLoading[id] = true
  try {
    let url = workStaticMediaUrl(work)
    if (url && !needsAuthForMediaPlayback(url)) {
      workPreviewUrls[id] = url
      return true
    }
    try {
      const res = await getWorkDownloadURL(work.id)
      if (res.source === 'object' && res.url && !needsAuthForMediaPlayback(res.url)) {
        workPreviewUrls[id] = res.url
        return true
      }
      if (
        res.url &&
        res.url.startsWith('http') &&
        !res.url.includes('/v1/videos/jobs/') &&
        !res.url.includes('/api/') &&
        !needsAuthForMediaPlayback(res.url)
      ) {
        workPreviewUrls[id] = res.url
        return true
      }
      // Session proxy path from download-url metadata (JWT content stream).
      if (res.source === 'session' || (res.url && res.url.includes('/creazy-canvas/works/') && res.url.includes('/content'))) {
        try {
          const blobUrl = await getWorkContentBlob(work.id)
          if (blobUrl) {
            if (isBlobUrl(blobUrl)) workPreviewBlobUrls.add(blobUrl)
            workPreviewUrls[id] = blobUrl
            return true
          }
        } catch (error) {
          console.warn('[creazy-canvas] session content preview failed', error)
        }
      }
    } catch {
      // ignore download-url fallback
    }

    // Direct JWT content stream (preferred for succeeded gateway works).
    if (canPreviewWork(work) && (work.gateway_remote_id || needsAuthForMediaPlayback(url))) {
      try {
        const blobUrl = await getWorkContentBlob(work.id)
        if (blobUrl) {
          if (isBlobUrl(blobUrl)) workPreviewBlobUrls.add(blobUrl)
          workPreviewUrls[id] = blobUrl
          return true
        }
      } catch (error) {
        console.warn('[creazy-canvas] session content preview failed', error)
      }
    }

    // Legacy fallback: client-held API key secret (still useful if session proxy unavailable).
    if ((work.kind || '').toLowerCase() === 'video' && work.gateway_remote_id) {
      const apiKey = resolveWorkApiKeySecret(work)
      if (apiKey) {
        try {
          const resolved = await getVideoContentURL(apiKey, work.gateway_remote_id)
          if (resolved) {
            if (isBlobUrl(resolved)) workPreviewBlobUrls.add(resolved)
            workPreviewUrls[id] = resolved
            return true
          }
        } catch (error) {
          console.warn('[creazy-canvas] video preview content failed', error)
        }
      }
    }
    if (url && isImageWork(work) && !needsAuthForMediaPlayback(url)) {
      workPreviewUrls[id] = url
      return true
    }
    return Boolean(workPreviewUrls[id])
  } finally {
    workPreviewLoading[id] = false
  }
}

async function openWorkPreview(work: CreazyWork) {
  // Align selected key with work for generation reuse / optional secret fallback.
  if (work.api_key_id && selectedKeyId.value !== work.api_key_id) {
    selectedKeyId.value = work.api_key_id
    await loadCatalog()
  }
  const ok = await loadWorkPreview(work)
  const url = workPreviewUrl(work) || workCoverUrl(work)
  if (!url) {
    appStore.showError(t('creazyCanvas.works.previewFailedGeneric'))
    return
  }
  openMediaPreview({ type: isImageWork(work) ? 'image' : 'video', url })
  void ok
}

async function hydrateWorkPreviews(list: CreazyWork[]) {
  const previewable = list.filter((w) => canPreviewWork(w))
  // Prefer lightweight covers: images / public posters first, then a few video blobs.
  const light = previewable.filter((w) => {
    if (isImageWork(w)) return true
    if (workPosterUrl(w)) return true
    const staticUrl = workStaticMediaUrl(w)
    return Boolean(staticUrl && !needsAuthForMediaPlayback(staticUrl))
  }).slice(0, 16)
  const heavyVideo = previewable
    .filter((w) => !isImageWork(w) && !light.includes(w))
    .slice(0, 4)
  const targets = [...light, ...heavyVideo]
  // Sequential-ish batches to reduce memory spikes on large MP4s.
  const batchSize = 4
  for (let i = 0; i < targets.length; i += batchSize) {
    if (cancelled) break
    await Promise.all(targets.slice(i, i + batchSize).map((w) => loadWorkPreview(w)))
  }
}

function revokePreviewUrl(item?: MediaItem | null) {
  if (item?.preview_url && item.preview_url.startsWith('blob:')) {
    try {
      URL.revokeObjectURL(item.preview_url)
    } catch {
      // ignore
    }
  }
}

function clearRefImages() {
  for (const item of refImages.value) revokePreviewUrl(item)
  refImages.value = []
}

function removeRefImage(idx: number) {
  const [item] = refImages.value.splice(idx, 1)
  revokePreviewUrl(item)
}

function clearRefVideos() {
  refVideos.value = []
}

function clearRefAudios() {
  refAudios.value = []
}

function resetVideoMedia() {
  startFrame.value = null
  endFrame.value = null
  clearRefImages()
  clearRefVideos()
  clearRefAudios()
  uploadingMedia.value = ''
  uploadProgressLabel.value = ''
  uploadProgress.value = { done: 0, total: 0 }
}

function clearStartFrame() {
  startFrame.value = null
  startFrameUrlInput.value = ''
  endFrame.value = null
}

function applyStartFrameUrl() {
  const url = startFrameUrlInput.value.trim()
  if (!url) return
  try {
    assertMediaRoom(startFrame.value ? 0 : 1)
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
    return
  }
  videoError.value = ''
  let name = 'start-frame'
  try {
    const u = new URL(url)
    const base = u.pathname.split('/').filter(Boolean).pop() || name
    name = decodeURIComponent(base).slice(0, 120) || name
  } catch {
    // keep default name for non-absolute URLs
  }
  if (mediaCaps.value.framesExclusiveWithRefs && hasExclusiveRefs.value) {
    videoError.value = t('creazyCanvas.form.framesExclusiveWithRefs')
    return
  }
  startFrame.value = { name, media_url: url }
  if (mediaCaps.value.framesExclusiveWithRefs) {
    refImages.value = []
    refAudios.value = []
  }
}

function mediaCountTotal(extra = 0) {
  const frames = (startFrame.value ? 1 : 0) + (endFrame.value ? 1 : 0)
  return frames + refImages.value.length + refVideos.value.length + refAudios.value.length + extra
}

function assertMediaRoom(extra = 0) {
  const maxTotal = mediaCaps.value.maxTotal
  if (maxTotal > 0 && mediaCountTotal(extra) > maxTotal) {
    throw new Error(t('creazyCanvas.form.totalExceeded', { max: maxTotal }))
  }
}

function imageCountTotal(extra = 0) {
  return (startFrame.value ? 1 : 0) + (endFrame.value ? 1 : 0) + refImages.value.length + extra
}

function assertImageRoom(extra = 0) {
  const max = mediaCaps.value.maxTotalImages
  if (max > 0 && imageCountTotal(extra) > max) {
    throw new Error(t('creazyCanvas.form.totalImagesExceeded', { max }))
  }
}

function assertFrameModeAllowed() {
  if (mediaCaps.value.framesExclusiveWithRefs && hasExclusiveRefs.value) {
    throw new Error(t('creazyCanvas.form.framesExclusiveWithRefs'))
  }
}

function assertRefModeAllowed() {
  if (mediaCaps.value.framesExclusiveWithRefs && hasStartOrEndFrame.value) {
    throw new Error(t('creazyCanvas.form.refsExclusiveWithFrames'))
  }
}

async function readMediaDuration(file: File): Promise<number> {
  const url = URL.createObjectURL(file)
  try {
    const isAudio = file.type.startsWith('audio/')
    const el = document.createElement(isAudio ? 'audio' : 'video')
    el.preload = 'metadata'
    el.src = url
    await new Promise<void>((resolve, reject) => {
      el.onloadedmetadata = () => resolve()
      el.onerror = () => reject(new Error('metadata'))
    })
    const duration = Number(el.duration)
    if (!Number.isFinite(duration) || duration <= 0) return 1
    return Math.round(duration * 100) / 100
  } catch {
    return 1
  } finally {
    URL.revokeObjectURL(url)
  }
}

async function uploadPickedFile(file: File, kind: 'image' | 'video' | 'audio') {
  const apiKey = resolveApiKeySecret()
  const uploaded = await uploadVideoAsset(apiKey, file, file.name, kind)
  const mediaUrl = String(uploaded.media_url || uploaded.url || '')
  if (!mediaUrl) throw new Error(t('creazyCanvas.errors.generateFailed'))
  return mediaUrl
}

async function onPickStartFrame(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  uploadingMedia.value = 'start'
  videoError.value = ''
  try {
    assertFrameModeAllowed()
    assertMediaRoom(startFrame.value ? 0 : 1)
    assertImageRoom(startFrame.value ? 0 : 1)
    const media_url = await uploadPickedFile(file, 'image')
    startFrame.value = { name: file.name, media_url }
    if (mediaCaps.value.framesExclusiveWithRefs) {
      refImages.value = []
      refAudios.value = []
    }
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
  } finally {
    uploadingMedia.value = ''
  }
}

async function onPickEndFrame(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!startFrame.value) {
    videoError.value = t('creazyCanvas.form.endNeedsStart')
    return
  }
  uploadingMedia.value = 'end'
  videoError.value = ''
  try {
    assertFrameModeAllowed()
    assertMediaRoom(endFrame.value ? 0 : 1)
    assertImageRoom(endFrame.value ? 0 : 1)
    const media_url = await uploadPickedFile(file, 'image')
    endFrame.value = { name: file.name, media_url }
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
  } finally {
    uploadingMedia.value = ''
  }
}

async function onPickRefImage(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  const room = Math.max(0, mediaCaps.value.maxImages - refImages.value.length)
  if (room <= 0) {
    videoError.value = t('creazyCanvas.form.imagesExceeded', { max: mediaCaps.value.maxImages })
    return
  }
  const batch = files.slice(0, room)
  uploadingMedia.value = 'ref-image'
  uploadProgress.value = { done: 0, total: batch.length }
  uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
    done: 0,
    total: batch.length,
  })
  videoError.value = ''
  try {
    assertRefModeAllowed()
    for (let i = 0; i < batch.length; i++) {
      const file = batch[i]
      assertMediaRoom(1)
      assertImageRoom(1)
      const media_url = await uploadPickedFile(file, 'image')
      const preview_url = URL.createObjectURL(file)
      refImages.value.push({ name: file.name, media_url, preview_url })
      if (mediaCaps.value.framesExclusiveWithRefs) {
        startFrame.value = null
        endFrame.value = null
      }
      uploadProgress.value = { done: i + 1, total: batch.length }
      uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
        done: i + 1,
        total: batch.length,
      })
    }
    if (files.length > room) {
      videoError.value = t('creazyCanvas.form.imagesExceeded', { max: mediaCaps.value.maxImages })
    }
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
  } finally {
    uploadingMedia.value = ''
    uploadProgressLabel.value = ''
    uploadProgress.value = { done: 0, total: 0 }
  }
}

async function onPickRefVideo(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  const room = Math.max(0, mediaCaps.value.maxVideos - refVideos.value.length)
  if (room <= 0) {
    videoError.value = t('creazyCanvas.form.videosExceeded', { max: mediaCaps.value.maxVideos })
    return
  }
  const batch = files.slice(0, room)
  uploadingMedia.value = 'ref-video'
  uploadProgress.value = { done: 0, total: batch.length }
  uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
    done: 0,
    total: batch.length,
  })
  videoError.value = ''
  try {
    for (let i = 0; i < batch.length; i++) {
      const file = batch[i]
      assertMediaRoom(1)
      const [media_url, duration_seconds] = await Promise.all([
        uploadPickedFile(file, 'video'),
        readMediaDuration(file),
      ])
      refVideos.value.push({ name: file.name, media_url, duration_seconds })
      uploadProgress.value = { done: i + 1, total: batch.length }
      uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
        done: i + 1,
        total: batch.length,
      })
    }
    if (files.length > room) {
      videoError.value = t('creazyCanvas.form.videosExceeded', { max: mediaCaps.value.maxVideos })
    }
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
  } finally {
    uploadingMedia.value = ''
    uploadProgressLabel.value = ''
    uploadProgress.value = { done: 0, total: 0 }
  }
}

async function onPickRefAudio(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  const room = Math.max(0, mediaCaps.value.maxAudios - refAudios.value.length)
  if (room <= 0) {
    videoError.value = t('creazyCanvas.form.audiosExceeded', { max: mediaCaps.value.maxAudios })
    return
  }
  const batch = files.slice(0, room)
  uploadingMedia.value = 'ref-audio'
  uploadProgress.value = { done: 0, total: batch.length }
  uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
    done: 0,
    total: batch.length,
  })
  videoError.value = ''
  try {
    assertRefModeAllowed()
    for (let i = 0; i < batch.length; i++) {
      const file = batch[i]
      assertMediaRoom(1)
      const [media_url, duration_seconds] = await Promise.all([
        uploadPickedFile(file, 'audio'),
        readMediaDuration(file),
      ])
      refAudios.value.push({ name: file.name, media_url, duration_seconds })
      // Gateway requires audio=true whenever audio references are present.
      if (mediaCaps.value.allowGeneratedAudio || mediaCaps.value.forceGeneratedAudio) {
        videoForm.generateAudio = true
      }
      uploadProgress.value = { done: i + 1, total: batch.length }
      uploadProgressLabel.value = t('creazyCanvas.form.uploadingProgress', {
        done: i + 1,
        total: batch.length,
      })
    }
    if (files.length > room) {
      videoError.value = t('creazyCanvas.form.audiosExceeded', { max: mediaCaps.value.maxAudios })
    }
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
  } finally {
    uploadingMedia.value = ''
    uploadProgressLabel.value = ''
    uploadProgress.value = { done: 0, total: 0 }
  }
}

function validateVideoMediaBeforeSubmit() {
  const prompt = String(videoForm.prompt || '').trim()
  if (mediaCaps.value.promptLimit > 0 && [...prompt].length > mediaCaps.value.promptLimit) {
    throw new Error(t('creazyCanvas.form.promptTooLong', { max: mediaCaps.value.promptLimit }))
  }
  if (mediaCaps.value.requireStartFrame && !startFrame.value) {
    throw new Error(t('creazyCanvas.form.startFrameRequiredError'))
  }
  // Contract/gateway: end frame depends on start frame (pair not required).
  if (endFrame.value && !startFrame.value) {
    throw new Error(t('creazyCanvas.form.endNeedsStart'))
  }
  if (mediaCaps.value.framesExclusiveWithRefs && hasStartOrEndFrame.value && hasExclusiveRefs.value) {
    throw new Error(t('creazyCanvas.form.framesExclusiveWithRefs'))
  }
  // Always enforce caps (including 0 = unsupported type).
  if (refImages.value.length > mediaCaps.value.maxImages) {
    throw new Error(t('creazyCanvas.form.imagesExceeded', { max: mediaCaps.value.maxImages }))
  }
  if (refVideos.value.length > mediaCaps.value.maxVideos) {
    throw new Error(t('creazyCanvas.form.videosExceeded', { max: mediaCaps.value.maxVideos }))
  }
  if (refAudios.value.length > mediaCaps.value.maxAudios) {
    throw new Error(t('creazyCanvas.form.audiosExceeded', { max: mediaCaps.value.maxAudios }))
  }
  // Catalog-driven: some models (MiniMax H3) require images with audio refs.
  if (
    mediaCaps.value.audioRequiresImageRefs &&
    refAudios.value.length > 0 &&
    refImages.value.length === 0
  ) {
    throw new Error(t('creazyCanvas.form.audioNeedsCompanion'))
  }
  // Gateway: audio=true is required when audio references are provided.
  if (
    refAudios.value.length > 0 &&
    !(mediaCaps.value.forceGeneratedAudio || videoForm.generateAudio)
  ) {
    throw new Error(t('creazyCanvas.form.audioRequiresNativeAudio'))
  }
  assertMediaRoom(0)
  assertImageRoom(0)
}

function buildVideoGenerationPayload() {
  const payload: Record<string, unknown> = {
    model: videoForm.model,
    prompt: videoForm.prompt.trim(),
    resolution: videoForm.resolution,
    duration: videoForm.duration,
    aspect_ratio: videoForm.aspectRatio,
  }
  // Public Seedance API uses `audio` (not generate_audio); DisallowUnknownFields rejects unknown fields.
  // H3-like models force native audio regardless of checkbox.
  if (
    mediaCaps.value.forceGeneratedAudio ||
    refAudios.value.length > 0 ||
    (mediaCaps.value.allowGeneratedAudio && videoForm.generateAudio)
  ) {
    payload.audio = true
  }
  // start/end frames are independent uploads
  if (startFrame.value) {
    payload.start_frame_url = startFrame.value.media_url
  }
  if (endFrame.value) {
    payload.end_frame_url = endFrame.value.media_url
  }
  // optional reference media
  const guidances: Record<string, unknown> = {}
  if (refImages.value.length) {
    guidances.image_reference = refImages.value.map((item, order) => ({
      image: { url: item.media_url, type: 'UPLOADED' },
      strength: 'MID',
      order,
    }))
  }
  if (refVideos.value.length) {
    guidances.video_reference_base = refVideos.value.map((item) => ({
      video: {
        url: item.media_url,
        type: 'UPLOADED',
        duration_seconds: item.duration_seconds || 1,
      },
    }))
  }
  if (refAudios.value.length) {
    guidances.audio_reference = refAudios.value.map((item) => ({
      audio: {
        url: item.media_url,
        type: 'UPLOADED',
        duration_seconds: item.duration_seconds || 1,
      },
    }))
    payload.audio = true
  }
  if (Object.keys(guidances).length) payload.guidances = guidances
  return payload
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    keys.value = await listKeys()
    await hydrateKeySecretsFromPlatform()
    if (!selectedKeyId.value && keys.value.length > 0) {
      selectedKeyId.value = keys.value[0].id
    }
    if (selectedKeyId.value) {
      await ensureKeySecret(selectedKeyId.value)
      await loadCatalog()
      await loadWorks({ quiet: true })
    }
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || error?.message || t('creazyCanvas.key.loadFailed'))
  } finally {
    loadingKeys.value = false
  }
}

async function loadCatalog() {
  if (!selectedKeyId.value) {
    catalog.value = null
    return
  }
  loadingCatalog.value = true
  try {
    catalog.value = await getCatalog(selectedKeyId.value)
    syncFormModelsFromCatalog()
  } catch (error: any) {
    catalog.value = {
      api_key_id: selectedKeyId.value,
      image_models: [],
      video_models: [],
    }
    appStore.showError(error?.response?.data?.detail || error?.message || t('creazyCanvas.catalog.loadFailed'))
  } finally {
    loadingCatalog.value = false
  }
}

async function onKeyChange() {
  clearWorksPoll()
  imageResultUrls.value = []
  clearVideoResultPlayback()
  videoStatus.value = ''
  videoJobId.value = ''
  imageError.value = ''
  videoError.value = ''
  imageSaveMessage.value = ''
  videoSaveMessage.value = ''
  lastImageTaskId.value = ''
  lastImageGatewayType.value = ''
  resetVideoMedia()
  works.value = []
  if (selectedKeyId.value) {
    await ensureKeySecret(selectedKeyId.value)
  }
  await loadCatalog()
  await loadWorks({ quiet: true })
}

function extractImageUrls(payload: any): string[] {
  const urls: string[] = []
  const data = payload?.data
  if (Array.isArray(data)) {
    for (const item of data) {
      if (item?.url) urls.push(item.url)
      else if (item?.b64_json) urls.push(`data:image/png;base64,${item.b64_json}`)
    }
  }
  if (payload?.url) urls.push(payload.url)
  if (payload?.result_url) urls.push(payload.result_url)
  if (Array.isArray(payload?.result_urls)) urls.push(...payload.result_urls.filter(Boolean))
  return [...new Set(urls.filter(Boolean))]
}

function isTerminalImageStatus(status?: string) {
  const s = (status || '').toLowerCase()
  return ['succeeded', 'completed', 'success', 'failed', 'error', 'cancelled'].includes(s)
}

function isTerminalVideoStatus(status?: string) {
  const s = (status || '').toLowerCase()
  return ['succeeded', 'completed', 'success', 'failed', 'error', 'cancelled', 'canceled'].includes(s)
}

async function generateImage() {
  imageError.value = ''
  imageSaveMessage.value = ''

  if (!selectedKeyId.value) {
    imageError.value = t('creazyCanvas.errors.selectKey')
    return
  }
  if (!imageForm.prompt.trim()) {
    imageError.value = t('creazyCanvas.errors.promptRequired')
    return
  }
  if (!imageForm.model || !selectedImageModel.value) {
    imageError.value = t('creazyCanvas.errors.selectModel')
    return
  }

  submittingImage.value = true
  const snapshot = {
    model: imageForm.model,
    prompt: imageForm.prompt.trim(),
    size: imageForm.size,
    preferAsync: Boolean(selectedImageModel.value?.async),
    keyId: selectedKeyId.value,
  }
  let gatewayAttempted = false
  let runningWorkId: number | null = null
  try {
    await ensureKeySecret(snapshot.keyId)
    const apiKey = resolveApiKeySecret(snapshot.keyId)
    gatewayAttempted = true

    const running = await persistWork({
      kind: 'image',
      api_key_id: snapshot.keyId,
      status: 'running',
      public_model: snapshot.model,
      prompt: snapshot.prompt,
      params: { size: snapshot.size, n: 1 },
      gateway_type: snapshot.preferAsync ? 'image_task' : 'image_sync',
    })
    if (running?.id) runningWorkId = running.id
    void loadWorks({ quiet: true })

    submittingImage.value = false
    imageSaveMessage.value = t('creazyCanvas.tasks.submitted')
    activeImageJobs.value += 1
    generatingImage.value = activeImageJobs.value > 0

    void runImageLifecycle({
      apiKey,
      snapshot,
      runningWorkId,
    })
  } catch (error: any) {
    const msg = mapGatewayError(error)
    imageError.value = msg
    if (gatewayAttempted && snapshot.keyId && snapshot.model && snapshot.prompt) {
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, { status: 'failed', error_message: msg })
      } else {
        await persistWork({
          kind: 'image',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: { size: snapshot.size, n: 1 },
          error_message: msg,
        })
      }
      void loadWorks({ quiet: true })
    }
    submittingImage.value = false
  }
}

async function runImageLifecycle(opts: {
  apiKey: string
  snapshot: { model: string; prompt: string; size: string; preferAsync: boolean; keyId: number }
  runningWorkId: number | null
}) {
  const { apiKey, snapshot } = opts
  let runningWorkId = opts.runningWorkId
  let failedPersisted = false
  let lastGatewayType: 'image_task' | 'image_sync' | '' = ''
  let lastTaskId = ''
  try {
    const imagePayload = {
      model: snapshot.model,
      prompt: snapshot.prompt,
      size: snapshot.size,
      n: 1,
    }
    let usedAsync = snapshot.preferAsync
    let response: any
    try {
      response = await gatewayGenerateImage(apiKey, imagePayload, { async: snapshot.preferAsync })
    } catch (asyncErr: any) {
      const status = Number(asyncErr?.status || 0)
      const msg = String(asyncErr?.message || asyncErr?.code || '')
      const asyncUnavailable =
        snapshot.preferAsync &&
        (status === 404 ||
          /async.*not enabled|not_found_error|model not found|unknown model/i.test(msg))
      if (!asyncUnavailable) throw asyncErr
      usedAsync = false
      response = await gatewayGenerateImage(apiKey, imagePayload, { async: false })
    }

    let urls = extractImageUrls(response)
    const taskId = response.task_id || response.id
    if (usedAsync || taskId) {
      lastGatewayType = 'image_task'
      lastTaskId = taskId ? String(taskId) : ''
    } else {
      lastGatewayType = 'image_sync'
    }
    if (runningWorkId && lastTaskId) {
      await updateWorkRecord(runningWorkId, {
        status: 'running',
        gateway_type: lastGatewayType || undefined,
        gateway_remote_id: lastTaskId,
      })
    }

    if (!urls.length && taskId) {
      lastGatewayType = 'image_task'
      lastTaskId = String(taskId)
      for (let i = 0; i < 90 && !cancelled; i++) {
        await sleepMs(2000)
        response = await getImageTask(apiKey, String(taskId))
        urls = extractImageUrls(response)
        if (urls.length || isTerminalImageStatus(response.status)) break
        if (runningWorkId && i % 5 === 4) {
          await updateWorkRecord(runningWorkId, {
            status: 'running',
            gateway_type: 'image_task',
            gateway_remote_id: lastTaskId,
          })
        }
        if (i % 3 === 2) void loadWorks({ quiet: true })
      }
    }

    if (!urls.length) {
      const msg = mapGatewayError(
        { message: String((response as any)?.error?.message || (response as any)?.error || response?.status || ''), status: 0 },
        response?.status
          ? `${t('creazyCanvas.result.failed')}: ${response.status}`
          : t('creazyCanvas.errors.generateFailed'),
      )
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, {
          status: 'failed',
          gateway_type: lastGatewayType || (taskId ? 'image_task' : 'image_sync'),
          gateway_remote_id: lastTaskId,
          error_message: msg,
        })
      } else {
        await persistWork({
          kind: 'image',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: { size: snapshot.size, n: 1 },
          gateway_type: lastGatewayType || (taskId ? 'image_task' : 'image_sync'),
          gateway_remote_id: lastTaskId,
          error_message: msg,
        })
      }
      failedPersisted = true
      throw new Error(msg)
    }

    const cleanUrls = urls.map((u) => sanitizeMediaUrl(u)).filter(Boolean)
    if (!cleanUrls.length) {
      throw new Error(t('creazyCanvas.errors.generateFailed'))
    }

    if (selectedKeyId.value === snapshot.keyId) {
      imageResultUrls.value = cleanUrls
      lastImageTaskId.value = lastTaskId
      lastImageGatewayType.value = lastGatewayType
      imageSaveMessage.value = t('creazyCanvas.result.autoSaved')
    }

    const successPayload = {
      status: 'succeeded' as const,
      public_model: snapshot.model,
      prompt: snapshot.prompt,
      params: { size: snapshot.size, n: 1, result_urls: cleanUrls },
      gateway_type: lastGatewayType || (taskId ? 'image_task' : 'image_sync'),
      gateway_remote_id: lastTaskId,
      preview_url: cleanUrls[0],
      object_url: cleanUrls[0],
      mime_type: 'image/png',
      error_message: '',
    }
    if (runningWorkId) {
      const updated = await updateWorkRecord(runningWorkId, successPayload)
      if (!updated) {
        await persistWork({ kind: 'image', api_key_id: snapshot.keyId, ...successPayload })
      }
    } else {
      await persistWork({ kind: 'image', api_key_id: snapshot.keyId, ...successPayload })
    }
    void loadWorks({ quiet: true })
  } catch (error: any) {
    const msg = mapGatewayError(error)
    if (selectedKeyId.value === snapshot.keyId) {
      imageError.value = msg
    }
    if (!failedPersisted && snapshot.model && snapshot.prompt) {
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, {
          status: 'failed',
          gateway_type: lastGatewayType || undefined,
          gateway_remote_id: lastTaskId || undefined,
          error_message: msg,
        })
      } else {
        await persistWork({
          kind: 'image',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: { size: snapshot.size, n: 1 },
          gateway_type: lastGatewayType || undefined,
          gateway_remote_id: lastTaskId || undefined,
          error_message: msg,
        })
      }
      void loadWorks({ quiet: true })
    }
  } finally {
    activeImageJobs.value = Math.max(0, activeImageJobs.value - 1)
    generatingImage.value = activeImageJobs.value > 0
  }
}

function extractVideoUrl(job: any): string {
  return (
    job?.video_url ||
    job?.content_url ||
    job?.url ||
    job?.result?.url ||
    job?.result?.content_url ||
    job?.result?.data?.[0]?.url ||
    job?.result?.data?.[0]?.mp4_url ||
    job?.result?.data?.[0]?.local_url ||
    ''
  )
}

async function generateVideo() {
  videoError.value = ''
  videoSaveMessage.value = ''

  if (!selectedKeyId.value) {
    videoError.value = t('creazyCanvas.errors.selectKey')
    return
  }
  if (!videoForm.model || !selectedVideoModel.value) {
    videoError.value = t('creazyCanvas.errors.selectModel')
    return
  }
  if (!videoForm.prompt.trim()) {
    videoError.value = t('creazyCanvas.errors.promptRequired')
    return
  }

  try {
    validateVideoMediaBeforeSubmit()
  } catch (error: any) {
    videoError.value = mapGatewayError(error)
    return
  }

  submittingVideo.value = true
  const forceAudio = mediaCaps.value.forceGeneratedAudio || refAudios.value.length > 0 || videoForm.generateAudio
  const snapshot = {
    model: videoForm.model,
    prompt: videoForm.prompt.trim(),
    resolution: videoForm.resolution,
    duration: videoForm.duration,
    aspectRatio: videoForm.aspectRatio,
    generateAudio: forceAudio,
    payload: buildVideoGenerationPayload() as any,
    startFrameUrl:
      startFrame.value?.media_url && !isBlobUrl(startFrame.value.media_url)
        ? startFrame.value.media_url
        : '',
    keyId: selectedKeyId.value,
  }
  let gatewayAttempted = false
  let runningWorkId: number | null = null
  try {
    await ensureKeySecret(snapshot.keyId)
    const apiKey = resolveApiKeySecret(snapshot.keyId)
    gatewayAttempted = true

    const job = await gatewayGenerateVideo(apiKey, snapshot.payload)
    const jobId = String(job.id || job.job_id || '')
    if (selectedKeyId.value === snapshot.keyId) {
      videoJobId.value = jobId
      videoStatus.value = job.status || 'submitted'
    }

    const baseParams = {
      resolution: snapshot.resolution,
      duration: snapshot.duration,
      aspect_ratio: snapshot.aspectRatio,
      generate_audio: snapshot.generateAudio || undefined,
    }

    if (jobId) {
      const running = await persistWork({
        kind: 'video',
        api_key_id: snapshot.keyId,
        status: 'running',
        public_model: snapshot.model,
        prompt: snapshot.prompt,
        params: baseParams,
        gateway_type: 'video_job',
        gateway_remote_id: jobId,
      })
      if (running?.id) {
        runningWorkId = running.id
        if (selectedKeyId.value === snapshot.keyId) {
          activeVideoWorkId.value = running.id
        }
      }
    } else {
      const running = await persistWork({
        kind: 'video',
        api_key_id: snapshot.keyId,
        status: 'running',
        public_model: snapshot.model,
        prompt: snapshot.prompt,
        params: baseParams,
        gateway_type: 'video_job',
      })
      if (running?.id) runningWorkId = running.id
    }

    void loadWorks({ quiet: true })
    submittingVideo.value = false
    videoSaveMessage.value = t('creazyCanvas.tasks.submitted')
    activeVideoJobs.value += 1
    generatingVideo.value = activeVideoJobs.value > 0

    void runVideoLifecycle({
      apiKey,
      snapshot,
      runningWorkId,
      initialJob: job,
      jobId,
    })
  } catch (error: any) {
    const msg = mapGatewayError(error)
    videoError.value = msg
    if (gatewayAttempted && snapshot.keyId && snapshot.model && snapshot.prompt) {
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, {
          status: 'failed',
          gateway_type: 'video_job',
          error_message: msg,
        })
      } else {
        await persistWork({
          kind: 'video',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: {
            resolution: snapshot.resolution,
            duration: snapshot.duration,
            aspect_ratio: snapshot.aspectRatio,
          },
          gateway_type: 'video_job',
          error_message: msg,
        })
      }
      void loadWorks({ quiet: true })
    }
    submittingVideo.value = false
  }
}

async function runVideoLifecycle(opts: {
  apiKey: string
  snapshot: {
    model: string
    prompt: string
    resolution: string
    duration: number
    aspectRatio: string
    generateAudio: boolean
    payload: any
    startFrameUrl: string
    keyId: number
  }
  runningWorkId: number | null
  initialJob: any
  jobId: string
}) {
  const { apiKey, snapshot, jobId } = opts
  let runningWorkId = opts.runningWorkId
  let job = opts.initialJob
  let failedPersisted = false
  const baseParams = {
    resolution: snapshot.resolution,
    duration: snapshot.duration,
    aspect_ratio: snapshot.aspectRatio,
    generate_audio: snapshot.generateAudio || undefined,
  }
  try {
    if (jobId) {
      for (let i = 0; i < 150 && !cancelled; i++) {
        if (isTerminalVideoStatus(job.status)) break
        await sleepMs(3000)
        job = await getVideoJob(apiKey, jobId)
        if (selectedKeyId.value === snapshot.keyId) {
          videoStatus.value = job.status || videoStatus.value
        }
        const st = String(job.status || '').toLowerCase()
        if (runningWorkId && (st === 'queued' || st === 'running' || st === 'created' || st === 'processing')) {
          if (i % 5 === 4) {
            await updateWorkRecord(runningWorkId, {
              status: st === 'queued' || st === 'created' ? st : 'running',
              gateway_remote_id: jobId,
            })
          }
        }
        if (i % 2 === 1) void loadWorks({ quiet: true })
      }
    }

    const extracted = extractVideoUrl(job)
    const failedStatus = ['failed', 'error', 'cancelled', 'canceled'].includes((job.status || '').toLowerCase())
    const timedOut = Boolean(jobId && !isTerminalVideoStatus(job.status) && !failedStatus)
    let playable = ''
    let persist = extracted
    if (jobId && isTerminalVideoStatus(job.status) && !failedStatus) {
      const resolved = await resolvePlayableVideoUrl(apiKey, jobId, extracted)
      playable = resolved.playable
      persist = resolved.persist || extracted
    } else if (extracted) {
      playable = extracted
      persist = extracted
    }
    const url = playable

    if (!url) {
      if (timedOut && jobId) {
        if (selectedKeyId.value === snapshot.keyId) {
          videoStatus.value = job.status || 'running'
          videoSaveMessage.value = t('creazyCanvas.result.autoSaved')
          videoError.value = t('creazyCanvas.errors.serviceBusy')
        }
        if (runningWorkId) {
          await updateWorkRecord(runningWorkId, {
            status: 'running',
            gateway_remote_id: jobId,
            error_message: '',
          })
        }
        void loadWorks({ quiet: true })
        return
      }
      const err =
        typeof job.error === 'string'
          ? job.error
          : job.error && typeof job.error === 'object'
            ? job.error.message
            : ''
      const msg = mapGatewayError(
        { message: err || '', status: 0 },
        err || (t('creazyCanvas.result.failed') + ': ' + (job.status || 'unknown')),
      )
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, {
          status: 'failed',
          gateway_remote_id: jobId || undefined,
          error_message: msg,
        })
      } else {
        await persistWork({
          kind: 'video',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: baseParams,
          gateway_type: 'video_job',
          gateway_remote_id: jobId,
          error_message: msg,
        })
      }
      failedPersisted = true
      throw new Error(msg)
    }

    if (selectedKeyId.value === snapshot.keyId) {
      setVideoResultPlayback(url, persist)
      videoSaveMessage.value = t('creazyCanvas.result.autoSaved')
    }

    const storedUrl =
      (persist && !isBlobUrl(persist) ? persist : '') ||
      (jobId ? '/v1/videos/jobs/' + jobId + '/content' : '') ||
      url
    const posterUrl = snapshot.startFrameUrl || ''
    const successPayload = {
      status: 'succeeded' as const,
      public_model: snapshot.model,
      prompt: snapshot.prompt,
      params: {
        ...baseParams,
        result_urls: [storedUrl],
        poster_url: posterUrl || undefined,
        start_frame_url: posterUrl || undefined,
      },
      gateway_type: 'video_job' as const,
      gateway_remote_id: jobId,
      preview_url: posterUrl || storedUrl,
      object_url: storedUrl,
      mime_type: 'video/mp4',
      error_message: '',
    }
    let saved: CreazyWork | null = null
    if (runningWorkId) {
      saved = await updateWorkRecord(runningWorkId, successPayload)
    }
    if (!saved) {
      saved = await persistWork({
        kind: 'video',
        api_key_id: snapshot.keyId,
        ...successPayload,
      })
    }
    if (saved?.id) {
      const playableUrl =
        selectedKeyId.value === snapshot.keyId ? videoResultUrl.value : url
      if (playableUrl) {
        if (isBlobUrl(playableUrl)) workPreviewBlobUrls.add(playableUrl)
        workPreviewUrls[String(saved.id)] = playableUrl
      } else if (posterUrl && !needsAuthForMediaPlayback(posterUrl)) {
        workPreviewUrls[String(saved.id)] = posterUrl
      }
    }
    void loadWorks({ quiet: true })
  } catch (error: any) {
    const msg = mapGatewayError(error)
    if (selectedKeyId.value === snapshot.keyId) {
      videoError.value = msg
    }
    if (!failedPersisted && snapshot.model && snapshot.prompt) {
      if (runningWorkId) {
        await updateWorkRecord(runningWorkId, {
          status: 'failed',
          gateway_type: 'video_job',
          gateway_remote_id: jobId || undefined,
          error_message: msg,
        })
      } else {
        await persistWork({
          kind: 'video',
          api_key_id: snapshot.keyId,
          status: 'failed',
          public_model: snapshot.model,
          prompt: snapshot.prompt,
          params: baseParams,
          gateway_type: 'video_job',
          gateway_remote_id: jobId || undefined,
          error_message: msg,
        })
      }
      void loadWorks({ quiet: true })
    }
  } finally {
    activeVideoJobs.value = Math.max(0, activeVideoJobs.value - 1)
    generatingVideo.value = activeVideoJobs.value > 0
    if (selectedKeyId.value === snapshot.keyId) {
      activeVideoWorkId.value = null
    }
  }
}



async function loadWorks(opts?: { quiet?: boolean }) {
  if (!selectedKeyId.value) {
    works.value = []
    loadingWorks.value = false
    clearWorksPoll()
    return
  }
  if (!opts?.quiet) loadingWorks.value = true
  try {
    // On works tab honor filters; on image/video task boards load full key history.
    const useFilters = activeTab.value === 'works'
    const res = await listWorks({
      page: 1,
      page_size: 50,
      api_key_id: selectedKeyId.value,
      kind: useFilters ? (worksFilterKind.value || undefined) : undefined,
      status: useFilters ? (worksFilterStatus.value || undefined) : undefined,
    })
    // Defense-in-depth: only keep works for the currently selected key.
    const keyId = Number(selectedKeyId.value)
    works.value = (res.items || []).filter((w) => Number(w.api_key_id) === keyId)
    void hydrateWorkPreviews(works.value)
    scheduleWorksPoll()
  } catch (error: any) {
    if (!opts?.quiet) {
      appStore.showError(error?.response?.data?.detail || error?.message || t('creazyCanvas.works.loadFailed'))
    }
  } finally {
    if (!opts?.quiet) loadingWorks.value = false
  }
}

function openOrDownloadUrl(url: string, filename = 'creazy-media') {
  if (!url) return
  if (isBlobUrl(url)) {
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    return
  }
  window.open(url, '_blank', 'noopener')
}

async function downloadWork(work: CreazyWork) {
  if (isExpired(work)) {
    appStore.showError(t('creazyCanvas.works.expiredDownload'))
    return
  }
  downloadingWorkId.value = String(work.id)
  try {
    const res = await getWorkDownloadURL(work.id)
    if (res.source === 'object' && res.url && !needsAuthForMediaPlayback(res.url)) {
      openOrDownloadUrl(res.url)
      return
    }
    const params = (work.params || {}) as Record<string, unknown>
    const resultUrls = Array.isArray(params.result_urls) ? (params.result_urls as string[]) : []
    const fallback =
      (res.source === 'object' ? res.url : '') ||
      work.object_url ||
      work.preview_url ||
      resultUrls[0] ||
      ''
    if (fallback && !needsAuthForMediaPlayback(fallback)) {
      openOrDownloadUrl(fallback)
      return
    }

    // JWT session content proxy — no pasted secret required.
    try {
      const blobUrl = await getWorkContentBlob(work.id)
      if (blobUrl) {
        openOrDownloadUrl(blobUrl, isImageWork(work) ? 'creazy-image.png' : 'creazy-video.mp4')
        if (isBlobUrl(blobUrl)) workPreviewBlobUrls.add(blobUrl)
        return
      }
    } catch (error) {
      console.warn('[creazy-canvas] session download failed', error)
    }

    // Legacy: client-held secret fallback
    if ((work.gateway_type === 'video_job' || (work.kind || '').toLowerCase() === 'video') && work.gateway_remote_id) {
      if (work.api_key_id && selectedKeyId.value !== work.api_key_id) {
        selectedKeyId.value = work.api_key_id
      }
      const apiKey = resolveWorkApiKeySecret(work)
      if (!apiKey) {
        // Session proxy already failed; secret is only a legacy fallback for download.
        throw new Error(t('creazyCanvas.errors.downloadFailed'))
      }
      const url = await getVideoContentURL(apiKey, work.gateway_remote_id)
      if (url) {
        openOrDownloadUrl(url, 'creazy-video.mp4')
        if (isBlobUrl(url)) workPreviewBlobUrls.add(url)
        return
      }
    }
    if (fallback) {
      openOrDownloadUrl(fallback)
      return
    }
    throw new Error(res.gateway_hint || t('creazyCanvas.errors.downloadFailed'))
  } catch (error: any) {
    appStore.showError(
      error?.response?.data?.detail || error?.response?.data?.message || error?.message || t('creazyCanvas.errors.downloadFailed'),
    )
  } finally {
    downloadingWorkId.value = ''
  }
}

async function removeWork(work: CreazyWork) {
  deletingWorkId.value = String(work.id)
  try {
    await deleteWork(work.id)
    works.value = works.value.filter((w) => w.id !== work.id)
    appStore.showSuccess(t('creazyCanvas.works.deleted'))
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || error?.message || t('creazyCanvas.works.deleteFailed'))
  } finally {
    deletingWorkId.value = ''
  }
}

async function reuseWork(work: CreazyWork) {
  if (work.api_key_id) selectedKeyId.value = work.api_key_id
  const params = (work.params || {}) as Record<string, unknown>
  const kind = (work.kind || '').toLowerCase()
  if (selectedKeyId.value) {
    await loadCatalog()
  }
  if (kind === 'video') {
    videoForm.prompt = work.prompt || ''
    if (work.public_model && videoModels.value.some((m) => m.id === work.public_model)) {
      videoForm.model = work.public_model
    }
    syncFormModelsFromCatalog()
    if (params.resolution && videoResolutionOptions.value.includes(String(params.resolution))) {
      videoForm.resolution = String(params.resolution)
    }
    if (params.duration != null && videoDurationOptions.value.includes(Number(params.duration))) {
      videoForm.duration = Number(params.duration)
    }
    if (params.aspect_ratio && videoAspectOptions.value.includes(String(params.aspect_ratio))) {
      videoForm.aspectRatio = String(params.aspect_ratio)
    }
    videoForm.generateAudio = Boolean(params.generate_audio) && mediaCaps.value.allowGeneratedAudio
    switchTab('video')
  } else {
    imageForm.prompt = work.prompt || ''
    if (work.public_model && imageModels.value.some((m) => m.id === work.public_model)) {
      imageForm.model = work.public_model
    }
    syncFormModelsFromCatalog()
    if (params.size && imageSizeOptions.value.includes(String(params.size))) {
      imageForm.size = String(params.size)
    }
    switchTab('image')
  }
}

watch(
  () => videoForm.model,
  () => {
    resetVideoMedia()
    if (mediaCaps.value.allowGeneratedAudio || mediaCaps.value.forceGeneratedAudio) {
      videoForm.generateAudio = true
    } else {
      videoForm.generateAudio = false
    }
  },
)

watch(
  () => [mediaCaps.value.allowGeneratedAudio, mediaCaps.value.forceGeneratedAudio] as const,
  ([allow, force]) => {
    videoForm.generateAudio = Boolean(allow || force)
  },
)

watch(
  () => route.path,
  () => {
    activeTab.value = resolveTabFromRoute()
    void loadWorks({ quiet: activeTab.value !== 'works' })
  },
)

onMounted(async () => {
  activeTab.value = resolveTabFromRoute()
  if (route.path === '/creazy-canvas') {
    router.replace('/creazy-canvas/image')
  }
  await loadKeys()
})

function onPreviewKeydown(ev: KeyboardEvent) {
  if (ev.key === 'Escape' && mediaPreview.value) {
    closeMediaPreview()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onPreviewKeydown)
})

onBeforeUnmount(() => {
  cancelled = true
  clearPoll()
  clearWorksPoll()
  closeMediaPreview()
  clearVideoResultPlayback()
  for (const u of workPreviewBlobUrls) revokeBlobUrl(u)
  workPreviewBlobUrls.clear()
  window.removeEventListener('keydown', onPreviewKeydown)
})
</script>


