package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SeedanceCreateTask accepts the Volcengine Ark task format and forwards it to
// the asynchronous video API configured on the selected Seedance account.
func (h *OpenAIGatewayHandler) SeedanceCreateTask(c *gin.Context) {
	h.handleSeedanceCreate(c, false)
}

// SeedanceCreateJob accepts the public Seedance-compatible video generation
// format and forwards it to the selected Seedance account.
func (h *OpenAIGatewayHandler) SeedanceCreateJob(c *gin.Context) {
	h.handleSeedanceCreate(c, true)
}

func (h *OpenAIGatewayHandler) handleSeedanceCreate(c *gin.Context, public bool) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()
	apiKey, subject, ok := h.seedanceAuthContext(c)
	if !ok {
		return
	}
	reqLog := requestLogger(c, "handler.seedance.create",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}
	if !h.ensureSeedanceGroup(c, apiKey) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, isMax := extractMaxBytesError(err); isMax {
			seedanceError(c, http.StatusRequestEntityTooLarge, "request_too_large", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		seedanceError(c, http.StatusBadRequest, "invalid_request", "Failed to read request body")
		return
	}
	var requestInfo *service.SeedanceRequestInfo
	if public {
		requestInfo, err = service.ParseSeedanceVideoGenerationRequest(body)
	} else {
		requestInfo, err = service.ParseSeedanceCreateRequest(body)
	}
	if err != nil {
		seedanceError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := service.ValidateFFLinkVideoModelPlatform(apiKey.Group.Platform, requestInfo.Model); err != nil {
		seedanceError(c, http.StatusBadRequest, "model_not_supported", err.Error())
		return
	}
	if status, code, message := seedanceVideoPricingError(apiKey.Group, requestInfo.Model, requestInfo.Resolution); status != 0 {
		seedanceError(c, status, code, message)
		return
	}
	clientIdempotencyKey := ensureSeedanceCreateIdempotencyKey(c)
	c.Request = c.Request.WithContext(service.WithSeedanceIdempotencyKey(
		c.Request.Context(),
		seedanceCreateIdempotencyScope(subject.UserID, apiKey.ID, clientIdempotencyKey),
	))

	reqLog = reqLog.With(zap.String("model", requestInfo.Model))
	setOpsRequestContext(c, requestInfo.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		seedanceError(c, status, code, message)
		return
	}
	if h.seedanceMediaService == nil {
		seedanceError(c, http.StatusServiceUnavailable, "media_service_unavailable", "Seedance media service is unavailable")
		return
	}
	mediaRelease, err := h.seedanceMediaService.AcquireMediaIO(c.Request.Context(), seedanceMediaOwner(apiKey, subject), subject.Concurrency)
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	defer mediaRelease()
	var materialized *service.SeedanceMaterializedImages
	materialized, err = h.seedanceMediaService.MaterializeImages(c.Request.Context(), seedanceMediaOwner(apiKey, subject), requestInfo)
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	defer materialized.Cleanup(context.WithoutCancel(c.Request.Context()))
	var fallbackRequestInfo *service.SeedanceRequestInfo
	defer func() {
		if requestInfo != nil && requestInfo.HuiquMedia != nil {
			requestInfo.HuiquMedia.Cleanup()
		}
		if fallbackRequestInfo != nil && fallbackRequestInfo.HuiquMedia != nil && fallbackRequestInfo != requestInfo {
			fallbackRequestInfo.HuiquMedia.Cleanup()
		}
	}()
	fallbackModel, fallbackEligible := service.SeedanceFallbackModelFor(requestInfo.Model, requestInfo.Resolution, requestInfo.DurationSeconds)
	mediaCleanupSnapshot, err := service.SnapshotSeedanceTaskMediaCleanup(requestInfo)
	if err != nil {
		seedanceError(c, http.StatusInternalServerError, "media_snapshot_failed", "Failed to prepare reference media cleanup")
		return
	}
	// Always persist the full request snapshot so admin can inspect prompt and
	// reference materials for every provider/model (including Weijin/Ximei and
	// non-fallback Huiqu paths). Fallback restore continues to use the same shape.
	requestSnapshot, err := service.SnapshotSeedanceFallbackRequest(requestInfo)
	if err != nil {
		seedanceError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	fallbackSnapshot := requestSnapshot

	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	failedAccountIDs := make(map[int64]struct{})
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError
	fallbackActive := false
	enterFallback := func() bool {
		if fallbackActive || !fallbackEligible {
			return false
		}
		fallbackActive = true
		failedAccountIDs = make(map[int64]struct{})
		switchCount = 0
		lastFailover = nil
		return true
	}

	for {
		selectionModel := requestInfo.Model
		activeRequestInfo := requestInfo
		if fallbackActive {
			selectionModel = fallbackModel
			if fallbackRequestInfo == nil {
				fallbackRequestInfo, err = service.RestoreSeedanceFallbackRequest(fallbackSnapshot, fallbackModel)
				if err != nil {
					seedanceError(c, http.StatusBadGateway, "fallback_request_invalid", "Seedance fallback request could not be prepared")
					return
				}
			}
			activeRequestInfo = fallbackRequestInfo
		}
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", sessionHash, selectionModel,
			failedAccountIDs, service.OpenAIUpstreamTransportHTTPSSE,
			"", false, false, false, apiKey.Group.Platform,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if !fallbackActive && enterFallback() {
				continue
			}
			if lastFailover != nil {
				h.handleFailoverExhausted(c, lastFailover, false)
				return
			}
			markOpsRoutingCapacityLimited(c)
			seedanceError(c, http.StatusServiceUnavailable, "no_available_account", "No available Seedance upstream account in this API key group")
			return
		}

		account := selection.Account
		if fallbackActive && !account.IsHuiquVideo() {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			failedAccountIDs[account.ID] = struct{}{}
			if switchCount >= maxSwitches {
				seedanceError(c, http.StatusServiceUnavailable, "no_available_account", "No compatible fallback account is currently available in this API key group")
				return
			}
			switchCount++
			continue
		}
		setOpsSelectedAccount(c, account.ID, account.Platform)
		if account.IsHuiquVideo() && activeRequestInfo.HasReferenceMedia() && activeRequestInfo.HuiquMedia == nil {
			activeRequestInfo.HuiquMedia, err = h.seedanceMediaService.PrepareHuiquMedia(c.Request.Context(), seedanceMediaOwner(apiKey, subject), activeRequestInfo)
			if err != nil {
				writeSeedanceMediaError(c, err)
				return
			}
		}
		accountRelease, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}
		forwarded, forwardErr := func() (*service.SeedanceUpstreamResponse, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, http.MethodPost, "", activeRequestInfo)
		}()
		if forwardErr != nil {
			var unknownAcceptanceErr *service.SeedanceUpstreamAcceptanceUnknownError
			if errors.As(forwardErr, &unknownAcceptanceErr) {
				// A transport failure or unreadable successful response cannot prove
				// rejection. Do not mark the account as a credential failure.
				seedanceError(c, http.StatusServiceUnavailable, "upstream_acceptance_unknown", "Seedance upstream request acceptance could not be confirmed; retry with the same Idempotency-Key")
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(activeRequestInfo.Model), false, nil)
				failedAccountIDs[account.ID] = struct{}{}
				lastFailover = failoverErr
				if !fallbackActive && enterFallback() {
					continue
				}
				if switchCount >= maxSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				h.gatewayService.RecordOpenAIAccountSwitch()
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(activeRequestInfo.Model), false, nil)
			h.writeSeedanceForwardError(c, forwardErr)
			return
		}

		result := forwarded.Result
		if result == nil || strings.TrimSpace(result.ResponseID) == "" {
			seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream did not return a task id")
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, result.UpstreamModel, true, nil)
		// Keep the requested model in every client-visible and billing-facing
		// field, even when the actual request was sent to the MX933 fallback.
		result.Model = requestInfo.Model
		result.BillingModel = requestInfo.Model
		bindingFallbackStatus := ""
		bindingFallbackModel := ""
		// Prefer full request snapshot (prompt + media URLs + stored_media).
		// Fall back to cleanup-only snapshot for defensive compatibility.
		bindingSnapshot := requestSnapshot
		if len(bindingSnapshot) == 0 {
			bindingSnapshot = mediaCleanupSnapshot
		}
		if fallbackEligible && (fallbackActive || !account.IsHuiquVideo()) {
			bindingFallbackModel = fallbackModel
			if len(fallbackSnapshot) > 0 {
				bindingSnapshot = fallbackSnapshot
			}
			bindingFallbackStatus = service.SeedanceFallbackStatusReady
			if fallbackActive {
				bindingFallbackStatus = service.SeedanceFallbackStatusActive
			}
		}
		upstreamResponseID := strings.TrimSpace(result.UpstreamResponseID)
		if upstreamResponseID == "" {
			upstreamResponseID = result.ResponseID
		}
		if err := h.gatewayService.BindSeedanceTaskAccountWithFallback(
			c.Request.Context(), apiKey.GroupID, result.ResponseID, upstreamResponseID,
			subject.UserID, apiKey.ID, account.ID, requestInfo.Model,
			bindingFallbackModel, bindingSnapshot, bindingFallbackStatus,
		); err != nil {
			reqLog.Error("seedance.bind_task_failed", zap.Error(err), zap.String("task_id", result.ResponseID), zap.Int64("account_id", account.ID))
			seedanceError(c, http.StatusBadGateway, "task_binding_failed", "Seedance task was accepted upstream but could not be registered locally")
			return
		}
		if err := recordSeedanceUsage(c, h, apiKey, subscription, account, result, requestInfo.Model, body); err != nil {
			reqLog.Error("seedance.record_usage_failed",
				zap.Error(err),
				zap.String("task_id", result.ResponseID),
				zap.Int64("account_id", account.ID),
			)
			seedanceError(c, http.StatusServiceUnavailable, "billing_unavailable", "Seedance task was accepted upstream but billing could not be finalized")
			return
		}
		// Keep materialized reference objects alive for the asynchronous task.
		// This is also required when the task was created through the Huiqu
		// fallback: the fallback multipart request may still be downloading
		// those signed URLs after this handler returns.
		if materialized != nil {
			materialized.Retain()
		}
		if public {
			statusURL := seedanceAbsoluteURL(c, service.SeedancePublicJobsEndpoint+"/"+url.PathEscape(result.ResponseID))
			response := gin.H{
				"job_id":     result.ResponseID,
				"status":     "queued",
				"status_url": statusURL,
				"model":      requestInfo.Model,
			}
			if forwarded != nil && len(forwarded.Body) > 0 {
				publicBody := forwarded.Body
				if service.IsOpaqueSeedanceVideoProvider(account.GetVideoProvider()) {
					if normalized, normalizeErr := service.NormalizeSeedanceJobForRoute(forwarded.Body, result.ResponseID, account.GetVideoProvider(), requestInfo.Model); normalizeErr == nil {
						publicBody = normalized
					} else {
						publicBody = nil
						reqLog.Warn("seedance.normalize_create_response_failed", zap.Error(normalizeErr), zap.String("task_id", result.ResponseID))
					}
				}
				var upstream map[string]any
				if err := json.Unmarshal(publicBody, &upstream); err == nil {
					response["model"] = requestInfo.Model
					if value, ok := upstream["status"].(string); ok && strings.TrimSpace(value) != "" {
						response["status"] = value
					}
					// The public model is the model requested by the client. Never
					// expose the provider's mapped MX933 model in this response.
					for _, key := range []string{"created_at", "updated_at", "completed_at", "seed", "resolution", "duration", "aspect_ratio"} {
						if value, exists := upstream[key]; exists && value != nil {
							response[key] = value
						}
					}
					if value, exists := upstream["content"]; exists && value != nil {
						response["content"] = value
					}
				}
			}
			c.Header("Preference-Applied", "respond-async")
			c.Header("Location", statusURL)
			c.JSON(http.StatusAccepted, response)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": result.ResponseID})
		return
	}
}

func ensureSeedanceCreateIdempotencyKey(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = "seedance-" + uuid.NewString()
		c.Request.Header.Set("Idempotency-Key", key)
	}
	c.Header("Idempotency-Key", key)
	return key
}

func seedanceCreateIdempotencyScope(userID, apiKeyID int64, clientKey string) string {
	return fmt.Sprintf("seedance-create:v1:user:%d:api-key:%d:client:%s", userID, apiKeyID, strings.TrimSpace(clientKey))
}

type seedanceBase64UploadRequest struct {
	ImageBase64 string `json:"image_base64"`
	ContentType string `json:"content_type,omitempty"`
	Filename    string `json:"filename,omitempty"`
}

func (h *OpenAIGatewayHandler) SeedanceUploadImage(c *gin.Context) {
	h.handleSeedanceUpload(c, false)
}

// SeedanceUploadMedia accepts the public Seedance-compatible media upload
// contract.
func (h *OpenAIGatewayHandler) SeedanceUploadMedia(c *gin.Context) {
	h.handleSeedanceUpload(c, true)
}

func (h *OpenAIGatewayHandler) handleSeedanceUpload(c *gin.Context, public bool) {
	apiKey, subject, ok := h.seedanceAuthContext(c)
	if !ok {
		return
	}
	if !h.ensureSeedanceGroup(c, apiKey) {
		return
	}
	if h.seedanceMediaService == nil || !h.seedanceMediaService.SupportsManagedUploads() {
		seedanceError(c, http.StatusServiceUnavailable, "media_storage_not_configured", "Seedance media storage is not configured")
		return
	}
	owner := seedanceMediaOwner(apiKey, subject)
	mediaRelease, err := h.seedanceMediaService.AcquireMediaIO(c.Request.Context(), owner, subject.Concurrency)
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	defer mediaRelease()
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	var upload *service.SeedanceImageUpload
	err = nil
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "multipart/form-data":
		bodyLimit := service.SeedanceMaxImageBytes + service.SeedanceUploadBodyOverhead
		if public {
			bodyLimit = 512<<20 + service.SeedanceUploadBodyOverhead
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, bodyLimit)
		fieldNames := []string{"image"}
		if public {
			fieldNames = []string{"image", "video", "audio"}
		}
		var file *multipart.FileHeader
		var mediaKind string
		for _, field := range fieldNames {
			candidate, formErr := c.FormFile(field)
			if formErr == nil && candidate != nil {
				file = candidate
				mediaKind = field
				break
			}
		}
		if file == nil {
			if public {
				seedanceError(c, http.StatusBadRequest, "media_required", "multipart field image, video, or audio is required")
			} else {
				seedanceError(c, http.StatusBadRequest, "image_required", "multipart field image is required")
			}
			return
		}
		openLimit := bodyLimit
		if public && strings.EqualFold(mediaKind, "image") {
			openLimit = service.SeedanceMaxImageBytes + service.SeedanceUploadBodyOverhead
		}
		if file.Size > openLimit {
			seedanceError(c, http.StatusRequestEntityTooLarge, "media_too_large", "uploaded media exceeds the configured size limit")
			return
		}
		source, openErr := file.Open()
		if openErr != nil {
			seedanceError(c, http.StatusBadRequest, "invalid_media", "failed to open uploaded media")
			return
		}
		defer func() { _ = source.Close() }()
		if public {
			upload, err = h.seedanceMediaService.UploadMedia(c.Request.Context(), service.SeedanceImageUploadInput{
				Owner:       owner,
				Body:        source,
				SizeBytes:   file.Size,
				ContentType: file.Header.Get("Content-Type"),
				Filename:    file.Filename,
				MediaKind:   mediaKind,
				Persistent:  true,
			})
		} else {
			upload, err = h.seedanceMediaService.UploadImage(c.Request.Context(), service.SeedanceImageUploadInput{
				Owner:       owner,
				Body:        source,
				SizeBytes:   file.Size,
				ContentType: file.Header.Get("Content-Type"),
				Filename:    file.Filename,
				Persistent:  true,
			})
		}
	case "application/json":
		limit := service.SeedanceMaxImageBytes*4/3 + service.SeedanceUploadBodyOverhead
		body, readErr := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
		if readErr != nil {
			if maxErr, isMax := extractMaxBytesError(readErr); isMax {
				seedanceError(c, http.StatusRequestEntityTooLarge, "image_too_large", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			seedanceError(c, http.StatusBadRequest, "invalid_request", "failed to read Base64 upload request")
			return
		}
		var request seedanceBase64UploadRequest
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&request); decodeErr != nil {
			seedanceError(c, http.StatusBadRequest, "invalid_request", "invalid Base64 upload JSON: "+decodeErr.Error())
			return
		}
		if trailingErr := decoder.Decode(&struct{}{}); !errors.Is(trailingErr, io.EOF) {
			seedanceError(c, http.StatusBadRequest, "invalid_request", "Base64 upload JSON must contain exactly one object")
			return
		}
		value := strings.TrimSpace(request.ImageBase64)
		if !strings.HasPrefix(strings.ToLower(value), "data:") {
			contentType := strings.TrimSpace(request.ContentType)
			if contentType == "" {
				seedanceError(c, http.StatusBadRequest, "content_type_required", "content_type is required for bare Base64 uploads")
				return
			}
			value = "data:" + contentType + ";base64," + value
		}
		upload, err = h.seedanceMediaService.UploadDataURI(c.Request.Context(), owner, value, true)
	default:
		seedanceError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be multipart/form-data or application/json")
		return
	}
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	uploadURL := seedanceUploadURL(c, upload.UploadID)
	if public {
		uploadURL = strings.TrimSpace(upload.MediaURL)
		if uploadURL == "" {
			seedanceError(c, http.StatusServiceUnavailable, "media_storage_error", "failed to sign Seedance media URL")
			return
		}
	}
	mediaTypeValue := strings.TrimSpace(upload.MediaType)
	if mediaTypeValue == "" {
		mediaTypeValue = seedanceMediaKindFromContentType(upload.ContentType)
	}
	if mediaTypeValue == "" {
		mediaTypeValue = "image"
	}
	response := gin.H{
		"upload_id":    upload.UploadID,
		"image_url":    uploadURL,
		"media_url":    uploadURL,
		"media_type":   mediaTypeValue,
		"content_type": upload.ContentType,
		"size":         upload.SizeBytes,
		"sha256":       upload.SHA256,
		"expires_at":   upload.ExpiresAt.Format(time.RFC3339),
	}
	if public {
		delete(response, "image_url")
	}
	c.JSON(http.StatusOK, response)
}

func (h *OpenAIGatewayHandler) SeedanceUploadedImageContent(c *gin.Context) {
	apiKey, subject, ok := h.seedanceAuthContext(c)
	if !ok {
		return
	}
	if !h.ensureSeedanceGroup(c, apiKey) {
		return
	}
	if h.seedanceMediaService == nil || !h.seedanceMediaService.SupportsManagedUploads() {
		seedanceError(c, http.StatusServiceUnavailable, "media_storage_not_configured", "Seedance media storage is not configured")
		return
	}
	owner := seedanceMediaOwner(apiKey, subject)
	mediaRelease, err := h.seedanceMediaService.AcquireMediaIO(c.Request.Context(), owner, subject.Concurrency)
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	defer mediaRelease()
	stream, err := h.seedanceMediaService.OpenManagedUpload(c.Request.Context(), owner, c.Param("upload_id"), c.GetHeader("Range"))
	if err != nil {
		writeSeedanceMediaError(c, err)
		return
	}
	h.writeSeedanceMediaStream(c, stream)
}

func (h *OpenAIGatewayHandler) SeedanceGetTask(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodGet, false, false)
}

func (h *OpenAIGatewayHandler) SeedanceCancelTask(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodDelete, false, false)
}

func (h *OpenAIGatewayHandler) SeedanceTaskContent(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodGet, true, false)
}

func (h *OpenAIGatewayHandler) SeedanceGetJob(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodGet, false, true)
}

func (h *OpenAIGatewayHandler) SeedanceDeleteJob(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodDelete, false, true)
}

func (h *OpenAIGatewayHandler) SeedanceJobContent(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodGet, true, true)
}

func (h *OpenAIGatewayHandler) SeedanceListJobs(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	apiKey, subject, ok := h.seedanceAuthContext(c)
	if !ok {
		return
	}
	reqLog := requestLogger(c, "handler.seedance.jobs",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) || !h.ensureSeedanceGroup(c, apiKey) {
		return
	}

	limit := service.DefaultSeedanceJobsLimit
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			seedanceError(c, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		limit = min(parsed, service.MaxSeedanceJobsLimit)
	}
	statusFilter := strings.ToLower(strings.TrimSpace(c.Query("status")))
	queryLimit := limit
	if statusFilter != "" {
		queryLimit = service.MaxSeedanceJobsLimit
	}
	jobs, err := h.gatewayService.ListOwnedSeedanceJobs(
		c.Request.Context(), apiKey.GroupID, subject.UserID, apiKey.ID,
		queryLimit, "",
	)
	if err != nil {
		reqLog.Error("seedance.list_jobs_failed", zap.Error(err))
		seedanceError(c, http.StatusServiceUnavailable, "task_index_unavailable", "Video task index is temporarily unavailable")
		return
	}
	if h.reconcileSeedanceListedJobs(c, reqLog, apiKey, subject, jobs, &streamStarted) {
		jobs, err = h.gatewayService.ListOwnedSeedanceJobs(
			c.Request.Context(), apiKey.GroupID, subject.UserID, apiKey.ID,
			queryLimit, "",
		)
		if err != nil {
			reqLog.Error("seedance.list_jobs_refresh_failed", zap.Error(err))
			seedanceError(c, http.StatusServiceUnavailable, "task_index_unavailable", "Video task index is temporarily unavailable")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": filterSeedanceListedJobs(jobs, statusFilter, limit)})
}

const maxSeedanceListFallbackStarts = 3

// reconcileSeedanceListedJobs gives collection-only pollers the same fallback
// and refund semantics as GET /videos/jobs/{job_id}. Work is intentionally
// bounded so one list request cannot prepare unbounded reference media.
func (h *OpenAIGatewayHandler) reconcileSeedanceListedJobs(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	jobs []map[string]any,
	streamStarted *bool,
) bool {
	if h == nil || h.gatewayService == nil || c == nil || apiKey == nil {
		return false
	}
	now := time.Now()
	fallbackAttempts := 0
	refresh := false
	for _, job := range jobs {
		status := strings.ToLower(seedanceListedJobString(job, "status"))
		if status != "failed" && status != "cancelled" && status != "queued" {
			continue
		}
		jobID := seedanceListedJobString(job, "job_id")
		if jobID == "" {
			jobID = seedanceListedJobString(job, "id")
		}
		if jobID == "" {
			continue
		}
		binding, err := h.gatewayService.GetSeedanceTaskBinding(
			c.Request.Context(), apiKey.GroupID, jobID, subject.UserID, apiKey.ID,
		)
		if err != nil || binding == nil {
			continue
		}

		if status == "cancelled" {
			h.refundSeedanceTask(c, reqLog, apiKey, subject, jobID, status)
			continue
		}

		shouldStart := status == "failed" && binding.FallbackStatus == service.SeedanceFallbackStatusReady
		shouldResume := status == "queued" && seedanceShouldResumeExpiredFallback(binding, http.MethodGet, false, now)
		if shouldStart || shouldResume {
			if fallbackAttempts >= maxSeedanceListFallbackStarts {
				continue
			}
			fallbackAttempts++
			result := h.executeSeedanceFallback(
				c, reqLog, apiKey, subject, binding, jobID, true, streamStarted, false,
			)
			refresh = true
			if result.Refund {
				h.refundSeedanceTask(c, reqLog, apiKey, subject, jobID, "failed")
			}
			continue
		}

		if status == "failed" && binding.FallbackStatus != service.SeedanceFallbackStatusStarting &&
			binding.FallbackStatus != service.SeedanceFallbackStatusCancelling {
			h.refundSeedanceTask(c, reqLog, apiKey, subject, jobID, status)
		}
	}
	return refresh
}

func seedanceListedJobString(job map[string]any, key string) string {
	if job == nil {
		return ""
	}
	value, _ := job[key].(string)
	return strings.TrimSpace(value)
}

func filterSeedanceListedJobs(jobs []map[string]any, status string, limit int) []map[string]any {
	if limit <= 0 {
		limit = service.DefaultSeedanceJobsLimit
	}
	if limit > service.MaxSeedanceJobsLimit {
		limit = service.MaxSeedanceJobsLimit
	}
	status = strings.ToLower(strings.TrimSpace(status))
	filtered := make([]map[string]any, 0, min(limit, len(jobs)))
	for _, job := range jobs {
		if status != "" && strings.ToLower(seedanceListedJobString(job, "status")) != status {
			continue
		}
		filtered = append(filtered, job)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func (h *OpenAIGatewayHandler) handleSeedanceTaskOperation(c *gin.Context, method string, content bool, public bool) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	apiKey, subject, ok := h.seedanceAuthContext(c)
	if !ok {
		return
	}
	reqLog := requestLogger(c, "handler.seedance.task",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) || !h.ensureSeedanceGroup(c, apiKey) {
		return
	}
	taskID := strings.TrimSpace(seedanceTaskIDParam(c, public))
	if taskID == "" {
		if public {
			seedanceError(c, http.StatusBadRequest, "invalid_request", "job_id is required")
		} else {
			seedanceError(c, http.StatusBadRequest, "invalid_request", "task_id is required")
		}
		return
	}
	binding, err := h.gatewayService.GetSeedanceTaskBinding(c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID)
	if err != nil || binding == nil || binding.AccountID <= 0 {
		seedanceError(c, http.StatusNotFound, "task_not_found", "Seedance task not found")
		return
	}
	boundAccountID := binding.AccountID
	upstreamTaskID := strings.TrimSpace(binding.UpstreamJobID)
	if upstreamTaskID == "" {
		upstreamTaskID = taskID
	}
	now := time.Now()
	if binding.FallbackStatus == service.SeedanceFallbackStatusStarting {
		if method == http.MethodDelete {
			seedanceError(c, http.StatusConflict, "task_not_ready", "Video fallback task is still being created")
			return
		}
		if seedanceFallbackLeaseActive(binding, now) {
			if content {
				seedanceError(c, http.StatusConflict, "task_not_ready", "Video fallback task is still being created")
				return
			}
			if method == http.MethodGet {
				writeSeedanceQueuedFallback(c, taskID, binding.Model, public)
				return
			}
		}
		// A creator whose lease expired may have lost its process after the
		// provider accepted the request. Re-enter the token-guarded fallback
		// path instead of forwarding the stale primary task and refunding it.
		if seedanceShouldResumeExpiredFallback(binding, method, content, now) {
			fallbackResult := h.tryStartSeedanceFallback(c, reqLog, apiKey, subject, binding, taskID, public, &streamStarted)
			if fallbackResult.Handled {
				return
			}
			// An explicit fallback rejection is finalized by the helper. Continue
			// through the primary status path so its failed response drives the
			// idempotent refund and normal public failure payload.
		}
	}
	if binding.FallbackStatus == service.SeedanceFallbackStatusCancelling {
		// Keep readers away from the primary task while DELETE owns the row.
		// An expired cancellation lease can be reclaimed by a subsequent DELETE.
		if method != http.MethodDelete || seedanceFallbackLeaseActive(binding, now) {
			if content {
				seedanceError(c, http.StatusConflict, "task_not_ready", "Video task cancellation is still in progress")
				return
			}
			if method == http.MethodGet {
				writeSeedanceCancellationPending(c, taskID, binding.Model, public)
				return
			}
			seedanceError(c, http.StatusConflict, "task_not_ready", "Video task cancellation is still in progress")
			return
		}
	}
	var cancellationClaimToken string
	cancellationAccepted := false
	if seedanceShouldClaimCancellation(binding, method, now) {
		claimed, claimToken, claimErr := h.gatewayService.ClaimSeedanceTaskCancellation(c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID)
		if claimErr != nil {
			reqLog.Error("seedance.cancellation_claim_failed", zap.Error(claimErr), zap.String("task_id", taskID))
			seedanceError(c, http.StatusServiceUnavailable, "cancellation_unavailable", "Video task cancellation is temporarily unavailable")
			return
		}
		if !claimed {
			latest, loadErr := h.gatewayService.GetSeedanceTaskBinding(c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID)
			if loadErr == nil && latest != nil && (latest.FallbackStatus == service.SeedanceFallbackStatusStarting || latest.FallbackStatus == service.SeedanceFallbackStatusActive || latest.FallbackStatus == service.SeedanceFallbackStatusCancelling) {
				seedanceError(c, http.StatusConflict, "task_not_ready", "Video fallback task is being created or cancelled")
				return
			}
			seedanceError(c, http.StatusConflict, "task_not_ready", "Video task state changed; retry cancellation")
			return
		}
		cancellationClaimToken = claimToken
		defer func() {
			if cancellationClaimToken == "" || cancellationAccepted {
				return
			}
			if released, releaseErr := h.gatewayService.ReleaseSeedanceTaskCancellation(context.WithoutCancel(c.Request.Context()), apiKey.GroupID, taskID, subject.UserID, apiKey.ID, cancellationClaimToken); releaseErr != nil || !released {
				reqLog.Error("seedance.cancellation_release_failed", zap.Error(releaseErr), zap.String("task_id", taskID))
			}
		}()
	}
	var selection *service.AccountSelectionResult
	owner := seedanceMediaOwner(apiKey, subject)
	if content && h.seedanceMediaService != nil && h.seedanceMediaService.IsConfigured() {
		mediaRelease, mediaErr := h.seedanceMediaService.AcquireMediaIO(c.Request.Context(), owner, subject.Concurrency)
		if mediaErr != nil {
			if infraerrors.Code(mediaErr) == http.StatusTooManyRequests {
				writeSeedanceMediaError(c, mediaErr)
				return
			}
			reqLog.Warn("seedance.media_concurrency_unavailable", zap.String("task_id", taskID))
		} else {
			defer mediaRelease()
		}
	}
	if content && h.seedanceMediaService != nil {
		cached, hit, cacheErr := h.seedanceMediaService.OpenCachedOutput(c.Request.Context(), owner, taskID, c.GetHeader("Range"))
		if cacheErr != nil {
			reqLog.Warn("seedance.output_cache_read_failed", zap.Error(cacheErr), zap.String("task_id", taskID))
		} else if hit {
			h.writeSeedanceMediaStream(c, cached)
			return
		}
	}

	sessionHash := service.SeedanceTaskSessionHash(taskID, subject.UserID, apiKey.ID)
	selection, err = h.gatewayService.SeedanceBoundTaskAccountSelection(c.Request.Context(), boundAccountID, apiKey.GroupID)
	if err != nil || selection == nil || selection.Account == nil {
		seedanceError(c, http.StatusNotFound, "task_not_found", "Seedance task not found")
		return
	}
	account := selection.Account
	setOpsSelectedAccount(c, account.ID, account.Platform)
	accountRelease, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if content && accountRelease != nil {
		defer accountRelease()
	}
	clientRange := strings.TrimSpace(c.GetHeader("Range"))
	var archiveLease *service.SeedanceOutputArchiveLease
	if content && h.seedanceMediaService != nil {
		if lease, won := h.seedanceMediaService.BeginOutputArchive(c.Request.Context(), owner, taskID); won {
			archiveLease = lease
			defer lease.Close()
		}
	}
	if archiveLease != nil {
		cached, hit, cacheErr := h.seedanceMediaService.OpenCachedOutput(c.Request.Context(), owner, taskID, clientRange)
		if cacheErr == nil && hit {
			archiveLease.Close()
			archiveLease = nil
			h.writeSeedanceMediaStream(c, cached)
			return
		}
	}
	forwarded, err := func() (*service.SeedanceUpstreamResponse, error) {
		if !content && accountRelease != nil {
			defer accountRelease()
		}
		if content && archiveLease != nil {
			return h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, upstreamTaskID, "")
		}
		return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, method, upstreamTaskID, nil)
	}()
	if err != nil {
		h.writeSeedanceForwardError(c, err)
		return
	}
	if content {
		if forwarded.BodyStream == nil {
			seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream video body is empty")
			return
		}
		defer func() {
			if forwarded != nil && forwarded.BodyStream != nil {
				_ = forwarded.BodyStream.Close()
			}
		}()
		contentLength, _ := strconv.ParseInt(strings.TrimSpace(forwarded.Header.Get("Content-Length")), 10, 64)
		canArchive := archiveLease != nil && forwarded.StatusCode == http.StatusOK && h.seedanceMediaService.CanArchiveOutput(c.Request.Context(), contentLength)
		if archiveLease != nil && !canArchive && clientRange == "" {
			archiveLease.Close()
			archiveLease = nil
		}
		if archiveLease != nil && !canArchive && clientRange != "" {
			_ = forwarded.BodyStream.Close()
			archiveLease.Close()
			archiveLease = nil
			forwarded, err = h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, upstreamTaskID, clientRange)
			if err != nil {
				h.writeSeedanceForwardError(c, err)
				return
			}
			if forwarded.BodyStream == nil {
				seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream video body is empty")
				return
			}
		}
		if canArchive {
			captured, captureErr := h.seedanceMediaService.CaptureAndStoreOutputWithLease(c.Request.Context(), archiveLease, owner, taskID, forwarded.ContentType, contentLength, forwarded.BodyStream)
			if captureErr != nil {
				if reason := infraerrors.Reason(captureErr); reason == "invalid_upstream_response" || reason == "video_too_large" {
					writeSeedanceMediaError(c, captureErr)
					return
				}
				reqLog.Warn("seedance.output_archive_capture_failed", zap.String("task_id", taskID))
				_ = forwarded.BodyStream.Close()
				archiveLease.Close()
				archiveLease = nil
				forwarded, err = h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, upstreamTaskID, clientRange)
				if err != nil {
					h.writeSeedanceForwardError(c, err)
					return
				}
				if forwarded.BodyStream == nil {
					seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream video body is empty")
					return
				}
				h.writeSeedanceBody(c, forwarded.StatusCode, forwarded.Header, forwarded.BodyStream)
				return
			}
			defer func() { _ = captured.Close() }()
			if captured.StorageError != nil {
				reqLog.Warn("seedance.output_archive_failed", zap.String("task_id", taskID))
			}
			if clientRange != "" {
				if captured.StorageError == nil {
					cached, hit, cacheErr := h.seedanceMediaService.OpenCachedOutput(c.Request.Context(), owner, taskID, clientRange)
					if cacheErr == nil && hit {
						h.writeSeedanceMediaStream(c, cached)
						return
					}
				}
				h.serveSeedanceCapturedVideo(c, captured)
				return
			}
			header := forwarded.Header.Clone()
			header.Set("Content-Type", captured.ContentType)
			header.Set("Content-Length", strconv.FormatInt(captured.SizeBytes, 10))
			h.writeSeedanceBody(c, forwarded.StatusCode, header, captured.File)
			return
		}
		h.writeSeedanceBody(c, forwarded.StatusCode, forwarded.Header, forwarded.BodyStream)
		return
	}
	if forwarded.Streamed {
		return
	}
	if method == http.MethodDelete {
		if cancellationClaimToken != "" {
			// Do not refund until the row is durably marked cancelled. If this
			// CAS loses a concurrent lease owner, leave the reservation in place
			// for recovery rather than allowing fallback creation and a free job.
			cancellationAccepted = true
			completed, completeErr := h.gatewayService.CompleteSeedanceTaskCancellation(
				context.WithoutCancel(c.Request.Context()), apiKey.GroupID, taskID,
				subject.UserID, apiKey.ID, cancellationClaimToken,
			)
			if completeErr != nil || !completed {
				reqLog.Error("seedance.cancellation_complete_failed", zap.Error(completeErr), zap.String("task_id", taskID))
				seedanceError(c, http.StatusServiceUnavailable, "cancellation_unavailable", "Video task cancellation is awaiting confirmation")
				return
			}
		}
		h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, "cancelled")
		c.Status(http.StatusNoContent)
		return
	}
	if public && !content {
		if len(forwarded.Body) == 0 {
			seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream job body is empty")
			return
		}
		status := seedanceStatusFromBody(forwarded.Body)
		if status == "failed" && binding.FallbackStatus == service.SeedanceFallbackStatusReady {
			fallbackResult := h.tryStartSeedanceFallback(c, reqLog, apiKey, subject, binding, taskID, public, &streamStarted)
			if fallbackResult.Handled {
				if fallbackResult.Refund {
					h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, status)
				}
				return
			}
		}
		if status == "failed" || status == "cancelled" {
			h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, status)
		}
		normalizedBody, normalizeErr := service.NormalizeSeedanceJobForRoute(forwarded.Body, taskID, account.GetVideoProvider(), binding.Model)
		if normalizeErr != nil {
			seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", normalizeErr.Error())
			return
		}
		header := forwarded.Header.Clone()
		header.Del("Content-Length")
		h.writeSeedanceBody(c, forwarded.StatusCode, header, bytes.NewReader(normalizedBody))
		return
	}
	official, err := service.BuildSeedanceOfficialTaskResponseForRoute(taskID, forwarded.Body, seedanceTaskContentURL(c, taskID), account.GetVideoProvider(), binding.Model)
	if err != nil {
		seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", err.Error())
		return
	}
	status, _ := official["status"].(string)
	if status == "failed" && binding.FallbackStatus == service.SeedanceFallbackStatusReady {
		fallbackResult := h.tryStartSeedanceFallback(c, reqLog, apiKey, subject, binding, taskID, public, &streamStarted)
		if fallbackResult.Handled {
			if fallbackResult.Refund {
				h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, status)
			}
			return
		}
	}
	if status == "failed" || status == "cancelled" {
		h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, status)
	}
	c.JSON(http.StatusOK, official)
}

type seedanceFallbackAttemptResult struct {
	// Handled means the helper already wrote the response and the caller must
	// stop processing the primary provider response.
	Handled bool
	// Refund is true only when no fallback request was accepted and the primary
	// task can be finalized as failed.
	Refund  bool
	Outcome seedanceFallbackOutcome
}

type seedanceFallbackOutcome uint8

const (
	seedanceFallbackOutcomeNone seedanceFallbackOutcome = iota
	seedanceFallbackOutcomeQueued
	seedanceFallbackOutcomeAcceptanceUnknown
	seedanceFallbackOutcomeRetryableLocal
	seedanceFallbackOutcomeExplicitRejected
)

func (h *OpenAIGatewayHandler) tryStartSeedanceFallback(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	binding *service.SeedanceTaskBinding,
	publicTaskID string,
	public bool,
	streamStarted *bool,
) (result seedanceFallbackAttemptResult) {
	return h.executeSeedanceFallback(c, reqLog, apiKey, subject, binding, publicTaskID, public, streamStarted, true)
}

// executeSeedanceFallback owns the fallback state transition. The list route
// uses it without a presenter so polling the collection has the same side
// effects as polling an individual task without corrupting the list response.
func (h *OpenAIGatewayHandler) executeSeedanceFallback(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	binding *service.SeedanceTaskBinding,
	publicTaskID string,
	public bool,
	streamStarted *bool,
	respond bool,
) (result seedanceFallbackAttemptResult) {
	if h == nil || h.gatewayService == nil || h.seedanceMediaService == nil || c == nil || apiKey == nil || binding == nil {
		if respond && c != nil {
			seedanceError(c, http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback is temporarily unavailable; retry this task")
		}
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
	}
	responseWritten := false
	writeError := func(status int, code, message string) {
		if !respond {
			return
		}
		seedanceError(c, status, code, message)
		responseWritten = true
	}
	writeQueued := func(model string) {
		if !respond {
			return
		}
		writeSeedanceQueuedFallback(c, publicTaskID, model, public)
		responseWritten = true
	}
	writeCancelling := func(model string) {
		if !respond {
			return
		}
		writeSeedanceCancellationPending(c, publicTaskID, model, public)
		responseWritten = true
	}
	writeCancelled := func(model string) {
		if !respond {
			return
		}
		writeSeedanceTaskState(c, publicTaskID, model, public, "cancelled")
		responseWritten = true
	}

	claimed, claimToken, err := h.gatewayService.ClaimSeedanceTaskFallback(c.Request.Context(), apiKey.GroupID, publicTaskID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Error("seedance.fallback_claim_failed", zap.Error(err), zap.String("task_id", publicTaskID))
		writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback is temporarily unavailable; retry this task")
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
	}
	if !claimed {
		latest, loadErr := h.gatewayService.GetSeedanceTaskBinding(c.Request.Context(), apiKey.GroupID, publicTaskID, subject.UserID, apiKey.ID)
		if loadErr == nil && latest != nil {
			switch latest.FallbackStatus {
			case service.SeedanceFallbackStatusStarting, service.SeedanceFallbackStatusActive:
				writeQueued(latest.Model)
				return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeQueued}
			case service.SeedanceFallbackStatusCancelling:
				writeCancelling(latest.Model)
				return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeQueued}
			case service.SeedanceFallbackStatusCancelled:
				writeCancelled(latest.Model)
				return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeQueued}
			case service.SeedanceFallbackStatusFailed:
				return seedanceFallbackAttemptResult{Refund: true, Outcome: seedanceFallbackOutcomeExplicitRejected}
			}
		}
		if loadErr != nil {
			writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback state is temporarily unavailable; retry this task")
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
		}
		writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback state changed; retry this task")
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
	}
	fallbackCtx, stopFallbackRenewal := service.MaintainSeedanceFallbackLease(
		c.Request.Context(),
		func(renewCtx context.Context) (bool, error) {
			return h.gatewayService.RenewSeedanceTaskFallback(
				renewCtx, apiKey.GroupID, publicTaskID, subject.UserID, apiKey.ID, claimToken,
			)
		},
	)
	originalRequest := c.Request
	c.Request = c.Request.WithContext(fallbackCtx)
	defer func() {
		c.Request = originalRequest
		if renewErr := stopFallbackRenewal(); renewErr != nil && originalRequest.Context().Err() == nil {
			reqLog.Warn("seedance.fallback_lease_lost", zap.Error(renewErr), zap.String("task_id", publicTaskID))
		}
	}()

	claimFinalized := false
	defer func() {
		if claimFinalized {
			return
		}
		var updated bool
		var finalizeErr error
		switch result.Outcome {
		case seedanceFallbackOutcomeExplicitRejected:
			updated, finalizeErr = h.gatewayService.FailSeedanceTaskFallback(
				context.WithoutCancel(c.Request.Context()), apiKey.GroupID, publicTaskID,
				subject.UserID, apiKey.ID, claimToken,
			)
		case seedanceFallbackOutcomeRetryableLocal:
			updated, finalizeErr = h.gatewayService.ReleaseSeedanceTaskFallback(
				context.WithoutCancel(c.Request.Context()), apiKey.GroupID, publicTaskID,
				subject.UserID, apiKey.ID, claimToken,
			)
		default:
			// Active jobs have already consumed the claim. Acceptance-unknown jobs
			// deliberately retain it until the lease is recovered with the same key.
			return
		}
		if finalizeErr != nil || !updated {
			reqLog.Error("seedance.fallback_finalize_failed", zap.Error(finalizeErr), zap.String("task_id", publicTaskID))
			result = seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
			if !responseWritten {
				writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback state is temporarily unavailable; retry this task")
			}
			return
		}
		claimFinalized = true
		if result.Outcome == seedanceFallbackOutcomeRetryableLocal && !responseWritten {
			writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback is temporarily unavailable; retry this task")
			result.Handled = true
		}
		if result.Outcome == seedanceFallbackOutcomeExplicitRejected && !result.Refund {
			result.Refund = true
		}
		if !responseWritten && result.Handled && respond {
			latest, loadErr := h.gatewayService.GetSeedanceTaskBinding(context.WithoutCancel(c.Request.Context()), apiKey.GroupID, publicTaskID, subject.UserID, apiKey.ID)
			if loadErr != nil || latest == nil || (latest.FallbackStatus != service.SeedanceFallbackStatusStarting && latest.FallbackStatus != service.SeedanceFallbackStatusActive) {
				writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback state changed; retry this task")
				return
			}
			writeQueued(latest.Model)
		}
	}()

	requestInfo, err := service.RestoreSeedanceFallbackRequest(binding.RequestSnapshot, binding.FallbackModel)
	if err != nil {
		reqLog.Error("seedance.fallback_restore_failed", zap.Error(err), zap.String("task_id", publicTaskID))
		writeError(http.StatusServiceUnavailable, "fallback_request_invalid", "Video fallback request could not be restored; retry this task")
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
	}
	mediaRelease, err := h.seedanceMediaService.AcquireMediaIO(c.Request.Context(), seedanceMediaOwner(apiKey, subject), subject.Concurrency)
	if err != nil {
		reqLog.Warn("seedance.fallback_media_slot_failed", zap.Error(err), zap.String("task_id", publicTaskID))
		writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback media preparation is temporarily unavailable; retry this task")
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
	}
	defer mediaRelease()
	if requestInfo.HasReferenceMedia() {
		requestInfo.HuiquMedia, err = h.seedanceMediaService.PrepareHuiquMedia(c.Request.Context(), seedanceMediaOwner(apiKey, subject), requestInfo)
		if err != nil {
			reqLog.Warn("seedance.fallback_media_prepare_failed", zap.Error(err), zap.String("task_id", publicTaskID))
			writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback reference media is temporarily unavailable; retry this task")
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
		}
		defer requestInfo.HuiquMedia.Cleanup()
	}

	failedAccountIDs := make(map[int64]struct{})
	explicitRejections := 0
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	sessionHash := service.SeedanceTaskSessionHash(publicTaskID+":fallback", subject.UserID, apiKey.ID)
	for switchCount := 0; switchCount <= maxSwitches; switchCount++ {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", sessionHash, binding.FallbackModel,
			failedAccountIDs, service.OpenAIUpstreamTransportHTTPSSE,
			"", false, false, false, apiKey.Group.Platform,
		)
		if selectErr != nil || selection == nil || selection.Account == nil || !selection.Account.IsHuiquVideo() {
			if selection != nil && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if explicitRejections > 0 {
				return seedanceFallbackAttemptResult{Refund: true, Outcome: seedanceFallbackOutcomeExplicitRejected}
			}
			writeError(http.StatusServiceUnavailable, "fallback_unavailable", "No compatible fallback account is currently available; retry this task")
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
		}
		account := selection.Account
		var accountRelease func()
		var acquired bool
		if respond {
			accountRelease, acquired = h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, streamStarted, reqLog)
			if !acquired {
				responseWritten = true
			}
		} else {
			accountRelease, acquired = h.tryAcquireSeedanceFallbackAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqLog)
		}
		if !acquired {
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
		}
		forwarded, forwardErr := func() (*service.SeedanceUpstreamResponse, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			oldKey := c.Request.Header.Get("Idempotency-Key")
			c.Request.Header.Set("Idempotency-Key", "seedance-fallback-"+publicTaskID)
			defer func() {
				if oldKey == "" {
					c.Request.Header.Del("Idempotency-Key")
				} else {
					c.Request.Header.Set("Idempotency-Key", oldKey)
				}
			}()
			return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, http.MethodPost, "", requestInfo)
		}()
		if forwardErr != nil {
			var unknownAcceptanceErr *service.SeedanceUpstreamAcceptanceUnknownError
			if errors.As(forwardErr, &unknownAcceptanceErr) {
				// The provider may already be running the task. Keep the claim/lease
				// so a later poll can retry with the same idempotency key; never
				// refund while acceptance remains unknown.
				writeQueued(binding.Model)
				return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeAcceptanceUnknown}
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				failedAccountIDs[account.ID] = struct{}{}
				explicitRejections++
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(binding.FallbackModel), false, nil)
				continue
			}
			reqLog.Warn("seedance.fallback_forward_failed", zap.Error(forwardErr), zap.String("task_id", publicTaskID), zap.Int64("account_id", account.ID))
			writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback request is temporarily unavailable; retry this task")
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
		}
		if forwarded == nil || forwarded.Result == nil || strings.TrimSpace(forwarded.Result.ResponseID) == "" {
			// The provider may have accepted the request before the gateway lost
			// the response. Keep the claim recoverable instead of refunding it.
			writeQueued(binding.Model)
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeAcceptanceUnknown}
		}
		activated, activateErr := h.gatewayService.ActivateSeedanceTaskFallback(
			c.Request.Context(), apiKey.GroupID, publicTaskID, subject.UserID, apiKey.ID,
			claimToken, account.ID, forwarded.Result.ResponseID,
		)
		if activateErr != nil || !activated {
			reqLog.Error("seedance.fallback_activate_failed", zap.Error(activateErr), zap.String("task_id", publicTaskID), zap.Int64("account_id", account.ID))
			writeQueued(binding.Model)
			return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeAcceptanceUnknown}
		}
		claimFinalized = true
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, forwarded.Result.UpstreamModel, true, nil)
		h.gatewayService.RecordOpenAIAccountSwitch()
		reqLog.Info("seedance.fallback_activated",
			zap.String("task_id", publicTaskID),
			zap.String("requested_model", binding.Model),
			zap.Int64("fallback_account_id", account.ID),
		)
		writeQueued(binding.Model)
		return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeQueued}
	}
	if explicitRejections > 0 {
		return seedanceFallbackAttemptResult{Refund: true, Outcome: seedanceFallbackOutcomeExplicitRejected}
	}
	writeError(http.StatusServiceUnavailable, "fallback_unavailable", "Video fallback is temporarily unavailable; retry this task")
	return seedanceFallbackAttemptResult{Handled: true, Outcome: seedanceFallbackOutcomeRetryableLocal}
}

// tryAcquireSeedanceFallbackAccountSlot is the non-blocking list-poll variant.
// A collection request must never wait once per listed task or write an item
// error into the collection response; lack of capacity simply returns the
// fallback claim to ready for a later poll.
func (h *OpenAIGatewayHandler) tryAcquireSeedanceFallbackAccountSlot(
	c *gin.Context,
	groupID *int64,
	sessionHash string,
	selection *service.AccountSelectionResult,
	reqLog *zap.Logger,
) (func(), bool) {
	if h == nil || h.concurrencyHelper == nil || c == nil || selection == nil || selection.Account == nil {
		return nil, false
	}
	ctx := c.Request.Context()
	account := selection.Account
	if selection.Acquired {
		return wrapReleaseOnDone(ctx, selection.ReleaseFunc), true
	}
	if selection.WaitPlan == nil {
		return nil, false
	}
	release, acquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
		ctx,
		account.ID,
		selection.WaitPlan.MaxConcurrency,
	)
	if err != nil {
		reqLog.Warn("seedance.list_fallback_slot_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return nil, false
	}
	if !acquired {
		return nil, false
	}
	if err := h.gatewayService.BindStickySession(ctx, groupID, sessionHash, account.ID); err != nil {
		reqLog.Warn("seedance.list_fallback_sticky_bind_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
	return wrapReleaseOnDone(ctx, release), true
}

func writeSeedanceQueuedFallback(c *gin.Context, taskID, model string, public bool) {
	model = service.PublicSeedanceModelID(model)
	if public {
		c.JSON(http.StatusOK, gin.H{
			"job_id":     taskID,
			"status":     "queued",
			"status_url": service.SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID),
			"model":      model,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": taskID, "status": "queued", "model": model})
}

func writeSeedanceCancellationPending(c *gin.Context, taskID, model string, public bool) {
	writeSeedanceTaskState(c, taskID, model, public, "running")
}

func writeSeedanceTaskState(c *gin.Context, taskID, model string, public bool, status string) {
	status = strings.TrimSpace(status)
	model = service.PublicSeedanceModelID(model)
	if public {
		c.JSON(http.StatusOK, gin.H{
			"job_id":     taskID,
			"status":     status,
			"status_url": service.SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID),
			"model":      model,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": taskID, "status": status, "model": model})
}

func seedanceFallbackLeaseActive(binding *service.SeedanceTaskBinding, now time.Time) bool {
	if binding == nil || binding.FallbackLeaseUntil.IsZero() {
		return true
	}
	return !now.After(binding.FallbackLeaseUntil)
}

func seedanceShouldResumeExpiredFallback(binding *service.SeedanceTaskBinding, method string, content bool, now time.Time) bool {
	return binding != nil && binding.FallbackStatus == service.SeedanceFallbackStatusStarting &&
		method == http.MethodGet && !content && !seedanceFallbackLeaseActive(binding, now)
}

func seedanceShouldClaimCancellation(binding *service.SeedanceTaskBinding, method string, now time.Time) bool {
	if binding == nil || method != http.MethodDelete {
		return false
	}
	if binding.FallbackStatus == service.SeedanceFallbackStatusReady {
		return true
	}
	return binding.FallbackStatus == service.SeedanceFallbackStatusCancelling && !seedanceFallbackLeaseActive(binding, now)
}

func (h *OpenAIGatewayHandler) serveSeedanceCapturedVideo(c *gin.Context, captured *service.SeedanceCapturedVideo) {
	if captured == nil || captured.File == nil {
		seedanceError(c, http.StatusBadGateway, "media_storage_error", "Seedance video is unavailable")
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", `inline; filename="seedance.mp4"`)
	http.ServeContent(c.Writer, c.Request, "seedance.mp4", time.Time{}, captured.File)
}

func (h *OpenAIGatewayHandler) writeSeedanceMediaStream(c *gin.Context, stream *service.SeedanceMediaStream) {
	if stream == nil || stream.Body == nil {
		seedanceError(c, http.StatusBadGateway, "media_storage_error", "Seedance media stream is unavailable")
		return
	}
	defer func() { _ = stream.Body.Close() }()
	h.writeSeedanceBody(c, stream.StatusCode, stream.Header, stream.Body)
}

func (h *OpenAIGatewayHandler) writeSeedanceBody(c *gin.Context, status int, header http.Header, body io.Reader) {
	if h.gatewayService != nil {
		h.gatewayService.WriteSeedanceContentResponseHeaders(c.Writer.Header(), header)
	} else {
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Accept-Ranges", "Content-Range", "ETag", "Last-Modified"} {
			if value := strings.TrimSpace(header.Get(name)); value != "" {
				c.Header(name, value)
			}
		}
	}
	if contentType := strings.TrimSpace(header.Get("Content-Type")); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Status(status)
	if _, err := io.CopyBuffer(c.Writer, body, make([]byte, 32<<10)); err != nil {
		_ = c.Error(fmt.Errorf("stream Seedance media response: %w", err))
	}
}

func writeSeedanceMediaError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := strings.TrimSpace(infraerrors.Reason(err))
	message := strings.TrimSpace(infraerrors.Message(err))
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	if code == "" {
		code = "media_storage_error"
	}
	if message == "" || message == infraerrors.UnknownMessage {
		message = "Seedance media request failed"
	}
	seedanceError(c, status, code, message)
}

func (h *OpenAIGatewayHandler) refundSeedanceTask(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	taskID string,
	status string,
) {
	result, err := h.gatewayService.RefundSeedanceUsage(c.Request.Context(), taskID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Error("seedance.refund_failed", zap.Error(err), zap.String("task_id", taskID), zap.String("status", status))
		return
	}
	if result == nil || !result.Applied {
		return
	}
	if h.apiKeyService != nil {
		h.apiKeyService.InvalidateAuthCacheByKey(c.Request.Context(), apiKey.Key)
	}
	reqLog.Info("seedance.refunded",
		zap.String("task_id", taskID),
		zap.String("status", status),
		zap.Int64("usage_log_id", result.UsageLogID),
		zap.Float64("refunded_cost", result.RefundedCost),
	)
}

func (h *OpenAIGatewayHandler) seedanceAuthContext(c *gin.Context) (*service.APIKey, middleware2.AuthSubject, bool) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		seedanceError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return nil, middleware2.AuthSubject{}, false
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		seedanceError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return nil, middleware2.AuthSubject{}, false
	}
	return apiKey, subject, true
}

func (h *OpenAIGatewayHandler) ensureSeedanceGroup(c *gin.Context, apiKey *service.APIKey) bool {
	if apiKey == nil || apiKey.GroupID == nil || apiKey.Group == nil {
		seedanceError(c, http.StatusForbidden, "permission_denied", "API key must be assigned to a Seedance-enabled group")
		return false
	}
	if !service.IsFFLinkVideoPlatform(apiKey.Group.Platform) {
		seedanceError(c, http.StatusForbidden, "permission_denied", "API key group does not support video generation")
		return false
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		seedanceError(c, http.StatusForbidden, "permission_denied", "Video generation is disabled for this API key group")
		return false
	}
	return true
}

func seedanceVideoPricingError(group *service.Group, requestedModel, resolution string) (int, string, string) {
	if group == nil {
		return http.StatusServiceUnavailable, "billing_not_configured", "Video pricing is not configured"
	}
	if service.IsFFLinkVideoPlatform(group.Platform) && len(group.VideoModelPrices) > 0 {
		model := strings.ToLower(strings.TrimSpace(requestedModel))
		if _, ok := group.VideoModelPrices[model]; !ok {
			return http.StatusBadRequest, "model_not_supported", "The requested model is not configured for this video group"
		}
	}
	if group.GetVideoPriceForModel(requestedModel, resolution) == nil {
		return http.StatusServiceUnavailable, "billing_not_configured", "Video price per second is not configured for the requested model and resolution"
	}
	return 0, "", ""
}

func (h *OpenAIGatewayHandler) writeSeedanceForwardError(c *gin.Context, err error) {
	var upstreamErr *service.SeedanceUpstreamError
	if errors.As(err, &upstreamErr) {
		status := upstreamErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		// Keep user-readable upstream validation messages (e.g. resolution limits),
		// but map codes to platform-owned values and scrub vendor names.
		code, message := service.SeedancePublicUpstreamError(status, upstreamErr.Body)
		seedanceError(c, status, code, message)
		return
	}
	seedanceError(c, http.StatusBadGateway, "upstream_error", "Video request failed")
}

func seedanceError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"code":    strings.TrimSpace(code),
		"message": strings.TrimSpace(message),
		"type":    "invalid_request_error",
	}})
}

func seedanceTaskContentURL(c *gin.Context, taskID string) string {
	path := service.SeedanceOfficialTasksEndpoint + "/" + url.PathEscape(taskID) + "/content"
	return seedanceAbsoluteURL(c, path)
}

func seedanceTaskIDParam(c *gin.Context, public bool) string {
	if c == nil {
		return ""
	}
	if public {
		if value := strings.TrimSpace(c.Param("job_id")); value != "" {
			return value
		}
	}
	return strings.TrimSpace(c.Param("task_id"))
}

func seedanceStatusFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if status, ok := payload["status"].(string); ok {
		return service.MapSeedanceTaskStatus(status)
	}
	return ""
}

func seedanceUploadURL(c *gin.Context, uploadID string) string {
	path := service.SeedanceOfficialUploadsEndpoint + "/" + url.PathEscape(uploadID)
	return seedanceAbsoluteURL(c, path)
}

func seedancePublicUploadURL(c *gin.Context, uploadID string) string {
	path := service.SeedancePublicUploadsEndpoint + "/" + url.PathEscape(uploadID)
	return seedanceAbsoluteURL(c, path)
}

func seedanceAbsoluteURL(c *gin.Context, path string) string {
	if c == nil || c.Request == nil {
		return path
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	if c.Request.Host == "" {
		return path
	}
	return fmt.Sprintf("%s://%s%s", scheme, c.Request.Host, path)
}

func seedanceMediaKindFromContentType(contentType string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/"):
		return "video"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "audio/"):
		return "audio"
	default:
		return ""
	}
}

func seedanceMediaOwner(apiKey *service.APIKey, subject middleware2.AuthSubject) service.SeedanceMediaOwner {
	owner := service.SeedanceMediaOwner{UserID: subject.UserID}
	if apiKey == nil {
		return owner
	}
	owner.APIKeyID = apiKey.ID
	if apiKey.GroupID != nil {
		owner.GroupID = *apiKey.GroupID
	}
	return owner
}

func recordSeedanceUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	requestModel string,
	body []byte,
) error {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	// The task ID is returned only after its provisional charge and usage row are
	// durable. This prevents an immediate terminal-status poll from racing ahead
	// of the refundable usage record.
	return h.gatewayService.RecordSeedanceUsage(c.Request.Context(), &service.SeedanceRecordUsageInput{
		OpenAIRecordUsageInput: service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: service.HashUsageRequestPayload(body),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: service.ChannelUsageFields{OriginalModel: requestModel, ChannelMappedModel: requestModel},
		},
		TaskID:         result.ResponseID,
		RequestedModel: requestModel,
	})
}
