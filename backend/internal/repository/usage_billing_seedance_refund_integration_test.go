//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSeedanceRefundPeriodicCountersPreserveLaterWindowsPostgres(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)

	_, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE user_subscriptions (
			id BIGINT PRIMARY KEY,
			daily_usage_usd NUMERIC NOT NULL,
			weekly_usage_usd NUMERIC NOT NULL,
			monthly_usage_usd NUMERIC NOT NULL,
			daily_window_start TIMESTAMPTZ,
			weekly_window_start TIMESTAMPTZ,
			monthly_window_start TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		) ON COMMIT DROP;
		CREATE TEMP TABLE api_keys (
			id BIGINT PRIMARY KEY,
			quota NUMERIC NOT NULL,
			quota_used NUMERIC NOT NULL,
			rate_limit_5h NUMERIC NOT NULL,
			rate_limit_1d NUMERIC NOT NULL,
			rate_limit_7d NUMERIC NOT NULL,
			usage_5h NUMERIC NOT NULL,
			usage_1d NUMERIC NOT NULL,
			usage_7d NUMERIC NOT NULL,
			window_5h_start TIMESTAMPTZ,
			window_1d_start TIMESTAMPTZ,
			window_7d_start TIMESTAMPTZ,
			status TEXT NOT NULL,
			updated_at TIMESTAMPTZ
		) ON COMMIT DROP;
		CREATE TEMP TABLE accounts (
			id BIGINT PRIMARY KEY,
			extra JSONB,
			updated_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		) ON COMMIT DROP;
		CREATE TEMP TABLE scheduler_outbox (
			event_type TEXT NOT NULL,
			account_id BIGINT,
			group_id BIGINT,
			payload JSONB,
			dedup_key TEXT
		) ON COMMIT DROP;
		CREATE UNIQUE INDEX scheduler_outbox_dedup_key_test
			ON scheduler_outbox (dedup_key) WHERE dedup_key IS NOT NULL;
		CREATE TEMP TABLE user_platform_quotas (
			user_id BIGINT NOT NULL,
			platform TEXT NOT NULL,
			daily_limit_usd NUMERIC,
			weekly_limit_usd NUMERIC,
			monthly_limit_usd NUMERIC,
			daily_usage_usd NUMERIC NOT NULL,
			weekly_usage_usd NUMERIC NOT NULL,
			monthly_usage_usd NUMERIC NOT NULL,
			daily_window_start TIMESTAMPTZ,
			weekly_window_start TIMESTAMPTZ,
			monthly_window_start TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		) ON COMMIT DROP;
	`)
	require.NoError(t, err)

	createdAt := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	laterWindow := createdAt.Add(48 * time.Hour)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_subscriptions (
			id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			daily_window_start, weekly_window_start, monthly_window_start
		) VALUES ($1, 10, 20, 30, $2, $2, $2)
	`, int64(29), laterWindow)
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceUserCharge(ctx, tx, &seedanceRefundUsageRow{
		billingType:    service.BillingTypeSubscription,
		subscriptionID: sql.NullInt64{Int64: 29, Valid: true},
		actualCost:     1.5,
		createdAt:      createdAt,
	}))

	var subscriptionDaily, subscriptionWeekly, subscriptionMonthly float64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_subscriptions WHERE id = $1
	`, int64(29)).Scan(&subscriptionDaily, &subscriptionWeekly, &subscriptionMonthly))
	require.InDelta(t, 10, subscriptionDaily, 1e-12)
	require.InDelta(t, 20, subscriptionWeekly, 1e-12)
	require.InDelta(t, 30, subscriptionMonthly, 1e-12)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_keys (
			id, quota, quota_used, rate_limit_5h, rate_limit_1d, rate_limit_7d,
			usage_5h, usage_1d, usage_7d,
			window_5h_start, window_1d_start, window_7d_start, status
		) VALUES ($1, 10, 10, 100, 100, 100, 11, 12, 13, $2, $2, $2, $3)
	`, int64(17), laterWindow, service.StatusAPIKeyQuotaExhausted)
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceAPIKeyUsage(ctx, tx, 17, 1.5, createdAt))

	var apiQuotaUsed, usage5h, usage1d, usage7d float64
	var apiStatus string
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT quota_used, usage_5h, usage_1d, usage_7d, status
		FROM api_keys WHERE id = $1
	`, int64(17)).Scan(&apiQuotaUsed, &usage5h, &usage1d, &usage7d, &apiStatus))
	require.InDelta(t, 8.5, apiQuotaUsed, 1e-12)
	require.InDelta(t, 11, usage5h, 1e-12)
	require.InDelta(t, 12, usage1d, 1e-12)
	require.InDelta(t, 13, usage7d, 1e-12)
	require.Equal(t, service.StatusAPIKeyActive, apiStatus)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (id, extra) VALUES ($1, jsonb_build_object(
			'quota_limit', 100,
			'quota_used', 20,
			'quota_daily_limit', 100,
			'quota_daily_used', 21,
			'quota_daily_start', $2::text,
			'quota_weekly_limit', 100,
			'quota_weekly_used', 22,
			'quota_weekly_start', $2::text
		))
	`, int64(39), laterWindow.Format(time.RFC3339))
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceAccountQuota(ctx, tx, &seedanceRefundUsageRow{
		accountID:             39,
		accountType:           service.AccountTypeAPIKey,
		totalCost:             4,
		accountRateMultiplier: 1.5,
		createdAt:             createdAt,
	}))

	var accountTotal, accountDaily, accountWeekly float64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT
			(extra->>'quota_used')::double precision,
			(extra->>'quota_daily_used')::double precision,
			(extra->>'quota_weekly_used')::double precision
		FROM accounts WHERE id = $1
	`, int64(39)).Scan(&accountTotal, &accountDaily, &accountWeekly))
	require.InDelta(t, 14, accountTotal, 1e-12)
	require.InDelta(t, 21, accountDaily, 1e-12)
	require.InDelta(t, 22, accountWeekly, 1e-12)

	var outboxCount int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox`).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_platform_quotas (
			user_id, platform, daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
			daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			daily_window_start, weekly_window_start, monthly_window_start
		) VALUES ($1, $2, 100, 100, 100, 31, 32, 33, $3, $3, $3)
	`, int64(62), service.PlatformSeedance, laterWindow)
	require.NoError(t, err)
	require.NoError(t, reverseSeedancePlatformQuota(ctx, tx, &seedanceRefundUsageRow{
		userID:      62,
		billingType: service.BillingTypeBalance,
		actualCost:  1.2,
		platform:    service.PlatformSeedance,
		createdAt:   createdAt,
	}))

	var platformDaily, platformWeekly, platformMonthly float64
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_platform_quotas WHERE user_id = $1 AND platform = $2
	`, int64(62), service.PlatformSeedance).Scan(&platformDaily, &platformWeekly, &platformMonthly))
	require.InDelta(t, 31, platformDaily, 1e-12)
	require.InDelta(t, 32, platformWeekly, 1e-12)
	require.InDelta(t, 33, platformMonthly, 1e-12)

	currentWindow := createdAt.Add(-time.Hour)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_subscriptions (
			id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			daily_window_start, weekly_window_start, monthly_window_start
		) VALUES ($1, 10, 20, 30, $2, $2, $2)
	`, int64(30), currentWindow)
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceUserCharge(ctx, tx, &seedanceRefundUsageRow{
		billingType:    service.BillingTypeSubscription,
		subscriptionID: sql.NullInt64{Int64: 30, Valid: true},
		actualCost:     1.5,
		createdAt:      createdAt,
	}))
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_subscriptions WHERE id = $1
	`, int64(30)).Scan(&subscriptionDaily, &subscriptionWeekly, &subscriptionMonthly))
	require.InDelta(t, 8.5, subscriptionDaily, 1e-12)
	require.InDelta(t, 18.5, subscriptionWeekly, 1e-12)
	require.InDelta(t, 28.5, subscriptionMonthly, 1e-12)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO api_keys (
			id, quota, quota_used, rate_limit_5h, rate_limit_1d, rate_limit_7d,
			usage_5h, usage_1d, usage_7d,
			window_5h_start, window_1d_start, window_7d_start, status
		) VALUES ($1, 10, 10, 100, 100, 100, 11, 12, 13, $2, $2, $2, $3)
	`, int64(18), currentWindow, service.StatusAPIKeyQuotaExhausted)
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceAPIKeyUsage(ctx, tx, 18, 1.5, createdAt))
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT quota_used, usage_5h, usage_1d, usage_7d, status
		FROM api_keys WHERE id = $1
	`, int64(18)).Scan(&apiQuotaUsed, &usage5h, &usage1d, &usage7d, &apiStatus))
	require.InDelta(t, 8.5, apiQuotaUsed, 1e-12)
	require.InDelta(t, 9.5, usage5h, 1e-12)
	require.InDelta(t, 10.5, usage1d, 1e-12)
	require.InDelta(t, 11.5, usage7d, 1e-12)
	require.Equal(t, service.StatusAPIKeyActive, apiStatus)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (id, extra) VALUES ($1, jsonb_build_object(
			'quota_limit', 100,
			'quota_used', 20,
			'quota_daily_limit', 100,
			'quota_daily_used', 21,
			'quota_daily_start', $2::text,
			'quota_weekly_limit', 100,
			'quota_weekly_used', 22,
			'quota_weekly_start', $2::text
		))
	`, int64(40), currentWindow.Format(time.RFC3339))
	require.NoError(t, err)
	require.NoError(t, reverseSeedanceAccountQuota(ctx, tx, &seedanceRefundUsageRow{
		accountID:             40,
		accountType:           service.AccountTypeAPIKey,
		totalCost:             4,
		accountRateMultiplier: 1.5,
		createdAt:             createdAt,
	}))
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT
			(extra->>'quota_used')::double precision,
			(extra->>'quota_daily_used')::double precision,
			(extra->>'quota_weekly_used')::double precision
		FROM accounts WHERE id = $1
	`, int64(40)).Scan(&accountTotal, &accountDaily, &accountWeekly))
	require.InDelta(t, 14, accountTotal, 1e-12)
	require.InDelta(t, 15, accountDaily, 1e-12)
	require.InDelta(t, 16, accountWeekly, 1e-12)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_platform_quotas (
			user_id, platform, daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
			daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			daily_window_start, weekly_window_start, monthly_window_start
		) VALUES ($1, $2, 100, 100, 100, 31, 32, 33, $3, $3, $3)
	`, int64(63), service.PlatformSeedance, currentWindow)
	require.NoError(t, err)
	require.NoError(t, reverseSeedancePlatformQuota(ctx, tx, &seedanceRefundUsageRow{
		userID:      63,
		billingType: service.BillingTypeBalance,
		actualCost:  1.2,
		platform:    service.PlatformSeedance,
		createdAt:   createdAt,
	}))
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_platform_quotas WHERE user_id = $1 AND platform = $2
	`, int64(63), service.PlatformSeedance).Scan(&platformDaily, &platformWeekly, &platformMonthly))
	require.InDelta(t, 29.8, platformDaily, 1e-12)
	require.InDelta(t, 30.8, platformWeekly, 1e-12)
	require.InDelta(t, 31.8, platformMonthly, 1e-12)
}
