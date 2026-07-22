package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type seedanceRefundUsageRow struct {
	id                    int64
	userID                int64
	apiKeyID              int64
	accountID             int64
	groupID               sql.NullInt64
	subscriptionID        sql.NullInt64
	billingType           int8
	createdAt             time.Time
	accountStatsCost      sql.NullFloat64
	totalCost             float64
	actualCost            float64
	accountRateMultiplier float64
	platform              string
	accountType           string
}

func (r *usageBillingRepository) RefundSeedanceUsage(
	ctx context.Context,
	taskID string,
	userID int64,
	apiKeyID int64,
) (_ *service.SeedanceUsageRefundResult, err error) {
	requestID := service.SeedanceUsageRequestID(taskID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return &service.SeedanceUsageRefundResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	row, err := loadSeedanceRefundUsageRow(ctx, tx, requestID, userID, apiKeyID)
	if errors.Is(err, sql.ErrNoRows) {
		return &service.SeedanceUsageRefundResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := seedanceRefundResult(row)
	if row.actualCost <= 0 {
		return result, nil
	}

	if err := reverseSeedanceUserCharge(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := reverseSeedanceAPIKeyUsage(ctx, tx, row.apiKeyID, row.actualCost); err != nil {
		return nil, err
	}
	if err := reverseSeedanceAccountQuota(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := reverseSeedancePlatformQuota(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := reverseSeedanceUsageAggregates(ctx, tx, row); err != nil {
		return nil, err
	}
	if err := zeroSeedanceUsageCosts(ctx, tx, row.id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	result.Applied = true
	result.RefundedCost = row.actualCost
	return result, nil
}

func loadSeedanceRefundUsageRow(ctx context.Context, tx *sql.Tx, requestID string, userID, apiKeyID int64) (*seedanceRefundUsageRow, error) {
	row := &seedanceRefundUsageRow{}
	err := tx.QueryRowContext(ctx, `
		SELECT
			ul.id, ul.user_id, ul.api_key_id, ul.account_id, ul.group_id,
			ul.subscription_id, ul.billing_type, ul.created_at, ul.account_stats_cost,
			ul.total_cost, ul.actual_cost, COALESCE(ul.account_rate_multiplier, 1),
			COALESCE(g.platform, ''), COALESCE(a.type, '')
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.request_id = $1
			AND ul.user_id = $2
			AND ul.api_key_id = $3
			AND COALESCE(ul.billing_mode, '') = 'video'
		ORDER BY ul.id DESC
		LIMIT 1
		FOR UPDATE OF ul
	`, requestID, userID, apiKeyID).Scan(
		&row.id, &row.userID, &row.apiKeyID, &row.accountID, &row.groupID,
		&row.subscriptionID, &row.billingType, &row.createdAt, &row.accountStatsCost,
		&row.totalCost, &row.actualCost,
		&row.accountRateMultiplier, &row.platform, &row.accountType,
	)
	return row, err
}

func seedanceRefundResult(row *seedanceRefundUsageRow) *service.SeedanceUsageRefundResult {
	result := &service.SeedanceUsageRefundResult{
		UsageLogID:  row.id,
		UserID:      row.userID,
		APIKeyID:    row.apiKeyID,
		AccountID:   row.accountID,
		BillingType: row.billingType,
		Platform:    strings.TrimSpace(row.platform),
	}
	if row.groupID.Valid {
		value := row.groupID.Int64
		result.GroupID = &value
	}
	if row.subscriptionID.Valid {
		value := row.subscriptionID.Int64
		result.SubscriptionID = &value
	}
	return result
}

func reverseSeedanceUserCharge(ctx context.Context, tx *sql.Tx, row *seedanceRefundUsageRow) error {
	if row.billingType == service.BillingTypeSubscription {
		if !row.subscriptionID.Valid {
			return errors.New("seedance subscription usage is missing subscription_id")
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE user_subscriptions
			SET daily_usage_usd = GREATEST(0, daily_usage_usd - $1),
				weekly_usage_usd = GREATEST(0, weekly_usage_usd - $1),
				monthly_usage_usd = GREATEST(0, monthly_usage_usd - $1),
				updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`, row.actualCost, row.subscriptionID.Int64)
		if err != nil {
			return err
		}
		return requireAffectedRow(res, service.ErrSubscriptionNotFound)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, row.actualCost, row.userID)
	if err != nil {
		return err
	}
	return requireAffectedRow(res, service.ErrUserNotFound)
}

func reverseSeedanceAPIKeyUsage(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET quota_used = CASE WHEN quota > 0 THEN GREATEST(0, quota_used - $1) ELSE quota_used END,
			usage_5h = CASE WHEN rate_limit_5h > 0 THEN GREATEST(0, usage_5h - $1) ELSE usage_5h END,
			usage_1d = CASE WHEN rate_limit_1d > 0 THEN GREATEST(0, usage_1d - $1) ELSE usage_1d END,
			usage_7d = CASE WHEN rate_limit_7d > 0 THEN GREATEST(0, usage_7d - $1) ELSE usage_7d END,
			status = CASE
				WHEN status = $3 AND quota > 0 AND GREATEST(0, quota_used - $1) < quota THEN $4
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $2
	`, cost, apiKeyID, service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyActive)
	return err
}

func reverseSeedanceAccountQuota(ctx context.Context, tx *sql.Tx, row *seedanceRefundUsageRow) error {
	if !strings.EqualFold(row.accountType, service.AccountTypeAPIKey) && !strings.EqualFold(row.accountType, service.AccountTypeBedrock) {
		return nil
	}
	accountCost := row.totalCost * row.accountRateMultiplier
	if accountCost <= 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = COALESCE(extra, '{}'::jsonb)
			|| CASE WHEN COALESCE((extra->>'quota_limit')::numeric, 0) > 0
				THEN jsonb_build_object('quota_used', GREATEST(0, COALESCE((extra->>'quota_used')::numeric, 0) - $1)) ELSE '{}'::jsonb END
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0
				THEN jsonb_build_object('quota_daily_used', GREATEST(0, COALESCE((extra->>'quota_daily_used')::numeric, 0) - $1)) ELSE '{}'::jsonb END
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0
				THEN jsonb_build_object('quota_weekly_used', GREATEST(0, COALESCE((extra->>'quota_weekly_used')::numeric, 0) - $1)) ELSE '{}'::jsonb END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
			AND (
				COALESCE((extra->>'quota_limit')::numeric, 0) > 0 OR
				COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 OR
				COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0
			)
	`, accountCost, row.accountID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &row.accountID, nil, nil)
	}
	return nil
}

func reverseSeedancePlatformQuota(ctx context.Context, tx *sql.Tx, row *seedanceRefundUsageRow) error {
	if row.billingType != service.BillingTypeBalance || strings.TrimSpace(row.platform) == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE user_platform_quotas
		SET daily_usage_usd = CASE WHEN daily_limit_usd IS NOT NULL THEN GREATEST(0, daily_usage_usd - $1) ELSE daily_usage_usd END,
			weekly_usage_usd = CASE WHEN weekly_limit_usd IS NOT NULL THEN GREATEST(0, weekly_usage_usd - $1) ELSE weekly_usage_usd END,
			monthly_usage_usd = CASE WHEN monthly_limit_usd IS NOT NULL THEN GREATEST(0, monthly_usage_usd - $1) ELSE monthly_usage_usd END,
			updated_at = NOW()
		WHERE user_id = $2 AND platform = $3 AND deleted_at IS NULL
	`, row.actualCost, row.userID, strings.TrimSpace(row.platform))
	return err
}

func reverseSeedanceUsageAggregates(ctx context.Context, tx *sql.Tx, row *seedanceRefundUsageRow) error {
	if row.createdAt.IsZero() {
		return nil
	}
	accountCost := row.totalCost
	if row.accountStatsCost.Valid {
		accountCost = row.accountStatsCost.Float64
	}
	accountCost *= row.accountRateMultiplier

	if _, err := tx.ExecContext(ctx, `
		UPDATE usage_dashboard_hourly
		SET total_cost = GREATEST(0, total_cost - $1),
			actual_cost = GREATEST(0, actual_cost - $2),
			account_cost = GREATEST(0, account_cost - $3),
			computed_at = NOW()
		WHERE bucket_start = date_trunc('hour', $4::timestamptz)
	`, row.totalCost, row.actualCost, accountCost, row.createdAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_dashboard_daily
		SET total_cost = GREATEST(0, total_cost - $1),
			actual_cost = GREATEST(0, actual_cost - $2),
			account_cost = GREATEST(0, account_cost - $3),
			computed_at = NOW()
		WHERE bucket_date = ($4::timestamptz)::date
	`, row.totalCost, row.actualCost, accountCost, row.createdAt)
	return err
}

func zeroSeedanceUsageCosts(ctx context.Context, tx *sql.Tx, usageLogID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_logs
		SET input_cost = 0, output_cost = 0, cache_creation_cost = 0, cache_read_cost = 0,
			image_input_cost = 0, image_output_cost = 0, total_cost = 0, actual_cost = 0,
			account_stats_cost = CASE WHEN account_stats_cost IS NULL THEN NULL ELSE 0 END
		WHERE id = $1
	`, usageLogID)
	return err
}

func requireAffectedRow(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return notFound
	}
	return nil
}
