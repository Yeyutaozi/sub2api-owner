package handler

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	creazyCanvasWorkIDHeader          = "X-Creazy-Canvas-Work-ID"
	creazyCanvasGatewayWorkContextKey = "creazy_canvas_gateway_work"
)

type creazyCanvasGatewayWorkService interface {
	CreateWork(context.Context, service.CreateCreazyCanvasWorkInput) (*service.CreazyCanvasWork, error)
	GetWork(context.Context, int64, int64) (*service.CreazyCanvasWork, error)
	SyncAcceptedVideoWork(context.Context, service.SyncAcceptedCreazyCanvasVideoInput) (*service.CreazyCanvasWork, error)
	UpdateWork(context.Context, service.UpdateCreazyCanvasWorkInput) (*service.CreazyCanvasWork, error)
}

type creazyCanvasGatewayWorkInput struct {
	UserID          int64
	APIKeyID        int64
	Kind            string
	PublicModel     string
	Prompt          string
	ParamsJSON      map[string]any
	GatewayType     string
	GatewayRemoteID string
	Status          string
}

type creazyCanvasGatewayWorkTracker struct {
	mu       sync.Mutex
	workID   int64
	userID   int64
	terminal bool
}

func (h *OpenAIGatewayHandler) setCreazyCanvasService(canvas *service.CreazyCanvasService) {
	if h != nil {
		h.creazyCanvasService = canvas
	}
}

func (h *OpenAIGatewayHandler) beginCreazyCanvasGatewayWork(c *gin.Context, input creazyCanvasGatewayWorkInput) *creazyCanvasGatewayWorkTracker {
	if h == nil || h.creazyCanvasService == nil || c == nil || input.UserID <= 0 || input.APIKeyID <= 0 {
		return nil
	}
	if existing, ok := c.Get(creazyCanvasGatewayWorkContextKey); ok {
		if tracker, valid := existing.(*creazyCanvasGatewayWorkTracker); valid && tracker != nil {
			return tracker
		}
	}

	ctx, cancel := creazyCanvasWorkWriteContext(c.Request.Context())
	defer cancel()

	if workID := creazyCanvasCorrelationWorkID(c.GetHeader(creazyCanvasWorkIDHeader)); workID > 0 {
		work, err := h.creazyCanvasService.GetWork(ctx, input.UserID, workID)
		if err == nil && creazyCanvasCorrelationMatches(work, input) {
			tracker := &creazyCanvasGatewayWorkTracker{workID: work.ID, userID: input.UserID}
			c.Set(creazyCanvasGatewayWorkContextKey, tracker)
			h.updateCreazyCanvasGatewayWorkRuntime(ctx, tracker, work, input)
			return tracker
		}
		if err != nil {
			logger.L().Warn("creazy_canvas.gateway_correlation_ignored", zap.Int64("user_id", input.UserID), zap.Int64("api_key_id", input.APIKeyID), zap.Int64("work_id", workID), zap.Error(err))
		}
	}

	params := cloneCreazyCanvasGatewayParams(input.ParamsJSON)
	params["source"] = "api"
	work, err := h.creazyCanvasService.CreateWork(ctx, service.CreateCreazyCanvasWorkInput{
		UserID:          input.UserID,
		APIKeyID:        input.APIKeyID,
		Kind:            input.Kind,
		PublicModel:     input.PublicModel,
		Prompt:          input.Prompt,
		ParamsJSON:      params,
		GatewayType:     input.GatewayType,
		GatewayRemoteID: input.GatewayRemoteID,
		Status:          input.Status,
	})
	if err != nil {
		logger.L().Warn("creazy_canvas.gateway_work_create_failed", zap.Int64("user_id", input.UserID), zap.Int64("api_key_id", input.APIKeyID), zap.String("kind", input.Kind), zap.Error(err))
		return nil
	}
	tracker := &creazyCanvasGatewayWorkTracker{workID: work.ID, userID: input.UserID}
	c.Set(creazyCanvasGatewayWorkContextKey, tracker)
	return tracker
}

func (h *OpenAIGatewayHandler) updateCreazyCanvasGatewayWorkRuntime(ctx context.Context, tracker *creazyCanvasGatewayWorkTracker, work *service.CreazyCanvasWork, input creazyCanvasGatewayWorkInput) {
	if h == nil || h.creazyCanvasService == nil || tracker == nil || work == nil {
		return
	}
	update := service.UpdateCreazyCanvasWorkInput{UserID: tracker.userID, WorkID: tracker.workID}
	if status := strings.TrimSpace(input.Status); status != "" {
		update.Status = &status
	}
	if gatewayType := strings.TrimSpace(input.GatewayType); gatewayType != "" {
		update.GatewayType = &gatewayType
	}
	if remoteID := strings.TrimSpace(input.GatewayRemoteID); remoteID != "" {
		update.GatewayRemoteID = &remoteID
	}
	if strings.TrimSpace(work.PublicModel) == "" && strings.TrimSpace(input.PublicModel) != "" {
		model := strings.TrimSpace(input.PublicModel)
		update.PublicModel = &model
	}
	if strings.TrimSpace(work.Prompt) == "" && strings.TrimSpace(input.Prompt) != "" {
		prompt := strings.TrimSpace(input.Prompt)
		update.Prompt = &prompt
	}
	if _, err := h.creazyCanvasService.UpdateWork(ctx, update); err != nil {
		logger.L().Warn("creazy_canvas.gateway_work_update_failed", zap.Int64("work_id", tracker.workID), zap.Error(err))
	}
}

func (h *OpenAIGatewayHandler) succeedCreazyCanvasGatewayWork(c *gin.Context, tracker *creazyCanvasGatewayWorkTracker, remoteID, mediaURL, mimeType string) {
	h.finishCreazyCanvasGatewayWork(c, tracker, service.CreazyCanvasWorkStatusSucceeded, "", remoteID, mediaURL, mimeType)
}

func (h *OpenAIGatewayHandler) failCreazyCanvasGatewayWork(c *gin.Context, tracker *creazyCanvasGatewayWorkTracker, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Generation failed"
	}
	h.finishCreazyCanvasGatewayWork(c, tracker, service.CreazyCanvasWorkStatusFailed, message, "", "", "")
}

func (h *OpenAIGatewayHandler) finishCreazyCanvasGatewayWork(c *gin.Context, tracker *creazyCanvasGatewayWorkTracker, status, errorMessage, remoteID, mediaURL, mimeType string) {
	if h == nil || h.creazyCanvasService == nil || tracker == nil {
		return
	}
	tracker.mu.Lock()
	if tracker.terminal {
		tracker.mu.Unlock()
		return
	}
	tracker.terminal = true
	tracker.mu.Unlock()

	base := context.Background()
	if c != nil && c.Request != nil {
		base = c.Request.Context()
	}
	ctx, cancel := creazyCanvasWorkWriteContext(base)
	defer cancel()
	status = strings.TrimSpace(status)
	update := service.UpdateCreazyCanvasWorkInput{
		UserID:       tracker.userID,
		WorkID:       tracker.workID,
		Status:       &status,
		ErrorMessage: &errorMessage,
	}
	if remoteID = strings.TrimSpace(remoteID); remoteID != "" {
		update.GatewayRemoteID = &remoteID
	}
	if mediaURL = strings.TrimSpace(mediaURL); mediaURL != "" {
		update.ObjectURL = &mediaURL
		update.PreviewURL = &mediaURL
	}
	if mimeType = strings.TrimSpace(mimeType); mimeType != "" {
		update.MimeType = &mimeType
	}
	if _, err := h.creazyCanvasService.UpdateWork(ctx, update); err != nil {
		logger.L().Warn("creazy_canvas.gateway_work_finish_failed", zap.Int64("work_id", tracker.workID), zap.String("status", status), zap.Error(err))
	}
}

func creazyCanvasWorkWriteContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
}

func creazyCanvasCorrelationWorkID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func creazyCanvasCorrelationMatches(work *service.CreazyCanvasWork, input creazyCanvasGatewayWorkInput) bool {
	if work == nil || work.UserID != input.UserID || work.APIKeyID != input.APIKeyID || !strings.EqualFold(strings.TrimSpace(work.Kind), strings.TrimSpace(input.Kind)) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(work.Status)) {
	case service.CreazyCanvasWorkStatusCreated, service.CreazyCanvasWorkStatusQueued, service.CreazyCanvasWorkStatusRunning:
		return true
	default:
		return false
	}
}

func cloneCreazyCanvasGatewayParams(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src)+1)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func creazyCanvasImageWorkParams(parsed *service.OpenAIImagesRequest) map[string]any {
	params := map[string]any{}
	if parsed == nil {
		return params
	}
	params["edit"] = parsed.IsEdits()
	params["stream"] = parsed.Stream
	if parsed.N > 0 {
		params["n"] = parsed.N
	}
	if value := strings.TrimSpace(parsed.Size); value != "" {
		params["size"] = value
	}
	if value := strings.TrimSpace(parsed.Quality); value != "" {
		params["quality"] = value
	}
	if value := strings.TrimSpace(parsed.OutputFormat); value != "" {
		params["output_format"] = value
	}
	if count := len(parsed.InputImageURLs) + len(parsed.Uploads); count > 0 {
		params["reference_count"] = count
	}
	return params
}

func creazyCanvasImageMimeType(parsed *service.OpenAIImagesRequest) string {
	if parsed == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(parsed.OutputFormat)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "png":
		return "image/png"
	default:
		return ""
	}
}

func firstCreazyCanvasMediaURL(urls []string) string {
	for _, mediaURL := range urls {
		if mediaURL = strings.TrimSpace(mediaURL); mediaURL != "" {
			return mediaURL
		}
	}
	return ""
}

func creazyCanvasImageURLFromResponse(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{
		"data.0.url",
		"data.0.image_url",
		"images.0.url",
		"images.0.image_url",
		"output.0.url",
		"output.0.image_url",
		"result.data.0.url",
		"result.images.0.url",
	} {
		mediaURL := strings.TrimSpace(gjson.GetBytes(body, path).String())
		lower := strings.ToLower(mediaURL)
		if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
			return mediaURL
		}
	}
	return ""
}

func creazyCanvasGatewayWorkFromContext(c *gin.Context) *creazyCanvasGatewayWorkTracker {
	if c == nil {
		return nil
	}
	value, ok := c.Get(creazyCanvasGatewayWorkContextKey)
	if !ok {
		return nil
	}
	tracker, _ := value.(*creazyCanvasGatewayWorkTracker)
	return tracker
}

func creazyCanvasGrokMediaParams(endpoint service.GrokMediaEndpoint, parsed service.GrokMediaRequestInfo) map[string]any {
	params := map[string]any{"endpoint": string(endpoint)}
	if parsed.N > 0 {
		params["n"] = parsed.N
	}
	if value := strings.TrimSpace(parsed.Size); value != "" {
		params["size"] = value
	}
	if value := strings.TrimSpace(parsed.Resolution); value != "" {
		params["resolution"] = value
	}
	if parsed.DurationSeconds > 0 {
		params["duration"] = parsed.DurationSeconds
	}
	if count := len(parsed.InputImageURLs) + len(parsed.Uploads); count > 0 {
		params["reference_count"] = count
	}
	return params
}
