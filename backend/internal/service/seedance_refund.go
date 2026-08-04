package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	if s != nil && s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return &SeedanceUsageRefundResult{NotRequired: true}, nil
	}
	if s == nil || s.usageBillingRepo == nil {
		return nil, ErrSeedanceUsageRefundUnavailable
	}
	repo, ok := s.usageBillingRepo.(SeedanceUsageRefundRepository)
	if !ok {
		return nil, ErrSeedanceUsageRefundUnavailable
	}
	result, err := repo.RefundSeedanceUsage(ctx, taskID, userID, apiKeyID)
	if err != nil || result == nil || !result.Found || result.UsageLogID <= 0 || s.billingCacheService == nil {
		return result, err
	}

	var cacheErrs []error
	if result.BillingType == BillingTypeBalance {
		if err := s.billingCacheService.InvalidateUserBalance(ctx, result.UserID); err != nil {
			cacheErrs = append(cacheErrs, fmt.Errorf("invalidate refunded user balance cache: %w", err))
		}
	}
	if result.BillingType == BillingTypeSubscription && result.GroupID != nil {
		if err := s.billingCacheService.InvalidateSubscription(ctx, result.UserID, *result.GroupID); err != nil {
			cacheErrs = append(cacheErrs, fmt.Errorf("invalidate refunded subscription cache: %w", err))
		}
	}
	if err := s.billingCacheService.InvalidateAPIKeyRateLimit(ctx, result.APIKeyID); err != nil {
		cacheErrs = append(cacheErrs, fmt.Errorf("invalidate refunded api key rate limit cache: %w", err))
	}
	if cache := s.billingCacheService.cache; cache != nil {
		if result.Platform != "" {
			if err := cache.DeleteUserPlatformQuotaCache(ctx, result.UserID, result.Platform); err != nil {
				cacheErrs = append(cacheErrs, fmt.Errorf("invalidate refunded platform quota cache: %w", err))
			}
		}
	}
	return result, errors.Join(cacheErrs...)
}
