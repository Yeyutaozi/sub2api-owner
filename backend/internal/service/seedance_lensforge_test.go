package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLensForgeProviderAndModel(t *testing.T) {
	provider, err := normalizeVideoProvider(PlatformSeedance, VideoProviderLensForge)
	require.NoError(t, err)
	require.Equal(t, VideoProviderLensForge, provider)
	require.True(t, videoProviderSupportsModelForPlatform(PlatformSeedance, provider, SeedanceLensForge933Model))
	require.False(t, videoProviderSupportsModelForPlatform(PlatformSeedance, provider, "sd2-mx933"))
}

func TestBuildLensForgeCreateRequest(t *testing.T) {
	body, err := buildLensForgeCreateRequest(&SeedanceRequestInfo{
		Model: SeedanceLensForge933Model, Prompt: "slow push in", Resolution: "720p",
		DurationSeconds: 5, GenerateAudio: true, StartFrameURL: "https://example.com/frame.jpg",
	})
	require.NoError(t, err)
	var request lensForgeCreateRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Equal(t, lensForge933OfferingID, request.Model)
	require.Equal(t, "5", request.Seconds)
	require.Equal(t, "1280x720", request.Size)
	require.Equal(t, "https://example.com/frame.jpg", request.InputReference)
}
