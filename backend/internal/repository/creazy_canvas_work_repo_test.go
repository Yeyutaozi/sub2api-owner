package repository

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateAcceptedVideoUsesOwnerScopedAtomicUpsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	expiresAt := now.Add(3 * 24 * time.Hour)
	work := &service.CreazyCanvasWork{
		UserID:          7,
		APIKeyID:        9,
		Kind:            service.CreazyCanvasWorkKindVideo,
		PublicModel:     "sd-2.0-900-720p",
		Status:          service.CreazyCanvasWorkStatusRunning,
		Prompt:          "direct request",
		ParamsJSON:      map[string]any{"resolution": "720p"},
		GatewayType:     service.CreazyCanvasGatewayVideoJob,
		GatewayRemoteID: "vidjob_123",
		ExpiresAt:       expiresAt,
	}
	query := `(?s)INSERT INTO creazy_canvas_works.*ON CONFLICT \(user_id, api_key_id, gateway_type, gateway_remote_id\).*kind = 'video'.*DO UPDATE SET.*RETURNING`
	mock.ExpectQuery(query).
		WithArgs(
			int64(7), int64(9), nil, service.CreazyCanvasWorkKindVideo,
			"sd-2.0-900-720p", service.CreazyCanvasWorkStatusRunning, "direct request", sqlmock.AnyArg(),
			service.CreazyCanvasGatewayVideoJob, "vidjob_123", "", "", "", "", "", "", int64(0), "", expiresAt,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "api_key_id", "group_id", "kind", "public_model", "status", "prompt", "params_json",
			"gateway_type", "gateway_remote_id", "object_key", "storage_provider", "bucket", "object_url", "preview_url", "mime_type",
			"size_bytes", "error_message", "expires_at", "created_at", "updated_at", "deleted_at",
		}).AddRow(
			int64(101), int64(7), int64(9), nil, service.CreazyCanvasWorkKindVideo, "sd-2.0-900-720p", service.CreazyCanvasWorkStatusRunning,
			"direct request", []byte(`{"resolution":"720p"}`), service.CreazyCanvasGatewayVideoJob, "vidjob_123", "", "", "", "", "", "",
			int64(0), "", expiresAt, now, now, nil,
		))

	repo := &creazyCanvasWorkRepository{db: db}
	err = repo.CreateOrUpdateAcceptedVideo(context.Background(), work)

	require.NoError(t, err)
	require.Equal(t, int64(101), work.ID)
	require.Equal(t, "720p", work.ParamsJSON["resolution"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAcceptedVideoUniqueMigrationMatchesUpsertConflictTarget(t *testing.T) {
	migration := regexp.MustCompile(`(?s)CREATE UNIQUE INDEX IF NOT EXISTS uq_creazy_canvas_works_gateway_remote_owner\s+ON creazy_canvas_works\(user_id, api_key_id, gateway_type, gateway_remote_id\)\s+WHERE deleted_at IS NULL AND kind = 'video' AND gateway_remote_id <> '';`)
	require.True(t, migration.MatchString(readRepositoryTestMigration(t, "206_creazy_canvas_video_remote_unique.sql")))
}

func readRepositoryTestMigration(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	require.NoError(t, err)
	return string(data)
}
