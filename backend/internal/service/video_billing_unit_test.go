package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculateVideoCostPerRequestIgnoresDuration(t *testing.T) {
	unitPrice := 0.24
	svc := NewBillingService(&config.Config{}, nil)
	priceConfig := &VideoPriceConfig{
		Price720P:   &unitPrice,
		BillingUnit: VideoBillingUnitPerRequest,
	}

	for _, duration := range []int{5, 10, 15} {
		cost := svc.CalculateVideoCost("seedance-2.0", VideoBillingResolution720P, 3, duration, priceConfig, 0.5)
		require.InDelta(t, unitPrice*3, cost.TotalCost, 1e-12)
		require.InDelta(t, unitPrice*3*0.5, cost.ActualCost, 1e-12)
		require.Equal(t, string(BillingModeVideo), cost.BillingMode)
	}
}

func TestCalculateVideoCostDefaultsToPerSecond(t *testing.T) {
	unitPrice := 0.24
	svc := NewBillingService(&config.Config{}, nil)

	cost := svc.CalculateVideoCost(
		"grok-imagine-video",
		VideoBillingResolution720P,
		2,
		10,
		&VideoPriceConfig{Price720P: &unitPrice},
		0.5,
	)

	require.InDelta(t, unitPrice*10*2, cost.TotalCost, 1e-12)
	require.InDelta(t, unitPrice*10*2*0.5, cost.ActualCost, 1e-12)
}

func TestEffectiveVideoBillingUnitIsSeedanceOnly(t *testing.T) {
	require.Equal(t, VideoBillingUnitPerRequest, EffectiveVideoBillingUnit(PlatformSeedance, VideoBillingUnitPerRequest))
	require.Equal(t, VideoBillingUnitPerSecond, EffectiveVideoBillingUnit(PlatformSeedance, ""))
	require.Equal(t, VideoBillingUnitPerSecond, EffectiveVideoBillingUnit(PlatformSeedance, "invalid"))

	for _, platform := range []string{PlatformGrok, PlatformLTX, PlatformHappyHorse} {
		require.Equal(t, VideoBillingUnitPerSecond, EffectiveVideoBillingUnit(platform, VideoBillingUnitPerRequest))
	}
}
