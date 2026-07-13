package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

func RegisterAgentWorkerCallbackRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	runs := v1.Group("/agent-runs")
	{
		runs.POST("/:id/callback", h.AgentRun.Callback)
		runs.POST("/:id/model-proxy", h.AgentRun.ModelProxy)
		runs.POST("/:id/artifacts", h.AgentRun.RegisterArtifact)
		runs.POST("/:id/artifacts/upload", h.AgentRun.UploadArtifact)
	}
}
