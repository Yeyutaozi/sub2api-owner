package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *creazyCanvasWorkRepository) CreateDocument(ctx context.Context, document *service.CreazyCanvasDocument) error {
	graph, err := marshalAgentJSON(document.GraphJSON)
	if err != nil {
		return err
	}
	return r.db.QueryRowContext(ctx, `
		INSERT INTO creazy_canvas_documents (user_id, name, graph_json, revision)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, document.UserID, document.Name, graph, document.Revision).Scan(
		&document.ID,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
}

func (r *creazyCanvasWorkRepository) ListDocumentsByUser(ctx context.Context, userID int64, limit int) ([]service.CreazyCanvasDocument, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, revision, created_at, updated_at
		FROM creazy_canvas_documents
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.CreazyCanvasDocument, 0)
	for rows.Next() {
		var item service.CreazyCanvasDocument
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Name,
			&item.Revision,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *creazyCanvasWorkRepository) GetDocumentByIDForUser(ctx context.Context, id, userID int64) (*service.CreazyCanvasDocument, error) {
	var (
		document service.CreazyCanvasDocument
		graph    []byte
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, graph_json, revision, created_at, updated_at
		FROM creazy_canvas_documents
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID).Scan(
		&document.ID,
		&document.UserID,
		&document.Name,
		&graph,
		&document.Revision,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, service.ErrCreazyCanvasDocumentNotFound
	}
	if err != nil {
		return nil, err
	}
	document.GraphJSON = unmarshalAgentJSON(graph)
	return &document, nil
}

func (r *creazyCanvasWorkRepository) UpdateDocument(ctx context.Context, document *service.CreazyCanvasDocument, expectedRevision int64) error {
	graph, err := marshalAgentJSON(document.GraphJSON)
	if err != nil {
		return err
	}
	err = r.db.QueryRowContext(ctx, `
		UPDATE creazy_canvas_documents
		SET name = $3,
		    graph_json = $4,
		    revision = revision + 1,
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revision = $5 AND deleted_at IS NULL
		RETURNING revision, updated_at
	`, document.ID, document.UserID, document.Name, graph, expectedRevision).Scan(
		&document.Revision,
		&document.UpdatedAt,
	)
	if err != sql.ErrNoRows {
		return err
	}

	var exists bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM creazy_canvas_documents
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		)
	`, document.ID, document.UserID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return service.ErrCreazyCanvasDocumentNotFound
	}
	return service.ErrCreazyCanvasDocumentConflict
}

func (r *creazyCanvasWorkRepository) SoftDeleteDocument(ctx context.Context, id, userID int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE creazy_canvas_documents
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return service.ErrCreazyCanvasDocumentNotFound
	}
	return nil
}
