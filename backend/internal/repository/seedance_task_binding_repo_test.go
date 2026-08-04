//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var seedanceTaskBindingTestColumns = []string{
	"id", "user_id", "api_key_id", "group_id", "account_id", "job_id", "upstream_job_id", "model",
	"fallback_model", "fallback_status", "fallback_claim_token", "fallback_lease_until", "request_snapshot",
	"task_status", "next_poll_at", "last_polled_at", "settled_at", "refunded_at", "refund_status", "refund_attempts",
	"settlement_attempts", "settlement_claimed_at", "settlement_claimed_by", "last_error", "created_at", "updated_at",
}

func seedanceTaskBindingTestRow(id, accountID int64, jobID, model string, createdAt time.Time) []driver.Value {
	return []driver.Value{
		id, int64(1), int64(2), int64(3), accountID, jobID, jobID, model,
		"", "", "", nil, nil, service.SeedanceTaskStatusQueued, createdAt, nil, nil, nil, "", 0, 0,
		nil, "", "", createdAt, createdAt,
	}
}

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

	mock.ExpectQuery(`(?s)SELECT\s+id, user_id, api_key_id, group_id, account_id, job_id, upstream_job_id`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one").
		WillReturnRows(sqlmock.NewRows(seedanceTaskBindingTestColumns).
			AddRow(seedanceTaskBindingTestRow(11, 4, "vidjob_one", "seedance-2.0", createdAt)...))
	binding, err := repo.GetSeedanceTaskBinding(context.Background(), 1, 2, 3, "vidjob_one")
	require.NoError(t, err)
	require.Equal(t, int64(4), binding.AccountID)
	require.Equal(t, createdAt, binding.CreatedAt)

	mock.ExpectQuery(`(?s)SELECT\s+id, user_id, api_key_id, group_id, account_id, job_id, upstream_job_id`).
		WithArgs(int64(1), int64(2), int64(3), 20).
		WillReturnRows(sqlmock.NewRows(seedanceTaskBindingTestColumns).
			AddRow(seedanceTaskBindingTestRow(12, 4, "vidjob_two", "ltx-2.0", createdAt.Add(time.Minute))...).
			AddRow(seedanceTaskBindingTestRow(11, 5, "vidjob_one", "seedance-2.0", createdAt)...))
	bindings, err := repo.ListSeedanceTaskBindings(context.Background(), 1, 2, 3, 20)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "vidjob_two", bindings[0].JobID)
	require.Equal(t, int64(5), bindings[1].AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryClaimsAndCompletesSeedanceSettlementWithLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	row := seedanceTaskBindingTestRow(21, 9, "hqv1_task", "sd2-mx933-720-1s", now)
	row[21] = now
	row[22] = "settlement-worker"

	mock.ExpectQuery(`(?s)FOR UPDATE SKIP LOCKED.*UPDATE fflink_video_job_bindings.*settlement_claimed_by.*RETURNING\s+binding\.id, binding\.user_id`).
		WithArgs(25, "settlement-worker", int64((3*time.Minute)/time.Millisecond)).
		WillReturnRows(sqlmock.NewRows(seedanceTaskBindingTestColumns).AddRow(row...))

	claimed, err := repo.ClaimSeedanceTaskSettlements(context.Background(), "settlement-worker", 25, 3*time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(21), claimed[0].ID)
	require.Equal(t, "settlement-worker", claimed[0].SettlementClaimedBy)

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*settlement_claimed_at = NOW\(\).*settlement_claimed_by = \$2`).
		WithArgs(int64(21), "settlement-worker").
		WillReturnResult(sqlmock.NewResult(0, 1))
	renewed, err := repo.RenewSeedanceTaskSettlement(context.Background(), 21, "settlement-worker")
	require.NoError(t, err)
	require.True(t, renewed)

	settledAt := now.Add(time.Minute)
	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*settled_at = \$5.*WHERE id = \$1 AND settlement_claimed_by = \$2`).
		WithArgs(int64(21), "settlement-worker", service.SeedanceTaskStatusFailed, nil, &settledAt, &settledAt,
			service.SeedanceRefundStatusApplied, 1, 2, "provider failed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	updated, err := repo.CompleteSeedanceTaskSettlement(context.Background(), 21, "settlement-worker", service.SeedanceTaskSettlementUpdate{
		TaskStatus: service.SeedanceTaskStatusFailed, SettledAt: &settledAt, RefundedAt: &settledAt,
		RefundStatus: service.SeedanceRefundStatusApplied, RefundAttempts: 1, SettlementAttempts: 2,
		LastError: "provider failed",
	})
	require.NoError(t, err)
	require.True(t, updated)
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

	mock.ExpectExec(`(?s)UPDATE fflink_video_job_bindings.*fallback_lease_until = NOW\(\).*fallback_status = \$5 AND fallback_claim_token = \$6`).
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one", service.SeedanceFallbackStatusStarting,
			"claim-token-4", service.SeedanceFallbackLeaseDuration.Milliseconds()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	renewed, err := repo.RenewSeedanceTaskFallback(context.Background(), 1, 2, 3, "vidjob_one", "claim-token-4")
	require.NoError(t, err)
	require.True(t, renewed)
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
