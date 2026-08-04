//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryClaimSeedanceTaskSettlementsUsesValidPostgresReturningList(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name:           fmt.Sprintf("seedance-settlement-claim-%d", suffix),
		Platform:       service.PlatformSeedance,
		RateMultiplier: 1,
		IsExclusive:    true,
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email:       fmt.Sprintf("seedance-settlement-claim-%d@example.com", suffix),
		Concurrency: 1,
	})
	groupID := group.ID
	apiKey := &service.APIKey{
		UserID:  user.ID,
		GroupID: &groupID,
		Key:     fmt.Sprintf("sk-seedance-settlement-claim-%d", suffix),
		Name:    "seedance settlement claim",
		Status:  service.StatusActive,
	}
	require.NoError(t, NewAPIKeyRepository(integrationEntClient, integrationDB).Create(ctx, apiKey))

	jobID := fmt.Sprintf("vidjob_claim_%d", suffix)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM fflink_video_job_bindings WHERE job_id = $1", jobID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
	})

	repo := newUsageLogRepositoryWithSQL(nil, integrationDB)
	require.NoError(t, repo.SaveSeedanceTaskBinding(ctx, &service.SeedanceTaskBinding{
		UserID: user.ID, APIKeyID: apiKey.ID, GroupID: group.ID, AccountID: 900001,
		JobID: jobID, Model: "sd2-mx933-720-1s",
	}))
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET next_poll_at = NOW() - INTERVAL '1 second'
		WHERE job_id = $1
	`, jobID)
	require.NoError(t, err)

	claimed, err := repo.ClaimSeedanceTaskSettlements(ctx, "postgres-claim-test", 100, 3*time.Minute)
	require.NoError(t, err)
	var claimedBinding *service.SeedanceTaskBinding
	for index := range claimed {
		if claimed[index].JobID == jobID {
			claimedBinding = &claimed[index]
			break
		}
	}
	require.NotNil(t, claimedBinding)
	require.Equal(t, "postgres-claim-test", claimedBinding.SettlementClaimedBy)
	require.False(t, claimedBinding.SettlementClaimedAt.IsZero())

	claimedAgain, err := repo.ClaimSeedanceTaskSettlements(ctx, "postgres-claim-test-2", 100, 3*time.Minute)
	require.NoError(t, err)
	for _, binding := range claimedAgain {
		require.NotEqual(t, jobID, binding.JobID, "an active settlement lease must not be stolen")
	}
}
