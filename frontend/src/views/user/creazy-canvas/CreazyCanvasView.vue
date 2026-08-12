<template>
  <AppLayout>
    <div class="cc-shell" :class="{ 'cc-shell--tray': showTaskTray, 'cc-shell--tray-open': showTaskTray && taskTrayExpanded }">
      <header class="cc-topbar">
        <div class="cc-topbar__main">
          <div class="cc-topbar__brand">
            <div class="cc-mark" aria-hidden="true">
              <span class="cc-mark__frame"></span>
              <span class="cc-mark__beam"></span>
            </div>
            <div class="cc-topbar__titles">
              <p class="cc-eyebrow">{{ t('creazyCanvas.brandEyebrow') }}</p>
              <h1 class="cc-topbar__title">{{ t('creazyCanvas.title') }}</h1>
              <p class="cc-topbar__sub">{{ t('creazyCanvas.subtitle') }}</p>
            </div>
          </div>

          <div class="cc-topbar__key">
            <div class="cc-topbar__key-head">
              <label class="cc-topbar__key-label" for="cc-key-select">{{ t('creazyCanvas.key.label') }}</label>
              <span v-if="selectedKeyId" class="cc-pill" :class="keyReadyChipClass">
                <i class="cc-pill__dot" :class="keyReadyDotClass" />
                {{ keyReadyLabel }}
              </span>
            </div>
            <select
              id="cc-key-select"
              v-model.number="selectedKeyId"
              class="input cc-topbar__select"
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
            <p v-if="!loadingKeys && keys.length === 0" class="cc-topbar__empty">
              {{ t('creazyCanvas.key.empty') }}
            </p>
            <div v-if="selectedKeyId" class="cc-topbar__meta">
              <span v-if="userBalance != null" class="cc-topbar__balance" :title="t('creazyCanvas.key.balanceHint')">
                <span class="cc-topbar__balance-label">{{ t('creazyCanvas.key.balance') }}</span>
                <strong class="cc-topbar__balance-value">
                  <span class="cc-topbar__balance-currency">$</span>{{ formatMoney(userBalance) }}
                </strong>
              </span>
              <p class="cc-topbar__hint">{{ t('creazyCanvas.key.selectOnlyHint') }}</p>
            </div>
          </div>
        </div>
      </header>

      <div class="cc-tabs" role="tablist">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          role="tab"
          class="cc-tab"
          :class="{ 'cc-tab--active': activeTab === tab.id }"
          :aria-selected="activeTab === tab.id"
          @click="switchTab(tab.id)"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- Image -->
      <section v-if="activeTab === 'image'" class="grid gap-5 lg:grid-cols-2">
        <div class="card cc-form-card cc-surface cc-form-stack">
          <div class="cc-studio-head">
            <div class="cc-studio-head__kicker">
              <span class="cc-studio-head__kicker-dot" aria-hidden="true"></span>
              STUDIO
            </div>
            <h2 class="cc-studio-head__title">{{ t('creazyCanvas.tabs.image') }}</h2>
            <p class="cc-studio-head__sub">{{ t('creazyCanvas.form.createSectionHintImage') }}</p>
          </div>
          <div class="cc-field" :class="{ 'cc-field--error': imageFieldErrors.prompt }">
            <label class="cc-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <div class="cc-prompt-wrap">
              <textarea
                ref="imagePromptEl"
                v-model="imageForm.prompt"
                rows="5"
                class="input cc-textarea"
                :class="{ 'cc-input--error': imageFieldErrors.prompt }"
                :placeholder="t('creazyCanvas.form.promptPlaceholder')"
                @paste="onPasteMedia($event, 'imageRefs')"
                @input="onPromptInput('image', $event)"
                @keydown="onPromptKeydown('image', $event)"
                @keyup="onPromptInput('image', $event)"
                @click="onPromptInput('image', $event)"
                @blur="onPromptBlur"
              />
              <div
                v-if="mention.open && mention.scope === 'image'"
                class="cc-mention-menu"
                role="listbox"
                :aria-label="t('creazyCanvas.form.mentionTitle')"
                @mousedown.prevent
              >
                <div class="cc-mention-menu__head">{{ t('creazyCanvas.form.mentionTitle') }}</div>
                <button
                  v-for="(item, mIdx) in mentionFilteredItems"
                  :key="item.id"
                  type="button"
                  class="cc-mention-item"
                  :class="{ 'cc-mention-item--active': mIdx === mention.index }"
                  role="option"
                  :aria-selected="mIdx === mention.index"
                  @mouseenter="mention.index = mIdx"
                  @click="insertMention(item)"
                >
                  <span class="cc-mention-item__thumb">
                    <img v-if="item.preview_url" :src="item.preview_url" alt="" />
                    <span v-else class="cc-mention-item__kind">{{ item.kindLabel }}</span>
                  </span>
                  <span class="cc-mention-item__body">
                    <span class="cc-mention-item__label">{{ item.label }}</span>
                    <span class="cc-mention-item__token">{{ item.token }}</span>
                  </span>
                </button>
                <div v-if="!mentionFilteredItems.length" class="cc-mention-menu__empty">
                  {{ mentionAllItems.length ? t('creazyCanvas.form.mentionNoMatch') : t('creazyCanvas.form.mentionEmpty') }}
                </div>
              </div>
            </div>
            <p class="cc-prompt-hint">{{ t('creazyCanvas.form.mentionHint') }}</p>
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
                class="input cc-control"
                :disabled="loadingCatalog"
              >
                <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : t('creazyCanvas.form.selectPlaceholder') }}</option>
                <option v-for="m in imageModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
              </select>
              <p
                v-if="!loadingCatalog && selectedKeyId && imageModels.length === 0"
                class="cc-callout cc-callout--warn"
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
              <p class="cc-field-hint">{{ imageSizeHintText }}</p>
              <p v-if="imageSizeLiveError" class="cc-field__error">{{ imageSizeLiveError }}</p>
            </div>
          </div>

          <div
            v-if="imageRefSupported"
            class="cc-media-panel"
          >
            <div class="cc-media-panel__head">
              <div>
                <label class="cc-label">
                  {{ t('creazyCanvas.form.imageRefs') }}
                  <span class="cc-media-count">({{ imageRefs.length }}/{{ imageRefMax }})</span>
                  <span v-if="imageRefRequired" class="cc-req">*</span>
                </label>
                <p class="cc-field-hint">
                  {{ imageRefRequired ? t('creazyCanvas.form.imageRefsRequiredHint') : t('creazyCanvas.form.imageRefsHint') }}
                </p>
              </div>
              <button
                v-if="imageRefs.length"
                type="button"
                class="cc-link-danger"
                :disabled="!!uploadingImageRef"
                @click="clearImageRefs"
              >
                {{ t('creazyCanvas.form.clearAll') }}
              </button>
            </div>
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
            <ul v-if="imageRefs.length" class="cc-media-list">
              <li
                v-for="(item, idx) in imageRefs"
                :key="'img-ref-' + idx + item.media_url"
                class="cc-media-item"
                draggable="true"
                @dragstart="onMediaDragStart('imageRefs', idx)"
                @dragover.prevent
                @drop.prevent="onMediaDropReorder('imageRefs', idx)"
              >
                <img :src="item.preview_url || item.media_url" alt="" class="cc-media-item__thumb" />
                <div class="cc-media-item__body">
                  <div class="cc-media-item__row">
                    <span class="cc-media-token">@Image{{ idx + 1 }}</span>
                    <p class="cc-media-item__name">{{ item.name }}</p>
                  </div>
                  <p class="cc-media-item__url">{{ item.media_url }}</p>
                </div>
                <button type="button" class="cc-link-danger" @click="removeImageRef(idx)">
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
            <p v-if="imageRunningCount" class="cc-callout cc-callout--live">
              {{ t('creazyCanvas.tasks.runningCount', { n: imageRunningCount }) }}
            </p>
            <p v-if="imageError" class="cc-callout cc-callout--bad">{{ imageError }}</p>
            <p v-if="imageSaveMessage" class="cc-callout cc-callout--ok">{{ imageSaveMessage }}</p>
          </div>
        </div>

        <div class="card cc-board-card cc-surface">
          <div class="cc-board-head">
            <div class="cc-board-head__main">
              <div class="cc-board-head__kicker">
                <span class="cc-board-head__kicker-dot" aria-hidden="true"></span>
                LIVE
              </div>
              <h2 class="cc-board-head__title">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="cc-board-head__sub">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm cc-board-head__refresh" :disabled="loadingWorks" @click="() => loadWorks()">
              <Icon name="refresh" size="sm" class="mr-1.5" :class="loadingWorks ? 'animate-spin' : ''" />
              {{ t('creazyCanvas.works.refresh') }}
            </button>
          </div>
          <div class="cc-board-body">
            <div v-if="imageResultUrls.length" class="cc-latest">
              <div class="cc-latest__head">
                <span class="cc-latest__badge">NOW</span>
                <p class="cc-latest__title">{{ t('creazyCanvas.tasks.latestPreview') }}</p>
              </div>
              <div class="cc-latest__grid">
                <button v-for="(url, idx) in imageResultUrls" :key="'latest-img-' + idx" type="button" class="cc-latest__tile" @click="openMediaPreview({ type: 'image', url })">
                  <img :src="url" alt="latest" />
                </button>
              </div>
            </div>
            <div v-if="!imageTaskWorks.length" class="cc-empty-stage cc-empty-stage--compact">
              <div class="cc-empty-stage__icon cc-empty-stage__icon--soft" aria-hidden="true"><span class="cc-empty-stage__glyph">◇</span></div>
              <p class="cc-empty-stage__title">{{ t('creazyCanvas.tasks.empty') }}</p>
              <p class="cc-empty-stage__sub">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <div v-else class="cc-task-list">
              <article v-for="work in imageTaskWorks" :key="work.id" class="cc-task-card" :class="taskCardClass(work)">
                <div class="cc-task-card__top">
                  <span class="cc-task-status" :class="taskStatusTone(work.status)">
                    <i class="cc-task-status__dot" :class="{ 'is-pulse': isActiveWorkStatus(work.status) }" />
                    {{ workStatusLabel(work.status) }}
                  </span>
                  <span class="cc-task-model">{{ work.public_model || '—' }}</span>
                  <span v-if="work.created_at" class="cc-task-time">{{ formatDateTime(work.created_at) }}</span>
                </div>
                <p class="cc-task-prompt">{{ work.prompt || ('#' + work.id) }}</p>
                <p v-if="isActiveWorkStatus(work.status)" class="cc-task-elapsed">{{ t('creazyCanvas.tasks.elapsed', { time: formatElapsed(work.created_at || work.updated_at) }) }}</p>
                <p v-if="workErrorText(work)" class="cc-task-error">{{ workErrorText(work) }}</p>
                <div class="cc-task-actions">
                  <button v-if="canPreviewWork(work)" type="button" class="btn btn-primary btn-sm" :disabled="workPreviewLoading[String(work.id)]" @click="openWorkPreview(work)">
                    {{ workPreviewLoading[String(work.id)] ? t('creazyCanvas.works.previewLoading') : t('creazyCanvas.works.preview') }}
                  </button>
                  <button v-if="canPreviewWork(work)" type="button" class="btn btn-secondary btn-sm" :disabled="downloadingWorkId === String(work.id)" @click="downloadWork(work)">
                    {{ t('creazyCanvas.works.download') }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" @click="reuseWork(work)">{{ t('creazyCanvas.works.reuse') }}</button>
                </div>
              </article>
            </div>
            <div v-if="worksTotal > 0" class="cc-pagination cc-pagination--board">
              <span class="cc-pagination__meta">{{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}</span>
              <div class="cc-pagination__actions">
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks || worksPage <= 1" @click="goToWorksPrevPage($event)">{{ t('creazyCanvas.works.prevPage') }}</button>
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks || worksPage >= worksPages" @click="goToWorksNextPage($event)">{{ t('creazyCanvas.works.nextPage') }}</button>
                <label class="cc-pagination__jump">
                  <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                  <input v-model="worksPageJumpInput" type="number" min="1" :max="worksPages" class="cc-pagination__input" :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')" @keyup.enter="jumpWorksPageFromInput" />
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">{{ t('creazyCanvas.works.pageJump') }}</button>
                </label>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Video -->
      <section v-else-if="activeTab === 'video'" class="grid gap-5 lg:grid-cols-2">
        <div class="card cc-form-card cc-surface cc-form-stack">
          <div class="cc-studio-head">
            <div class="cc-studio-head__kicker">
              <span class="cc-studio-head__kicker-dot" aria-hidden="true"></span>
              STUDIO
            </div>
            <h2 class="cc-studio-head__title">{{ t('creazyCanvas.tabs.video') }}</h2>
            <p class="cc-studio-head__sub">{{ t('creazyCanvas.form.createSectionHintVideo') }}</p>
          </div>
          <div class="cc-field" :class="{ 'cc-field--error': videoFieldErrors.prompt }">
            <label class="cc-label">{{ t('creazyCanvas.form.prompt') }}</label>
            <div class="cc-prompt-wrap">
              <textarea
                ref="videoPromptEl"
                v-model="videoForm.prompt"
                rows="5"
                class="input cc-textarea"
                :class="{ 'cc-input--error': videoFieldErrors.prompt }"
                :placeholder="t('creazyCanvas.form.promptPlaceholder')"
                @paste="onPasteMedia($event, 'refImages')"
                @input="onPromptInput('video', $event)"
                @keydown="onPromptKeydown('video', $event)"
                @keyup="onPromptInput('video', $event)"
                @click="onPromptInput('video', $event)"
                @blur="onPromptBlur"
              />
              <div
                v-if="mention.open && mention.scope === 'video'"
                class="cc-mention-menu"
                role="listbox"
                :aria-label="t('creazyCanvas.form.mentionTitle')"
                @mousedown.prevent
              >
                <div class="cc-mention-menu__head">{{ t('creazyCanvas.form.mentionTitle') }}</div>
                <button
                  v-for="(item, mIdx) in mentionFilteredItems"
                  :key="item.id"
                  type="button"
                  class="cc-mention-item"
                  :class="{ 'cc-mention-item--active': mIdx === mention.index }"
                  role="option"
                  :aria-selected="mIdx === mention.index"
                  @mouseenter="mention.index = mIdx"
                  @click="insertMention(item)"
                >
                  <span class="cc-mention-item__thumb">
                    <img v-if="item.preview_url" :src="item.preview_url" alt="" />
                    <span v-else class="cc-mention-item__kind">{{ item.kindLabel }}</span>
                  </span>
                  <span class="cc-mention-item__body">
                    <span class="cc-mention-item__label">{{ item.label }}</span>
                    <span class="cc-mention-item__token">{{ item.token }}</span>
                  </span>
                </button>
                <div v-if="!mentionFilteredItems.length" class="cc-mention-menu__empty">
                  {{ mentionAllItems.length ? t('creazyCanvas.form.mentionNoMatch') : t('creazyCanvas.form.mentionEmpty') }}
                </div>
              </div>
            </div>
            <p class="cc-prompt-hint">{{ t('creazyCanvas.form.mentionHint') }}</p>
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
                class="input cc-control"
                :disabled="loadingCatalog"
              >
                <option value="">{{ loadingCatalog ? t('creazyCanvas.catalog.loading') : t('creazyCanvas.form.selectPlaceholder') }}</option>
                <option v-for="m in videoModels" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
              </select>
              <p
                v-if="!loadingCatalog && selectedKeyId && videoModels.length === 0"
                class="cc-callout cc-callout--warn"
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
            class="cc-check"
            :title="mediaCaps.forceGeneratedAudio ? t('creazyCanvas.form.forceGeneratedAudio') : undefined"
          >
            <input
              v-model="videoForm.generateAudio"
              type="checkbox"
              class="cc-check__input"
              :disabled="mediaCaps.forceGeneratedAudio"
            />
            {{ t('creazyCanvas.form.generateAudio') }}
            <span v-if="mediaCaps.forceGeneratedAudio" class="cc-field-hint cc-field-hint--inline">
              ({{ t('creazyCanvas.form.forceGeneratedAudio') }})
            </span>
          </label>

          <div
            v-if="selectedVideoModel"
            class="cc-limits-card"
          >
            <div class="cc-limits-card__title">
              {{ t('creazyCanvas.form.mediaLimits') }}
            </div>
            <div class="cc-limits-card__body">
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
            <div class="cc-limits-card__progress">
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
            <p class="cc-field-hint">
              {{ t('creazyCanvas.form.optionalMediaHint') }}
            </p>
            <p v-if="mediaCaps.requireStartFrame" class="cc-callout cc-callout--warn">
              {{ t('creazyCanvas.form.constraintHintRequireStart') }}
            </p>
            <p v-if="mediaCaps.allowEndFrame" class="cc-callout cc-callout--warn">
              {{ t('creazyCanvas.form.constraintHintEndNeedsStart') }}
            </p>
            <p v-if="mediaCaps.framesExclusiveWithRefs" class="cc-callout cc-callout--warn">
              {{ t('creazyCanvas.form.constraintHintExclusiveModes') }}
            </p>
          </div>

          <div class="cc-media-zone">
            <div v-if="mediaCaps.allowStartFrame" class="cc-media-panel">
              <div class="cc-media-panel__head">
                <div>
                  <label class="cc-label">
                    {{ mediaCaps.requireStartFrame ? t('creazyCanvas.form.startFrameRequired') : t('creazyCanvas.form.startFrame') }}
                    <span v-if="mediaCaps.requireStartFrame" class="cc-req">*</span>
                  </label>
                </div>
                <button
                  v-if="startFrame"
                  type="button"
                  class="cc-link-danger"
                  :disabled="!!uploadingMedia"
                  @click="clearStartFrame"
                >
                  {{ t('creazyCanvas.form.remove') }}
                </button>
              </div>
              <div
                class="cc-dropzone"
                :class="{ 'is-disabled': frameUploadsBlocked }"
                @dragover.prevent
                @drop="onDropMedia($event, 'startFrame')"
              >
                <div class="cc-dropzone__actions">
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
                  <span v-if="startFrameToken" class="cc-media-token">{{ startFrameToken }}</span>
                </div>
                <div class="cc-url-row">
                  <input
                    v-model="startFrameUrlInput"
                    type="url"
                    class="input cc-control cc-url-row__input"
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
                <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
              </div>
              <ul v-if="startFrame" class="cc-media-list cc-media-list--single">
                <li class="cc-media-item">
                  <img
                    v-if="startFrame.preview_url || startFrame.media_url"
                    :src="startFrame.preview_url || startFrame.media_url"
                    alt=""
                    class="cc-media-item__thumb"
                  />
                  <div v-else class="cc-media-item__badge">FR</div>
                  <div class="cc-media-item__body">
                    <div class="cc-media-item__row">
                      <span class="cc-media-token">{{ startFrameToken || '@Image1' }}</span>
                      <p class="cc-media-item__name">{{ startFrame.name }}</p>
                    </div>
                    <p class="cc-media-item__url">{{ startFrame.media_url }}</p>
                  </div>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.allowEndFrame" class="cc-media-panel">
              <div class="cc-media-panel__head">
                <div>
                  <label class="cc-label">{{ t('creazyCanvas.form.endFrame') }}</label>
                  <p class="cc-field-hint">{{ t('creazyCanvas.form.endNeedsStart') }}</p>
                </div>
                <button
                  v-if="endFrame"
                  type="button"
                  class="cc-link-danger"
                  :disabled="!!uploadingMedia"
                  @click="endFrame = null"
                >
                  {{ t('creazyCanvas.form.remove') }}
                </button>
              </div>
              <div
                class="cc-dropzone"
                :class="{ 'is-disabled': !startFrame || frameUploadsBlocked }"
                @dragover.prevent
                @drop="onDropMedia($event, 'endFrame')"
              >
                <div class="cc-dropzone__actions">
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
                  <span v-if="endFrameToken" class="cc-media-token">{{ endFrameToken }}</span>
                </div>
                <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
              </div>
              <ul v-if="endFrame" class="cc-media-list cc-media-list--single">
                <li class="cc-media-item">
                  <img
                    v-if="endFrame.preview_url || endFrame.media_url"
                    :src="endFrame.preview_url || endFrame.media_url"
                    alt=""
                    class="cc-media-item__thumb"
                  />
                  <div v-else class="cc-media-item__badge">FR</div>
                  <div class="cc-media-item__body">
                    <div class="cc-media-item__row">
                      <span class="cc-media-token">{{ endFrameToken || '@Image2' }}</span>
                      <p class="cc-media-item__name">{{ endFrame.name }}</p>
                    </div>
                    <p class="cc-media-item__url">{{ endFrame.media_url }}</p>
                  </div>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.maxImages > 0" class="cc-media-panel">
              <div class="cc-media-panel__head">
                <div>
                  <label class="cc-label">
                    {{ t('creazyCanvas.form.refImages') }}
                    <span class="cc-media-count">({{ refImages.length }}/{{ mediaCaps.maxImages }})</span>
                  </label>
                </div>
                <button
                  v-if="refImages.length"
                  type="button"
                  class="cc-link-danger"
                  :disabled="!!uploadingMedia"
                  @click="clearRefImages"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </div>
              <div
                class="cc-dropzone"
                :class="{ 'is-disabled': refUploadsBlocked || refImages.length >= mediaCaps.maxImages }"
                @dragover.prevent
                @drop="onDropMedia($event, 'refImages')"
              >
                <div class="cc-dropzone__actions">
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
                        : mediaCaps.maxImages >= 5
                          ? t('creazyCanvas.form.uploadMultiple')
                          : t('creazyCanvas.form.upload')
                    }}
                  </button>
                  <span v-if="mediaCaps.maxImages > 0" class="cc-media-count">{{ mediaProgressText(refImages.length, mediaCaps.maxImages) }}</span>
                </div>
                <div v-if="mediaCaps.maxImages > 0" class="cc-progress">
                  <div class="cc-progress__bar" :style="{ width: Math.min(100, (refImages.length / mediaCaps.maxImages) * 100) + '%' }" />
                </div>
                <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
                <p v-if="refImages.length > 1" class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaReorderHint') }}</p>
              </div>
              <ul v-if="refImages.length" class="cc-media-list">
                <li
                  v-for="(item, idx) in refImages"
                  :key="'ref-img-' + idx + '-' + item.media_url"
                  class="cc-media-item"
                  draggable="true"
                  @dragstart="onMediaDragStart('refImages', idx)"
                  @dragover.prevent
                  @drop.prevent="onMediaDropReorder('refImages', idx)"
                >
                  <img :src="item.preview_url || item.media_url" alt="" class="cc-media-item__thumb" />
                  <div class="cc-media-item__body">
                    <div class="cc-media-item__row">
                      <span class="cc-media-token">@Image{{ idx + 1 }}</span>
                      <p class="cc-media-item__name">{{ item.name }}</p>
                    </div>
                    <p class="cc-media-item__url">{{ item.media_url }}</p>
                  </div>
                  <button type="button" class="cc-link-danger" @click="refImages.splice(idx, 1)">
                    {{ t('creazyCanvas.form.remove') }}
                  </button>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.maxVideos > 0" class="cc-media-panel">
              <div class="cc-media-panel__head">
                <div>
                  <label class="cc-label">
                    {{ t('creazyCanvas.form.refVideos') }}
                    <span class="cc-media-count">({{ refVideos.length }}/{{ mediaCaps.maxVideos }})</span>
                  </label>
                </div>
                <button
                  v-if="refVideos.length"
                  type="button"
                  class="cc-link-danger"
                  :disabled="!!uploadingMedia"
                  @click="clearRefVideos"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </div>
              <div
                class="cc-dropzone"
                :class="{ 'is-disabled': refVideos.length >= mediaCaps.maxVideos }"
                @dragover.prevent
                @drop="onDropMedia($event, 'refVideos')"
              >
                <div class="cc-dropzone__actions">
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
                        : mediaCaps.maxVideos >= 3
                          ? t('creazyCanvas.form.uploadMultiple')
                          : t('creazyCanvas.form.upload')
                    }}
                  </button>
                  <span class="cc-media-count">{{ mediaProgressText(refVideos.length, mediaCaps.maxVideos) }}</span>
                </div>
                <div class="cc-progress">
                  <div class="cc-progress__bar" :style="{ width: Math.min(100, (refVideos.length / mediaCaps.maxVideos) * 100) + '%' }" />
                </div>
                <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
                <p v-if="refVideos.length > 1" class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaReorderHint') }}</p>
              </div>
              <ul v-if="refVideos.length" class="cc-media-list">
                <li
                  v-for="(item, idx) in refVideos"
                  :key="'ref-vid-' + idx + '-' + item.media_url"
                  class="cc-media-item"
                  draggable="true"
                  @dragstart="onMediaDragStart('refVideos', idx)"
                  @dragover.prevent
                  @drop.prevent="onMediaDropReorder('refVideos', idx)"
                >
                  <div class="cc-media-item__badge cc-media-item__badge--vid">VID</div>
                  <div class="cc-media-item__body">
                    <div class="cc-media-item__row">
                      <span class="cc-media-token">@Video{{ idx + 1 }}</span>
                      <p class="cc-media-item__name">{{ item.name }}</p>
                    </div>
                    <p class="cc-media-item__url">
                      {{ item.media_url }}
                      <span v-if="item.duration_seconds"> · {{ item.duration_seconds }}s</span>
                    </p>
                  </div>
                  <button type="button" class="cc-link-danger" @click="refVideos.splice(idx, 1)">
                    {{ t('creazyCanvas.form.remove') }}
                  </button>
                </li>
              </ul>
            </div>

            <div v-if="mediaCaps.maxAudios > 0" class="cc-media-panel">
              <div class="cc-media-panel__head">
                <div>
                  <label class="cc-label">
                    {{ t('creazyCanvas.form.refAudios') }}
                    <span class="cc-media-count">({{ refAudios.length }}/{{ mediaCaps.maxAudios }})</span>
                  </label>
                </div>
                <button
                  v-if="refAudios.length"
                  type="button"
                  class="cc-link-danger"
                  :disabled="!!uploadingMedia"
                  @click="clearRefAudios"
                >
                  {{ t('creazyCanvas.form.clearAll') }}
                </button>
              </div>
              <div
                class="cc-dropzone"
                :class="{ 'is-disabled': refUploadsBlocked || refAudios.length >= mediaCaps.maxAudios }"
                @dragover.prevent
                @drop="onDropMedia($event, 'refAudios')"
              >
                <div class="cc-dropzone__actions">
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
                  <span class="cc-media-count">{{ mediaProgressText(refAudios.length, mediaCaps.maxAudios) }}</span>
                </div>
                <div class="cc-progress">
                  <div class="cc-progress__bar" :style="{ width: Math.min(100, (refAudios.length / mediaCaps.maxAudios) * 100) + '%' }" />
                </div>
                <p class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaDropHint') }}</p>
                <p v-if="refAudios.length > 1" class="cc-dropzone__hint">{{ t('creazyCanvas.form.mediaReorderHint') }}</p>
              </div>
              <ul v-if="refAudios.length" class="cc-media-list">
                <li
                  v-for="(item, idx) in refAudios"
                  :key="'ref-aud-' + idx + '-' + item.media_url"
                  class="cc-media-item"
                  draggable="true"
                  @dragstart="onMediaDragStart('refAudios', idx)"
                  @dragover.prevent
                  @drop.prevent="onMediaDropReorder('refAudios', idx)"
                >
                  <div class="cc-media-item__badge cc-media-item__badge--aud">AUD</div>
                  <div class="cc-media-item__body">
                    <div class="cc-media-item__row">
                      <span class="cc-media-token">@Audio{{ idx + 1 }}</span>
                      <p class="cc-media-item__name">{{ item.name }}</p>
                    </div>
                    <p class="cc-media-item__url">
                      {{ item.media_url }}
                      <span v-if="item.duration_seconds"> · {{ item.duration_seconds }}s</span>
                    </p>
                  </div>
                  <button type="button" class="cc-link-danger" @click="refAudios.splice(idx, 1)">
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
            <p v-if="videoRunningCount" class="cc-callout cc-callout--live">
              {{ t('creazyCanvas.tasks.runningCount', { n: videoRunningCount }) }}
            </p>
            <p v-if="videoStatus" class="cc-callout">
              {{ t('creazyCanvas.result.status') }}: {{ videoStatus }}
            </p>
            <p v-if="videoError" class="cc-callout cc-callout--bad">{{ videoError }}</p>
            <p v-if="videoSaveMessage" class="cc-callout cc-callout--ok">{{ videoSaveMessage }}</p>
          </div>
        </div>

        <div class="card cc-board-card cc-surface">
          <div class="cc-board-head">
            <div class="cc-board-head__main">
              <div class="cc-board-head__kicker">
                <span class="cc-board-head__kicker-dot" aria-hidden="true"></span>
                LIVE
              </div>
              <h2 class="cc-board-head__title">{{ t('creazyCanvas.tasks.title') }}</h2>
              <p class="cc-board-head__sub">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm cc-board-head__refresh" :disabled="loadingWorks" @click="() => loadWorks()">
              <Icon name="refresh" size="sm" class="mr-1.5" :class="loadingWorks ? 'animate-spin' : ''" />
              {{ t('creazyCanvas.works.refresh') }}
            </button>
          </div>
          <div class="cc-board-body">
            <div v-if="videoResultUrl" class="cc-latest cc-latest--video">
              <div class="cc-latest__head">
                <span class="cc-latest__badge">NOW</span>
                <p class="cc-latest__title">{{ t('creazyCanvas.tasks.latestPreview') }}</p>
              </div>
              <div class="cc-latest__stage">
                <video :src="videoResultUrl" controls playsinline preload="metadata" :muted="false" />
              </div>
              <p class="cc-latest__hint">{{ t('creazyCanvas.result.videoAudioHint') }}</p>
            </div>
            <div v-if="!videoTaskWorks.length" class="cc-empty-stage cc-empty-stage--compact">
              <div class="cc-empty-stage__icon cc-empty-stage__icon--soft" aria-hidden="true"><span class="cc-empty-stage__glyph">◇</span></div>
              <p class="cc-empty-stage__title">{{ t('creazyCanvas.tasks.empty') }}</p>
              <p class="cc-empty-stage__sub">{{ t('creazyCanvas.tasks.subtitle') }}</p>
            </div>
            <div v-else class="cc-task-list">
              <article v-for="work in videoTaskWorks" :key="work.id" class="cc-task-card" :class="taskCardClass(work)">
                <div class="cc-task-card__top">
                  <span class="cc-task-status" :class="taskStatusTone(work.status)">
                    <i class="cc-task-status__dot" :class="{ 'is-pulse': isActiveWorkStatus(work.status) }" />
                    {{ workStatusLabel(work.status) }}
                  </span>
                  <span class="cc-task-model">{{ work.public_model || '—' }}</span>
                  <span v-if="work.created_at" class="cc-task-time">{{ formatDateTime(work.created_at) }}</span>
                </div>
                <p class="cc-task-prompt">{{ work.prompt || ('#' + work.id) }}</p>
                <p v-if="isActiveWorkStatus(work.status)" class="cc-task-elapsed">{{ t('creazyCanvas.tasks.elapsed', { time: formatElapsed(work.created_at || work.updated_at) }) }}</p>
                <p v-if="workErrorText(work)" class="cc-task-error">{{ workErrorText(work) }}</p>
                <div class="cc-task-actions">
                  <button v-if="canPreviewWork(work)" type="button" class="btn btn-primary btn-sm" :disabled="workPreviewLoading[String(work.id)]" @click="openWorkPreview(work)">
                    {{ workPreviewLoading[String(work.id)] ? t('creazyCanvas.works.previewLoading') : t('creazyCanvas.works.preview') }}
                  </button>
                  <button v-if="canPreviewWork(work)" type="button" class="btn btn-secondary btn-sm" :disabled="downloadingWorkId === String(work.id)" @click="downloadWork(work)">
                    {{ t('creazyCanvas.works.download') }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" @click="reuseWork(work)">{{ t('creazyCanvas.works.reuse') }}</button>
                  <button type="button" class="btn btn-secondary btn-sm" @click="copyWorkPrompt(work)">{{ t('creazyCanvas.tasks.copyPrompt') }}</button>
                  <button v-if="['failed','error'].includes(String(work.status||'').toLowerCase())" type="button" class="btn btn-primary btn-sm" @click="retryWork(work)">{{ t('creazyCanvas.tasks.retry') }}</button>
                  <button v-if="isActiveWorkStatus(work.status) && !stoppedTrackIds[String(work.id)]" type="button" class="btn btn-secondary btn-sm" @click="stopLocalTrack(work)">{{ t('creazyCanvas.tasks.stopTrack') }}</button>
                </div>
              </article>
            </div>
            <div v-if="worksTotal > 0" class="cc-pagination cc-pagination--board">
              <span class="cc-pagination__meta">{{ t('creazyCanvas.works.pageInfo', { page: worksPage, pages: worksPages, total: worksTotal }) }}</span>
              <div class="cc-pagination__actions">
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks || worksPage <= 1" @click="goToWorksPrevPage($event)">{{ t('creazyCanvas.works.prevPage') }}</button>
                <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks || worksPage >= worksPages" @click="goToWorksNextPage($event)">{{ t('creazyCanvas.works.nextPage') }}</button>
                <label class="cc-pagination__jump">
                  <span class="sr-only">{{ t('creazyCanvas.works.pageJump') }}</span>
                  <input v-model="worksPageJumpInput" type="number" min="1" :max="worksPages" class="cc-pagination__input" :placeholder="t('creazyCanvas.works.pageJumpPlaceholder')" @keyup.enter="jumpWorksPageFromInput" />
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingWorks" @click="jumpWorksPageFromInput">{{ t('creazyCanvas.works.pageJump') }}</button>
                </label>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Works -->
      <section v-else class="space-y-4">
        <div class="card cc-works overflow-hidden">
          <div class="cc-works__head">
            <div class="cc-works__title-row">
              <div class="cc-works__title-block">
                <div class="cc-works__kicker">
                  <span class="cc-works__kicker-dot" aria-hidden="true"></span>
                  LIBRARY
                </div>
                <h2 class="cc-works__title">
                  {{ t('creazyCanvas.works.title') }}
                </h2>
                <p class="cc-works__sub">
                  <template v-if="!selectedKeyId">{{ t('creazyCanvas.works.selectKeyFirst') }}</template>
                  <template v-else>
                    {{ t('creazyCanvas.works.filteredByKey') }}
                    <span class="cc-works__keychip">
                      {{ selectedKeyLabel }}
                    </span>
                  </template>
                </p>
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm cc-works__refresh"
                :disabled="!selectedKeyId || loadingWorks"
                @click="() => loadWorks()"
              >
                <Icon name="refresh" size="sm" class="mr-1.5" :class="loadingWorks ? 'animate-spin' : ''" />
                {{ t('creazyCanvas.works.refresh') }}
              </button>
            </div>

            <div class="cc-filterbar" role="search">
              <label class="cc-filter">
                <span class="cc-filter__label">{{ t('creazyCanvas.works.filterKind') }}</span>
                <select
                  v-model="worksFilterKind"
                  class="input cc-filter__control"
                  :disabled="!selectedKeyId || loadingWorks"
                  @change="() => reloadWorksFromStart()"
                >
                  <option value="">{{ t('creazyCanvas.works.filterAll') }}</option>
                  <option value="image">{{ t('creazyCanvas.works.image') }}</option>
                  <option value="video">{{ t('creazyCanvas.works.video') }}</option>
                </select>
              </label>
              <label class="cc-filter">
                <span class="cc-filter__label">{{ t('creazyCanvas.works.filterStatus') }}</span>
                <select
                  v-model="worksFilterStatus"
                  class="input cc-filter__control"
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
              <label class="cc-filter">
                <span class="cc-filter__label">{{ t('creazyCanvas.works.filterModel') }}</span>
                <select
                  v-model="worksFilterModel"
                  class="input cc-filter__control"
                  :disabled="!selectedKeyId || loadingWorks"
                >
                  <option value="">{{ t('creazyCanvas.works.filterAll') }}</option>
                  <option v-for="m in worksModelOptions" :key="'wm-' + m" :value="m">{{ m }}</option>
                </select>
              </label>
              <label class="cc-filter cc-filter--grow">
                <span class="cc-filter__label">{{ t('creazyCanvas.works.filterQuery') }}</span>
                <input
                  v-model="worksFilterQuery"
                  type="search"
                  class="input cc-filter__control"
                  :placeholder="t('creazyCanvas.works.filterQueryPlaceholder')"
                  :disabled="!selectedKeyId || loadingWorks"
                />
              </label>
            </div>

            <div v-if="selectedKeyId && worksStatusSummary.total > 0" class="cc-status-strip">
              <span
                v-for="item in worksStatusSummary.items"
                :key="item.key"
                class="cc-status-chip"
                :class="item.chipClass"
              >
                <span class="cc-status-chip__dot" :class="item.dotClass" />
                <span class="cc-status-chip__label">{{ item.label }}</span>
                <span class="cc-status-chip__count">{{ item.count }}</span>
              </span>
            </div>
          </div>

          <div class="cc-works-body-pad">

            <div
              v-if="worksNeedSecretBanner"
              class="cc-banner cc-banner--warn"
            >
              {{ t('creazyCanvas.works.needSecretBanner') }}
            </div>

            <div v-if="!selectedKeyId" class="cc-empty-stage">
              <div class="cc-empty-stage__icon" aria-hidden="true">
                <Icon name="key" size="md" />
              </div>
              <p class="cc-empty-stage__title">{{ t('creazyCanvas.works.selectKeyFirst') }}</p>
              <p class="cc-empty-stage__sub">{{ t('creazyCanvas.works.selectKeyFirstHint') }}</p>
            </div>

            <div v-else-if="loadingWorks" class="cc-works-skeleton">
              <div v-for="i in 6" :key="i" class="cc-works-skeleton__card">
                <div class="cc-works-skeleton__media" />
                <div class="cc-works-skeleton__lines">
                  <i></i><i></i><i></i>
                </div>
              </div>
            </div>

            <div
              v-else-if="works.length === 0"
              class="cc-empty-stage"
            >
              <div class="cc-empty-stage__icon cc-empty-stage__icon--soft" aria-hidden="true">
                <span class="cc-empty-stage__glyph">◇</span>
              </div>
              <p class="cc-empty-stage__title">{{ t('creazyCanvas.works.emptyForKey') }}</p>
              <p class="cc-empty-stage__sub">{{ t('creazyCanvas.works.emptyForKeyHint') }}</p>
              <div class="cc-empty-guide">
                <p class="cc-empty-guide__title">{{ t('creazyCanvas.works.emptyGuideTitle') }}</p>
                <ol class="cc-empty-guide__list">
                  <li>1. {{ t('creazyCanvas.works.emptyGuide1') }}</li>
                  <li>2. {{ t('creazyCanvas.works.emptyGuide2') }}</li>
                  <li>3. {{ t('creazyCanvas.works.emptyGuide3') }}</li>
                </ol>
              </div>
            </div>

            <div
              v-else-if="filteredWorks.length === 0"
              class="cc-empty-stage cc-empty-stage--compact"
            >
              <p class="cc-empty-stage__title">{{ t('creazyCanvas.works.filterEmpty') }}</p>
            </div>

            <div v-else class="cc-works-body">
              <div class="cc-batch-bar">
                <div class="cc-batch-bar__left">
                  <button type="button" class="btn btn-secondary btn-sm" @click="selectAllWorksOnPage(true)">
                    {{ t('creazyCanvas.works.selectAllPage') }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedWorkIds.length" @click="selectAllWorksOnPage(false)">
                    {{ t('creazyCanvas.works.clearSelection') }}
                  </button>
                  <span v-if="selectedWorkIds.length" class="cc-batch-bar__count">{{ t('creazyCanvas.works.selectedCount', { n: selectedWorkIds.length }) }}</span>
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm cc-batch-bar__danger"
                  :disabled="!selectedWorkIds.length || loadingWorks"
                  @click="batchDeleteSelectedWorks"
                >
                  {{ t('creazyCanvas.works.batchDelete') }}
                </button>
              </div>

              <div class="cc-works-grid">
                <article
                  v-for="work in filteredWorks"
                  :key="String(work.id)"
                  :data-work-id="work.id"
                  class="cc-work-card group"
                  :class="workCardClass(work)"
                >
                  <div class="cc-work-media">
                    <label class="cc-work-check" @click.stop>
                      <input
                        type="checkbox"
                        :checked="selectedWorkIds.includes(Number(work.id))"
                        @change="onWorkSelectChange(work, $event)"
                      />
                    </label>

                    <span class="cc-work-status" :class="workStatusClass(work.status)">
                      <i class="cc-work-status__dot" :class="workStatusDotClass(work.status)" />
                      {{ workStatusLabel(work.status) }}
                    </span>
                    <span v-if="isExpired(work)" class="cc-work-expired">{{ t('creazyCanvas.works.expired') }}</span>

                    <div
                      v-if="isWorkCoverLoading(work)"
                      class="cc-cover-skeleton"
                      aria-hidden="true"
                    >
                      <div class="cc-cover-skeleton__shine" />
                      <span class="cc-cover-skeleton__label">{{ t('creazyCanvas.works.coverLoading') }}</span>
                    </div>

                    <button
                      v-if="workCoverUrl(work)"
                      type="button"
                      class="cc-work-cover"
                      :class="{ 'is-ready': isWorkCoverReady(work) }"
                      :title="t('creazyCanvas.works.preview')"
                      @click="openWorkPreview(work)"
                    >
                      <img
                        v-if="isImageWork(work) || workCoverIsImage(work)"
                        :src="workCoverUrl(work)"
                        alt=""
                        class="cc-work-cover__media"
                        @load="onCoverMediaReady(work, $event)"
                        @error="onCoverMediaError(work)"
                      />
                      <video
                        v-else
                        :src="workCoverVideoSrc(work)"
                        muted
                        playsinline
                        preload="metadata"
                        class="cc-work-cover__media"
                        @loadeddata="onCoverVideoLoaded($event, work)"
                        @error="onCoverMediaError(work)"
                      />
                      <span class="cc-work-cover__veil">
                        <span class="cc-work-cover__play">{{ t('creazyCanvas.works.preview') }}</span>
                      </span>
                    </button>

                    <button
                      v-else-if="canPreviewWork(work)"
                      type="button"
                      class="cc-work-cover cc-work-cover--pending"
                      :disabled="workPreviewLoading[String(work.id)]"
                      @click="openWorkPreview(work)"
                    >
                      <span class="cc-work-cover__pending-text">
                        {{
                          workPreviewLoading[String(work.id)]
                            ? t('creazyCanvas.works.coverLoading')
                            : t('creazyCanvas.works.preview')
                        }}
                      </span>
                    </button>

                    <div v-else class="cc-work-cover cc-work-cover--empty">
                      <span>{{ workCoverUnavailableReason(work) }}</span>
                    </div>

                    <div class="cc-work-media__corners" aria-hidden="true">
                      <i></i><i></i><i></i><i></i>
                    </div>
                  </div>

                  <div class="cc-work-body">
                    <p class="cc-work-prompt" :title="work.prompt || work.public_model || ('#' + work.id)">
                      {{ work.prompt || work.public_model || ('#' + work.id) }}
                    </p>

                    <div class="cc-work-meta">
                      <span class="cc-token-chip" :title="t('creazyCanvas.form.model')">
                        <em>{{ t('creazyCanvas.form.model') }}</em>
                        <strong>{{ work.public_model || '—' }}</strong>
                      </span>
                      <span v-if="work.created_at" class="cc-work-time">
                        {{ formatDateTime(work.created_at) }}
                      </span>
                    </div>

                    <div v-if="workErrorText(work)" class="cc-work-error">
                      <p :class="isErrorExpanded(work) ? '' : 'line-clamp-2'">{{ workErrorText(work) }}</p>
                      <div class="cc-work-error__actions">
                        <button type="button" @click="toggleErrorExpand(work)">
                          {{ isErrorExpanded(work) ? t('creazyCanvas.tasks.collapseError') : t('creazyCanvas.tasks.expandError') }}
                        </button>
                        <button type="button" @click="copyWorkError(work)">
                          {{ t('creazyCanvas.tasks.copyError') }}
                        </button>
                      </div>
                    </div>

                    <p v-if="isActiveWorkStatus(work.status) && work.created_at" class="cc-work-elapsed">
                      {{ t('creazyCanvas.tasks.elapsed', { time: formatElapsed(work.created_at) }) }}
                    </p>

                    <div class="cc-work-actions">
                      <button
                        v-if="canPreviewWork(work)"
                        type="button"
                        class="btn btn-primary btn-sm"
                        :disabled="workPreviewLoading[String(work.id)]"
                        @click="openWorkPreview(work)"
                      >
                        {{
                          workPreviewLoading[String(work.id)]
                            ? t('creazyCanvas.works.previewLoading')
                            : t('creazyCanvas.works.preview')
                        }}
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
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="downloadingWorkId === String(work.id) || isExpired(work) || work.status === 'failed'"
                        @click="downloadWork(work)"
                      >
                        {{ t('creazyCanvas.works.download') }}
                      </button>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
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
    <!-- Floating task tray for concurrent jobs -->
    <div
      v-if="showTaskTray"
      class="cc-task-tray"
      :class="{
        'cc-task-tray--collapsed': !taskTrayExpanded,
        'cc-task-tray--has-live': trayStatusCounts.running > 0,
      }"
      role="complementary"
      :aria-label="t('creazyCanvas.tasks.trayTitle')"
    >
      <div class="cc-task-tray__bar">
        <button
          type="button"
          class="cc-task-tray__toggle"
          :aria-expanded="taskTrayExpanded"
          @click="taskTrayExpanded = !taskTrayExpanded"
        >
          <span class="cc-task-tray__pulse" aria-hidden="true" />
          <span class="cc-task-tray__head">
            <span class="cc-task-tray__title-row">
              <span class="cc-task-tray__title">{{ t('creazyCanvas.tasks.trayTitle') }}</span>
              <span v-if="totalRunningJobs" class="cc-task-tray__badge">{{ totalRunningJobs }}</span>
              <span class="cc-task-tray__chev" :class="{ 'is-open': taskTrayExpanded }" aria-hidden="true">▾</span>
            </span>
            <span class="cc-task-tray__hint">{{ t('creazyCanvas.tasks.trayHint') }}</span>
          </span>
          <span class="cc-task-tray__counts" aria-hidden="true">
            <span v-if="trayStatusCounts.running" class="cc-task-tray__chip is-live">
              <i class="cc-task-tray__chip-dot is-pulse" />
              {{ t('creazyCanvas.tasks.trayRunning', { n: trayStatusCounts.running }) }}
            </span>
            <span v-if="trayStatusCounts.succeeded" class="cc-task-tray__chip is-ok">
              {{ t('creazyCanvas.tasks.traySucceeded', { n: trayStatusCounts.succeeded }) }}
            </span>
            <span v-if="trayStatusCounts.failed" class="cc-task-tray__chip is-bad">
              {{ t('creazyCanvas.tasks.trayFailed', { n: trayStatusCounts.failed }) }}
            </span>
          </span>
        </button>
        <div class="cc-task-tray__actions">
          <button type="button" class="cc-task-tray__link" @click="() => openTrayTaskBoard()">
            {{ t('creazyCanvas.tasks.open') }}
          </button>
          <button
            type="button"
            class="cc-task-tray__dismiss"
            :title="t('creazyCanvas.tasks.dismiss')"
            @click="taskTrayDismissed = true"
          >
            ×
          </button>
        </div>
      </div>
      <div v-if="taskTrayExpanded" class="cc-task-tray__body">
        <div v-if="trayWorks.length" class="cc-task-tray__list">
          <button
            v-for="work in trayWorks"
            :key="'tray-' + work.id"
            type="button"
            class="cc-task-tray__item"
            :class="trayItemClass(work)"
            :title="work.prompt || work.public_model || ('#' + work.id)"
            @click="() => openTrayTaskBoard(work)"
          >
            <span class="cc-task-tray__item-rail" aria-hidden="true" />
            <span class="cc-task-tray__item-main">
              <span class="cc-task-tray__item-top">
                <span class="cc-task-tray__status" :class="taskStatusTone(work.status)">
                  <i class="cc-task-tray__status-dot" :class="{ 'is-pulse': isActiveWorkStatus(work.status) }" />
                  {{ workStatusLabel(work.status) }}
                </span>
                <span class="cc-task-tray__kind" :data-kind="String(work.kind || 'other').toLowerCase()">
                  {{ trayKindLabel(work.kind) }}
                </span>
                <span class="cc-task-tray__model">{{ work.public_model || '—' }}</span>
                <span v-if="isActiveWorkStatus(work.status)" class="cc-task-tray__elapsed">
                  {{ formatElapsed(work.created_at || work.updated_at) }}
                </span>
                <span v-else-if="work.created_at" class="cc-task-tray__time">
                  {{ formatDateTime(work.created_at) }}
                </span>
              </span>
              <span class="cc-task-tray__prompt">{{ trayPromptText(work) }}</span>
              <span v-if="workErrorText(work)" class="cc-task-tray__error">{{ workErrorText(work) }}</span>
            </span>
          </button>
        </div>
        <p v-else class="cc-task-tray__empty">{{ t('creazyCanvas.tasks.empty') }}</p>
      </div>
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
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
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

type PromptScope = 'image' | 'video'
type MentionItem = {
  id: string
  token: string
  label: string
  kind: 'image' | 'video' | 'audio' | 'start' | 'end'
  kindLabel: string
  preview_url?: string
}

const imagePromptEl = ref<HTMLTextAreaElement | null>(null)
const videoPromptEl = ref<HTMLTextAreaElement | null>(null)
const mention = reactive({
  open: false,
  scope: 'image' as PromptScope,
  query: '',
  index: 0,
  start: 0,
  end: 0,
})
let mentionBlurTimer: ReturnType<typeof setTimeout> | null = null

/** Platform numbering: refs -> start frame -> end frame; video/audio each from 1. */
const startFrameToken = computed(() => {
  if (!startFrame.value) return ''
  return `@Image${refImages.value.length + 1}`
})
const endFrameToken = computed(() => {
  if (!endFrame.value) return ''
  const n = refImages.value.length + (startFrame.value ? 1 : 0) + 1
  return `@Image${n}`
})

function buildImageMentionItems(): MentionItem[] {
  return imageRefs.value.map((item, i) => {
    const n = i + 1
    return {
      id: `image-ref-${i}`,
      token: `@Image${n}`,
      label: t('creazyCanvas.form.mentionRefImage', { n }),
      kind: 'image' as const,
      kindLabel: 'IMG',
      preview_url: item.preview_url || item.media_url,
    }
  })
}

function buildVideoMentionItems(): MentionItem[] {
  const items: MentionItem[] = []
  let imgN = 1
  refImages.value.forEach((item, i) => {
    items.push({
      id: `video-ref-img-${i}`,
      token: `@Image${imgN}`,
      label: t('creazyCanvas.form.mentionRefImage', { n: imgN }),
      kind: 'image',
      kindLabel: 'IMG',
      preview_url: item.preview_url || item.media_url,
    })
    imgN += 1
  })
  if (startFrame.value) {
    items.push({
      id: 'video-start-frame',
      token: `@Image${imgN}`,
      label: t('creazyCanvas.form.mentionStartFrame'),
      kind: 'start',
      kindLabel: 'S',
      preview_url: startFrame.value.preview_url || startFrame.value.media_url,
    })
    imgN += 1
  }
  if (endFrame.value) {
    items.push({
      id: 'video-end-frame',
      token: `@Image${imgN}`,
      label: t('creazyCanvas.form.mentionEndFrame'),
      kind: 'end',
      kindLabel: 'E',
      preview_url: endFrame.value.preview_url || endFrame.value.media_url,
    })
  }
  refVideos.value.forEach((item, i) => {
    const n = i + 1
    items.push({
      id: `video-ref-vid-${i}`,
      token: `@Video${n}`,
      label: t('creazyCanvas.form.mentionRefVideo', { n }),
      kind: 'video',
      kindLabel: 'VID',
      preview_url: item.preview_url,
    })
  })
  refAudios.value.forEach((item, i) => {
    const n = i + 1
    items.push({
      id: `video-ref-aud-${i}`,
      token: `@Audio${n}`,
      label: t('creazyCanvas.form.mentionRefAudio', { n }),
      kind: 'audio',
      kindLabel: 'AUD',
      preview_url: item.preview_url,
    })
  })
  return items
}

const mentionAllItems = computed(() =>
  mention.scope === 'image' ? buildImageMentionItems() : buildVideoMentionItems(),
)

const mentionFilteredItems = computed(() => {
  const q = mention.query.trim().toLowerCase()
  const all = mentionAllItems.value
  if (!q) return all
  return all.filter((it) => {
    return (
      it.token.toLowerCase().includes(q) ||
      it.label.toLowerCase().includes(q) ||
      it.kind.toLowerCase().includes(q) ||
      it.kindLabel.toLowerCase().includes(q)
    )
  })
})

function closeMentionMenu() {
  mention.open = false
  mention.query = ''
  mention.index = 0
}

function onPromptInput(scope: PromptScope, event: Event) {
  const el = event.target as HTMLTextAreaElement | null
  if (!el) return
  detectMentionAtCaret(scope, el)
}

function detectMentionAtCaret(scope: PromptScope, el: HTMLTextAreaElement) {
  const value = el.value
  const caret = el.selectionStart ?? 0
  const before = value.slice(0, caret)
  const m = before.match(/@([A-Za-z0-9_]*)$/)
  if (!m) {
    closeMentionMenu()
    return
  }
  mention.open = true
  mention.scope = scope
  mention.query = m[1] || ''
  mention.start = caret - m[0].length
  mention.end = caret
  if (mention.index >= mentionFilteredItems.value.length) {
    mention.index = 0
  }
}

function insertMention(item: MentionItem) {
  const scope = mention.scope
  const form = scope === 'image' ? imageForm : videoForm
  const el = scope === 'image' ? imagePromptEl.value : videoPromptEl.value
  const value = form.prompt
  const start = mention.start
  const end = mention.end
  const insert = `${item.token} `
  form.prompt = value.slice(0, start) + insert + value.slice(end)
  closeMentionMenu()
  void nextTick(() => {
    if (!el) return
    const pos = start + insert.length
    el.focus()
    el.setSelectionRange(pos, pos)
  })
}

function onPromptKeydown(scope: PromptScope, event: KeyboardEvent) {
  if (!mention.open || mention.scope !== scope) return
  const items = mentionFilteredItems.value
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    if (!items.length) return
    mention.index = (mention.index + 1) % items.length
    return
  }
  if (event.key === 'ArrowUp') {
    event.preventDefault()
    if (!items.length) return
    mention.index = (mention.index - 1 + items.length) % items.length
    return
  }
  if (event.key === 'Enter' || event.key === 'Tab') {
    if (items[mention.index]) {
      event.preventDefault()
      insertMention(items[mention.index])
    }
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    closeMentionMenu()
  }
}

function onPromptBlur() {
  if (mentionBlurTimer) clearTimeout(mentionBlurTimer)
  mentionBlurTimer = setTimeout(() => {
    closeMentionMenu()
    mentionBlurTimer = null
  }, 160)
}


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
const workCoverReady = reactive<Record<string, boolean>>({})
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
/** Skip model-change media wipe while programmatically reusing/retrying a work. */
const suppressVideoMediaReset = ref(false)
const suppressImageMediaTrim = ref(false)
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
    if (suppressImageMediaTrim.value) return
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
  if (resolvingKeySecret.value) return 'is-warn'
  if (hasKeySecret.value) return 'is-ready'
  return 'is-bad'
})

const keyReadyDotClass = computed(() => {
  if (resolvingKeySecret.value) return 'is-warn is-pulse'
  if (hasKeySecret.value) return 'is-ready'
  return 'is-bad'
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
  const abs = Math.abs(v)
  let raw = ''
  if (abs >= 100) raw = v.toFixed(2)
  else if (abs >= 1) raw = v.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
  else raw = v.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
  // 大额余额加千分位，提升可读性（如 999998.55 -> 999,998.55）
  const neg = raw.startsWith('-')
  const body = neg ? raw.slice(1) : raw
  const [intPart, decPart] = body.split('.')
  const withSep = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return (neg ? '-' : '') + withSep + (decPart != null ? '.' + decPart : '')
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
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) return 'is-ok'
  if (['failed', 'error', 'expired'].includes(s)) return 'is-bad'
  if (['running', 'pending', 'processing'].includes(s)) return 'is-live is-pulse'
  if (['queued'].includes(s)) return 'is-live'
  if (['created'].includes(s)) return 'is-new'
  if (['canceled', 'cancelled'].includes(s)) return 'is-idle'
  return 'is-idle'
}

function workStatusChipClass(status?: string) {
  return taskStatusTone(status)
}

function taskStatusTone(status?: string) {
  const s = (status || '').toLowerCase()
  if (['succeeded', 'completed', 'success', 'done'].includes(s)) return 'is-ok'
  if (['failed', 'error', 'expired'].includes(s)) return 'is-bad'
  if (['running', 'pending', 'processing', 'queued'].includes(s)) return 'is-live'
  if (['created'].includes(s)) return 'is-new'
  return 'is-idle'
}

function taskCardClass(work: CreazyWork) {
  const s = (work.status || '').toLowerCase()
  const classes: string[] = []
  if (isExpired(work) || s === 'expired') classes.push('is-bad')
  else if (['succeeded', 'completed', 'success', 'done'].includes(s)) classes.push('is-ok')
  else if (['failed', 'error'].includes(s)) classes.push('is-bad')
  else if (['running', 'pending', 'processing', 'queued', 'created'].includes(s) || isActiveWorkStatus(s)) classes.push('is-live')
  const id = String(work.id || '')
  if (id && flashWorkIds[id] && flashWorkIds[id] > nowTick.value) classes.push('is-flash')
  if (focusWorkId.value && Number(work.id) === Number(focusWorkId.value)) classes.push('is-focus')
  if (id && stoppedTrackIds[id]) classes.push('is-dim')
  return classes.join(' ')
}

function workCardClass(work: CreazyWork) {
  const s = (work.status || '').toLowerCase()
  const classes: string[] = []
  if (isExpired(work) || s === 'expired') {
    classes.push('cc-work-card--expired')
  } else if (['succeeded', 'completed', 'success', 'done'].includes(s)) {
    classes.push('cc-work-card--ok')
  } else if (['failed', 'error'].includes(s)) {
    classes.push('cc-work-card--bad')
  } else if (['running', 'pending', 'processing', 'queued', 'created'].includes(s) || isActiveWorkStatus(s)) {
    classes.push('cc-work-card--live')
  } else {
    classes.push('cc-work-card--idle')
  }
  const id = String(work.id || '')
  if (id && flashWorkIds[id] && flashWorkIds[id] > nowTick.value) {
    classes.push('cc-work-card--flash')
  }
  if (focusWorkId.value && Number(work.id) === Number(focusWorkId.value)) {
    classes.push('cc-work-card--focus')
  }
  if (id && stoppedTrackIds[id]) {
    classes.push('cc-work-card--dim')
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

function trayKindLabel(kind?: string | null): string {
  const k = String(kind || '').toLowerCase()
  if (k === 'video') return 'VID'
  if (k === 'image' || k === 'img') return 'IMG'
  if (k === 'audio') return 'AUD'
  return k ? k.slice(0, 3).toUpperCase() : 'JOB'
}

function trayPromptText(work: CreazyWork): string {
  const raw = String(work.prompt || '').replace(/\s+/g, ' ').trim()
  if (raw) return raw
  return work.public_model ? String(work.public_model) : `#${work.id}`
}

function trayItemClass(work: CreazyWork): string {
  return [taskStatusTone(work.status), isActiveWorkStatus(work.status) ? 'is-active' : '']
    .filter(Boolean)
    .join(' ')
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
  // Ensure restored media survives any deferred watchers before auto-submit.
  await nextTick()
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

function onCoverVideoLoaded(ev: Event, work?: CreazyWork) {
  const el = ev.target as HTMLVideoElement | null
  if (!el) return
  try {
    if (el.readyState >= 1 && el.currentTime < 0.05) {
      el.currentTime = 0.1
    }
  } catch {
    // ignore seek failures (some blobs disallow seek until more data)
  }
  if (work) onCoverMediaReady(work, ev)
}

function onCoverMediaReady(work: CreazyWork, _ev?: Event) {
  workCoverReady[String(work.id)] = true
}

function onCoverMediaError(work: CreazyWork) {
  workCoverReady[String(work.id)] = false
}

function isWorkCoverReady(work: CreazyWork): boolean {
  const id = String(work.id)
  if (!workCoverUrl(work)) return false
  return Boolean(workCoverReady[id])
}

function isWorkCoverLoading(work: CreazyWork): boolean {
  if (!canPreviewWork(work)) return false
  const id = String(work.id)
  if (workPreviewLoading[id]) return true
  if (workCoverUrl(work) && !workCoverReady[id]) return true
  return false
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
  workCoverReady[id] = false
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
    .slice(0, 8)
  const targets = [...light, ...heavyVideo]
  // Sequential-ish batches to reduce memory spikes on large MP4s.
  const batchSize = 3
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
    suppressVideoMediaReset.value = true
    try {
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
      const applyVideoAudioFlag = () => {
        if (mediaCaps.value.forceGeneratedAudio) {
          videoForm.generateAudio = true
          return
        }
        if (params.generate_audio != null) {
          videoForm.generateAudio = Boolean(params.generate_audio) && mediaCaps.value.allowGeneratedAudio
        } else {
          videoForm.generateAudio = Boolean(mediaCaps.value.allowGeneratedAudio)
        }
      }
      applyVideoAudioFlag()

      const applyVideoMediaFromParams = () => {
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

        // Clear first so re-apply is deterministic after model-change watchers.
        startFrame.value = null
        endFrame.value = null
        startFrameUrlInput.value = ''
        refImages.value = []
        refVideos.value = []
        refAudios.value = []

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
        const rImgs = pickStringListParam(params, 'ref_images', 'reference_images', 'image_refs')
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
        return { mediaRestored, mediaSkipped, hadAnyMediaKey }
      }

      let mediaStats = applyVideoMediaFromParams()
      // Let deferred model watchers run while suppress is on, then re-apply media/audio.
      await nextTick()
      applyVideoAudioFlag()
      mediaStats = applyVideoMediaFromParams()

      if (mediaStats.mediaSkipped) {
        notes.push(t('creazyCanvas.form.reuseMediaSkipped'))
        partial = true
      } else if (!mediaStats.hadAnyMediaKey && !mediaStats.mediaRestored) {
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
    } finally {
      await nextTick()
      suppressVideoMediaReset.value = false
    }
  } else {
    imageError.value = ''
    imageSaveMessage.value = ''
    suppressImageMediaTrim.value = true
    try {
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

      const applyImageMediaFromParams = () => {
        const imgRefs = pickStringListParam(params, 'image_refs', 'ref_images', 'reference_images', 'images')
        if (imgRefs.length) {
          imageRefs.value = imgRefs.map((u, i) => ({ name: `image-ref-${i + 1}`, media_url: u, preview_url: u }))
          return imgRefs.length
        }
        return 0
      }

      let restored = applyImageMediaFromParams()
      await nextTick()
      restored = applyImageMediaFromParams()

      if (!restored && (params.reference_count || params.edit)) {
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
    } finally {
      await nextTick()
      suppressImageMediaTrim.value = false
    }
  }
}

watch(
  () => videoForm.model,
  () => {
    if (!suppressVideoMediaReset.value) {
      resetVideoMedia()
    }
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
/* =========================================================
   Creazy Canvas / Stage Console
   Media-first workbench: cool graphite, cyan signal, stage bezels.
   ========================================================= */

.cc-shell {
  --cc-ink: #0f172a;
  --cc-ink-soft: #1e293b;
  --cc-muted: #64748b;
  --cc-faint: #94a3b8;
  --cc-line: #e2e8f0;
  --cc-line-2: #cbd5e1;
  --cc-paper: #eef2f6;
  --cc-surface: #ffffff;
  --cc-surface-2: #f8fafc;
  --cc-surface-3: #f1f5f9;
  --cc-stage: #0b1220;
  --cc-stage-2: #111827;
  --cc-accent: #1d4ed8;
  --cc-accent-2: #2563eb;
  --cc-accent-bright: #60a5fa;
  --cc-accent-soft: #dbeafe;
  --cc-ok: #059669;
  --cc-ok-soft: #d1fae5;
  --cc-warn: #d97706;
  --cc-warn-soft: #fef3c7;
  --cc-bad: #dc2626;
  --cc-bad-soft: #fee2e2;
  --cc-live: #ea580c;
  --cc-live-soft: #ffedd5;
  --cc-radius: 16px;
  --cc-radius-sm: 12px;
  --cc-shadow: 0 1px 0 rgb(15 23 42 / 0.04), 0 10px 30px -18px rgb(15 23 42 / 0.28);
  --cc-shadow-lg: 0 2px 4px rgb(15 23 42 / 0.05), 0 24px 48px -28px rgb(15 23 42 / 0.35);
  --cc-font: "IBM Plex Sans", "Source Sans 3", "Segoe UI", "PingFang SC", "Microsoft YaHei UI", system-ui, sans-serif;
  --cc-display: "Outfit", "Space Grotesk", "Segoe UI", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --cc-mono: "JetBrains Mono", ui-monospace, "Cascadia Mono", Consolas, monospace;
  position: relative;
  margin: 0 auto;
  max-width: 84rem;
  padding: 0.35rem 0 3.25rem;
  display: flex;
  flex-direction: column;
  gap: 1.05rem;
  color: var(--cc-ink);
  font-family: var(--cc-font);
}
.cc-shell::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: -1;
  background:
    radial-gradient(58rem 28rem at 8% -8%, rgb(37 99 235 / 0.08), transparent 58%),
    radial-gradient(42rem 24rem at 96% 0%, rgb(180 83 9 / 0.06), transparent 52%),
    linear-gradient(180deg, #e8eef7 0%, #dfe7f2 42%, #eef3f8 100%);
}
:global(.dark) .cc-shell::before {
  background:
    radial-gradient(58rem 28rem at 8% -8%, rgb(45 212 191 / 0.08), transparent 58%),
    radial-gradient(42rem 24rem at 96% 0%, rgb(34 211 238 / 0.06), transparent 52%),
    linear-gradient(180deg, #070b14 0%, #0a101b 46%, #0b1220 100%);
}


:global(.dark) .cc-shell {
  --cc-ink: #e8eef8;
  --cc-ink-soft: #cbd5e1;
  --cc-muted: #94a3b8;
  --cc-faint: #64748b;
  --cc-line: #1e293b;
  --cc-line-2: #334155;
  --cc-paper: #0b1220;
  --cc-surface: #0f172a;
  --cc-surface-2: #111c2e;
  --cc-surface-3: #162033;
  --cc-stage: #05080f;
  --cc-stage-2: #0b1220;
  --cc-accent: #a5b4fc;
  --cc-accent-2: #60a5fa;
  --cc-accent-bright: #bfdbfe;
  --cc-accent-soft: #1e3a8a;
  --cc-ok: #34d399;
  --cc-ok-soft: #064e3b;
  --cc-warn: #fbbf24;
  --cc-warn-soft: #422006;
  --cc-bad: #f87171;
  --cc-bad-soft: #450a0a;
  --cc-live: #fb923c;
  --cc-live-soft: #431407;
  --cc-shadow: 0 1px 0 rgb(255 255 255 / 0.03), 0 16px 36px -22px rgb(0 0 0 / 0.7);
  --cc-shadow-lg: 0 2px 6px rgb(0 0 0 / 0.4), 0 28px 56px -28px rgb(0 0 0 / 0.75);
}

/* Top bar */
.cc-topbar {
  position: relative;
  border: 1px solid rgba(11, 18, 32, 0.45);
  border-radius: calc(var(--cc-radius) + 2px);
  background:
    radial-gradient(120% 140% at 100% 0%, rgba(37, 99, 235, 0.28), transparent 45%),
    radial-gradient(90% 120% at 0% 100%, rgba(180, 83, 9, 0.16), transparent 42%),
    linear-gradient(135deg, #101a2b 0%, #15233a 48%, #0f1728 100%);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.06) inset,
    0 18px 36px -28px rgba(11, 18, 32, 0.75);
  overflow: hidden;
  color: #e2e8f0;
}
.cc-topbar::before {
  content: "";
  position: absolute;
  inset: 0 auto 0 0;
  width: 5px;
  background: linear-gradient(180deg, #60a5fa, #2563eb 45%, #b45309);
}
:global(.dark) .cc-topbar {
  background:
    radial-gradient(120% 140% at 100% 0%, rgba(37, 99, 235, 0.22), transparent 45%),
    linear-gradient(135deg, #0c1422 0%, #121c2e 100%);
}
/* forge topbar text */
.cc-topbar .cc-eyebrow { color: #93c5fd !important; }
.cc-topbar .cc-topbar__title { color: #f8fafc !important; }
.cc-topbar .cc-topbar__sub { color: #9fb0c8 !important; }
.cc-topbar .cc-topbar__key-label { color: #bfdbfe !important; }
.cc-topbar .cc-topbar__hint,
.cc-topbar .cc-topbar__empty { color: #8fa0b8 !important; }
.cc-topbar .cc-topbar__balance {
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.92), rgba(2, 6, 23, 0.88)) !important;
  border: 1px solid rgba(250, 204, 21, 0.45) !important;
  box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.35), 0 8px 18px rgba(2, 6, 23, 0.28) !important;
  color: #f8fafc !important;
}
.cc-topbar .cc-topbar__balance-label,
.cc-topbar .cc-topbar__balance em {
  color: #fde68a !important;
  font-weight: 700 !important;
  letter-spacing: 0.04em !important;
}
.cc-topbar .cc-topbar__balance-value,
.cc-topbar .cc-topbar__balance strong {
  color: #ffffff !important;
  font-weight: 800 !important;
  text-shadow: 0 1px 0 rgba(0, 0, 0, 0.35) !important;
}
.cc-topbar .cc-topbar__balance-currency {
  color: #facc15 !important;
  margin-right: 0.08em;
}
.cc-topbar .cc-topbar__select,
.cc-topbar select.input {
  background: rgba(255,255,255,0.1) !important;
  border-color: rgba(147,197,253,0.35) !important;
  color: #f8fafc !important;
  font-weight: 600 !important;
}
.cc-topbar .cc-topbar__select option {
  color: #0b1220;
  background: #fff;
}
.cc-topbar .cc-topbar__key {
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(148,163,184,0.16);
  border-radius: 14px;
  padding: 12px 14px;
}
.cc-topbar__main {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(17rem, 0.95fr);
  gap: 1.1rem 1.6rem;
  padding: 1.15rem 1.25rem 1.15rem 1.35rem;
  align-items: center;
}
@media (max-width: 860px) {
  .cc-topbar__main { grid-template-columns: 1fr; }
}
.cc-topbar__brand {
  display: flex;
  align-items: flex-start;
  gap: 0.9rem;
  min-width: 0;
}
.cc-mark {
  position: relative;
  width: 2.9rem;
  height: 2.9rem;
  flex: 0 0 auto;
  border-radius: 0.95rem;
  background:
    radial-gradient(circle at 30% 25%, rgb(45 212 191 / 0.35), transparent 48%),
    linear-gradient(160deg, #132033 0%, #0a101b 55%, #071018 100%);
  box-shadow:
    inset 0 0 0 1px rgb(255 255 255 / 0.1),
    0 0 0 1px rgb(15 23 42 / 0.08),
    0 14px 28px -16px rgb(15 118 110 / 0.75);
}

.cc-mark__frame {
  position: absolute;
  inset: 0.45rem;
  border: 1.5px solid rgb(34 211 238 / 0.55);
  border-radius: 0.35rem;
}
.cc-mark__frame::before,
.cc-mark__frame::after {
  content: "";
  position: absolute;
  width: 0.45rem;
  height: 0.45rem;
  border: 1.5px solid var(--cc-accent-bright);
}
.cc-mark__frame::before { top: -1px; left: -1px; border-right: 0; border-bottom: 0; }
.cc-mark__frame::after { right: -1px; bottom: -1px; border-left: 0; border-top: 0; }
.cc-mark__beam {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 58%;
  height: 2px;
  transform: translate(-50%, -50%) rotate(-18deg);
  background: linear-gradient(90deg, transparent, var(--cc-accent-bright), transparent);
  box-shadow: 0 0 12px rgb(34 211 238 / 0.55);
}
.cc-eyebrow {
  margin: 0 0 0.2rem;
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--cc-accent-2);
}
.cc-topbar__titles { min-width: 0; }
.cc-topbar__title {
  margin: 0;
  font-family: var(--cc-display);
  font-size: clamp(1.35rem, 2vw, 1.7rem);
  font-weight: 750;
  letter-spacing: -0.03em;
  line-height: 1.15;
  color: var(--cc-ink);
}
.cc-topbar__sub {
  margin: 0.4rem 0 0;
  max-width: 38rem;
  font-size: 0.875rem;
  line-height: 1.55;
  color: var(--cc-muted);
}
.cc-topbar__key {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  padding: 0.85rem 0.95rem;
  border-radius: var(--cc-radius-sm);
  border: 1px solid var(--cc-line);
  background: linear-gradient(180deg, color-mix(in srgb, var(--cc-surface-2) 80%, #fff), var(--cc-surface-2));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.55);
}
:global(.dark) .cc-topbar__key { box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.03); }
.cc-topbar__key-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.cc-topbar__key-label {
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--cc-muted);
}
.cc-topbar__select {
  width: 100%;
  min-height: 2.55rem !important;
  border-radius: 0.75rem !important;
  border-color: var(--cc-line-2) !important;
  background: var(--cc-surface) !important;
  font-size: 0.875rem !important;
  font-weight: 600 !important;
}
.cc-topbar__select:focus {
  border-color: var(--cc-accent) !important;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent) 20%, transparent) !important;
}
.cc-topbar__empty { margin: 0; font-size: 0.78rem; color: var(--cc-muted); }
.cc-topbar__meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem 0.7rem; }
.cc-topbar__balance {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.34rem 0.7rem;
  border-radius: 999px;
  background: linear-gradient(180deg, #0f172a, #111827);
  border: 1px solid rgba(250, 204, 21, 0.42);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.18);
  font-size: 0.8rem;
  color: #f8fafc;
  max-width: 100%;
}
.cc-topbar__balance-label,
.cc-topbar__balance em {
  font-style: normal;
  color: #fde68a;
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.03em;
  white-space: nowrap;
}
.cc-topbar__balance-value,
.cc-topbar__balance strong {
  display: inline-flex;
  align-items: baseline;
  font-family: var(--cc-mono);
  font-weight: 800;
  color: #ffffff;
  font-size: 0.98rem;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
  white-space: nowrap;
}
.cc-topbar__balance-currency {
  color: #facc15;
  font-size: 0.82em;
  font-weight: 800;
  margin-right: 0.05em;
}
.cc-topbar__hint { margin: 0; flex: 1 1 12rem; font-size: 0.72rem; line-height: 1.4; color: var(--cc-faint); }

.cc-pill {
  display: inline-flex; align-items: center; gap: 0.32rem;
  padding: 0.18rem 0.55rem; border-radius: 999px; border: 1px solid var(--cc-line);
  background: var(--cc-surface); color: var(--cc-muted); font-size: 0.72rem; font-weight: 700;
}
.cc-pill__dot { width: 0.4rem; height: 0.4rem; border-radius: 999px; background: currentColor; }
.cc-pill.is-ready { color: var(--cc-ok); background: color-mix(in srgb, var(--cc-ok-soft) 75%, var(--cc-surface)); border-color: color-mix(in srgb, var(--cc-ok) 30%, var(--cc-line)); }
.cc-pill.is-warn { color: var(--cc-warn); background: color-mix(in srgb, var(--cc-warn-soft) 75%, var(--cc-surface)); border-color: color-mix(in srgb, var(--cc-warn) 30%, var(--cc-line)); }
.cc-pill.is-bad { color: var(--cc-bad); background: color-mix(in srgb, var(--cc-bad-soft) 75%, var(--cc-surface)); border-color: color-mix(in srgb, var(--cc-bad) 30%, var(--cc-line)); }
.cc-pill__dot.is-ready { color: var(--cc-ok); }
.cc-pill__dot.is-warn { color: var(--cc-warn); }
.cc-pill__dot.is-bad { color: var(--cc-bad); }
.cc-pill__dot.is-pulse { animation: cc-pulse 1.4s ease-out infinite; }
@keyframes cc-pulse {
  0% { box-shadow: 0 0 0 0 color-mix(in srgb, currentColor 40%, transparent); }
  70% { box-shadow: 0 0 0 0.4rem transparent; }
  100% { box-shadow: 0 0 0 0 transparent; }
}

/* Tabs */
.cc-tabs {
  display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0.3rem; padding: 0.3rem;
  border: 1px solid var(--cc-line); border-radius: calc(var(--cc-radius) + 2px);
  background: var(--cc-surface-2); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.45);
}
:global(.dark) .cc-tabs { box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.03); }
.cc-tab {
  appearance: none; border: 0; background: transparent; color: var(--cc-muted); font: inherit;
  font-size: 0.92rem; font-weight: 650; padding: 0.7rem 0.8rem; border-radius: var(--cc-radius-sm);
  cursor: pointer; transition: color 0.15s ease, background 0.15s ease, box-shadow 0.15s ease;
}
.cc-tab:hover { color: var(--cc-ink); background: color-mix(in srgb, var(--cc-surface) 85%, transparent); }
.cc-tab--active {
  color: var(--cc-ink); background: var(--cc-surface);
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.06), 0 0 0 1px color-mix(in srgb, var(--cc-accent) 18%, transparent), inset 0 -2px 0 0 var(--cc-accent);
}

/* Surfaces / form */
.cc-surface, .cc-form-card, .cc-board-card {
  border: 1px solid var(--cc-line) !important;
  border-radius: calc(var(--cc-radius) + 2px) !important;
  background: var(--cc-surface) !important;
  box-shadow: var(--cc-shadow) !important;
}
.cc-form-card { position: relative; overflow: hidden; }
.cc-form-card::before {
  content: ""; position: absolute; left: 0; top: 0; bottom: 0; width: 3px;
  background: linear-gradient(180deg, var(--cc-accent-bright), var(--cc-accent) 55%, #0369a1);
}
.cc-label, .cc-label-row {
  display: flex; align-items: center; justify-content: space-between; gap: 0.5rem;
  margin-bottom: 0.42rem; font-size: 0.8rem; font-weight: 700; color: var(--cc-ink);
}
.cc-label-row { width: 100%; }
.cc-field { display: flex; flex-direction: column; min-width: 0; }
.cc-field--error .cc-control, .cc-field--error .cc-textarea, .cc-input--error {
  border-color: var(--cc-bad) !important;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-bad) 16%, transparent) !important;
}
.cc-field__error { margin: 0.35rem 0 0; font-size: 0.76rem; color: var(--cc-bad); }
.cc-control, .cc-params-grid .cc-control, select.cc-control, input.cc-control {
  width: 100%; min-height: 2.55rem; height: 2.55rem; box-sizing: border-box;
  border-radius: 0.8rem !important; border: 1px solid var(--cc-line-2) !important;
  background: var(--cc-surface) !important; color: var(--cc-ink) !important;
  font-size: 0.875rem !important; line-height: 1.25 !important; padding: 0.45rem 0.8rem !important;
}
.cc-control:focus, .cc-textarea:focus {
  outline: none; border-color: var(--cc-accent) !important;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent) 18%, transparent);
}
.cc-params-grid {
  align-items: stretch;
  display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0.9rem 0.95rem; align-items: start;
}
@media (min-width: 640px) { .cc-params-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
.cc-params-grid > * { min-width: 0; }
.cc-textarea {
  width: 100%; min-height: 8.5rem; resize: vertical; border-radius: 0.95rem !important;
  border: 1px solid var(--cc-line-2) !important; background: var(--cc-surface) !important;
  color: var(--cc-ink) !important; font-size: 0.94rem !important; line-height: 1.55 !important;
  padding: 0.85rem 0.95rem !important; font-family: var(--cc-font) !important;
}
.cc-prompt-wrap { position: relative; }
.cc-prompt-hint { margin: 0.4rem 0 0; font-size: 0.75rem; color: var(--cc-faint); }
.cc-size-current { margin-top: 0.3rem; font-family: var(--cc-mono); font-size: 0.74rem; color: var(--cc-accent); }
.cc-panel {
  border: 1px solid var(--cc-line); border-radius: var(--cc-radius); background: var(--cc-surface-2); padding: 0.95rem 1rem;
}
.cc-panel__head { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; margin-bottom: 0.8rem; }
.cc-panel__badge {
  display: inline-flex; align-items: center; padding: 0.18rem 0.55rem; border-radius: 999px;
  font-size: 0.72rem; font-weight: 750; color: var(--cc-accent);
  background: color-mix(in srgb, var(--cc-accent-soft) 80%, var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-accent) 22%, var(--cc-line));
}
.cc-panel__meta { font-size: 0.76rem; color: var(--cc-muted); }
.cc-chip-row, .cc-cap-row { display: flex; flex-wrap: wrap; gap: 0.4rem; }
.cc-chip, .cc-cap-chip, .cc-token-chip {
  display: inline-flex; align-items: center; gap: 0.28rem; padding: 0.34rem 0.7rem; border-radius: 999px;
  border: 1px solid var(--cc-line); background: var(--cc-surface); color: var(--cc-muted);
  font-size: 0.78rem; font-weight: 650; cursor: pointer; transition: 0.12s ease;
}
.cc-chip:hover, .cc-cap-chip:hover { border-color: var(--cc-line-2); color: var(--cc-ink); }
.cc-chip--active, .cc-chip.is-active {
  color: var(--cc-accent); border-color: color-mix(in srgb, var(--cc-accent) 40%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-accent-soft) 80%, var(--cc-surface));
}
.cc-cap-chip { cursor: default; font-size: 0.72rem; background: var(--cc-surface-3); }
.cc-token-chip { cursor: default; font-family: var(--cc-mono); font-size: 0.7rem; padding: 0.28rem 0.55rem; }
.cc-token-chip em {
  font-style: normal; font-family: var(--cc-font); font-size: 0.65rem; font-weight: 700;
  letter-spacing: 0.04em; text-transform: uppercase; color: var(--cc-faint);
}
.cc-token-chip strong {
  font-weight: 700; color: var(--cc-ink); max-width: 10rem;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.cc-linkish { color: var(--cc-accent); text-decoration: none; font-weight: 700; font-size: 0.8rem; }
.cc-linkish:hover { text-decoration: underline; }
.cc-media-token { font-family: var(--cc-mono); font-size: 0.7rem; color: var(--cc-accent); }
.cc-media-panel {
  border: 1px dashed var(--cc-line-2); border-radius: var(--cc-radius);
  background: linear-gradient(180deg, color-mix(in srgb, var(--cc-surface-2) 70%, transparent), var(--cc-surface-2));
  padding: 1rem;
}
.cc-dropzone {
  border: 1px dashed var(--cc-line-2); border-radius: var(--cc-radius-sm); background: var(--cc-surface);
  padding: 0.95rem; text-align: center; color: var(--cc-muted); font-size: 0.84rem; transition: 0.12s ease;
}
.cc-dropzone:hover, .cc-dropzone.is-drag {
  border-color: var(--cc-accent);
  background: color-mix(in srgb, var(--cc-accent-soft) 45%, var(--cc-surface));
  color: var(--cc-ink);
}
.cc-dropzone__hint { margin: 0.25rem 0 0; font-size: 0.74rem; color: var(--cc-faint); }
.cc-create {
  display: flex; flex-direction: column; gap: 0.75rem; padding: 1rem 1.05rem;
  border: 1px solid var(--cc-line); border-radius: var(--cc-radius);
  background: linear-gradient(135deg, color-mix(in srgb, var(--cc-accent-soft) 50%, transparent), transparent 52%), var(--cc-surface-2);
}
.cc-create__head { display: flex; align-items: baseline; justify-content: space-between; gap: 0.5rem; }
.cc-create__title { font-size: 0.86rem; font-weight: 750; color: var(--cc-ink); }
.cc-create__hint { font-size: 0.75rem; color: var(--cc-muted); }
.cc-create__meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem 0.85rem; font-size: 0.8rem; color: var(--cc-muted); }
.cc-create__price { font-family: var(--cc-mono); font-weight: 750; color: var(--cc-accent); }
.cc-create__shortcut { font-size: 0.72rem; color: var(--cc-faint); }
.cc-create__draft { font-size: 0.78rem; color: var(--cc-muted); }
.cc-create__balance { font-size: 0.8rem; color: var(--cc-ink); }
.cc-create__balance--blocked { color: var(--cc-bad); }
.cc-create__actions { display: flex; flex-wrap: wrap; gap: 0.5rem; }
.cc-submit {
  min-height: 2.7rem !important; border-radius: 0.9rem !important; font-weight: 760 !important;
  background: linear-gradient(135deg, #2dd4bf 0%, #0f766e 48%, #0e7490 100%) !important;
  border-color: transparent !important; color: #042f2e !important;
  box-shadow: 0 14px 28px -16px rgb(15 118 110 / 0.9);
}
.cc-submit:hover:not(:disabled) { filter: brightness(1.04); }
.cc-submit:disabled { opacity: 0.55; box-shadow: none !important; }
.cc-form-stack {
  display: flex !important;
  flex-direction: column;
  gap: 1.05rem !important;
  padding: 1.15rem 1.2rem 1.25rem !important;
}
.cc-studio-head {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  padding-bottom: 0.35rem;
  border-bottom: 1px solid color-mix(in srgb, var(--cc-line) 80%, transparent);
  margin-bottom: 0.1rem;
}
.cc-studio-head__kicker,
.cc-board-head__kicker {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  font-weight: 750;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--cc-accent-2);
}
.cc-studio-head__kicker-dot,
.cc-board-head__kicker-dot {
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 999px;
  background: var(--cc-accent-bright);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent-bright) 22%, transparent);
}
.cc-studio-head__title {
  margin: 0.15rem 0 0;
  font-family: var(--cc-display);
  font-size: 1.08rem;
  font-weight: 750;
  letter-spacing: -0.025em;
  color: var(--cc-ink);
}
.cc-studio-head__sub {
  margin: 0.15rem 0 0;
  font-size: 0.8rem;
  line-height: 1.45;
  color: var(--cc-muted);
}
.cc-field-hint {
  margin: 0.35rem 0 0;
  font-size: 0.74rem;
  line-height: 1.45;
  color: var(--cc-faint);
}
.cc-callout {
  margin: 0.35rem 0 0;
  padding: 0.45rem 0.65rem;
  border-radius: 0.7rem;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-2);
  font-size: 0.78rem;
  line-height: 1.45;
  color: var(--cc-muted);
}
.cc-callout--warn {
  border-color: color-mix(in srgb, var(--cc-warn) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-warn-soft) 75%, var(--cc-surface));
  color: color-mix(in srgb, var(--cc-warn) 78%, var(--cc-ink));
}
.cc-callout--bad {
  border-color: color-mix(in srgb, var(--cc-bad) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-bad-soft) 75%, var(--cc-surface));
  color: color-mix(in srgb, var(--cc-bad) 80%, var(--cc-ink));
}
.cc-callout--ok {
  border-color: color-mix(in srgb, var(--cc-ok) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-ok-soft) 75%, var(--cc-surface));
  color: color-mix(in srgb, var(--cc-ok) 80%, var(--cc-ink));
}
.cc-callout--live {
  border-color: color-mix(in srgb, var(--cc-live) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-live-soft) 75%, var(--cc-surface));
  color: color-mix(in srgb, var(--cc-live) 80%, var(--cc-ink));
}
.cc-limits-card {
  border: 1px solid var(--cc-line);
  border-radius: var(--cc-radius-sm);
  background: linear-gradient(180deg, var(--cc-surface-2), var(--cc-surface));
  padding: 0.85rem 0.95rem;
  font-size: 0.78rem;
  color: var(--cc-muted);
  line-height: 1.5;
}

.cc-field-hint--inline {
  display: inline;
  margin: 0 0 0 0.35rem;
  font-size: 0.74rem;
  color: var(--cc-faint);
}
.cc-limits-card__body { margin-top: 0.2rem; }
.cc-limits-card__progress {
  display: grid;
  gap: 0.45rem;
  margin-top: 0.65rem;
}
.cc-limits-card__title {
  font-size: 0.8rem;
  font-weight: 750;
  color: var(--cc-ink);
  margin-bottom: 0.25rem;
}

.cc-media-zone {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.cc-dropzone__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
}
.cc-dropzone.is-disabled {
  opacity: 0.55;
  pointer-events: none;
}
.cc-url-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.65rem;
}
.cc-url-row__input {
  flex: 1 1 14rem;
  min-width: 0;
  font-family: var(--cc-mono);
  font-size: 0.74rem !important;
}
.cc-media-list--single {
  grid-template-columns: minmax(0, 1fr);
}
.cc-media-item__badge {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: 0.55rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  font-weight: 750;
  letter-spacing: 0.04em;
  color: var(--cc-accent-2);
  background: linear-gradient(160deg, color-mix(in srgb, var(--cc-accent-soft) 70%, var(--cc-surface)), var(--cc-stage));
  border: 1px solid var(--cc-line);
}
.cc-media-item__badge--vid {
  color: #0e7490;
  background: linear-gradient(160deg, color-mix(in srgb, #67e8f9 35%, var(--cc-surface)), var(--cc-stage));
}
.cc-media-item__badge--aud {
  color: #047857;
  background: linear-gradient(160deg, color-mix(in srgb, #6ee7b7 35%, var(--cc-surface)), var(--cc-stage));
}
.cc-media-panel {
  border: 1px solid var(--cc-line);
  border-radius: var(--cc-radius);
  background: var(--cc-surface-2);
  padding: 0.9rem 1rem;
}
.cc-media-panel__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.65rem;
}
.cc-media-count {
  margin-left: 0.25rem;
  font-family: var(--cc-mono);
  font-size: 0.74rem;
  font-weight: 650;
  color: var(--cc-faint);
}
.cc-req { color: var(--cc-bad); margin-left: 0.15rem; }
.cc-link-danger {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: 0.74rem;
  font-weight: 700;
  color: var(--cc-bad);
  cursor: pointer;
  white-space: nowrap;
}
.cc-link-danger:hover { text-decoration: underline; }
.cc-link-danger:disabled { opacity: 0.5; cursor: not-allowed; }
.cc-media-list {
  list-style: none;
  margin: 0.75rem 0 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(14rem, 1fr));
  gap: 0.55rem;
}
.cc-media-item {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  min-width: 0;
  padding: 0.45rem 0.55rem;
  border: 1px solid var(--cc-line);
  border-radius: 0.8rem;
  background: var(--cc-surface);
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
}
.cc-media-item__thumb {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: 0.55rem;
  object-fit: cover;
  background: var(--cc-stage);
  flex: 0 0 auto;
}
.cc-media-item__body { min-width: 0; flex: 1 1 auto; }
.cc-media-item__row { display: flex; align-items: center; gap: 0.4rem; min-width: 0; }
.cc-media-item__name {
  margin: 0;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.76rem;
  font-weight: 650;
  color: var(--cc-ink);
}
.cc-media-item__url {
  margin: 0.15rem 0 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--cc-mono);
  font-size: 0.66rem;
  color: var(--cc-faint);
}
.cc-board-card { padding: 0 !important; overflow: hidden; }
.cc-board-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1rem 1.1rem 0.95rem;
  border-bottom: 1px solid var(--cc-line);
  background:
    radial-gradient(28rem 12rem at 0% 0%, color-mix(in srgb, var(--cc-accent-soft) 42%, transparent), transparent 70%),
    linear-gradient(180deg, color-mix(in srgb, var(--cc-surface) 92%, #fff), var(--cc-surface-2));
}
:global(.dark) .cc-board-head {
  background:
    radial-gradient(28rem 12rem at 0% 0%, color-mix(in srgb, var(--cc-accent-soft) 35%, transparent), transparent 70%),
    linear-gradient(180deg, var(--cc-surface-2), color-mix(in srgb, var(--cc-surface) 88%, #000));
}
.cc-board-head__main { min-width: 0; flex: 1 1 12rem; }
.cc-board-head__title {
  margin: 0.18rem 0 0;
  font-family: var(--cc-display);
  font-size: 1.02rem;
  font-weight: 750;
  letter-spacing: -0.02em;
  color: var(--cc-ink);
}
.cc-board-head__sub {
  margin: 0.2rem 0 0;
  font-size: 0.78rem;
  line-height: 1.45;
  color: var(--cc-muted);
}
.cc-board-head__refresh { flex: 0 0 auto; }
.cc-board-body {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  padding: 0.95rem 1rem 1.05rem;
}
.cc-latest {
  border: 1px solid color-mix(in srgb, var(--cc-ok) 28%, var(--cc-line));
  border-radius: var(--cc-radius-sm);
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--cc-ok-soft) 55%, transparent), transparent 55%),
    var(--cc-surface);
  padding: 0.8rem 0.85rem;
}
.cc-latest__head {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  margin-bottom: 0.55rem;
}
.cc-latest__badge {
  display: inline-flex;
  align-items: center;
  padding: 0.12rem 0.45rem;
  border-radius: 999px;
  font-family: var(--cc-mono);
  font-size: 0.64rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  color: var(--cc-ok);
  background: color-mix(in srgb, var(--cc-ok-soft) 80%, var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-ok) 28%, var(--cc-line));
}
.cc-latest__title {
  margin: 0;
  font-size: 0.78rem;
  font-weight: 750;
  color: color-mix(in srgb, var(--cc-ok) 75%, var(--cc-ink));
}
.cc-latest__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(8.5rem, 1fr));
  gap: 0.5rem;
}
.cc-latest__tile {
  appearance: none;
  border: 1px solid color-mix(in srgb, var(--cc-ok) 18%, var(--cc-line));
  border-radius: 0.7rem;
  overflow: hidden;
  padding: 0;
  background: var(--cc-stage);
  cursor: pointer;
  min-height: 6.5rem;
}
.cc-latest__tile img {
  width: 100%;
  height: 100%;
  max-height: 9rem;
  object-fit: contain;
  display: block;
  background: #0b1220;
}
.cc-latest__stage {
  border-radius: 0.75rem;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--cc-ok) 18%, var(--cc-line));
  background: #000;
}
.cc-latest__stage video {
  display: block;
  width: 100%;
  max-height: 16rem;
  object-fit: contain;
  background: #000;
}
.cc-latest__hint {
  margin: 0.45rem 0 0;
  font-size: 0.72rem;
  color: var(--cc-faint);
}
.cc-task-list {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}
.cc-task-card {
  position: relative;
  border: 1px solid var(--cc-line);
  border-radius: calc(var(--cc-radius-sm) + 2px);
  background: var(--cc-surface);
  padding: 0.8rem 0.9rem;
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.cc-task-card::before {
  content: "";
  position: absolute;
  left: 0;
  top: 0.7rem;
  bottom: 0.7rem;
  width: 3px;
  border-radius: 999px;
  background: var(--cc-line-2);
}
.cc-task-card.is-live {
  border-color: color-mix(in srgb, var(--cc-live) 34%, var(--cc-line));
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--cc-live-soft) 55%, transparent), transparent 46%),
    var(--cc-surface);
}
.cc-task-card.is-live::before { background: var(--cc-live); }
.cc-task-card.is-ok { border-color: color-mix(in srgb, var(--cc-ok) 28%, var(--cc-line)); }
.cc-task-card.is-ok::before { background: var(--cc-ok); }
.cc-task-card.is-bad { border-color: color-mix(in srgb, var(--cc-bad) 28%, var(--cc-line)); }
.cc-task-card.is-bad::before { background: var(--cc-bad); }
.cc-task-card.is-flash { animation: cc-task-flash 1.1s ease; }
.cc-task-card.is-focus {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent) 22%, transparent), var(--cc-shadow);
}
.cc-task-card.is-dim { opacity: 0.62; }
@keyframes cc-task-flash {
  0% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--cc-accent) 35%, transparent); }
  100% { box-shadow: 0 0 0 0 transparent; }
}
.cc-task-card__top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem 0.5rem;
  padding-left: 0.35rem;
}
.cc-task-status {
  display: inline-flex;
  align-items: center;
  gap: 0.32rem;
  padding: 0.18rem 0.52rem;
  border-radius: 999px;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-2);
  font-size: 0.7rem;
  font-weight: 750;
  color: var(--cc-muted);
}
.cc-task-status__dot {
  width: 0.38rem;
  height: 0.38rem;
  border-radius: 999px;
  background: currentColor;
  flex: 0 0 auto;
}
.cc-task-status__dot.is-pulse,
.cc-work-status__dot.is-pulse,
.cc-status-chip__dot.is-pulse {
  animation: cc-pulse 1.4s ease-out infinite;
}
.cc-task-status.is-ok,
.cc-status-chip.is-ok {
  color: var(--cc-ok);
  background: color-mix(in srgb, var(--cc-ok-soft) 78%, var(--cc-surface));
  border-color: color-mix(in srgb, var(--cc-ok) 30%, var(--cc-line));
}
.cc-task-status.is-bad,
.cc-status-chip.is-bad {
  color: var(--cc-bad);
  background: color-mix(in srgb, var(--cc-bad-soft) 78%, var(--cc-surface));
  border-color: color-mix(in srgb, var(--cc-bad) 30%, var(--cc-line));
}
.cc-task-status.is-live,
.cc-status-chip.is-live {
  color: var(--cc-live);
  background: color-mix(in srgb, var(--cc-live-soft) 78%, var(--cc-surface));
  border-color: color-mix(in srgb, var(--cc-live) 30%, var(--cc-line));
}
.cc-task-status.is-new,
.cc-status-chip.is-new {
  color: var(--cc-accent-2);
  background: color-mix(in srgb, var(--cc-accent-soft) 78%, var(--cc-surface));
  border-color: color-mix(in srgb, var(--cc-accent) 28%, var(--cc-line));
}
.cc-task-status.is-idle,
.cc-status-chip.is-idle { color: var(--cc-muted); }
.cc-work-status__dot.is-ok,
.cc-status-chip__dot.is-ok { color: var(--cc-ok); background: var(--cc-ok); }
.cc-work-status__dot.is-bad,
.cc-status-chip__dot.is-bad { color: var(--cc-bad); background: var(--cc-bad); }
.cc-work-status__dot.is-live,
.cc-status-chip__dot.is-live { color: var(--cc-live); background: var(--cc-live); }
.cc-work-status__dot.is-new,
.cc-status-chip__dot.is-new { color: var(--cc-accent-2); background: var(--cc-accent-2); }
.cc-work-status__dot.is-idle,
.cc-status-chip__dot.is-idle { color: var(--cc-faint); background: var(--cc-faint); }
.cc-task-model {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  padding: 0.16rem 0.48rem;
  border-radius: 0.45rem;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-2);
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  font-weight: 700;
  color: var(--cc-ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cc-task-time {
  margin-left: auto;
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  color: var(--cc-faint);
}
.cc-task-prompt {
  margin: 0.55rem 0 0;
  padding-left: 0.35rem;
  font-size: 0.86rem;
  line-height: 1.45;
  color: var(--cc-ink);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.cc-task-error {
  margin: 0.4rem 0 0;
  padding-left: 0.35rem;
  font-size: 0.76rem;
  line-height: 1.4;
  color: var(--cc-bad);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.cc-task-elapsed {
  margin: 0.35rem 0 0;
  padding-left: 0.35rem;
  font-family: var(--cc-mono);
  font-size: 0.72rem;
  color: var(--cc-live);
}
.cc-task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  margin-top: 0.7rem;
  padding-left: 0.35rem;
}
.cc-pagination--board {
  margin-top: 0.15rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--cc-line);
}

/* Filter bar / status / empty stage */
.cc-filterbar {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0.65rem;
  margin-top: 1rem;
  padding: 0.75rem;
  border: 1px solid var(--cc-line);
  border-radius: calc(var(--cc-radius-sm) + 2px);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--cc-surface) 88%, #fff), var(--cc-surface-2));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.55);
}
:global(.dark) .cc-filterbar {
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.03);
  background: linear-gradient(180deg, var(--cc-surface-2), color-mix(in srgb, var(--cc-surface) 80%, #000));
}
@media (max-width: 1100px) {
  .cc-filterbar { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 640px) {
  .cc-filterbar { grid-template-columns: 1fr; }
}
.cc-filter {
  display: flex;
  flex-direction: column;
  gap: 0.28rem;
  min-width: 0;
}
.cc-filter--grow { min-width: 0; }
.cc-filter__label {
  font-size: 0.68rem;
  font-weight: 750;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--cc-faint);
}
.cc-filter__control {
  width: 100% !important;
  min-height: 2.35rem !important;
  height: 2.35rem !important;
  border-radius: 0.7rem !important;
  border-color: var(--cc-line-2) !important;
  background: var(--cc-surface) !important;
  font-size: 0.82rem !important;
  font-weight: 600 !important;
  color: var(--cc-ink) !important;
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
}
.cc-filter__control:focus {
  border-color: var(--cc-accent) !important;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent) 18%, transparent) !important;
}
.cc-filter__control:disabled { opacity: 0.55; cursor: not-allowed; }
.cc-status-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  margin-top: 0.9rem;
}
.cc-status-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.28rem 0.62rem;
  border-radius: 999px;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface);
  font-size: 0.72rem;
  font-weight: 700;
  color: var(--cc-muted);
  box-shadow: 0 1px 0 rgb(15 23 42 / 0.03);
}
.cc-status-chip__dot {
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 999px;
  background: currentColor;
  flex: 0 0 auto;
}
.cc-status-chip__count {
  font-family: var(--cc-mono);
  font-size: 0.72rem;
  opacity: 0.85;
}
.cc-banner {
  margin-bottom: 1rem;
  border-radius: 0.9rem;
  padding: 0.75rem 0.9rem;
  font-size: 0.8rem;
  line-height: 1.5;
  border: 1px solid var(--cc-line);
}
.cc-banner--warn {
  border-color: color-mix(in srgb, var(--cc-warn) 28%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-warn-soft) 78%, var(--cc-surface));
  color: color-mix(in srgb, var(--cc-warn) 80%, var(--cc-ink));
}
.cc-empty-stage {
  min-height: 18rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: 0.35rem;
  padding: 2rem 1.25rem;
  border: 1px dashed color-mix(in srgb, var(--cc-line-2) 80%, var(--cc-accent));
  border-radius: calc(var(--cc-radius) + 2px);
  background:
    radial-gradient(28rem 12rem at 50% 0%, color-mix(in srgb, var(--cc-accent-soft) 55%, transparent), transparent 70%),
    linear-gradient(180deg, var(--cc-surface-2), var(--cc-surface));
}
.cc-empty-stage__icon {
  width: 3.1rem;
  height: 3.1rem;
  border-radius: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 0.55rem;
  color: var(--cc-accent);
  background:
    linear-gradient(160deg, color-mix(in srgb, var(--cc-accent-soft) 80%, #fff), var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-accent) 22%, var(--cc-line));
  box-shadow: 0 10px 24px -16px rgb(15 118 110 / 0.65);
}
.cc-empty-stage__icon--soft {
  background: var(--cc-stage);
  color: var(--cc-accent-bright);
  border-color: rgb(255 255 255 / 0.08);
}
.cc-empty-stage__glyph { font-size: 1.15rem; line-height: 1; opacity: 0.9; }
.cc-empty-stage--compact { min-height: 10rem; padding: 1.4rem 1rem; }
.cc-check {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 0.86rem;
  font-weight: 650;
  color: var(--cc-ink);
  cursor: pointer;
  user-select: none;
}
.cc-check__input {
  width: 1rem;
  height: 1rem;
  accent-color: var(--cc-accent);
  border-radius: 0.25rem;
}
.cc-empty-stage__title {
  margin: 0;
  font-size: 0.98rem;
  font-weight: 750;
  letter-spacing: -0.02em;
  color: var(--cc-ink);
}
.cc-empty-stage__sub {
  margin: 0;
  max-width: 28rem;
  font-size: 0.8rem;
  line-height: 1.5;
  color: var(--cc-muted);
}
.cc-empty-guide {
  margin-top: 1.15rem;
  width: 100%;
  max-width: 26rem;
  text-align: left;
  border: 1px solid var(--cc-line);
  border-radius: var(--cc-radius-sm);
  background: var(--cc-surface);
  padding: 0.85rem 0.95rem;
  box-shadow: var(--cc-shadow);
}
.cc-empty-guide__title {
  margin: 0 0 0.45rem;
  font-size: 0.7rem;
  font-weight: 750;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--cc-accent);
}
.cc-empty-guide__list {
  margin: 0;
  padding-left: 1.05rem;
  display: grid;
  gap: 0.35rem;
  font-size: 0.78rem;
  color: var(--cc-muted);
  line-height: 1.45;
}
.cc-works-skeleton {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(17.5rem, 1fr));
  gap: 0.95rem;
}
.cc-works-skeleton__card {
  border: 1px solid var(--cc-line);
  border-radius: calc(var(--cc-radius-sm) + 2px);
  overflow: hidden;
  background: var(--cc-surface);
}
.cc-works-skeleton__media {
  aspect-ratio: 16 / 10;
  background:
    linear-gradient(90deg, var(--cc-surface-3) 0%, color-mix(in srgb, var(--cc-surface-2) 40%, #fff) 50%, var(--cc-surface-3) 100%);
  background-size: 200% 100%;
  animation: cc-shimmer 1.35s linear infinite;
}
.cc-works-skeleton__lines {
  display: grid;
  gap: 0.45rem;
  padding: 0.85rem 0.9rem 1rem;
}
.cc-works-skeleton__lines i {
  display: block;
  height: 0.65rem;
  border-radius: 999px;
  background: var(--cc-surface-3);
}
.cc-works-skeleton__lines i:nth-child(1) { width: 88%; height: 0.8rem; }
.cc-works-skeleton__lines i:nth-child(2) { width: 62%; }
.cc-works-skeleton__lines i:nth-child(3) { width: 42%; }

/* Works library */
.cc-works {
  border: 1px solid var(--cc-line) !important;
  border-radius: calc(var(--cc-radius) + 4px) !important;
  background: var(--cc-surface) !important;
  box-shadow: var(--cc-shadow) !important;
  overflow: hidden;
}
.cc-works__head {
  border-bottom: 1px solid var(--cc-line);
  background:
    radial-gradient(40rem 12rem at 0% 0%, color-mix(in srgb, var(--cc-accent-soft) 42%, transparent), transparent 60%),
    linear-gradient(180deg, color-mix(in srgb, var(--cc-surface-2) 88%, #fff), var(--cc-surface-2));
  padding: 1.15rem 1.25rem 1.05rem;
}
.cc-works__title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.85rem 1rem;
}
.cc-works__title-block { min-width: 0; flex: 1 1 16rem; }
.cc-works__kicker {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  margin-bottom: 0.35rem;
  font-size: 0.66rem;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--cc-accent);
}
.cc-works__kicker-dot {
  width: 0.38rem;
  height: 0.38rem;
  border-radius: 999px;
  background: var(--cc-accent-bright);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent-bright) 25%, transparent);
}
.cc-works__title {
  margin: 0; font-family: var(--cc-display); font-size: 1.18rem; font-weight: 760;
  letter-spacing: -0.03em; color: var(--cc-ink);
}
.cc-works__refresh { border-radius: 0.75rem !important; }
.cc-works-body-pad { padding: 1.15rem 1.2rem 1.25rem; }

.cc-works__sub { margin: 0.35rem 0 0; font-size: 0.8rem; color: var(--cc-muted); line-height: 1.45; }
.cc-works__keychip {
  display: inline-flex; align-items: center; margin-left: 0.35rem; padding: 0.14rem 0.55rem;
  border-radius: 999px; font-size: 0.74rem; font-weight: 700; color: var(--cc-accent);
  background: color-mix(in srgb, var(--cc-accent-soft) 75%, var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-accent) 22%, var(--cc-line));
  max-width: 18rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: middle;
}
.cc-works-body { display: flex; flex-direction: column; gap: 0.85rem; }
.cc-batch-bar {
  display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0.6rem;
  padding: 0.65rem 0.75rem; border: 1px solid var(--cc-line); border-radius: var(--cc-radius-sm);
  background: var(--cc-surface-2);
}
.cc-batch-bar__left { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem; }
.cc-batch-bar__count { font-size: 0.76rem; color: var(--cc-muted); }
.cc-batch-bar__danger { color: var(--cc-bad) !important; }
.cc-works-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(17.5rem, 1fr)); gap: 0.95rem;
}
.cc-work-card {
  display: flex; flex-direction: column; border: 1px solid var(--cc-line);
  border-radius: calc(var(--cc-radius) + 2px); background: var(--cc-surface);
  box-shadow: var(--cc-shadow); overflow: hidden;
  transition: transform 0.16s ease, box-shadow 0.16s ease, border-color 0.16s ease;

  isolation: isolate;
  transition: transform 0.18s ease, box-shadow 0.18s ease, border-color 0.18s ease;
}
.cc-work-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--cc-accent) 28%, var(--cc-line));
  box-shadow: var(--cc-shadow-lg);
}

.cc-work-card:hover {
  transform: translateY(-2px); box-shadow: var(--cc-shadow-lg);
  border-color: color-mix(in srgb, var(--cc-accent) 22%, var(--cc-line));
}
.cc-work-card--ok { box-shadow: var(--cc-shadow), inset 0 0 0 1px color-mix(in srgb, var(--cc-ok) 16%, transparent); }
.cc-work-card--bad { box-shadow: var(--cc-shadow), inset 0 0 0 1px color-mix(in srgb, var(--cc-bad) 18%, transparent); }
.cc-work-card--live { box-shadow: var(--cc-shadow), inset 0 0 0 1px color-mix(in srgb, var(--cc-live) 20%, transparent); }
.cc-work-card--expired { opacity: 0.78; }
.cc-work-card--dim { opacity: 0.72; }
.cc-work-card--flash { animation: cc-flash 1.1s ease; }
.cc-work-card--focus { outline: 2px solid color-mix(in srgb, var(--cc-accent) 50%, transparent); outline-offset: 2px; }
.cc-work-card--idle {}
@keyframes cc-flash {
  0%, 100% { box-shadow: var(--cc-shadow); }
  40% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--cc-accent) 30%, transparent); }
}
.cc-work-media {
  position: relative; aspect-ratio: 16 / 10;
  background: radial-gradient(90% 80% at 50% 40%, #1a2438 0%, var(--cc-stage) 70%), var(--cc-stage);
  overflow: hidden;
}
.cc-work-media__corners { pointer-events: none; position: absolute; inset: 0.45rem; z-index: 3; }
.cc-work-media__corners i {
  position: absolute; width: 0.7rem; height: 0.7rem; border: 1.5px solid rgb(34 211 238 / 0.55); opacity: 0.7;
}
.cc-work-media__corners i:nth-child(1) { top: 0; left: 0; border-right: 0; border-bottom: 0; }
.cc-work-media__corners i:nth-child(2) { top: 0; right: 0; border-left: 0; border-bottom: 0; }
.cc-work-media__corners i:nth-child(3) { bottom: 0; left: 0; border-right: 0; border-top: 0; }
.cc-work-media__corners i:nth-child(4) { bottom: 0; right: 0; border-left: 0; border-top: 0; }
.cc-work-check {
  position: absolute; top: 0.55rem; left: 0.55rem; z-index: 5; display: grid; place-items: center;
  width: 1.55rem; height: 1.55rem; border-radius: 0.45rem; background: rgb(15 23 42 / 0.55);
  border: 1px solid rgb(255 255 255 / 0.12); backdrop-filter: blur(6px);
}
.cc-work-check input { width: 0.95rem; height: 0.95rem; accent-color: var(--cc-accent-2); margin: 0; }
.cc-work-status {
  position: absolute; top: 0.55rem; right: 0.55rem; z-index: 5; display: inline-flex; align-items: center;
  gap: 0.28rem; max-width: calc(100% - 3.5rem); padding: 0.22rem 0.5rem; border-radius: 999px;
  font-size: 0.68rem; font-weight: 750; letter-spacing: 0.02em; background: rgb(15 23 42 / 0.62);
  color: #e2e8f0; border: 1px solid rgb(255 255 255 / 0.1); backdrop-filter: blur(8px);
}
.cc-work-status__dot { width: 0.38rem; height: 0.38rem; border-radius: 999px; background: currentColor; flex: 0 0 auto; }
.cc-work-expired {
  position: absolute; top: 2.25rem; right: 0.55rem; z-index: 5; padding: 0.14rem 0.42rem; border-radius: 999px;
  font-size: 0.65rem; font-weight: 750; color: #fecaca; background: rgb(127 29 29 / 0.75);
  border: 1px solid rgb(248 113 113 / 0.35);
}
.cc-work-cover {
  appearance: none; border: 0; padding: 0; margin: 0; width: 100%; height: 100%; display: block;
  position: relative; cursor: pointer; background: transparent; opacity: 0; transition: opacity 0.25s ease;
}
.cc-work-cover.is-ready { opacity: 1; }
.cc-work-cover__media { width: 100%; height: 100%; object-fit: cover; display: block; background: var(--cc-stage); }
.cc-work-cover__veil {
  position: absolute; inset: 0; display: grid; place-items: center;
  background: linear-gradient(180deg, transparent 40%, rgb(0 0 0 / 0.45));
  opacity: 0; transition: opacity 0.18s ease;
}
.cc-work-card:hover .cc-work-cover__veil { opacity: 1; }
.cc-work-cover__play {
  display: inline-flex; align-items: center; justify-content: center; min-width: 4.5rem;
  padding: 0.4rem 0.8rem; border-radius: 999px; font-size: 0.75rem; font-weight: 750; color: #ecfeff;
  background: rgb(8 145 178 / 0.88); border: 1px solid rgb(165 243 252 / 0.35);
  box-shadow: 0 10px 24px -12px rgb(0 0 0 / 0.7);
}
.cc-work-cover--pending,
.cc-work-cover--empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  text-align: center;
  padding: 1.1rem;
  font-size: 0.78rem;
  font-weight: 650;
  background:
    radial-gradient(18rem 10rem at 50% 30%, rgb(45 212 191 / 0.08), transparent 70%),
    linear-gradient(180deg, #0d1524, #0a101b);
}
.cc-work-cover--pending { color: #a5f3fc; cursor: pointer; }
.cc-work-cover--empty { color: rgb(148 163 184 / 0.92); cursor: default; }
.cc-work-cover--empty::before {
  content: "";
  width: 2.2rem;
  height: 2.2rem;
  border-radius: 0.65rem;
  border: 1px dashed rgb(148 163 184 / 0.35);
  background: rgb(255 255 255 / 0.02);
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.03);
}
.cc-work-cover__pending-text { color: #a5f3fc; letter-spacing: 0.02em; }
.cc-cover-skeleton {
  position: absolute; inset: 0; z-index: 2; display: grid; place-items: center; overflow: hidden;
  background: linear-gradient(135deg, #0b1220 0%, #152033 45%, #0b1220 100%);
}
.cc-cover-skeleton__shine {
  position: absolute; inset: 0;
  background: linear-gradient(105deg, transparent 30%, rgb(34 211 238 / 0.12) 48%, transparent 66%);
  transform: translateX(-60%); animation: cc-shimmer 1.35s ease-in-out infinite;
}
.cc-cover-skeleton__label {
  position: relative; z-index: 1; padding: 0.35rem 0.7rem; border-radius: 999px; font-size: 0.72rem;
  font-weight: 700; letter-spacing: 0.04em; color: #a5f3fc; background: rgb(8 145 178 / 0.18);
  border: 1px solid rgb(34 211 238 / 0.28); backdrop-filter: blur(4px);
}
@keyframes cc-shimmer {
  0% { transform: translateX(-70%); }
  100% { transform: translateX(70%); }
}
.cc-work-body {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  padding: 0.9rem 0.95rem 1rem;
  background: linear-gradient(180deg, color-mix(in srgb, var(--cc-surface) 92%, var(--cc-surface-2)), var(--cc-surface));
  border-top: 1px solid color-mix(in srgb, var(--cc-line) 80%, transparent);
}

.cc-work-prompt {
  margin: 0; font-size: 0.88rem; font-weight: 700; line-height: 1.4; color: var(--cc-ink);
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; min-height: 2.45rem;
}
.cc-work-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem 0.55rem; }
.cc-work-time { font-family: var(--cc-mono); font-size: 0.7rem; color: var(--cc-muted); }
.cc-work-error {
  border-radius: 0.7rem; background: color-mix(in srgb, var(--cc-bad-soft) 80%, var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-bad) 22%, var(--cc-line)); padding: 0.5rem 0.6rem;
}
.cc-work-error p { margin: 0; font-size: 0.74rem; color: var(--cc-bad); line-height: 1.4; }
.cc-work-error__actions { margin-top: 0.3rem; display: flex; flex-wrap: wrap; gap: 0.55rem; }
.cc-work-error__actions button {
  appearance: none; border: 0; background: transparent; padding: 0; font-size: 0.7rem; font-weight: 700;
  color: var(--cc-muted); cursor: pointer; text-decoration: underline; text-underline-offset: 2px;
}
.cc-work-elapsed { margin: 0; font-family: var(--cc-mono); font-size: 0.72rem; color: var(--cc-live); }
.cc-work-actions { display: flex; flex-wrap: wrap; gap: 0.4rem; padding-top: 0.15rem; }
.cc-work-actions .btn { border-radius: 0.7rem !important; }
.cc-empty-guide {
  border: 1px solid var(--cc-line); border-radius: var(--cc-radius-sm); background: var(--cc-surface-2); padding: 0.85rem 0.95rem;
}
.cc-rules-card {
  border: 1px solid var(--cc-line); border-radius: var(--cc-radius-sm); background: var(--cc-surface-2);
  padding: 0.8rem 0.9rem; font-size: 0.8rem; color: var(--cc-muted); line-height: 1.5;
}
.cc-rules-card--empty { border-style: dashed; }
.cc-progress { height: 0.35rem; border-radius: 999px; background: var(--cc-line); overflow: hidden; }
.cc-progress__bar {
  height: 100%; border-radius: inherit; background: linear-gradient(90deg, #22d3ee, #0e7490); transition: width 0.25s ease;
}
.cc-mention-menu {
  position: absolute; left: 0; right: 0; top: calc(100% + 0.35rem); z-index: 40; max-height: 16rem; overflow: auto;
  border: 1px solid var(--cc-line-2); border-radius: var(--cc-radius); background: var(--cc-surface); box-shadow: var(--cc-shadow-lg);
}
.cc-mention-menu__head {
  padding: 0.55rem 0.75rem 0.4rem; font-size: 0.72rem; font-weight: 750; color: var(--cc-muted); border-bottom: 1px solid var(--cc-line);
}
.cc-mention-menu__empty { padding: 0.75rem; font-size: 0.8rem; color: var(--cc-muted); text-align: center; }
.cc-mention-item {
  display: grid; grid-template-columns: 2.5rem minmax(0, 1fr); gap: 0.55rem; width: 100%; text-align: left;
  border: 0; border-bottom: 1px solid var(--cc-line); background: transparent; color: inherit; padding: 0.55rem 0.7rem; cursor: pointer;
}
.cc-mention-item:last-child { border-bottom: 0; }
.cc-mention-item:hover, .cc-mention-item--active {
  background: color-mix(in srgb, var(--cc-accent-soft) 50%, var(--cc-surface));
}
.cc-mention-item__thumb {
  width: 2.5rem; height: 2.5rem; border-radius: 0.45rem; overflow: hidden; background: var(--cc-surface-2);
  border: 1px solid var(--cc-line); display: flex; align-items: center; justify-content: center;
}
.cc-mention-item__thumb img { width: 100%; height: 100%; object-fit: cover; }
.cc-mention-item__kind { font-size: 0.6rem; font-weight: 750; color: var(--cc-muted); }
.cc-mention-item__body { min-width: 0; display: flex; flex-direction: column; gap: 0.1rem; }
.cc-mention-item__label {
  font-size: 0.84rem; font-weight: 650; color: var(--cc-ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.cc-mention-item__token { font-family: var(--cc-mono); font-size: 0.72rem; color: var(--cc-accent); }
.cc-pagination {
  display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0.75rem;
  margin-top: 1rem; padding-top: 0.85rem; border-top: 1px solid var(--cc-line);
}
.cc-pagination__meta { font-size: 0.78rem; color: var(--cc-muted); }
.cc-pagination__actions { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem; }
.cc-pagination__select {
  min-height: 2rem !important; height: 2rem !important; border-radius: 0.6rem !important;
  font-size: 0.78rem !important; padding: 0 0.45rem !important;
}

.cc-shell--tray { padding-bottom: 5.5rem; }
.cc-shell--tray-open { padding-bottom: min(48vh, 26rem); }
/* ===== Floating task tray ===== */
.cc-task-tray {
  position: fixed;
  left: 50%;
  bottom: 1rem;
  z-index: 70;
  width: min(52rem, calc(100vw - 1.5rem));
  transform: translateX(-50%);
  border: 1px solid color-mix(in srgb, var(--cc-line-2) 70%, #94a3b8);
  border-radius: 1.05rem;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--cc-surface) 92%, #fff), color-mix(in srgb, var(--cc-surface-2) 88%, #e2e8f0));
  box-shadow:
    0 1px 0 rgb(255 255 255 / 0.65) inset,
    0 18px 50px -28px rgb(15 23 42 / 0.55),
    0 8px 20px -12px rgb(15 23 42 / 0.28);
  backdrop-filter: blur(14px);
  overflow: hidden;
  color: var(--cc-ink);
  font-family: var(--cc-font);
}
:global(.dark) .cc-task-tray {
  border-color: color-mix(in srgb, var(--cc-line) 80%, #334155);
  background: linear-gradient(180deg, color-mix(in srgb, var(--cc-surface) 88%, #0b1220), var(--cc-surface-2));
  box-shadow:
    0 1px 0 rgb(255 255 255 / 0.04) inset,
    0 22px 48px -24px rgb(0 0 0 / 0.65);
}
.cc-task-tray--has-live {
  border-color: color-mix(in srgb, var(--cc-live) 35%, var(--cc-line-2));
}
.cc-task-tray__bar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.55rem;
  align-items: center;
  padding: 0.55rem 0.6rem 0.55rem 0.7rem;
  min-height: 3.15rem;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--cc-live-soft) 35%, transparent), transparent 42%);
}
.cc-task-tray--collapsed .cc-task-tray__bar {
  background: transparent;
}
.cc-task-tray__toggle {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0.15rem 0.1rem;
  margin: 0;
  min-width: 0;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 0.55rem 0.7rem;
  align-items: center;
  text-align: left;
  color: inherit;
  cursor: pointer;
}
.cc-task-tray__pulse {
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 999px;
  background: var(--cc-faint);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--cc-faint) 18%, transparent);
  flex: 0 0 auto;
}
.cc-task-tray--has-live .cc-task-tray__pulse {
  background: var(--cc-live);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--cc-live) 22%, transparent);
  animation: cc-tray-pulse 1.6s ease-in-out infinite;
}
@keyframes cc-tray-pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.12); opacity: 0.72; }
}
.cc-task-tray__head {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.12rem;
}
.cc-task-tray__title-row {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 0;
}
.cc-task-tray__title {
  font-family: var(--cc-display);
  font-size: 0.9rem;
  font-weight: 750;
  letter-spacing: 0.01em;
  color: var(--cc-ink);
  white-space: nowrap;
}
.cc-task-tray__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.25rem;
  height: 1.25rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  background: color-mix(in srgb, var(--cc-live) 16%, var(--cc-surface));
  border: 1px solid color-mix(in srgb, var(--cc-live) 35%, var(--cc-line));
  color: var(--cc-live);
  font-family: var(--cc-mono);
  font-size: 0.68rem;
  font-weight: 750;
}
.cc-task-tray__chev {
  font-size: 0.72rem;
  color: var(--cc-muted);
  transition: transform 0.18s ease;
  line-height: 1;
}
.cc-task-tray__chev.is-open { transform: rotate(180deg); }
.cc-task-tray__hint {
  font-size: 0.72rem;
  color: var(--cc-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cc-task-tray__counts {
  display: none;
  flex-wrap: wrap;
  gap: 0.3rem;
  justify-content: flex-end;
}
@media (min-width: 720px) {
  .cc-task-tray__counts { display: inline-flex; }
}
.cc-task-tray__chip {
  display: inline-flex;
  align-items: center;
  gap: 0.28rem;
  padding: 0.16rem 0.45rem;
  border-radius: 999px;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface);
  font-size: 0.68rem;
  font-weight: 700;
  color: var(--cc-muted);
  white-space: nowrap;
}
.cc-task-tray__chip.is-live {
  color: #c2410c;
  border-color: color-mix(in srgb, var(--cc-live) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-live-soft) 70%, var(--cc-surface));
}
.cc-task-tray__chip.is-ok {
  color: #047857;
  border-color: color-mix(in srgb, var(--cc-ok) 28%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-ok-soft) 70%, var(--cc-surface));
}
.cc-task-tray__chip.is-bad {
  color: #b91c1c;
  border-color: color-mix(in srgb, var(--cc-bad) 28%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-bad-soft) 70%, var(--cc-surface));
}
.cc-task-tray__chip-dot {
  width: 0.35rem;
  height: 0.35rem;
  border-radius: 999px;
  background: currentColor;
}
.cc-task-tray__chip-dot.is-pulse {
  animation: cc-tray-pulse 1.4s ease-in-out infinite;
}
.cc-task-tray__actions {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  flex: 0 0 auto;
}
.cc-task-tray__link {
  appearance: none;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface);
  color: var(--cc-ink-soft);
  border-radius: 0.7rem;
  padding: 0.38rem 0.7rem;
  font-size: 0.74rem;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
}
.cc-task-tray__link:hover {
  border-color: color-mix(in srgb, var(--cc-accent) 35%, var(--cc-line));
  color: var(--cc-accent);
  background: color-mix(in srgb, var(--cc-accent-soft) 55%, var(--cc-surface));
}
.cc-task-tray__dismiss {
  appearance: none;
  border: 1px solid transparent;
  background: transparent;
  color: var(--cc-faint);
  width: 1.85rem;
  height: 1.85rem;
  border-radius: 0.65rem;
  font-size: 1.1rem;
  line-height: 1;
  cursor: pointer;
}
.cc-task-tray__dismiss:hover {
  color: var(--cc-ink);
  background: var(--cc-surface-3);
  border-color: var(--cc-line);
}
.cc-task-tray__body {
  border-top: 1px solid var(--cc-line);
  max-height: min(42vh, 22rem);
  overflow: auto;
  padding: 0.45rem;
  background: color-mix(in srgb, var(--cc-surface-2) 70%, transparent);
}
.cc-task-tray__list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.cc-task-tray__item {
  appearance: none;
  width: 100%;
  border: 1px solid var(--cc-line);
  border-radius: 0.85rem;
  background: var(--cc-surface);
  color: inherit;
  text-align: left;
  padding: 0;
  cursor: pointer;
  display: grid;
  grid-template-columns: 0.28rem minmax(0, 1fr);
  overflow: hidden;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
}
.cc-task-tray__item:hover {
  border-color: color-mix(in srgb, var(--cc-accent) 30%, var(--cc-line));
  box-shadow: 0 8px 18px -16px rgb(15 23 42 / 0.45);
  transform: translateY(-1px);
}
.cc-task-tray__item-rail {
  display: block;
  background: var(--cc-faint);
}
.cc-task-tray__item.is-live .cc-task-tray__item-rail,
.cc-task-tray__item.is-new .cc-task-tray__item-rail { background: var(--cc-live); }
.cc-task-tray__item.is-ok .cc-task-tray__item-rail { background: var(--cc-ok); }
.cc-task-tray__item.is-bad .cc-task-tray__item-rail { background: var(--cc-bad); }
.cc-task-tray__item.is-idle .cc-task-tray__item-rail { background: var(--cc-line-2); }
.cc-task-tray__item-main {
  min-width: 0;
  padding: 0.55rem 0.7rem 0.55rem 0.65rem;
  display: flex;
  flex-direction: column;
  gap: 0.28rem;
}
.cc-task-tray__item-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem 0.4rem;
  min-width: 0;
}
.cc-task-tray__status {
  display: inline-flex;
  align-items: center;
  gap: 0.28rem;
  padding: 0.12rem 0.42rem;
  border-radius: 999px;
  font-size: 0.66rem;
  font-weight: 750;
  letter-spacing: 0.02em;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-2);
  color: var(--cc-muted);
  white-space: nowrap;
}
.cc-task-tray__status.is-live,
.cc-task-tray__status.is-new {
  color: #c2410c;
  border-color: color-mix(in srgb, var(--cc-live) 30%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-live-soft) 75%, var(--cc-surface));
}
.cc-task-tray__status.is-ok {
  color: #047857;
  border-color: color-mix(in srgb, var(--cc-ok) 28%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-ok-soft) 75%, var(--cc-surface));
}
.cc-task-tray__status.is-bad {
  color: #b91c1c;
  border-color: color-mix(in srgb, var(--cc-bad) 28%, var(--cc-line));
  background: color-mix(in srgb, var(--cc-bad-soft) 75%, var(--cc-surface));
}
.cc-task-tray__status-dot {
  width: 0.35rem;
  height: 0.35rem;
  border-radius: 999px;
  background: currentColor;
  flex: 0 0 auto;
}
.cc-task-tray__status-dot.is-pulse {
  animation: cc-tray-pulse 1.4s ease-in-out infinite;
}
.cc-task-tray__kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2rem;
  padding: 0.1rem 0.35rem;
  border-radius: 0.4rem;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-3);
  font-family: var(--cc-mono);
  font-size: 0.62rem;
  font-weight: 800;
  letter-spacing: 0.04em;
  color: var(--cc-muted);
}
.cc-task-tray__kind[data-kind="video"] {
  color: #0369a1;
  border-color: color-mix(in srgb, #0ea5e9 35%, var(--cc-line));
  background: color-mix(in srgb, #e0f2fe 70%, var(--cc-surface));
}
.cc-task-tray__kind[data-kind="image"],
.cc-task-tray__kind[data-kind="img"] {
  color: #6d28d9;
  border-color: color-mix(in srgb, #8b5cf6 30%, var(--cc-line));
  background: color-mix(in srgb, #ede9fe 70%, var(--cc-surface));
}
.cc-task-tray__model {
  display: inline-flex;
  max-width: 12rem;
  align-items: center;
  padding: 0.1rem 0.4rem;
  border-radius: 0.4rem;
  border: 1px solid var(--cc-line);
  background: var(--cc-surface-2);
  font-family: var(--cc-mono);
  font-size: 0.66rem;
  font-weight: 700;
  color: var(--cc-ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cc-task-tray__elapsed {
  margin-left: auto;
  font-family: var(--cc-mono);
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--cc-live);
  white-space: nowrap;
}
.cc-task-tray__time {
  margin-left: auto;
  font-family: var(--cc-mono);
  font-size: 0.66rem;
  color: var(--cc-faint);
  white-space: nowrap;
}
.cc-task-tray__prompt {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  font-size: 0.8rem;
  line-height: 1.4;
  color: var(--cc-ink-soft);
  word-break: break-word;
}
.cc-task-tray__error {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  font-size: 0.72rem;
  color: var(--cc-bad);
  line-height: 1.35;
}
.cc-task-tray__empty {
  margin: 0;
  padding: 1rem 0.75rem;
  text-align: center;
  font-size: 0.8rem;
  color: var(--cc-muted);
}
@media (max-width: 720px) {
  .cc-task-tray {
    left: 0.75rem;
    right: 0.75rem;
    width: auto;
    transform: none;
    bottom: 0.75rem;
  }
  .cc-task-tray__toggle {
    grid-template-columns: auto minmax(0, 1fr);
  }
  .cc-task-tray__model { max-width: 8rem; }
  .cc-task-tray__elapsed,
  .cc-task-tray__time { margin-left: 0; }
}

.cc-keydeck, .cc-hero { display: contents; }
.cc-shell :deep(.badge) { border-radius: 999px; }
@media (max-width: 640px) {
  .cc-shell { gap: 0.8rem; }
  .cc-params-grid { grid-template-columns: 1fr; }
  .cc-create__actions { flex-direction: column; }
  .cc-create__actions > * { width: 100%; }
  .cc-tabs { grid-template-columns: 1fr; }
  .cc-works-grid { grid-template-columns: 1fr; }
}
@media (prefers-reduced-motion: reduce) {
  .cc-pill__dot.is-pulse, .cc-work-card--flash, .cc-cover-skeleton__shine { animation: none !important; }
  .cc-work-card:hover { transform: none; }
}

</style>
