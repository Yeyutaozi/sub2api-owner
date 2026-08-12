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
	require.Equal(t, DefaultWeijinVideoBaseURL, account.GetSeedanceBaseURL())
	require.True(t, account.IsModelSupported(SeedanceWeijinFaceRef480pModel))
	require.True(t, account.IsModelSupported(SeedanceWeijinFaceRef720pModel))
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

func TestBuildWeijinRequestMapsPublicSchema(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "测试卡人脸人物缓慢转头，表情自然",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References: []SeedanceReferenceImage{{URL: "https://example.com/face.png"}},
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
	require.Equal(t, "测试卡人脸人物缓慢转头，表情自然", payload["prompt"])
	images, ok := payload["images"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(images), 1)
	videos, ok := payload["videos"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"https://example.com/ref.mp4"}, videos)
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
	require.Equal(t, true, payload["audio"])
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
	require.Equal(t, true, payload["audio"])
	require.Len(t, payload["images"].([]any), 9)
	require.Len(t, payload["videos"].([]any), 3)
	require.Equal(t, []any{
		"https://example.com/a1.mp3",
		"https://example.com/a2.mp3",
		"https://example.com/a3.mp3",
	}, payload["audios"].([]any))
}

func TestWeijinFaceModelsAllowNineThreeThreeMedia(t *testing.T) {
	for _, model := range []string{SeedanceWeijinFaceRef480pModel, SeedanceWeijinFaceRef720pModel} {
		res := VideoBillingResolution480P
		if model == SeedanceWeijinFaceRef720pModel {
			res = VideoBillingResolution720P
		}
		info := &SeedanceRequestInfo{
			Model: model, Prompt: "face test", DurationSeconds: 5, Resolution: res, AspectRatio: "16:9",
			GenerateAudio: true,
			References: make([]SeedanceReferenceImage, 9),
			VideoReferences: make([]SeedanceReferenceVideo, 3),
			AudioReferences: make([]SeedanceReferenceAudio, 3),
		}
		require.NoError(t, validateFFLinkVideoRequestInfo(info), model)

		tooManyAudio := *info
		tooManyAudio.AudioReferences = make([]SeedanceReferenceAudio, 4)
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyAudio), "at most 3 reference audio files")
	}
}

func TestSanitizeWeijinSeedanceUpstreamErrorBodyScrubsVendor(t *testing.T) {
	body := []byte(`{"error":{"message":"weijinapi upstream failed at https://www.weijinapi.top/v1/videos one-api channel","code":"upstream_error"}}`)
	sanitized := sanitizeWeijinSeedanceUpstreamErrorBody(body)
	text := string(sanitized)
	require.NotContains(t, strings.ToLower(text), "weijin")
	require.NotContains(t, strings.ToLower(text), "weijinapi")
	require.NotContains(t, strings.ToLower(text), "one-api")
	require.NotContains(t, strings.ToLower(text), "oneapi")
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
