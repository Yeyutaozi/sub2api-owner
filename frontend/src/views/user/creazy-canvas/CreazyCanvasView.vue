<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <div class="cc-hero rounded-2xl border border-gray-200/80 bg-gradient-to-br from-white via-slate-50 to-indigo-50/60 p-5 shadow-sm dark:border-dark-700 dark:from-dark-900 dark:via-dark-900 dark:to-indigo-950/20 sm:p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="min-w-0">
            <div class="inline-flex items-center gap-2 rounded-full bg-indigo-100/80 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300">
              Creazy Canvas
            </div>
            <h1 class="mt-2 text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
              {{ t('creazyCanvas.title') }}
            </h1>
            <p class="mt-1 max-w-2xl text-sm leading-6 text-gray-500 dark:text-gray-400">
              {{ t('creazyCanvas.subtitle') }}
            </p>
          </div>
          <div class="w-full space-y-2 lg:max-w-md">
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
              <p class="input-hint">{{ t('creazyCanvas.key.capabilityHint') }}</p>
              <p v-if="!loadingKeys && keys.length === 0" class="input-hint text-amber-600 dark:text-amber-400">
                {{ t('creazyCanvas.key.empty') }}
              </p>
            </div>
            <div v-if="selectedKeyId" class="rounded-xl border border-white/70 bg-white/80 p-3 shadow-sm backdrop-blur dark:border-dark-600 dark:bg-dark-800/70">
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
              <div
                v-if="userBalance != null"
                class="mt-2 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-2 dark:border-dark-600"
              >
                <span class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                  {{ t('creazyCanvas.key.balance') }}
                </span>
                <span class="font-mono text-sm font-semibold tabular-nums text-emerald-700 dark:text-emerald-300">
                  {{ formatMoney(userBalance) }}
                </span>
                <span class="text-[11px] text-gray-400">{{ t('creazyCanvas.key.balanceHint') }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="cc-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          class="cc-tab"
          :class="{ 'cc-tab--active': activeTab === tab.id }"
          @click="switchTab(tab.id)"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- Image -->
      <section v-if="activeTab === 'image'" class="grid gap-5 lg:grid-cols-2">
        <div class="card cc-form-card space-y-5 p-5 sm:p-6">
          <div class="cc-field" :class="{ 'cc-field--error': imageFieldErrors.prompt }">
            <label class="cc-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <textarea
              v-model="imageForm.prompt"
              rows="5"
              class="input cc-textarea"
              :class="{ 'cc-input--error': imageFieldErrors.prompt }"
              :placeholder="t('creazyCanvas.form.promptPlaceholder')"
              @paste="onPasteMedia($event, 'imageRefs')"
            />
            <p v-if="imageFieldErrors.prompt" class="cc-field__error">{{ imageFieldErrors.prompt }}</p>
          </div>
          <div class="cc-panel cc-panel--image">
            <div class="cc-panel__head">
              <span class="cc-panel__badge cc-panel__badge--image">{{ t('creazyCanvas.form.paramsSection') }}</span>
              <span v-if="imageForm.model" class="cc-panel__meta">
                {{ selectedImageModel?.name || imageForm.model }}
              </span>
            </div>

            <div class="cc-field">
              <label class="cc-label">{{ t('creazyCanvas.form.model') }}</label>
              <select
                v-model="imageForm.model"
                class="input cc-control border-indigo-200 focus:border-indigo-400 dark:border-indigo-800"
                :disabled="loadingCatalog"
              >
                <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : t('creazyCanvas.form.selectPlaceholder') }}</option>
                <option v-for="m in imageModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
              </select>
              <p
                v-if="!loadingCatalog && selectedKeyId && imageModels.length === 0"
                class="input-hint text-amber-600 dark:text-amber-400"
              >
                {{ t('creazyCanvas.catalog.emptyImage') }}
              </p>
              <div v-if="imageModelCapChips.length" class="cc-cap-row" :aria-label="t('creazyCanvas.form.capsTitle')">
                <span v-for="(chip, idx) in imageModelCapChips" :key="'img-cap-' + idx" class="cc-cap-chip">{{ chip }}</span>
              </div>
            </div>

            <div class="cc-field">
              <div class="cc-label-row">
                <label class="cc-label">{{ t('creazyCanvas.form.size') }}</label>
                <span class="cc-size-current font-mono">{{ imageForm.size || '—' }}</span>
              </div>
              <div class="cc-chip-row" role="listbox" :aria-label="t('creazyCanvas.form.size')">
                <button
                  v-for="s in imageSizeOptions"
                  :key="'size-chip-' + s"
                  type="button"
                  role="option"
                  class="cc-chip"
                  :class="{ 'cc-chip--active': imageSizePresetValue === s }"
                  :aria-selected="imageSizePresetValue === s"
                  @click="selectImageSizePreset(s)"
                >
                  {{ s }}
                </button>
                <button
                  v-if="imageAllowCustomSize"
                  type="button"
                  role="option"
                  class="cc-chip"
                  :class="{ 'cc-chip--active': imageSizePresetValue === '__custom__' }"
                  :aria-selected="imageSizePresetValue === '__custom__'"
                  @click="focusImageSizeCustom"
                >
                  {{ t('creazyCanvas.form.sizeCustomOption') }}
                </button>
              </div>
              <input
                v-if="imageAllowCustomSize"
                ref="imageSizeCustomInput"
                v-model="imageForm.size"
                type="text"
                class="input cc-control font-mono text-sm"
                :placeholder="t('creazyCanvas.form.sizeCustomPlaceholder')"
                list="creazy-canvas-image-sizes"
                spellcheck="false"
                autocomplete="off"
                @blur="normalizeImageSizeInput"
              />
              <datalist v-if="imageAllowCustomSize" id="creazy-canvas-image-sizes">
                <option v-for="s in imageSizeOptions" :key="'dl-' + s" :value="s" />
              </datalist>
              <p class="input-hint">{{ imageSizeHintText }}</p>
              <p v-if="imageSizeLiveError" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ imageSizeLiveError }}</p>
            </div>
          </div>

          <div
            v-if="imageRefSupported"
            class="cc-media-panel rounded-xl border border-dashed border-indigo-300/80 bg-white/70 p-4 dark:border-indigo-800 dark:bg-dark-900/40"
          >
            <label class="input-label flex items-center justify-between gap-2">
              <span>
                {{ t('creazyCanvas.form.imageRefs') }}
                ({{ imageRefs.length }}/{{ imageRefMax }})
                <span v-if="imageRefRequired" class="ml-1 text-rose-500">*</span>
              </span>
              <button
                v-if="imageRefs.length"
                type="button"
                class="text-[11px] font-medium text-rose-600 hover:underline dark:text-rose-400"
                :disabled="!!uploadingImageRef"
                @click="clearImageRefs"
              >
                {{ t('creazyCanvas.form.clearAll') }}
              </button>
            </label>
            <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">
              {{ imageRefRequired ? t('creazyCanvas.form.imageRefsRequiredHint') : t('creazyCanvas.form.imageRefsHint') }}
            </p>
            <div
              class="cc-dropzone"
              :class="{ 'cc-field--error': imageFieldErrors.refs }"
              @dragover.prevent
              @drop="onDropMedia($event, 'imageRefs')"
            >
              <div class="flex flex-wrap items-center gap-2">
                <input
                  ref="imageRefInput"
                  type="file"
                  accept="image/*"
                  multiple
                  class="hidden"
                  @change="onPickImageRefs"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!!uploadingImageRef || !selectedKeyId || imageRefs.length >= imageRefMax"
                  @click="openImageRefPicker"
                >
                  {{ uploadingImageRef ? t('creazyCanvas.form.uploading') : t('creazyCanvas.form.uploadImage') }}
                </button>
                <span v-if="uploadingImageRef" class="text-xs text-gray-500">{{ imageRefUploadLabel }}</span>
                <span v-if="imageRefMax > 0" class="text-[11px] text-gray-500">{{ mediaProgressText(imageRefs.length, imageRefMax) }}</span>
              </div>
              <div v-if="imageRefMax > 0" class="cc-progress">
                <div class="cc-progress__bar" :style="{ width: Math.min(100, (imageRefs.length / imageRefMax) * 100) + '%' }" />
              </div>
              <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
              <p v-if="imageRefs.length > 1" class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaReorderHint') }}</p>
              <p v-if="imageFieldErrors.refs" class="cc-field__error">{{ imageFieldErrors.refs }}</p>
            </div>
            <ul v-if="imageRefs.length" class="mt-3 grid gap-2 sm:grid-cols-2">
              <li
                v-for="(item, idx) in imageRefs"
                :key="'img-ref-' + idx + item.media_url"
                class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-900"
                draggable="true"
                @dragstart="onMediaDragStart('imageRefs', idx)"
                @dragover.prevent
                @drop.prevent="onMediaDropReorder('imageRefs', idx)"
              >
                <img
                  :src="item.preview_url || item.media_url"
                  alt=""
                  class="h-12 w-12 rounded object-cover"
                />
                <div class="min-w-0 flex-1">
                  <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-100">{{ item.name }}</p>
                  <p class="truncate text-[11px] text-gray-400">{{ item.media_url }}</p>
                </div>
                <button type="button" class="text-xs text-rose-600 hover:underline" @click="removeImageRef(idx)">
                  {{ t('creazyCanvas.form.remove') }}
                </button>
              </li>
            </ul>
          </div>

          <div class="cc-rules-card" v-if="selectedImageModel || imageModelCapChips.length">
            <div class="cc-rules-card__title">{{ t('creazyCanvas.form.rulesTitle') }}</div>
            <div class="cc-rules-card__chips">
              <span v-for="(chip, idx) in imageModelCapChips" :key="'img-cap-' + idx" class="cc-rules-card__chip">{{ chip }}</span>
            </div>
          </div>
          <div v-else class="cc-rules-card cc-rules-card--empty">{{ t('creazyCanvas.form.rulesEmpty') }}</div>

          <div class="cc-create">
            <div class="cc-create__head">
              <span class="cc-create__title">{{ t('creazyCanvas.form.createSection') }}</span>
              <span class="cc-create__hint">{{ t('creazyCanvas.form.createSectionHintImage') }}</span>
            </div>
            <div class="cc-create__meta" aria-live="polite">
              <span class="cc-create__price">{{ imagePriceEstimateText }}</span>
              <span class="cc-create__shortcut">{{ t('creazyCanvas.form.submitShortcut') }}</span>
            </div>
            <p v-if="imageBalanceHint" class="cc-create__balance" :class="{ 'cc-create__balance--blocked': imageBalanceBlocked }">{{ imageBalanceHint }}</p>
            <p v-if="draftNotice && activeTab === 'image'" class="cc-create__draft">{{ draftNotice }}</p>
            <p v-else-if="draftSavedAtText && activeTab === 'image'" class="cc-create__draft">{{ draftSavedAtText }}</p>
            <div class="cc-create__actions">
            <button
              type="button"
              class="btn btn-primary cc-submit"
              :disabled="submittingImage || !selectedKeyId || resolvingKeySecret || !hasKeySecret || imageBalanceBlocked"
              @click="generateImage"
            >
              <Icon v-if="submittingImage" name="refresh" size="sm" class="mr-2 animate-spin" />
              {{ submittingImage ? t('creazyCanvas.form.submitting') : t('creazyCanvas.form.generate') }}
            </button>
            <button type="button" class="btn btn-secondary cc-submit-secondary" :disabled="submittingImage" @click="clearImageForm()">
              {{ t('creazyCanvas.form.clearForm') }}
            </button>
            </div>
            <p v-if="imageRunningCount" class="text-xs text-amber-600 dark:text-amber-400">
              {{ t('creazyCanvas.tasks.runningCount', { n: imageRunningCount }) }}
            </p>
            <p v-if="imageError" class="text-sm text-red-600 dark:text-red-400">{{ imageError }}</p>
            <p v-if="imageSaveMessage" class="text-sm text-green-600 dark:text-green-400">{{ imageSaveMessage }}</p>
          </div>
        </div>

        <div class="card cc-board-card space-y-4 p-5 sm:p-6">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <h2 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="mt-0.5 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.tasks.subtitle') }}</p>
              <div v-if="worksTotal > 0" class="cc-pagination cc-pagination--board mt-2">
                <span class="cc-pagination__meta">
                  {{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}
                </span>
                <div class="cc-pagination__actions">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage <= 1"
                @click="goToWorksPrevPage($event)"
              >
                {{ t('creazyCanvas.works.prevPage') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage >= worksPages"
                @click="goToWorksNextPage($event)"
              >
                {{ t('creazyCanvas.works.nextPage') }}
              </button>
              <label class="cc-pagination__jump">
                <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                <input
                  v-model="worksPageJumpInput"
                  type="number"
                  min="1"
                  :max="worksPages"
                  class="cc-pagination__input"
                  :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')"
                  @keyup.enter="jumpWorksPageFromInput"
                />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">
                  {{ t('creazyCanvas.works.pageJump') }}
                </button>
              </label>
              <label v-if="String(activeTab) === 'works'" class="cc-pagination__size">
                <span class="text-[11px] text-gray-500">{{ t('creazyCanvas.works.pageSize') }}</span>
                <select v-model.number="worksPageSizeChoice" class="cc-pagination__select" @change="onWorksPageSizeChange">
                  <option v-for="n in worksPageSizeOptions" :key="'ps-' + n" :value="n">
                    {{ t('creazyCanvas.works.pageSizeOption', { n }) }}
                  </option>
                </select>
              </label>
            </div>
              </div>
            </div>
            <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loadingWorks" @click="() => loadWorks()">
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
              <p v-if="workErrorText(work)" class="mt-1 line-clamp-2 text-xs text-red-600 dark:text-red-300">{{ workErrorText(work) }}</p>
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

          <div v-if="worksTotal > 0" class="cc-pagination">
            <span class="cc-pagination__meta">
              {{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}
            </span>
            <div class="cc-pagination__actions">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage <= 1"
                @click="goToWorksPrevPage($event)"
              >
                {{ t('creazyCanvas.works.prevPage') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage >= worksPages"
                @click="goToWorksNextPage($event)"
              >
                {{ t('creazyCanvas.works.nextPage') }}
              </button>
              <label class="cc-pagination__jump">
                <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                <input
                  v-model="worksPageJumpInput"
                  type="number"
                  min="1"
                  :max="worksPages"
                  class="cc-pagination__input"
                  :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')"
                  @keyup.enter="jumpWorksPageFromInput"
                />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">
                  {{ t('creazyCanvas.works.pageJump') }}
                </button>
              </label>
              <label v-if="String(activeTab) === 'works'" class="cc-pagination__size">
                <span class="text-[11px] text-gray-500">{{ t('creazyCanvas.works.pageSize') }}</span>
                <select v-model.number="worksPageSizeChoice" class="cc-pagination__select" @change="onWorksPageSizeChange">
                  <option v-for="n in worksPageSizeOptions" :key="'ps-' + n" :value="n">
                    {{ t('creazyCanvas.works.pageSizeOption', { n }) }}
                  </option>
                </select>
              </label>
            </div>
          </div>
        </div>
      </section>

      <!-- Video -->
      <section v-else-if="activeTab === 'video'" class="grid gap-5 lg:grid-cols-2">
        <div class="card cc-form-card space-y-5 p-5 sm:p-6">
          <div class="cc-field" :class="{ 'cc-field--error': videoFieldErrors.prompt }">
            <label class="cc-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <textarea
              v-model="videoForm.prompt"
              rows="5"
              class="input cc-textarea"
              :class="{ 'cc-input--error': videoFieldErrors.prompt }"
              :placeholder="t('creazyCanvas.form.promptPlaceholder')"
              @paste="onPasteMedia($event, 'refImages')"
            />
            <p v-if="videoFieldErrors.prompt" class="cc-field__error">{{ videoFieldErrors.prompt }}</p>
          </div>
          <div class="cc-panel cc-panel--video">
            <div class="cc-panel__head">
              <span class="cc-panel__badge cc-panel__badge--video">{{ t('creazyCanvas.form.paramsSection') }}</span>
              <span v-if="videoForm.model" class="cc-panel__meta">
                {{ selectedVideoModel?.name || videoForm.model }}
              </span>
            </div>

            <div class="cc-field">
              <label class="cc-label">{{ t('creazyCanvas.form.model') }}</label>
              <select
                v-model="videoForm.model"
                class="input cc-control border-violet-200 focus:border-violet-400 dark:border-violet-800"
                :disabled="loadingCatalog"
              >
                <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : t('creazyCanvas.form.selectPlaceholder') }}</option>
                <option v-for="m in videoModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
              </select>
              <p
                v-if="!loadingCatalog && selectedKeyId && videoModels.length === 0"
                class="input-hint text-amber-600 dark:text-amber-400"
              >
                {{ t('creazyCanvas.catalog.emptyVideo') }}
              </p>
              <div v-if="videoModelCapChips.length" class="cc-cap-row" :aria-label="t('creazyCanvas.form.capsTitle')">
                <span v-for="(chip, idx) in videoModelCapChips" :key="'vid-cap-' + idx" class="cc-cap-chip cc-cap-chip--video">{{ chip }}</span>
              </div>
            </div>

            <div class="cc-params-grid">
              <div class="cc-field">
                <label class="cc-label">{{ t('creazyCanvas.form.resolution') }}</label>
                <select v-model="videoForm.resolution" class="input cc-control">
                  <option v-for="r in videoResolutionOptions" :key="r" :value="r">{{ r }}</option>
                </select>
              </div>
              <div class="cc-field">
                <label class="cc-label">{{ t('creazyCanvas.form.duration') }}</label>
                <select v-model.number="videoForm.duration" class="input cc-control">
                  <option v-for="d in videoDurationOptions" :key="d" :value="d">{{ d }}</option>
                </select>
              </div>
              <div class="cc-field">
                <label class="cc-label">{{ t('creazyCanvas.form.aspectRatio') }}</label>
                <select v-model="videoForm.aspectRatio" class="input cc-control">
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
            <div class="mt-2 grid gap-1.5">
              <div v-if="mediaCaps.maxImages > 0" class="cc-progress-row">
                <span>{{ t('creazyCanvas.form.mediaLimitProgress', { used: refImages.length, max: mediaCaps.maxImages }) }}</span>
                <div class="cc-progress"><div class="cc-progress__bar" :style="{ width: Math.min(100, (refImages.length / mediaCaps.maxImages) * 100) + '%' }" /></div>
              </div>
              <div v-if="mediaCaps.maxVideos > 0" class="cc-progress-row">
                <span>{{ t('creazyCanvas.form.mediaLimitProgress', { used: refVideos.length, max: mediaCaps.maxVideos }) }}</span>
                <div class="cc-progress"><div class="cc-progress__bar" :style="{ width: Math.min(100, (refVideos.length / mediaCaps.maxVideos) * 100) + '%' }" /></div>
              </div>
              <div v-if="mediaCaps.maxAudios > 0" class="cc-progress-row">
                <span>{{ t('creazyCanvas.form.mediaLimitProgress', { used: refAudios.length, max: mediaCaps.maxAudios }) }}</span>
                <div class="cc-progress"><div class="cc-progress__bar" :style="{ width: Math.min(100, (refAudios.length / mediaCaps.maxAudios) * 100) + '%' }" /></div>
              </div>
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

          <div class="cc-rules-card" v-if="selectedVideoModel || videoModelCapChips.length">
            <div class="cc-rules-card__title">{{ t('creazyCanvas.form.rulesTitle') }}</div>
            <div class="cc-rules-card__chips">
              <span v-for="(chip, idx) in videoModelCapChips" :key="'vid-cap-' + idx" class="cc-rules-card__chip">{{ chip }}</span>
            </div>
            <p v-if="mediaCaps.maxImages || mediaCaps.maxVideos || mediaCaps.maxAudios" class="cc-rules-card__meta">
              {{ t('creazyCanvas.form.mediaLimitsDetail', {
                images: mediaCaps.maxImages,
                videos: mediaCaps.maxVideos,
                audios: mediaCaps.maxAudios,
                total: mediaCaps.maxTotal ? t('creazyCanvas.form.mediaTotal', { total: mediaCaps.maxTotal }) : ''
              }) }}
            </p>
          </div>
          <div v-else class="cc-rules-card cc-rules-card--empty">{{ t('creazyCanvas.form.rulesEmpty') }}</div>

          <div class="cc-create">
            <div class="cc-create__head">
              <span class="cc-create__title">{{ t('creazyCanvas.form.createSection') }}</span>
              <span class="cc-create__hint">{{ t('creazyCanvas.form.createSectionHintVideo') }}</span>
            </div>
            <div class="cc-create__meta" aria-live="polite">
              <span class="cc-create__price">{{ videoPriceEstimateText }}</span>
              <span class="cc-create__shortcut">{{ t('creazyCanvas.form.submitShortcut') }}</span>
            </div>
            <p v-if="videoBalanceHint" class="cc-create__balance" :class="{ 'cc-create__balance--blocked': videoBalanceBlocked }">{{ videoBalanceHint }}</p>
            <p v-if="draftNotice && activeTab === 'video'" class="cc-create__draft">{{ draftNotice }}</p>
            <p v-else-if="draftSavedAtText && activeTab === 'video'" class="cc-create__draft">{{ draftSavedAtText }}</p>
            <div class="cc-create__actions">
            <button
              type="button"
              class="btn btn-primary cc-submit"
              :disabled="submittingVideo || !!uploadingMedia || !selectedKeyId || resolvingKeySecret || !hasKeySecret || videoBalanceBlocked"
              @click="generateVideo"
            >
              <Icon v-if="submittingVideo" name="refresh" size="sm" class="mr-2 animate-spin" />
              {{ submittingVideo ? t('creazyCanvas.form.submitting') : t('creazyCanvas.form.generate') }}
            </button>
            <button type="button" class="btn btn-secondary cc-submit-secondary" :disabled="submittingVideo || !!uploadingMedia" @click="clearVideoForm()">
              {{ t('creazyCanvas.form.clearForm') }}
            </button>
            </div>
            <p v-if="videoRunningCount" class="text-xs text-amber-600 dark:text-amber-400">
              {{ t('creazyCanvas.tasks.runningCount', { n: videoRunningCount }) }}
            </p>
            <p v-if="videoStatus" class="text-sm text-gray-600 dark:text-gray-300">
              {{ t('creazyCanvas.result.status') }}: {{ videoStatus }}
            </p>
            <p v-if="videoError" class="text-sm text-red-600 dark:text-red-400">{{ videoError }}</p>
            <p v-if="videoSaveMessage" class="text-sm text-green-600 dark:text-green-400">{{ videoSaveMessage }}</p>
          </div>
        </div>

        <div class="card cc-board-card space-y-4 p-5 sm:p-6">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <h2 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="mt-0.5 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('creazyCanvas.tasks.subtitle') }}</p>
              <div v-if="worksTotal > 0" class="cc-pagination cc-pagination--board mt-2">
                <span class="cc-pagination__meta">
                  {{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}
                </span>
                <div class="cc-pagination__actions">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage <= 1"
                @click="goToWorksPrevPage($event)"
              >
                {{ t('creazyCanvas.works.prevPage') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage >= worksPages"
                @click="goToWorksNextPage($event)"
              >
                {{ t('creazyCanvas.works.nextPage') }}
              </button>
              <label class="cc-pagination__jump">
                <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                <input
                  v-model="worksPageJumpInput"
                  type="number"
                  min="1"
                  :max="worksPages"
                  class="cc-pagination__input"
                  :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')"
                  @keyup.enter="jumpWorksPageFromInput"
                />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">
                  {{ t('creazyCanvas.works.pageJump') }}
                </button>
              </label>
              <label v-if="String(activeTab) === 'works'" class="cc-pagination__size">
                <span class="text-[11px] text-gray-500">{{ t('creazyCanvas.works.pageSize') }}</span>
                <select v-model.number="worksPageSizeChoice" class="cc-pagination__select" @change="onWorksPageSizeChange">
                  <option v-for="n in worksPageSizeOptions" :key="'ps-' + n" :value="n">
                    {{ t('creazyCanvas.works.pageSizeOption', { n }) }}
                  </option>
                </select>
              </label>
            </div>
              </div>
            </div>
            <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loadingWorks" @click="() => loadWorks()">
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
              :data-work-id="work.id"
              class="cc-work-card rounded-xl border p-3 shadow-sm transition-colors"
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
              <div class="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-gray-500">
                <span v-if="isActiveWorkStatus(work.status) && work.created_at" class="font-mono tabular-nums text-amber-700 dark:text-amber-300">
                  {{ t('creazyCanvas.tasks.elapsed', { time: formatElapsed(work.created_at) }) }}
                </span>
                <span v-if="flashWorkIds[String(work.id)] && flashWorkIds[String(work.id)] > nowTick" class="rounded bg-indigo-100 px-1.5 py-0.5 font-semibold text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300">
                  {{ t('creazyCanvas.tasks.newBadge') }}
                </span>
              </div>
              <p class="mt-2 line-clamp-2 text-sm text-gray-800 dark:text-gray-100">{{ work.prompt || ('#' + work.id) }}</p>
              <div v-if="workErrorText(work)" class="mt-1">
                <p
                  class="text-xs text-red-600 dark:text-red-300"
                  :class="isErrorExpanded(work) ? '' : 'line-clamp-2'"
                >{{ workErrorText(work) }}</p>
                <div class="mt-1 flex flex-wrap gap-2">
                  <button type="button" class="text-[11px] font-medium text-red-600 underline-offset-2 hover:underline dark:text-red-300" @click="toggleErrorExpand(work)">
                    {{ isErrorExpanded(work) ? t('creazyCanvas.tasks.collapseError') : t('creazyCanvas.tasks.expandError') }}
                  </button>
                  <button type="button" class="text-[11px] font-medium text-gray-500 underline-offset-2 hover:underline" @click="copyWorkError(work)">
                    {{ t('creazyCanvas.tasks.copyError') }}
                  </button>
                </div>
              </div>
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
                <button type="button" class="btn btn-secondary btn-sm" @click="copyWorkPrompt(work)">
                  {{ t('creazyCanvas.tasks.copyPrompt') }}
                </button>
                <button
                  v-if="['failed','error'].includes(String(work.status||'').toLowerCase())"
                  type="button"
                  class="btn btn-primary btn-sm"
                  @click="retryWork(work)"
                >
                  {{ t('creazyCanvas.tasks.retry') }}
                </button>
                <button
                  v-if="isActiveWorkStatus(work.status) && !stoppedTrackIds[String(work.id)]"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="stopLocalTrack(work)"
                >
                  {{ t('creazyCanvas.tasks.stopTrack') }}
                </button>
              </div>
            </article>
          </div>

          <div v-if="worksTotal > 0" class="cc-pagination">
            <span class="cc-pagination__meta">
              {{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}
            </span>
            <div class="cc-pagination__actions">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage <= 1"
                @click="goToWorksPrevPage($event)"
              >
                {{ t('creazyCanvas.works.prevPage') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage >= worksPages"
                @click="goToWorksNextPage($event)"
              >
                {{ t('creazyCanvas.works.nextPage') }}
              </button>
              <label class="cc-pagination__jump">
                <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                <input
                  v-model="worksPageJumpInput"
                  type="number"
                  min="1"
                  :max="worksPages"
                  class="cc-pagination__input"
                  :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')"
                  @keyup.enter="jumpWorksPageFromInput"
                />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">
                  {{ t('creazyCanvas.works.pageJump') }}
                </button>
              </label>
              <label v-if="String(activeTab) === 'works'" class="cc-pagination__size">
                <span class="text-[11px] text-gray-500">{{ t('creazyCanvas.works.pageSize') }}</span>
                <select v-model.number="worksPageSizeChoice" class="cc-pagination__select" @change="onWorksPageSizeChange">
                  <option v-for="n in worksPageSizeOptions" :key="'ps-' + n" :value="n">
                    {{ t('creazyCanvas.works.pageSizeOption', { n }) }}
                  </option>
                </select>
              </label>
            </div>
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
                    @change="() => reloadWorksFromStart()"
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
                    @change="() => reloadWorksFromStart()"
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
                <label class="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
                  <span>{{ t('creazyCanvas.works.filterModel') }}</span>
                  <select
                    v-model="worksFilterModel"
                    class="input min-w-[8rem] py-1 text-xs"
                    :disabled="!selectedKeyId || loadingWorks"
                  >
                    <option value="">{{ t('creazyCanvas.works.filterAll') }}</option>
                    <option v-for="m in worksModelOptions" :key="'wm-' + m" :value="m">{{ m }}</option>
                  </select>
                </label>
                <label class="flex min-w-[10rem] flex-1 items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300 sm:max-w-[16rem]">
                  <span class="shrink-0">{{ t('creazyCanvas.works.filterQuery') }}</span>
                  <input
                    v-model="worksFilterQuery"
                    type="search"
                    class="input w-full min-w-0 py-1 text-xs"
                    :placeholder="t('creazyCanvas.works.filterQueryPlaceholder')"
                    :disabled="!selectedKeyId || loadingWorks"
                  />
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
              <div class="cc-empty-guide mt-5 w-full max-w-md text-left">
                <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-indigo-600 dark:text-indigo-300">{{ t('creazyCanvas.works.emptyGuideTitle') }}</p>
                <ol class="space-y-1.5 text-xs text-gray-600 dark:text-gray-300">
                  <li>1. {{ t('creazyCanvas.works.emptyGuide1') }}</li>
                  <li>2. {{ t('creazyCanvas.works.emptyGuide2') }}</li>
                  <li>3. {{ t('creazyCanvas.works.emptyGuide3') }}</li>
                </ol>
              </div>
            </div>

            <div
              v-else-if="filteredWorks.length === 0"
              class="flex min-h-[200px] flex-col items-center justify-center rounded-2xl border border-dashed border-gray-200 bg-gray-50/70 px-6 text-center dark:border-dark-700 dark:bg-dark-900/40"
            >
              <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('creazyCanvas.works.filterEmpty') }}</p>
            </div>

            <div v-else class="space-y-3">
              <div class="cc-batch-bar">
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="btn btn-secondary btn-sm" @click="selectAllWorksOnPage(true)">
                    {{ t('creazyCanvas.works.selectAllPage') }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedWorkIds.length" @click="selectAllWorksOnPage(false)">
                    {{ t('creazyCanvas.works.clearSelection') }}
                  </button>
                  <span v-if="selectedWorkIds.length" class="text-xs text-gray-500">{{ t('creazyCanvas.works.selectedCount', { n: selectedWorkIds.length }) }}</span>
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm text-rose-600"
                  :disabled="!selectedWorkIds.length || loadingWorks"
                  @click="batchDeleteSelectedWorks"
                >
                  {{ t('creazyCanvas.works.batchDelete') }}
                </button>
              </div>
              <article
                v-for="work in filteredWorks"
                :key="String(work.id)"
                :data-work-id="work.id"
                class="cc-work-card group relative overflow-hidden rounded-2xl border bg-white shadow-sm transition hover:shadow-md dark:bg-dark-900"
                :class="workCardClass(work)"
              >
                <div class="absolute left-3 top-3 z-10">
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
                    :checked="selectedWorkIds.includes(Number(work.id))"
                    @change="onWorkSelectChange(work, $event)"
                  />
                </div>
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

                    <div v-if="workErrorText(work)" class="mt-2 rounded-lg bg-red-50 px-2.5 py-1.5 dark:bg-red-950/30">
                      <p
                        class="text-xs text-red-600 dark:text-red-300"
                        :class="isErrorExpanded(work) ? '' : 'line-clamp-2'"
                      >{{ workErrorText(work) }}</p>
                      <div class="mt-1 flex flex-wrap gap-2">
                        <button type="button" class="text-[11px] font-medium text-red-600 underline-offset-2 hover:underline dark:text-red-300" @click="toggleErrorExpand(work)">
                          {{ isErrorExpanded(work) ? t('creazyCanvas.tasks.collapseError') : t('creazyCanvas.tasks.expandError') }}
                        </button>
                        <button type="button" class="text-[11px] font-medium text-gray-500 underline-offset-2 hover:underline" @click="copyWorkError(work)">
                          {{ t('creazyCanvas.tasks.copyError') }}
                        </button>
                      </div>
                    </div>
                    <p v-if="isActiveWorkStatus(work.status) && work.created_at" class="mt-2 font-mono text-[11px] tabular-nums text-amber-700 dark:text-amber-300">
                      {{ t('creazyCanvas.tasks.elapsed', { time: formatElapsed(work.created_at) }) }}
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
                    <button type="button" class="btn btn-secondary btn-sm sm:min-w-[6.5rem]" @click="copyWorkPrompt(work)">
                      {{ t('creazyCanvas.tasks.copyPrompt') }}
                    </button>
                    <button
                      v-if="['failed','error'].includes(String(work.status||'').toLowerCase())"
                      type="button"
                      class="btn btn-primary btn-sm sm:min-w-[6.5rem]"
                      @click="retryWork(work)"
                    >
                      {{ t('creazyCanvas.tasks.retry') }}
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

            <div v-if="worksTotal > 0" class="cc-pagination">
            <span class="cc-pagination__meta">
              {{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}
            </span>
            <div class="cc-pagination__actions">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage <= 1"
                @click="goToWorksPrevPage($event)"
              >
                {{ t('creazyCanvas.works.prevPage') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="loadingWorks || worksPage >= worksPages"
                @click="goToWorksNextPage($event)"
              >
                {{ t('creazyCanvas.works.nextPage') }}
              </button>
              <label class="cc-pagination__jump">
                <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                <input
                  v-model="worksPageJumpInput"
                  type="number"
                  min="1"
                  :max="worksPages"
                  class="cc-pagination__input"
                  :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')"
                  @keyup.enter="jumpWorksPageFromInput"
                />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">
                  {{ t('creazyCanvas.works.pageJump') }}
                </button>
              </label>
              <label v-if="String(activeTab) === 'works'" class="cc-pagination__size">
                <span class="text-[11px] text-gray-500">{{ t('creazyCanvas.works.pageSize') }}</span>
                <select v-model.number="worksPageSizeChoice" class="cc-pagination__select" @change="onWorksPageSizeChange">
                  <option v-for="n in worksPageSizeOptions" :key="'ps-' + n" :value="n">
                    {{ t('creazyCanvas.works.pageSizeOption', { n }) }}
                  </option>
                </select>
              </label>
            </div>
          </div>
          </div>
        </div>
      </section>
    </div>
    <!-- Floating task tray for concurrent jobs -->
    <div
      v-if="showTaskTray"
      class="cc-task-tray"
      :class="{ 'cc-task-tray--collapsed': !taskTrayExpanded }"
      role="complementary"
      :aria-label="t('creazyCanvas.tasks.trayTitle')"
    >
      <div class="cc-task-tray__bar">
        <button type="button" class="cc-task-tray__toggle" @click="taskTrayExpanded = !taskTrayExpanded">
          <span class="cc-task-tray__title">
            {{ t('creazyCanvas.tasks.trayTitle') }}
            <span v-if="totalRunningJobs" class="cc-task-tray__badge">{{ totalRunningJobs }}</span>
          </span>
          <span class="cc-task-tray__hint">{{ t('creazyCanvas.tasks.trayHint') }}</span>
          <span class="cc-task-tray__counts">
            <span v-if="trayStatusCounts.running">{{ t('creazyCanvas.tasks.trayRunning', { n: trayStatusCounts.running }) }}</span>
            <span v-if="trayStatusCounts.succeeded">{{ t('creazyCanvas.tasks.traySucceeded', { n: trayStatusCounts.succeeded }) }}</span>
            <span v-if="trayStatusCounts.failed">{{ t('creazyCanvas.tasks.trayFailed', { n: trayStatusCounts.failed }) }}</span>
          </span>
        </button>
        <div class="cc-task-tray__actions">
          <button type="button" class="cc-task-tray__link" @click="() => openTrayTaskBoard()">
            {{ t('creazyCanvas.tasks.open') }}
          </button>
          <button type="button" class="cc-task-tray__dismiss" @click="taskTrayDismissed = true">
            {{ t('creazyCanvas.tasks.dismiss') }}
          </button>
        </div>
      </div>
      <div v-if="taskTrayExpanded" class="cc-task-tray__list">
        <button
          v-for="work in trayWorks"
          :key="'tray-' + work.id"
          type="button"
          class="cc-task-tray__item"
          :class="workCardClass(work)"
          @click="() => openTrayTaskBoard(work)"
        >
          <span class="badge inline-flex items-center gap-1.5" :class="workStatusClass(work.status)">
            <span class="h-1.5 w-1.5 rounded-full" :class="workStatusDotClass(work.status)" />
            {{ workStatusLabel(work.status) }}
          </span>
          <span class="cc-task-tray__kind">{{ work.kind || '—' }}</span>
          <span class="cc-task-tray__model">{{ work.public_model || '—' }}</span>
          <span class="cc-task-tray__prompt">{{ work.prompt || ('#' + work.id) }}</span>
        </button>
        <p v-if="!trayWorks.length" class="cc-task-tray__empty">{{ t('creazyCanvas.tasks.empty') }}</p>
      </div>
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
  type ImageGenerationRequest,
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
import { useAppStore, useAuthStore } from '@/stores'
import {
  buildImageWorkParams,
  buildVideoWorkParams,
  pickStringParam,
  pickStringListParam,
  isReusableMediaUrl,
  readCanvasDraft,
  writeCanvasDraft,
  clearCanvasDraft,
  gatewayParamFieldKey,
  type CanvasDraftV1,
} from './composables/workParams'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

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
const imageRefs = ref<MediaItem[]>([])
const imageRefInput = ref<HTMLInputElement | null>(null)
const imageSizeCustomInput = ref<HTMLInputElement | null>(null)
const uploadingImageRef = ref(false)
const imageRefUploadLabel = ref('')
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
const resumingVideoWorkIds = new Set<number>()
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
const worksFilterModel = ref('')
const worksFilterQuery = ref('')
const worksPage = ref(1)
const worksPageSize = ref(10)
const worksTotal = ref(0)
const worksPages = ref(1)
/** Bumped on each works list request; ignore stale responses so poll cannot undo pagination. */
let worksLoadSeq = 0
/** Bumped only for non-quiet loads; used to clear loadingWorks even when quiet polls supersede. */
let worksLoadingSeq = 0
const taskTrayExpanded = ref(false)
const taskTrayDismissed = ref(false)
const draftNotice = ref('')
const draftSavedAt = ref<number | null>(null)
let draftSaveTimer: ReturnType<typeof setTimeout> | null = null
let draftHydrated = false
const downloadingWorkId = ref('')
const deletingWorkId = ref('')
/** UX: live clock for running-task elapsed timers */
const nowTick = ref(Date.now())
let nowTickTimer: ReturnType<typeof setInterval> | null = null
/** Brief highlight for freshly submitted work cards */
const flashWorkIds = reactive<Record<string, number>>({})
const expandedErrorIds = reactive<Record<string, boolean>>({})
const selectedWorkIds = ref<number[]>([])
const worksPageJumpInput = ref('')
const worksPageSizeChoice = ref(12)
const imageFieldErrors = reactive<Record<string, string>>({})
const videoFieldErrors = reactive<Record<string, string>>({})
const dragMediaKey = ref('')
const dragMediaIndex = ref(-1)
const actionNotice = ref('')
const focusWorkId = ref<number | null>(null)
const stoppedTrackIds = reactive<Record<string, boolean>>({})
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
const imageRefSupported = computed(() => {
  const m = selectedImageModel.value
  if (!m) return false
  if (m.supports_reference) return true
  return Number(m.max_reference_images || 0) > 0
})
const imageRefMax = computed(() => {
  const m = selectedImageModel.value
  const n = Number(m?.max_reference_images || 0)
  if (n > 0) return n
  return imageRefSupported.value ? 1 : 0
})
const imageRefRequired = computed(() => Boolean(selectedImageModel.value?.require_reference))

watch(
  () => [imageForm.model, imageRefSupported.value, imageRefMax.value] as const,
  () => {
    if (!imageRefSupported.value) {
      clearImageRefs()
      return
    }
    while (imageRefs.value.length > imageRefMax.value) {
      removeImageRef(imageRefs.value.length - 1)
    }
  },
)

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
  return (
    s === 'created' ||
    s === 'queued' ||
    s === 'running' ||
    s === 'pending' ||
    s === 'processing' ||
    s === 'settling' ||
    s === 'in_progress' ||
    s === 'inprogress' ||
    s === 'generating' ||
    s === 'working' ||
    s === 'submitted'
  )
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
  )
})

const videoTaskWorks = computed(() => {
  return sortTaskWorks(
    works.value.filter((w) => (w.kind || '').toLowerCase() === 'video'),
  )
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
  return { total: worksTotal.value || works.value.length, items }
})

const worksNeedSecretBanner = computed(() => {
  if (!works.value.length) return false
  return works.value.some((w) => canPreviewWork(w) && workNeedsSecret(w) && !workCoverUrl(w))
})

const userBalance = computed(() => {
  const b = authStore.user?.balance
  return typeof b === 'number' && Number.isFinite(b) ? b : null
})

const worksModelOptions = computed(() => {
  const set = new Set<string>()
  for (const w of works.value) {
    const m = String(w.public_model || '').trim()
    if (m) set.add(m)
  }
  return Array.from(set).sort((a, b) => a.localeCompare(b))
})

const filteredWorks = computed(() => {
  let list = works.value.slice()
  const model = worksFilterModel.value.trim()
  const q = worksFilterQuery.value.trim().toLowerCase()
  if (model) {
    list = list.filter((w) => String(w.public_model || '') === model)
  }
  if (q) {
    list = list.filter((w) => {
      const hay = [
        w.prompt,
        w.public_model,
        w.status,
        w.kind,
        w.error_message,
        w.gateway_remote_id,
      ]
        .map((x) => String(x || '').toLowerCase())
        .join(' ')
      return hay.includes(q)
    })
  }
  return list
})

const totalRunningJobs = computed(
  () =>
    activeImageJobs.value +
    activeVideoJobs.value +
    works.value.filter((w) => isActiveWorkStatus(w.status)).length,
)

const trayWorks = computed(() => {
  const active = works.value.filter((w) => isActiveWorkStatus(w.status))
  const rest = works.value.filter((w) => !isActiveWorkStatus(w.status))
  return [...sortTaskWorks(active), ...sortTaskWorks(rest)].slice(0, 12)
})

const showTaskTray = computed(
  () =>
    !taskTrayDismissed.value &&
    Boolean(selectedKeyId.value) &&
    (totalRunningJobs.value > 0 || trayWorks.value.some((w) => isActiveWorkStatus(w.status))),
)

function formatMoney(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(Number(n))) return ''
  const v = Number(n)
  if (Math.abs(v) >= 100) return v.toFixed(2)
  if (Math.abs(v) >= 1) return v.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
  return v.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

function pickPrice(prices: Record<string, number | null | undefined> | undefined, keys: string[]): number | null {
  if (!prices) return null
  for (const k of keys) {
    if (!k) continue
    const v = prices[k]
    if (v != null && Number.isFinite(Number(v))) return Number(v)
  }
  const entries = Object.entries(prices)
  for (const k of keys) {
    const hit = entries.find(([ek, ev]) => ek.toLowerCase() === k.toLowerCase() && ev != null)
    if (hit && hit[1] != null) return Number(hit[1])
  }
  return null
}

function imageBillingTier(size: string): string[] {
  const s = String(size || '').trim().toLowerCase()
  const out = [size, s]
  const m = s.match(/^(\d+)\s*[x×]\s*(\d+)$/i)
  if (m) {
    const w = Number(m[1])
    const h = Number(m[2])
    const longEdge = Math.max(w, h)
    if (longEdge >= 3000) out.push('4K', '4k')
    else if (longEdge >= 1600) out.push('2K', '2k')
    else out.push('1K', '1k')
  } else if (s === 'auto') {
    out.push('1K', '1k')
  }
  out.push('1K', '1k')
  return out
}

const imagePriceEstimate = computed(() => {
  const model = selectedImageModel.value
  if (!model) return null
  return pickPrice(model.prices as any, imageBillingTier(imageForm.size))
})

const videoUnitPriceEstimate = computed(() => {
  const model = selectedVideoModel.value
  if (!model) return null
  const res = String(videoForm.resolution || '')
  return pickPrice(model.prices as any, [res, res.toLowerCase(), res.toUpperCase(), '720p', '1080p', '1440p', '1440P'])
})

const videoPriceEstimate = computed(() => {
  const unit = videoUnitPriceEstimate.value
  if (unit == null) return null
  const model = selectedVideoModel.value
  const billingUnit = String(model?.billing_unit || 'per_second').toLowerCase()
  if (billingUnit === 'per_request' || billingUnit === 'per-request' || billingUnit === 'request') {
    return unit
  }
  // Default video billing is per generated second.
  const duration = Number(videoForm.duration || 0)
  if (!Number.isFinite(duration) || duration <= 0) return unit
  return unit * duration
})

const imagePriceEstimateText = computed(() => {
  const p = imagePriceEstimate.value
  if (p == null) return t('creazyCanvas.form.priceEstimateUnknown')
  return t('creazyCanvas.form.priceEstimate', { price: formatMoney(p) })
})

const videoPriceEstimateText = computed(() => {
  const p = videoPriceEstimate.value
  if (p == null) return t('creazyCanvas.form.priceEstimateUnknown')
  const model = selectedVideoModel.value
  const billingUnit = String(model?.billing_unit || 'per_second').toLowerCase()
  const isPerRequest = billingUnit === 'per_request' || billingUnit === 'per-request' || billingUnit === 'request'
  if (isPerRequest) {
    return t('creazyCanvas.form.priceEstimatePerRequest', { price: formatMoney(p) })
  }
  return t('creazyCanvas.form.priceEstimatePerSecond', {
    price: formatMoney(p),
    seconds: Number(videoForm.duration || 0) || '-',
  })
})

const trayStatusCounts = computed(() => {
  let running = 0
  let succeeded = 0
  let failed = 0
  for (const w of works.value) {
    const s = String(w.status || '').toLowerCase()
    if (isActiveWorkStatus(s)) running += 1
    else if (['succeeded', 'completed', 'success', 'done'].includes(s)) succeeded += 1
    else if (['failed', 'error', 'expired'].includes(s)) failed += 1
  }
  return { running, succeeded, failed }
})

const imageBalanceBlocked = computed(() => {
  const bal = userBalance.value
  const price = imagePriceEstimate.value
  if (bal == null || price == null) return false
  return bal + 1e-9 < price
})

const videoBalanceBlocked = computed(() => {
  const bal = userBalance.value
  const price = videoPriceEstimate.value
  if (bal == null || price == null) return false
  return bal + 1e-9 < price
})

const imageBalanceHint = computed(() => {
  const bal = userBalance.value
  const price = imagePriceEstimate.value
  if (bal == null || price == null) return ''
  if (bal + 1e-9 < price) {
    return t('creazyCanvas.form.balanceInsufficient', {
      price: formatMoney(price),
      balance: formatMoney(bal),
    })
  }
  if (bal < price * 2) {
    return t('creazyCanvas.form.balanceWarning', {
      price: formatMoney(price),
      balance: formatMoney(bal),
    })
  }
  return ''
})

const videoBalanceHint = computed(() => {
  const bal = userBalance.value
  const price = videoPriceEstimate.value
  if (bal == null || price == null) return ''
  if (bal + 1e-9 < price) {
    return t('creazyCanvas.form.balanceInsufficient', {
      price: formatMoney(price),
      balance: formatMoney(bal),
    })
  }
  if (bal < price * 2) {
    return t('creazyCanvas.form.balanceWarning', {
      price: formatMoney(price),
      balance: formatMoney(bal),
    })
  }
  return ''
})

const draftSavedAtText = computed(() => {
  void nowTick.value
  const ts = draftSavedAt.value
  if (!ts) return ''
  return t('creazyCanvas.form.draftSavedAt', { time: formatClockTime(ts) })
})

const worksPageSizeOptions = [6, 12, 24, 48]


const imageModelCapChips = computed(() => {
  const m = selectedImageModel.value
  if (!m) return [] as string[]
  const chips: string[] = []
  chips.push(m.async ? t('creazyCanvas.caps.async') : t('creazyCanvas.caps.sync'))
  if (imageRefSupported.value) {
    chips.push(t('creazyCanvas.caps.refImages', { n: imageRefMax.value }))
    if (imageRefRequired.value) chips.push(t('creazyCanvas.caps.refRequired'))
  } else {
    chips.push(t('creazyCanvas.caps.noRef'))
  }
  chips.push(imageAllowCustomSize.value ? t('creazyCanvas.caps.customSize') : t('creazyCanvas.caps.presetSize'))
  return chips
})

const videoModelCapChips = computed(() => {
  const m = selectedVideoModel.value
  if (!m) return [] as string[]
  const chips: string[] = []
  if (mediaCaps.value.allowGeneratedAudio) {
    chips.push(
      mediaCaps.value.forceGeneratedAudio
        ? t('creazyCanvas.caps.forceAudio')
        : t('creazyCanvas.caps.audio'),
    )
  }
  if (mediaCaps.value.allowStartFrame) chips.push(t('creazyCanvas.caps.startFrame'))
  if (mediaCaps.value.allowEndFrame) chips.push(t('creazyCanvas.caps.endFrame'))
  if (mediaCaps.value.maxImages > 0) chips.push(t('creazyCanvas.caps.refImages', { n: mediaCaps.value.maxImages }))
  if (mediaCaps.value.maxVideos > 0) chips.push(t('creazyCanvas.caps.refVideo', { n: mediaCaps.value.maxVideos }))
  if (mediaCaps.value.maxAudios > 0) chips.push(t('creazyCanvas.caps.refAudio', { n: mediaCaps.value.maxAudios }))
  if (mediaCaps.value.maxTotal > 0) chips.push(t('creazyCanvas.caps.mediaTotal', { n: mediaCaps.value.maxTotal }))
  return chips
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
    // Keep valid custom sizes (WxH / 1K-4K); only fall back when empty/invalid.
    if (!isValidImageSizeInput(imageForm.size)) {
      imageForm.size = imageSizeOptions.value[0]
    }
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

const imageAllowCustomSize = computed(() => Boolean(selectedImageModel.value?.allow_custom_size))

const imageSizeHintText = computed(() => {
  if (!imageAllowCustomSize.value) {
    return t('creazyCanvas.form.sizePresetOnlyHint')
  }
  const c = selectedImageModel.value?.size_constraints
  if (c?.max_edge && c?.multiple_of) {
    return t('creazyCanvas.form.sizeCustomHintShort')
  }
  return t('creazyCanvas.form.sizeCustomHintShort')
})

const imageSizeLiveError = computed(() => {
  const size = String(imageForm.size || '').trim()
  if (!size) return ''
  if (isValidImageSizeInput(size)) return ''
  // Avoid nagging while the user is still typing a partial WxH value.
  if (/^\d{1,5}$/.test(size)) return ''
  if (/^\d{1,5}\s*[xX×*]\s*$/.test(size)) return ''
  if (/^\d{1,5}\s*[xX×*]\s*\d{1,5}$/.test(size) && !parseImageSizeWxH(size)) return ''
  return describeImageSizeInvalid(size)
})

const imageSizePresetValue = computed(() => {
  const size = String(imageForm.size || '').trim()
  if (!size) return imageSizeOptions.value[0] || ''
  if (imageSizeOptions.value.includes(size)) return size
  return '__custom__'
})

function selectImageSizePreset(size: string) {
  imageForm.size = size
}

function focusImageSizeCustom() {
  // Mark as custom by leaving freeform value; focus the text field for editing.
  if (imageSizeOptions.value.includes(String(imageForm.size || '').trim())) {
    // Keep current size text so user can tweak from a known preset.
  } else if (!String(imageForm.size || '').trim()) {
    imageForm.size = imageSizeOptions.value[0] || '1024x1024'
  }
  requestAnimationFrame(() => {
    imageSizeCustomInput.value?.focus()
    imageSizeCustomInput.value?.select()
  })
}

function normalizeImageSizeInput() {
  const raw = String(imageForm.size || '').trim()
  imageForm.size = raw ? canonicalizeImageSizeInput(raw) : raw
}

function parseImageSizeWxH(raw: string): { w: number; h: number } | null {
  const size = String(raw || '').trim()
  // Accept x / X / × as separators.
  const m = size.match(/^(\d{2,5})\s*[xX×]\s*(\d{2,5})$/)
  if (!m) return null
  const w = Number(m[1])
  const h = Number(m[2])
  if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) return null
  return { w, h }
}

function isValidImageSizeInput(raw: string): boolean {
  const size = String(raw || '').trim()
  if (!size) return false

  const model = selectedImageModel.value
  const presets = imageSizeOptions.value
  if (presets.some((s) => String(s).toLowerCase() === size.toLowerCase())) return true

  const allowCustom = Boolean(model?.allow_custom_size)
  if (!allowCustom) return false

  const constraints = model?.size_constraints || null
  const aliases = (constraints?.aliases || []).map((a) => String(a).toLowerCase())
  if (aliases.includes(size.toLowerCase())) return true

  // Without official constraints: keep legacy free-form aliases + soft WxH bounds.
  if (!constraints) {
    if (/^(auto|1k|2k|4k)$/i.test(size)) return true
    const dims = parseImageSizeWxH(size)
    if (!dims) return false
    return dims.w >= 64 && dims.h >= 64 && dims.w <= 8192 && dims.h <= 8192
  }

  // Official geometric constraints (gpt-image-2).
  // Note: 1K/2K/4K are Gemini-style enums, not accepted as free-form for OpenAI.
  const dims = parseImageSizeWxH(size)
  if (!dims) return false
  const { w, h } = dims
  const multiple = Number(constraints.multiple_of || 0)
  if (multiple > 0 && (w % multiple !== 0 || h % multiple !== 0)) return false
  const maxEdge = Number(constraints.max_edge || 0)
  if (maxEdge > 0 && (w > maxEdge || h > maxEdge)) return false
  const pixels = w * h
  const minPixels = Number(constraints.min_pixels || 0)
  const maxPixels = Number(constraints.max_pixels || 0)
  if (minPixels > 0 && pixels < minPixels) return false
  if (maxPixels > 0 && pixels > maxPixels) return false
  const maxRatio = Number(constraints.max_aspect_ratio || 0)
  if (maxRatio > 0) {
    const long = Math.max(w, h)
    const short = Math.min(w, h)
    if (short <= 0 || long / short > maxRatio + 1e-9) return false
  }
  return true
}

/** Field-level size validation message for custom / constrained models. Empty when valid. */
function describeImageSizeInvalid(raw: string): string {
  const size = String(raw || '').trim()
  if (!size) return t('creazyCanvas.form.sizeRequired')

  const model = selectedImageModel.value
  const presets = imageSizeOptions.value
  if (presets.some((s) => String(s).toLowerCase() === size.toLowerCase())) return ''

  const allowCustom = Boolean(model?.allow_custom_size)
  if (!allowCustom) {
    const list = presets.slice(0, 10).join(', ') || '-'
    return t('creazyCanvas.form.sizeNotInPresets', { size, presets: list })
  }

  const constraints = model?.size_constraints || null
  const aliases = (constraints?.aliases || []).map((a) => String(a).toLowerCase())
  if (aliases.includes(size.toLowerCase())) return ''

  // Without official constraints: free-form aliases + soft WxH bounds.
  if (!constraints) {
    if (/^(auto|1k|2k|4k)$/i.test(size)) return ''
    const dims = parseImageSizeWxH(size)
    if (!dims) return t('creazyCanvas.form.sizeFormatInvalid')
    if (dims.w < 64 || dims.h < 64 || dims.w > 8192 || dims.h > 8192) {
      return t('creazyCanvas.form.sizeOutOfRange', {
        size: `${dims.w}x${dims.h}`,
        min: 64,
        max: 8192,
      })
    }
    return ''
  }

  // Constrained models (e.g. gpt-image-2): reject Gemini-style enums unless aliased.
  if (/^(auto|1k|2k|4k)$/i.test(size)) {
    const aliasHint = (constraints.aliases || []).join(', ') || t('creazyCanvas.form.sizeCustomOption')
    return t('creazyCanvas.form.sizeAliasNotSupported', { size, aliases: aliasHint })
  }

  const dims = parseImageSizeWxH(size)
  if (!dims) return t('creazyCanvas.form.sizeFormatInvalid')
  const { w, h } = dims
  const display = `${w}x${h}`
  const multiple = Number(constraints.multiple_of || 0)
  if (multiple > 0 && (w % multiple !== 0 || h % multiple !== 0)) {
    return t('creazyCanvas.form.sizeNotMultiple', { size: display, multiple })
  }
  const maxEdge = Number(constraints.max_edge || 0)
  if (maxEdge > 0 && (w > maxEdge || h > maxEdge)) {
    return t('creazyCanvas.form.sizeMaxEdge', {
      size: display,
      max: maxEdge,
      edge: Math.max(w, h),
    })
  }
  const pixels = w * h
  const minPixels = Number(constraints.min_pixels || 0)
  const maxPixels = Number(constraints.max_pixels || 0)
  if (minPixels > 0 && pixels < minPixels) {
    return t('creazyCanvas.form.sizeMinPixels', { size: display, pixels, min: minPixels })
  }
  if (maxPixels > 0 && pixels > maxPixels) {
    return t('creazyCanvas.form.sizeMaxPixels', { size: display, pixels, max: maxPixels })
  }
  const maxRatio = Number(constraints.max_aspect_ratio || 0)
  if (maxRatio > 0) {
    const long = Math.max(w, h)
    const short = Math.min(w, h)
    if (short <= 0 || long / short > maxRatio + 1e-9) {
      const ratio = short > 0 ? (long / short).toFixed(2) : '∞'
      return t('creazyCanvas.form.sizeAspectRatio', { size: display, ratio, max: maxRatio })
    }
  }
  return ''
}

function canonicalizeImageSizeInput(raw: string): string {
  const size = String(raw || '').trim()
  if (!size) return size
  if (/^auto$/i.test(size)) return 'auto'
  if (/^(1k|2k|4k)$/i.test(size)) return size.toUpperCase()
  const dims = parseImageSizeWxH(size)
  if (!dims) return size
  return `${dims.w}x${dims.h}`
}

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
  resetWorksPage()
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

function normalizeWorkErrorMessage(status: string | undefined, message?: string | null): string | undefined {
  const st = String(status || '').toLowerCase()
  const msg = String(message ?? '').trim()
  if (st === 'failed' || st === 'error') {
    return msg || t('creazyCanvas.errors.generateFailed')
  }
  // Allow explicit clear on success paths.
  if (message === '') return ''
  return message === undefined || message === null ? undefined : msg
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
    if (partial.error_message !== undefined || partial.status) {
      const normalized = normalizeWorkErrorMessage(partial.status, partial.error_message)
      if (normalized !== undefined) payload.error_message = normalized
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
    const status = partial.status || 'created'
    const payload: CreateCreazyWorkRequest = {
      api_key_id: partial.api_key_id ?? selectedKeyId.value,
      kind: partial.kind,
      public_model: partial.public_model || '',
      prompt: partial.prompt || '',
      params: partial.params || {},
      gateway_type: partial.gateway_type,
      gateway_remote_id: partial.gateway_remote_id || '',
      status,
      error_message: normalizeWorkErrorMessage(status, partial.error_message),
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

function worksPollIntervalMs() {
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return 12000
  const busy =
    activeImageJobs.value > 0 ||
    activeVideoJobs.value > 0 ||
    works.value.some((w) => isActiveWorkStatus(w.status))
  return busy ? 3000 : 4000
}

function scheduleWorksPoll() {
  clearWorksPoll()
  const hasActive = works.value.some((w) => isActiveWorkStatus(w.status))
  if (!hasActive && activeImageJobs.value <= 0 && activeVideoJobs.value <= 0) return
  if (hasActive) void resumeOrphanedVideoWorks()
  worksPollTimer = setTimeout(() => {
    worksPollTimer = null
    if (cancelled) return
    void loadWorks({ quiet: true })
  }, worksPollIntervalMs())
}

function onVisibilityChange() {
  if (cancelled) return
  if (typeof document !== 'undefined' && document.visibilityState === 'visible') {
    const hasActive =
      works.value.some((w) => isActiveWorkStatus(w.status)) ||
      activeImageJobs.value > 0 ||
      activeVideoJobs.value > 0
    if (hasActive) {
      void loadWorks({ quiet: true })
    }
  } else {
    scheduleWorksPoll()
  }
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
  const param = String(
    error?.param ||
      error?.response?.data?.error?.param ||
      error?.response?.data?.param ||
      error?.response?.data?.field ||
      '',
  ).trim()

  let raw = String(
    error?.message ||
      error?.response?.data?.error?.message ||
      error?.response?.data?.message ||
      error?.response?.data?.detail ||
      '',
  ).trim()

  // Prefer structured body from gateway parse (error.raw) when message is empty/opaque JSON.
  if (error?.raw && typeof error.raw === 'object') {
    const body = error.raw as Record<string, any>
    const nested =
      (typeof body?.error === 'object' && body.error
        ? String(body.error.message || body.error.msg || body.error.detail || '')
        : typeof body?.error === 'string'
          ? body.error
          : '') ||
      String(body?.message || body?.msg || (typeof body?.detail === 'string' ? body.detail : '') || '')
    if (nested) {
      const rawIsGeneric =
        !raw ||
        /^invalid(\s+parameters?)?$/i.test(raw) ||
        raw === '参数不合法' ||
        raw === 'Bad Request' ||
        raw.startsWith('{')
      if (rawIsGeneric || nested.length > raw.length) {
        raw = nested.trim()
      }
    }
    // FastAPI detail array
    if (Array.isArray(body?.detail) && (!raw || /^invalid/i.test(raw) || raw === '参数不合法')) {
      const parts = body.detail
        .map((item: any) => {
          if (typeof item === 'string') return item
          if (!item || typeof item !== 'object') return ''
          const loc = Array.isArray(item.loc) ? item.loc.filter((x: any) => x !== 'body').join('.') : ''
          const msg = String(item.msg || item.message || '')
          if (loc && msg) return `${loc}: ${msg}`
          return msg || loc
        })
        .filter(Boolean)
      if (parts.length) raw = parts.join('; ')
    }
  }

  // Drop trailing "(param: x)" duplication; we re-attach via i18n when needed.
  raw = raw.replace(/\s*\(param:\s*[^)]+\)\s*$/i, '').trim()

  const lower = raw.toLowerCase()
  const code = String(error?.code || error?.response?.data?.error?.code || error?.response?.data?.code || '').toLowerCase()
  const blob = `${lower} ${code}`

  const looksUnsafe =
    blob.includes('traceback') ||
    blob.includes('stack trace') ||
    blob.includes('<html') ||
    blob.includes('<!doctype') ||
    (blob.includes(' at ') && blob.includes('.go:')) ||
    (blob.includes(' at ') && blob.includes('.ts:'))

  const isGenericInvalid =
    !raw ||
    /^invalid(\s+parameters?)?\.?$/i.test(raw) ||
    raw === '参数不合法' ||
    raw === 'Bad Request' ||
    /^validation\s*error\.?$/i.test(raw)

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
  if (
    status === 401 ||
    status === 403 ||
    blob.includes('invalid api key') ||
    blob.includes('unauthorized') ||
    (blob.includes('forbidden') && !blob.includes('upstream'))
  ) {
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
    if (looksUnsafe) {
      return t('creazyCanvas.errors.invalidParams')
    }
    // Allow longer field-level messages (e.g. size constraint explanations).
    const detail = raw.length > 360 ? raw.slice(0, 360).trim() + '…' : raw
    if (param && detail && !isGenericInvalid) {
      const fieldKey = gatewayParamFieldKey(param)
      const fieldLabel = t(`creazyCanvas.errors.fields.${fieldKey}`)
      const field = fieldLabel.startsWith('creazyCanvas.') ? param : fieldLabel
      if (detail.toLowerCase().includes(param.toLowerCase()) || detail.includes(field)) {
        return detail.length <= 360 ? detail : t('creazyCanvas.errors.invalidParamsDetail', { detail })
      }
      return t('creazyCanvas.errors.invalidParamField', { field, detail })
    }
    if (param && (isGenericInvalid || !detail)) {
      const fieldKey = gatewayParamFieldKey(param)
      const fieldLabel = t(`creazyCanvas.errors.fields.${fieldKey}`)
      const field = fieldLabel.startsWith('creazyCanvas.') ? param : fieldLabel
      return t('creazyCanvas.errors.invalidParamField', {
        field,
        detail: detail || t('creazyCanvas.errors.invalidParams'),
      })
    }
    if (detail && !isGenericInvalid && !(detail.includes('{') && detail.includes('}')) && !detail.includes('[')) {
      return detail
    }
    if (detail && !isGenericInvalid && detail.length <= 360) {
      return t('creazyCanvas.errors.invalidParamsDetail', { detail })
    }
    if (detail && isGenericInvalid) {
      return t('creazyCanvas.errors.invalidParams')
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

  if (
    raw &&
    raw.length <= 200 &&
    !(raw.includes('{') || raw.includes('[')) &&
    !blob.includes('http') &&
    !looksUnsafe
  ) {
    return raw
  }
  return fallback || t('creazyCanvas.errors.generateFailed')
}
function mapContentPreviewError(error: any): string {
  const status = Number(error?.status || error?.response?.status || 0)
  const reason = String(error?.reason || error?.response?.data?.reason || '').trim()
  let raw = String(
    error?.message ||
      error?.response?.data?.message ||
      error?.response?.data?.error?.message ||
      error?.response?.data?.detail ||
      '',
  ).trim()
  if (error?.raw && typeof error.raw === 'object') {
    const body = error.raw as Record<string, any>
    const nested =
      String(body?.message || body?.error?.message || body?.detail || body?.msg || '').trim()
    if (nested) raw = nested
  }
  const unsafe =
    /traceback|stack trace|<!doctype|<html|\bat\s+\S+\.(go|ts):/i.test(raw)
  if (raw && !unsafe && raw.length <= 360) {
    // Prefer backend creazy-canvas content errors as-is (already user-facing Chinese).
    if (
      reason.startsWith('CREAZY_CANVAS_') ||
      raw.includes('预览') ||
      raw.includes('过期') ||
      raw.includes('就绪') ||
      raw.includes('作品') ||
      raw.includes('上游') ||
      raw.toLowerCase().includes('service temporarily unavailable') ||
      raw.toLowerCase().includes('preview')
    ) {
      return raw
    }
  }
  if (status === 404) return t('creazyCanvas.works.previewFailedGeneric')
  const mapped = mapGatewayError(error, t('creazyCanvas.works.previewFailedGeneric'))
  // Avoid collapsing specific preview failures into pure "busy" without detail.
  if (
    mapped === t('creazyCanvas.errors.serviceBusy') &&
    raw &&
    !unsafe &&
    raw.length <= 240 &&
    !/^request failed with status code/i.test(raw)
  ) {
    return raw
  }
  return mapped || t('creazyCanvas.works.previewFailedGeneric')
}

function workErrorText(work?: CreazyWork | null): string {
  if (!work) return ''
  const msg = String(work.error_message || '').trim()
  if (msg) return msg
  const st = String(work.status || '').toLowerCase()
  if (st === 'failed' || st === 'error') return t('creazyCanvas.errors.generateFailed')
  return ''
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
  const classes: string[] = []
  if (isExpired(work) || s === 'expired') {
    classes.push('border-red-200/80 dark:border-red-900/40')
  } else if (['succeeded', 'completed', 'success', 'done'].includes(s)) {
    classes.push('border-emerald-200/90 dark:border-emerald-900/40')
  } else if (['failed', 'error'].includes(s)) {
    classes.push('border-red-200/90 dark:border-red-900/40')
  } else if (['running', 'pending', 'processing', 'queued'].includes(s)) {
    classes.push('border-amber-200/90 dark:border-amber-900/40')
  } else if (['created'].includes(s)) {
    classes.push('border-primary-200/90 dark:border-primary-900/40')
  } else {
    classes.push('border-gray-200 dark:border-dark-700')
  }
  const id = String(work.id || '')
  if (id && flashWorkIds[id] && flashWorkIds[id] > nowTick.value) {
    classes.push('cc-work-card--flash')
  }
  if (focusWorkId.value && Number(work.id) === Number(focusWorkId.value)) {
    classes.push('cc-work-card--focus')
  }
  if (id && stoppedTrackIds[id]) {
    classes.push('opacity-70')
  }
  return classes.join(' ')
}

function formatClockTime(ts: number): string {
  try {
    return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return ''
  }
}

function formatElapsed(fromIso?: string | null): string {
  void nowTick.value
  if (!fromIso) return ''
  const start = new Date(fromIso).getTime()
  if (!Number.isFinite(start)) return ''
  let sec = Math.max(0, Math.floor((nowTick.value - start) / 1000))
  const h = Math.floor(sec / 3600)
  sec %= 3600
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

function markWorkFlash(id?: number | null) {
  if (!id) return
  flashWorkIds[String(id)] = Date.now() + 4500
}

function clearFieldErrors(kind: 'image' | 'video') {
  const bag = kind === 'image' ? imageFieldErrors : videoFieldErrors
  for (const k of Object.keys(bag)) delete bag[k]
}

function setFieldError(kind: 'image' | 'video', field: string, msg: string) {
  const bag = kind === 'image' ? imageFieldErrors : videoFieldErrors
  bag[field] = msg
}

function validateImageFormFields(): boolean {
  clearFieldErrors('image')
  let ok = true
  if (!selectedKeyId.value) {
    imageError.value = t('creazyCanvas.errors.selectKey')
    return false
  }
  if (!imageForm.prompt.trim()) {
    setFieldError('image', 'prompt', t('creazyCanvas.errors.promptRequired'))
    ok = false
  }
  if (!imageForm.model || !selectedImageModel.value) {
    setFieldError('image', 'model', t('creazyCanvas.errors.selectModel'))
    ok = false
  }
  imageForm.size = canonicalizeImageSizeInput(imageForm.size)
  const sizeErr = describeImageSizeInvalid(imageForm.size)
  if (sizeErr) {
    setFieldError('image', 'size', sizeErr)
    ok = false
  }
  if (imageRefRequired.value && imageRefs.value.length === 0) {
    setFieldError('image', 'refs', t('creazyCanvas.form.imageRefsRequired'))
    ok = false
  }
  if (imageRefs.value.length > imageRefMax.value) {
    setFieldError('image', 'refs', t('creazyCanvas.form.imagesExceeded', { max: imageRefMax.value }))
    ok = false
  }
  if (imageBalanceBlocked.value) {
    imageError.value = imageBalanceHint.value || t('creazyCanvas.errors.insufficientBalance')
    ok = false
  }
  if (!ok && !imageError.value) {
    imageError.value = t('creazyCanvas.form.validationFailed')
  }
  return ok
}

function validateVideoFormFields(): boolean {
  clearFieldErrors('video')
  let ok = true
  if (!selectedKeyId.value) {
    videoError.value = t('creazyCanvas.errors.selectKey')
    return false
  }
  if (!videoForm.prompt.trim()) {
    setFieldError('video', 'prompt', t('creazyCanvas.errors.promptRequired'))
    ok = false
  }
  if (!videoForm.model || !selectedVideoModel.value) {
    setFieldError('video', 'model', t('creazyCanvas.errors.selectModel'))
    ok = false
  }
  if (mediaCaps.value.requireStartFrame && !startFrame.value?.media_url && !startFrameUrlInput.value.trim()) {
    setFieldError('video', 'startFrame', t('creazyCanvas.form.startFrameRequired'))
    ok = false
  }
  if (endFrame.value?.media_url && !startFrame.value?.media_url && !startFrameUrlInput.value.trim()) {
    setFieldError('video', 'endFrame', t('creazyCanvas.form.endNeedsStart'))
    ok = false
  }
  if (videoBalanceBlocked.value) {
    videoError.value = videoBalanceHint.value || t('creazyCanvas.errors.insufficientBalance')
    ok = false
  }
  if (!ok && !videoError.value) {
    videoError.value = t('creazyCanvas.form.validationFailed')
  }
  return ok
}

async function copyTextToClipboard(value: string): Promise<boolean> {
  const textValue = String(value || '')
  if (!textValue) return false
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(textValue)
      return true
    }
  } catch {
    /* fallback below */
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = textValue
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

async function copyWorkError(work: CreazyWork) {
  const msg = workErrorText(work)
  if (!msg) return
  const ok = await copyTextToClipboard(msg)
  actionNotice.value = ok ? t('creazyCanvas.tasks.copied') : msg
  if (ok) appStore.showSuccess(t('creazyCanvas.tasks.copied'))
}

async function copyWorkPrompt(work: CreazyWork) {
  const prompt = String(work.prompt || '')
  if (!prompt) return
  const ok = await copyTextToClipboard(prompt)
  actionNotice.value = ok ? t('creazyCanvas.form.copyPromptOk') : ''
  if (ok) appStore.showSuccess(t('creazyCanvas.form.copyPromptOk'))
}

function toggleErrorExpand(work: CreazyWork) {
  const id = String(work.id || '')
  if (!id) return
  expandedErrorIds[id] = !expandedErrorIds[id]
}

function isErrorExpanded(work: CreazyWork) {
  return Boolean(expandedErrorIds[String(work.id || '')])
}

function stopLocalTrack(work: CreazyWork) {
  const id = Number(work.id || 0)
  if (!id) return
  stoppedTrackIds[String(id)] = true
  clearPoll()
  if (activeVideoWorkId.value === id) activeVideoWorkId.value = null
  actionNotice.value = t('creazyCanvas.tasks.stoppedLocal')
  appStore.showInfo(t('creazyCanvas.tasks.stoppedLocal'))
}

async function retryWork(work: CreazyWork) {
  await reuseWork(work)
  if ((work.kind || '').toLowerCase() === 'video') {
    await generateVideo()
  } else {
    await generateImage()
  }
}

function focusWorkCard(workOrId: CreazyWork | number) {
  const id = typeof workOrId === 'number' ? workOrId : Number(workOrId.id || 0)
  if (!id) return
  focusWorkId.value = id
  const kind = typeof workOrId === 'number' ? '' : String(workOrId.kind || '').toLowerCase()
  if (kind === 'video') switchTab('video')
  else if (kind === 'image') switchTab('image')
  else if (activeTab.value === 'works') {
    /* stay */
  } else if (kind) {
    switchTab(kind === 'video' ? 'video' : 'image')
  }
  requestAnimationFrame(() => {
    const el = document.querySelector(`[data-work-id="${id}"]`) as HTMLElement | null
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  })
  window.setTimeout(() => {
    if (focusWorkId.value === id) focusWorkId.value = null
  }, 3500)
}

function onWorkSelectChange(work: CreazyWork, ev: Event) {
  const target = ev.target as HTMLInputElement | null
  toggleSelectWork(work, Boolean(target?.checked))
}

function toggleSelectWork(work: CreazyWork, checked?: boolean) {
  const id = Number(work.id || 0)
  if (!id) return
  const set = new Set(selectedWorkIds.value)
  const on = checked == null ? !set.has(id) : Boolean(checked)
  if (on) set.add(id)
  else set.delete(id)
  selectedWorkIds.value = Array.from(set)
}

function selectAllWorksOnPage(on = true) {
  if (!on) {
    selectedWorkIds.value = []
    return
  }
  selectedWorkIds.value = filteredWorks.value.map((w) => Number(w.id || 0)).filter(Boolean)
}

async function batchDeleteSelectedWorks() {
  const ids = selectedWorkIds.value.slice()
  if (!ids.length) return
  const ok = window.confirm(t('creazyCanvas.works.batchDeleteConfirm', { n: ids.length }))
  if (!ok) return
  for (const id of ids) {
    const work = works.value.find((w) => Number(w.id) === id)
    if (work) await removeWork(work)
  }
  selectedWorkIds.value = []
  await loadWorks()
}

function jumpWorksPageFromInput() {
  const raw = worksPageJumpInput.value.trim()
  const n = Number(raw)
  if (!Number.isFinite(n)) return
  void goWorksPage(n)
}

function onWorksPageSizeChange() {
  worksPageSizeChoice.value = Math.max(1, Number(worksPageSizeChoice.value) || 12)
  resetWorksPage()
  void loadWorks({ page: 1 })
}

function scrollWorksBoardTop() {
  const el = document.querySelector('.cc-board-card, .cc-works-root, [data-cc-board]') as HTMLElement | null
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  else window.scrollTo({ top: 0, behavior: 'smooth' })
}

function clearImageForm(confirm = true) {
  if (confirm && !window.confirm(t('creazyCanvas.form.clearFormConfirm'))) return
  imageForm.prompt = ''
  imageForm.size = imageSizeOptions.value[0] || '1024x1024'
  for (const item of imageRefs.value) revokePreviewUrl(item)
  imageRefs.value = []
  clearFieldErrors('image')
  imageError.value = ''
  imageSaveMessage.value = t('creazyCanvas.form.formCleared')
  clearCanvasDraft()
  draftSavedAt.value = null
  scheduleDraftSave()
}

function clearVideoForm(confirm = true) {
  if (confirm && !window.confirm(t('creazyCanvas.form.clearFormConfirm'))) return
  videoForm.prompt = ''
  resetVideoMedia()
  clearFieldErrors('video')
  videoError.value = ''
  videoSaveMessage.value = t('creazyCanvas.form.formCleared')
  clearCanvasDraft()
  draftSavedAt.value = null
  scheduleDraftSave()
}

function mediaProgressText(used: number, max: number) {
  if (!max || max <= 0) return ''
  return t('creazyCanvas.form.mediaLimitProgress', { used, max })
}

function onMediaDragStart(key: string, index: number) {
  dragMediaKey.value = key
  dragMediaIndex.value = index
}

function onMediaDropReorder(key: string, index: number) {
  if (dragMediaKey.value !== key || dragMediaIndex.value < 0 || dragMediaIndex.value === index) {
    dragMediaKey.value = ''
    dragMediaIndex.value = -1
    return
  }
  const list =
    key === 'imageRefs'
      ? imageRefs
      : key === 'refImages'
        ? refImages
        : key === 'refVideos'
          ? refVideos
          : key === 'refAudios'
            ? refAudios
            : null
  if (!list) return
  const arr = list.value.slice()
  const from = dragMediaIndex.value
  if (from < 0 || from >= arr.length || index < 0 || index >= arr.length) {
    dragMediaKey.value = ''
    dragMediaIndex.value = -1
    return
  }
  const [item] = arr.splice(from, 1)
  arr.splice(index, 0, item)
  list.value = arr
  dragMediaKey.value = ''
  dragMediaIndex.value = -1
}

function onPasteMedia(ev: ClipboardEvent, target: 'imageRefs' | 'refImages' | 'startFrame' | 'endFrame') {
  const files = Array.from(ev.clipboardData?.files || [])
  if (!files.length) return
  ev.preventDefault()
  if (target === 'imageRefs') void onPickImageRefs({ target: { files } } as any)
  else if (target === 'refImages') void onPickRefImage({ target: { files } } as any)
  else if (target === 'startFrame') void onPickStartFrame({ target: { files } } as any)
  else if (target === 'endFrame') void onPickEndFrame({ target: { files } } as any)
}

function onDropMedia(ev: DragEvent, target: 'imageRefs' | 'refImages' | 'refVideos' | 'refAudios' | 'startFrame' | 'endFrame') {
  ev.preventDefault()
  const files = Array.from(ev.dataTransfer?.files || [])
  if (!files.length) return
  if (target === 'imageRefs') void onPickImageRefs({ target: { files } } as any)
  else if (target === 'refImages') void onPickRefImage({ target: { files } } as any)
  else if (target === 'refVideos') void onPickRefVideo({ target: { files } } as any)
  else if (target === 'refAudios') void onPickRefAudio({ target: { files } } as any)
  else if (target === 'startFrame') void onPickStartFrame({ target: { files } } as any)
  else if (target === 'endFrame') void onPickEndFrame({ target: { files } } as any)
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
  let lastError: any = null
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
          lastError = error
          console.warn('[creazy-canvas] session content preview failed', error)
        }
      }
    } catch (error) {
      lastError = error
      // ignore download-url fallback, try content stream below
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
        lastError = error
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
          lastError = error
          console.warn('[creazy-canvas] video preview content failed', error)
        }
      }
    }
    if (url && isImageWork(work) && !needsAuthForMediaPlayback(url)) {
      workPreviewUrls[id] = url
      return true
    }
    if (lastError) {
      ;(work as any).__previewError = mapContentPreviewError(lastError)
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
  ;(work as any).__previewError = ''
  const ok = await loadWorkPreview(work)
  const url = workPreviewUrl(work) || workCoverUrl(work)
  if (!url) {
    const detail =
      String((work as any).__previewError || '').trim() ||
      (work.error_message ? String(work.error_message) : '') ||
      t('creazyCanvas.works.previewFailedGeneric')
    appStore.showError(detail)
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

function clearImageRefs() {
  for (const item of imageRefs.value) revokePreviewUrl(item)
  imageRefs.value = []
}

function openImageRefPicker() {
  if (!imageRefSupported.value || !!uploadingImageRef.value) return
  if (imageRefs.value.length >= imageRefMax.value) return
  imageRefInput.value?.click()
}

function removeImageRef(idx: number) {
  const [item] = imageRefs.value.splice(idx, 1)
  if (item) revokePreviewUrl(item)
}

async function onPickImageRefs(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  if (!imageRefSupported.value) {
    imageError.value = t('creazyCanvas.form.imageRefsUnsupported')
    return
  }
  const room = Math.max(0, imageRefMax.value - imageRefs.value.length)
  if (room <= 0) {
    imageError.value = t('creazyCanvas.form.imagesExceeded', { max: imageRefMax.value })
    return
  }
  const batch = files.slice(0, room)
  uploadingImageRef.value = true
  imageRefUploadLabel.value = t('creazyCanvas.form.uploadingProgress', { done: 0, total: batch.length })
  imageError.value = ''
  try {
    for (let i = 0; i < batch.length; i++) {
      const file = batch[i]
      const media_url = await uploadPickedFile(file, 'image')
      const preview_url = URL.createObjectURL(file)
      imageRefs.value.push({ name: file.name, media_url, preview_url })
      imageRefUploadLabel.value = t('creazyCanvas.form.uploadingProgress', {
        done: i + 1,
        total: batch.length,
      })
    }
    if (files.length > room) {
      imageError.value = t('creazyCanvas.form.imagesExceeded', { max: imageRefMax.value })
    }
  } catch (error: any) {
    imageError.value = mapGatewayError(error)
  } finally {
    uploadingImageRef.value = false
    imageRefUploadLabel.value = ''
  }
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
  clearImageRefs()
  resetVideoMedia()
  works.value = []
  worksTotal.value = 0
  worksPages.value = 1
  resetWorksPage()
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
  return ['succeeded', 'completed', 'success', 'finished', 'done', 'complete', 'failed', 'error', 'cancelled', 'canceled'].includes(s)
}

function isFailedVideoStatus(status?: string) {
  const s = (status || '').toLowerCase()
  return ['failed', 'error', 'cancelled', 'canceled'].includes(s)
}

function isSucceededVideoStatus(status?: string) {
  return isTerminalVideoStatus(status) && !isFailedVideoStatus(status)
}

/** Normalize gateway/work status labels for UI consistency across providers. */
function normalizeGatewayVideoStatus(status?: string): string {
  const s = String(status || '').toLowerCase().trim()
  if (!s) return ''
  if (['pending', 'queued', 'submitted', 'created'].includes(s)) return 'queued'
  if (['running', 'processing', 'settling', 'in_progress', 'inprogress', 'generating', 'working'].includes(s)) return 'running'
  if (['completed', 'succeeded', 'success', 'finished', 'done', 'complete'].includes(s)) return 'completed'
  if (['failed', 'error'].includes(s)) return 'failed'
  if (['canceled', 'cancelled'].includes(s)) return 'cancelled'
  return s
}

async function generateImage() {
  imageError.value = ''
  imageSaveMessage.value = ''
  if (!validateImageFormFields()) return

  submittingImage.value = true
  const snapshot = {
    model: imageForm.model,
    prompt: imageForm.prompt.trim(),
    size: imageForm.size,
    preferAsync: Boolean(selectedImageModel.value?.async),
    keyId: selectedKeyId.value,
    refs: imageRefs.value.map((item) => item.media_url).filter(Boolean),
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
      params: buildImageWorkParams({ size: snapshot.size, refs: snapshot.refs }),
      gateway_type: snapshot.preferAsync ? 'image_task' : 'image_sync',
    })
    if (running?.id) {
      runningWorkId = running.id
      markWorkFlash(running.id)
      focusWorkId.value = running.id
    }
    resetWorksPage()
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
          params: buildImageWorkParams({ size: snapshot.size, refs: snapshot.refs }),
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
  snapshot: { model: string; prompt: string; size: string; preferAsync: boolean; keyId: number; refs: string[] }
  runningWorkId: number | null
}) {
  const { apiKey, snapshot } = opts
  let runningWorkId = opts.runningWorkId
  let failedPersisted = false
  let lastGatewayType: 'image_task' | 'image_sync' | '' = ''
  let lastTaskId = ''
  try {
    const useEdit = snapshot.refs.length > 0
    const imagePayload: ImageGenerationRequest = {
      model: snapshot.model,
      prompt: snapshot.prompt,
      size: snapshot.size,
      n: 1,
    }
    if (useEdit) {
      imagePayload.images = snapshot.refs.map((url) => ({ image_url: url }))
      // Grok-compatible alias
      imagePayload.reference_images = snapshot.refs.map((url) => ({ image_url: url }))
    }
    let usedAsync = snapshot.preferAsync
    let response: any
    try {
      response = await gatewayGenerateImage(apiKey, imagePayload, { async: snapshot.preferAsync, edit: useEdit })
    } catch (asyncErr: any) {
      const status = Number(asyncErr?.status || 0)
      const msg = String(asyncErr?.message || asyncErr?.code || '')
      const asyncUnavailable =
        snapshot.preferAsync &&
        (status === 404 ||
          /async.*not enabled|not_found_error|model not found|unknown model/i.test(msg))
      if (!asyncUnavailable) throw asyncErr
      usedAsync = false
      response = await gatewayGenerateImage(apiKey, imagePayload, { async: false, edit: useEdit })
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
          params: buildImageWorkParams({ size: snapshot.size, refs: snapshot.refs }),
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
      params: buildImageWorkParams({ size: snapshot.size, refs: snapshot.refs, resultUrls: cleanUrls }),
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
          params: buildImageWorkParams({ size: snapshot.size, refs: snapshot.refs }),
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
    job?.download_url ||
    job?.content?.video_url ||
    job?.content?.download_url ||
    job?.content?.url ||
    job?.url ||
    job?.result?.url ||
    job?.result?.content_url ||
    job?.result?.video_url ||
    job?.result?.download_url ||
    job?.result?.data?.[0]?.url ||
    job?.result?.data?.[0]?.mp4_url ||
    job?.result?.data?.[0]?.local_url ||
    job?.result?.data?.[0]?.download_url ||
    ''
  )
}

async function generateVideo() {
  videoError.value = ''
  videoSaveMessage.value = ''
  if (!validateVideoFormFields()) return

  try {
    validateVideoMediaBeforeSubmit()
  } catch (error: any) {
    const msg = mapGatewayError(error)
    videoError.value = msg
    const low = msg.toLowerCase()
    if (msg.includes('首帧') || low.includes('start frame') || low.includes('start_frame')) {
      setFieldError('video', 'startFrame', msg)
    } else if (msg.includes('尾帧') || low.includes('end frame') || low.includes('end_frame')) {
      setFieldError('video', 'endFrame', msg)
    } else if (msg.includes('参考图') || low.includes('image')) {
      setFieldError('video', 'refs', msg)
    } else if (msg.includes('音频') || low.includes('audio')) {
      setFieldError('video', 'audio', msg)
    }
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
    endFrameUrl:
      endFrame.value?.media_url && !isBlobUrl(endFrame.value.media_url)
        ? endFrame.value.media_url
        : '',
    refImageUrls: refImages.value.map((x) => x.media_url).filter((u) => u && !isBlobUrl(u)),
    refVideoUrls: refVideos.value.map((x) => x.media_url).filter((u) => u && !isBlobUrl(u)),
    refAudioUrls: refAudios.value.map((x) => x.media_url).filter((u) => u && !isBlobUrl(u)),
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

    const baseParams = buildVideoWorkParams({
      resolution: snapshot.resolution,
      duration: snapshot.duration,
      aspectRatio: snapshot.aspectRatio,
      generateAudio: snapshot.generateAudio,
      startFrame: snapshot.startFrameUrl,
      endFrame: (snapshot as any).endFrameUrl,
      refImages: (snapshot as any).refImageUrls || [],
      refVideos: (snapshot as any).refVideoUrls || [],
      refAudios: (snapshot as any).refAudioUrls || [],
    })

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
        markWorkFlash(running.id)
        focusWorkId.value = running.id
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
      if (running?.id) {
        runningWorkId = running.id
        markWorkFlash(running.id)
        focusWorkId.value = running.id
      }
    }

    resetWorksPage()
    void loadWorks({ quiet: true })
    submittingVideo.value = false
    videoSaveMessage.value = t('creazyCanvas.tasks.submitted')
    activeVideoJobs.value += 1
    generatingVideo.value = activeVideoJobs.value > 0

    if (runningWorkId) resumingVideoWorkIds.add(runningWorkId)
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
          params: buildVideoWorkParams({
            resolution: snapshot.resolution,
            duration: snapshot.duration,
            aspectRatio: snapshot.aspectRatio,
            generateAudio: snapshot.generateAudio,
            startFrame: snapshot.startFrameUrl,
            endFrame: (snapshot as any).endFrameUrl,
            refImages: (snapshot as any).refImageUrls || [],
            refVideos: (snapshot as any).refVideoUrls || [],
            refAudios: (snapshot as any).refAudioUrls || [],
          }),
          gateway_type: 'video_job',
          error_message: msg,
        })
      }
      void loadWorks({ quiet: true })
    }
    submittingVideo.value = false
  }
}


async function resumeOrphanedVideoWorks() {
  if (cancelled) return
  const candidates = works.value.filter((w) => {
    if ((w.kind || '').toLowerCase() !== 'video') return false
    if (!isActiveWorkStatus(w.status)) return false
    if (!w.gateway_remote_id) return false
    if (!w.id || resumingVideoWorkIds.has(w.id)) return false
    return true
  })
  for (const work of candidates.slice(0, 5)) {
    const workId = Number(work.id)
    const jobId = String(work.gateway_remote_id || '')
    const keyId = Number(work.api_key_id || 0)
    if (!workId || !jobId || !keyId) continue
    resumingVideoWorkIds.add(workId)
    void (async () => {
      try {
        await ensureKeySecret(keyId)
        const apiKey = resolveApiKeySecret(keyId)
        if (!apiKey) return
        let job: any = null
        for (let i = 0; i < 120 && !cancelled; i++) {
          job = await getVideoJob(apiKey, jobId)
          const st = normalizeGatewayVideoStatus(job?.status) || String(job?.status || '').toLowerCase()
          if (isTerminalVideoStatus(st)) break
          if (i % 4 === 3) {
            await updateWorkRecord(workId, {
              status: st === 'queued' || st === 'created' || st === 'submitted' ? 'queued' : 'running',
              gateway_remote_id: jobId,
            })
            void loadWorks({ quiet: true })
          }
          await sleepMs(3000)
        }
        if (!job) return
        const st = normalizeGatewayVideoStatus(job.status) || String(job.status || '').toLowerCase()
        if (isSucceededVideoStatus(st)) {
          const contentPath = '/v1/videos/jobs/' + jobId + '/content'
          const extracted = extractVideoUrl(job) || contentPath
          const storedUrl =
            (extracted && !isBlobUrl(extracted) && !needsAuthForMediaPlayback(extracted) ? extracted : '') ||
            contentPath
          await updateWorkRecord(workId, {
            status: 'succeeded',
            gateway_type: 'video_job',
            gateway_remote_id: jobId,
            object_url: storedUrl,
            preview_url: storedUrl,
            mime_type: 'video/mp4',
            error_message: '',
            params: {
              ...(work.params || {}),
              result_urls: [storedUrl],
            },
          })
          // Flip board status immediately; preview download can take long for H3.
          void loadWorks({ quiet: true })
          // best-effort preview (non-blocking)
          void (async () => {
            try {
              const resolved = await resolvePlayableVideoUrl(apiKey, jobId, extracted)
              if (resolved.playable) {
                if (isBlobUrl(resolved.playable)) workPreviewBlobUrls.add(resolved.playable)
                workPreviewUrls[String(workId)] = resolved.playable
              }
            } catch {
              /* ignore */
            }
          })()
          return
        }
        if (isFailedVideoStatus(st)) {
          const err =
            typeof job.error === 'string'
              ? job.error
              : job.error && typeof job.error === 'object'
                ? job.error.message
                : ''
          await updateWorkRecord(workId, {
            status: 'failed',
            gateway_remote_id: jobId,
            error_message: mapGatewayError(
              { message: err || '', status: 0 },
              err || t('creazyCanvas.result.failed') + ': ' + (job.status || 'unknown'),
            ),
          })
          void loadWorks({ quiet: true })
        }
      } catch (error) {
        console.warn('[creazy-canvas] resume video work failed', workId, error)
      } finally {
        resumingVideoWorkIds.delete(workId)
      }
    })()
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
  const baseParams = buildVideoWorkParams({
    resolution: snapshot.resolution,
    duration: snapshot.duration,
    aspectRatio: snapshot.aspectRatio,
    generateAudio: snapshot.generateAudio,
    startFrame: snapshot.startFrameUrl,
    endFrame: (snapshot as any).endFrameUrl,
    refImages: (snapshot as any).refImageUrls || [],
    refVideos: (snapshot as any).refVideoUrls || [],
    refAudios: (snapshot as any).refAudioUrls || [],
  })
  try {
    if (jobId) {
      for (let i = 0; i < 150 && !cancelled; i++) {
        const normalized = normalizeGatewayVideoStatus(job.status)
        if (isTerminalVideoStatus(normalized || job.status)) break
        await sleepMs(3000)
        job = await getVideoJob(apiKey, jobId)
        const stRaw = String(job.status || '')
        const st = normalizeGatewayVideoStatus(stRaw) || stRaw.toLowerCase()
        if (selectedKeyId.value === snapshot.keyId) {
          videoStatus.value = st || videoStatus.value
        }
        if (runningWorkId && isActiveWorkStatus(st) && !isTerminalVideoStatus(st)) {
          if (i % 5 === 4) {
            await updateWorkRecord(runningWorkId, {
              status: st === 'queued' || st === 'created' || st === 'submitted' ? 'queued' : 'running',
              gateway_remote_id: jobId,
            })
          }
        }
        if (i % 2 === 1) void loadWorks({ quiet: true })
      }
    }

    const terminalStatus = normalizeGatewayVideoStatus(job.status) || String(job.status || '').toLowerCase()
    const failedStatus = isFailedVideoStatus(terminalStatus)
    const succeededStatus = isSucceededVideoStatus(terminalStatus)
    const timedOut = Boolean(jobId && !isTerminalVideoStatus(terminalStatus) && !failedStatus)
    const contentPath = jobId ? '/v1/videos/jobs/' + jobId + '/content' : ''
    const extracted = extractVideoUrl(job) || (succeededStatus ? contentPath : '')
    const posterUrl = snapshot.startFrameUrl || ''

    // Mark work terminal ASAP — do not wait for full MP4 blob download (H3 1440p can be large).
    if (succeededStatus) {
      const storedUrl =
        (extracted && !isBlobUrl(extracted) && !needsAuthForMediaPlayback(extracted) ? extracted : '') ||
        contentPath ||
        extracted
      const successPayload = {
        status: 'succeeded' as const,
        public_model: snapshot.model,
        prompt: snapshot.prompt,
        params: {
          ...baseParams,
          result_urls: storedUrl ? [storedUrl] : [],
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
      if (selectedKeyId.value === snapshot.keyId) {
        videoStatus.value = 'completed'
        videoSaveMessage.value = t('creazyCanvas.result.autoSaved')
      }
      // Prefer poster immediately so task board leaves "running" without waiting for MP4 blob.
      if (saved?.id && posterUrl && !needsAuthForMediaPlayback(posterUrl) && !workPreviewUrls[String(saved.id)]) {
        workPreviewUrls[String(saved.id)] = posterUrl
      }
      void loadWorks({ quiet: true })

      // Resolve playable preview after status flip (best-effort, non-blocking).
      // H3 1440p content download can take a long time; do not keep work "active" for it.
      const savedId = saved?.id
      void (async () => {
        let playable = ''
        let persist = storedUrl
        try {
          const resolved = await resolvePlayableVideoUrl(apiKey, jobId, extracted)
          playable = resolved.playable
          persist = resolved.persist || storedUrl
        } catch {
          playable = extracted && !needsAuthForMediaPlayback(extracted) ? extracted : ''
        }
        if (selectedKeyId.value === snapshot.keyId && (playable || persist)) {
          setVideoResultPlayback(playable || persist, persist)
        }
        if (savedId) {
          const playableUrl =
            selectedKeyId.value === snapshot.keyId ? videoResultUrl.value : playable || persist
          if (playableUrl) {
            if (isBlobUrl(playableUrl)) workPreviewBlobUrls.add(playableUrl)
            workPreviewUrls[String(savedId)] = playableUrl
          } else if (posterUrl && !needsAuthForMediaPlayback(posterUrl)) {
            workPreviewUrls[String(savedId)] = posterUrl
          }
        }
      })()
      return
    }

    if (!extracted && !contentPath) {
      if (timedOut && jobId) {
        if (selectedKeyId.value === snapshot.keyId) {
          videoStatus.value = terminalStatus || 'running'
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

    // Failed terminal (or unknown non-timeout) path
    if (failedStatus || !timedOut) {
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

    // Still running after poll budget
    if (timedOut && jobId) {
      if (selectedKeyId.value === snapshot.keyId) {
        videoStatus.value = terminalStatus || 'running'
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
    if (runningWorkId) resumingVideoWorkIds.delete(runningWorkId)
    activeVideoJobs.value = Math.max(0, activeVideoJobs.value - 1)
    generatingVideo.value = activeVideoJobs.value > 0
    if (selectedKeyId.value === snapshot.keyId) {
      activeVideoWorkId.value = null
    }
  }
}



function taskBoardPageSize() {
  if (activeTab.value === 'works') {
    const n = Number(worksPageSizeChoice.value) || 12
    return Math.min(48, Math.max(6, n))
  }
  return 10
}

function resolveWorksListKind(): string | undefined {
  if (activeTab.value === 'image') return 'image'
  if (activeTab.value === 'video') return 'video'
  const k = String(worksFilterKind.value || '').trim()
  return k || undefined
}

function resetWorksPage() {
  worksPage.value = 1
}

async function goWorksPage(page: number) {
  const maxPages = Math.max(1, Number(worksPages.value) || 1)
  const raw = Number(page)
  const next = Math.max(1, Math.min(Number.isFinite(raw) ? Math.trunc(raw) : 1, maxPages))
  // Stop scheduled quiet polls so they cannot race a user page change.
  clearWorksPoll()
  worksPage.value = next
  worksPageJumpInput.value = String(next)
  await loadWorks({ page: next })
  scrollWorksBoardTop()
}

function goToWorksPrevPage(ev?: Event) {
  if (ev) {
    ev.preventDefault()
    ev.stopPropagation()
  }
  void goWorksPage(Number(worksPage.value || 1) - 1)
}

function goToWorksNextPage(ev?: Event) {
  if (ev) {
    ev.preventDefault()
    ev.stopPropagation()
  }
  void goWorksPage(Number(worksPage.value || 1) + 1)
}

async function reloadWorksFromStart(opts?: { quiet?: boolean }) {
  resetWorksPage()
  await loadWorks({ ...opts, page: 1 })
}

async function loadWorks(opts?: { quiet?: boolean; page?: number }) {
  if (!selectedKeyId.value) {
    works.value = []
    worksTotal.value = 0
    worksPages.value = 1
    worksPage.value = 1
    loadingWorks.value = false
    clearWorksPoll()
    return
  }

  if (opts?.page != null && Number.isFinite(Number(opts.page))) {
    worksPage.value = Math.max(1, Math.trunc(Number(opts.page)))
  }

  const requestedPage = Math.max(1, Number(worksPage.value) || 1)
  const requestedKeyId = Number(selectedKeyId.value)
  const requestedKind = resolveWorksListKind()
  const requestedStatus = activeTab.value === 'works' ? (worksFilterStatus.value || undefined) : undefined
  const pageSize = taskBoardPageSize()
  worksPageSize.value = pageSize

  const seq = ++worksLoadSeq
  const showLoading = !opts?.quiet
  let loadingToken = 0
  if (showLoading) {
    loadingToken = ++worksLoadingSeq
    loadingWorks.value = true
  }

  try {
    const res = await listWorks({
      page: requestedPage,
      page_size: pageSize,
      api_key_id: requestedKeyId,
      kind: requestedKind,
      status: requestedStatus,
    })
    // Drop stale responses: a newer load/poll/page-change already started.
    if (seq !== worksLoadSeq) return
    if (Number(selectedKeyId.value) !== requestedKeyId) return
    // If the user flipped pages after this request started, ignore this payload.
    if (Number(worksPage.value) !== requestedPage) return

    // Defense-in-depth: only keep works for the currently selected key.
    const keyId = Number(selectedKeyId.value)
    let items = (res.items || []).filter((w) => Number(w.api_key_id) === keyId)
    // Keep active tasks first within the current page for task boards.
    if (activeTab.value !== 'works') {
      items = sortTaskWorks(items)
    }
    works.value = items
    worksTotal.value = Number(res.total || 0)
    // Keep the page the user asked for; do not let a mismatched server page snap back.
    worksPage.value = requestedPage
    worksPageSize.value = Number(res.page_size || pageSize)
    const pagesFromRes = Number(res.pages || 0)
    worksPages.value =
      pagesFromRes > 0
        ? pagesFromRes
        : Math.max(1, Math.ceil((worksTotal.value || 0) / Math.max(1, worksPageSize.value)))

    // Beyond last page (deletes / filter) -> clamp and reload.
    if (worksPage.value > worksPages.value && worksPages.value >= 1) {
      worksPage.value = worksPages.value
      await loadWorks({ ...opts, page: worksPage.value })
      return
    }
    // Empty page after deletes / filter changes, step back.
    if (works.value.length === 0 && worksPage.value > 1 && worksTotal.value > 0) {
      worksPage.value = Math.min(worksPage.value - 1, worksPages.value)
      await loadWorks({ ...opts, page: worksPage.value })
      return
    }
    void hydrateWorkPreviews(works.value)
    // Only the latest successful load should (re)schedule poll, avoiding overlapping quiet loads.
    if (seq === worksLoadSeq) scheduleWorksPoll()
  } catch (error: any) {
    if (seq !== worksLoadSeq) return
    if (!opts?.quiet) {
      appStore.showError(error?.response?.data?.detail || error?.message || t('creazyCanvas.works.loadFailed'))
    }
  } finally {
    // Clear spinner even if a quiet poll supersedes this user load.
    if (showLoading && loadingToken === worksLoadingSeq) {
      loadingWorks.value = false
    }
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
    worksTotal.value = Math.max(0, Number(worksTotal.value || 0) - 1)
    appStore.showSuccess(t('creazyCanvas.works.deleted'))
    if (!works.value.length && worksPage.value > 1) {
      await goWorksPage(worksPage.value - 1)
    } else {
      void loadWorks({ quiet: true })
    }
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
  const notes: string[] = []
  let partial = false

  if (selectedKeyId.value) {
    await loadCatalog()
  }

  if (kind === 'video') {
    videoError.value = ''
    videoSaveMessage.value = ''
    videoForm.prompt = work.prompt || ''

    const wantedModel = String(work.public_model || '').trim()
    if (wantedModel && videoModels.value.some((m) => m.id === wantedModel)) {
      videoForm.model = wantedModel
    } else if (wantedModel) {
      notes.push(t('creazyCanvas.form.reuseModelMissing', { model: wantedModel }))
      partial = true
    }
    // Ensure model/options are synced before applying size-like params
    syncFormModelsFromCatalog()

    if (params.resolution != null && String(params.resolution).trim()) {
      const resolution = String(params.resolution)
      if (videoResolutionOptions.value.includes(resolution)) {
        videoForm.resolution = resolution
      } else {
        partial = true
      }
    }
    if (params.duration != null && String(params.duration).trim() !== '') {
      const duration = Number(params.duration)
      if (videoDurationOptions.value.includes(duration)) {
        videoForm.duration = duration
      } else {
        partial = true
      }
    }
    if (params.aspect_ratio != null && String(params.aspect_ratio).trim()) {
      const aspect = String(params.aspect_ratio)
      if (videoAspectOptions.value.includes(aspect)) {
        videoForm.aspectRatio = aspect
      } else {
        partial = true
      }
    }
    videoForm.generateAudio = Boolean(params.generate_audio) && mediaCaps.value.allowGeneratedAudio

    // Restore reusable media (absolute/http only; skip blob/data)
    let mediaRestored = 0
    let mediaSkipped = 0
    const legacyKeys = [
      'start_frame',
      'start_frame_url',
      'end_frame',
      'end_frame_url',
      'ref_images',
      'ref_videos',
      'ref_audios',
      'image_refs',
    ]
    const hadAnyMediaKey = legacyKeys.some((k) => params[k] != null)

    const sf = pickStringParam(params, 'start_frame', 'start_frame_url', 'first_frame')
    if (sf) {
      if (isReusableMediaUrl(sf)) {
        startFrame.value = { name: 'start-frame', media_url: sf, preview_url: sf }
        startFrameUrlInput.value = sf
        mediaRestored++
      } else {
        mediaSkipped++
      }
    }
    const ef = pickStringParam(params, 'end_frame', 'end_frame_url', 'last_frame')
    if (ef) {
      if (isReusableMediaUrl(ef)) {
        endFrame.value = { name: 'end-frame', media_url: ef, preview_url: ef }
        mediaRestored++
      } else {
        mediaSkipped++
      }
    }
    const rImgs = pickStringListParam(params, 'ref_images', 'reference_images')
    if (rImgs.length) {
      refImages.value = rImgs.map((u, i) => ({ name: `ref-image-${i + 1}`, media_url: u, preview_url: u }))
      mediaRestored += rImgs.length
    }
    const rVids = pickStringListParam(params, 'ref_videos', 'reference_videos')
    if (rVids.length) {
      refVideos.value = rVids.map((u, i) => ({ name: `ref-video-${i + 1}`, media_url: u }))
      mediaRestored += rVids.length
    }
    const rAuds = pickStringListParam(params, 'ref_audios', 'reference_audios')
    if (rAuds.length) {
      refAudios.value = rAuds.map((u, i) => ({ name: `ref-audio-${i + 1}`, media_url: u }))
      mediaRestored += rAuds.length
    }
    if (mediaSkipped) {
      notes.push(t('creazyCanvas.form.reuseMediaSkipped'))
      partial = true
    } else if (!hadAnyMediaKey && !mediaRestored) {
      notes.push(t('creazyCanvas.form.reuseMediaMissingLegacy'))
      partial = true
    }

    switchTab('video')

    if (notes.length) {
      videoError.value = notes.join('；')
      videoSaveMessage.value = t('creazyCanvas.form.reusePartial')
    } else if (partial) {
      videoSaveMessage.value = t('creazyCanvas.form.reusePartial')
    } else {
      videoSaveMessage.value = t('creazyCanvas.form.reuseApplied')
    }
  } else {
    imageError.value = ''
    imageSaveMessage.value = ''
    imageForm.prompt = work.prompt || ''

    const wantedModel = String(work.public_model || '').trim()
    if (wantedModel && imageModels.value.some((m) => m.id === wantedModel)) {
      imageForm.model = wantedModel
    } else if (wantedModel) {
      notes.push(t('creazyCanvas.form.reuseModelMissing', { model: wantedModel }))
      partial = true
    }
    // Model must be set before size validation (depends on selectedImageModel)
    syncFormModelsFromCatalog()

    if (params.size != null && String(params.size).trim()) {
      const size = String(params.size)
      if (imageSizeOptions.value.includes(size) || isValidImageSizeInput(size)) {
        imageForm.size = canonicalizeImageSizeInput(size)
      } else {
        const fallback = imageSizeOptions.value[0] || '1024x1024'
        imageForm.size = fallback
        notes.push(t('creazyCanvas.form.reuseSizeSkipped', { size, fallback }))
        partial = true
      }
    }

    const imgRefs = pickStringListParam(params, 'image_refs', 'ref_images', 'reference_images', 'images')
    if (imgRefs.length) {
      imageRefs.value = imgRefs.map((u, i) => ({ name: `image-ref-${i + 1}`, media_url: u, preview_url: u }))
    } else if (params.reference_count || params.edit) {
      notes.push(t('creazyCanvas.form.reuseMediaMissingLegacy'))
      partial = true
    }

    switchTab('image')

    if (notes.length) {
      imageError.value = notes.join('；')
      imageSaveMessage.value = t('creazyCanvas.form.reusePartial')
    } else if (partial) {
      imageSaveMessage.value = t('creazyCanvas.form.reusePartial')
    } else {
      imageSaveMessage.value = t('creazyCanvas.form.reuseApplied')
    }
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
    resetWorksPage()
    void loadWorks({ quiet: activeTab.value !== 'works' })
  },
)

function collectCanvasDraft(): CanvasDraftV1 {
  return {
    v: 1,
    selectedKeyId: selectedKeyId.value || undefined,
    activeTab: activeTab.value,
    image: {
      prompt: imageForm.prompt,
      model: imageForm.model,
      size: imageForm.size,
      refs: imageRefs.value.map((x) => x.media_url).filter((u) => isReusableMediaUrl(u)),
    },
    video: {
      prompt: videoForm.prompt,
      model: videoForm.model,
      resolution: videoForm.resolution,
      duration: videoForm.duration,
      aspectRatio: videoForm.aspectRatio,
      generateAudio: videoForm.generateAudio,
      startFrame:
        startFrame.value?.media_url && isReusableMediaUrl(startFrame.value.media_url)
          ? startFrame.value.media_url
          : undefined,
      endFrame:
        endFrame.value?.media_url && isReusableMediaUrl(endFrame.value.media_url)
          ? endFrame.value.media_url
          : undefined,
      refImages: refImages.value.map((x) => x.media_url).filter((u) => isReusableMediaUrl(u)),
      refVideos: refVideos.value.map((x) => x.media_url).filter((u) => isReusableMediaUrl(u)),
      refAudios: refAudios.value.map((x) => x.media_url).filter((u) => isReusableMediaUrl(u)),
    },
  }
}

function scheduleDraftSave() {
  if (!draftHydrated) return
  if (draftSaveTimer) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => {
    draftSaveTimer = null
    writeCanvasDraft(collectCanvasDraft())
    draftSavedAt.value = Date.now()
  }, 400)
}

function applyCanvasDraft(draft: CanvasDraftV1) {
  if (draft.selectedKeyId && keys.value.some((k) => k.id === draft.selectedKeyId)) {
    selectedKeyId.value = draft.selectedKeyId
  }
  if (draft.image) {
    if (draft.image.prompt != null) imageForm.prompt = draft.image.prompt
    if (draft.image.model) imageForm.model = draft.image.model
    if (draft.image.size) imageForm.size = draft.image.size
    if (draft.image.refs?.length) {
      imageRefs.value = draft.image.refs
        .filter((u) => isReusableMediaUrl(u))
        .map((u, i) => ({ name: `draft-ref-${i + 1}`, media_url: u, preview_url: u }))
    }
  }
  if (draft.video) {
    if (draft.video.prompt != null) videoForm.prompt = draft.video.prompt
    if (draft.video.model) videoForm.model = draft.video.model
    if (draft.video.resolution) videoForm.resolution = draft.video.resolution
    if (draft.video.duration != null) videoForm.duration = Number(draft.video.duration)
    if (draft.video.aspectRatio) videoForm.aspectRatio = draft.video.aspectRatio
    if (draft.video.generateAudio != null) videoForm.generateAudio = Boolean(draft.video.generateAudio)
    if (draft.video.startFrame && isReusableMediaUrl(draft.video.startFrame)) {
      startFrame.value = {
        name: 'draft-start',
        media_url: draft.video.startFrame,
        preview_url: draft.video.startFrame,
      }
      startFrameUrlInput.value = draft.video.startFrame
    }
    if (draft.video.endFrame && isReusableMediaUrl(draft.video.endFrame)) {
      endFrame.value = {
        name: 'draft-end',
        media_url: draft.video.endFrame,
        preview_url: draft.video.endFrame,
      }
    }
    if (draft.video.refImages?.length) {
      refImages.value = draft.video.refImages
        .filter((u) => isReusableMediaUrl(u))
        .map((u, i) => ({ name: `draft-img-${i + 1}`, media_url: u, preview_url: u }))
    }
    if (draft.video.refVideos?.length) {
      refVideos.value = draft.video.refVideos
        .filter((u) => isReusableMediaUrl(u))
        .map((u, i) => ({ name: `draft-vid-${i + 1}`, media_url: u }))
    }
    if (draft.video.refAudios?.length) {
      refAudios.value = draft.video.refAudios
        .filter((u) => isReusableMediaUrl(u))
        .map((u, i) => ({ name: `draft-aud-${i + 1}`, media_url: u }))
    }
  }
  draftNotice.value = t('creazyCanvas.form.draftRestored')
  window.setTimeout(() => {
    if (draftNotice.value === t('creazyCanvas.form.draftRestored')) {
      draftNotice.value = ''
    }
  }, 6000)
}


function onCanvasKeydown(ev: KeyboardEvent) {
  if (ev.key === 'Escape' && mediaPreview.value) {
    closeMediaPreview()
    return
  }
  const isSubmit = (ev.ctrlKey || ev.metaKey) && ev.key === 'Enter'
  if (!isSubmit) return
  ev.preventDefault()
  if (activeTab.value === 'image') {
    void generateImage()
  } else if (activeTab.value === 'video') {
    void generateVideo()
  }
}

function openTrayTaskBoard(work?: CreazyWork) {
  taskTrayExpanded.value = true
  if (work?.id) {
    focusWorkCard(work)
    return
  }
  if (activeTab.value === 'works') {
    void loadWorks()
    return
  }
  const running = trayWorks.value.find((w) => isActiveWorkStatus(w.status))
  if (running) {
    focusWorkCard(running)
    return
  }
  const first = trayWorks.value[0]
  if (first) {
    focusWorkCard(first)
    return
  }
  switchTab(activeTab.value === 'video' ? 'video' : 'image')
}

// Expand tray only when this browser session submits new jobs (not merely loading historical running works).
watch([activeImageJobs, activeVideoJobs], ([img, vid], prev) => {
  const prevImg = Number((prev && prev[0]) || 0)
  const prevVid = Number((prev && prev[1]) || 0)
  if (Number(img) > prevImg || Number(vid) > prevVid) {
    taskTrayDismissed.value = false
    taskTrayExpanded.value = true
  }
})

onMounted(async () => {
  activeTab.value = resolveTabFromRoute()
  if (route.path === '/creazy-canvas') {
    router.replace('/creazy-canvas/image')
  }
  await loadKeys()
  const draft = readCanvasDraft()
  if (draft) {
    applyCanvasDraft(draft)
    if (draft.selectedKeyId && selectedKeyId.value === draft.selectedKeyId) {
      await loadCatalog()
      syncFormModelsFromCatalog()
    }
  }
  draftHydrated = true
})

watch(
  () => [
    selectedKeyId.value,
    activeTab.value,
    imageForm.prompt,
    imageForm.model,
    imageForm.size,
    imageRefs.value.map((x) => x.media_url).join('|'),
    videoForm.prompt,
    videoForm.model,
    videoForm.resolution,
    videoForm.duration,
    videoForm.aspectRatio,
    videoForm.generateAudio,
    startFrame.value?.media_url,
    endFrame.value?.media_url,
    refImages.value.map((x) => x.media_url).join('|'),
    refVideos.value.map((x) => x.media_url).join('|'),
    refAudios.value.map((x) => x.media_url).join('|'),
  ],
  () => scheduleDraftSave(),
)

onMounted(() => {
  window.addEventListener('keydown', onCanvasKeydown)
  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('beforeunload', () => {
    try {
      writeCanvasDraft(collectCanvasDraft())
    } catch {
      /* ignore */
    }
  })
  nowTick.value = Date.now()
  if (nowTickTimer) clearInterval(nowTickTimer)
  nowTickTimer = setInterval(() => {
    nowTick.value = Date.now()
  }, 1000)
})

onBeforeUnmount(() => {
  cancelled = true
  clearPoll()
  clearWorksPoll()
  if (nowTickTimer) {
    clearInterval(nowTickTimer)
    nowTickTimer = null
  }
  if (draftSaveTimer) {
    clearTimeout(draftSaveTimer)
    draftSaveTimer = null
  }
  try {
    writeCanvasDraft(collectCanvasDraft())
  } catch {
    /* ignore */
  }
  closeMediaPreview()
  clearVideoResultPlayback()
  for (const u of workPreviewBlobUrls) revokeBlobUrl(u)
  workPreviewBlobUrls.clear()
  window.removeEventListener('keydown', onCanvasKeydown)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.cc-hero {
  position: relative;
  overflow: hidden;
}

.cc-hero::after {
  content: '';
  position: absolute;
  inset: auto -20% -40% auto;
  width: 280px;
  height: 280px;
  border-radius: 9999px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.18), transparent 70%);
  pointer-events: none;
}

.cc-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
  padding: 0.375rem;
  border-radius: 0.875rem;
  border: 1px solid rgb(229 231 235 / 1);
  background: rgb(255 255 255 / 1);
}

:global(.dark) .cc-tabs {
  border-color: rgb(55 65 81 / 1);
  background: rgb(17 24 39 / 1);
}

.cc-tab {
  border-radius: 0.625rem;
  padding: 0.5rem 0.95rem;
  font-size: 0.875rem;
  font-weight: 500;
  color: rgb(75 85 99 / 1);
  transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

:global(.dark) .cc-tab {
  color: rgb(209 213 219 / 1);
}

.cc-tab:hover {
  background: rgb(243 244 246 / 1);
}

:global(.dark) .cc-tab:hover {
  background: rgb(31 41 55 / 1);
}

.cc-tab--active {
  background: rgb(79 70 229 / 1);
  color: white;
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
}

:global(.dark) .cc-tab--active {
  background: rgb(99 102 241 / 1);
  color: white;
}

.cc-form-card {
  border-radius: 1rem;
}

.cc-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  border-radius: 0.9rem;
  padding: 1rem;
  border: 1px solid transparent;
}

.cc-panel--image {
  border-color: rgb(199 210 254 / 0.9);
  background: linear-gradient(160deg, rgb(238 242 255 / 0.95), rgb(255 255 255 / 0.98) 55%);
}

:global(.dark) .cc-panel--image {
  border-color: rgb(49 46 129 / 0.55);
  background: linear-gradient(160deg, rgb(30 27 75 / 0.35), rgb(17 24 39 / 0.9) 60%);
}

.cc-panel--video {
  border-color: rgb(221 214 254 / 0.95);
  background: linear-gradient(160deg, rgb(245 243 255 / 0.95), rgb(255 255 255 / 0.98) 55%);
}

:global(.dark) .cc-panel--video {
  border-color: rgb(76 29 149 / 0.55);
  background: linear-gradient(160deg, rgb(46 16 101 / 0.3), rgb(17 24 39 / 0.9) 60%);
}

.cc-panel__head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.cc-panel__badge {
  display: inline-flex;
  align-items: center;
  height: 1.5rem;
  border-radius: 0.375rem;
  padding: 0 0.5rem;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: white;
  flex-shrink: 0;
}

.cc-panel__badge--image {
  background: rgb(79 70 229 / 1);
}

.cc-panel__badge--video {
  background: rgb(124 58 237 / 1);
}

.cc-panel__meta {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.75rem;
  font-weight: 600;
  color: rgb(67 56 202 / 1);
}

:global(.dark) .cc-panel__meta {
  color: rgb(196 181 253 / 1);
}

.cc-field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  min-width: 0;
}

.cc-label {
  display: block;
  margin: 0;
  font-size: 0.8125rem;
  font-weight: 600;
  line-height: 1.25rem;
  color: rgb(55 65 81 / 1);
}

:global(.dark) .cc-label {
  color: rgb(209 213 219 / 1);
}

.cc-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.cc-size-current {
  font-size: 0.75rem;
  font-weight: 600;
  color: rgb(79 70 229 / 1);
}

:global(.dark) .cc-size-current {
  color: rgb(165 180 252 / 1);
}

.cc-control {
  height: 2.625rem;
  min-height: 2.625rem;
  box-sizing: border-box;
  padding-top: 0;
  padding-bottom: 0;
  line-height: 2.5rem;
}

select.cc-control {
  appearance: none;
  background-image:
    linear-gradient(45deg, transparent 50%, rgb(107 114 128) 50%),
    linear-gradient(135deg, rgb(107 114 128) 50%, transparent 50%);
  background-position:
    calc(100% - 16px) calc(50% - 2px),
    calc(100% - 11px) calc(50% - 2px);
  background-size: 5px 5px, 5px 5px;
  background-repeat: no-repeat;
  padding-right: 2rem;
}


.cc-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.cc-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2rem;
  border-radius: 9999px;
  border: 1px solid rgb(199 210 254 / 1);
  background: rgb(255 255 255 / 0.9);
  padding: 0.25rem 0.75rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: rgb(67 56 202 / 1);
  transition: all 0.15s ease;
}

:global(.dark) .cc-chip {
  border-color: rgb(67 56 202 / 0.6);
  background: rgb(17 24 39 / 0.7);
  color: rgb(199 210 254 / 1);
}

.cc-chip:hover {
  border-color: rgb(129 140 248 / 1);
  background: rgb(238 242 255 / 1);
}

:global(.dark) .cc-chip:hover {
  background: rgb(49 46 129 / 0.35);
}

.cc-chip--active {
  border-color: transparent;
  background: rgb(79 70 229 / 1);
  color: white;
  box-shadow: 0 1px 2px rgb(79 70 229 / 0.35);
}

:global(.dark) .cc-chip--active {
  background: rgb(99 102 241 / 1);
  color: white;
}

.cc-media-panel {
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.cc-create {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
  padding-top: 0.25rem;
}

.cc-create__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.35rem 0.75rem;
}

.cc-create__title {
  font-size: 0.8125rem;
  font-weight: 700;
  color: rgb(31 41 55 / 1);
}

:global(.dark) .cc-create__title {
  color: rgb(243 244 246 / 1);
}

.cc-create__hint {
  font-size: 0.75rem;
  color: rgb(107 114 128 / 1);
}

:global(.dark) .cc-create__hint {
  color: rgb(156 163 175 / 1);
}

.cc-submit {
  width: 100%;
  justify-content: center;
  min-height: 2.75rem;
  border-radius: 0.85rem;
  font-weight: 600;
}

.cc-textarea {
  min-height: 8.5rem;
  line-height: 1.55;
  resize: vertical;
}

.cc-board-card {
  border-radius: 1rem;
  background:
    linear-gradient(180deg, rgb(255 255 255 / 1), rgb(248 250 252 / 0.95));
}

:global(.dark) .cc-board-card {
  background:
    linear-gradient(180deg, rgb(17 24 39 / 1), rgb(15 23 42 / 0.92));
}

.cc-create__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.35rem 0.75rem;
  font-size: 0.75rem;
}

.cc-create__price {
  font-weight: 600;
  color: rgb(5 150 105 / 1);
}

:global(.dark) .cc-create__price {
  color: rgb(52 211 153 / 1);
}

.cc-create__shortcut {
  color: rgb(107 114 128 / 1);
}

:global(.dark) .cc-create__shortcut {
  color: rgb(156 163 175 / 1);
}

.cc-create__draft {
  margin: 0;
  border-radius: 0.65rem;
  border: 1px solid rgb(191 219 254 / 1);
  background: rgb(239 246 255 / 0.9);
  padding: 0.45rem 0.65rem;
  font-size: 0.75rem;
  color: rgb(29 78 216 / 1);
}

:global(.dark) .cc-create__draft {
  border-color: rgb(30 64 175 / 0.55);
  background: rgb(23 37 84 / 0.55);
  color: rgb(147 197 253 / 1);
}

.cc-cap-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.55rem;
}

.cc-cap-chip {
  display: inline-flex;
  align-items: center;
  border-radius: 9999px;
  border: 1px solid rgb(199 210 254 / 1);
  background: rgb(238 242 255 / 0.9);
  padding: 0.15rem 0.55rem;
  font-size: 0.6875rem;
  font-weight: 600;
  color: rgb(67 56 202 / 1);
  line-height: 1.35;
}

:global(.dark) .cc-cap-chip {
  border-color: rgb(67 56 202 / 0.45);
  background: rgb(49 46 129 / 0.35);
  color: rgb(199 210 254 / 1);
}

.cc-cap-chip--video {
  border-color: rgb(221 214 254 / 1);
  background: rgb(245 243 255 / 0.95);
  color: rgb(109 40 217 / 1);
}

:global(.dark) .cc-cap-chip--video {
  border-color: rgb(91 33 182 / 0.5);
  background: rgb(76 29 149 / 0.35);
  color: rgb(221 214 254 / 1);
}

.cc-params-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 0.85rem 0.9rem;
  align-items: start;
}

@media (min-width: 640px) {
  .cc-params-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.cc-params-grid > .cc-field {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.cc-params-grid .cc-control,
.cc-params-grid select.cc-control {
  width: 100%;
  min-height: 2.5rem;
}

.cc-task-tray {
  position: fixed;
  right: 1rem;
  bottom: 1rem;
  z-index: 90;
  width: min(20rem, calc(100vw - 1.5rem));
  max-height: min(40vh, 18rem);
  border-radius: 1rem;
  border: 1px solid rgb(226 232 240 / 1);
  background: rgb(255 255 255 / 0.96);
  box-shadow: 0 18px 40px rgb(15 23 42 / 0.16);
  backdrop-filter: blur(10px);
  overflow: hidden;
  pointer-events: auto;
}

:global(.dark) .cc-task-tray {
  border-color: rgb(51 65 85 / 1);
  background: rgb(15 23 42 / 0.94);
  box-shadow: 0 18px 40px rgb(0 0 0 / 0.45);
}

.cc-task-tray--collapsed {
  width: min(16rem, calc(100vw - 1.5rem));
}

.cc-task-tray__bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.7rem 0.8rem 0.55rem;
  border-bottom: 1px solid rgb(241 245 249 / 1);
}

:global(.dark) .cc-task-tray__bar {
  border-bottom-color: rgb(30 41 59 / 1);
}

.cc-task-tray__toggle {
  flex: 1;
  min-width: 0;
  text-align: left;
  background: transparent;
  border: 0;
  padding: 0;
  cursor: pointer;
}

.cc-task-tray__title {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.8125rem;
  font-weight: 700;
  color: rgb(15 23 42 / 1);
}

:global(.dark) .cc-task-tray__title {
  color: rgb(248 250 252 / 1);
}

.cc-task-tray__badge {
  display: inline-flex;
  min-width: 1.25rem;
  height: 1.25rem;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  background: rgb(79 70 229 / 1);
  color: white;
  font-size: 0.6875rem;
  font-weight: 700;
  padding: 0 0.35rem;
}

.cc-task-tray__hint {
  display: block;
  margin-top: 0.15rem;
  font-size: 0.6875rem;
  color: rgb(100 116 139 / 1);
}

.cc-task-tray__actions {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.2rem;
}

.cc-task-tray__link,
.cc-task-tray__dismiss {
  border: 0;
  background: transparent;
  padding: 0;
  font-size: 0.6875rem;
  cursor: pointer;
}

.cc-task-tray__link {
  color: rgb(79 70 229 / 1);
  font-weight: 600;
}

.cc-task-tray__dismiss {
  color: rgb(148 163 184 / 1);
}

.cc-pagination {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem 0.75rem;
  padding-top: 0.25rem;
}

.cc-pagination--inline {
  padding-top: 0;
  justify-content: flex-end;
}

.cc-pagination--board {
  padding-top: 0;
  justify-content: flex-start;
}

.cc-board-head-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem 0.75rem;
  max-width: min(100%, 34rem);
}

/* Keep bottom board content clear of the floating task tray */
.cc-board-card {
  padding-bottom: 0.25rem;
  margin-bottom: 5.5rem;
}

.cc-pagination__meta {
  font-size: 0.75rem;
  color: rgb(100 116 139 / 1);
}

:global(.dark) .cc-pagination__meta {
  color: rgb(148 163 184 / 1);
}

.cc-pagination__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem;
}

.cc-task-tray__list {
  max-height: min(28vh, 11rem);
  overflow: auto;
  padding: 0.45rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.cc-task-tray__item {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr);
  grid-template-rows: auto auto;
  gap: 0.2rem 0.45rem;
  width: 100%;
  text-align: left;
  border-radius: 0.75rem;
  border: 1px solid rgb(226 232 240 / 0.9);
  background: rgb(248 250 252 / 0.85);
  padding: 0.5rem 0.55rem;
  cursor: pointer;
}

:global(.dark) .cc-task-tray__item {
  border-color: rgb(51 65 85 / 0.9);
  background: rgb(15 23 42 / 0.65);
}

.cc-task-tray__item .badge {
  grid-column: 1;
  grid-row: 1;
}

.cc-task-tray__kind {
  grid-column: 2;
  grid-row: 1;
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: rgb(100 116 139 / 1);
  align-self: center;
}

.cc-task-tray__model {
  grid-column: 3;
  grid-row: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.6875rem;
  font-weight: 600;
  color: rgb(67 56 202 / 1);
  align-self: center;
}

:global(.dark) .cc-task-tray__model {
  color: rgb(165 180 252 / 1);
}

.cc-task-tray__prompt {
  grid-column: 1 / -1;
  grid-row: 2;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.75rem;
  color: rgb(51 65 85 / 1);
}

:global(.dark) .cc-task-tray__prompt {
  color: rgb(203 213 225 / 1);
}

.cc-task-tray__empty {
  margin: 0;
  padding: 0.75rem;
  text-align: center;
  font-size: 0.75rem;
  color: rgb(148 163 184 / 1);
}


.cc-work-card--flash {
  animation: cc-flash 1.2s ease-in-out 0s 2;
  box-shadow: 0 0 0 2px rgb(99 102 241 / 0.35);
}
.cc-work-card--focus {
  box-shadow: 0 0 0 2px rgb(16 185 129 / 0.45);
}
@keyframes cc-flash {
  0%, 100% { background-color: transparent; }
  50% { background-color: rgb(238 242 255 / 0.85); }
}
:global(.dark) .cc-work-card--flash {
  animation-name: cc-flash-dark;
}
@keyframes cc-flash-dark {
  0%, 100% { background-color: transparent; }
  50% { background-color: rgb(49 46 129 / 0.35); }
}
.cc-create__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: stretch;
}
.cc-submit-secondary {
  min-height: 2.5rem;
}
.cc-create__balance {
  margin: 0;
  font-size: 0.75rem;
  color: rgb(180 83 9 / 1);
}
.cc-create__balance--blocked {
  color: rgb(220 38 38 / 1);
  font-weight: 600;
}
:global(.dark) .cc-create__balance {
  color: rgb(251 191 36 / 1);
}
:global(.dark) .cc-create__balance--blocked {
  color: rgb(252 165 165 / 1);
}
.cc-field--error .cc-label {
  color: rgb(220 38 38 / 1);
}
.cc-input--error {
  border-color: rgb(248 113 113 / 1) !important;
  box-shadow: 0 0 0 1px rgb(248 113 113 / 0.35);
}
.cc-field__error {
  margin: 0.35rem 0 0;
  font-size: 0.75rem;
  color: rgb(220 38 38 / 1);
}
:global(.dark) .cc-field__error {
  color: rgb(252 165 165 / 1);
}
.cc-rules-card {
  border-radius: 0.9rem;
  border: 1px solid rgb(226 232 240 / 1);
  background: linear-gradient(180deg, rgb(248 250 252 / 0.95), rgb(255 255 255 / 0.9));
  padding: 0.75rem 0.9rem;
}
:global(.dark) .cc-rules-card {
  border-color: rgb(51 65 85 / 1);
  background: linear-gradient(180deg, rgb(15 23 42 / 0.8), rgb(15 23 42 / 0.55));
}
.cc-rules-card--empty {
  font-size: 0.75rem;
  color: rgb(148 163 184 / 1);
}
.cc-rules-card__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: rgb(71 85 105 / 1);
  margin-bottom: 0.4rem;
}
:global(.dark) .cc-rules-card__title {
  color: rgb(203 213 225 / 1);
}
.cc-rules-card__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}
.cc-rules-card__chip {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  border: 1px solid rgb(199 210 254 / 1);
  background: rgb(238 242 255 / 1);
  color: rgb(67 56 202 / 1);
  font-size: 0.6875rem;
  font-weight: 600;
  padding: 0.15rem 0.55rem;
}
:global(.dark) .cc-rules-card__chip {
  border-color: rgb(67 56 202 / 0.45);
  background: rgb(49 46 129 / 0.35);
  color: rgb(199 210 254 / 1);
}
.cc-rules-card__meta {
  margin: 0.45rem 0 0;
  font-size: 0.6875rem;
  color: rgb(100 116 139 / 1);
}
.cc-dropzone {
  border-radius: 0.85rem;
  border: 1px dashed rgb(203 213 225 / 1);
  background: rgb(248 250 252 / 0.7);
  padding: 0.7rem 0.8rem;
}
:global(.dark) .cc-dropzone {
  border-color: rgb(71 85 105 / 1);
  background: rgb(15 23 42 / 0.45);
}
.cc-dropzone__hint {
  margin: 0.4rem 0 0;
  font-size: 0.6875rem;
  color: rgb(148 163 184 / 1);
}
.cc-progress {
  margin-top: 0.4rem;
  height: 0.35rem;
  border-radius: 999px;
  background: rgb(226 232 240 / 1);
  overflow: hidden;
}
:global(.dark) .cc-progress {
  background: rgb(51 65 85 / 1);
}
.cc-progress__bar {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, rgb(99 102 241 / 1), rgb(56 189 248 / 1));
  transition: width 0.2s ease;
}
.cc-progress-row {
  display: grid;
  gap: 0.2rem;
  font-size: 0.6875rem;
  color: rgb(100 116 139 / 1);
}
.cc-pagination__jump,
.cc-pagination__size {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
}
.cc-pagination__input {
  width: 4.2rem;
  border-radius: 0.5rem;
  border: 1px solid rgb(203 213 225 / 1);
  background: white;
  padding: 0.25rem 0.4rem;
  font-size: 0.75rem;
}
:global(.dark) .cc-pagination__input {
  border-color: rgb(71 85 105 / 1);
  background: rgb(15 23 42 / 1);
  color: rgb(226 232 240 / 1);
}
.cc-pagination__select {
  border-radius: 0.5rem;
  border: 1px solid rgb(203 213 225 / 1);
  background: white;
  padding: 0.25rem 0.4rem;
  font-size: 0.75rem;
}
:global(.dark) .cc-pagination__select {
  border-color: rgb(71 85 105 / 1);
  background: rgb(15 23 42 / 1);
  color: rgb(226 232 240 / 1);
}
.cc-batch-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.55rem 0.7rem;
  border-radius: 0.85rem;
  border: 1px solid rgb(226 232 240 / 1);
  background: rgb(248 250 252 / 0.85);
}
:global(.dark) .cc-batch-bar {
  border-color: rgb(51 65 85 / 1);
  background: rgb(15 23 42 / 0.55);
}
.cc-task-tray__counts {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.15rem;
  font-size: 0.625rem;
  font-weight: 600;
  color: rgb(100 116 139 / 1);
}
.cc-empty-guide {
  border-radius: 0.85rem;
  border: 1px solid rgb(226 232 240 / 1);
  background: white;
  padding: 0.75rem 0.9rem;
}
:global(.dark) .cc-empty-guide {
  border-color: rgb(51 65 85 / 1);
  background: rgb(15 23 42 / 0.6);
}

</style>
