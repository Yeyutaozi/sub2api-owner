package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type seedancePlatformQuotaCacheStub struct {
	BillingCache
	increments  int
	invalidates int
	limit       float64
}

func (s *seedancePlatformQuotaCacheStub) GetUserPlatformQuotaCache(
	context.Context,
	int64,
	string,
) (*UserPlatformQuotaCacheEntry, bool, error) {
	return &UserPlatformQuotaCacheEntry{DailyLimitUSD: &s.limit}, true, nil
}

func (s *seedancePlatformQuotaCacheStub) IncrUserPlatformQuotaUsageCache(
	context.Context,
	int64,
	string,
	float64,
	time.Duration,
	bool,
) error {
	s.increments++
	return nil
}

func (s *seedancePlatformQuotaCacheStub) DeleteUserPlatformQuotaCache(context.Context, int64, string) error {
	s.invalidates++
	return nil
}

type seedancePlatformQuotaRepositoryStub struct {
	UserPlatformQuotaRepository
	increments int
	userID     int64
	platform   string
	cost       float64
}

type seedanceDurableBillingRepositoryStub struct {
	UsageBillingRepository
	result *UsageBillingApplyResult
	calls  int
}

func (s *seedanceDurableBillingRepositoryStub) ApplyWithUsageLog(
	context.Context,
	*UsageBillingCommand,
	*UsageLog,
) (*UsageBillingApplyResult, error) {
	s.calls++
	return s.result, nil
}

func (s *seedancePlatformQuotaRepositoryStub) IncrementUsageWithReset(
	_ context.Context,
	userID int64,
	platform string,
	cost float64,
	_ time.Time,
) error {
	s.increments++
	s.userID = userID
	s.platform = platform
	s.cost = cost
	return nil
}

func TestFinalizePostUsagePlatformQuotaRefundableVideoBypassesSnapshotFlusher(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.UserPlatformQuotaFlusherEnabled = true
	cache := &seedancePlatformQuotaCacheStub{limit: 100}
	quotaRepo := &seedancePlatformQuotaRepositoryStub{}
	cacheService := &BillingCacheService{cache: cache, cfg: cfg}

	finalizePostUsagePlatformQuota(context.Background(), &postUsageBillingParams{
		Cost:             &CostBreakdown{ActualCost: 4.4},
		User:             &User{ID: 71},
		Platform:         PlatformSeedance,
		SynchronousCache: true,
	}, &billingDeps{
		cfg:                   cfg,
		billingCacheService:   cacheService,
		userPlatformQuotaRepo: quotaRepo,
	})

	require.Equal(t, 1, quotaRepo.increments)
	require.Equal(t, int64(71), quotaRepo.userID)
	require.Equal(t, PlatformSeedance, quotaRepo.platform)
	require.InDelta(t, 4.4, quotaRepo.cost, 1e-9)
	require.Zero(t, cache.increments, "refundable video usage must never create a dirty snapshot")
	require.Equal(t, 1, cache.invalidates)
}

func TestApplyUsageBillingDuplicateSeedanceRequestDoesNotReplayQuotaSideEffects(t *testing.T) {
	cache := &seedancePlatformQuotaCacheStub{limit: 100}
	quotaRepo := &seedancePlatformQuotaRepositoryStub{}
	billingRepo := &seedanceDurableBillingRepositoryStub{
		result: &UsageBillingApplyResult{Applied: false},
	}

	applied, err := applyUsageBilling(context.Background(), "seedance:duplicate-job", &UsageLog{
		RequestID: "seedance:duplicate-job",
		APIKeyID:  72,
	}, &postUsageBillingParams{
		Cost:             &CostBreakdown{ActualCost: 4.4},
		User:             &User{ID: 71},
		APIKey:           &APIKey{ID: 72},
		Account:          &Account{ID: 73},
		Platform:         PlatformSeedance,
		SynchronousCache: true,
		DurableUsageLog:  true,
	}, &billingDeps{
		billingCacheService:   &BillingCacheService{cache: cache},
		deferredService:       &DeferredService{},
		userPlatformQuotaRepo: quotaRepo,
	}, billingRepo)

	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, 1, billingRepo.calls)
	require.Zero(t, quotaRepo.increments)
	require.Zero(t, cache.increments)
	require.Zero(t, cache.invalidates)
}
