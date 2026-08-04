//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositorySeedanceTaskBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	createdAt := time.Date(2026, 8, 3, 10, 30, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO fflink_video_job_bindings").
		WithArgs(int64(1), int64(2), int64(3), int64(4), "vidjob_one", "vidjob_one", "seedance-2.0", "", "", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.SaveSeedanceTaskBinding(context.Background(), &service.SeedanceTaskBinding{
		UserID: 1, APIKeyID: 2, GroupID: 3, AccountID: 4,
		JobID: "vidjob_one", Model: "seedance-2.0",
	}))

	mock.ExpectQuery("SELECT user_id, api_key_id, group_id, account_id, job_id, upstream_job_id").
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one").
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "api_key_id", "group_id", "account_id", "job_id", "upstream_job_id", "model",
			"fallback_model", "fallback_status", "fallback_claim_token", "fallback_lease_until", "request_snapshot", "created_at", "updated_at",
		}).AddRow(1, 2, 3, 4, "vidjob_one", "vidjob_one", "seedance-2.0", "", "", "", nil, nil, createdAt, createdAt))
	binding, err := repo.GetSeedanceTaskBinding(context.Background(), 1, 2, 3, "vidjob_one")
	require.NoError(t, err)
	require.Equal(t, int64(4), binding.AccountID)
	require.Equal(t, createdAt, binding.CreatedAt)

	mock.ExpectQuery("SELECT user_id, api_key_id, group_id, account_id, job_id, upstream_job_id").
		WithArgs(int64(1), int64(2), int64(3), 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "api_key_id", "group_id", "account_id", "job_id", "upstream_job_id", "model",
			"fallback_model", "fallback_status", "fallback_claim_token", "fallback_lease_until", "request_snapshot", "created_at", "updated_at",
		}).
			AddRow(1, 2, 3, 4, "vidjob_two", "vidjob_two", "ltx-2.0", "", "", "", nil, nil, createdAt.Add(time.Minute), createdAt.Add(time.Minute)).
			AddRow(1, 2, 3, 5, "vidjob_one", "vidjob_one", "seedance-2.0", "", "", "", nil, nil, createdAt, createdAt))
	bindings, err := repo.ListSeedanceTaskBindings(context.Background(), 1, 2, 3, 20)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "vidjob_two", bindings[0].JobID)
	require.Equal(t, int64(5), bindings[1].AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositorySeedanceFallbackClaimUsesLeaseToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)

	mock.ExpectQuery(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$8 OR.*fallback_status = \$5.*fallback_lease_until < NOW\(\)`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one", service.SeedanceFallbackStatusStarting,
			sqlmock.AnyArg(), sqlmock.AnyArg(), service.SeedanceFallbackStatusReady).
		WillReturnRows(sqlmock.NewRows([]string{"fallback_claim_token"}).AddRow("claim-token"))
	claimed, token, err := repo.ClaimSeedanceTaskFallback(context.Background(), 1, 2, 3, "vidjob_one")
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "claim-token", token)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$9 AND fallback_claim_token = \$5`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one", "claim-token", int64(9), "hqv1_task", service.SeedanceFallbackStatusActive, service.SeedanceFallbackStatusStarting).
		WillReturnResult(sqlmock.NewResult(0, 1))
	activated, err := repo.ActivateSeedanceTaskFallback(context.Background(), 1, 2, 3, "vidjob_one", "claim-token", 9, "hqv1_task")
	require.NoError(t, err)
	require.True(t, activated)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$6 AND fallback_claim_token = \$7`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one", service.SeedanceFallbackStatusFailed, service.SeedanceFallbackStatusStarting, "claim-token-2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	failed, err := repo.FailSeedanceTaskFallback(context.Background(), 1, 2, 3, "vidjob_one", "claim-token-2")
	require.NoError(t, err)
	require.True(t, failed)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$6 AND fallback_claim_token = \$7`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one", service.SeedanceFallbackStatusReady, service.SeedanceFallbackStatusStarting, "claim-token-3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	released, err := repo.ReleaseSeedanceTaskFallback(context.Background(), 1, 2, 3, "vidjob_one", "claim-token-3")
	require.NoError(t, err)
	require.True(t, released)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositorySeedanceCancellationClaimUsesSameLeaseTokenProtocol(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)

	mock.ExpectQuery(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$8 OR.*fallback_status = \$5.*fallback_lease_until < NOW\(\)`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_delete", service.SeedanceFallbackStatusCancelling,
			sqlmock.AnyArg(), sqlmock.AnyArg(), service.SeedanceFallbackStatusReady).
		WillReturnRows(sqlmock.NewRows([]string{"fallback_claim_token"}).AddRow("delete-token"))
	claimed, token, err := repo.ClaimSeedanceTaskCancellation(context.Background(), 1, 2, 3, "vidjob_delete")
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "delete-token", token)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$6 AND fallback_claim_token = \$7`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_delete", service.SeedanceFallbackStatusCancelled,
			service.SeedanceFallbackStatusCancelling, "delete-token").
		WillReturnResult(sqlmock.NewResult(0, 1))
	completed, err := repo.CompleteSeedanceTaskCancellation(context.Background(), 1, 2, 3, "vidjob_delete", "delete-token")
	require.NoError(t, err)
	require.True(t, completed)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_status = \$6 AND fallback_claim_token = \$7`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_delete", service.SeedanceFallbackStatusReady,
			service.SeedanceFallbackStatusCancelling, "delete-token-2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	released, err := repo.ReleaseSeedanceTaskCancellation(context.Background(), 1, 2, 3, "vidjob_delete", "delete-token-2")
	require.NoError(t, err)
	require.True(t, released)
	require.NoError(t, mock.ExpectationsWereMet())
}
