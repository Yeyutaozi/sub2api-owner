package service

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ffLinkVideoModelProfile struct {
	Platform            string
	DefaultResolution   string
	DefaultDuration     int
	AllowedResolutions  map[string]struct{}
	AllowedAspectRatios map[string]struct{}
	PromptLimit         int
	MaxImageReferences  int
	MaxVideoReferences  int
	MaxAudioReferences  int
	AllowFrames         bool
	AllowGeneratedAudio bool
	PromptEnhanceMode   string
	ValidateDuration    func(int, string) bool
}

var ffLinkVideoModelProfiles = map[string]ffLinkVideoModelProfile{
	"seedance-2.0": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 8,
		AllowedResolutions: resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21"),
		PromptLimit: 5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1,
		AllowFrames: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, resolution string) bool {
			return duration >= 4 && duration <= 15 && !(resolution == VideoBillingResolution1080P && duration > 12)
		},
	},
	"seedance-2.0-fast": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 8,
		AllowedResolutions: resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21"),
		PromptLimit: 5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1,
		AllowFrames: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 4 && duration <= 15 },
	},
	"seedance-2.0-mini": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 8,
		AllowedResolutions: resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "1:1", "9:16"),
		PromptLimit: 5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1,
		AllowFrames: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 4 && duration <= 15 },
	},
	"ltx-2.3-pro": {
		Platform: PlatformLTX, DefaultResolution: VideoBillingResolution1080P, DefaultDuration: 6,
		AllowedResolutions: resolutionSet(VideoBillingResolution1080P, VideoBillingResolution1440P, VideoBillingResolution2160P),
		AllowedAspectRatios: ratioSet("16:9"), PromptLimit: 5000,
		AllowFrames: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration == 6 || duration == 8 || duration == 10 },
	},
	"ltx-2.3-fast": {
		Platform: PlatformLTX, DefaultResolution: VideoBillingResolution1080P, DefaultDuration: 6,
		AllowedResolutions: resolutionSet(VideoBillingResolution1080P, VideoBillingResolution1440P, VideoBillingResolution2160P),
		AllowedAspectRatios: ratioSet("16:9"), PromptLimit: 5000,
		AllowFrames: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 6 && duration <= 20 && duration%2 == 0 },
	},
}

func resolutionSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func ratioSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func ffLinkVideoModelProfileFor(model string) (ffLinkVideoModelProfile, bool) {
	profile, ok := ffLinkVideoModelProfiles[strings.ToLower(strings.TrimSpace(model))]
	return profile, ok
}

func FFLinkVideoModelIDsForPlatform(platform string) []string {
	switch platform {
	case PlatformSeedance:
		return []string{"seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini"}
	case PlatformLTX:
		return []string{"ltx-2.3-pro", "ltx-2.3-fast"}
	default:
		return nil
	}
}

func ValidateFFLinkVideoModelPlatform(platform, model string) error {
	profile, ok := ffLinkVideoModelProfileFor(model)
	if !ok || profile.Platform != platform {
		return fmt.Errorf("model %s is not supported by the %s platform", strings.TrimSpace(model), platform)
	}
	return nil
}

func validateFFLinkVideoRequestInfo(info *SeedanceRequestInfo) error {
	if info == nil {
		return fmt.Errorf("video request is required")
	}
	profile, ok := ffLinkVideoModelProfileFor(info.Model)
	if !ok {
		return fmt.Errorf("unsupported video model: %s", strings.TrimSpace(info.Model))
	}
	info.Model = strings.ToLower(strings.TrimSpace(info.Model))
	if info.Resolution == "" {
		info.Resolution = profile.DefaultResolution
	}
	if _, ok := profile.AllowedResolutions[info.Resolution]; !ok {
		return fmt.Errorf("resolution %s is not supported by model %s", info.Resolution, info.Model)
	}
	if info.DurationSeconds == 0 {
		info.DurationSeconds = profile.DefaultDuration
	}
	if profile.ValidateDuration == nil || !profile.ValidateDuration(info.DurationSeconds, info.Resolution) {
		return fmt.Errorf("duration %d is not supported by model %s at %s", info.DurationSeconds, info.Model, info.Resolution)
	}
	ratio := strings.ToLower(strings.TrimSpace(info.AspectRatio))
	if ratio == "" || ratio == "adaptive" {
		ratio = "16:9"
	}
	if _, ok := profile.AllowedAspectRatios[ratio]; !ok {
		return fmt.Errorf("aspect_ratio %s is not supported by model %s", info.AspectRatio, info.Model)
	}
	if profile.Platform == PlatformSeedance && info.Resolution == VideoBillingResolution720P && ratio == "9:21" {
		return fmt.Errorf("aspect_ratio 9:21 is not supported by model %s at 720p", info.Model)
	}
	info.AspectRatio = ratio
	if profile.PromptLimit > 0 && utf8.RuneCountInString(info.Prompt) > profile.PromptLimit {
		return fmt.Errorf("prompt exceeds the %d character limit for model %s", profile.PromptLimit, info.Model)
	}
	if len(info.References) > profile.MaxImageReferences {
		return fmt.Errorf("model %s supports at most %d reference images", info.Model, profile.MaxImageReferences)
	}
	if len(info.VideoReferences) > profile.MaxVideoReferences {
		return fmt.Errorf("model %s supports at most %d reference videos", info.Model, profile.MaxVideoReferences)
	}
	if len(info.AudioReferences) > profile.MaxAudioReferences {
		return fmt.Errorf("model %s supports at most %d reference audio files", info.Model, profile.MaxAudioReferences)
	}
	if !profile.AllowFrames && (info.StartFrameURL != "" || info.EndFrameURL != "") {
		return fmt.Errorf("model %s does not support first or last frames", info.Model)
	}
	if !profile.AllowGeneratedAudio && info.GenerateAudio {
		return fmt.Errorf("model %s does not support generated audio", info.Model)
	}
	if len(info.AudioReferences) > 0 && len(info.References) == 0 && len(info.VideoReferences) == 0 {
		return fmt.Errorf("reference audio requires at least one reference image or reference video")
	}
	return nil
}

func normalizeFFLinkPromptEnhance(value any, model string) (any, error) {
	if value == nil {
		return nil, nil
	}
	profile, ok := ffLinkVideoModelProfileFor(model)
	if !ok {
		return nil, fmt.Errorf("unsupported video model: %s", strings.TrimSpace(model))
	}
	if profile.PromptEnhanceMode == "legacy" {
		switch typed := value.(type) {
		case bool:
			return typed, nil
		case string:
			mode := strings.ToUpper(strings.TrimSpace(typed))
			if mode == "AUTO" || mode == "ON" || mode == "OFF" {
				return mode, nil
			}
		}
		return nil, fmt.Errorf("prompt_enhance must be a boolean or one of AUTO, ON, OFF")
	}
	var mode string
	switch typed := value.(type) {
	case bool:
		if typed {
			mode = "ON"
		} else {
			mode = "OFF"
		}
	case string:
		mode = strings.ToUpper(strings.TrimSpace(typed))
	default:
		return nil, fmt.Errorf("prompt_enhance must be one of AUTO, ON, OFF")
	}
	if mode != "AUTO" && mode != "ON" && mode != "OFF" {
		return nil, fmt.Errorf("prompt_enhance must be one of AUTO, ON, OFF")
	}
	return mode, nil
}
