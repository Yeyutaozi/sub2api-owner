package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func weijinAccountWithLingdongMapping(enabled bool, lingdongKey string) *Account {
	creds := map[string]any{
		"api_key":        "sk_weijin_test",
		"video_provider": VideoProviderWeijin,
		"base_url":       DefaultWeijinVideoBaseURL,
	}
	if enabled {
		creds[credentialLingdongMappingEnabled] = true
		if lingdongKey != "" {
			creds[credentialLingdongAPIKey] = lingdongKey
		}
		creds[credentialLingdongBaseURL] = DefaultLingdongVideoBaseURL
		creds[credentialLingdongUpstreamModel] = DefaultLingdongUpstreamModel
	}
	return &Account{
		ID:          42,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Credentials: creds,
	}
}

func TestLingdongMappingCredentialsAndValidation(t *testing.T) {
	account := weijinAccountWithLingdongMapping(true, "sk_lingdong_test")
	require.True(t, account.IsWeijinVideo())
	require.True(t, account.IsLingdongMappingEnabled())
	require.True(t, account.IsLingdongMappingReady())
	require.Equal(t, "sk_lingdong_test", account.GetLingdongAPIKey())
	require.Equal(t, DefaultLingdongVideoBaseURL, account.GetLingdongBaseURL())
	require.Equal(t, DefaultLingdongUpstreamModel, account.GetLingdongUpstreamModel())

	disabled := weijinAccountWithLingdongMapping(false, "")
	require.False(t, disabled.IsLingdongMappingEnabled())
	require.False(t, disabled.IsLingdongMappingReady())

	enabledNoKey := weijinAccountWithLingdongMapping(true, "")
	require.True(t, enabledNoKey.IsLingdongMappingEnabled())
	require.False(t, enabledNoKey.IsLingdongMappingReady())

	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":                        "sk_weijin",
		"video_provider":                 VideoProviderWeijin,
		credentialLingdongMappingEnabled: true,
		credentialLingdongAPIKey:         "sk_lingdong",
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":                        "sk_weijin",
		"video_provider":                 VideoProviderWeijin,
		credentialLingdongMappingEnabled: true,
	}))
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_weijin",
		"video_provider": VideoProviderWeijin,
	}))
}

func TestDecideWeijinSeedanceRoute(t *testing.T) {
	mapped := weijinAccountWithLingdongMapping(true, "sk_ld")
	plain := weijinAccountWithLingdongMapping(false, "")

	route, err := decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:     "hello",
		References: []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
	})
	require.NoError(t, err)
	require.Equal(t, "weijin", route)

	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:          "hello",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "pixelle", route)

	_, err = decideWeijinSeedanceRoute(plain, &SeedanceRequestInfo{
		Prompt:          "hello",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "参考视频")

	// Audio alone still needs at least one image for Pixelle.
	_, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:          "hello",
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "参考图")

	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.NoError(t, err)
	require.Equal(t, "pixelle", route)

	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.NoError(t, err)
	require.Equal(t, "pixelle", route)

		// Empty mapping: 480p + reference video stays on Weijin (native, cheap path).
	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "weijin", route)

	// Plain account (no extension mapping): 480p + video still Weijin.
	route, err = decideWeijinSeedanceRoute(plain, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "weijin", route)

	// 480p + audio is NOT native Weijin; without 480p mapping it is rejected.
	_, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)

// Empty mapping: 720p multi-modal still allowed.
	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "pixelle", route)

	// Explicit mapping can enable 480p if admin chooses (and picks upstream model).
	mapped.Credentials[credentialPixelleModelMapping] = map[string]any{
		SeedanceWeijinFaceRef480pModel: "sora-v3-fast",
		SeedanceWeijinFaceRef720pModel: "sora-v3-pro",
	}
	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "pixelle", route)
	up, ok := mapped.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef480pModel)
	require.True(t, ok)
	require.Equal(t, "sora-v3-fast", up)
	up, ok = mapped.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef720pModel)
	require.True(t, ok)
	require.Equal(t, "sora-v3-pro", up)

		// Explicit mapping without 480p entry: 480p+video stays on native Weijin
	// (do not force expensive Pixelle). Audio still requires a 480p mapping entry.
	mapped.Credentials[credentialPixelleModelMapping] = map[string]any{
		SeedanceWeijinFaceRef720pModel: "sora-v3-pro",
	}
	route, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	require.NoError(t, err)
	require.Equal(t, "weijin", route)

	_, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "hello",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
}

func TestShouldFallbackWeijin720pCreateToMapped(t *testing.T) {
	mapped := weijinAccountWithLingdongMapping(true, "sk_ld")
	plain := weijinAccountWithLingdongMapping(false, "")
	request := &SeedanceRequestInfo{Model: SeedanceWeijinFaceRef720pModel, Prompt: "hello"}

	require.True(t, shouldFallbackWeijin720pCreateToMapped(mapped, request, &UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.True(t, shouldFallbackWeijin720pCreateToMapped(mapped, request, &SeedanceUpstreamError{StatusCode: http.StatusServiceUnavailable}))
	require.False(t, shouldFallbackWeijin720pCreateToMapped(mapped, request, &SeedanceUpstreamError{StatusCode: http.StatusTooManyRequests}))
	require.False(t, shouldFallbackWeijin720pCreateToMapped(plain, request, &SeedanceUpstreamError{StatusCode: http.StatusBadGateway}))

	withVideo := *request
	withVideo.VideoReferences = []SeedanceReferenceVideo{{URL: "https://example.com/v.mp4"}}
	require.False(t, shouldFallbackWeijin720pCreateToMapped(mapped, &withVideo, &SeedanceUpstreamError{StatusCode: http.StatusBadGateway}))

	request480p := *request
	request480p.Model = SeedanceWeijinFaceRef480pModel
	require.False(t, shouldFallbackWeijin720pCreateToMapped(mapped, &request480p, &SeedanceUpstreamError{StatusCode: http.StatusBadGateway}))
}

func TestResolveLingdongMappedUpstreamModel(t *testing.T) {
	acc := weijinAccountWithLingdongMapping(true, "sk_ld")
	up, ok := acc.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef720pModel)
	require.True(t, ok)
	require.Equal(t, DefaultLingdongUpstreamModel, up)
	_, ok = acc.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef480pModel)
	require.False(t, ok)

	acc.Credentials[credentialPixelleUpstreamModel] = "sora-v3-pro"
	acc.Credentials[credentialPixelleModelMapping] = map[string]any{
		SeedanceWeijinFaceRef480pModel: "cheap-model",
	}
	up, ok = acc.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef480pModel)
	require.True(t, ok)
	require.Equal(t, "cheap-model", up)
	_, ok = acc.ResolveLingdongMappedUpstreamModel(SeedanceWeijinFaceRef720pModel)
	require.False(t, ok)
}

func TestBuildLingdongVideoCreateRequestTruncatesVideosAndIncludesAudio(t *testing.T) {
	images := make([]SeedanceReferenceImage, 7)
	for i := range images {
		images[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/img%d.png", i+1)}
	}
	videos := make([]SeedanceReferenceVideo, 4)
	for i := range videos {
		videos[i] = SeedanceReferenceVideo{URL: fmt.Sprintf("https://example.com/v%d.mp4", i+1)}
	}
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "seven images, three videos, two audios",
		DurationSeconds: 12,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      images,
		VideoReferences: videos,
		AudioReferences: []SeedanceReferenceAudio{
			{URL: "https://example.com/a1.mp3"},
			{URL: "https://example.com/a2.mp3"},
			{URL: "https://example.com/a3.mp3"},
		},
	}

	body, err := buildLingdongVideoCreateRequest(info, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, DefaultLingdongUpstreamModel, payload["model"])
	require.Equal(t, "sora-v3-pro", DefaultLingdongUpstreamModel)
	require.Equal(t, "15", payload["seconds"])
	require.Equal(t, "16:9", payload["aspect_ratio"])
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, "https://example.com/img1.png", payload["image_url"])
	require.Len(t, payload["reference_image_urls"].([]any), 6)
	// 7 images + max 3 videos leave room for 2 audios under total=12; 4th video dropped.
	require.Equal(t, []any{
		"https://example.com/v1.mp4",
		"https://example.com/v2.mp4",
		"https://example.com/v3.mp4",
	}, payload["reference_videos"].([]any))
	_, hasAudioURL := payload["audio_url"]
	require.False(t, hasAudioURL)
	require.Equal(t, []any{
		"https://example.com/a1.mp3",
		"https://example.com/a2.mp3",
	}, payload["audio_urls"].([]any))
	_, hasDuration := payload["duration"]
	require.False(t, hasDuration)
}

func TestBuildLingdongVideoCreateRequest480pModel(t *testing.T) {
	body, err := buildLingdongVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "only prompt",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution480P,
		AspectRatio:     "9:16",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	}, "sora-v3-pro")
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "480p", payload["resolution"])
	require.Equal(t, "9:16", payload["aspect_ratio"])
	require.Equal(t, "10", payload["seconds"])
	require.Equal(t, "https://example.com/v1.mp4", payload["reference_video"])
}

func TestBuildLingdongVideoCreateRequestCapsTotalAt12(t *testing.T) {
	images := make([]SeedanceReferenceImage, 9)
	for i := range images {
		images[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/img%d.png", i+1)}
	}
	videos := make([]SeedanceReferenceVideo, 3)
	for i := range videos {
		videos[i] = SeedanceReferenceVideo{URL: fmt.Sprintf("https://example.com/v%d.mp4", i+1)}
	}
	body, err := buildLingdongVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "9i3v total 12",
		DurationSeconds: 15,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      images,
		VideoReferences: videos,
	}, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Len(t, payload["reference_videos"].([]any), 3)
}

func TestLegacyLingdongTaskPrefixStillRecognized(t *testing.T) {
	require.True(t, IsLingdongMappedSeedanceTaskID("ldv1_task_old"))
	upstream, err := upstreamLingdongMappedTaskID("ldv1_task_old")
	require.NoError(t, err)
	require.Equal(t, "task_old", upstream)
}

func TestLingdongMappedTaskIDPrefix(t *testing.T) {
	publicID, err := publicLingdongMappedTaskID("task_abc123")
	require.NoError(t, err)
	require.Equal(t, "pxv1_task_abc123", publicID)
	require.True(t, IsLingdongMappedSeedanceTaskID(publicID))
	upstream, err := upstreamLingdongMappedTaskID(publicID)
	require.NoError(t, err)
	require.Equal(t, "task_abc123", upstream)
	_, err = upstreamLingdongMappedTaskID("task_abc123")
	require.Error(t, err)
}

func TestSanitizeLingdongNamesInPublicErrors(t *testing.T) {
	msg := scrubSeedancePublicErrorMessage("lingdongapi cvk-s rejected the payload")
	require.NotContains(t, strings.ToLower(msg), "lingdong")
	require.NotContains(t, strings.ToLower(msg), "cvk")
	require.Contains(t, strings.ToLower(msg), "upstream provider")

	body := sanitizeLingdongSeedanceUpstreamErrorBody([]byte(`{"error":{"message":"lingdongapi channel cvk-s failed"}}`))
	require.NotContains(t, strings.ToLower(string(body)), "lingdong")
	require.NotContains(t, strings.ToLower(string(body)), "cvk-s")
}

func TestForwardWeijinRoutesAudioAndVideoBeforeUpstream(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := weijinAccountWithLingdongMapping(true, "sk_ld")

	_, err := svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodPost, "", &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "x",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a.mp3"}},
	}, nil)
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "参考图")

	plain := weijinAccountWithLingdongMapping(false, "")
	_, err = svc.forwardWeijinSeedance(context.Background(), nil, plain, http.MethodPost, "", &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "x",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v.mp4"}},
	}, nil)
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "参考视频")
}

func TestForwardLingdongMappedCreateUsesOpaqueTaskID(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_upstream_1","task_id":"task_upstream_1","status":"processing"}`}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := weijinAccountWithLingdongMapping(true, "sk_ld_live")

	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "map me",
		DurationSeconds: 8,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/1.png"}},
		VideoReferences: []SeedanceReferenceVideo{
			{URL: "https://example.com/v1.mp4"},
			{URL: "https://example.com/v2.mp4"},
			{URL: "https://example.com/v3.mp4"},
		},
	}
	resp, err := svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodPost, "", info, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Result)
	require.Equal(t, "pxv1_task_upstream_1", resp.Result.ResponseID)
	require.Equal(t, "task_upstream_1", resp.Result.UpstreamResponseID)
	require.Equal(t, DefaultLingdongVideoBaseURL+lingdongVideoCreatePath, upstream.request.URL.String())
	require.Equal(t, "Bearer sk_ld_live", upstream.request.Header.Get("Authorization"))

	var seenBody map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &seenBody))
	require.Equal(t, []any{"https://example.com/v1.mp4", "https://example.com/v2.mp4", "https://example.com/v3.mp4"}, seenBody["reference_videos"])
	require.NotContains(t, seenBody, "audios")
	require.NotContains(t, seenBody, "audio_url")
	require.Equal(t, DefaultLingdongUpstreamModel, seenBody["model"])
	require.Equal(t, "10", seenBody["seconds"])

	var publicBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &publicBody))
	require.Equal(t, "pxv1_task_upstream_1", publicBody["id"])
	require.Equal(t, "pxv1_task_upstream_1", publicBody["task_id"])

	// Poll should strip public prefix and hit lingdong status path.
	upstream.reply = `{"id":"task_upstream_1","status":"completed"}`
	_, err = svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodGet, resp.Result.ResponseID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, DefaultLingdongVideoBaseURL+lingdongVideoTaskPath+"/task_upstream_1", upstream.request.URL.String())
}


func TestForwardLingdongMappedCreateIncludesAudio(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_audio_1","task_id":"task_audio_1","status":"processing"}`}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := weijinAccountWithLingdongMapping(true, "sk_ld_live")

	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "with audio",
		DurationSeconds: 10,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/1.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{
			{URL: "https://example.com/a1.mp3"},
			{URL: "https://example.com/a2.mp3"},
		},
	}
	resp, err := svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodPost, "", info, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "pxv1_task_audio_1", resp.Result.ResponseID)

	var seenBody map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &seenBody))
	require.Equal(t, "https://example.com/1.png", seenBody["image_url"])
	require.Equal(t, "https://example.com/v1.mp4", seenBody["reference_video"])
	_, hasAudioURL := seenBody["audio_url"]
	require.False(t, hasAudioURL)
	require.Equal(t, []any{"https://example.com/a1.mp3", "https://example.com/a2.mp3"}, seenBody["audio_urls"].([]any))
}

func TestNormalizeSeedanceJobTreatsLingdongPrefixAsOpaque(t *testing.T) {
	body := []byte(`{"id":"task_upstream_1","status":"completed","result":{"data":[{"url":"https://lingdongapi.com/secret.mp4"}]}}`)
	normalized, err := NormalizeSeedanceJob(body, "pxv1_task_upstream_1")
	require.NoError(t, err)
	var job map[string]any
	require.NoError(t, json.Unmarshal(normalized, &job))
	require.Equal(t, "pxv1_task_upstream_1", job["id"])
	require.Equal(t, "completed", job["status"])
	raw := string(normalized)
	require.NotContains(t, raw, "lingdongapi.com")
	require.Contains(t, raw, "/content")
}


func TestSeedanceForwardTaskIDPrefersPublicLingdongID(t *testing.T) {
	// Settlement / list hydration must poll with ldv1_* so routing sticks to Lingdong.
	require.Equal(t, "ldv1_task_abc", seedanceForwardTaskID(&SeedanceTaskBinding{
		JobID:         "ldv1_task_abc",
		UpstreamJobID: "task_abc",
	}))
	// Non-mapped tasks keep the private upstream id when present.
	require.Equal(t, "task_plain", seedanceForwardTaskID(&SeedanceTaskBinding{
		JobID:         "task_public_alias",
		UpstreamJobID: "task_plain",
	}))
	require.Equal(t, "task_only", seedanceForwardTaskID(&SeedanceTaskBinding{
		JobID: "task_only",
	}))
	require.Equal(t, "", seedanceForwardTaskID(nil))
}

func TestComposeWeijinFaceRefPromptInjectsQualityHints(t *testing.T) {
	out := composeWeijinFaceRefPrompt("苏月坐在辇车上说话")
	require.Contains(t, out, "苏月坐在辇车上说话")
	require.Contains(t, out, seedanceFaceRefQualityHintMarker)
	require.Contains(t, out, "禁止远景虚化")
	require.Contains(t, out, "深景深")

	// idempotent when marker already present
	again := composeWeijinFaceRefPrompt(out)
	require.Equal(t, out, again)

	// user already expressed the intent
	user := "画面禁止远景虚化，全景深景深"
	require.Equal(t, user, composeWeijinFaceRefPrompt(user))
}

func TestBuildWeijinVideoCreateRequestInjectsQualityPrompt(t *testing.T) {
	body, err := buildWeijinVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "太后与太监对话",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
	}, SeedanceWeijinFaceRef720pModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	prompt, _ := payload["prompt"].(string)
	require.Contains(t, prompt, "太后与太监对话")
	require.Contains(t, prompt, seedanceFaceRefQualityHintMarker)
}


func TestBuildWeijinVideoCreateRequestIncludesVideosFor480p(t *testing.T) {
	body, err := buildWeijinVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "nine images plus video",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution480P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{
			{URL: "https://example.com/v1.mp4"},
			{URL: "https://example.com/v2.mp4"},
			{URL: "https://example.com/v3.mp4"},
			{URL: "https://example.com/v4.mp4"},
		},
	}, SeedanceWeijinFaceRef480pModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, []any{
		"https://example.com/v1.mp4",
		"https://example.com/v2.mp4",
		"https://example.com/v3.mp4",
	}, payload["videos"])
	require.NotContains(t, payload, "audios")
	require.NotContains(t, payload, "audio_url")

	// 720p Weijin builder never sends videos (multi-modal is Pixelle-only for 720p).
	body720, err := buildWeijinVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "should strip videos",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	}, SeedanceWeijinFaceRef720pModel)
	require.NoError(t, err)
	var payload720 map[string]any
	require.NoError(t, json.Unmarshal(body720, &payload720))
	require.NotContains(t, payload720, "videos")
}

func TestBuildLingdongVideoCreateRequestInjectsQualityPrompt(t *testing.T) {
	body, err := buildLingdongVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "辇车行驶近景对话",
		DurationSeconds: 10,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v.mp4"}},
	}, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	prompt, _ := payload["prompt"].(string)
	require.Contains(t, prompt, "辇车行驶近景对话")
	require.Contains(t, prompt, seedanceFaceRefQualityHintMarker)
	require.Contains(t, prompt, "禁止远景虚化")
}
