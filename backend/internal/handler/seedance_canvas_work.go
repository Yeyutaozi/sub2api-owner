package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) syncAcceptedSeedanceVideoWork(
	c *gin.Context,
	apiKey *service.APIKey,
	userID int64,
	requestInfo *service.SeedanceRequestInfo,
	gatewayRemoteID string,
) (*service.CreazyCanvasWork, error) {
	if h == nil || h.creazyCanvasService == nil || c == nil || apiKey == nil {
		return nil, nil
	}
	ctx, cancel := creazyCanvasWorkWriteContext(c.Request.Context())
	defer cancel()
	return h.creazyCanvasService.SyncAcceptedVideoWork(ctx, service.SyncAcceptedCreazyCanvasVideoInput{
		UserID:           userID,
		APIKey:           apiKey,
		AssociatedWorkID: creazyCanvasCorrelationWorkID(c.GetHeader(creazyCanvasWorkIDHeader)),
		PublicModel:      seedanceCanvasPublicModel(requestInfo),
		Prompt:           seedanceCanvasPrompt(requestInfo),
		ParamsJSON:       seedanceAcceptedVideoWorkParams(requestInfo),
		GatewayRemoteID:  strings.TrimSpace(gatewayRemoteID),
	})
}

func seedanceCanvasPublicModel(info *service.SeedanceRequestInfo) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.Model)
}

func seedanceCanvasPrompt(info *service.SeedanceRequestInfo) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.Prompt)
}

func seedanceAcceptedVideoWorkParams(info *service.SeedanceRequestInfo) map[string]any {
	params := map[string]any{}
	if info == nil {
		return params
	}
	params["resolution"] = strings.TrimSpace(info.Resolution)
	params["duration"] = info.DurationSeconds
	params["aspect_ratio"] = strings.TrimSpace(info.AspectRatio)
	params["generate_audio"] = info.GenerateAudio || len(info.AudioReferences) > 0
	if info.PromptEnhance != nil {
		params["prompt_enhance"] = info.PromptEnhance
	}
	if value := reusableSeedanceCanvasMediaURL(info.StartFrameURL); value != "" {
		params["start_frame"] = value
	}
	if value := reusableSeedanceCanvasMediaURL(info.EndFrameURL); value != "" {
		params["end_frame"] = value
	}
	params["image_reference_count"] = len(info.References)
	params["video_reference_count"] = len(info.VideoReferences)
	params["audio_reference_count"] = len(info.AudioReferences)
	if values := seedanceCanvasImageReferenceURLs(info.References); len(values) > 0 {
		params["ref_images"] = values
	}
	if values := seedanceCanvasVideoReferenceURLs(info.VideoReferences); len(values) > 0 {
		params["ref_videos"] = values
	}
	if values := seedanceCanvasAudioReferenceURLs(info.AudioReferences); len(values) > 0 {
		params["ref_audios"] = values
	}
	return params
}

func reusableSeedanceCanvasMediaURL(raw string) string {
	value := strings.TrimSpace(raw)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(value, "/") {
		return value
	}
	return ""
}

func seedanceCanvasImageReferenceURLs(refs []service.SeedanceReferenceImage) []string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		if value := reusableSeedanceCanvasMediaURL(ref.URL); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func seedanceCanvasVideoReferenceURLs(refs []service.SeedanceReferenceVideo) []string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		if value := reusableSeedanceCanvasMediaURL(ref.URL); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func seedanceCanvasAudioReferenceURLs(refs []service.SeedanceReferenceAudio) []string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		if value := reusableSeedanceCanvasMediaURL(ref.URL); value != "" {
			values = append(values, value)
		}
	}
	return values
}
