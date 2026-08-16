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

func (r *creazyCanvasWorkRepository) CreateOrUpdateAcceptedVideo(ctx context.Context, work *service.CreazyCanvasWork) error {
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
	row := r.db.QueryRowContext(ctx, `
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
		ON CONFLICT (user_id, api_key_id, gateway_type, gateway_remote_id)
			WHERE deleted_at IS NULL AND kind = 'video' AND gateway_remote_id <> ''
		DO UPDATE SET
			group_id = COALESCE(creazy_canvas_works.group_id, EXCLUDED.group_id),
			public_model = EXCLUDED.public_model,
			status = CASE
				WHEN creazy_canvas_works.status IN ('created', 'queued', 'running') THEN EXCLUDED.status
				ELSE creazy_canvas_works.status
			END,
			prompt = EXCLUDED.prompt,
			params_json = EXCLUDED.params_json,
			error_message = CASE
				WHEN creazy_canvas_works.status IN ('created', 'queued', 'running') THEN EXCLUDED.error_message
				ELSE creazy_canvas_works.error_message
			END,
			expires_at = GREATEST(creazy_canvas_works.expires_at, EXCLUDED.expires_at),
			updated_at = NOW()
		RETURNING id, user_id, api_key_id, group_id, kind, public_model, status, prompt, params_json,
		          COALESCE(gateway_type, ''), COALESCE(gateway_remote_id, ''),
		          COALESCE(object_key, ''), COALESCE(storage_provider, ''), COALESCE(bucket, ''),
		          COALESCE(object_url, ''), COALESCE(preview_url, ''), COALESCE(mime_type, ''),
		          size_bytes, COALESCE(error_message, ''), expires_at, created_at, updated_at, deleted_at
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
	)
	saved, err := scanCreazyCanvasWork(row)
	if err != nil {
		return err
	}
	*work = *saved
	return nil
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

func (r *creazyCanvasWorkRepository) ListAdminImageWorks(ctx context.Context, params pagination.PaginationParams, filters service.CreazyCanvasAdminWorkFilters) ([]service.CreazyCanvasAdminWork, *pagination.PaginationResult, error) {
	where, args := buildCreazyCanvasAdminImageWhere(filters)
	from := creazyCanvasAdminWorkFromSQL()
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) `+from+` `+where, args, &total); err != nil {
		return nil, nil, err
	}
	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.db.QueryContext(ctx,
		creazyCanvasAdminWorkSelectSQL()+" "+where+creazyCanvasAdminWorkOrderClause(params)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos),
		args...,
	)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.CreazyCanvasAdminWork, 0)
	for rows.Next() {
		work, err := scanCreazyCanvasAdminWork(rows)
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

func (r *creazyCanvasWorkRepository) GetAdminImageWork(ctx context.Context, id int64) (*service.CreazyCanvasAdminWork, error) {
	rows, err := r.db.QueryContext(ctx, creazyCanvasAdminWorkSelectSQL()+`
		WHERE w.id = $1 AND w.kind = 'image' AND w.deleted_at IS NULL
	`, id)
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
	work, err := scanCreazyCanvasAdminWork(rows)
	if err != nil {
		return nil, err
	}
	return work, rows.Err()
}

func (r *creazyCanvasWorkRepository) UpdateAdminImageWorkStatus(ctx context.Context, id int64, status, errorMessage string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE creazy_canvas_works
		SET status = $2, error_message = $3, updated_at = NOW()
		WHERE id = $1 AND kind = 'image' AND deleted_at IS NULL
	`, id, status, errorMessage)
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
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND status <> 'canceled'
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
		var status string
		if err := r.db.QueryRowContext(ctx, `
			SELECT status FROM creazy_canvas_works
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		`, work.ID, work.UserID).Scan(&status); err == nil && status == service.CreazyCanvasWorkStatusCanceled {
			return service.ErrCreazyCanvasWorkTerminated
		}
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

func creazyCanvasAdminWorkFromSQL() string {
	return `
		FROM creazy_canvas_works w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN api_keys k ON k.id = w.api_key_id
		LEFT JOIN groups g ON g.id = COALESCE(w.group_id, k.group_id)
	`
}

func creazyCanvasAdminWorkSelectSQL() string {
	return `
		SELECT w.id, w.user_id, w.api_key_id, w.group_id, w.kind, w.public_model, w.status, w.prompt, w.params_json,
		       COALESCE(w.gateway_type, ''), COALESCE(w.gateway_remote_id, ''),
		       COALESCE(w.object_key, ''), COALESCE(w.storage_provider, ''), COALESCE(w.bucket, ''),
		       COALESCE(w.object_url, ''), COALESCE(w.preview_url, ''), COALESCE(w.mime_type, ''),
		       w.size_bytes, COALESCE(w.error_message, ''), w.expires_at, w.created_at, w.updated_at, w.deleted_at,
		       COALESCE(u.email, ''), COALESCE(u.username, ''), COALESCE(k.name, ''), COALESCE(g.name, '')
	` + creazyCanvasAdminWorkFromSQL()
}

func buildCreazyCanvasAdminImageWhere(filters service.CreazyCanvasAdminWorkFilters) (string, []any) {
	conditions := []string{"w.kind = 'image'", "w.deleted_at IS NULL"}
	args := make([]any, 0, 3)
	if status := strings.TrimSpace(filters.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("w.status = $%d", len(args)))
	}
	if gatewayType := strings.TrimSpace(filters.GatewayType); gatewayType != "" {
		args = append(args, gatewayType)
		conditions = append(conditions, fmt.Sprintf("w.gateway_type = $%d", len(args)))
	}
	if filters.ActiveOnly {
		conditions = append(conditions, "w.status IN ('created', 'queued', 'running') AND w.expires_at > NOW()")
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		args = append(args, "%"+search+"%")
		pos := len(args)
		conditions = append(conditions, fmt.Sprintf(`(
			w.prompt ILIKE $%d OR w.public_model ILIKE $%d OR w.gateway_remote_id ILIKE $%d OR
			u.email ILIKE $%d OR u.username ILIKE $%d OR k.name ILIKE $%d OR g.name ILIKE $%d OR
			CAST(w.id AS TEXT) ILIKE $%d
		)`, pos, pos, pos, pos, pos, pos, pos, pos))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func creazyCanvasAdminWorkOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "w.created_at"
	switch sortBy {
	case "id":
		field = "w.id"
	case "status":
		field = "w.status"
	case "updated_at":
		field = "w.updated_at"
	case "created_at":
		field = "w.created_at"
	}
	if sortOrder == pagination.SortOrderAsc {
		return " ORDER BY " + field + " ASC, w.id ASC"
	}
	return " ORDER BY " + field + " DESC, w.id DESC"
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

func scanCreazyCanvasAdminWork(scanner creazyCanvasWorkScanner) (*service.CreazyCanvasAdminWork, error) {
	item := &service.CreazyCanvasAdminWork{}
	work := &item.CreazyCanvasWork
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
		&item.UserEmail,
		&item.Username,
		&item.APIKeyName,
		&item.GroupName,
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
	return item, nil
}
