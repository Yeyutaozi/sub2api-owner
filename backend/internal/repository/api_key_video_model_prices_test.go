package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_VideoModelPricesAreSeedanceOnly(t *testing.T) {
	price720P := 0.16
	prices := service.VideoModelPrices{
		"seedance-2.0": {Price720P: &price720P},
	}

	seedance := groupEntityToService(&dbent.Group{
		Platform:         service.PlatformSeedance,
		VideoModelPrices: prices,
	})
	require.Equal(t, prices, seedance.VideoModelPrices)

	grok := groupEntityToService(&dbent.Group{
		Platform:         service.PlatformGrok,
		VideoModelPrices: prices,
	})
	require.Empty(t, grok.VideoModelPrices)
}
