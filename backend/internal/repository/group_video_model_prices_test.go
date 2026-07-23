package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVideoModelPricesForPersistenceIsSeedanceOnly(t *testing.T) {
	price := 0.12
	prices := service.VideoModelPrices{
		"seedance-2.0": {Price480P: &price},
	}

	seedance := &service.Group{Platform: service.PlatformSeedance, VideoModelPrices: prices}
	require.Equal(t, prices, videoModelPricesForPersistence(seedance))

	for _, platform := range []string{
		service.PlatformAnthropic,
		service.PlatformOpenAI,
		service.PlatformGemini,
		service.PlatformAntigravity,
		service.PlatformGrok,
	} {
		t.Run(platform, func(t *testing.T) {
			group := &service.Group{Platform: platform, VideoModelPrices: prices}
			require.Empty(t, videoModelPricesForPersistence(group))
			require.Equal(t, prices, group.VideoModelPrices, "isolation must not mutate the caller's group")
		})
	}
}
