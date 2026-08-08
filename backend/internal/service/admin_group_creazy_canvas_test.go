//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateGroupAllowCreazyCanvasDefaultTrue(t *testing.T) {
	repo := &groupRepoStubForAdmin{createID: 1}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "canvas-default",
		Platform:       PlatformSeedance,
		RateMultiplier: 1.0,
	})
	require.NoError(t, err)
	require.True(t, group.AllowCreazyCanvas)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.AllowCreazyCanvas)
}

func TestCreateGroupAllowCreazyCanvasExplicitFalse(t *testing.T) {
	repo := &groupRepoStubForAdmin{createID: 2}
	svc := &adminServiceImpl{groupRepo: repo}
	falseVal := false

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:              "canvas-off",
		Platform:          PlatformSeedance,
		RateMultiplier:    1.0,
		AllowCreazyCanvas: &falseVal,
	})
	require.NoError(t, err)
	require.False(t, group.AllowCreazyCanvas)
	require.False(t, repo.created.AllowCreazyCanvas)
}

func TestUpdateGroupAllowCreazyCanvas(t *testing.T) {
	existing := &Group{
		ID:                10,
		Name:              "g1",
		Platform:          PlatformSeedance,
		Status:            StatusActive,
		RateMultiplier:    1.0,
		AllowCreazyCanvas: true,
		Hydrated:          true,
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}
	falseVal := false

	group, err := svc.UpdateGroup(context.Background(), 10, &UpdateGroupInput{
		Name:              "g1",
		AllowCreazyCanvas: &falseVal,
	})
	require.NoError(t, err)
	require.False(t, group.AllowCreazyCanvas)
	require.NotNil(t, repo.updated)
	require.False(t, repo.updated.AllowCreazyCanvas)
}
