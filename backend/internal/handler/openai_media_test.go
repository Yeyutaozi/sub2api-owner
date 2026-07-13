package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTrustedOpenAIMediaModelHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video_1", nil)
	c.Request.RemoteAddr = "127.0.0.1:54321"
	c.Request.Header.Set("User-Agent", "Sub2API-Agent-ModelProxy/1.0")
	c.Request.Header.Set("X-Sub2API-Model", " sora-2 ")
	require.Equal(t, "sora-2", trustedOpenAIMediaModelHeader(c))

	c.Request.RemoteAddr = "203.0.113.9:54321"
	require.Empty(t, trustedOpenAIMediaModelHeader(c))
}
