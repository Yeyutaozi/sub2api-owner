package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type LocalMediaHandler struct{ store service.AgentArtifactStore }

func NewLocalMediaHandler(store service.AgentArtifactStore) *LocalMediaHandler {
	return &LocalMediaHandler{store: store}
}

func (h *LocalMediaHandler) Serve(c *gin.Context) {
	reader, ok := h.store.(service.AgentArtifactSignedReader)
	if !ok {
		response.NotFound(c, "Local media storage is not enabled")
		return
	}
	expires, err := strconv.ParseInt(strings.TrimSpace(c.Query("expires")), 10, 64)
	if err != nil {
		response.Unauthorized(c, "Invalid or expired media URL")
		return
	}
	result, err := reader.OpenSignedObject(c.Request.Context(), c.Query("key"), expires, c.Query("signature"), c.GetHeader("Range"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer result.Body.Close()
	for key, values := range result.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Status(result.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = io.Copy(c.Writer, result.Body)
	}
}
