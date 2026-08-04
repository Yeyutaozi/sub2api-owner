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
			model, fallback_model, fallback_status, request_snapshot, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, NOW())
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
		SELECT user_id, api_key_id, group_id, account_id, job_id, upstream_job_id,
		       model, COALESCE(fallback_model, ''), fallback_status,
		       COALESCE(fallback_claim_token, ''), fallback_lease_until, request_snapshot,
		       created_at, updated_at
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
	binding := &service.SeedanceTaskBinding{}
	var fallbackLeaseUntil sql.NullTime
	if err := rows.Scan(
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
		&binding.CreatedAt,
		&binding.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan seedance task binding: %w", err)
	}
	if fallbackLeaseUntil.Valid {
		binding.FallbackLeaseUntil = fallbackLeaseUntil.Time
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
		SELECT user_id, api_key_id, group_id, account_id, job_id, upstream_job_id,
		       model, COALESCE(fallback_model, ''), fallback_status,
		       COALESCE(fallback_claim_token, ''), fallback_lease_until, request_snapshot,
		       created_at, updated_at
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
		var binding service.SeedanceTaskBinding
		var fallbackLeaseUntil sql.NullTime
		if err := rows.Scan(
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
			&binding.CreatedAt,
			&binding.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan seedance task binding: %w", err)
		}
		if fallbackLeaseUntil.Valid {
			binding.FallbackLeaseUntil = fallbackLeaseUntil.Time
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list seedance task bindings: %w", err)
	}
	return bindings, nil
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
