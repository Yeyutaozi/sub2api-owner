package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSeedanceVideoPricingError(t *testing.T) {
	pro720P := 0.16
	legacy720P := 0.09

	tests := []struct {
		name       string
		group      *service.Group
		model      string
		resolution string
		wantStatus int
		wantCode   string
	}{
		{
			name: "configured model and resolution",
			group: &service.Group{
				Platform: service.PlatformSeedance,
				VideoModelPrices: service.VideoModelPrices{
					"seedance-2.0": {Price720P: &pro720P},
				},
			},
			model:      " Seedance-2.0 ",
			resolution: service.VideoBillingResolution720P,
		},
		{
			name: "model is not in authoritative matrix",
			group: &service.Group{
				Platform: service.PlatformSeedance,
				VideoModelPrices: service.VideoModelPrices{
					"seedance-2.0": {Price720P: &pro720P},
				},
			},
			model:      "seedance-2.0-fast",
			resolution: service.VideoBillingResolution720P,
			wantStatus: http.StatusBadRequest,
			wantCode:   "model_not_supported",
		},
		{
			name: "model resolution is not priced",
			group: &service.Group{
				Platform: service.PlatformSeedance,
				VideoModelPrices: service.VideoModelPrices{
					"seedance-2.0": {Price720P: &pro720P},
				},
			},
			model:      "seedance-2.0",
			resolution: service.VideoBillingResolution1080P,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "billing_not_configured",
		},
		{
			name: "empty matrix retains legacy resolution pricing",
			group: &service.Group{
				Platform:       service.PlatformSeedance,
				VideoPrice720P: &legacy720P,
			},
			model:      "legacy-seedance-model",
			resolution: service.VideoBillingResolution720P,
		},
		{
			name: "empty matrix without legacy price is unconfigured",
			group: &service.Group{
				Platform: service.PlatformSeedance,
			},
			model:      "legacy-seedance-model",
			resolution: service.VideoBillingResolution720P,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "billing_not_configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, _ := seedanceVideoPricingError(tt.group, tt.model, tt.resolution)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantCode, code)
		})
	}
}
