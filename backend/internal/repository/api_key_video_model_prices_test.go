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
		VideoBillingUnit: service.VideoBillingUnitPerRequest,
	})
	require.Equal(t, prices, seedance.VideoModelPrices)
	require.Equal(t, service.VideoBillingUnitPerRequest, seedance.VideoBillingUnit)

	grok := groupEntityToService(&dbent.Group{
		Platform:         service.PlatformGrok,
		VideoModelPrices: prices,
		VideoBillingUnit: service.VideoBillingUnitPerRequest,
	})
	require.Empty(t, grok.VideoModelPrices)
	require.Equal(t, service.VideoBillingUnitPerSecond, grok.VideoBillingUnit)

	legacySeedance := groupEntityToService(&dbent.Group{Platform: service.PlatformSeedance})
	require.Equal(t, service.VideoBillingUnitPerSecond, legacySeedance.VideoBillingUnit)
}
