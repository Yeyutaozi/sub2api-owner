package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type tokenRewardRepository struct {
	db *sql.DB
}

func NewTokenRewardRepository(db *sql.DB) service.TokenRewardRepository {
	return &tokenRewardRepository{db: db}
}

func (r *tokenRewardRepository) GetCycleTokens(ctx context.Context, userID int64, start, end time.Time) (int64, error) {
	return r.getCycleTokens(ctx, r.db, userID, start, end)
}

func (r *tokenRewardRepository) ListClaims(ctx context.Context, userID int64, cycleType string, cycleStart time.Time) ([]service.TokenRewardClaim, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, tier_id, cycle_type, cycle_start, cycle_end,
		       required_tokens, token_unit, reward_balance, token_snapshot, claimed_at
		FROM token_reward_claims
		WHERE user_id = $1 AND cycle_type = $2 AND cycle_start = $3
		ORDER BY required_tokens ASC, tier_id ASC
	`, userID, cycleType, cycleStart)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	claims := make([]service.TokenRewardClaim, 0)
	for rows.Next() {
		claim, scanErr := scanTokenRewardClaim(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claims, nil
}

func (r *tokenRewardRepository) ListClaimHistory(ctx context.Context, userID int64, page, pageSize int) ([]service.TokenRewardClaim, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, service.ErrTokenRewardUnavailable
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := scanSingleRow(ctx, r.db, `
		SELECT COUNT(*)
		FROM token_reward_claims
		WHERE user_id = $1
	`, []any{userID}, &total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, tier_id, cycle_type, cycle_start, cycle_end,
		       required_tokens, token_unit, reward_balance, token_snapshot, claimed_at
		FROM token_reward_claims
		WHERE user_id = $1
		ORDER BY claimed_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	claims := make([]service.TokenRewardClaim, 0)
	for rows.Next() {
		claim, scanErr := scanTokenRewardClaim(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return claims, total, nil
}

func (r *tokenRewardRepository) Claim(ctx context.Context, input service.TokenRewardClaimInput) (*service.TokenRewardClaimResult, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrTokenRewardUnavailable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	tokens, err := r.getCycleTokens(ctx, tx, input.UserID, input.CycleStart, input.CycleEnd)
	if err != nil {
		return nil, err
	}
	if tokens < input.RequiredTokens {
		return nil, service.ErrTokenRewardNotEligible
	}

	var existingClaimID int64
	if err := scanSingleRow(ctx, tx, `
		SELECT id
		FROM token_reward_claims
		WHERE user_id = $1
		  AND (tier_id = $2 OR required_tokens = $3)
		  AND cycle_start < $4
		  AND cycle_end > $5
		LIMIT 1
	`, []any{input.UserID, input.TierID, input.RequiredTokens, input.CycleEnd, input.CycleStart}, &existingClaimID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("check token reward duplicate claim: %w", err)
		}
	} else {
		return nil, service.ErrTokenRewardAlreadyClaimed
	}

	claim := service.TokenRewardClaim{
		UserID:         input.UserID,
		TierID:         input.TierID,
		CycleType:      input.CycleType,
		CycleStart:     input.CycleStart,
		CycleEnd:       input.CycleEnd,
		RequiredTokens: input.RequiredTokens,
		TokenUnit:      input.TokenUnit,
		RewardBalance:  input.RewardBalance,
		TokenSnapshot:  tokens,
	}
	if err := scanSingleRow(ctx, tx, `
		INSERT INTO token_reward_claims (
			user_id, tier_id, cycle_type, cycle_start, cycle_end,
			required_tokens, token_unit, reward_balance, token_snapshot
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, claimed_at
	`, []any{
		claim.UserID,
		claim.TierID,
		claim.CycleType,
		claim.CycleStart,
		claim.CycleEnd,
		claim.RequiredTokens,
		claim.TokenUnit,
		claim.RewardBalance,
		claim.TokenSnapshot,
	}, &claim.ID, &claim.ClaimedAt); err != nil {
		if isTokenRewardUniqueViolation(err) {
			return nil, service.ErrTokenRewardAlreadyClaimed
		}
		return nil, fmt.Errorf("insert token reward claim: %w", err)
	}

	var newBalance float64
	if err := scanSingleRow(ctx, tx, `
		UPDATE users
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance
	`, []any{claim.RewardBalance, claim.UserID}, &newBalance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrUserNotFound
		}
		return nil, fmt.Errorf("update token reward balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return &service.TokenRewardClaimResult{
		Claim:      claim,
		NewBalance: newBalance,
	}, nil
}

func (r *tokenRewardRepository) getCycleTokens(ctx context.Context, q sqlQueryer, userID int64, start, end time.Time) (int64, error) {
	var tokens int64
	if err := scanSingleRow(ctx, q, `
		SELECT COALESCE(SUM(
			COALESCE(input_tokens, 0) +
			COALESCE(output_tokens, 0) +
			COALESCE(cache_creation_tokens, 0) +
			COALESCE(cache_read_tokens, 0)
		), 0)::BIGINT
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at < $3
	`, []any{userID, start, end}, &tokens); err != nil {
		return 0, err
	}
	return tokens, nil
}

func scanTokenRewardClaim(scanner interface{ Scan(...any) error }) (service.TokenRewardClaim, error) {
	var claim service.TokenRewardClaim
	err := scanner.Scan(
		&claim.ID,
		&claim.UserID,
		&claim.TierID,
		&claim.CycleType,
		&claim.CycleStart,
		&claim.CycleEnd,
		&claim.RequiredTokens,
		&claim.TokenUnit,
		&claim.RewardBalance,
		&claim.TokenSnapshot,
		&claim.ClaimedAt,
	)
	return claim, err
}

func isTokenRewardUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && string(pqErr.Code) == "23505"
}
