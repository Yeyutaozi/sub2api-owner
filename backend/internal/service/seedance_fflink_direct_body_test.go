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

func TestForwardSeedanceMappedDirectModelsUseFlatReferenceFields(t *testing.T) {
	tests := []struct {
		publicModel   string
		upstreamModel string
		resolution    string
	}{
		{publicModel: "seedance-2.0", upstreamModel: SeedanceFFLinkSD20480PModel, resolution: VideoBillingResolution480P},
		{publicModel: "seedance-2.0-mini", upstreamModel: SeedanceFFLinkSD20Mini720PModel, resolution: VideoBillingResolution720P},
	}

	for _, tt := range tests {
		t.Run(tt.upstreamModel, func(t *testing.T) {
			upstream := &seedanceHTTPUpstreamStub{body: `{"job_id":"vidjob_flat","status":"pending"}`}
			service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"base_url":       "https://video.example.test",
					"api_key":        "secret",
					"video_provider": VideoProviderFFLink,
					"model_mapping":  map[string]any{tt.publicModel: tt.upstreamModel},
				},
			}
			info := &SeedanceRequestInfo{
				Model: tt.publicModel, Prompt: "keep every reference", Resolution: tt.resolution,
				DurationSeconds: 9, AspectRatio: "16:9", GenerateAudio: true,
				StartFrameURL: "https://media.example.test/start.png",
				EndFrameURL:   "https://media.example.test/end.png",
				References: []SeedanceReferenceImage{
					{URL: "https://media.example.test/ref-1.png", Strength: "MID"},
					{URL: "https://media.example.test/ref-2.png", Strength: "HIGH"},
				},
				VideoReferences: []SeedanceReferenceVideo{
					{URL: "https://media.example.test/ref-1.mp4", DurationSeconds: 4.25},
					{URL: "https://media.example.test/ref-2.mp4", DurationSeconds: 7.5},
				},
				AudioReferences: []SeedanceReferenceAudio{
					{URL: "https://media.example.test/ref-1.wav", DurationSeconds: 3.75},
				},
			}

			require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
			response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", info)
			require.NoError(t, err)
			require.Equal(t, tt.upstreamModel, response.Result.UpstreamModel)

			forwardedBody, err := io.ReadAll(upstream.request.Body)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(forwardedBody, &payload))
			require.Equal(t, tt.upstreamModel, payload["model"])
			require.Equal(t, "https://media.example.test/start.png", payload["start_frame_url"])
			require.Equal(t, "https://media.example.test/end.png", payload["end_frame_url"])
			require.Equal(t, []any{
				"https://media.example.test/ref-1.png",
				"https://media.example.test/ref-2.png",
			}, payload["reference_images"])
			require.Equal(t, []any{
				"https://media.example.test/ref-1.mp4",
				"https://media.example.test/ref-2.mp4",
			}, payload["reference_videos"])
			require.Equal(t, []any{"https://media.example.test/ref-1.wav"}, payload["reference_audios"])
			require.NotContains(t, payload, "guidances")
			require.NotContains(t, payload, "image_url")
			require.NotContains(t, string(forwardedBody), "duration_seconds")
		})
	}
}

func TestFFLinkDirectModelStartFrameDoesNotUseLegacyImageURL(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model: SeedanceFFLinkSD20480PModel, Prompt: "start frame only",
		Resolution: VideoBillingResolution480P, DurationSeconds: 5, AspectRatio: "16:9",
		StartFrameURL: "https://media.example.test/start.png",
	}

	body, err := info.UpstreamBody(strings.ToUpper(SeedanceFFLinkSD20480PModel))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "https://media.example.test/start.png", payload["start_frame_url"])
	require.NotContains(t, payload, "image_url")
	require.NotContains(t, payload, "guidances")
}
