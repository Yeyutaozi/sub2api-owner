package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFFLinkSeedance20ExactModelsValidateConfirmedContract(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
	}{
		{model: SeedanceFFLinkSD20480PModel, resolution: VideoBillingResolution480P},
		{model: SeedanceFFLinkSD20Mini720PModel, resolution: VideoBillingResolution720P},
	}

	models := FFLinkVideoModelIDsForPlatform(PlatformSeedance)
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.Contains(t, models, tt.model)
			require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, tt.model))

			profile, ok := ffLinkVideoModelProfileFor(tt.model)
			require.True(t, ok)
			require.False(t, profile.RequireStartFrame)
			require.True(t, profile.AllowGeneratedAudio)
			require.Equal(t, 9, profile.MaxImageReferences)
			require.Equal(t, 9, profile.MaxTotalImages)
			require.Equal(t, 3, profile.MaxVideoReferences)
			require.Equal(t, 3, profile.MaxAudioReferences)
			require.Equal(t, 15, profile.MaxTotalMedia)

			for duration := 4; duration <= 15; duration++ {
				for _, ratio := range []string{"16:9", "9:16", "1:1", "4:3", "3:4"} {
					info := &SeedanceRequestInfo{
						Model: tt.model, Prompt: "video", Resolution: tt.resolution,
						DurationSeconds: duration, AspectRatio: ratio, GenerateAudio: true,
					}
					require.NoError(t, validateFFLinkVideoRequestInfo(info), "duration=%d ratio=%s", duration, ratio)
				}
			}

			for _, duration := range []int{3, 16} {
				info := &SeedanceRequestInfo{Model: tt.model, Prompt: "video", Resolution: tt.resolution, DurationSeconds: duration, AspectRatio: "16:9"}
				require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), "duration")
			}

			wrongResolution := VideoBillingResolution720P
			if tt.resolution == VideoBillingResolution720P {
				wrongResolution = VideoBillingResolution480P
			}
			info := &SeedanceRequestInfo{Model: tt.model, Prompt: "video", Resolution: wrongResolution, DurationSeconds: 4, AspectRatio: "16:9"}
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), "resolution")

			info = &SeedanceRequestInfo{Model: tt.model, Prompt: "video", Resolution: tt.resolution, DurationSeconds: 4, AspectRatio: "21:9"}
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), "aspect_ratio")

			info = &SeedanceRequestInfo{Model: tt.model, Prompt: strings.Repeat("a", 5001), Resolution: tt.resolution, DurationSeconds: 4, AspectRatio: "16:9"}
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), "5000 character limit")

			fullMedia := &SeedanceRequestInfo{
				Model: tt.model, Prompt: "video", Resolution: tt.resolution,
				DurationSeconds: 15, AspectRatio: "16:9", GenerateAudio: true,
				References:      make([]SeedanceReferenceImage, 9),
				VideoReferences: make([]SeedanceReferenceVideo, 3),
				AudioReferences: make([]SeedanceReferenceAudio, 3),
			}
			require.NoError(t, validateFFLinkVideoRequestInfo(fullMedia))
			tooManyMedia := *fullMedia
			tooManyMedia.AudioReferences = make([]SeedanceReferenceAudio, 4)
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyMedia), "at most 3 reference audio files")
		})
	}
}

func TestFFLinkSeedance20ExactModelsAllowIdentityAndEmptyMappings(t *testing.T) {
	for _, model := range []string{SeedanceFFLinkSD20480PModel, SeedanceFFLinkSD20Mini720PModel} {
		t.Run(model, func(t *testing.T) {
			for _, mapping := range []map[string]any{nil, {model: model}} {
				credentials := map[string]any{
					"api_key": "secret", "base_url": "https://video.example.test", "video_provider": VideoProviderFFLink,
				}
				if mapping != nil {
					credentials["model_mapping"] = mapping
				}
				require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, credentials))
				account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: credentials}
				require.True(t, account.IsModelSupported(model))
				resolution := VideoBillingResolution480P
				if model == SeedanceFFLinkSD20Mini720PModel {
					resolution = VideoBillingResolution720P
				}
				info := &SeedanceRequestInfo{Model: model, Prompt: "video", Resolution: resolution, DurationSeconds: 9, AspectRatio: "4:3", GenerateAudio: true}
				require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
			}
		})
	}
}

func TestFFLinkSeedance20ExactModelUsesCustomBaseURLTransparently(t *testing.T) {
	upstream := &seedanceHTTPUpstreamStub{body: `{"job_id":"vidjob_exact","status":"pending"}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://custom-video.example.test/api", "api_key": "secret",
			"model_mapping": map[string]any{SeedanceFFLinkSD20480PModel: SeedanceFFLinkSD20480PModel},
		},
	}
	info := &SeedanceRequestInfo{
		Model: SeedanceFFLinkSD20480PModel, Prompt: "video", Resolution: VideoBillingResolution480P,
		DurationSeconds: 9, AspectRatio: "3:4", GenerateAudio: true,
	}

	response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", info)
	require.NoError(t, err)
	require.Equal(t, "vidjob_exact", response.Result.ResponseID)
	require.Equal(t, SeedanceFFLinkSD20480PModel, response.Result.UpstreamModel)
	require.Equal(t, "https://custom-video.example.test/api/v1/videos/generations", upstream.request.URL.String())
	body, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, SeedanceFFLinkSD20480PModel, payload["model"])
	require.Equal(t, float64(9), payload["duration"])
	require.Equal(t, VideoBillingResolution480P, payload["resolution"])
	require.Equal(t, true, payload["audio"])

	upstream.body = `{"job_id":"vidjob_exact","status":"pending"}`
	_, err = service.ForwardSeedance(context.Background(), nil, account, http.MethodGet, "vidjob_exact", nil)
	require.NoError(t, err)
	require.Equal(t, "https://custom-video.example.test/api/v1/videos/jobs/vidjob_exact", upstream.request.URL.String())
}
