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

func TestHydrateSeedanceSettlementStateAddsAdminUsageStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	billingMode := string(service.BillingModeVideo)
	settledAt := time.Date(2026, 8, 4, 12, 30, 0, 0, time.UTC)
	logs := []service.UsageLog{
		{UserID: 7, APIKeyID: 8, RequestID: "seedance:hqv1_failed", BillingMode: &billingMode},
		{UserID: 7, APIKeyID: 8, RequestID: "regular-request", BillingMode: &billingMode},
	}

	mock.ExpectQuery(`(?s)SELECT user_id, api_key_id, job_id, task_status, refund_status.*FROM fflink_video_job_bindings.*job_id IN \(\$1\)`).
		WithArgs("hqv1_failed").
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "api_key_id", "job_id", "task_status", "refund_status", "settled_at", "last_error",
		}).AddRow(7, 8, "hqv1_failed", service.SeedanceTaskStatusFailed, service.SeedanceRefundStatusApplied, settledAt, "provider failed"))

	require.NoError(t, repo.hydrateSeedanceSettlementState(context.Background(), logs))
	require.NotNil(t, logs[0].VideoTaskStatus)
	require.Equal(t, service.SeedanceTaskStatusFailed, *logs[0].VideoTaskStatus)
	require.NotNil(t, logs[0].VideoRefundStatus)
	require.Equal(t, service.SeedanceRefundStatusApplied, *logs[0].VideoRefundStatus)
	require.NotNil(t, logs[0].VideoSettledAt)
	require.Equal(t, settledAt, *logs[0].VideoSettledAt)
	require.NotNil(t, logs[0].VideoSettlementError)
	require.Equal(t, "provider failed", *logs[0].VideoSettlementError)
	require.Nil(t, logs[1].VideoTaskStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}
