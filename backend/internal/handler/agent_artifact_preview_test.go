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

func TestWriteArtifactPreviewContentStreamsInlineRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	content := &service.ArtifactPreviewContent{
		StatusCode:    http.StatusPartialContent,
		Body:          io.NopCloser(strings.NewReader("test")),
		ContentType:   "video/mp4",
		ContentLength: 4,
		ContentRange:  "bytes 0-3/10",
		AcceptRanges:  "bytes",
		ETag:          `"preview-etag"`,
		LastModified:  "Thu, 23 Jul 2026 08:00:00 GMT",
	}

	writeArtifactPreviewContent(ctx, content)

	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "inline", recorder.Header().Get("Content-Disposition"))
	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	require.Equal(t, "4", recorder.Header().Get("Content-Length"))
	require.Equal(t, "bytes 0-3/10", recorder.Header().Get("Content-Range"))
	require.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	require.Equal(t, `"preview-etag"`, recorder.Header().Get("ETag"))
	require.Equal(t, "Thu, 23 Jul 2026 08:00:00 GMT", recorder.Header().Get("Last-Modified"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "test", recorder.Body.String())
}

func TestWriteArtifactPreviewContentReturnsEmptyUnsatisfiedRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	content := &service.ArtifactPreviewContent{
		StatusCode:    http.StatusRequestedRangeNotSatisfiable,
		Body:          io.NopCloser(strings.NewReader("upstream error body")),
		ContentType:   "video/mp4",
		ContentLength: 19,
		ContentRange:  "bytes */10",
		AcceptRanges:  "bytes",
	}

	writeArtifactPreviewContent(ctx, content)

	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, recorder.Code)
	require.Equal(t, "bytes */10", recorder.Header().Get("Content-Range"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Body.String())
}
