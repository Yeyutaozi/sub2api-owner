package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestForwardZhi168CreateSendsPublicReferenceMedia(t *testing.T) {
	upstream := &huiquCapturingUpstream{
		status: http.StatusOK,
		reply:  `{"task_id":12345,"status":"queued"}`,
	}
	gateway := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 49, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "zhi-secret",
			"video_provider": VideoProviderZhi168,
		},
	}
	request := &SeedanceRequestInfo{
		Model: SeedanceZhi168Model, Prompt: "animate all references",
		Resolution: VideoBillingResolution1080P, DurationSeconds: 15, AspectRatio: "16:9",
		References: []SeedanceReferenceImage{
			{URL: "https://litter.catbox.moe/reference-1.png"},
			{URL: "https://litter.catbox.moe/reference-2.png"},
		},
		StartFrameURL: "https://litter.catbox.moe/start.png",
		EndFrameURL:   "https://litter.catbox.moe/end.png",
		VideoReferences: []SeedanceReferenceVideo{
			{URL: "https://files.catbox.moe/reference.mp4"},
		},
		AudioReferences: []SeedanceReferenceAudio{
			{URL: "https://files.catbox.moe/reference.mp3"},
		},
	}

	response, err := gateway.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", request)

	require.NoError(t, err)
	require.NotNil(t, response.Result)
	require.Equal(t, "12345", response.Result.ResponseID)
	require.Equal(t, SeedanceZhi168UpstreamModel, response.Result.UpstreamModel)
	require.Equal(t, DefaultZhi168VideoBaseURL+"/v1/video-tasks", upstream.request.URL.String())
	require.Equal(t, "zhi-secret", upstream.request.Header.Get("X-API-Key"))

	var payload zhi168VideoRequest
	require.NoError(t, json.Unmarshal(upstream.body, &payload))
	require.Equal(t, SeedanceZhi168UpstreamModel, payload.ModelCode)
	require.Equal(t, []string{
		"https://litter.catbox.moe/reference-1.png",
		"https://litter.catbox.moe/reference-2.png",
		"https://litter.catbox.moe/start.png",
		"https://litter.catbox.moe/end.png",
	}, payload.ReferenceImageURLs)
	require.Equal(t, []string{"https://files.catbox.moe/reference.mp4"}, payload.VideoURLs)
	require.Equal(t, []string{"https://files.catbox.moe/reference.mp3"}, payload.AudioURLs)
	allMediaURLs := append([]string{}, payload.ReferenceImageURLs...)
	allMediaURLs = append(allMediaURLs, payload.VideoURLs...)
	allMediaURLs = append(allMediaURLs, payload.AudioURLs...)
	for _, mediaURL := range allMediaURLs {
		require.NotContains(t, mediaURL, "/local-media")
	}
}
