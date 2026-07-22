package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

	imageRelease, acquired := h.acquireImageGenerationSlot(c, streamStarted)
	if !acquired {
		return
	}
	if imageRelease != nil {
		defer imageRelease()
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		seedanceError(c, status, code, message)
		return
	}

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
		c.JSON(http.StatusOK, gin.H{"id": result.ResponseID})
		return
	}
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
	forwarded, err := func() (*service.SeedanceUpstreamResponse, error) {
		if accountRelease != nil {
			defer accountRelease()
		}
		return h.gatewayService.ForwardSeedance(c.Request.Context(), c, account, method, taskID, nil)
	}()
	if err != nil {
		h.writeSeedanceForwardError(c, err)
		return
	}
	if content || forwarded.Streamed {
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
