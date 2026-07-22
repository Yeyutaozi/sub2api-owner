//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func videoModelPriceTestPointer(value float64) *float64 {
	return &value
}

func TestNormalizeVideoModelPricesSeedance(t *testing.T) {
	free := 0.0
	pro720P := 0.16
	fast480P := 0.06
	input := VideoModelPrices{
		" Doubao-Seedance-2-0-Pro ": {
			Price480P: &free,
			Price720P: &pro720P,
		},
		"DOUBAO-SEEDANCE-2-0-FAST": {
			Price480P: &fast480P,
		},
	}

	normalized, err := normalizeVideoModelPrices(PlatformSeedance, input)

	require.NoError(t, err)
	require.Len(t, normalized, 2)
	require.Contains(t, normalized, "doubao-seedance-2-0-pro")
	require.Contains(t, normalized, "doubao-seedance-2-0-fast")
	require.Zero(t, *normalized["doubao-seedance-2-0-pro"].Price480P)
	require.InDelta(t, 0.16, *normalized["doubao-seedance-2-0-pro"].Price720P, 1e-12)
	require.Nil(t, normalized["doubao-seedance-2-0-pro"].Price1080P)
	require.NotSame(t, input[" Doubao-Seedance-2-0-Pro "].Price480P, normalized["doubao-seedance-2-0-pro"].Price480P)

	*normalized["doubao-seedance-2-0-pro"].Price480P = 1
	require.Zero(t, *input[" Doubao-Seedance-2-0-Pro "].Price480P)
}

func TestNormalizeVideoModelPricesRejectsInvalidSeedanceCards(t *testing.T) {
	tests := []struct {
		name      string
		prices    VideoModelPrices
		errSubstr string
	}{
		{
			name: "blank model",
			prices: VideoModelPrices{
				"   ": {Price480P: videoModelPriceTestPointer(0.1)},
			},
			errSubstr: "video model name is required",
		},
		{
			name: "empty price card",
			prices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {},
			},
			errSubstr: "must configure at least one resolution price",
		},
		{
			name: "negative price",
			prices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price480P: videoModelPriceTestPointer(-0.1)},
			},
			errSubstr: "price must be a finite number >= 0",
		},
		{
			name: "NaN price",
			prices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price720P: videoModelPriceTestPointer(math.NaN())},
			},
			errSubstr: "price must be a finite number >= 0",
		},
		{
			name: "infinite price",
			prices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price1080P: videoModelPriceTestPointer(math.Inf(1))},
			},
			errSubstr: "price must be a finite number >= 0",
		},
		{
			name: "normalized duplicate model",
			prices: VideoModelPrices{
				"Doubao-Seedance-2-0-Pro":   {Price480P: videoModelPriceTestPointer(0.1)},
				" doubao-seedance-2-0-pro ": {Price720P: videoModelPriceTestPointer(0.2)},
			},
			errSubstr: "duplicate video model pricing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := normalizeVideoModelPrices(PlatformSeedance, test.prices)
			require.ErrorContains(t, err, test.errSubstr)
			require.Nil(t, normalized)
		})
	}
}

func TestNormalizeVideoModelPricesClearsEveryNonSeedancePlatform(t *testing.T) {
	dirtyPrices := VideoModelPrices{
		"unexpected": {Price480P: videoModelPriceTestPointer(-1)},
	}
	platforms := []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
		"unknown",
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			normalized, err := normalizeVideoModelPrices(platform, dirtyPrices)
			require.NoError(t, err)
			require.NotNil(t, normalized)
			require.Empty(t, normalized)
		})
	}
}

func TestGroupGetVideoPriceForModelUsesSeedanceMatrixAndLegacyFallback(t *testing.T) {
	legacy480P := 0.2
	group := &Group{
		Platform:       PlatformSeedance,
		VideoPrice480P: &legacy480P,
	}

	require.Same(t, group.VideoPrice480P, group.GetVideoPriceForModel("unlisted", "480p"))

	free := 0.0
	pro720P := 0.16
	group.VideoModelPrices = VideoModelPrices{
		"doubao-seedance-2-0-pro": {
			Price480P: &free,
			Price720P: &pro720P,
		},
	}

	require.Zero(t, *group.GetVideoPriceForModel(" DOUBAO-SEEDANCE-2-0-PRO ", "480p"))
	require.InDelta(t, 0.16, *group.GetVideoPriceForModel("doubao-seedance-2-0-pro", "720p"), 1e-12)
	require.Nil(t, group.GetVideoPriceForModel("doubao-seedance-2-0-pro", "1080p"))
	require.Nil(t, group.GetVideoPriceForModel("doubao-seedance-2-0-fast", "480p"))
}

func TestGroupGetVideoPriceForModelKeepsGrokGroupWidePricing(t *testing.T) {
	legacy480P := 0.08
	dirtyMatrixPrice := 9.0
	group := &Group{
		Platform:       PlatformGrok,
		VideoPrice480P: &legacy480P,
		VideoModelPrices: VideoModelPrices{
			"doubao-seedance-2-0-pro": {Price480P: &dirtyMatrixPrice},
		},
	}

	require.Same(t, group.VideoPrice480P, group.GetVideoPriceForModel("doubao-seedance-2-0-pro", "480p"))
	require.InDelta(t, 0.08, *group.GetVideoPriceForModel("any-grok-model", "480p"), 1e-12)
}
