//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestApplyWithUsageLogCommitsBillingIdentityAndUsageRowTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	createdAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO usage_billing_dedup.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT request_fingerprint.*usage_billing_dedup_archive`).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectQuery(`(?s)INSERT INTO usage_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(41, createdAt))
	mock.ExpectCommit()

	repo := &usageBillingRepository{db: db}
	usageLog := &service.UsageLog{RequestID: "seedance:job-atomic", APIKeyID: 7}
	result, err := repo.ApplyWithUsageLog(context.Background(), &service.UsageBillingCommand{
		RequestID: "seedance:job-atomic", APIKeyID: 7, UserID: 8, AccountID: 9,
	}, usageLog)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, int64(41), usageLog.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyWithUsageLogRollsBackWhenUsageRowCannotPersist(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO usage_billing_dedup.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT request_fingerprint.*usage_billing_dedup_archive`).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectQuery(`(?s)UPDATE users.*SET balance = balance -.*RETURNING balance`).
		WithArgs(1.25, int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(8.75))
	mock.ExpectQuery(`(?s)INSERT INTO usage_logs`).
		WillReturnError(errors.New("usage log write failed"))
	mock.ExpectRollback()

	repo := &usageBillingRepository{db: db}
	_, err = repo.ApplyWithUsageLog(context.Background(), &service.UsageBillingCommand{
		RequestID: "seedance:job-rollback", APIKeyID: 7, UserID: 8, AccountID: 9, BalanceCost: 1.25,
	}, &service.UsageLog{RequestID: "seedance:job-rollback", APIKeyID: 7})
	require.ErrorContains(t, err, "usage log write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}
