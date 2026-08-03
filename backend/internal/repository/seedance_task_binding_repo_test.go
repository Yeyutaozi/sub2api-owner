//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositorySeedanceTaskBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	createdAt := time.Date(2026, 8, 3, 10, 30, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO fflink_video_job_bindings").
		WithArgs(int64(1), int64(2), int64(3), int64(4), "vidjob_one", "seedance-2.0").
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.SaveSeedanceTaskBinding(context.Background(), &service.SeedanceTaskBinding{
		UserID: 1, APIKeyID: 2, GroupID: 3, AccountID: 4,
		JobID: "vidjob_one", Model: "seedance-2.0",
	}))

	mock.ExpectQuery("SELECT user_id, api_key_id, group_id, account_id, job_id, model, created_at").
		WithArgs(int64(1), int64(2), int64(3), "vidjob_one").
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "api_key_id", "group_id", "account_id", "job_id", "model", "created_at",
		}).AddRow(1, 2, 3, 4, "vidjob_one", "seedance-2.0", createdAt))
	binding, err := repo.GetSeedanceTaskBinding(context.Background(), 1, 2, 3, "vidjob_one")
	require.NoError(t, err)
	require.Equal(t, int64(4), binding.AccountID)
	require.Equal(t, createdAt, binding.CreatedAt)

	mock.ExpectQuery("SELECT user_id, api_key_id, group_id, account_id, job_id, model, created_at").
		WithArgs(int64(1), int64(2), int64(3), 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "api_key_id", "group_id", "account_id", "job_id", "model", "created_at",
		}).
			AddRow(1, 2, 3, 4, "vidjob_two", "ltx-2.0", createdAt.Add(time.Minute)).
			AddRow(1, 2, 3, 5, "vidjob_one", "seedance-2.0", createdAt))
	bindings, err := repo.ListSeedanceTaskBindings(context.Background(), 1, 2, 3, 20)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "vidjob_two", bindings[0].JobID)
	require.Equal(t, int64(5), bindings[1].AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}
