package service

import (
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

func TestParseGlobalAIOPCTaskResult(t *testing.T) {
	status, videoURL, err := parseGlobalAIOPCTaskResult([]byte(`{"status":"completed","result_url":"https://cdn.example.com/video.mp4"}`))
	require.NoError(t, err)
	require.Equal(t, "completed", status)
	require.Equal(t, "https://cdn.example.com/video.mp4", videoURL)
}
