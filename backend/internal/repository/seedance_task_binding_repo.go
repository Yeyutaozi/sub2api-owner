package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.SeedanceTaskBindingRepository = (*usageLogRepository)(nil)

func (r *usageLogRepository) SaveSeedanceTaskBinding(ctx context.Context, binding *service.SeedanceTaskBinding) error {
	if r == nil || r.sql == nil {
		return errors.New("seedance task binding repository is unavailable")
	}
	if binding == nil {
		return errors.New("seedance task binding is required")
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO fflink_video_job_bindings (
			user_id, api_key_id, group_id, account_id, job_id, model
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, api_key_id, group_id, job_id)
		DO UPDATE SET
			account_id = EXCLUDED.account_id,
			model = EXCLUDED.model
	`, binding.UserID, binding.APIKeyID, binding.GroupID, binding.AccountID, strings.TrimSpace(binding.JobID), strings.TrimSpace(binding.Model))
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
		SELECT user_id, api_key_id, group_id, account_id, job_id, model, created_at
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
	if err := rows.Scan(
		&binding.UserID,
		&binding.APIKeyID,
		&binding.GroupID,
		&binding.AccountID,
		&binding.JobID,
		&binding.Model,
		&binding.CreatedAt,
	); err != nil {
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
		SELECT user_id, api_key_id, group_id, account_id, job_id, model, created_at
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
		if err := rows.Scan(
			&binding.UserID,
			&binding.APIKeyID,
			&binding.GroupID,
			&binding.AccountID,
			&binding.JobID,
			&binding.Model,
			&binding.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan seedance task binding: %w", err)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list seedance task bindings: %w", err)
	}
	return bindings, nil
}
