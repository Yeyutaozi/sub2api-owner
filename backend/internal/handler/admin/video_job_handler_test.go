package admin

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestToAdminVideoJobDTOIncludesSettledExecutionDuration(t *testing.T) {
	createdAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	settledAt := createdAt.Add(2*time.Minute + 345*time.Millisecond)
	item := &service.SeedanceTaskAdminItem{
		SeedanceTaskBinding: service.SeedanceTaskBinding{
			JobID:      "job-duration",
			TaskStatus: service.SeedanceTaskStatusSucceeded,
			CreatedAt:  createdAt,
			UpdatedAt:  settledAt,
			SettledAt:  settledAt,
		},
	}

	dto := toAdminVideoJobDTO(item)

	require.NotNil(t, dto.ExecutionDurationMs)
	require.EqualValues(t, 120_345, *dto.ExecutionDurationMs)
}

func TestToAdminVideoJobDTOOmitsInvalidExecutionDuration(t *testing.T) {
	createdAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	item := &service.SeedanceTaskAdminItem{
		SeedanceTaskBinding: service.SeedanceTaskBinding{
			JobID:     "job-invalid-duration",
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
			SettledAt: createdAt.Add(-time.Second),
		},
	}

	dto := toAdminVideoJobDTO(item)

	require.Nil(t, dto.ExecutionDurationMs)
}
