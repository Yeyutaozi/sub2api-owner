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
	require.Equal(t, "lingdong", route)

	_, err = decideWeijinSeedanceRoute(plain, &SeedanceRequestInfo{
		Prompt:          "hello",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	})
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "参考视频")

	_, err = decideWeijinSeedanceRoute(mapped, &SeedanceRequestInfo{
		Prompt:          "hello",
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	})
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Contains(t, string(upstreamErr.Body), "音频")
	require.NotContains(t, strings.ToLower(string(upstreamErr.Body)), "lingdong")
	require.NotContains(t, strings.ToLower(string(upstreamErr.Body)), "cvk")
}

func TestBuildLingdongVideoCreateRequestTruncatesVideosAndOmitsAudio(t *testing.T) {
	images := make([]SeedanceReferenceImage, 9)
	for i := range images {
		images[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/img%d.png", i+1)}
	}
	videos := make([]SeedanceReferenceVideo, 3)
	for i := range videos {
		videos[i] = SeedanceReferenceVideo{URL: fmt.Sprintf("https://example.com/v%d.mp4", i+1)}
	}
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "nine images and three videos",
		DurationSeconds: 12,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      images,
		VideoReferences: videos,
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a1.mp3"}},
	}

	body, err := buildLingdongVideoCreateRequest(info, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, DefaultLingdongUpstreamModel, payload["model"])
	require.EqualValues(t, 12, payload["duration"])
	require.Equal(t, "16:9", payload["ratio"])
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, false, payload["watermark"])
	require.Len(t, payload["images"].([]any), 9)
	require.Equal(t, []any{
		"https://example.com/v1.mp4",
		"https://example.com/v2.mp4",
	}, payload["videos"].([]any))
	_, hasAudios := payload["audios"]
	require.False(t, hasAudios)
	_, hasSeconds := payload["seconds"]
	require.False(t, hasSeconds)
}

func TestBuildLingdongVideoCreateRequest480pModel(t *testing.T) {
	body, err := buildLingdongVideoCreateRequest(&SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef480pModel,
		Prompt:          "only prompt",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution480P,
		AspectRatio:     "9:16",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/v1.mp4"}},
	}, "cvk-s")
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "480p", payload["resolution"])
	require.Equal(t, "9:16", payload["ratio"])
}

func TestLingdongMappedTaskIDPrefix(t *testing.T) {
	publicID, err := publicLingdongMappedTaskID("task_abc123")
	require.NoError(t, err)
	require.Equal(t, "ldv1_task_abc123", publicID)
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
	require.Contains(t, string(upstreamErr.Body), "音频")

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
	require.Equal(t, "ldv1_task_upstream_1", resp.Result.ResponseID)
	require.Equal(t, "task_upstream_1", resp.Result.UpstreamResponseID)
	require.Equal(t, DefaultLingdongVideoBaseURL+lingdongVideoCreatePath, upstream.request.URL.String())
	require.Equal(t, "Bearer sk_ld_live", upstream.request.Header.Get("Authorization"))

	var seenBody map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &seenBody))
	require.Equal(t, []any{"https://example.com/v1.mp4", "https://example.com/v2.mp4"}, seenBody["videos"])
	require.NotContains(t, seenBody, "audios")
	require.Equal(t, DefaultLingdongUpstreamModel, seenBody["model"])
	require.EqualValues(t, 8, seenBody["duration"])

	var publicBody map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &publicBody))
	require.Equal(t, "ldv1_task_upstream_1", publicBody["id"])
	require.Equal(t, "ldv1_task_upstream_1", publicBody["task_id"])

	// Poll should strip public prefix and hit lingdong status path.
	upstream.reply = `{"id":"task_upstream_1","status":"completed"}`
	_, err = svc.forwardWeijinSeedance(context.Background(), nil, account, http.MethodGet, resp.Result.ResponseID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, DefaultLingdongVideoBaseURL+lingdongVideoTaskPath+"/task_upstream_1", upstream.request.URL.String())
}

func TestNormalizeSeedanceJobTreatsLingdongPrefixAsOpaque(t *testing.T) {
	body := []byte(`{"id":"task_upstream_1","status":"completed","result":{"data":[{"url":"https://lingdongapi.com/secret.mp4"}]}}`)
	normalized, err := NormalizeSeedanceJob(body, "ldv1_task_upstream_1")
	require.NoError(t, err)
	var job map[string]any
	require.NoError(t, json.Unmarshal(normalized, &job))
	require.Equal(t, "ldv1_task_upstream_1", job["id"])
	require.Equal(t, "completed", job["status"])
	raw := string(normalized)
	require.NotContains(t, raw, "lingdongapi.com")
	require.Contains(t, raw, "/content")
}
