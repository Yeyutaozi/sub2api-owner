package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupFromServiceVideoModelPricesAreSeedanceOnly(t *testing.T) {
	price := 0.16
	dirtyPrices := service.VideoModelPrices{
		"grok-imagine-video": {Price720P: &price},
	}

	grokJSON, err := json.Marshal(GroupFromService(&service.Group{
		Platform:         service.PlatformGrok,
		VideoModelPrices: dirtyPrices,
	}))
	require.NoError(t, err)
	require.NotContains(t, string(grokJSON), "video_model_prices")
	require.Contains(t, string(grokJSON), `"video_billing_unit":"per_second"`)

	seedanceJSON, err := json.Marshal(GroupFromService(&service.Group{
		Platform: service.PlatformSeedance,
	}))
	require.NoError(t, err)
	require.Contains(t, string(seedanceJSON), `"video_model_prices":{}`)
	require.Contains(t, string(seedanceJSON), `"video_billing_unit":"per_second"`)

	seedancePerRequestJSON, err := json.Marshal(GroupFromService(&service.Group{
		Platform:         service.PlatformSeedance,
		VideoBillingUnit: service.VideoBillingUnitPerRequest,
	}))
	require.NoError(t, err)
	require.Contains(t, string(seedancePerRequestJSON), `"video_billing_unit":"per_request"`)
}
