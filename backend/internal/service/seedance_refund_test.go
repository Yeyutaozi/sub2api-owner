package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type seedanceRefundRetryRepositoryStub struct {
	UsageBillingRepository
	results []*SeedanceUsageRefundResult
	calls   int
}

func (s *seedanceRefundRetryRepositoryStub) RefundSeedanceUsage(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
) (*SeedanceUsageRefundResult, error) {
	index := s.calls
	s.calls++
	if index >= len(s.results) {
		return nil, errors.New("unexpected refund repository call")
	}
	return s.results[index], nil
}

type seedanceRefundCacheStub struct {
	BillingCache
	balanceFailures int
	balanceCalls    int
	apiKeyCalls     int
	platformCalls   int
}

func (s *seedanceRefundCacheStub) InvalidateUserBalance(context.Context, int64) error {
	s.balanceCalls++
	if s.balanceFailures > 0 {
		s.balanceFailures--
		return errors.New("balance cache unavailable")
	}
	return nil
}

func (s *seedanceRefundCacheStub) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	s.apiKeyCalls++
	return nil
}

func (s *seedanceRefundCacheStub) DeleteUserPlatformQuotaCache(context.Context, int64, string) error {
	s.platformCalls++
	return nil
}

func TestRefundSeedanceUsageRetriesCacheInvalidationAfterDurableRefund(t *testing.T) {
	first := &SeedanceUsageRefundResult{
		Found:        true,
		Applied:      true,
		UsageLogID:   91,
		UserID:       42,
		APIKeyID:     7,
		BillingType:  BillingTypeBalance,
		Platform:     PlatformSeedance,
		RefundedCost: 1.2,
	}
	second := &SeedanceUsageRefundResult{
		Found:       true,
		Applied:     false,
		UsageLogID:  91,
		UserID:      42,
		APIKeyID:    7,
		BillingType: BillingTypeBalance,
		Platform:    PlatformSeedance,
	}
	repo := &seedanceRefundRetryRepositoryStub{results: []*SeedanceUsageRefundResult{first, second}}
	cache := &seedanceRefundCacheStub{balanceFailures: 1}
	svc := &OpenAIGatewayService{
		usageBillingRepo:    repo,
		billingCacheService: &BillingCacheService{cache: cache},
	}

	result, err := svc.RefundSeedanceUsage(context.Background(), "vidjob_123", 42, 7)
	require.Same(t, first, result)
	require.ErrorContains(t, err, "balance cache unavailable")
	require.Equal(t, 1, cache.balanceCalls)
	require.Equal(t, 1, cache.apiKeyCalls)
	require.Equal(t, 1, cache.platformCalls)

	result, err = svc.RefundSeedanceUsage(context.Background(), "vidjob_123", 42, 7)
	require.NoError(t, err)
	require.Same(t, second, result)
	require.False(t, result.Applied)
	require.Equal(t, 2, repo.calls)
	require.Equal(t, 2, cache.balanceCalls)
	require.Equal(t, 2, cache.apiKeyCalls)
	require.Equal(t, 2, cache.platformCalls)
}
