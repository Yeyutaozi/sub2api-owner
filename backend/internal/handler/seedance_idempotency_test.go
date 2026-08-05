package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEnsureSeedanceCreateIdempotencyKeyGeneratesAndEchoesStableKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/videos/generations", strings.NewReader(`{"model":"sd-2.0-mx933"}`))

	first := ensureSeedanceCreateIdempotencyKey(c)
	second := ensureSeedanceCreateIdempotencyKey(c)

	require.True(t, strings.HasPrefix(first, "seedance-"))
	require.Equal(t, first, second)
	require.Equal(t, first, c.Request.Header.Get("Idempotency-Key"))
	require.Equal(t, first, recorder.Header().Get("Idempotency-Key"))
}

func TestEnsureSeedanceCreateIdempotencyKeyPreservesClientKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/videos/generations", nil)
	c.Request.Header.Set("Idempotency-Key", "client-operation-123")

	require.Equal(t, "client-operation-123", ensureSeedanceCreateIdempotencyKey(c))
	require.Equal(t, "client-operation-123", recorder.Header().Get("Idempotency-Key"))
}

func TestSeedanceCreateIdempotencyScopeIsStableAndTenantScoped(t *testing.T) {
	first := seedanceCreateIdempotencyScope(10, 20, "client-operation-123")

	require.Equal(t, first, seedanceCreateIdempotencyScope(10, 20, " client-operation-123 "))
	require.NotEqual(t, first, seedanceCreateIdempotencyScope(11, 20, "client-operation-123"))
	require.NotEqual(t, first, seedanceCreateIdempotencyScope(10, 21, "client-operation-123"))
	require.NotEqual(t, first, seedanceCreateIdempotencyScope(10, 20, "client-operation-456"))
}
