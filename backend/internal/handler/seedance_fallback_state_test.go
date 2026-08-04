package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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

func TestFilterSeedanceListedJobsAppliesStatusAfterFallbackTransition(t *testing.T) {
	jobs := []map[string]any{
		{"job_id": "CaseSensitive_A", "status": "queued"},
		{"job_id": "CaseSensitive_B", "status": "failed"},
		{"job_id": "CaseSensitive_C", "status": "completed"},
	}

	failed := filterSeedanceListedJobs(jobs, "FAILED", 10)
	require.Len(t, failed, 1)
	require.Equal(t, "CaseSensitive_B", seedanceListedJobString(failed[0], "job_id"))

	limited := filterSeedanceListedJobs(jobs, "", 2)
	require.Len(t, limited, 2)
	require.Equal(t, "CaseSensitive_A", seedanceListedJobString(limited[0], "job_id"))
}

func TestSeedanceListFallbackResumeCandidateRequiresExpiredLease(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	binding := &service.SeedanceTaskBinding{
		FallbackStatus:     service.SeedanceFallbackStatusStarting,
		FallbackLeaseUntil: now.Add(-time.Second),
	}
	require.True(t, seedanceShouldResumeExpiredFallback(binding, http.MethodGet, false, now))

	binding.FallbackLeaseUntil = now.Add(time.Minute)
	require.False(t, seedanceShouldResumeExpiredFallback(binding, http.MethodGet, false, now))
}

func TestSeedanceQueuedStateHidesLegacyMX933Model(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeSeedanceQueuedFallback(ctx, "hqv1_legacy_task", service.SeedanceMX933LegacyModel, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"job_id":"hqv1_legacy_task",
		"status":"queued",
		"status_url":"/v1/videos/jobs/hqv1_legacy_task",
		"model":"sd2-mx933"
	}`, recorder.Body.String())
}
