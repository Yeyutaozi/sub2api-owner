package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteAgentModelProxyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	streamBody := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"

	err := writeAgentModelProxyStream(ctx, &service.ModelProxyStreamResponse{
		Status:      http.StatusOK,
		ContentType: "text/event-stream; charset=utf-8",
		Headers: map[string]string{
			"Cache-Control":  "private",
			"Content-Length": "999",
			"X-Request-Id":   "req-stream-1",
		},
		Body: io.NopCloser(strings.NewReader(streamBody)),
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/event-stream; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
	require.Equal(t, "req-stream-1", recorder.Header().Get("X-Request-Id"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Equal(t, streamBody, recorder.Body.String())
}
