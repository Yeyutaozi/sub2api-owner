package service

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	SeedanceFallbackStatusReady      = "ready"
	SeedanceFallbackStatusStarting   = "starting"
	SeedanceFallbackStatusActive     = "active"
	SeedanceFallbackStatusFailed     = "failed"
	SeedanceFallbackStatusCancelling = "cancelling"
	SeedanceFallbackStatusCancelled  = "cancelled"
)

type seedanceFallbackSnapshot struct {
	Prompt          string                   `json:"prompt"`
	Resolution      string                   `json:"resolution"`
	DurationSeconds int                      `json:"duration_seconds"`
	AspectRatio     string                   `json:"aspect_ratio"`
	GenerateAudio   bool                     `json:"generate_audio"`
	PromptEnhance   any                      `json:"prompt_enhance,omitempty"`
	StartFrameURL   string                   `json:"start_frame_url,omitempty"`
	EndFrameURL     string                   `json:"end_frame_url,omitempty"`
	References      []SeedanceReferenceImage `json:"image_references,omitempty"`
	VideoReferences []SeedanceReferenceVideo `json:"video_references,omitempty"`
	AudioReferences []SeedanceReferenceAudio `json:"audio_references,omitempty"`
}

// SeedanceFallbackModelFor maps only the 720p FFLink Seedance 431 family to
// its explicitly configured Huiqu MX933 counterpart. Other resolutions and
// models remain on their original provider.
func SeedanceFallbackModelFor(model, resolution string) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(resolution), VideoBillingResolution720P) {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "seedance-2.0":
		return "sd2-mx933-720-1s", true
	case "seedance-2.0-fast":
		return "sd2-mx933-720-fast-1s", true
	default:
		return "", false
	}
}

func SnapshotSeedanceFallbackRequest(info *SeedanceRequestInfo) ([]byte, error) {
	if info == nil {
		return nil, errors.New("seedance request info is required")
	}
	if _, ok := SeedanceFallbackModelFor(info.Model, info.Resolution); !ok {
		return nil, nil
	}
	snapshot := seedanceFallbackSnapshot{
		Prompt:          info.Prompt,
		Resolution:      info.Resolution,
		DurationSeconds: info.DurationSeconds,
		AspectRatio:     info.AspectRatio,
		GenerateAudio:   info.GenerateAudio,
		PromptEnhance:   info.PromptEnhance,
		StartFrameURL:   info.StartFrameURL,
		EndFrameURL:     info.EndFrameURL,
		References:      append([]SeedanceReferenceImage(nil), info.References...),
		VideoReferences: append([]SeedanceReferenceVideo(nil), info.VideoReferences...),
		AudioReferences: append([]SeedanceReferenceAudio(nil), info.AudioReferences...),
	}
	return json.Marshal(snapshot)
}

func RestoreSeedanceFallbackRequest(snapshot []byte, fallbackModel string) (*SeedanceRequestInfo, error) {
	if len(snapshot) == 0 {
		return nil, errors.New("seedance fallback request snapshot is empty")
	}
	if !IsHuiquVideoModel(fallbackModel) {
		return nil, errors.New("seedance fallback model is invalid")
	}
	var stored seedanceFallbackSnapshot
	if err := json.Unmarshal(snapshot, &stored); err != nil {
		return nil, errors.New("seedance fallback request snapshot is invalid")
	}
	info := &SeedanceRequestInfo{
		Model:           strings.ToLower(strings.TrimSpace(fallbackModel)),
		Prompt:          stored.Prompt,
		Resolution:      stored.Resolution,
		DurationSeconds: stored.DurationSeconds,
		AspectRatio:     stored.AspectRatio,
		GenerateAudio:   stored.GenerateAudio,
		PromptEnhance:   stored.PromptEnhance,
		StartFrameURL:   stored.StartFrameURL,
		EndFrameURL:     stored.EndFrameURL,
		References:      append([]SeedanceReferenceImage(nil), stored.References...),
		VideoReferences: append([]SeedanceReferenceVideo(nil), stored.VideoReferences...),
		AudioReferences: append([]SeedanceReferenceAudio(nil), stored.AudioReferences...),
	}
	if err := validateFFLinkVideoRequestInfo(info); err != nil {
		return nil, err
	}
	return info, nil
}
