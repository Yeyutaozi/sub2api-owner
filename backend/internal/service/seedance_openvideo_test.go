package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenVideoProviderAndCreateRequest(t *testing.T) {
	provider, err := normalizeVideoProvider(PlatformSeedance, VideoProviderOpenVideo)
	require.NoError(t, err)
	require.Equal(t, VideoProviderOpenVideo, provider)
	require.True(t, videoProviderSupportsModelForPlatform(PlatformSeedance, provider, SeedanceOpenVideoMiniModel))

	body, err := buildOpenVideoCreateRequest(&SeedanceRequestInfo{
		Model: SeedanceOpenVideoMiniModel, Prompt: "slow camera move", DurationSeconds: 5,
		AspectRatio: "16:9", Resolution: "720p",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/a.jpg"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/a.mp3"}},
	})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, SeedanceOpenVideoMiniModel, payload["model"])
	require.Equal(t, "video", payload["object"])
	require.Equal(t, float64(5), payload["duration"])
	require.Len(t, payload["images"], 1)
	require.Len(t, payload["audios"], 1)
}

func TestOpenVideoModelProfileRejectsVideoReferences(t *testing.T) {
	err := validateFFLinkVideoRequestInfo(&SeedanceRequestInfo{
		Model: SeedanceOpenVideoMiniModel, Prompt: "x", DurationSeconds: 5,
		Resolution: "720p", AspectRatio: "16:9",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/a.mp4"}},
	})
	require.Error(t, err)
}
