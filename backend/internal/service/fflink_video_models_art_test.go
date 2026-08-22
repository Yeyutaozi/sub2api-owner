//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtSD20ModelProfile(t *testing.T) {
	require.Contains(t, FFLinkVideoModelIDsForPlatform(PlatformSeedance), SeedanceArtSD20Model)
	require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, SeedanceArtSD20Model))
	require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformLTX, SeedanceArtSD20Model))

	profile, ok := ffLinkVideoModelProfileFor(SeedanceArtSD20Model)
	require.True(t, ok)
	require.Equal(t, VideoBillingResolution720P, profile.DefaultResolution)
	require.Equal(t, 10, profile.DefaultDuration)
	require.Equal(t, resolutionSet(VideoBillingResolution720P), profile.AllowedResolutions)
}

func TestArtSD20RequestValidation(t *testing.T) {
	for _, duration := range []int{10, 15} {
		info := artSD20RequestInfo(duration, VideoBillingResolution720P)
		require.NoError(t, validateFFLinkVideoRequestInfo(info), fmt.Sprintf("duration %d", duration))
	}

	for _, duration := range []int{0, 5, 9, 11, 14, 16} {
		info := artSD20RequestInfo(duration, VideoBillingResolution720P)
		if duration == 0 {
			require.NoError(t, validateFFLinkVideoRequestInfo(info))
			require.Equal(t, 10, info.DurationSeconds)
			continue
		}
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), fmt.Sprintf("duration %d is not supported", duration))
	}

	for _, resolution := range []string{VideoBillingResolution480P, VideoBillingResolution1080P} {
		info := artSD20RequestInfo(10, resolution)
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), fmt.Sprintf("resolution %s is not supported", resolution))
	}
}

func TestArtSD20VideoModelPricing(t *testing.T) {
	price720P := 0.37
	normalized, err := normalizeVideoModelPrices(PlatformSeedance, VideoModelPrices{
		SeedanceArtSD20Model: {Price720P: &price720P},
	})
	require.NoError(t, err)
	require.InDelta(t, price720P, *normalized[SeedanceArtSD20Model].Price720P, 1e-12)
	require.Nil(t, normalized[SeedanceArtSD20Model].Price480P)
	require.Nil(t, normalized[SeedanceArtSD20Model].Price1080P)

	for resolution, price := range map[string]VideoModelPrice{
		VideoBillingResolution480P:  {Price480P: &price720P},
		VideoBillingResolution1080P: {Price1080P: &price720P},
	} {
		prices, err := normalizeVideoModelPrices(PlatformSeedance, VideoModelPrices{
			SeedanceArtSD20Model: price,
		})
		require.ErrorContains(t, err, fmt.Sprintf("does not support %s pricing", resolution))
		require.Nil(t, prices)
	}
}

func artSD20RequestInfo(duration int, resolution string) *SeedanceRequestInfo {
	return &SeedanceRequestInfo{
		Model:           SeedanceArtSD20Model,
		Prompt:          "A cinematic tracking shot",
		Resolution:      resolution,
		DurationSeconds: duration,
		AspectRatio:     "16:9",
	}
}
