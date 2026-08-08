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

type creazyCanvasWorkRepository struct {
	db *sql.DB
}

func NewCreazyCanvasWorkRepository(db *sql.DB) service.CreazyCanvasWorkRepository {
	return &creazyCanvasWorkRepository{db: db}
}

func (r *creazyCanvasWorkRepository) Create(ctx context.Context, work *service.CreazyCanvasWork) error {
	if work == nil {
		return fmt.Errorf("work is nil")
	}
	params, err := marshalAgentJSON(work.ParamsJSON)
	if err != nil {
		return err
	}
	if work.ExpiresAt.IsZero() {
		work.ExpiresAt = time.Now().Add(3 * 24 * time.Hour)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO creazy_canvas_works (
			user_id, api_key_id, group_id, kind, public_model, status, prompt, params_json,
			gateway_type, gateway_remote_id, object_key, storage_provider, bucket, object_url,
			preview_url, mime_type, size_bytes, error_message, expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19
		)
		RETURNING id, created_at, updated_at
	`,
		work.UserID,
		work.APIKeyID,
		work.GroupID,
		work.Kind,
		work.PublicModel,
		work.Status,
		work.Prompt,
		params,
		work.GatewayType,
		work.GatewayRemoteID,
		work.ObjectKey,
		work.StorageProvider,
		work.Bucket,
		work.ObjectURL,
		work.PreviewURL,
		work.MimeType,
		work.SizeBytes,
		work.ErrorMessage,
		work.ExpiresAt,
	).Scan(&work.ID, &work.CreatedAt, &work.UpdatedAt)
	return err
}

func (r *creazyCanvasWorkRepository) GetByIDForUser(ctx context.Context, id, userID int64) (*service.CreazyCanvasWork, error) {
	rows, err := r.db.QueryContext(ctx, creazyCanvasWorkSelectSQL()+`
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrCreazyCanvasWorkNotFound
	}
	work, err := scanCreazyCanvasWork(rows)
	if err != nil {
		return nil, err
	}
	return work, rows.Err()
}

func (r *creazyCanvasWorkRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.CreazyCanvasWorkListFilters) ([]service.CreazyCanvasWork, *pagination.PaginationResult, error) {
	where, args := buildCreazyCanvasWorkWhere(userID, filters)
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM creazy_canvas_works `+where, args, &total); err != nil {
		return nil, nil, err
	}
	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.db.QueryContext(ctx,
		creazyCanvasWorkSelectSQL()+" "+where+creazyCanvasWorkOrderClause(params)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos),
		args...,
	)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.CreazyCanvasWork, 0)
	for rows.Next() {
		work, err := scanCreazyCanvasWork(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *work)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *creazyCanvasWorkRepository) SoftDelete(ctx context.Context, id, userID int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE creazy_canvas_works
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrCreazyCanvasWorkNotFound
	}
	return nil
}

func (r *creazyCanvasWorkRepository) UpdateContentMeta(ctx context.Context, work *service.CreazyCanvasWork) error {
	if work == nil {
		return fmt.Errorf("work is nil")
	}
	params, err := marshalAgentJSON(work.ParamsJSON)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE creazy_canvas_works
		SET object_key = $3,
		    storage_provider = $4,
		    bucket = $5,
		    object_url = $6,
		    preview_url = $7,
		    mime_type = $8,
		    size_bytes = $9,
		    status = $10,
		    error_message = $11,
		    gateway_type = $12,
		    gateway_remote_id = $13,
		    params_json = $14,
		    public_model = $15,
		    prompt = $16,
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`,
		work.ID,
		work.UserID,
		work.ObjectKey,
		work.StorageProvider,
		work.Bucket,
		work.ObjectURL,
		work.PreviewURL,
		work.MimeType,
		work.SizeBytes,
		work.Status,
		work.ErrorMessage,
		work.GatewayType,
		work.GatewayRemoteID,
		params,
		work.PublicModel,
		work.Prompt,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrCreazyCanvasWorkNotFound
	}
	return nil
}

func creazyCanvasWorkSelectSQL() string {
	return `
		SELECT id, user_id, api_key_id, group_id, kind, public_model, status, prompt, params_json,
		       COALESCE(gateway_type, ''), COALESCE(gateway_remote_id, ''),
		       COALESCE(object_key, ''), COALESCE(storage_provider, ''), COALESCE(bucket, ''),
		       COALESCE(object_url, ''), COALESCE(preview_url, ''), COALESCE(mime_type, ''),
		       size_bytes, COALESCE(error_message, ''), expires_at, created_at, updated_at, deleted_at
		FROM creazy_canvas_works
	`
}

func buildCreazyCanvasWorkWhere(userID int64, filters service.CreazyCanvasWorkListFilters) (string, []any) {
	conditions := []string{"user_id = $1", "deleted_at IS NULL"}
	args := []any{userID}
	if kind := strings.TrimSpace(filters.Kind); kind != "" {
		args = append(args, kind)
		conditions = append(conditions, fmt.Sprintf("kind = $%d", len(args)))
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if filters.APIKeyID != nil && *filters.APIKeyID > 0 {
		args = append(args, *filters.APIKeyID)
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func creazyCanvasWorkOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "created_at"
	switch sortBy {
	case "id":
		field = "id"
	case "status":
		field = "status"
	case "kind":
		field = "kind"
	case "updated_at":
		field = "updated_at"
	case "expires_at":
		field = "expires_at"
	case "created_at":
		field = "created_at"
	}
	if sortOrder == pagination.SortOrderAsc {
		return " ORDER BY " + field + " ASC, id ASC"
	}
	return " ORDER BY " + field + " DESC, id DESC"
}

type creazyCanvasWorkScanner interface {
	Scan(dest ...any) error
}

func scanCreazyCanvasWork(scanner creazyCanvasWorkScanner) (*service.CreazyCanvasWork, error) {
	work := &service.CreazyCanvasWork{}
	var (
		groupID   sql.NullInt64
		params    []byte
		deletedAt sql.NullTime
	)
	if err := scanner.Scan(
		&work.ID,
		&work.UserID,
		&work.APIKeyID,
		&groupID,
		&work.Kind,
		&work.PublicModel,
		&work.Status,
		&work.Prompt,
		&params,
		&work.GatewayType,
		&work.GatewayRemoteID,
		&work.ObjectKey,
		&work.StorageProvider,
		&work.Bucket,
		&work.ObjectURL,
		&work.PreviewURL,
		&work.MimeType,
		&work.SizeBytes,
		&work.ErrorMessage,
		&work.ExpiresAt,
		&work.CreatedAt,
		&work.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	if groupID.Valid {
		id := groupID.Int64
		work.GroupID = &id
	}
	work.ParamsJSON = unmarshalAgentJSON(params)
	if deletedAt.Valid {
		t := deletedAt.Time
		work.DeletedAt = &t
	}
	return work, nil
}
