//go:build unit

package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type creazyCanvasDocumentRepoStub struct {
	*creazyCanvasWorkRepoStub
	nextDocumentID int64
	documents      map[int64]*CreazyCanvasDocument
	listLimit      int
}

func newCreazyCanvasDocumentRepoStub() *creazyCanvasDocumentRepoStub {
	return &creazyCanvasDocumentRepoStub{
		creazyCanvasWorkRepoStub: newCreazyCanvasWorkRepoStub(),
		nextDocumentID:           1,
		documents:                make(map[int64]*CreazyCanvasDocument),
	}
}

func (r *creazyCanvasDocumentRepoStub) CreateDocument(_ context.Context, document *CreazyCanvasDocument) error {
	if document.ID == 0 {
		document.ID = r.nextDocumentID
		r.nextDocumentID++
	}
	r.documents[document.ID] = cloneCreazyCanvasDocument(document)
	return nil
}

func (r *creazyCanvasDocumentRepoStub) ListDocumentsByUser(_ context.Context, userID int64, limit int) ([]CreazyCanvasDocument, error) {
	r.listLimit = limit
	items := make([]CreazyCanvasDocument, 0)
	for _, document := range r.documents {
		if document.UserID != userID || document.DeletedAt != nil {
			continue
		}
		items = append(items, *cloneCreazyCanvasDocument(document))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *creazyCanvasDocumentRepoStub) GetDocumentByIDForUser(_ context.Context, id, userID int64) (*CreazyCanvasDocument, error) {
	document := r.documents[id]
	if document == nil || document.UserID != userID || document.DeletedAt != nil {
		return nil, ErrCreazyCanvasDocumentNotFound
	}
	return cloneCreazyCanvasDocument(document), nil
}

func (r *creazyCanvasDocumentRepoStub) UpdateDocument(_ context.Context, document *CreazyCanvasDocument, expectedRevision int64) error {
	existing := r.documents[document.ID]
	if existing == nil || existing.UserID != document.UserID || existing.DeletedAt != nil {
		return ErrCreazyCanvasDocumentNotFound
	}
	if existing.Revision != expectedRevision {
		return ErrCreazyCanvasDocumentConflict
	}
	document.Revision = existing.Revision + 1
	if document.UpdatedAt.IsZero() {
		document.UpdatedAt = time.Now().UTC()
	}
	r.documents[document.ID] = cloneCreazyCanvasDocument(document)
	return nil
}

func (r *creazyCanvasDocumentRepoStub) SoftDeleteDocument(_ context.Context, id, userID int64) error {
	document := r.documents[id]
	if document == nil || document.UserID != userID || document.DeletedAt != nil {
		return ErrCreazyCanvasDocumentNotFound
	}
	now := time.Now().UTC()
	document.DeletedAt = &now
	document.UpdatedAt = now
	return nil
}

func cloneCreazyCanvasDocument(document *CreazyCanvasDocument) *CreazyCanvasDocument {
	if document == nil {
		return nil
	}
	cloned := *document
	if document.GraphJSON != nil {
		raw, _ := json.Marshal(document.GraphJSON)
		_ = json.Unmarshal(raw, &cloned.GraphJSON)
	}
	return &cloned
}

func TestCreazyCanvasCreateDocumentNormalizesWithoutMutatingInput(t *testing.T) {
	repo := newCreazyCanvasDocumentRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)
	graph := map[string]any{
		"nodes": []map[string]any{{"id": "prompt-1"}},
	}

	document, err := svc.CreateDocument(context.Background(), CreateCreazyCanvasDocumentInput{
		UserID:    7,
		Name:      "   ",
		GraphJSON: graph,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), document.ID)
	require.Equal(t, int64(1), document.Revision)
	require.Equal(t, "我的工作流", document.Name)
	require.Len(t, document.GraphJSON["nodes"], 1)
	require.Empty(t, document.GraphJSON["edges"])
	require.Equal(t, map[string]any{"x": float64(0), "y": float64(0), "zoom": float64(1)}, document.GraphJSON["viewport"])
	require.NotContains(t, graph, "edges")
	require.NotContains(t, graph, "viewport")
}

func TestCreazyCanvasDocumentLifecycleAndRevisionConflict(t *testing.T) {
	repo := newCreazyCanvasDocumentRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)
	ctx := context.Background()

	document, err := svc.CreateDocument(ctx, CreateCreazyCanvasDocumentInput{
		UserID: 7,
		Name:   "第一版",
		GraphJSON: map[string]any{
			"nodes": []any{},
			"edges": []any{},
		},
	})
	require.NoError(t, err)

	name := "  第二版  "
	updated, err := svc.UpdateDocument(ctx, UpdateCreazyCanvasDocumentInput{
		UserID:           7,
		DocumentID:       document.ID,
		Name:             &name,
		GraphJSON:        map[string]any{"nodes": []any{map[string]any{"id": "image-1"}}, "edges": []any{}},
		ExpectedRevision: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "第二版", updated.Name)
	require.Equal(t, int64(2), updated.Revision)

	staleName := "不应覆盖"
	_, err = svc.UpdateDocument(ctx, UpdateCreazyCanvasDocumentInput{
		UserID:           7,
		DocumentID:       document.ID,
		Name:             &staleName,
		ExpectedRevision: 1,
	})
	require.ErrorIs(t, err, ErrCreazyCanvasDocumentConflict)
	stored, err := svc.GetDocument(ctx, 7, document.ID)
	require.NoError(t, err)
	require.Equal(t, "第二版", stored.Name)
	require.Equal(t, int64(2), stored.Revision)

	items, err := svc.ListDocuments(ctx, 7)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 50, repo.listLimit)

	require.NoError(t, svc.DeleteDocument(ctx, 7, document.ID))
	_, err = svc.GetDocument(ctx, 7, document.ID)
	require.ErrorIs(t, err, ErrCreazyCanvasDocumentNotFound)
	require.ErrorIs(t, svc.DeleteDocument(ctx, 7, document.ID), ErrCreazyCanvasDocumentNotFound)
}

func TestCreazyCanvasDocumentRejectsInvalidAndOversizedGraphs(t *testing.T) {
	repo := newCreazyCanvasDocumentRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)
	tests := []struct {
		name   string
		graph  map[string]any
		reason string
	}{
		{name: "nodes are not an array", graph: map[string]any{"nodes": "bad"}, reason: "CREAZY_CANVAS_GRAPH_INVALID"},
		{name: "edges are not an array", graph: map[string]any{"edges": map[string]any{}}, reason: "CREAZY_CANVAS_GRAPH_INVALID"},
		{name: "viewport is not an object", graph: map[string]any{"viewport": "bad"}, reason: "CREAZY_CANVAS_GRAPH_INVALID"},
		{name: "graph cannot be serialized", graph: map[string]any{"nodes": []any{}, "bad": func() {}}, reason: "CREAZY_CANVAS_GRAPH_INVALID"},
		{name: "graph is too large", graph: map[string]any{"payload": strings.Repeat("x", creazyCanvasGraphMaxSize)}, reason: "CREAZY_CANVAS_GRAPH_TOO_LARGE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateDocument(context.Background(), CreateCreazyCanvasDocumentInput{UserID: 7, GraphJSON: tt.graph})
			require.Error(t, err)
			require.Equal(t, tt.reason, infraerrors.Reason(err))
		})
	}
	require.Empty(t, repo.documents)
}

func TestCreazyCanvasDocumentRepositoryUnavailable(t *testing.T) {
	var nilService *CreazyCanvasService
	_, err := nilService.ListDocuments(context.Background(), 7)
	require.EqualError(t, err, "creazy canvas document repository is unavailable")

	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{}, nil, nil)
	_, err = svc.CreateDocument(context.Background(), CreateCreazyCanvasDocumentInput{UserID: 7})
	require.EqualError(t, err, "creazy canvas document repository is unavailable")
}
