package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type agentWorkerHostRepository struct {
	db *sql.DB
}

func NewAgentWorkerHostRepository(db *sql.DB) service.AgentWorkerHostRepository {
	return &agentWorkerHostRepository{db: db}
}

func (r *agentWorkerHostRepository) Create(ctx context.Context, host *service.AgentWorkerHost) error {
	metadata, err := marshalAgentJSON(host.Metadata)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO agent_worker_hosts (
			name, base_url, protocol, auth_type, secret_ref, health_path, run_path, cancel_path,
			max_concurrency, timeout_seconds, status, metadata
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, NULLIF($8, ''), $9, $10, $11, $12)
		RETURNING id, last_health_status, created_at, updated_at
	`,
		host.Name,
		host.BaseURL,
		host.Protocol,
		host.AuthType,
		host.SecretRef,
		host.HealthPath,
		host.RunPath,
		host.CancelPath,
		host.MaxConcurrency,
		host.TimeoutSeconds,
		host.Status,
		metadata,
	).Scan(&host.ID, &host.LastHealthStatus, &host.CreatedAt, &host.UpdatedAt)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrAgentWorkerHostExists)
	}
	return nil
}

func (r *agentWorkerHostRepository) GetByID(ctx context.Context, id int64) (*service.AgentWorkerHost, error) {
	rows, err := r.db.QueryContext(ctx, agentWorkerHostSelectSQL()+` WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAgentWorkerHostNotFound
	}
	host, err := scanAgentWorkerHost(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return host, nil
}

func (r *agentWorkerHostRepository) List(ctx context.Context, params pagination.PaginationParams, filters service.AgentWorkerHostListFilters) ([]service.AgentWorkerHost, *pagination.PaginationResult, error) {
	where, args := buildAgentWorkerHostWhere(filters)
	var total int64
	if err := scanSingleRow(ctx, r.db, `SELECT COUNT(*) FROM agent_worker_hosts `+where, args, &total); err != nil {
		return nil, nil, err
	}

	args = append(args, params.Limit(), params.Offset())
	limitPos := len(args) - 1
	offsetPos := len(args)
	query := agentWorkerHostSelectSQL() + " " + where + agentWorkerHostOrderClause(params) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentWorkerHost, 0)
	for rows.Next() {
		host, err := scanAgentWorkerHost(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *host)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return items, paginationResultFromTotal(total, params), nil
}

func (r *agentWorkerHostRepository) ListAll(ctx context.Context, status string) ([]service.AgentWorkerHost, error) {
	filters := service.AgentWorkerHostListFilters{Status: status, SortBy: "name", SortOrder: pagination.SortOrderAsc}
	where, args := buildAgentWorkerHostWhere(filters)
	rows, err := r.db.QueryContext(ctx, agentWorkerHostSelectSQL()+" "+where+agentWorkerHostOrderClause(pagination.PaginationParams{SortBy: "name", SortOrder: pagination.SortOrderAsc}), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AgentWorkerHost, 0)
	for rows.Next() {
		host, err := scanAgentWorkerHost(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *host)
	}
	return items, rows.Err()
}

func (r *agentWorkerHostRepository) Update(ctx context.Context, host *service.AgentWorkerHost) error {
	metadata, err := marshalAgentJSON(host.Metadata)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		UPDATE agent_worker_hosts
		SET name = $1,
		    base_url = $2,
		    protocol = $3,
		    auth_type = $4,
		    secret_ref = NULLIF($5, ''),
		    health_path = $6,
		    run_path = $7,
		    cancel_path = NULLIF($8, ''),
		    max_concurrency = $9,
		    timeout_seconds = $10,
		    status = $11,
		    metadata = $12,
		    updated_at = NOW()
		WHERE id = $13 AND deleted_at IS NULL
		RETURNING created_at, updated_at
	`,
		host.Name,
		host.BaseURL,
		host.Protocol,
		host.AuthType,
		host.SecretRef,
		host.HealthPath,
		host.RunPath,
		host.CancelPath,
		host.MaxConcurrency,
		host.TimeoutSeconds,
		host.Status,
		metadata,
		host.ID,
	).Scan(&host.CreatedAt, &host.UpdatedAt)
	return translatePersistenceError(err, service.ErrAgentWorkerHostNotFound, service.ErrAgentWorkerHostExists)
}

func (r *agentWorkerHostRepository) UpdateHealth(ctx context.Context, id int64, status, healthStatus, message string, latencyMS *int, checkedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_worker_hosts
		SET status = $1,
		    last_health_status = $2,
		    last_health_message = NULLIF($3, ''),
		    last_health_latency_ms = $4,
		    last_checked_at = $5,
		    updated_at = NOW()
		WHERE id = $6 AND deleted_at IS NULL
	`, status, healthStatus, message, latencyMS, checkedAt, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentWorkerHostNotFound
	}
	return nil
}

func (r *agentWorkerHostRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_worker_hosts
		SET deleted_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrAgentWorkerHostNotFound
	}
	return nil
}

func agentWorkerHostSelectSQL() string {
	return `
		SELECT id, name, base_url, protocol, auth_type, secret_ref, health_path, run_path, cancel_path,
		       max_concurrency, timeout_seconds, status, last_health_status, last_health_message,
		       last_health_latency_ms, last_checked_at, metadata, created_at, updated_at, deleted_at
		FROM agent_worker_hosts`
}

func buildAgentWorkerHostWhere(filters service.AgentWorkerHostListFilters) (string, []any) {
	conditions := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 2)
	if status := strings.TrimSpace(filters.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		args = append(args, "%"+search+"%")
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR base_url ILIKE $%d)", len(args), len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func agentWorkerHostOrderClause(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	field := "id"
	switch sortBy {
	case "name":
		field = "name"
	case "status":
		field = "status"
	case "last_checked_at":
		field = "last_checked_at"
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

type agentWorkerHostScanner interface {
	Scan(dest ...any) error
}

func scanAgentWorkerHost(scanner agentWorkerHostScanner) (*service.AgentWorkerHost, error) {
	host := &service.AgentWorkerHost{}
	var (
		secretRef         sql.NullString
		cancelPath        sql.NullString
		lastHealthMessage sql.NullString
		lastLatency       sql.NullInt64
		lastCheckedAt     sql.NullTime
		deletedAt         sql.NullTime
		metadata          []byte
	)
	if err := scanner.Scan(
		&host.ID,
		&host.Name,
		&host.BaseURL,
		&host.Protocol,
		&host.AuthType,
		&secretRef,
		&host.HealthPath,
		&host.RunPath,
		&cancelPath,
		&host.MaxConcurrency,
		&host.TimeoutSeconds,
		&host.Status,
		&host.LastHealthStatus,
		&lastHealthMessage,
		&lastLatency,
		&lastCheckedAt,
		&metadata,
		&host.CreatedAt,
		&host.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	host.SecretRef = secretRef.String
	host.CancelPath = cancelPath.String
	host.LastHealthMessage = lastHealthMessage.String
	if lastLatency.Valid {
		v := int(lastLatency.Int64)
		host.LastHealthLatencyMS = &v
	}
	if lastCheckedAt.Valid {
		host.LastCheckedAt = &lastCheckedAt.Time
	}
	if deletedAt.Valid {
		host.DeletedAt = &deletedAt.Time
	}
	host.Metadata = unmarshalAgentJSON(metadata)
	return host, nil
}

func marshalAgentJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func unmarshalAgentJSON(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
