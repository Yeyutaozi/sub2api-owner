package service

import (
	"context"
	"fmt"
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
		{model: "seedance-2.0", want: 1.6},
		{model: "seedance-2.0-fast", want: 0.8},
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
					"seedance-2.0":      {Price720P: &pro720P},
					"seedance-2.0-fast": {Price720P: &fast720P},
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

func TestRecordSeedanceUsage_PerRequestIgnoresDurationAndUsesVideoCountAndMultiplier(t *testing.T) {
	price720P := 0.16
	groupID := int64(705)
	for _, duration := range []int{5, 10, 15} {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(
			usageRepo,
			&openAIRecordUsageUserRepoStub{},
			&openAIRecordUsageSubRepoStub{},
			nil,
		)
		apiKey := &APIKey{
			ID: 1705, UserID: 2705, GroupID: &groupID, User: &User{ID: 2705},
			Group: &Group{
				ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1.5,
				VideoBillingUnit: VideoBillingUnitPerRequest,
				VideoModelPrices: VideoModelPrices{
					"seedance-2.0": {Price720P: &price720P},
				},
			},
		}

		err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
			OpenAIRecordUsageInput: OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					Model: "seedance-2.0", VideoCount: 2,
					VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: duration,
				},
				APIKey: apiKey, User: apiKey.User,
				Account: &Account{ID: 3705, Platform: PlatformSeedance},
			},
			TaskID: fmt.Sprintf("per-request-%d", duration), RequestedModel: "seedance-2.0",
		})

		require.NoError(t, err)
		require.NotNil(t, usageRepo.lastLog)
		require.InDelta(t, price720P*2, usageRepo.lastLog.TotalCost, 1e-12)
		require.InDelta(t, price720P*2*1.5, usageRepo.lastLog.ActualCost, 1e-12)
	}
}

func TestCalculateOpenAIVideoCost_UsesPerModelBillingUnitOverride(t *testing.T) {
	price := 0.16
	groupID := int64(707)
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{
		ID: groupID, Platform: PlatformSeedance, VideoBillingUnit: VideoBillingUnitPerSecond,
		VideoModelPrices: VideoModelPrices{
			"seedance-2.0":      {BillingUnit: VideoBillingUnitPerRequest, Price720P: &price},
			"seedance-2.0-fast": {Price720P: &price},
		},
	}}
	result := &OpenAIForwardResult{
		VideoCount: 1, VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 10,
	}

	perRequest := svc.calculateOpenAIVideoCost(context.Background(), "seedance-2.0", apiKey, result, 1)
	perSecond := svc.calculateOpenAIVideoCost(context.Background(), "seedance-2.0-fast", apiKey, result, 1)

	require.InDelta(t, price, perRequest.TotalCost, 1e-12)
	require.InDelta(t, price*10, perSecond.TotalCost, 1e-12)
}

func TestRecordSeedanceUsage_UserPriceKeepsPerModelBillingUnitOverride(t *testing.T) {
	groupPrice := 0.16
	userPrice := 0.05
	groupID := int64(708)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(
		usageRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		&openAIUserGroupRateRepoStub{videoPrices: VideoModelPrices{
			"seedance-2.0": {Price720P: &userPrice},
		}},
	)
	apiKey := &APIKey{
		ID: 1708, UserID: 2708, GroupID: &groupID, User: &User{ID: 2708},
		Group: &Group{
			ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1,
			VideoBillingUnit: VideoBillingUnitPerSecond,
			VideoModelPrices: VideoModelPrices{
				"seedance-2.0": {BillingUnit: VideoBillingUnitPerRequest, Price720P: &groupPrice},
			},
		},
	}

	err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
		OpenAIRecordUsageInput: OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				Model: "seedance-2.0", VideoCount: 1,
				VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 15,
			},
			APIKey: apiKey, User: apiKey.User,
			Account: &Account{ID: 3708, Platform: PlatformSeedance},
		},
		TaskID: "per-model-unit-user-price", RequestedModel: "seedance-2.0",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, userPrice, usageRepo.lastLog.TotalCost, 1e-12)
}

func TestRecordSeedanceUsage_PerRequestUsesUserResolutionOverride(t *testing.T) {
	group720P := 0.16
	user720P := 0.05
	groupID := int64(706)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(
		usageRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		&openAIUserGroupRateRepoStub{videoPrices: VideoModelPrices{
			"seedance-2.0": {Price720P: &user720P},
		}},
	)
	apiKey := &APIKey{
		ID: 1706, UserID: 2706, GroupID: &groupID, User: &User{ID: 2706},
		Group: &Group{
			ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1,
			VideoBillingUnit: VideoBillingUnitPerRequest,
			VideoModelPrices: VideoModelPrices{
				"seedance-2.0": {Price720P: &group720P},
			},
		},
	}

	err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
		OpenAIRecordUsageInput: OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				Model: "seedance-2.0", VideoCount: 1,
				VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 15,
			},
			APIKey: apiKey, User: apiKey.User,
			Account: &Account{ID: 3706, Platform: PlatformSeedance},
		},
		TaskID: "per-request-user-price", RequestedModel: "seedance-2.0",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, user720P, usageRepo.lastLog.TotalCost, 1e-12)
}

func TestRecordSeedanceUsage_UserVideoPriceOverridesGroupPricePerResolution(t *testing.T) {
	group720P := 0.16
	group1080P := 0.24
	user720P := 0.05
	groupID := int64(704)
	rateRepo := &openAIUserGroupRateRepoStub{
		videoPrices: VideoModelPrices{
			"seedance-2.0": {Price720P: &user720P},
		},
	}
	apiKey := &APIKey{
		ID: 1704, UserID: 2704, GroupID: &groupID, User: &User{ID: 2704},
		Group: &Group{
			ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1,
			VideoModelPrices: VideoModelPrices{
				"seedance-2.0": {Price720P: &group720P, Price1080P: &group1080P},
			},
		},
	}

	for _, test := range []struct {
		name       string
		resolution string
		want       float64
	}{
		{name: "overrides 720p", resolution: VideoBillingResolution720P, want: user720P * 10},
		{name: "inherits 1080p", resolution: VideoBillingResolution1080P, want: group1080P * 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(
				usageRepo,
				&openAIRecordUsageUserRepoStub{},
				&openAIRecordUsageSubRepoStub{},
				rateRepo,
			)
			err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
				OpenAIRecordUsageInput: OpenAIRecordUsageInput{
					Result: &OpenAIForwardResult{
						Model: "seedance-2.0", VideoCount: 1,
						VideoResolution: test.resolution, VideoDurationSeconds: 10,
					},
					APIKey: apiKey, User: apiKey.User,
					Account: &Account{ID: 3704, Platform: PlatformSeedance},
				},
				TaskID: "user-price-" + test.resolution, RequestedModel: "seedance-2.0",
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, test.want, usageRepo.lastLog.TotalCost, 1e-12)
		})
	}

	require.GreaterOrEqual(t, rateRepo.videoPriceCalls, 2)
	require.InDelta(t, group720P, *apiKey.Group.VideoModelPrices["seedance-2.0"].Price720P, 1e-12)
}

func TestRecordSeedanceUsage_UserPriceInheritsLegacyGroupResolutionPrices(t *testing.T) {
	group720P := 0.16
	group1080P := 0.24
	user720P := 0.05
	groupID := int64(709)
	rateRepo := &openAIUserGroupRateRepoStub{videoPrices: VideoModelPrices{
		"seedance-2.0": {Price720P: &user720P},
	}}
	apiKey := &APIKey{
		ID: 1709, UserID: 2709, GroupID: &groupID, User: &User{ID: 2709},
		Group: &Group{
			ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1,
			VideoBillingUnit: VideoBillingUnitPerRequest,
			VideoPrice720P:   &group720P,
			VideoPrice1080P:  &group1080P,
		},
	}

	for _, test := range []struct {
		resolution string
		want       float64
	}{
		{resolution: VideoBillingResolution720P, want: user720P},
		{resolution: VideoBillingResolution1080P, want: group1080P},
	} {
		t.Run(test.resolution, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(
				usageRepo,
				&openAIRecordUsageUserRepoStub{},
				&openAIRecordUsageSubRepoStub{},
				rateRepo,
			)
			err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
				OpenAIRecordUsageInput: OpenAIRecordUsageInput{
					Result: &OpenAIForwardResult{
						Model: "seedance-2.0", VideoCount: 1,
						VideoResolution: test.resolution, VideoDurationSeconds: 15,
					},
					APIKey: apiKey, User: apiKey.User,
					Account: &Account{ID: 3709, Platform: PlatformSeedance},
				},
				TaskID: "legacy-user-price-" + test.resolution, RequestedModel: "seedance-2.0",
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, test.want, usageRepo.lastLog.TotalCost, 1e-12)
		})
	}

	require.Empty(t, apiKey.Group.VideoModelPrices, "settlement must not mutate the API key group")
}

func TestRecordSeedanceUsage_UserPriceInheritsLegacyHighResolutionPrice(t *testing.T) {
	legacyPrice := 0.03
	user1080P := 0.01
	groupID := int64(710)
	rateRepo := &openAIUserGroupRateRepoStub{videoPrices: VideoModelPrices{
		"ltx-2.3-pro": {Price1080P: &user1080P},
	}}
	apiKey := &APIKey{
		ID: 1710, UserID: 2710, GroupID: &groupID, User: &User{ID: 2710},
		Group: &Group{
			ID: groupID, Platform: PlatformLTX, RateMultiplier: 1,
			VideoPrice480P: &legacyPrice,
		},
	}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(
		usageRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		rateRepo,
	)

	err := svc.RecordSeedanceUsage(context.Background(), &SeedanceRecordUsageInput{
		OpenAIRecordUsageInput: OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				Model: "ltx-2.3-pro", VideoCount: 1,
				VideoResolution: VideoBillingResolution2160P, VideoDurationSeconds: 6,
			},
			APIKey: apiKey, User: apiKey.User,
			Account: &Account{ID: 3710, Platform: PlatformLTX},
		},
		TaskID: "legacy-ltx-user-price-2160p", RequestedModel: "ltx-2.3-pro",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, legacyPrice*6, usageRepo.lastLog.TotalCost, 1e-12)
	require.Empty(t, apiKey.Group.VideoModelPrices, "settlement must not mutate the API key group")
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
				Model:                "seedance-2.0",
				BillingModel:         "seedance-2.0",
				UpstreamModel:        "seedance-2.0",
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
				ChannelMappedModel: "seedance-2.0",
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
			ID:               groupID,
			Platform:         PlatformGrok,
			VideoPrice720P:   &groupPrice720P,
			VideoBillingUnit: VideoBillingUnitPerRequest,
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
			"seedance-2.0-fast": {Price480P: &free},
		},
	}))
}
