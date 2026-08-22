package service

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTianyueAccountSupportsCustomPublicModelMapping(t *testing.T) {
	credentials := map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping": map[string]any{
			"my-standard-video": SeedanceTianyueSD20Model,
			"my-fast-video":     SeedanceTianyueSD20FastModel,
		},
	}
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, credentials))

	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: credentials}
	require.True(t, account.IsTianyueVideo())
	require.True(t, account.IsModelSupported("my-standard-video"))
	require.True(t, account.IsModelSupported("my-fast-video"))
	require.True(t, account.IsModelSupported("MY-STANDARD-VIDEO"))
	require.False(t, account.IsModelSupported(SeedanceTianyueSD20Model), "Tianyue requires an explicit public mapping")
	require.False(t, account.IsModelSupported("unmapped-video"))
	require.Equal(t, DefaultTianyueVideoBaseURL, account.GetSeedanceBaseURL())
}

func TestParseTianyueCanonicalModelPreservesPublicCasing(t *testing.T) {
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"b-sd2.0-f-933",
		"prompt":"cinematic portrait",
		"resolution":"720p",
		"duration":15
	}`))
	require.NoError(t, err)
	require.Equal(t, SeedanceTianyueSD20FastModel, info.Model)

	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{SeedanceTianyueSD20FastModel: SeedanceTianyueSD20FastModel},
	}}
	require.True(t, account.IsModelSupported(info.Model))
	require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
	require.Equal(t, SeedanceTianyueSD20FastModel, account.GetMappedModel(info.Model))
}

func TestTianyueAccountRejectsUnsupportedMappingTarget(t *testing.T) {
	err := ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": "other-model"},
	})
	require.ErrorContains(t, err, "must map to a supported Tianyue model")
}

func TestValidateTianyueAliasUsesMappedModelProfile(t *testing.T) {
	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": SeedanceTianyueSD20Model},
	}}
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"my-video",
		"prompt":"cinematic portrait"
	}`))
	require.NoError(t, err)
	require.Zero(t, info.DurationSeconds)

	require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
	require.Equal(t, "my-video", info.Model)
	require.Equal(t, 15, info.DurationSeconds)
	require.Equal(t, VideoBillingResolution720P, info.Resolution)
	require.Equal(t, "16:9", info.AspectRatio)
}

func TestBuildTianyueVideoCreateRequestUsesPerRequestBillingDuration(t *testing.T) {
	body, err := buildTianyueVideoCreateRequest(&SeedanceRequestInfo{
		Prompt: "cinematic portrait", DurationSeconds: 15, AspectRatio: "9:16", Resolution: VideoBillingResolution720P,
		StartFrameURL:   "https://example.com/start.jpg",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/reference.jpg"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/reference.mp3"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/reference.mp4"}},
	}, "b-sd2.0-f-933")
	require.NoError(t, err)

	var request tianyueVideoCreateRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Equal(t, SeedanceTianyueSD20FastModel, request.Model)
	require.Equal(t, 1, request.Duration, "per-request Tianyue models must always send duration=1")
	require.Equal(t, 15, request.VideoDuration)
	require.Equal(t, []string{"https://example.com/reference.jpg", "https://example.com/start.jpg"}, request.ImageURLs)
	require.Equal(t, []string{"https://example.com/reference.mp3"}, request.AudioURLs)
	require.Equal(t, []string{"https://example.com/reference.mp4"}, request.VideoURLs)
}

func TestForwardTianyueSeedanceCreatesOpaqueBoundTask(t *testing.T) {
	upstream := &seedanceHTTPUpstreamStub{body: `{"id":"task_123","task_id":"task_123","status":"queued"}`}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	gateway := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "secret",
		"base_url":       "http://tianyue.example",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": SeedanceTianyueSD20Model},
	}}
	info := &SeedanceRequestInfo{Model: "my-video", Prompt: "cinematic portrait", DurationSeconds: 15, AspectRatio: "16:9", Resolution: VideoBillingResolution720P}

	response, err := gateway.ForwardSeedance(t.Context(), nil, account, "POST", "", info)
	require.NoError(t, err)
	require.NotNil(t, response.Result)
	require.Equal(t, "task_123", response.Result.UpstreamResponseID)
	require.Equal(t, SeedanceTianyueSD20Model, response.Result.UpstreamModel)
	require.NotEqual(t, "task_123", response.Result.ResponseID)
	require.Contains(t, response.Result.ResponseID, "vidjob_")
	require.Equal(t, "Bearer secret", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "http://tianyue.example/v1/videos", upstream.request.URL.String())

	requestBody, readErr := io.ReadAll(upstream.request.Body)
	require.NoError(t, readErr)
	var request tianyueVideoCreateRequest
	require.NoError(t, json.Unmarshal(requestBody, &request))
	require.Equal(t, 1, request.Duration)
	require.Equal(t, 15, request.VideoDuration)
}

func TestParseTianyueTaskResult(t *testing.T) {
	status, resultURL, err := parseTianyueTaskResult([]byte(`{
		"status":"completed",
		"video_url":"https://cdn.example.com/video.mp4"
	}`))
	require.NoError(t, err)
	require.Equal(t, "completed", status)
	require.Equal(t, "https://cdn.example.com/video.mp4", resultURL)
}

func TestNormalizeTianyueTaskHidesUpstreamDetails(t *testing.T) {
	body, err := NormalizeSeedanceJobForRoute(
		[]byte(`{"id":"upstream-task","status":"completed","model":"B-SD2.0-933","video_url":"https://cdn.example.com/video.mp4"}`),
		"vidjob_public", VideoProviderTianyue, "my-video",
	)
	require.NoError(t, err)
	require.NotContains(t, string(body), "upstream-task")
	require.NotContains(t, string(body), "cdn.example.com")
	require.NotContains(t, string(body), SeedanceTianyueSD20Model)
	require.Contains(t, string(body), `"model":"my-video"`)
	require.Contains(t, string(body), `/v1/videos/jobs/vidjob_public/content`)
}
