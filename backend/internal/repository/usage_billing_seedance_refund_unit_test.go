//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const seedanceRefundUsageSelectSQL = `(?s)SELECT\s+ul.id, ul.user_id, ul.api_key_id.*FOR UPDATE OF ul`

var seedanceRefundUsageColumns = []string{
	"id", "user_id", "api_key_id", "account_id", "group_id", "subscription_id",
	"billing_type", "created_at", "account_stats_cost", "total_cost", "actual_cost",
	"account_rate_multiplier", "platform", "type",
}

func TestRefundSeedanceUsage_ReversesBalanceAndUsageOnce(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &usageBillingRepository{db: db}
	createdAt := time.Date(2026, time.July, 22, 13, 15, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(seedanceRefundUsageSelectSQL).
		WithArgs("seedance:vidjob_123", int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows(seedanceRefundUsageColumns).AddRow(
			91, 42, 7, 9, nil, nil, service.BillingTypeBalance,
			createdAt, nil, 0.8, 0.8, 1.0, "", service.AccountTypeOAuth,
		))
	mock.ExpectExec(`(?s)UPDATE users\s+SET balance = balance \+ \$1`).
		WithArgs(0.8, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE api_keys\s+SET quota_used`).
		WithArgs(0.8, int64(7), service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_hourly`).
		WithArgs(0.8, 0.8, 0.8, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_daily`).
		WithArgs(0.8, 0.8, 0.8, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_logs\s+SET input_cost = 0`).
		WithArgs(int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.RefundSeedanceUsage(ctx, "vidjob_123", 42, 7)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, int64(91), result.UsageLogID)
	require.InDelta(t, 0.8, result.RefundedCost, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectBegin()
	mock.ExpectQuery(seedanceRefundUsageSelectSQL).
		WithArgs("seedance:vidjob_123", int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows(seedanceRefundUsageColumns).AddRow(
			91, 42, 7, 9, nil, nil, service.BillingTypeBalance,
			createdAt, nil, 0.0, 0.0, 1.0, "", service.AccountTypeOAuth,
		))
	mock.ExpectRollback()

	result, err = repo.RefundSeedanceUsage(ctx, "vidjob_123", 42, 7)
	require.NoError(t, err)
	require.False(t, result.Applied)
	require.Zero(t, result.RefundedCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefundSeedanceUsage_MissingUsageLogIsRetryableNoop(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &usageBillingRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(seedanceRefundUsageSelectSQL).
		WithArgs("seedance:vidjob_missing", int64(42), int64(7)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	result, err := repo.RefundSeedanceUsage(ctx, "vidjob_missing", 42, 7)
	require.NoError(t, err)
	require.False(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefundSeedanceUsage_ReversesSubscriptionUsage(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &usageBillingRepository{db: db}
	createdAt := time.Date(2026, time.July, 22, 14, 15, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(seedanceRefundUsageSelectSQL).
		WithArgs("seedance:vidjob_subscription", int64(52), int64(17)).
		WillReturnRows(sqlmock.NewRows(seedanceRefundUsageColumns).AddRow(
			92, 52, 17, 19, 3, 29, service.BillingTypeSubscription,
			createdAt, nil, 1.2, 1.5, 1.0, service.PlatformSeedance, service.AccountTypeOAuth,
		))
	mock.ExpectExec(`(?s)UPDATE user_subscriptions\s+SET daily_usage_usd`).
		WithArgs(1.5, int64(29)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE api_keys\s+SET quota_used`).
		WithArgs(1.5, int64(17), service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_hourly`).
		WithArgs(1.2, 1.5, 1.2, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_daily`).
		WithArgs(1.2, 1.5, 1.2, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_logs\s+SET input_cost = 0`).
		WithArgs(int64(92)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.RefundSeedanceUsage(ctx, "vidjob_subscription", 52, 17)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NotNil(t, result.SubscriptionID)
	require.Equal(t, int64(29), *result.SubscriptionID)
	require.InDelta(t, 1.5, result.RefundedCost, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefundSeedanceUsage_ReversesAccountAndPlatformQuotas(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &usageBillingRepository{db: db}
	createdAt := time.Date(2026, time.July, 22, 15, 15, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(seedanceRefundUsageSelectSQL).
		WithArgs("seedance:vidjob_quotas", int64(62), int64(27)).
		WillReturnRows(sqlmock.NewRows(seedanceRefundUsageColumns).AddRow(
			93, 62, 27, 39, 3, nil, service.BillingTypeBalance,
			createdAt, 2.0, 4.0, 1.2, 1.5, service.PlatformSeedance, service.AccountTypeAPIKey,
		))
	mock.ExpectExec(`(?s)UPDATE users\s+SET balance = balance \+ \$1`).
		WithArgs(1.2, int64(62)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE api_keys\s+SET quota_used`).
		WithArgs(1.2, int64(27), service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE accounts\s+SET extra`).
		WithArgs(6.0, int64(39)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO scheduler_outbox`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE user_platform_quotas\s+SET daily_usage_usd`).
		WithArgs(1.2, int64(62), service.PlatformSeedance).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_hourly`).
		WithArgs(4.0, 1.2, 3.0, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_dashboard_daily`).
		WithArgs(4.0, 1.2, 3.0, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE usage_logs\s+SET input_cost = 0`).
		WithArgs(int64(93)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.RefundSeedanceUsage(ctx, "vidjob_quotas", 62, 27)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, service.PlatformSeedance, result.Platform)
	require.InDelta(t, 1.2, result.RefundedCost, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())
}
