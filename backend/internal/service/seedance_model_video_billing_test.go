package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRecordSeedanceUsage_UsesRequestedModelPriceMatrix(t *testing.T) {
	pro720P := 0.16
	fast720P := 0.08
	groupID := int64(701)
	for _, tc := range []struct {
		model string
		want  float64
	}{
		{model: "doubao-seedance-2-0-pro", want: 1.6},
		{model: "doubao-seedance-2-0-fast", want: 0.8},
	} {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(
			usageRepo,
			&openAIRecordUsageUserRepoStub{},
			&openAIRecordUsageSubRepoStub{},
			nil,
		)
		apiKey := &APIKey{
			ID:      1701,
			UserID:  2701,
			GroupID: &groupID,
			User:    &User{ID: 2701},
			Group: &Group{
				ID:             groupID,
				Platform:       PlatformSeedance,
				RateMultiplier: 1,
				VideoModelPrices: VideoModelPrices{
					"doubao-seedance-2-0-pro":  {Price720P: &pro720P},
					"doubao-seedance-2-0-fast": {Price720P: &fast720P},
				},
			},
		}
		err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
			OpenAIRecordUsageInput: OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					Model:                "mapped-model-must-not-select-the-price",
					VideoCount:           1,
					VideoResolution:      VideoBillingResolution720P,
					VideoDurationSeconds: 10,
				},
				APIKey:  apiKey,
				User:    apiKey.User,
				Account: &Account{ID: 3701, Platform: PlatformSeedance},
			},
			TaskID:         "matrix-" + tc.model,
			RequestedModel: tc.model,
		})
		require.NoError(t, err)
		require.NotNil(t, usageRepo.lastLog)
		require.InDelta(t, tc.want, usageRepo.lastLog.TotalCost, 1e-12)
		require.Equal(t, string(BillingModeVideo), *usageRepo.lastLog.BillingMode)
	}
}

func TestOpenAIGatewayServiceRecordSeedanceUsage_UsesInboundRequestedModel(t *testing.T) {
	pro720P := 0.16
	groupID := int64(703)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(
		usageRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	apiKey := &APIKey{
		ID:      1703,
		UserID:  2703,
		GroupID: &groupID,
		User:    &User{ID: 2703},
		Group: &Group{
			ID:             groupID,
			Platform:       PlatformSeedance,
			RateMultiplier: 1,
			VideoModelPrices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price720P: &pro720P},
			},
		},
	}

	err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
		OpenAIRecordUsageInput: OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID:            "seedance-requested-model-billing",
				Model:                "seedance-2.0-pro",
				BillingModel:         "seedance-2.0-pro",
				UpstreamModel:        "seedance-2.0-pro",
				VideoCount:           1,
				VideoResolution:      VideoBillingResolution720P,
				VideoDurationSeconds: 10,
			},
			APIKey: apiKey,
			User:   apiKey.User,
			Account: &Account{
				ID:       3703,
				Platform: PlatformSeedance,
			},
			ChannelUsageFields: ChannelUsageFields{
				OriginalModel:      "doubao-seedance-2-0-pro",
				ChannelMappedModel: "seedance-2.0-pro",
			},
		},
		TaskID:         "seedance-requested-model-billing",
		RequestedModel: "doubao-seedance-2-0-pro",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "doubao-seedance-2-0-pro", usageRepo.lastLog.RequestedModel)
	require.InDelta(t, 1.6, usageRepo.lastLog.TotalCost, 1e-12)
}

func TestCalculateOpenAIVideoCost_GrokIgnoresSeedanceModelPriceMatrix(t *testing.T) {
	groupPrice720P := 0.037
	dirtyMatrixPrice720P := 0.001
	groupID := int64(702)
	apiKey := &APIKey{
		GroupID: &groupID,
		Group: &Group{
			ID:             groupID,
			Platform:       PlatformGrok,
			VideoPrice720P: &groupPrice720P,
			VideoModelPrices: VideoModelPrices{
				"grok-imagine-video": {Price720P: &dirtyMatrixPrice720P},
			},
		},
	}
	result := &OpenAIForwardResult{
		Model:                "grok-imagine-video",
		VideoCount:           1,
		VideoResolution:      VideoBillingResolution720P,
		VideoDurationSeconds: 5,
	}
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}

	cost := svc.calculateOpenAIVideoCost(
		context.Background(),
		"grok-imagine-video",
		apiKey,
		result,
		1,
	)

	require.InDelta(t, groupPrice720P*5, cost.TotalCost, 1e-12)
}

func TestGroupMediaPricingLooksIncomplete_IgnoresSeedanceOnlyMatrix(t *testing.T) {
	free := 0.0
	require.True(t, groupMediaPricingLooksIncomplete(&Group{
		Platform: PlatformSeedance,
		VideoModelPrices: VideoModelPrices{
			"doubao-seedance-2-0-fast": {Price480P: &free},
		},
	}))
}
