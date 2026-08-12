package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var _ service.SeedanceTaskBindingRepository = (*usageLogRepository)(nil)
var _ service.SeedanceTaskFallbackRepository = (*usageLogRepository)(nil)
var _ service.SeedanceTaskCancellationRepository = (*usageLogRepository)(nil)
var _ service.SeedanceTaskSettlementRepository = (*usageLogRepository)(nil)
var _ service.SeedanceTaskAdminRepository = (*usageLogRepository)(nil)

const seedanceTaskBindingSelectColumns = `
		id, user_id, api_key_id, group_id, account_id, job_id, upstream_job_id,
		model, COALESCE(fallback_model, ''), fallback_status,
		COALESCE(fallback_claim_token, ''), fallback_lease_until, request_snapshot,
		task_status, next_poll_at, last_polled_at, settled_at, refunded_at,
		refund_status, refund_attempts, settlement_attempts, settlement_claimed_at,
		COALESCE(settlement_claimed_by, ''), COALESCE(last_error, ''), created_at, updated_at`

const seedanceTaskBindingReturningColumns = `
		binding.id, binding.user_id, binding.api_key_id, binding.group_id, binding.account_id,
		binding.job_id, binding.upstream_job_id, binding.model,
		COALESCE(binding.fallback_model, ''), binding.fallback_status,
		COALESCE(binding.fallback_claim_token, ''), binding.fallback_lease_until, binding.request_snapshot,
		binding.task_status, binding.next_poll_at, binding.last_polled_at, binding.settled_at, binding.refunded_at,
		binding.refund_status, binding.refund_attempts, binding.settlement_attempts, binding.settlement_claimed_at,
		COALESCE(binding.settlement_claimed_by, ''), COALESCE(binding.last_error, ''),
		binding.created_at, binding.updated_at`

type seedanceTaskBindingScanner interface {
	Scan(dest ...any) error
}

func scanSeedanceTaskBinding(scanner seedanceTaskBindingScanner) (*service.SeedanceTaskBinding, error) {
	binding := &service.SeedanceTaskBinding{}
	var fallbackLeaseUntil sql.NullTime
	var lastPolledAt sql.NullTime
	var settledAt sql.NullTime
	var refundedAt sql.NullTime
	var settlementClaimedAt sql.NullTime
	if err := scanner.Scan(
		&binding.ID,
		&binding.UserID,
		&binding.APIKeyID,
		&binding.GroupID,
		&binding.AccountID,
		&binding.JobID,
		&binding.UpstreamJobID,
		&binding.Model,
		&binding.FallbackModel,
		&binding.FallbackStatus,
		&binding.FallbackClaimToken,
		&fallbackLeaseUntil,
		&binding.RequestSnapshot,
		&binding.TaskStatus,
		&binding.NextPollAt,
		&lastPolledAt,
		&settledAt,
		&refundedAt,
		&binding.RefundStatus,
		&binding.RefundAttempts,
		&binding.SettlementAttempts,
		&settlementClaimedAt,
		&binding.SettlementClaimedBy,
		&binding.LastError,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if fallbackLeaseUntil.Valid {
		binding.FallbackLeaseUntil = fallbackLeaseUntil.Time
	}
	if lastPolledAt.Valid {
		binding.LastPolledAt = lastPolledAt.Time
	}
	if settledAt.Valid {
		binding.SettledAt = settledAt.Time
	}
	if refundedAt.Valid {
		binding.RefundedAt = refundedAt.Time
	}
	if settlementClaimedAt.Valid {
		binding.SettlementClaimedAt = settlementClaimedAt.Time
	}
	return binding, nil
}

func (r *usageLogRepository) SaveSeedanceTaskBinding(ctx context.Context, binding *service.SeedanceTaskBinding) error {
	if r == nil || r.sql == nil {
		return errors.New("seedance task binding repository is unavailable")
	}
	if binding == nil {
		return errors.New("seedance task binding is required")
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO fflink_video_job_bindings (
			user_id, api_key_id, group_id, account_id, job_id, upstream_job_id,
			model, fallback_model, fallback_status, request_snapshot,
			task_status, next_poll_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, 'queued', NOW() + INTERVAL '10 seconds', NOW())
		ON CONFLICT (user_id, api_key_id, group_id, job_id) DO NOTHING
	`, binding.UserID, binding.APIKeyID, binding.GroupID, binding.AccountID,
		strings.TrimSpace(binding.JobID), firstSeedanceBindingValue(binding.UpstreamJobID, binding.JobID),
		strings.TrimSpace(binding.Model), strings.TrimSpace(binding.FallbackModel),
		strings.TrimSpace(binding.FallbackStatus), nullableSeedanceSnapshot(binding.RequestSnapshot))
	if err != nil {
		return fmt.Errorf("save seedance task binding: %w", err)
	}
	return nil
}

func (r *usageLogRepository) GetSeedanceTaskBinding(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
) (*service.SeedanceTaskBinding, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("seedance task binding repository is unavailable")
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT `+seedanceTaskBindingSelectColumns+`
		FROM fflink_video_job_bindings
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		LIMIT 1
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID))
	if err != nil {
		return nil, fmt.Errorf("get seedance task binding: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get seedance task binding: %w", err)
		}
		return nil, errors.New("seedance task binding not found")
	}
	binding, err := scanSeedanceTaskBinding(rows)
	if err != nil {
		return nil, fmt.Errorf("scan seedance task binding: %w", err)
	}
	return binding, nil
}

func (r *usageLogRepository) ListSeedanceTaskBindings(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	limit int,
) ([]service.SeedanceTaskBinding, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("seedance task binding repository is unavailable")
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT `+seedanceTaskBindingSelectColumns+`
		FROM fflink_video_job_bindings
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, userID, apiKeyID, groupID, limit)
	if err != nil {
		return nil, fmt.Errorf("list seedance task bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	bindings := make([]service.SeedanceTaskBinding, 0)
	for rows.Next() {
		binding, err := scanSeedanceTaskBinding(rows)
		if err != nil {
			return nil, fmt.Errorf("scan seedance task binding: %w", err)
		}
		bindings = append(bindings, *binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list seedance task bindings: %w", err)
	}
	return bindings, nil
}

func (r *usageLogRepository) ClaimSeedanceTaskSettlements(
	ctx context.Context,
	workerID string,
	limit int,
	leaseDuration time.Duration,
) ([]service.SeedanceTaskBinding, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("seedance task settlement repository is unavailable")
	}
	if limit <= 0 {
		limit = 25
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, errors.New("seedance task settlement worker id is required")
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM fflink_video_job_bindings
			WHERE settled_at IS NULL
			  AND next_poll_at <= NOW()
			  AND (settlement_claimed_at IS NULL OR settlement_claimed_at < NOW() - ($3 * INTERVAL '1 millisecond'))
			ORDER BY next_poll_at ASC, id ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE fflink_video_job_bindings AS binding
		SET settlement_claimed_by = $2,
		    settlement_claimed_at = NOW(),
		    updated_at = NOW()
		FROM candidates
		WHERE binding.id = candidates.id
		RETURNING `+seedanceTaskBindingReturningColumns+`
	`, limit, workerID, leaseDuration.Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("claim seedance task settlements: %w", err)
	}
	defer rows.Close()
	bindings := make([]service.SeedanceTaskBinding, 0, limit)
	for rows.Next() {
		binding, scanErr := scanSeedanceTaskBinding(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed seedance task settlement: %w", scanErr)
		}
		bindings = append(bindings, *binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed seedance task settlements: %w", err)
	}
	return bindings, nil
}

func (r *usageLogRepository) RenewSeedanceTaskSettlement(
	ctx context.Context,
	id int64,
	workerID string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	if id <= 0 || strings.TrimSpace(workerID) == "" {
		return false, errors.New("seedance task settlement claim is invalid")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET settlement_claimed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND settlement_claimed_by = $2 AND settled_at IS NULL
	`, id, strings.TrimSpace(workerID))
	if err != nil {
		return false, fmt.Errorf("renew seedance task settlement: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("renew seedance task settlement rows: %w", err)
	}
	return rows == 1, nil
}

func (r *usageLogRepository) CompleteSeedanceTaskSettlement(
	ctx context.Context,
	id int64,
	workerID string,
	update service.SeedanceTaskSettlementUpdate,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	if id <= 0 || strings.TrimSpace(workerID) == "" {
		return false, errors.New("seedance task settlement claim is invalid")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET task_status = $3,
		    next_poll_at = COALESCE($4, next_poll_at),
		    last_polled_at = NOW(),
		    settled_at = $5,
		    refunded_at = $6,
		    refund_status = $7,
		    refund_attempts = $8,
		    settlement_attempts = $9,
		    last_error = NULLIF($10, ''),
		    settlement_claimed_at = NULL,
		    settlement_claimed_by = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND settlement_claimed_by = $2
	`, id, strings.TrimSpace(workerID), strings.TrimSpace(update.TaskStatus),
		update.NextPollAt, update.SettledAt, update.RefundedAt, strings.TrimSpace(update.RefundStatus),
		update.RefundAttempts, update.SettlementAttempts, strings.TrimSpace(update.LastError))
	if err != nil {
		return false, fmt.Errorf("complete seedance task settlement: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("complete seedance task settlement rows: %w", err)
	}
	return rows == 1, nil
}

func (r *usageLogRepository) ClaimSeedanceTaskFallback(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
) (bool, string, error) {
	if r == nil || r.sql == nil {
		return false, "", errors.New("seedance task binding repository is unavailable")
	}
	claimToken := uuid.NewString()
	leaseUntil := time.Now().UTC().Add(service.SeedanceFallbackLeaseDuration)
	rows, err := r.sql.QueryContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_status = $5,
		    fallback_claim_token = $6,
		    fallback_lease_until = $7,
		    updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND (
			fallback_status = $8 OR
			(fallback_status = $5 AND (fallback_lease_until IS NULL OR fallback_lease_until < NOW()))
		  )
		RETURNING fallback_claim_token
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID),
		service.SeedanceFallbackStatusStarting, claimToken, leaseUntil, service.SeedanceFallbackStatusReady)
	if err != nil {
		return false, "", fmt.Errorf("claim seedance task fallback: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, "", fmt.Errorf("claim seedance task fallback: %w", err)
		}
		return false, "", nil
	}
	var storedToken string
	if err := rows.Scan(&storedToken); err != nil {
		return false, "", fmt.Errorf("claim seedance task fallback: %w", err)
	}
	return storedToken != "", storedToken, nil
}

func (r *usageLogRepository) ActivateSeedanceTaskFallback(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
	claimToken string,
	accountID int64,
	upstreamJobID string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task binding repository is unavailable")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET account_id = $6, upstream_job_id = $7, fallback_status = $8,
		    fallback_claim_token = NULL, fallback_lease_until = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND fallback_status = $9 AND fallback_claim_token = $5
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID), strings.TrimSpace(claimToken), accountID,
		strings.TrimSpace(upstreamJobID), service.SeedanceFallbackStatusActive,
		service.SeedanceFallbackStatusStarting)
	if err != nil {
		return false, fmt.Errorf("activate seedance task fallback: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("activate seedance task fallback: %w", err)
	}
	return rows == 1, nil
}

func (r *usageLogRepository) FailSeedanceTaskFallback(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
	claimToken string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task binding repository is unavailable")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_status = $5, fallback_claim_token = NULL, fallback_lease_until = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND fallback_status = $6 AND fallback_claim_token = $7
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID),
		service.SeedanceFallbackStatusFailed, service.SeedanceFallbackStatusStarting, strings.TrimSpace(claimToken))
	if err != nil {
		return false, fmt.Errorf("fail seedance task fallback: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("fail seedance task fallback: %w", err)
	}
	return rows == 1, nil
}

// ReleaseSeedanceTaskFallback returns a locally blocked fallback attempt to the
// retryable state. The claim token prevents an expired worker from releasing a
// newer creator's reservation.
func (r *usageLogRepository) ReleaseSeedanceTaskFallback(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
	claimToken string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task binding repository is unavailable")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_status = $5, fallback_claim_token = NULL,
		    fallback_lease_until = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND fallback_status = $6 AND fallback_claim_token = $7
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID),
		service.SeedanceFallbackStatusReady, service.SeedanceFallbackStatusStarting, strings.TrimSpace(claimToken))
	if err != nil {
		return false, fmt.Errorf("release seedance task fallback: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("release seedance task fallback: %w", err)
	}
	return rows == 1, nil
}

func (r *usageLogRepository) RenewSeedanceTaskFallback(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
	claimToken string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task binding repository is unavailable")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_lease_until = NOW() + ($7 * INTERVAL '1 millisecond'),
		    updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND fallback_status = $5 AND fallback_claim_token = $6
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID),
		service.SeedanceFallbackStatusStarting, strings.TrimSpace(claimToken),
		service.SeedanceFallbackLeaseDuration.Milliseconds())
	if err != nil {
		return false, fmt.Errorf("renew seedance task fallback: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("renew seedance task fallback rows: %w", err)
	}
	return rows == 1, nil
}

// ClaimSeedanceTaskCancellation atomically reserves a task for DELETE. The
// reservation uses the same row/token protocol as fallback creation, so a
// ready task can be claimed by exactly one of DELETE or fallback creation.
func (r *usageLogRepository) ClaimSeedanceTaskCancellation(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
) (bool, string, error) {
	if r == nil || r.sql == nil {
		return false, "", errors.New("seedance task binding repository is unavailable")
	}
	claimToken := uuid.NewString()
	leaseUntil := time.Now().UTC().Add(service.SeedanceFallbackLeaseDuration)
	rows, err := r.sql.QueryContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_status = $5,
		    fallback_claim_token = $6,
		    fallback_lease_until = $7,
		    updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND (
			fallback_status = $8 OR
			(fallback_status = $5 AND (fallback_lease_until IS NULL OR fallback_lease_until < NOW()))
		  )
		RETURNING fallback_claim_token
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID),
		service.SeedanceFallbackStatusCancelling, claimToken, leaseUntil, service.SeedanceFallbackStatusReady)
	if err != nil {
		return false, "", fmt.Errorf("claim seedance task cancellation: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, "", fmt.Errorf("claim seedance task cancellation: %w", err)
		}
		return false, "", nil
	}
	var storedToken string
	if err := rows.Scan(&storedToken); err != nil {
		return false, "", fmt.Errorf("claim seedance task cancellation: %w", err)
	}
	return storedToken != "", storedToken, nil
}

func (r *usageLogRepository) CompleteSeedanceTaskCancellation(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID, claimToken string,
) (bool, error) {
	return r.updateSeedanceTaskCancellation(
		ctx, userID, apiKeyID, groupID, jobID, claimToken,
		service.SeedanceFallbackStatusCancelled,
	)
}

func (r *usageLogRepository) ReleaseSeedanceTaskCancellation(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID, claimToken string,
) (bool, error) {
	return r.updateSeedanceTaskCancellation(
		ctx, userID, apiKeyID, groupID, jobID, claimToken,
		service.SeedanceFallbackStatusReady,
	)
}

func (r *usageLogRepository) updateSeedanceTaskCancellation(
	ctx context.Context,
	userID, apiKeyID, groupID int64,
	jobID, claimToken, nextStatus string,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task binding repository is unavailable")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET fallback_status = $5, fallback_claim_token = NULL,
		    fallback_lease_until = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3 AND job_id = $4
		  AND fallback_status = $6 AND fallback_claim_token = $7
	`, userID, apiKeyID, groupID, strings.TrimSpace(jobID), nextStatus,
		service.SeedanceFallbackStatusCancelling, strings.TrimSpace(claimToken))
	if err != nil {
		return false, fmt.Errorf("update seedance task cancellation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update seedance task cancellation: %w", err)
	}
	return rows == 1, nil
}

func nullableSeedanceSnapshot(snapshot []byte) any {
	if len(snapshot) == 0 {
		return nil
	}
	return string(snapshot)
}

func firstSeedanceBindingValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}


func (r *usageLogRepository) ListAdminSeedanceTaskBindings(
	ctx context.Context,
	filters service.SeedanceTaskAdminFilters,
	page, pageSize int,
) ([]service.SeedanceTaskAdminItem, int64, error) {
	if r == nil || r.sql == nil {
		return nil, 0, errors.New("seedance task admin repository is unavailable")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	where := []string{"1=1"}
	args := make([]any, 0, 16)
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if jobID := strings.TrimSpace(filters.JobID); jobID != "" {
		where = append(where, "binding.job_id = "+next(jobID))
	}
	if filters.UserID > 0 {
		where = append(where, "binding.user_id = "+next(filters.UserID))
	}
	if filters.GroupID > 0 {
		where = append(where, "binding.group_id = "+next(filters.GroupID))
	}
	if filters.APIKeyID > 0 {
		where = append(where, "binding.api_key_id = "+next(filters.APIKeyID))
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		where = append(where, "binding.task_status = "+next(status))
	}
	if model := strings.TrimSpace(filters.Model); model != "" {
		where = append(where, "binding.model = "+next(model))
	}
	if filters.UnsettledOnly {
		where = append(where, "binding.settled_at IS NULL")
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		pattern := "%" + search + "%"
		p1, p2, p3, p4, p5 := next(pattern), next(pattern), next(pattern), next(pattern), next(pattern)
		where = append(where, fmt.Sprintf(
			"(binding.job_id ILIKE %s OR binding.upstream_job_id ILIKE %s OR COALESCE(u.email, '') ILIKE %s OR COALESCE(u.username, '') ILIKE %s OR binding.model ILIKE %s)",
			p1, p2, p3, p4, p5,
		))
	}
	whereSQL := strings.Join(where, " AND ")

	countQuery := `
		SELECT COUNT(*)
		FROM fflink_video_job_bindings AS binding
		LEFT JOIN users AS u ON u.id = binding.user_id
		WHERE ` + whereSQL
	var total int64
	countRows, err := r.sql.QueryContext(ctx, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin seedance task bindings: %w", err)
	}
	if !countRows.Next() {
		_ = countRows.Close()
		return nil, 0, fmt.Errorf("count admin seedance task bindings: no rows")
	}
	if err := countRows.Scan(&total); err != nil {
		_ = countRows.Close()
		return nil, 0, fmt.Errorf("count admin seedance task bindings: %w", err)
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, fmt.Errorf("count admin seedance task bindings: %w", err)
	}

	offset := (page - 1) * pageSize
	limitPH := next(pageSize)
	offsetPH := next(offset)
	listQuery := `
		SELECT ` + seedanceTaskBindingReturningColumns + `,
			COALESCE(u.email, ''), COALESCE(u.username, ''),
			COALESCE(g.name, ''), COALESCE(k.name, '')
		FROM fflink_video_job_bindings AS binding
		LEFT JOIN users AS u ON u.id = binding.user_id
		LEFT JOIN groups AS g ON g.id = binding.group_id
		LEFT JOIN api_keys AS k ON k.id = binding.api_key_id
		WHERE ` + whereSQL + `
		ORDER BY binding.created_at DESC, binding.id DESC
		LIMIT ` + limitPH + ` OFFSET ` + offsetPH

	rows, err := r.sql.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin seedance task bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.SeedanceTaskAdminItem, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanAdminSeedanceTaskBinding(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan admin seedance task binding: %w", scanErr)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin seedance task bindings: %w", err)
	}
	return items, total, nil
}

func (r *usageLogRepository) GetAdminSeedanceTaskBindingByJobID(
	ctx context.Context,
	jobID string,
) (*service.SeedanceTaskAdminItem, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("seedance task admin repository is unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("job_id is required")
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT `+seedanceTaskBindingReturningColumns+`,
			COALESCE(u.email, ''), COALESCE(u.username, ''),
			COALESCE(g.name, ''), COALESCE(k.name, '')
		FROM fflink_video_job_bindings AS binding
		LEFT JOIN users AS u ON u.id = binding.user_id
		LEFT JOIN groups AS g ON g.id = binding.group_id
		LEFT JOIN api_keys AS k ON k.id = binding.api_key_id
		WHERE binding.job_id = $1
		ORDER BY binding.created_at DESC, binding.id DESC
		LIMIT 1
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("get admin seedance task binding: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get admin seedance task binding: %w", err)
		}
		return nil, errors.New("seedance task binding not found")
	}
	item, err := scanAdminSeedanceTaskBinding(rows)
	if err != nil {
		return nil, fmt.Errorf("scan admin seedance task binding: %w", err)
	}
	return item, nil
}

func (r *usageLogRepository) ForceCompleteSeedanceTaskSettlement(
	ctx context.Context,
	id int64,
	update service.SeedanceTaskSettlementUpdate,
) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("seedance task admin repository is unavailable")
	}
	if id <= 0 {
		return false, errors.New("seedance task id is invalid")
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE fflink_video_job_bindings
		SET task_status = $2,
		    next_poll_at = COALESCE($3, next_poll_at),
		    last_polled_at = NOW(),
		    settled_at = $4,
		    refunded_at = $5,
		    refund_status = $6,
		    refund_attempts = $7,
		    settlement_attempts = $8,
		    last_error = NULLIF($9, ''),
		    settlement_claimed_at = NULL,
		    settlement_claimed_by = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND settled_at IS NULL
	`, id, strings.TrimSpace(update.TaskStatus), update.NextPollAt, update.SettledAt, update.RefundedAt,
		strings.TrimSpace(update.RefundStatus), update.RefundAttempts, update.SettlementAttempts,
		strings.TrimSpace(update.LastError))
	if err != nil {
		return false, fmt.Errorf("force complete seedance task settlement: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("force complete seedance task settlement rows: %w", err)
	}
	return rows == 1, nil
}

func scanAdminSeedanceTaskBinding(scanner seedanceTaskBindingScanner) (*service.SeedanceTaskAdminItem, error) {
	binding := &service.SeedanceTaskBinding{}
	var fallbackLeaseUntil sql.NullTime
	var lastPolledAt sql.NullTime
	var settledAt sql.NullTime
	var refundedAt sql.NullTime
	var settlementClaimedAt sql.NullTime
	var userEmail, username, groupName, apiKeyName string
	if err := scanner.Scan(
		&binding.ID,
		&binding.UserID,
		&binding.APIKeyID,
		&binding.GroupID,
		&binding.AccountID,
		&binding.JobID,
		&binding.UpstreamJobID,
		&binding.Model,
		&binding.FallbackModel,
		&binding.FallbackStatus,
		&binding.FallbackClaimToken,
		&fallbackLeaseUntil,
		&binding.RequestSnapshot,
		&binding.TaskStatus,
		&binding.NextPollAt,
		&lastPolledAt,
		&settledAt,
		&refundedAt,
		&binding.RefundStatus,
		&binding.RefundAttempts,
		&binding.SettlementAttempts,
		&settlementClaimedAt,
		&binding.SettlementClaimedBy,
		&binding.LastError,
		&binding.CreatedAt,
		&binding.UpdatedAt,
		&userEmail,
		&username,
		&groupName,
		&apiKeyName,
	); err != nil {
		return nil, err
	}
	if fallbackLeaseUntil.Valid {
		binding.FallbackLeaseUntil = fallbackLeaseUntil.Time
	}
	if lastPolledAt.Valid {
		binding.LastPolledAt = lastPolledAt.Time
	}
	if settledAt.Valid {
		binding.SettledAt = settledAt.Time
	}
	if refundedAt.Valid {
		binding.RefundedAt = refundedAt.Time
	}
	if settlementClaimedAt.Valid {
		binding.SettlementClaimedAt = settlementClaimedAt.Time
	}
	return &service.SeedanceTaskAdminItem{
		SeedanceTaskBinding: *binding,
		UserEmail:           userEmail,
		Username:            username,
		GroupName:           groupName,
		APIKeyName:          apiKeyName,
	}, nil
}
