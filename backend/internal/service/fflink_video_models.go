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
	MaxTotalImages      int
	MaxVideoReferences  int
	MaxAudioReferences  int
	MaxTotalMedia       int
	AllowStartFrame     bool
	RequireStartFrame   bool
	AllowEndFrame       bool
	AllowGeneratedAudio bool
	PromptEnhanceMode   string
	ValidateDuration    func(int, string) bool
}

var ffLinkVideoModelProfiles = map[string]ffLinkVideoModelProfile{
	"seedance-2.0": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21"),
		PromptLimit:         5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1, MaxTotalMedia: 8,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, resolution string) bool {
			return isSeedanceDurationSupported(duration) && !(resolution == VideoBillingResolution1080P && duration > 12)
		},
	},
	"seedance-2.0-fast": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21"),
		PromptLimit:         5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1, MaxTotalMedia: 8,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	"seedance-2.0-mini": {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "1:1", "9:16"),
		PromptLimit:         5000, MaxImageReferences: 4, MaxVideoReferences: 3, MaxAudioReferences: 1, MaxTotalMedia: 8,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "legacy",
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	SeedanceMX933Model: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3"),
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 12,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	SeedanceMX933FastModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3"),
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 12,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	// Legacy model IDs remain readable for existing account mappings and
	// already-created tasks. New requests must use the logical model IDs above;
	// RestoreSeedanceFallbackRequest may temporarily allow their old variable
	// duration snapshots so a deployment cannot strand an in-flight task.
	SeedanceMX933LegacyModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3"),
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 12,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	SeedanceMX933LegacyFastModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3"),
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 12,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	SeedanceMiniMaxH3Model: {
		Platform: PlatformMiniMax, DefaultResolution: VideoBillingResolution1440P, DefaultDuration: 8,
		AllowedResolutions:  resolutionSet(VideoBillingResolution1440P),
		AllowedAspectRatios: ratioSet("16:9", "9:16"),
		PromptLimit:         5000, MaxImageReferences: 5, MaxTotalImages: 5, MaxVideoReferences: 0, MaxAudioReferences: 3, MaxTotalMedia: 8,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isHuiquMiniMaxH3DurationSupported(duration) },
	},
	SeedanceXimeiSD20Model: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P, VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 15,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isSeedanceDurationSupported(duration) },
	},
	SeedanceXimeiSD25Model: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: seedanceXimeiSD25DefaultDurationSeconds,
		AllowedResolutions:  resolutionSet(VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1", "4:3", "3:4", "21:9"),
		PromptLimit:         30000, MaxImageReferences: 30, MaxTotalImages: 30, MaxVideoReferences: 10, MaxAudioReferences: 10, MaxTotalMedia: 50,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool {
			return isXimeiVideoDurationSupported(SeedanceXimeiSD25Model, duration)
		},
	},
	// Unofficial Ximei Seedance 2.5 channel (lajiao_pool). Media matrix same as official; ratios/prompt from upstream health.
	SeedanceXimeiSD25UnofficialModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: seedanceXimeiSD25DefaultDurationSeconds,
		AllowedResolutions:  resolutionSet(VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16", "1:1"),
		PromptLimit:         5000, MaxImageReferences: 30, MaxTotalImages: 30, MaxVideoReferences: 10, MaxAudioReferences: 10, MaxTotalMedia: 50,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool {
			return isXimeiVideoDurationSupported(SeedanceXimeiSD25UnofficialModel, duration)
		},
	},
	SeedanceWeijinFaceRef480pModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution480P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution480P),
		AllowedAspectRatios: ratioSet("21:9", "16:9", "4:3", "1:1", "3:4", "9:16"),
		// Public special-offer face models: allow full mixed load 9 images + 3 videos + 3 audios.
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 15,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isWeijinFaceReferenceDurationSupported(duration) },
	},
	SeedanceWeijinFaceRef720pModel: {
		Platform: PlatformSeedance, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("21:9", "16:9", "4:3", "1:1", "3:4", "9:16"),
		// Public special-offer face models: allow full mixed load 9 images + 3 videos + 3 audios.
		PromptLimit:         5000, MaxImageReferences: 9, MaxTotalImages: 9, MaxVideoReferences: 3, MaxAudioReferences: 3, MaxTotalMedia: 15,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true,
		ValidateDuration: func(duration int, _ string) bool { return isWeijinFaceReferenceDurationSupported(duration) },
	},
	"ltx-2.3-pro": {
		Platform: PlatformLTX, DefaultResolution: VideoBillingResolution1080P, DefaultDuration: 6,
		AllowedResolutions:  resolutionSet(VideoBillingResolution1080P, VideoBillingResolution1440P, VideoBillingResolution2160P),
		AllowedAspectRatios: ratioSet("16:9"), PromptLimit: 5000,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration == 6 || duration == 8 || duration == 10 },
	},
	"ltx-2.3-fast": {
		Platform: PlatformLTX, DefaultResolution: VideoBillingResolution1080P, DefaultDuration: 6,
		AllowedResolutions:  resolutionSet(VideoBillingResolution1080P, VideoBillingResolution1440P, VideoBillingResolution2160P),
		AllowedAspectRatios: ratioSet("16:9"), PromptLimit: 5000,
		AllowStartFrame: true, AllowEndFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 6 && duration <= 20 && duration%2 == 0 },
	},
	"happy-horse-1.1": {
		Platform: PlatformHappyHorse, DefaultResolution: VideoBillingResolution1080P, DefaultDuration: 5,
		AllowedResolutions:  resolutionSet(VideoBillingResolution720P, VideoBillingResolution1080P),
		AllowedAspectRatios: ratioSet("16:9", "4:3", "1:1", "3:4", "9:16"), PromptLimit: 2500,
		MaxImageReferences: 9, AllowStartFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 3 && duration <= 15 },
	},
	"grok-imagine-1.5": {
		Platform: PlatformGrokImagine, DefaultResolution: VideoBillingResolution720P, DefaultDuration: 6,
		AllowedResolutions:  resolutionSet(VideoBillingResolution720P),
		AllowedAspectRatios: ratioSet("16:9", "9:16"),
		PromptLimit:         2500, MaxImageReferences: 0, MaxVideoReferences: 0, MaxAudioReferences: 0,
		AllowStartFrame: true, RequireStartFrame: true, AllowGeneratedAudio: true, PromptEnhanceMode: "enum",
		ValidateDuration: func(duration int, _ string) bool { return duration >= 3 && duration <= 15 },
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
		return []string{
			"seedance-2.0",
			"seedance-2.0-fast",
			"seedance-2.0-mini",
			SeedanceMX933Model,
			SeedanceMX933FastModel,
			SeedanceXimeiSD20Model,
			SeedanceXimeiSD25Model,
			SeedanceXimeiSD25UnofficialModel,
			SeedanceWeijinFaceRef480pModel,
			SeedanceWeijinFaceRef720pModel,
		}
	case PlatformMiniMax:
		return []string{
			SeedanceMiniMaxH3Model,
		}
	case PlatformLTX:
		return []string{"ltx-2.3-pro", "ltx-2.3-fast"}
	case PlatformHappyHorse:
		return []string{"happy-horse-1.1"}
	case PlatformGrokImagine:
		return []string{"grok-imagine-1.5"}
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
	return validateFFLinkVideoRequestInfoWithLegacyDuration(info, false)
}

func validateFFLinkVideoRequestInfoWithLegacyDuration(info *SeedanceRequestInfo, allowLegacyVariableDuration bool) error {
	if info == nil {
		return fmt.Errorf("video request is required")
	}
	if isLegacyHuiquVariableDurationModel(info.Model) && !allowLegacyVariableDuration {
		return fmt.Errorf("unsupported video model: %s", info.Model)
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
	durationValid := profile.ValidateDuration != nil && profile.ValidateDuration(info.DurationSeconds, info.Resolution)
	if !durationValid && allowLegacyVariableDuration && isLegacyHuiquVariableDurationModel(info.Model) {
		durationValid = info.DurationSeconds >= 1 && info.DurationSeconds <= 15
	}
	if !durationValid {
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
	if profile.MaxTotalImages > 0 {
		totalImages := len(info.References)
		if strings.TrimSpace(info.StartFrameURL) != "" {
			totalImages++
		}
		if strings.TrimSpace(info.EndFrameURL) != "" {
			totalImages++
		}
		if totalImages > profile.MaxTotalImages {
			return fmt.Errorf("model %s supports at most %d total images including reference images and first/last frames", info.Model, profile.MaxTotalImages)
		}
	}
	if len(info.VideoReferences) > profile.MaxVideoReferences {
		return fmt.Errorf("model %s supports at most %d reference videos", info.Model, profile.MaxVideoReferences)
	}
	if len(info.AudioReferences) > profile.MaxAudioReferences {
		return fmt.Errorf("model %s supports at most %d reference audio files", info.Model, profile.MaxAudioReferences)
	}
	if profile.MaxTotalMedia > 0 {
		totalMedia := len(info.References) + len(info.VideoReferences) + len(info.AudioReferences)
		if strings.TrimSpace(info.StartFrameURL) != "" {
			totalMedia++
		}
		if strings.TrimSpace(info.EndFrameURL) != "" {
			totalMedia++
		}
		if totalMedia > profile.MaxTotalMedia {
			return fmt.Errorf("model %s supports at most %d total reference media files", info.Model, profile.MaxTotalMedia)
		}
	}
	if !profile.AllowStartFrame && info.StartFrameURL != "" {
		return fmt.Errorf("model %s does not support a first frame", info.Model)
	}
	if profile.RequireStartFrame && strings.TrimSpace(info.StartFrameURL) == "" {
		return fmt.Errorf("start frame is required by %s", info.Model)
	}
	if !profile.AllowEndFrame && info.EndFrameURL != "" {
		return fmt.Errorf("model %s does not support a last frame", info.Model)
	}
	// Gateway contract: end frame always depends on start frame (pair not required).
	if strings.TrimSpace(info.EndFrameURL) != "" && strings.TrimSpace(info.StartFrameURL) == "" {
		return fmt.Errorf("a first frame is required when a last frame is provided")
	}
	if !profile.AllowGeneratedAudio && info.GenerateAudio {
		return fmt.Errorf("model %s does not support generated audio", info.Model)
	}
	if len(info.AudioReferences) > 0 && !info.GenerateAudio {
		return fmt.Errorf("audio=true is required when guidances.audio_reference is provided")
	}
	if isHuiquMiniMaxH3Model(info.Model) {
		// Upstream hailuo-03/H3 always generates native audio and rejects audio=false.
		info.GenerateAudio = true
		hasStart := strings.TrimSpace(info.StartFrameURL) != ""
		hasEnd := strings.TrimSpace(info.EndFrameURL) != ""
		hasFrames := hasStart || hasEnd
		hasImageRefs := len(info.References) > 0
		hasAudioRefs := len(info.AudioReferences) > 0
		if len(info.VideoReferences) > 0 {
			return fmt.Errorf("model %s does not support reference videos", info.Model)
		}
		if hasEnd && !hasStart {
			return fmt.Errorf("model %s requires a first frame when a last frame is provided", info.Model)
		}
		if hasFrames && (hasImageRefs || hasAudioRefs) {
			return fmt.Errorf("model %s first/last frames cannot be combined with reference images or audio", info.Model)
		}
		if hasAudioRefs && !hasImageRefs {
			return fmt.Errorf("model %s requires reference images when audio_reference is provided", info.Model)
		}
	}
	// 参考音频可单独上传，不再强制搭配参考图/视频。
	if isXimeiVideoModel(info.Model) {
		product, err := ximeiVideoProductFor(info.Model, info.Resolution)
		if err != nil {
			return err
		}
		if err := validateXimeiReferenceDurations(info, product); err != nil {
			return err
		}
		// Allow user-written @ImageN/@AudioN/@VideoN. composeSeedancePromptWithMediaHints
		// skips role redefinition when the user already owns numbers.
		if compiledPrompt := compileXimeiPrompt(info); profile.PromptLimit > 0 && utf8.RuneCountInString(compiledPrompt) > profile.PromptLimit {
			return fmt.Errorf("compiled prompt exceeds the %d character limit for model %s", profile.PromptLimit, info.Model)
		}
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
	if profile.PromptEnhanceMode == "" {
		return nil, fmt.Errorf("model %s does not support prompt_enhance", strings.TrimSpace(model))
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
