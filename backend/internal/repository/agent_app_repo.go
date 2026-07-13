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

type agentAppRepository struct {
	db *sql.DB
}

type agentAppExecQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewAgentAppRepository(db *sql.DB) service.AgentAppRepository {
	return &agentAppRepository{db: db}
}

func (r *agentAppRepository) CreateApp(ctx context.Context, app *service.AgentApp) error {
	err := insertAgentApp(ctx, r.db, app)
	return translatePersistenceError(err, nil, service.ErrAgentAppExists)
}

func (r *agentAppRepository) CreateAppWithVersion(ctx context.Context, app *service.AgentApp, version *service.AgentAppVersion) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = insertAgentApp(ctx, tx, app); err != nil {
		return translatePersistenceError(err, nil, service.ErrAgentAppExists)
	}
	version.AppID = app.ID
	if err = insertAgentAppVersion(ctx, tx, version); err != nil {
		return translatePersistenceError(err, nil, service.ErrAgentAppVersionExists)
	}
	if version.Status == service.AgentAppStatusPublished {
		now := time.Now().UTC()
		if err = publishVersionTx(ctx, tx, app.ID, version.ID, app.UpdatedBy, now); err != nil {
			return err
		}
		app.Status = service.AgentAppStatusPublished
		app.PublishedVersionID = &version.ID
		version.PublishedAt = &now
	}

	err = tx.Commit()
	return err
}

func (r *agentAppRepository) UpdateApp(ctx context.Context, app *service.AgentApp) error {
	err := r.db.QueryRowContext(ctx, `
		UPDATE agent_apps
		SET name = $2,
			slug = $3,
			description = NULLIF($4, ''),
			icon_url = NULLIF($5, ''),
			category = NULLIF($6, ''),
			app_type = $7,
			visibility = $8,
			status = $9,
			updated_by = $10,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at
	`, app.ID, app.Name, app.Slug, app.Description, app.IconURL, app.Category, app.AppType, app.Visibility, app.Status, app.UpdatedBy).Scan(&app.UpdatedAt)
	if err == sql.ErrNoRows {
		return service.ErrAgentAppNotFound
	}
	return translatePersistenceError(err, nil, service.ErrAgentAppExists)
}

func (r *agentAppRepository) DeleteApp(ctx context.Context, id int64, updatedBy *int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_apps
		SET status = $2, deleted_at = NOW(), updated_by = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, service.AgentAppStatusArchived, updatedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrAgentAppNotFound
	}
	return nil
}

func insertAgentApp(ctx context.Context, db agentAppExecQuerier, app *service.AgentApp) error {
	return db.QueryRowContext(ctx, `
		INSERT INTO agent_apps (
			name, slug, description, icon_url, category, app_type, visibility, status, created_by, updated_by
		)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`,
		app.Name,
		app.Slug,
		app.Description,
		app.IconURL,
		app.Category,
		app.AppType,
		app.Visibility,
		app.Status,
		app.CreatedBy,
		app.UpdatedBy,
	).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)
}

func (r *agentAppRepository) GetAppByID(ctx context.Context, id int64) (*service.AgentApp, error) {
	rows, err := r.db.QueryContext(ctx, agentAppSelectSQL()+` WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentAppNotFound
	}
	app, err := scanAgentApp(rows)
	if err != nil {
		return nil, err
	}
	return app, rows.Err()
}

func (r *agentAppRepository) ListApps(ctx context.Context, params pagination.PaginationParams, filters service.AgentAppListFilters) ([]service.AgentApp, *pagination.PaginationResult, error) {
	where, args := buildAgentAppWhere(filters)
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM agent_apps `+where, args, &total); err != nil {
		return nil, nil, err
	}
	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.db.QueryContext(ctx, agentAppSelectSQL()+" "+where+agentAppOrderClause(params)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos), args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentApp, 0)
	for rows.Next() {
		app, err := scanAgentApp(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *app)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *agentAppRepository) CreateVersion(ctx context.Context, version *service.AgentAppVersion) error {
	err := insertAgentAppVersion(ctx, r.db, version)
	return translatePersistenceError(err, nil, service.ErrAgentAppVersionExists)
}

func insertAgentAppVersion(ctx context.Context, db agentAppExecQuerier, version *service.AgentAppVersion) error {
	inputSchema, err := marshalAgentJSON(version.InputSchemaJSON)
	if err != nil {
		return err
	}
	outputSchema, err := marshalAgentJSON(version.OutputSchemaJSON)
	if err != nil {
		return err
	}
	capabilities, err := marshalAgentJSON(version.CapabilitiesJSON)
	if err != nil {
		return err
	}
	defaultModelConfig, err := marshalAgentJSON(version.DefaultModelConfigJSON)
	if err != nil {
		return err
	}
	nodeModelPolicy, err := marshalAgentJSON(version.NodeModelPolicyJSON)
	if err != nil {
		return err
	}
	artifactPolicy, err := marshalAgentJSON(version.ArtifactPolicyJSON)
	if err != nil {
		return err
	}

	return db.QueryRowContext(ctx, `
		INSERT INTO agent_app_versions (
			app_id, version, status, runtime_type, worker_host_id, worker_route, worker_health_route,
			image_ref, source_ref, input_schema_json, output_schema_json, capabilities_json,
			default_model_config_json, node_model_policy_json, artifact_policy_json, changelog, created_by
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''),
		        $10, $11, $12, $13, $14, $15, NULLIF($16, ''), $17)
		RETURNING id, created_at, updated_at
	`,
		version.AppID,
		version.Version,
		version.Status,
		version.RuntimeType,
		version.WorkerHostID,
		version.WorkerRoute,
		version.WorkerHealthRoute,
		version.ImageRef,
		version.SourceRef,
		inputSchema,
		outputSchema,
		capabilities,
		defaultModelConfig,
		nodeModelPolicy,
		artifactPolicy,
		version.Changelog,
		version.CreatedBy,
	).Scan(&version.ID, &version.CreatedAt, &version.UpdatedAt)
}

func (r *agentAppRepository) GetVersionByID(ctx context.Context, id int64) (*service.AgentAppVersion, error) {
	rows, err := r.db.QueryContext(ctx, agentAppVersionSelectSQL()+`
		WHERE v.id = $1 AND v.deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentAppVersionNotFound
	}
	version, err := scanAgentAppVersion(rows)
	if err != nil {
		return nil, err
	}
	return version, rows.Err()
}

func (r *agentAppRepository) GetPublishedVersionForApp(ctx context.Context, appID, versionID int64) (*service.AgentApp, *service.AgentAppVersion, error) {
	app, err := r.GetAppByID(ctx, appID)
	if err != nil {
		return nil, nil, err
	}
	if app.Status != service.AgentAppStatusPublished || app.PublishedVersionID == nil {
		return nil, nil, service.ErrAgentAppVersionNotFound
	}
	selectedVersionID := *app.PublishedVersionID
	if versionID > 0 {
		selectedVersionID = versionID
		if *app.PublishedVersionID != versionID {
			return nil, nil, service.ErrAgentAppVersionNotFound
		}
	}
	version, err := r.GetVersionByID(ctx, selectedVersionID)
	if err != nil {
		return nil, nil, err
	}
	if version.AppID != app.ID || version.Status != service.AgentAppStatusPublished {
		return nil, nil, service.ErrAgentAppVersionNotFound
	}
	return app, version, nil
}

func (r *agentAppRepository) ListVersions(ctx context.Context, appID int64) ([]service.AgentAppVersion, error) {
	rows, err := r.db.QueryContext(ctx, agentAppVersionSelectSQL()+`
		WHERE v.app_id = $1 AND v.deleted_at IS NULL
		ORDER BY v.created_at DESC, v.id DESC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentAppVersion, 0)
	for rows.Next() {
		version, err := scanAgentAppVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *version)
	}
	return items, rows.Err()
}

func (r *agentAppRepository) PublishVersion(ctx context.Context, appID, versionID int64, updatedBy *int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var exists bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM agent_app_versions
			WHERE id = $1 AND app_id = $2 AND deleted_at IS NULL
		)
	`, versionID, appID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrAgentAppVersionNotFound
	}

	err = publishVersionTx(ctx, tx, appID, versionID, updatedBy, time.Now().UTC())
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func publishVersionTx(ctx context.Context, tx *sql.Tx, appID, versionID int64, updatedBy *int64, publishedAt time.Time) error {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM agent_app_versions
			WHERE id = $1 AND app_id = $2 AND deleted_at IS NULL
		)
	`, versionID, appID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrAgentAppVersionNotFound
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE agent_app_versions
		SET status = CASE WHEN id = $1 THEN $2 ELSE status END,
		    published_at = CASE WHEN id = $1 THEN COALESCE(published_at, $3) ELSE published_at END,
		    updated_at = NOW()
		WHERE app_id = $4 AND deleted_at IS NULL
	`, versionID, service.AgentAppStatusPublished, publishedAt, appID)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE agent_apps
		SET status = $1,
		    published_version_id = $2,
		    updated_by = $3,
		    updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
	`, service.AgentAppStatusPublished, versionID, updatedBy, appID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentAppNotFound
	}
	return nil
}

func (r *agentAppRepository) SetVersionStatus(ctx context.Context, appID, versionID int64, status string, updatedBy *int64) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE agent_app_versions
		SET status = $1,
		    updated_at = NOW()
		WHERE id = $2 AND app_id = $3 AND deleted_at IS NULL
	`, status, versionID, appID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentAppVersionNotFound
	}

	if status != service.AgentAppStatusPublished {
		_, err = tx.ExecContext(ctx, `
			UPDATE agent_apps
			SET published_version_id = NULL,
			    status = CASE WHEN status = $1 THEN $2 ELSE status END,
			    updated_by = $3,
			    updated_at = NOW()
			WHERE id = $4 AND published_version_id = $5 AND deleted_at IS NULL
		`, service.AgentAppStatusPublished, service.AgentAppStatusDraft, updatedBy, appID, versionID)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

func agentAppSelectSQL() string {
	return `
		SELECT id, name, slug, description, icon_url, category, app_type, visibility, status,
		       published_version_id, created_by, updated_by, created_at, updated_at, deleted_at
		FROM agent_apps`
}

func buildAgentAppWhere(filters service.AgentAppListFilters) (string, []any) {
	conditions := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 3)
	if status := strings.TrimSpace(filters.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if visibility := strings.TrimSpace(filters.Visibility); visibility != "" {
		args = append(args, visibility)
		conditions = append(conditions, fmt.Sprintf("visibility = $%d", len(args)))
	}
	if filters.RequirePublishedVersion {
		conditions = append(conditions, "published_version_id IS NOT NULL")
	}
	if appType := strings.TrimSpace(filters.AppType); appType != "" {
		args = append(args, appType)
		conditions = append(conditions, fmt.Sprintf("app_type = $%d", len(args)))
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		args = append(args, "%"+search+"%")
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d)", len(args), len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func agentAppOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "id"
	switch sortBy {
	case "name":
		field = "name"
	case "slug":
		field = "slug"
	case "status":
		field = "status"
	case "app_type":
		field = "app_type"
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

func scanAgentApp(scanner agentWorkerHostScanner) (*service.AgentApp, error) {
	app := &service.AgentApp{}
	var (
		description        sql.NullString
		iconURL            sql.NullString
		category           sql.NullString
		publishedVersionID sql.NullInt64
		createdBy          sql.NullInt64
		updatedBy          sql.NullInt64
		deletedAt          sql.NullTime
	)
	if err := scanner.Scan(
		&app.ID,
		&app.Name,
		&app.Slug,
		&description,
		&iconURL,
		&category,
		&app.AppType,
		&app.Visibility,
		&app.Status,
		&publishedVersionID,
		&createdBy,
		&updatedBy,
		&app.CreatedAt,
		&app.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	app.Description = description.String
	app.IconURL = iconURL.String
	app.Category = category.String
	if publishedVersionID.Valid {
		app.PublishedVersionID = &publishedVersionID.Int64
	}
	if createdBy.Valid {
		app.CreatedBy = &createdBy.Int64
	}
	if updatedBy.Valid {
		app.UpdatedBy = &updatedBy.Int64
	}
	if deletedAt.Valid {
		app.DeletedAt = &deletedAt.Time
	}
	return app, nil
}

func agentAppVersionSelectSQL() string {
	return `
		SELECT v.id, v.app_id, v.version, v.status, v.runtime_type, v.worker_host_id, v.worker_route,
		       v.worker_health_route, v.image_ref, v.source_ref, v.input_schema_json, v.output_schema_json,
		       v.capabilities_json, v.default_model_config_json, v.node_model_policy_json,
		       v.artifact_policy_json, v.changelog, v.created_by, v.published_at,
		       v.created_at, v.updated_at, v.deleted_at,
		       h.id, h.name, h.base_url, h.protocol, h.auth_type, h.secret_ref, h.health_path, h.run_path,
		       h.cancel_path, h.max_concurrency, h.timeout_seconds, h.status, h.last_health_status,
		       h.last_health_message, h.last_health_latency_ms, h.last_checked_at, h.metadata,
		       h.created_at, h.updated_at, h.deleted_at
		FROM agent_app_versions v
		LEFT JOIN agent_worker_hosts h ON h.id = v.worker_host_id`
}

func scanAgentAppVersion(scanner agentWorkerHostScanner) (*service.AgentAppVersion, error) {
	version := &service.AgentAppVersion{}
	var (
		workerHostID      sql.NullInt64
		workerRoute       sql.NullString
		workerHealthRoute sql.NullString
		imageRef          sql.NullString
		sourceRef         sql.NullString
		inputSchema       []byte
		outputSchema      []byte
		capabilities      []byte
		defaultModel      []byte
		nodePolicy        []byte
		artifactPolicy    []byte
		changelog         sql.NullString
		createdBy         sql.NullInt64
		publishedAt       sql.NullTime
		deletedAt         sql.NullTime
		host              nullableAgentWorkerHostScan
	)
	if err := scanner.Scan(
		&version.ID,
		&version.AppID,
		&version.Version,
		&version.Status,
		&version.RuntimeType,
		&workerHostID,
		&workerRoute,
		&workerHealthRoute,
		&imageRef,
		&sourceRef,
		&inputSchema,
		&outputSchema,
		&capabilities,
		&defaultModel,
		&nodePolicy,
		&artifactPolicy,
		&changelog,
		&createdBy,
		&publishedAt,
		&version.CreatedAt,
		&version.UpdatedAt,
		&deletedAt,
		&host.ID,
		&host.Name,
		&host.BaseURL,
		&host.Protocol,
		&host.AuthType,
		&host.SecretRef,
		&host.HealthPath,
		&host.RunPath,
		&host.CancelPath,
		&host.MaxConcurrency,
		&host.TimeoutSeconds,
		&host.Status,
		&host.LastHealthStatus,
		&host.LastHealthMessage,
		&host.LastHealthLatencyMS,
		&host.LastCheckedAt,
		&host.Metadata,
		&host.CreatedAt,
		&host.UpdatedAt,
		&host.DeletedAt,
	); err != nil {
		return nil, err
	}
	if workerHostID.Valid {
		version.WorkerHostID = &workerHostID.Int64
	}
	version.WorkerRoute = workerRoute.String
	version.WorkerHealthRoute = workerHealthRoute.String
	version.ImageRef = imageRef.String
	version.SourceRef = sourceRef.String
	version.InputSchemaJSON = unmarshalAgentJSON(inputSchema)
	version.OutputSchemaJSON = unmarshalAgentJSON(outputSchema)
	version.CapabilitiesJSON = unmarshalAgentJSON(capabilities)
	version.DefaultModelConfigJSON = unmarshalAgentJSON(defaultModel)
	version.NodeModelPolicyJSON = unmarshalAgentJSON(nodePolicy)
	version.ArtifactPolicyJSON = unmarshalAgentJSON(artifactPolicy)
	version.Changelog = changelog.String
	if createdBy.Valid {
		version.CreatedBy = &createdBy.Int64
	}
	if publishedAt.Valid {
		version.PublishedAt = &publishedAt.Time
	}
	if deletedAt.Valid {
		version.DeletedAt = &deletedAt.Time
	}
	if host.ID.Valid {
		version.WorkerHost = host.toService()
	}
	return version, nil
}

type nullableAgentWorkerHostScan struct {
	ID                  sql.NullInt64
	Name                sql.NullString
	BaseURL             sql.NullString
	Protocol            sql.NullString
	AuthType            sql.NullString
	SecretRef           sql.NullString
	HealthPath          sql.NullString
	RunPath             sql.NullString
	CancelPath          sql.NullString
	MaxConcurrency      sql.NullInt64
	TimeoutSeconds      sql.NullInt64
	Status              sql.NullString
	LastHealthStatus    sql.NullString
	LastHealthMessage   sql.NullString
	LastHealthLatencyMS sql.NullInt64
	LastCheckedAt       sql.NullTime
	Metadata            []byte
	CreatedAt           sql.NullTime
	UpdatedAt           sql.NullTime
	DeletedAt           sql.NullTime
}

func (h nullableAgentWorkerHostScan) toService() *service.AgentWorkerHost {
	host := &service.AgentWorkerHost{
		ID:                h.ID.Int64,
		Name:              h.Name.String,
		BaseURL:           h.BaseURL.String,
		Protocol:          h.Protocol.String,
		AuthType:          h.AuthType.String,
		SecretRef:         h.SecretRef.String,
		HealthPath:        h.HealthPath.String,
		RunPath:           h.RunPath.String,
		CancelPath:        h.CancelPath.String,
		MaxConcurrency:    int(h.MaxConcurrency.Int64),
		TimeoutSeconds:    int(h.TimeoutSeconds.Int64),
		Status:            h.Status.String,
		LastHealthStatus:  h.LastHealthStatus.String,
		LastHealthMessage: h.LastHealthMessage.String,
		Metadata:          unmarshalAgentJSON(h.Metadata),
	}
	if h.LastHealthLatencyMS.Valid {
		v := int(h.LastHealthLatencyMS.Int64)
		host.LastHealthLatencyMS = &v
	}
	if h.LastCheckedAt.Valid {
		host.LastCheckedAt = &h.LastCheckedAt.Time
	}
	if h.CreatedAt.Valid {
		host.CreatedAt = h.CreatedAt.Time
	}
	if h.UpdatedAt.Valid {
		host.UpdatedAt = h.UpdatedAt.Time
	}
	if h.DeletedAt.Valid {
		host.DeletedAt = &h.DeletedAt.Time
	}
	return host
}
