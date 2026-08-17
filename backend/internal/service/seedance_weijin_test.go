package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const legacyWeijin900PublicModelForTest = "sd-2.0-900"

func TestWeijinVideoProviderRoutesOnlySupportedModels(t *testing.T) {
	account := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_live_test",
			"video_provider": VideoProviderWeijin,
		},
	}

	require.True(t, account.IsWeijinVideo())
	require.Equal(t, "sd-2.0-900-720p", SeedanceWeijin900Model)
	require.Equal(t, "seedance2.0-900-3", SeedanceWeijin900UpstreamModel)
	require.Equal(t, DefaultWeijinVideoBaseURL, account.GetSeedanceBaseURL())
	require.True(t, account.IsModelSupported(SeedanceWeijinFaceRef480pModel))
	require.True(t, account.IsModelSupported(SeedanceWeijinFaceRef720pModel))
	require.False(t, account.IsModelSupported(SeedanceWeijin900Model), "the dedicated 900 tier requires an explicit account mapping")
	require.False(t, account.IsModelSupported(legacyWeijin900PublicModelForTest))
	require.False(t, account.IsModelSupported("seedance-2.0"))
	require.False(t, account.IsModelSupported(SeedanceMX933Model))
	require.False(t, account.IsModelSupported(SeedanceXimeiSD20Model))

	fflink := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ff"}}
	huiqu := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "hq", "video_provider": VideoProviderHuiqu}}
	ximei := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "xm", "video_provider": VideoProviderXimei}}
	require.False(t, fflink.IsModelSupported(SeedanceWeijinFaceRef720pModel))
	require.False(t, huiqu.IsModelSupported(SeedanceWeijinFaceRef720pModel))
	require.False(t, ximei.IsModelSupported(SeedanceWeijinFaceRef720pModel))

	require.True(t, IsOpaqueSeedanceVideoProvider(VideoProviderWeijin))
	require.True(t, videoProviderSupportsModelForPlatform(PlatformSeedance, VideoProviderWeijin, SeedanceWeijinFaceRef480pModel))
	require.False(t, videoProviderSupportsModelForPlatform(PlatformSeedance, VideoProviderFFLink, SeedanceWeijinFaceRef480pModel))
	require.False(t, videoProviderSupportsModelForPlatform(PlatformMiniMax, VideoProviderWeijin, SeedanceWeijinFaceRef480pModel))
}

func TestWeijin900AccountMappingIsolatesUpstreamKeys(t *testing.T) {
	legacy := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_legacy",
			"video_provider": VideoProviderWeijin,
			"model_mapping": map[string]any{
				SeedanceWeijinFaceRef480pModel: SeedanceWeijinFaceRef480pModel,
				SeedanceWeijinFaceRef720pModel: SeedanceWeijinFaceRef720pModel,
			},
		},
	}
	dedicated := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_900",
			"video_provider": VideoProviderWeijin,
			"model_mapping": map[string]any{
				SeedanceWeijin900Model: SeedanceWeijin900UpstreamModel,
			},
		},
	}
	legacyAlias := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_old_alias",
			"video_provider": VideoProviderWeijin,
			"model_mapping": map[string]any{
				legacyWeijin900PublicModelForTest: SeedanceWeijin900UpstreamModel,
			},
		},
	}

	require.False(t, legacy.IsModelSupported(SeedanceWeijin900Model))
	require.True(t, legacy.IsModelSupported(SeedanceWeijinFaceRef720pModel))
	require.True(t, dedicated.IsModelSupported(SeedanceWeijin900Model))
	require.False(t, dedicated.IsModelSupported(legacyWeijin900PublicModelForTest))
	require.False(t, dedicated.IsModelSupported(SeedanceWeijinFaceRef720pModel))
	require.False(t, legacyAlias.IsModelSupported(SeedanceWeijin900Model))
	require.False(t, legacyAlias.IsModelSupported(legacyWeijin900PublicModelForTest))
	require.Equal(t, SeedanceWeijin900UpstreamModel, dedicated.GetMappedModel(SeedanceWeijin900Model))

	gateway := &GatewayService{}
	require.True(t, gateway.isModelSupportedByAccount(dedicated, SeedanceWeijin900Model))
	require.False(t, gateway.isModelSupportedByAccount(legacy, SeedanceWeijin900Model))
	require.False(t, gateway.isModelSupportedByAccount(legacyAlias, SeedanceWeijin900Model))
}

func TestValidateWeijinVideoAccountConfiguration(t *testing.T) {
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderWeijin,
		"model_mapping": map[string]any{
			SeedanceWeijinFaceRef480pModel: SeedanceWeijinFaceRef480pModel,
			SeedanceWeijinFaceRef720pModel: SeedanceWeijinFaceRef720pModel,
		},
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeOAuth, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderWeijin,
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderWeijin,
		"model_mapping":  map[string]any{SeedanceWeijinFaceRef720pModel: "seedance-2.0"},
	}))
	_, err := normalizeVideoProvider(PlatformMiniMax, VideoProviderWeijin)
	require.Error(t, err)
}

func TestValidateWeijin900AccountConfigurationRequiresDedicatedMapping(t *testing.T) {
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_900",
		"video_provider": VideoProviderWeijin,
		"model_mapping": map[string]any{
			SeedanceWeijin900Model: SeedanceWeijin900UpstreamModel,
		},
	}))

	for _, mapping := range []map[string]any{
		{SeedanceWeijin900Model: SeedanceWeijin900Model},
		{legacyWeijin900PublicModelForTest: SeedanceWeijin900UpstreamModel},
		{legacyWeijin900PublicModelForTest: legacyWeijin900PublicModelForTest},
		{SeedanceWeijinFaceRef720pModel: SeedanceWeijin900UpstreamModel},
		{SeedanceWeijin900UpstreamModel: SeedanceWeijinFaceRef720pModel},
		{SeedanceWeijin900UpstreamModel: SeedanceWeijin900UpstreamModel},
		{strings.ToUpper(SeedanceWeijin900Model): SeedanceWeijin900UpstreamModel},
	} {
		require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
			"api_key":        "sk_invalid",
			"video_provider": VideoProviderWeijin,
			"model_mapping":  mapping,
		}))
	}
}

func TestGroupAllowsWeijin900ExposureOnlyWithCanonicalPublicIDAndPriceCard(t *testing.T) {
	price720 := 0.05
	legacyPrice720 := 0.02
	group := &Group{
		Platform:       PlatformSeedance,
		VideoPrice720P: &legacyPrice720,
	}

	require.False(t, GroupAllowsVideoModelExposure(group, SeedanceWeijin900Model))
	require.False(t, GroupAllowsVideoModelExposure(group, legacyWeijin900PublicModelForTest))
	require.False(t, GroupAllowsVideoModelExposure(group, SeedanceWeijin900UpstreamModel))
	require.True(t, GroupAllowsVideoModelExposure(group, SeedanceWeijinFaceRef720pModel))

	group.VideoModelPrices = VideoModelPrices{
		SeedanceWeijin900Model: {BillingUnit: VideoBillingUnitPerRequest, Price720P: &price720},
	}
	require.True(t, GroupAllowsVideoModelExposure(group, SeedanceWeijin900Model))
	require.False(t, GroupAllowsVideoModelExposure(group, legacyWeijin900PublicModelForTest))
	require.False(t, GroupAllowsVideoModelExposure(group, strings.ToUpper(SeedanceWeijin900Model)))
	require.False(t, GroupAllowsVideoModelExposure(group, SeedanceWeijin900UpstreamModel))
}

func TestWeijin900ProfileExposesOnlyVerifiedCapabilities(t *testing.T) {
	models := FFLinkVideoModelIDsForPlatform(PlatformSeedance)
	require.Contains(t, models, SeedanceWeijin900Model)
	require.NotContains(t, models, legacyWeijin900PublicModelForTest)
	require.NotContains(t, models, SeedanceWeijin900UpstreamModel)
	_, legacyProfileExists := ffLinkVideoModelProfileFor(legacyWeijin900PublicModelForTest)
	require.False(t, legacyProfileExists)

	profile, ok := ffLinkVideoModelProfileFor(SeedanceWeijin900Model)
	require.True(t, ok)
	require.Equal(t, VideoBillingResolution720P, profile.DefaultResolution)
	require.Equal(t, 9, profile.MaxImageReferences)
	require.Equal(t, 9, profile.MaxTotalImages)
	require.Zero(t, profile.MaxVideoReferences)
	require.Zero(t, profile.MaxAudioReferences)
	require.Equal(t, 9, profile.MaxTotalMedia)
	require.False(t, profile.AllowGeneratedAudio)
	require.Equal(t, ratioSet("16:9"), profile.AllowedAspectRatios)

	for duration := 5; duration <= 15; duration++ {
		info := &SeedanceRequestInfo{
			Model: SeedanceWeijin900Model, Prompt: "animate all references",
			Resolution: VideoBillingResolution720P, DurationSeconds: duration, AspectRatio: "16:9",
			References: make([]SeedanceReferenceImage, 9),
		}
		require.NoError(t, validateFFLinkVideoRequestInfo(info), "duration=%d", duration)
	}

	for _, duration := range []int{4, 16} {
		info := &SeedanceRequestInfo{
			Model: SeedanceWeijin900Model, Prompt: "duration boundary",
			Resolution: VideoBillingResolution720P, DurationSeconds: duration, AspectRatio: "16:9",
		}
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), "duration")
	}

	base := func() *SeedanceRequestInfo {
		return &SeedanceRequestInfo{
			Model: SeedanceWeijin900Model, Prompt: "capability validation",
			Resolution: VideoBillingResolution720P, DurationSeconds: 5, AspectRatio: "16:9",
		}
	}
	tooManyImages := base()
	tooManyImages.References = make([]SeedanceReferenceImage, 10)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooManyImages), "at most 9 reference images")
	withVideo := base()
	withVideo.VideoReferences = []SeedanceReferenceVideo{{URL: "https://example.com/reference.mp4"}}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(withVideo), "at most 0 reference videos")
	withAudio := base()
	withAudio.AudioReferences = []SeedanceReferenceAudio{{URL: "https://example.com/reference.mp3"}}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(withAudio), "at most 0 reference audio files")
	generatedAudio := base()
	generatedAudio.GenerateAudio = true
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(generatedAudio), "does not support generated audio")
	wrongRatio := base()
	wrongRatio.AspectRatio = "9:16"
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(wrongRatio), "aspect_ratio")
	wrongResolution := base()
	wrongResolution.Resolution = VideoBillingResolution480P
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(wrongResolution), "resolution")
}

func TestBuildWeijin900RequestUsesPrivateModelWithoutFacePromptInjection(t *testing.T) {
	images := make([]SeedanceReferenceImage, 9)
	for i := range images {
		images[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/image-%d.png", i+1)}
	}
	info := &SeedanceRequestInfo{
		Model: SeedanceWeijin900Model, Prompt: "Keep the nine reference subjects consistent.",
		Resolution: VideoBillingResolution720P, DurationSeconds: 11, AspectRatio: "16:9",
		References: images,
	}
	upstreamModel, err := weijinUpstreamModelFor(info, SeedanceWeijin900UpstreamModel)
	require.NoError(t, err)
	require.Equal(t, SeedanceWeijin900UpstreamModel, upstreamModel)

	body, err := buildWeijinVideoCreateRequest(info, upstreamModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, SeedanceWeijin900UpstreamModel, payload["model"])
	require.Equal(t, info.Prompt, payload["prompt"])
	require.EqualValues(t, 11, payload["seconds"])
	require.Equal(t, "16:9", payload["aspect_ratio"])
	require.Len(t, payload["images"], 9)
	require.NotContains(t, payload, "videos")
	require.NotContains(t, payload, "audios")
	require.NotContains(t, payload, "audio")
	require.NotContains(t, payload, "resolution")

	_, err = weijinUpstreamModelFor(info, SeedanceWeijin900Model)
	require.ErrorContains(t, err, "explicit account mapping")
	_, err = buildWeijinVideoCreateRequest(info, SeedanceWeijin900Model)
	require.ErrorContains(t, err, "explicit account mapping")
}

func TestBuildWeijinRequestMapsPublicSchema(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "测试卡人脸人物缓慢转头，表情自然",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/face.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/ref.mp4"}},
		StartFrameURL:   "https://example.com/start.png",
		EndFrameURL:     "https://example.com/end.png",
	}
	body, err := buildWeijinVideoCreateRequest(info, SeedanceWeijinFaceRef720pModel)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, SeedanceWeijinFaceRef720pModel, payload["model"])
	require.EqualValues(t, 5, payload["seconds"])
	require.Equal(t, "16:9", payload["aspect_ratio"])
	prompt, ok := payload["prompt"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(prompt, "测试卡人脸人物缓慢转头，表情自然"))
	require.Contains(t, prompt, seedanceFaceRefQualityHintMarker)
	images, ok := payload["images"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(images), 1)
	_, hasVideos := payload["videos"]
	require.False(t, hasVideos, "pure Weijin create must not forward reference videos")
	_, hasAudios := payload["audios"]
	require.False(t, hasAudios, "pure Weijin create must not forward reference audios")
	_, hasDurationSeconds := payload["duration_seconds"]
	require.False(t, hasDurationSeconds)
	_, hasSize := payload["size"]
	require.False(t, hasSize)
}

func TestWeijinUpstreamModelEnforcesFixedResolutionAndDuration(t *testing.T) {
	_, err := weijinUpstreamModelFor(&SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef720pModel, DurationSeconds: 5, Resolution: VideoBillingResolution480P,
	}, SeedanceWeijinFaceRef720pModel)
	require.Error(t, err)

	_, err = weijinUpstreamModelFor(&SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef480pModel, DurationSeconds: 3, Resolution: VideoBillingResolution480P,
	}, SeedanceWeijinFaceRef480pModel)
	require.Error(t, err)

	model, err := weijinUpstreamModelFor(&SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef480pModel, DurationSeconds: 15, Resolution: VideoBillingResolution480P,
	}, SeedanceWeijinFaceRef480pModel)
	require.NoError(t, err)
	require.Equal(t, SeedanceWeijinFaceRef480pModel, model)

	body, err := buildWeijinVideoCreateRequest(&SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef720pModel, Prompt: "x", DurationSeconds: 5,
		Resolution: VideoBillingResolution720P, AspectRatio: "16:9", GenerateAudio: true,
	}, SeedanceWeijinFaceRef720pModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	_, hasAudio := payload["audio"]
	require.False(t, hasAudio, "Weijin create must not send boolean audio field")
}

func TestBuildWeijinRequestForwardsFullMixedMediaLoad(t *testing.T) {
	images := make([]SeedanceReferenceImage, 9)
	for i := range images {
		images[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/img%d.png", i+1)}
	}
	videos := make([]SeedanceReferenceVideo, 3)
	for i := range videos {
		videos[i] = SeedanceReferenceVideo{URL: fmt.Sprintf("https://example.com/v%d.mp4", i+1)}
	}
	audios := make([]SeedanceReferenceAudio, 3)
	for i := range audios {
		audios[i] = SeedanceReferenceAudio{URL: fmt.Sprintf("https://example.com/a%d.mp3", i+1)}
	}
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "测试卡人脸，参考图中人物保持身份一致，缓慢转头，口型随参考音频自然变化，无字幕无水印",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
		References:      images,
		VideoReferences: videos,
		AudioReferences: audios,
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(info))

	body, err := buildWeijinVideoCreateRequest(info, SeedanceWeijinFaceRef720pModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	_, hasAudio := payload["audio"]
	require.False(t, hasAudio, "Weijin create must not send boolean audio field")
	require.Len(t, payload["images"].([]any), 9)
	_, hasVideos := payload["videos"]
	require.False(t, hasVideos, "pure Weijin path is images-only; videos map via Lingdong")
	_, hasAudios := payload["audios"]
	require.False(t, hasAudios, "pure Weijin path is images-only; audio is rejected at route layer")
}

func TestWeijinFaceModelsAllowNineThreeThreeMedia(t *testing.T) {
	for _, model := range []string{SeedanceWeijinFaceRef480pModel, SeedanceWeijinFaceRef720pModel} {
		res := VideoBillingResolution480P
		if model == SeedanceWeijinFaceRef720pModel {
			res = VideoBillingResolution720P
		}
		info := &SeedanceRequestInfo{
			Model: model, Prompt: "face test", DurationSeconds: 5, Resolution: res, AspectRatio: "16:9",
			GenerateAudio:   true,
			References:      make([]SeedanceReferenceImage, 9),
			VideoReferences: make([]SeedanceReferenceVideo, 3),
			AudioReferences: make([]SeedanceReferenceAudio, 3),
		}
		require.NoError(t, validateFFLinkVideoRequestInfo(info), model)

		tooManyAudio := *info
		tooManyAudio.AudioReferences = make([]SeedanceReferenceAudio, 4)
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyAudio), "at most 3 reference audio files")
	}
}

func TestSeedancePublicUpstreamErrorKeepsReadableAdapterMessage(t *testing.T) {
	body := []byte(`{"error":{"code":"adapter_error","message":"Xmanway HTTP 400: 参考视频分辨率必须在 480p 和 720p 之间"}}`)
	code, msg := SeedancePublicUpstreamError(400, body)
	require.Equal(t, "invalid_request", code)
	require.Equal(t, "参考视频分辨率必须在 480p 和 720p 之间", msg)
	require.NotContains(t, strings.ToLower(msg), "xmanway")

	nested := []byte(`{"error":{"code":"upstream_error","message":"{\"error\":{\"code\":\"adapter_error\",\"message\":\"Xmanway HTTP 400: 参考视频分辨率必须在 480p 和 720p 之间\"}}"}}`)
	code, msg = SeedancePublicUpstreamError(400, nested)
	require.Equal(t, "invalid_request", code)
	require.Equal(t, "参考视频分辨率必须在 480p 和 720p 之间", msg)
	require.NotContains(t, strings.ToLower(msg), "xmanway")

	// plain text body path (some providers store extracted message bytes only)
	plain := []byte(`Xmanway HTTP 400: 参考视频分辨率必须在 480p 和 720p 之间`)
	require.Equal(t, "参考视频分辨率必须在 480p 和 720p 之间", SeedanceUpstreamErrorMessage(plain))
}

func TestSanitizeWeijinSeedanceUpstreamErrorBodyScrubsVendor(t *testing.T) {
	body := []byte(`{"error":{"message":"weijinapi upstream rejected SEEDANCE2.0-900-3 at https://www.weijinapi.top/v1/videos one-api channel","code":"upstream_error"}}`)
	sanitized := sanitizeWeijinSeedanceUpstreamErrorBody(body)
	text := string(sanitized)
	require.NotContains(t, strings.ToLower(text), "weijin")
	require.NotContains(t, strings.ToLower(text), "weijinapi")
	require.NotContains(t, strings.ToLower(text), "one-api")
	require.NotContains(t, strings.ToLower(text), "oneapi")
	require.NotContains(t, strings.ToLower(text), SeedanceWeijin900UpstreamModel)
	require.Contains(t, text, SeedanceWeijin900Model)
}

func TestNormalizeWeijinFailedJobHidesUpstreamError(t *testing.T) {
	job := map[string]any{
		"status": "failed",
		"error":  map[string]any{"message": "weijinapi channel unavailable"},
	}
	normalizeSeedancePublicJob(job, "task_abc", VideoProviderWeijin, SeedanceWeijinFaceRef720pModel)
	require.Equal(t, "failed", job["status"])
	errObj, ok := job["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Video generation failed", errObj["message"])
}

func TestBuildWeijinOfficialFailedResponseHidesVendor(t *testing.T) {
	upstream := []byte(`{"id":"vid_123","status":"failed","error":{"message":"weijinapi rejected request"}}`)
	resp, err := BuildSeedanceOfficialTaskResponseForRoute(
		"vid_123", upstream, "", VideoProviderWeijin, SeedanceWeijinFaceRef720pModel,
	)
	require.NoError(t, err)
	require.Equal(t, "failed", resp["status"])
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Video generation failed", errObj["message"])
	raw, _ := json.Marshal(resp)
	require.NotContains(t, strings.ToLower(string(raw)), "weijin")
}

func TestWeijinDeleteNotSupported(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_live_test",
			"video_provider": VideoProviderWeijin,
			"base_url":       DefaultWeijinVideoBaseURL,
		},
	}
	_, err := svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodDelete, "task_1", nil, nil)
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusMethodNotAllowed, upstreamErr.StatusCode)
}
