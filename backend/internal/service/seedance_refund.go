package service

import (
	"context"
	"errors"
)

var ErrSeedanceUsageRefundUnavailable = errors.New("seedance usage refund repository is unavailable")

// RefundSeedanceUsage reverses the provisional charge recorded when an
// asynchronous Seedance task was accepted. The repository owns idempotency and
// all durable balance/quota mutations; this layer only invalidates caches.
func (s *OpenAIGatewayService) RefundSeedanceUsage(
	ctx context.Context,
	taskID string,
	userID int64,
	apiKeyID int64,
) (*SeedanceUsageRefundResult, error) {
	if s == nil || s.usageBillingRepo == nil {
		return nil, ErrSeedanceUsageRefundUnavailable
	}
	repo, ok := s.usageBillingRepo.(SeedanceUsageRefundRepository)
	if !ok {
		return nil, ErrSeedanceUsageRefundUnavailable
	}
	result, err := repo.RefundSeedanceUsage(ctx, taskID, userID, apiKeyID)
	if err != nil || result == nil || !result.Applied || s.billingCacheService == nil {
		return result, err
	}

	if result.BillingType == BillingTypeBalance {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, result.UserID)
	}
	if cache := s.billingCacheService.cache; cache != nil {
		if result.BillingType == BillingTypeSubscription && result.GroupID != nil {
			_ = cache.InvalidateSubscriptionCache(ctx, result.UserID, *result.GroupID)
		}
		_ = cache.InvalidateAPIKeyRateLimit(ctx, result.APIKeyID)
		if result.Platform != "" {
			_ = cache.DeleteUserPlatformQuotaCache(ctx, result.UserID, result.Platform)
		}
	}
	return result, nil
}
