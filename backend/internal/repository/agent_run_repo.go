package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type agentRunRepository struct {
	db *sql.DB
}

type agentRunSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewAgentRunRepository(db *sql.DB) service.AgentRunRepository {
	return &agentRunRepository{db: db}
}

func (r *agentRunRepository) CreateRun(ctx context.Context, run *service.AgentRun) error {
	return createAgentRun(ctx, r.db, run)
}

func (r *agentRunRepository) CreateRunWithKeyBindings(ctx context.Context, run *service.AgentRun, bindings []service.AgentRunKeyBinding) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := createAgentRun(ctx, tx, run); err != nil {
		return err
	}
	for i := range bindings {
		bindings[i].RunID = run.ID
		if bindings[i].UserID <= 0 {
			bindings[i].UserID = run.UserID
		}
		if err := createAgentRunKeyBinding(ctx, tx, &bindings[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func createAgentRun(ctx context.Context, q agentRunSQLExecutor, run *service.AgentRun) error {
	inputSummary, err := marshalAgentJSON(run.InputSummaryJSON)
	if err != nil {
		return err
	}
	outputSummary, err := marshalAgentJSON(run.OutputSummaryJSON)
	if err != nil {
		return err
	}
	usage, err := marshalAgentJSON(run.UsageJSON)
	if err != nil {
		return err
	}
	err = q.QueryRowContext(ctx, `
		INSERT INTO agent_runs (
			app_id, app_version_id, user_id, api_key_id, worker_host_id, run_token_hash, status,
			input_ref_url, input_summary_json, output_ref_url, output_summary_json,
			error_code, error_message, usage_json, started_at, completed_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, NULLIF($10, ''), $11,
		        NULLIF($12, ''), NULLIF($13, ''), $14, $15, $16, $17)
		RETURNING id, created_at, updated_at
	`,
		run.AppID,
		run.AppVersionID,
		run.UserID,
		run.APIKeyID,
		run.WorkerHostID,
		run.RunTokenHash,
		run.Status,
		run.InputRefURL,
		inputSummary,
		run.OutputRefURL,
		outputSummary,
		run.ErrorCode,
		run.ErrorMessage,
		usage,
		run.StartedAt,
		run.CompletedAt,
		run.ExpiresAt,
	).Scan(&run.ID, &run.CreatedAt, &run.UpdatedAt)
	return err
}

func createAgentRunKeyBinding(ctx context.Context, q agentRunSQLExecutor, binding *service.AgentRunKeyBinding) error {
	err := q.QueryRowContext(ctx, `
		INSERT INTO agent_run_key_bindings (
			run_id, user_id, api_key_id, policy_key, node_id, node_role,
			model_group_id, capability, is_default
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, NULLIF($8, ''), $9)
		RETURNING id, created_at
	`,
		binding.RunID,
		binding.UserID,
		binding.APIKeyID,
		binding.PolicyKey,
		binding.NodeID,
		binding.Role,
		binding.ModelGroupID,
		binding.Capability,
		binding.IsDefault,
	).Scan(&binding.ID, &binding.CreatedAt)
	return err
}

func (r *agentRunRepository) GetRunByID(ctx context.Context, id int64) (*service.AgentRun, error) {
	rows, err := r.db.QueryContext(ctx, agentRunSelectSQL()+` WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentRunNotFound
	}
	run, err := scanAgentRun(rows)
	if err != nil {
		return nil, err
	}
	return run, rows.Err()
}

func (r *agentRunRepository) GetRunByIDForUser(ctx context.Context, id, userID int64) (*service.AgentRun, error) {
	rows, err := r.db.QueryContext(ctx, agentRunSelectSQL()+` WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentRunNotFound
	}
	run, err := scanAgentRun(rows)
	if err != nil {
		return nil, err
	}
	return run, rows.Err()
}

func (r *agentRunRepository) ListRunsByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.AgentRunListFilters) ([]service.AgentRun, *pagination.PaginationResult, error) {
	where, args := buildAgentRunWhere(userID, filters)
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM agent_runs `+where, args, &total); err != nil {
		return nil, nil, err
	}

	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.db.QueryContext(ctx, agentRunSelectSQL()+" "+where+agentRunOrderClause(params)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos), args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentRun, 0)
	for rows.Next() {
		run, err := scanAgentRun(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *agentRunRepository) ListRunKeyBindings(ctx context.Context, runID int64) ([]service.AgentRunKeyBinding, error) {
	rows, err := r.db.QueryContext(ctx, agentRunKeyBindingSelectSQL()+`
		WHERE run_id = $1
		ORDER BY is_default ASC, id ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentRunKeyBinding, 0)
	for rows.Next() {
		binding, err := scanAgentRunKeyBinding(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *binding)
	}
	return items, rows.Err()
}

func (r *agentRunRepository) CreateInputAsset(ctx context.Context, asset *service.AgentInputAsset) error {
	metadata, err := marshalAgentJSON(asset.MetadataJSON)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO agent_input_assets (
			run_id, user_id, app_id, field_name, asset_type, asset_role, name, mime_type,
			storage_provider, bucket, object_key, object_url, size_bytes, sha256, metadata_json, expires_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, NULLIF($8, ''),
		        $9, NULLIF($10, ''), $11, $12, $13, NULLIF($14, ''), $15, $16)
		RETURNING id, created_at
	`,
		asset.RunID,
		asset.UserID,
		asset.AppID,
		asset.FieldName,
		asset.AssetType,
		asset.AssetRole,
		asset.Name,
		asset.MimeType,
		asset.StorageProvider,
		asset.Bucket,
		asset.ObjectKey,
		asset.ObjectURL,
		asset.SizeBytes,
		asset.SHA256,
		metadata,
		asset.ExpiresAt,
	).Scan(&asset.ID, &asset.CreatedAt)
	return err
}

func (r *agentRunRepository) ListInputAssetsByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.AgentInputAssetListFilters) ([]service.AgentInputAsset, *pagination.PaginationResult, error) {
	where, args := buildAgentInputAssetWhere(userID, filters)
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM agent_input_assets `+where, args, &total); err != nil {
		return nil, nil, err
	}

	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.db.QueryContext(ctx, agentInputAssetSelectSQL()+" "+where+agentInputAssetOrderClause(params)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos), args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentInputAsset, 0)
	for rows.Next() {
		asset, err := scanAgentInputAsset(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *asset)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *agentRunRepository) ListInputAssetsByIDsForUser(ctx context.Context, userID int64, ids []int64) ([]service.AgentInputAsset, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := []any{userID}
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	if len(placeholders) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, agentInputAssetSelectSQL()+`
		WHERE user_id = $1
		  AND deleted_at IS NULL
		  AND id IN (`+strings.Join(placeholders, ", ")+`)
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentInputAsset, 0)
	for rows.Next() {
		asset, err := scanAgentInputAsset(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *asset)
	}
	return items, rows.Err()
}

func (r *agentRunRepository) GetInputAssetByID(ctx context.Context, assetID int64) (*service.AgentInputAsset, error) {
	rows, err := r.db.QueryContext(ctx, agentInputAssetSelectSQL()+`
		WHERE id = $1 AND deleted_at IS NULL
	`, assetID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentInputAssetNotFound
	}
	asset, err := scanAgentInputAsset(rows)
	if err != nil {
		return nil, err
	}
	return asset, rows.Err()
}

func (r *agentRunRepository) MarkRunning(ctx context.Context, id int64, startedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = $1,
		    started_at = COALESCE(started_at, $2),
		    updated_at = NOW()
		WHERE id = $3
		  AND status IN ($4, $5)
	`, service.AgentRunStatusRunning, startedAt, id, service.AgentRunStatusQueued, service.AgentRunStatusRunning)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentRunNotFound
	}
	return nil
}

func (r *agentRunRepository) MarkFailed(ctx context.Context, id int64, code, message string, completedAt time.Time) error {
	return r.markTerminal(ctx, id, service.AgentRunStatusFailed, code, message, completedAt)
}

func (r *agentRunRepository) MarkTimeout(ctx context.Context, id int64, completedAt time.Time) error {
	return r.markTerminal(ctx, id, service.AgentRunStatusTimeout, "WORKER_TIMEOUT", "worker run timeout", completedAt)
}

func (r *agentRunRepository) MarkCanceled(ctx context.Context, id, userID int64, code, message string, completedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = $1,
		    error_code = NULLIF($2, ''),
		    error_message = NULLIF($3, ''),
		    completed_at = COALESCE(completed_at, $4),
		    updated_at = NOW()
		WHERE id = $5
		  AND user_id = $6
		  AND status IN ($7, $8)
	`, service.AgentRunStatusCanceled, code, message, completedAt, id, userID, service.AgentRunStatusQueued, service.AgentRunStatusRunning)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentRunNotFound
	}
	return nil
}

func (r *agentRunRepository) markTerminal(ctx context.Context, id int64, status, code, message string, completedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = $1,
		    error_code = NULLIF($2, ''),
		    error_message = NULLIF($3, ''),
		    completed_at = COALESCE(completed_at, $4),
		    updated_at = NOW()
		WHERE id = $5
		  AND status NOT IN ($6, $7)
	`, status, code, message, completedAt, id, service.AgentRunStatusSucceeded, service.AgentRunStatusCanceled)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentRunNotFound
	}
	return nil
}

func (r *agentRunRepository) UpdateFromCallback(ctx context.Context, run *service.AgentRun) error {
	outputSummary, err := marshalAgentJSON(run.OutputSummaryJSON)
	if err != nil {
		return err
	}
	usage, err := marshalAgentJSON(run.UsageJSON)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = $1,
		    output_ref_url = NULLIF($2, ''),
		    output_summary_json = $3,
		    error_code = NULLIF($4, ''),
		    error_message = NULLIF($5, ''),
		    usage_json = $6,
		    started_at = COALESCE(started_at, $7),
		    completed_at = $8,
		    updated_at = NOW()
		WHERE id = $9
		  AND (
		    status NOT IN ($10, $11, $12, $13)
		    OR status = $1
		  )
	`, run.Status, run.OutputRefURL, outputSummary, run.ErrorCode, run.ErrorMessage, usage, run.StartedAt, run.CompletedAt, run.ID,
		service.AgentRunStatusSucceeded,
		service.AgentRunStatusFailed,
		service.AgentRunStatusCanceled,
		service.AgentRunStatusTimeout,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentRunNotFound
	}
	return nil
}

func (r *agentRunRepository) CreateRunEvent(ctx context.Context, event *service.AgentRunEvent) error {
	if event == nil {
		return nil
	}
	metadata, err := marshalAgentJSON(event.MetadataJSON)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO agent_run_events (
			run_id, user_id, event_type, status, node_id, node_role,
			message, progress, metadata_json
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
		        NULLIF($7, ''), $8, $9)
		RETURNING id, created_at
	`,
		event.RunID,
		event.UserID,
		event.EventType,
		event.Status,
		event.NodeID,
		event.Role,
		event.Message,
		event.Progress,
		metadata,
	).Scan(&event.ID, &event.CreatedAt)
	return err
}

func (r *agentRunRepository) ListRunEventsByRunForUser(ctx context.Context, runID, userID int64, params pagination.PaginationParams) ([]service.AgentRunEvent, *pagination.PaginationResult, error) {
	args := []any{runID, userID}
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM agent_run_events WHERE run_id = $1 AND user_id = $2`, args, &total); err != nil {
		return nil, nil, err
	}

	args = append(args, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, agentRunEventSelectSQL()+`
		WHERE run_id = $1 AND user_id = $2
		ORDER BY created_at ASC, id ASC
		LIMIT $3 OFFSET $4
	`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentRunEvent, 0)
	for rows.Next() {
		event, err := scanAgentRunEvent(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *agentRunRepository) CreateArtifact(ctx context.Context, artifact *service.AgentArtifact) error {
	metadata, err := marshalAgentJSON(artifact.MetadataJSON)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO agent_artifacts (
			run_id, user_id, artifact_type, name, mime_type, storage_provider, bucket, object_key,
			object_url, size_bytes, sha256, metadata_json, expires_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8,
		        $9, $10, NULLIF($11, ''), $12, $13)
		RETURNING id, created_at
	`,
		artifact.RunID,
		artifact.UserID,
		artifact.ArtifactType,
		artifact.Name,
		artifact.MimeType,
		artifact.StorageProvider,
		artifact.Bucket,
		artifact.ObjectKey,
		artifact.ObjectURL,
		artifact.SizeBytes,
		artifact.SHA256,
		metadata,
		artifact.ExpiresAt,
	).Scan(&artifact.ID, &artifact.CreatedAt)
	return err
}

func (r *agentRunRepository) ListArtifactsByRun(ctx context.Context, runID int64) ([]service.AgentArtifact, error) {
	rows, err := r.db.QueryContext(ctx, agentArtifactSelectSQL()+`
		WHERE run_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC, id ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentArtifact, 0)
	for rows.Next() {
		artifact, err := scanAgentArtifact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *artifact)
	}
	return items, rows.Err()
}

func (r *agentRunRepository) GetArtifactByID(ctx context.Context, artifactID int64) (*service.AgentArtifact, error) {
	rows, err := r.db.QueryContext(ctx, agentArtifactSelectSQL()+`
		WHERE id = $1 AND deleted_at IS NULL
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentArtifactNotFound
	}
	artifact, err := scanAgentArtifact(rows)
	if err != nil {
		return nil, err
	}
	return artifact, rows.Err()
}

func (r *agentRunRepository) ListExpiredArtifacts(ctx context.Context, now time.Time, limit int) ([]service.AgentCleanupObjectRef, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, storage_provider, bucket, object_key, object_url
		FROM agent_artifacts
		WHERE deleted_at IS NULL
		  AND expires_at IS NOT NULL
		  AND expires_at <= $1
		ORDER BY expires_at ASC, id ASC
		LIMIT $2
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAgentCleanupObjectRefs(rows)
}

func (r *agentRunRepository) MarkArtifactsDeleted(ctx context.Context, ids []int64, deletedAt time.Time) (int64, error) {
	return r.markAgentRowsDeleted(ctx, "agent_artifacts", ids, deletedAt)
}

func (r *agentRunRepository) ListExpiredInputAssets(ctx context.Context, now time.Time, limit int) ([]service.AgentCleanupObjectRef, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, storage_provider, bucket, object_key, object_url
		FROM agent_input_assets
		WHERE deleted_at IS NULL
		  AND expires_at IS NOT NULL
		  AND expires_at <= $1
		ORDER BY expires_at ASC, id ASC
		LIMIT $2
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAgentCleanupObjectRefs(rows)
}

func (r *agentRunRepository) MarkInputAssetsDeleted(ctx context.Context, ids []int64, deletedAt time.Time) (int64, error) {
	return r.markAgentRowsDeleted(ctx, "agent_input_assets", ids, deletedAt)
}

func (r *agentRunRepository) markAgentRowsDeleted(ctx context.Context, table string, ids []int64, deletedAt time.Time) (int64, error) {
	ids = uniquePositiveAgentIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	switch table {
	case "agent_artifacts", "agent_input_assets":
	default:
		return 0, fmt.Errorf("unsupported agent cleanup table %q", table)
	}
	args := []any{deletedAt}
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE `+table+`
		SET deleted_at = $1
		WHERE deleted_at IS NULL
		  AND id IN (`+strings.Join(placeholders, ", ")+`)
	`, args...)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

func agentRunSelectSQL() string {
	return `
		SELECT id, app_id, app_version_id, user_id, api_key_id, worker_host_id, run_token_hash,
		       status, input_ref_url, input_summary_json, output_ref_url, output_summary_json,
		       error_code, error_message, usage_json, started_at, completed_at, expires_at,
		       created_at, updated_at
		FROM agent_runs`
}

func buildAgentRunWhere(userID int64, filters service.AgentRunListFilters) (string, []any) {
	conditions := []string{"user_id = $1"}
	args := []any{userID}
	if filters.AppID != nil && *filters.AppID > 0 {
		args = append(args, *filters.AppID)
		conditions = append(conditions, fmt.Sprintf("app_id = $%d", len(args)))
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func agentRunOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "created_at"
	switch sortBy {
	case "id":
		field = "id"
	case "status":
		field = "status"
	case "created_at":
		field = "created_at"
	case "updated_at":
		field = "updated_at"
	}
	if sortOrder == pagination.SortOrderAsc {
		return " ORDER BY " + field + " ASC, id ASC"
	}
	return " ORDER BY " + field + " DESC, id DESC"
}

func buildAgentInputAssetWhere(userID int64, filters service.AgentInputAssetListFilters) (string, []any) {
	conditions := []string{"user_id = $1", "deleted_at IS NULL"}
	args := []any{userID}
	if filters.AppID != nil && *filters.AppID > 0 {
		args = append(args, *filters.AppID)
		conditions = append(conditions, fmt.Sprintf("(app_id IS NULL OR app_id = $%d)", len(args)))
	}
	if assetType := strings.TrimSpace(filters.AssetType); assetType != "" {
		args = append(args, assetType)
		conditions = append(conditions, fmt.Sprintf("asset_type = $%d", len(args)))
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		conditions = append(conditions, fmt.Sprintf("(LOWER(name) LIKE $%d OR LOWER(field_name) LIKE $%d OR LOWER(asset_role) LIKE $%d)", len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func agentInputAssetOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "created_at"
	switch sortBy {
	case "id":
		field = "id"
	case "name":
		field = "name"
	case "asset_type":
		field = "asset_type"
	case "created_at":
		field = "created_at"
	}
	if sortOrder == pagination.SortOrderAsc {
		return " ORDER BY " + field + " ASC, id ASC"
	}
	return " ORDER BY " + field + " DESC, id DESC"
}

func agentRunKeyBindingSelectSQL() string {
	return `
		SELECT id, run_id, user_id, api_key_id, policy_key, node_id, node_role,
		       model_group_id, capability, is_default, created_at
		FROM agent_run_key_bindings`
}

func scanAgentRunKeyBinding(scanner agentWorkerHostScanner) (*service.AgentRunKeyBinding, error) {
	binding := &service.AgentRunKeyBinding{}
	var (
		nodeID       sql.NullString
		role         sql.NullString
		modelGroupID sql.NullInt64
		capability   sql.NullString
	)
	if err := scanner.Scan(
		&binding.ID,
		&binding.RunID,
		&binding.UserID,
		&binding.APIKeyID,
		&binding.PolicyKey,
		&nodeID,
		&role,
		&modelGroupID,
		&capability,
		&binding.IsDefault,
		&binding.CreatedAt,
	); err != nil {
		return nil, err
	}
	binding.NodeID = nodeID.String
	binding.Role = role.String
	if modelGroupID.Valid {
		binding.ModelGroupID = &modelGroupID.Int64
	}
	binding.Capability = capability.String
	return binding, nil
}

func scanAgentRun(scanner agentWorkerHostScanner) (*service.AgentRun, error) {
	run := &service.AgentRun{}
	var (
		workerHostID  sql.NullInt64
		inputRefURL   sql.NullString
		outputRefURL  sql.NullString
		inputSummary  []byte
		outputSummary []byte
		errorCode     sql.NullString
		errorMessage  sql.NullString
		usage         []byte
		startedAt     sql.NullTime
		completedAt   sql.NullTime
		expiresAt     sql.NullTime
	)
	if err := scanner.Scan(
		&run.ID,
		&run.AppID,
		&run.AppVersionID,
		&run.UserID,
		&run.APIKeyID,
		&workerHostID,
		&run.RunTokenHash,
		&run.Status,
		&inputRefURL,
		&inputSummary,
		&outputRefURL,
		&outputSummary,
		&errorCode,
		&errorMessage,
		&usage,
		&startedAt,
		&completedAt,
		&expiresAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if workerHostID.Valid {
		run.WorkerHostID = &workerHostID.Int64
	}
	run.InputRefURL = inputRefURL.String
	run.InputSummaryJSON = unmarshalAgentJSON(inputSummary)
	run.OutputRefURL = outputRefURL.String
	run.OutputSummaryJSON = unmarshalAgentJSON(outputSummary)
	run.ErrorCode = errorCode.String
	run.ErrorMessage = errorMessage.String
	run.UsageJSON = unmarshalAgentJSON(usage)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	if expiresAt.Valid {
		run.ExpiresAt = &expiresAt.Time
	}
	return run, nil
}

func agentInputAssetSelectSQL() string {
	return `
		SELECT id, run_id, user_id, app_id, field_name, asset_type, asset_role, name, mime_type,
		       storage_provider, bucket, object_key, object_url, size_bytes, sha256, metadata_json,
		       expires_at, created_at, deleted_at
		FROM agent_input_assets`
}

func scanAgentInputAsset(scanner agentWorkerHostScanner) (*service.AgentInputAsset, error) {
	asset := &service.AgentInputAsset{}
	var (
		runID     sql.NullInt64
		appID     sql.NullInt64
		fieldName sql.NullString
		assetRole sql.NullString
		mimeType  sql.NullString
		bucket    sql.NullString
		sha256    sql.NullString
		metadata  []byte
		expiresAt sql.NullTime
		deletedAt sql.NullTime
	)
	if err := scanner.Scan(
		&asset.ID,
		&runID,
		&asset.UserID,
		&appID,
		&fieldName,
		&asset.AssetType,
		&assetRole,
		&asset.Name,
		&mimeType,
		&asset.StorageProvider,
		&bucket,
		&asset.ObjectKey,
		&asset.ObjectURL,
		&asset.SizeBytes,
		&sha256,
		&metadata,
		&expiresAt,
		&asset.CreatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	if runID.Valid {
		asset.RunID = &runID.Int64
	}
	if appID.Valid {
		asset.AppID = &appID.Int64
	}
	asset.FieldName = fieldName.String
	asset.AssetRole = assetRole.String
	asset.MimeType = mimeType.String
	asset.Bucket = bucket.String
	asset.SHA256 = sha256.String
	asset.MetadataJSON = unmarshalAgentJSON(metadata)
	if expiresAt.Valid {
		asset.ExpiresAt = &expiresAt.Time
	}
	if deletedAt.Valid {
		asset.DeletedAt = &deletedAt.Time
	}
	return asset, nil
}

func agentArtifactSelectSQL() string {
	return `
		SELECT id, run_id, user_id, artifact_type, name, mime_type, storage_provider,
		       bucket, object_key, object_url, size_bytes, sha256, metadata_json,
		       expires_at, created_at, deleted_at
		FROM agent_artifacts`
}

func scanAgentArtifact(scanner agentWorkerHostScanner) (*service.AgentArtifact, error) {
	artifact := &service.AgentArtifact{}
	var (
		mimeType  sql.NullString
		bucket    sql.NullString
		sha256    sql.NullString
		metadata  []byte
		expiresAt sql.NullTime
		deletedAt sql.NullTime
	)
	if err := scanner.Scan(
		&artifact.ID,
		&artifact.RunID,
		&artifact.UserID,
		&artifact.ArtifactType,
		&artifact.Name,
		&mimeType,
		&artifact.StorageProvider,
		&bucket,
		&artifact.ObjectKey,
		&artifact.ObjectURL,
		&artifact.SizeBytes,
		&sha256,
		&metadata,
		&expiresAt,
		&artifact.CreatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	artifact.MimeType = mimeType.String
	artifact.Bucket = bucket.String
	artifact.SHA256 = sha256.String
	artifact.MetadataJSON = unmarshalAgentJSON(metadata)
	if expiresAt.Valid {
		artifact.ExpiresAt = &expiresAt.Time
	}
	if deletedAt.Valid {
		artifact.DeletedAt = &deletedAt.Time
	}
	return artifact, nil
}

func agentRunEventSelectSQL() string {
	return `
		SELECT id, run_id, user_id, event_type, status, node_id, node_role,
		       message, progress, metadata_json, created_at
		FROM agent_run_events`
}

func scanAgentRunEvent(scanner agentWorkerHostScanner) (*service.AgentRunEvent, error) {
	event := &service.AgentRunEvent{}
	var (
		status   sql.NullString
		nodeID   sql.NullString
		role     sql.NullString
		message  sql.NullString
		progress sql.NullFloat64
		metadata []byte
	)
	if err := scanner.Scan(
		&event.ID,
		&event.RunID,
		&event.UserID,
		&event.EventType,
		&status,
		&nodeID,
		&role,
		&message,
		&progress,
		&metadata,
		&event.CreatedAt,
	); err != nil {
		return nil, err
	}
	event.Status = status.String
	event.NodeID = nodeID.String
	event.Role = role.String
	event.Message = message.String
	if progress.Valid {
		event.Progress = &progress.Float64
	}
	event.MetadataJSON = unmarshalAgentJSON(metadata)
	return event, nil
}

func scanAgentCleanupObjectRefs(rows *sql.Rows) ([]service.AgentCleanupObjectRef, error) {
	items := make([]service.AgentCleanupObjectRef, 0)
	for rows.Next() {
		var (
			ref             service.AgentCleanupObjectRef
			storageProvider sql.NullString
			bucket          sql.NullString
			objectURL       sql.NullString
		)
		if err := rows.Scan(&ref.ID, &storageProvider, &bucket, &ref.ObjectKey, &objectURL); err != nil {
			return nil, err
		}
		ref.StorageProvider = storageProvider.String
		ref.Bucket = bucket.String
		ref.ObjectURL = objectURL.String
		items = append(items, ref)
	}
	return items, rows.Err()
}

func uniquePositiveAgentIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
