package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSeedanceShouldResumeExpiredFallbackOnlyForStatusPoll(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	binding := &service.SeedanceTaskBinding{
		FallbackStatus:     service.SeedanceFallbackStatusStarting,
		FallbackLeaseUntil: now.Add(-time.Second),
	}
	require.True(t, seedanceShouldResumeExpiredFallback(binding, http.MethodGet, false, now))
	require.False(t, seedanceShouldResumeExpiredFallback(binding, http.MethodGet, true, now))
	require.False(t, seedanceShouldResumeExpiredFallback(binding, http.MethodDelete, false, now))

	binding.FallbackLeaseUntil = now.Add(time.Minute)
	require.False(t, seedanceShouldResumeExpiredFallback(binding, http.MethodGet, false, now))
}

func TestSeedanceShouldClaimCancellationExcludesActiveFallbackAndSerializesExpiredDelete(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	ready := &service.SeedanceTaskBinding{FallbackStatus: service.SeedanceFallbackStatusReady}
	require.True(t, seedanceShouldClaimCancellation(ready, http.MethodDelete, now))

	active := &service.SeedanceTaskBinding{FallbackStatus: service.SeedanceFallbackStatusActive}
	require.False(t, seedanceShouldClaimCancellation(active, http.MethodDelete, now))

	starting := &service.SeedanceTaskBinding{FallbackStatus: service.SeedanceFallbackStatusStarting}
	require.False(t, seedanceShouldClaimCancellation(starting, http.MethodDelete, now))

	cancelling := &service.SeedanceTaskBinding{
		FallbackStatus:     service.SeedanceFallbackStatusCancelling,
		FallbackLeaseUntil: now.Add(-time.Second),
	}
	require.True(t, seedanceShouldClaimCancellation(cancelling, http.MethodDelete, now))
	cancelling.FallbackLeaseUntil = now.Add(time.Minute)
	require.False(t, seedanceShouldClaimCancellation(cancelling, http.MethodDelete, now))
}

func TestSeedanceRecoveryPreservesUncertainFallbackClaim(t *testing.T) {
	require.True(t, seedanceShouldFailFallbackClaim(false, false, false))
	require.False(t, seedanceShouldFailFallbackClaim(false, false, true))
	require.False(t, seedanceShouldFailFallbackClaim(false, true, false))
	require.False(t, seedanceShouldFailFallbackClaim(true, false, false))
}
