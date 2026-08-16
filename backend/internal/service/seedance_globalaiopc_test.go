package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlobalAIOPCVideoProviderRoutesOnlyDedicatedModel(t *testing.T) {
	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-key", "video_provider": VideoProviderGlobalAIOPC,
		"model_mapping": map[string]any{SeedanceGlobalAIOPCC1Model: seedanceGlobalAIOPCUpstreamModel},
	}}
	require.True(t, account.IsGlobalAIOPCVideo())
	require.True(t, account.IsModelSupported(SeedanceGlobalAIOPCC1Model))
	require.False(t, account.IsModelSupported(SeedanceXimeiSD25Model))
	require.Equal(t, DefaultGlobalAIOPCVideoBaseURL, account.GetSeedanceBaseURL())
	require.NoError(t, ValidateSeedanceAccountConfiguration(account.Platform, account.Type, account.Credentials))
}

func TestBuildGlobalAIOPCVideoCreateRequest(t *testing.T) {
	body, err := buildGlobalAIOPCVideoCreateRequest(&SeedanceRequestInfo{
		Model: SeedanceGlobalAIOPCC1Model, Prompt: "cinematic scene", DurationSeconds: 8,
		AspectRatio: "9:16", Resolution: VideoBillingResolution720P,
		StartFrameURL: "https://example.com/first.png", EndFrameURL: "https://example.com/last.png",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/ref.png"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/ref.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/ref.mp3"}},
	})
	require.NoError(t, err)
	var payload globalAIOPCVideoCreateRequest
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, seedanceGlobalAIOPCUpstreamModel, payload.Model)
	require.Equal(t, []string{"https://example.com/ref.png"}, payload.ReferenceImages)
	require.Equal(t, "https://example.com/first.png", payload.FirstImage)
	require.Equal(t, 8, payload.Duration)
}

func TestBuildGlobalAIOPCVideoCreateRequestRequiresImageWithAudio(t *testing.T) {
	_, err := buildGlobalAIOPCVideoCreateRequest(&SeedanceRequestInfo{
		Model: SeedanceGlobalAIOPCC1Model, Prompt: "test", DurationSeconds: 5,
		AspectRatio: "16:9", Resolution: VideoBillingResolution720P,
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/ref.mp3"}},
	})
	require.ErrorContains(t, err, "requires at least one reference image")
}

func TestGlobalAIOPCC1OnlySupports720P(t *testing.T) {
	profile, ok := ffLinkVideoModelProfileFor(SeedanceGlobalAIOPCC1Model)
	require.True(t, ok)
	require.Equal(t, map[string]struct{}{VideoBillingResolution720P: {}}, profile.AllowedResolutions)

	_, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"seedance-2.5-c1-03",
		"prompt":"cinematic scene",
		"resolution":"480p",
		"duration":5,
		"aspect_ratio":"16:9"
	}`))
	require.ErrorContains(t, err, "resolution 480p is not supported")
}

func TestGlobalAIOPCC130SecondUsageIsNotClampedTo15Seconds(t *testing.T) {
	price720P := 0.39
	groupID := int64(725)
	svc := &OpenAIGatewayService{billingService: NewBillingService(nil, nil)}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{
		ID: groupID, Platform: PlatformSeedance,
		VideoModelPrices: VideoModelPrices{
			SeedanceGlobalAIOPCC1Model: {Price720P: &price720P},
		},
	}}
	result := &OpenAIForwardResult{
		VideoCount: 1, VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 30,
	}

	require.Equal(t, 30, NormalizeVideoBillingDurationSecondsForModelOrDefault(SeedanceGlobalAIOPCC1Model, 30))
	cost := svc.calculateOpenAIVideoCost(
		context.Background(), SeedanceGlobalAIOPCC1Model, apiKey, result, 1,
	)
	require.InDelta(t, 11.7, cost.TotalCost, 1e-12)
	require.InDelta(t, 11.7, cost.ActualCost, 1e-12)
}

func TestParseGlobalAIOPCTaskResult(t *testing.T) {
	status, videoURL, err := parseGlobalAIOPCTaskResult([]byte(`{"status":"completed","result_url":"https://cdn.example.com/video.mp4"}`))
	require.NoError(t, err)
	require.Equal(t, "completed", status)
	require.Equal(t, "https://cdn.example.com/video.mp4", videoURL)
}
