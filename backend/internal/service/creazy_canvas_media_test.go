//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreazyCanvasPublicMediaURL_GatewayVideoIgnoresPoster(t *testing.T) {
	work := &CreazyCanvasWork{
		Kind:            CreazyCanvasWorkKindVideo,
		GatewayType:     CreazyCanvasGatewayVideoJob,
		GatewayRemoteID: "task-1",
		ObjectURL:       "/v1/videos/jobs/task-1/content",
		PreviewURL:      "https://cdn.example.com/start-frame.png",
		ParamsJSON: map[string]any{
			"poster_url": "https://cdn.example.com/start-frame.png",
			"result_urls": []any{
				"/v1/videos/jobs/task-1/content",
			},
		},
	}

	require.Empty(t, creazyCanvasPublicMediaURL(work))
	require.Equal(t, "/v1/videos/jobs/task-1/content", creazyCanvasGatewayContentPath(work))
}

func TestCreazyCanvasPublicMediaURL_PrefersActualOutput(t *testing.T) {
	t.Run("gateway video absolute object", func(t *testing.T) {
		work := &CreazyCanvasWork{
			Kind:            CreazyCanvasWorkKindVideo,
			GatewayType:     CreazyCanvasGatewayVideoJob,
			GatewayRemoteID: "task-2",
			ObjectURL:       "https://cdn.example.com/result.mp4",
			PreviewURL:      "https://cdn.example.com/poster.png",
		}
		require.Equal(t, "https://cdn.example.com/result.mp4", creazyCanvasPublicMediaURL(work))
	})

	t.Run("image preview", func(t *testing.T) {
		work := &CreazyCanvasWork{
			Kind:       CreazyCanvasWorkKindImage,
			PreviewURL: "https://cdn.example.com/result.png",
		}
		require.Equal(t, "https://cdn.example.com/result.png", creazyCanvasPublicMediaURL(work))
	})

	t.Run("legacy video without gateway", func(t *testing.T) {
		work := &CreazyCanvasWork{
			Kind:       CreazyCanvasWorkKindVideo,
			PreviewURL: "https://cdn.example.com/legacy-result.mp4",
		}
		require.Equal(t, "https://cdn.example.com/legacy-result.mp4", creazyCanvasPublicMediaURL(work))
	})
}
