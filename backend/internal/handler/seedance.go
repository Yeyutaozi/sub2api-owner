package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SeedanceCreateTask accepts the Volcengine Ark task format and forwards it to
// the FYLink asynchronous video API configured on the selected Seedance account.
func (h *OpenAIGatewayHandler) SeedanceCreateTask(c *gin.Context) {
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
	requestInfo, err := service.ParseSeedanceCreateRequest(body)
	if err != nil {
		seedanceError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if status, code, message := seedanceVideoPricingError(apiKey.Group, requestInfo.Model, requestInfo.Resolution); status != 0 {
		seedanceError(c, status, code, message)
		return
	}

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

	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	failedAccountIDs := make(map[int64]struct{})
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", sessionHash, requestInfo.Model,
			failedAccountIDs, service.OpenAIUpstreamTransportHTTPSSE,
			"", false, false, false, service.PlatformSeedance,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if lastFailover != nil {
				h.handleFailoverExhausted(c, lastFailover, false)
				return
			}
			markOpsRoutingCapacityLimited(c)
			seedanceError(c, http.StatusServiceUnavailable, "no_available_account", "No available Seedance upstream account in this API key group")
			return
		}

		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountRelease, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}
		forwarded, forwardErr := func() (*service.SeedanceUpstreamResponse, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, http.MethodPost, "", requestInfo)
		}()
		if forwardErr != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestInfo.Model), false, nil)
				failedAccountIDs[account.ID] = struct{}{}
				lastFailover = failoverErr
				if switchCount >= maxSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				h.gatewayService.RecordOpenAIAccountSwitch()
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestInfo.Model), false, nil)
			h.writeSeedanceForwardError(c, forwardErr)
			return
		}

		result := forwarded.Result
		if result == nil || strings.TrimSpace(result.ResponseID) == "" {
			seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream did not return a task id")
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, result.UpstreamModel, true, nil)
		if err := h.gatewayService.BindSeedanceTaskAccount(c.Request.Context(), apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID); err != nil {
			reqLog.Error("seedance.bind_task_failed", zap.Error(err), zap.String("task_id", result.ResponseID), zap.Int64("account_id", account.ID))
			seedanceError(c, http.StatusBadGateway, "task_binding_failed", "Seedance task was accepted upstream but could not be registered locally")
			return
		}
		recordSeedanceUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestInfo.Model, body)
		if materialized != nil {
			materialized.Retain()
		}
		c.JSON(http.StatusOK, gin.H{"id": result.ResponseID})
		return
	}
}

type seedanceBase64UploadRequest struct {
	ImageBase64 string `json:"image_base64"`
	ContentType string `json:"content_type,omitempty"`
	Filename    string `json:"filename,omitempty"`
}

func (h *OpenAIGatewayHandler) SeedanceUploadImage(c *gin.Context) {
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.SeedanceMaxImageBytes+service.SeedanceUploadBodyOverhead)
		file, formErr := c.FormFile("image")
		if formErr != nil {
			if maxErr, isMax := extractMaxBytesError(formErr); isMax {
				seedanceError(c, http.StatusRequestEntityTooLarge, "image_too_large", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			seedanceError(c, http.StatusBadRequest, "image_required", "multipart field image is required")
			return
		}
		if file.Size > service.SeedanceMaxImageBytes {
			seedanceError(c, http.StatusRequestEntityTooLarge, "image_too_large", "image must not exceed 10 MiB")
			return
		}
		source, openErr := file.Open()
		if openErr != nil {
			seedanceError(c, http.StatusBadRequest, "invalid_image", "failed to open uploaded image")
			return
		}
		defer func() { _ = source.Close() }()
		upload, err = h.seedanceMediaService.UploadImage(c.Request.Context(), service.SeedanceImageUploadInput{
			Owner:       owner,
			Body:        source,
			SizeBytes:   file.Size,
			ContentType: file.Header.Get("Content-Type"),
			Persistent:  true,
		})
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
	c.JSON(http.StatusOK, gin.H{
		"upload_id":    upload.UploadID,
		"image_url":    seedanceUploadURL(c, upload.UploadID),
		"content_type": upload.ContentType,
		"size":         upload.SizeBytes,
		"sha256":       upload.SHA256,
		"expires_at":   upload.ExpiresAt.Format(time.RFC3339),
	})
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
	h.handleSeedanceTaskOperation(c, http.MethodGet, false)
}

func (h *OpenAIGatewayHandler) SeedanceCancelTask(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodDelete, false)
}

func (h *OpenAIGatewayHandler) SeedanceTaskContent(c *gin.Context) {
	h.handleSeedanceTaskOperation(c, http.MethodGet, true)
}

func (h *OpenAIGatewayHandler) handleSeedanceTaskOperation(c *gin.Context, method string, content bool) {
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
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		seedanceError(c, http.StatusBadRequest, "invalid_request", "task_id is required")
		return
	}
	boundAccountID, err := h.gatewayService.ResolveSeedanceTaskAccount(c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID)
	if err != nil || boundAccountID <= 0 {
		seedanceError(c, http.StatusNotFound, "task_not_found", "Seedance task not found")
		return
	}
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
	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		c.Request.Context(), apiKey.GroupID, "", sessionHash, "", nil,
		service.OpenAIUpstreamTransportHTTPSSE, "",
		false, false, false, service.PlatformSeedance,
	)
	if err != nil || selection == nil || selection.Account == nil || selection.Account.ID != boundAccountID {
		if selection != nil && selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
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
			return h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, taskID, "")
		}
		return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, method, taskID, nil)
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
			forwarded, err = h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, taskID, clientRange)
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
				forwarded, err = h.gatewayService.ForwardSeedanceContent(c.Request.Context(), c, account, taskID, clientRange)
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
		h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, "cancelled")
		c.Status(http.StatusNoContent)
		return
	}
	official, err := service.BuildSeedanceOfficialTaskResponse(taskID, forwarded.Body, seedanceTaskContentURL(c, taskID))
	if err != nil {
		seedanceError(c, http.StatusBadGateway, "invalid_upstream_response", err.Error())
		return
	}
	if status, _ := official["status"].(string); status == "failed" || status == "cancelled" {
		h.refundSeedanceTask(c, reqLog, apiKey, subject, taskID, status)
	}
	c.JSON(http.StatusOK, official)
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
	if apiKey.Group.Platform != service.PlatformSeedance {
		seedanceError(c, http.StatusForbidden, "permission_denied", "API key group does not support Seedance")
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
	if group.Platform == service.PlatformSeedance && len(group.VideoModelPrices) > 0 {
		model := strings.ToLower(strings.TrimSpace(requestedModel))
		if _, ok := group.VideoModelPrices[model]; !ok {
			return http.StatusBadRequest, "model_not_supported", "The requested model is not configured for this Seedance group"
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
		seedanceError(c, status, "upstream_error", service.SeedanceUpstreamErrorMessage(upstreamErr.Body))
		return
	}
	seedanceError(c, http.StatusBadGateway, "upstream_error", "Seedance upstream request failed")
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

func seedanceUploadURL(c *gin.Context, uploadID string) string {
	path := service.SeedanceOfficialUploadsEndpoint + "/" + url.PathEscape(uploadID)
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
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	requestModel string,
	body []byte,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	// The task ID is returned only after its provisional charge and usage row are
	// durable. This prevents an immediate terminal-status poll from racing ahead
	// of the refundable usage record.
	if err := h.gatewayService.RecordSeedanceUsage(c.Request.Context(), &service.SeedanceRecordUsageInput{
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
	}); err != nil {
		logger.L().With(
			zap.String("component", "handler.seedance"),
			zap.Int64("user_id", subject.UserID),
			zap.Int64("api_key_id", apiKey.ID),
			zap.String("model", requestModel),
			zap.Int64("account_id", account.ID),
		).Error("seedance.record_usage_failed", zap.Error(err))
		reqLog.Debug("seedance.record_usage_failed", zap.Error(err))
	}
}
