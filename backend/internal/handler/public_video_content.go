package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type publicVideoContentRoute int

const publicVideoContentAccountContextKey = "public_video_content_account"

const (
	publicVideoContentRouteUnknown publicVideoContentRoute = iota
	publicVideoContentRouteSeedanceJob
	publicVideoContentRouteSeedanceTask
	publicVideoContentRouteGeneric
)

// PublicVideoContent resolves an opaque task ID back to its creation context.
// It deliberately delegates to the existing content handlers so Range,
// archival, provider affinity, and deferred completion billing stay unchanged.
func (h *OpenAIGatewayHandler) PublicVideoContent(c *gin.Context) {
	if h == nil || h.gatewayService == nil || h.apiKeyService == nil || c == nil || c.Request == nil {
		publicVideoContentFallbackOrNotFound(c)
		return
	}

	route := classifyPublicVideoContentRoute(c.FullPath())
	requestID := publicVideoContentRequestID(c, route)
	if requestID == "" {
		publicVideoContentFallbackOrNotFound(c)
		return
	}

	var (
		binding *service.PublicVideoContentBinding
		err     error
	)
	switch route {
	case publicVideoContentRouteSeedanceJob, publicVideoContentRouteSeedanceTask:
		binding, err = h.gatewayService.ResolvePublicSeedanceTaskBinding(c.Request.Context(), requestID)
	case publicVideoContentRouteGeneric:
		binding, err = h.gatewayService.ResolvePublicVideoContentBinding(c.Request.Context(), requestID)
	default:
		publicVideoContentFallbackOrNotFound(c)
		return
	}
	if err != nil || binding == nil {
		if err != nil && !errors.Is(err, service.ErrPublicVideoContentBindingNotFound) {
			requestLogger(c, "handler.public_video_content").Warn("public_video_content.resolve_failed", zap.Error(err))
		}
		publicVideoContentFallbackOrNotFound(c)
		return
	}

	apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), binding.APIKeyID)
	if err != nil || !publicVideoBindingMatchesAPIKey(binding, apiKey) {
		publicVideoContentFallbackOrNotFound(c)
		return
	}

	var subscription *service.UserSubscription
	if h.subscriptionService != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		subscription, _ = h.subscriptionService.GetActiveSubscription(c.Request.Context(), binding.UserID, binding.GroupID)
	}
	restorePublicVideoAuthContext(c, apiKey, subscription, binding.Provider)
	if !h.restorePublicVideoAccountBinding(c.Request.Context(), binding) {
		publicVideoContentFallbackOrNotFound(c)
		return
	}
	if binding.Provider != service.PublicVideoProviderSeedance {
		account, accountErr := h.gatewayService.ResolvePublicVideoContentAccount(c.Request.Context(), binding)
		if accountErr != nil || account == nil {
			publicVideoContentFallbackOrNotFound(c)
			return
		}
		c.Set(publicVideoContentAccountContextKey, account)
	}
	// The public binding is authoritative. Stop the route chain before the
	// legacy API-key fallback, including when callers send a stale credential.
	c.Abort()

	switch route {
	case publicVideoContentRouteSeedanceJob:
		if binding.Provider != service.PublicVideoProviderSeedance {
			publicVideoContentNotFound(c)
			return
		}
		h.SeedanceJobContent(c)
	case publicVideoContentRouteSeedanceTask:
		if binding.Provider != service.PublicVideoProviderSeedance {
			publicVideoContentNotFound(c)
			return
		}
		h.SeedanceTaskContent(c)
	case publicVideoContentRouteGeneric:
		switch binding.Provider {
		case service.PublicVideoProviderGrok:
			h.GrokVideoContent(c)
		case service.PublicVideoProviderOpenAI:
			h.Media(c)
		default:
			publicVideoContentNotFound(c)
		}
	}
}

func publicVideoContentFallbackOrNotFound(c *gin.Context) {
	if publicVideoContentHasLegacyCredential(c) {
		return
	}
	publicVideoContentNotFound(c)
	if c != nil {
		c.Abort()
	}
}

func publicVideoContentHasLegacyCredential(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return c.GetHeader("Authorization") != "" ||
		c.GetHeader("x-api-key") != "" ||
		c.GetHeader("x-goog-api-key") != "" ||
		strings.TrimSpace(c.Query("key")) != "" ||
		strings.TrimSpace(c.Query("api_key")) != ""
}

func publicVideoContentAccount(c *gin.Context) (*service.Account, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(publicVideoContentAccountContextKey)
	if !exists {
		return nil, false
	}
	account, ok := value.(*service.Account)
	return account, ok && account != nil
}

func (h *OpenAIGatewayHandler) restorePublicVideoAccountBinding(ctx context.Context, binding *service.PublicVideoContentBinding) bool {
	if h == nil || h.gatewayService == nil || binding == nil {
		return false
	}
	groupID := binding.GroupID
	var err error
	switch binding.Provider {
	case service.PublicVideoProviderGrok:
		err = h.gatewayService.BindGrokMediaVideoRequestAccount(
			ctx, &groupID, binding.RequestID, binding.UserID, binding.APIKeyID, binding.AccountID,
		)
	case service.PublicVideoProviderOpenAI:
		err = h.gatewayService.BindStickySession(
			ctx, &groupID, service.OpenAIMediaResourceSessionHash(binding.APIKeyID, binding.RequestID), binding.AccountID,
		)
	case service.PublicVideoProviderSeedance:
		return true
	default:
		return false
	}
	return err == nil
}

func classifyPublicVideoContentRoute(path string) publicVideoContentRoute {
	switch {
	case strings.Contains(path, "/videos/jobs/:job_id/content"):
		return publicVideoContentRouteSeedanceJob
	case strings.Contains(path, "/contents/generations/tasks/:task_id/content"):
		return publicVideoContentRouteSeedanceTask
	case strings.Contains(path, "/videos/") && strings.HasSuffix(path, "/content"):
		return publicVideoContentRouteGeneric
	default:
		return publicVideoContentRouteUnknown
	}
}

func publicVideoContentRequestID(c *gin.Context, route publicVideoContentRoute) string {
	if c == nil {
		return ""
	}
	switch route {
	case publicVideoContentRouteSeedanceJob:
		return strings.TrimSpace(c.Param("job_id"))
	case publicVideoContentRouteSeedanceTask:
		return strings.TrimSpace(c.Param("task_id"))
	case publicVideoContentRouteGeneric:
		return strings.TrimSpace(c.Param("request_id"))
	default:
		return ""
	}
}

func publicVideoBindingMatchesAPIKey(binding *service.PublicVideoContentBinding, apiKey *service.APIKey) bool {
	if binding == nil || apiKey == nil || apiKey.User == nil || apiKey.Group == nil || apiKey.GroupID == nil {
		return false
	}
	return apiKey.ID == binding.APIKeyID && apiKey.User.ID == binding.UserID && apiKey.UserID == binding.UserID &&
		*apiKey.GroupID == binding.GroupID && apiKey.Group.ID == binding.GroupID
}

func restorePublicVideoAuthContext(c *gin.Context, apiKey *service.APIKey, subscription *service.UserSubscription, provider string) {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.User == nil {
		return
	}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:      apiKey.User.ID,
		Concurrency: apiKey.User.Concurrency,
	})
	c.Set(string(middleware2.ContextKeyUserRole), apiKey.User.Role)
	if subscription != nil {
		c.Set(string(middleware2.ContextKeySubscription), subscription)
	}
	ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, apiKey.User.ID)
	ctx = context.WithValue(ctx, ctxkey.Group, apiKey.Group)
	if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformComposite {
		ctx = service.WithResolvedTargetPlatform(ctx, provider)
	}
	c.Request = c.Request.WithContext(ctx)
}

func publicVideoContentNotFound(c *gin.Context) {
	if c == nil {
		return
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": gin.H{
			"type":    "not_found_error",
			"message": "Video content not found",
		},
	})
}
