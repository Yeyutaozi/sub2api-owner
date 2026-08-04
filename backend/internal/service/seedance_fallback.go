package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type SeedanceFallbackLeaseRenewFunc func(context.Context) (bool, error)

// MaintainSeedanceFallbackLease cancels the returned context if ownership of
// a fallback-creation claim cannot be renewed. Media preparation and create
// forwarding should use that context so an expired creator cannot race a newer
// worker after a long multi-file download.
func MaintainSeedanceFallbackLease(parent context.Context, renew SeedanceFallbackLeaseRenewFunc) (context.Context, func() error) {
	ctx, cancel := context.WithCancel(parent)
	if renew == nil {
		return ctx, func() error { cancel(); return nil }
	}
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(SeedanceFallbackLeaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				renewed, err := renew(renewCtx)
				renewCancel()
				if err != nil || !renewed {
					if err == nil {
						err = errors.New("seedance fallback claim is no longer owned")
					}
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return ctx, func() error {
		cancel()
		return <-done
	}
}

const (
	SeedanceFallbackStatusReady      = "ready"
	SeedanceFallbackStatusStarting   = "starting"
	SeedanceFallbackStatusActive     = "active"
	SeedanceFallbackStatusFailed     = "failed"
	SeedanceFallbackStatusCancelling = "cancelling"
	SeedanceFallbackStatusCancelled  = "cancelled"
)

type seedanceFallbackSnapshot struct {
	Prompt          string                         `json:"prompt"`
	Resolution      string                         `json:"resolution"`
	DurationSeconds int                            `json:"duration_seconds"`
	AspectRatio     string                         `json:"aspect_ratio"`
	GenerateAudio   bool                           `json:"generate_audio"`
	PromptEnhance   any                            `json:"prompt_enhance,omitempty"`
	StartFrameURL   string                         `json:"start_frame_url,omitempty"`
	EndFrameURL     string                         `json:"end_frame_url,omitempty"`
	References      []SeedanceReferenceImage       `json:"image_references,omitempty"`
	VideoReferences []SeedanceReferenceVideo       `json:"video_references,omitempty"`
	AudioReferences []SeedanceReferenceAudio       `json:"audio_references,omitempty"`
	StoredMedia     []SeedanceStoredMediaReference `json:"stored_media,omitempty"`
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
		StoredMedia:     append([]SeedanceStoredMediaReference(nil), info.StoredMedia...),
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
		StoredMedia:     append([]SeedanceStoredMediaReference(nil), stored.StoredMedia...),
	}
	if err := validateFFLinkVideoRequestInfo(info); err != nil {
		return nil, err
	}
	return info, nil
}
