package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRestorePublicVideoAuthContextPreservesRangeAndRestoresOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video_opaque/content", nil)
	c.Request.Header.Set("Range", "bytes=100-199")
	groupID := int64(13)
	apiKey := &service.APIKey{
		ID:      11,
		UserID:  7,
		GroupID: &groupID,
		User: &service.User{
			ID:          7,
			Role:        service.RoleUser,
			Concurrency: 3,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
	}
	subscription := &service.UserSubscription{ID: 17}

	restorePublicVideoAuthContext(c, apiKey, subscription, service.PublicVideoProviderGrok)

	gotKey, ok := middleware2.GetAPIKeyFromContext(c)
	require.True(t, ok)
	require.Same(t, apiKey, gotKey)
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	require.True(t, ok)
	require.Equal(t, int64(7), subject.UserID)
	require.Equal(t, 3, subject.Concurrency)
	gotSubscription, ok := middleware2.GetSubscriptionFromContext(c)
	require.True(t, ok)
	require.Same(t, subscription, gotSubscription)
	require.Equal(t, int64(7), c.Request.Context().Value(ctxkey.UserID))
	require.Same(t, apiKey.Group, c.Request.Context().Value(ctxkey.Group))
	provider, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, service.PublicVideoProviderGrok, provider)
	require.Equal(t, "bytes=100-199", c.GetHeader("Range"))
}

func TestPublicVideoBindingMustMatchLoadedAPIKeyOwner(t *testing.T) {
	groupID := int64(13)
	apiKey := &service.APIKey{
		ID:      11,
		UserID:  7,
		GroupID: &groupID,
		User:    &service.User{ID: 7},
		Group:   &service.Group{ID: groupID},
	}
	binding := &service.PublicVideoContentBinding{APIKeyID: 11, UserID: 7, GroupID: 13}
	require.True(t, publicVideoBindingMatchesAPIKey(binding, apiKey))

	binding.UserID = 8
	require.False(t, publicVideoBindingMatchesAPIKey(binding, apiKey))
	binding.UserID = 7
	binding.GroupID = 14
	require.False(t, publicVideoBindingMatchesAPIKey(binding, apiKey))
}

func TestPublicVideoContentAccountContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := &service.Account{ID: 29, Platform: service.PlatformGrok}
	c.Set(publicVideoContentAccountContextKey, account)

	got, ok := publicVideoContentAccount(c)
	require.True(t, ok)
	require.Same(t, account, got)
}

func TestClassifyPublicVideoContentRoute(t *testing.T) {
	require.Equal(t, publicVideoContentRouteSeedanceJob, classifyPublicVideoContentRoute("/v1/videos/jobs/:job_id/content"))
	require.Equal(t, publicVideoContentRouteSeedanceTask, classifyPublicVideoContentRoute("/api/v3/contents/generations/tasks/:task_id/content"))
	require.Equal(t, publicVideoContentRouteGeneric, classifyPublicVideoContentRoute("/videos/extensions/:request_id/content"))
	require.Equal(t, publicVideoContentRouteUnknown, classifyPublicVideoContentRoute("/v1/videos/:request_id"))
}
