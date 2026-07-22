//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsListCandidateIDsSeedance(t *testing.T) {
	require.Equal(t, []string{
		"doubao-seedance-2-0-pro",
		"doubao-seedance-2-0-fast",
	}, defaultModelsListCandidateIDs(PlatformSeedance))
}

func TestAdminServiceCreateGroupNormalizesSeedanceVideoModelPrices(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}
	price480P := 0.12
	prices := VideoModelPrices{
		" Doubao-Seedance-2-0-Pro ": {Price480P: &price480P},
	}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "seedance",
		Platform:         PlatformSeedance,
		RateMultiplier:   1,
		VideoModelPrices: prices,
	})

	require.NoError(t, err)
	require.Same(t, group, repo.created)
	require.NotContains(t, group.VideoModelPrices, " Doubao-Seedance-2-0-Pro ")
	require.Contains(t, group.VideoModelPrices, "doubao-seedance-2-0-pro")
	require.InDelta(t, 0.12, *group.VideoModelPrices["doubao-seedance-2-0-pro"].Price480P, 1e-12)
	require.NotSame(t, prices[" Doubao-Seedance-2-0-Pro "].Price480P, group.VideoModelPrices["doubao-seedance-2-0-pro"].Price480P)
}

func TestAdminServiceCreateGroupClearsVideoModelPricesForOtherPlatforms(t *testing.T) {
	platforms := []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			repo := &groupRepoStubForAdmin{}
			svc := &adminServiceImpl{groupRepo: repo}
			legacy480P := 0.08
			group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
				Name:             platform,
				Platform:         platform,
				RateMultiplier:   1,
				VideoPrice480P:   &legacy480P,
				VideoModelPrices: VideoModelPrices{"unexpected": {Price480P: videoModelPriceTestPointer(9)}},
			})

			require.NoError(t, err)
			require.Empty(t, group.VideoModelPrices)
			require.InDelta(t, 0.08, *group.VideoPrice480P, 1e-12)
			if platform == PlatformGrok {
				require.Same(t, group.VideoPrice480P, group.GetVideoPriceForModel("any-model", "480p"))
			}
		})
	}
}

func TestAdminServiceCreateGroupRejectsInvalidSeedanceVideoModelPrices(t *testing.T) {
	tests := []struct {
		name   string
		prices VideoModelPrices
	}{
		{
			name:   "empty card",
			prices: VideoModelPrices{"doubao-seedance-2-0-pro": {}},
		},
		{
			name:   "negative",
			prices: VideoModelPrices{"doubao-seedance-2-0-pro": {Price480P: videoModelPriceTestPointer(-1)}},
		},
		{
			name:   "NaN",
			prices: VideoModelPrices{"doubao-seedance-2-0-pro": {Price720P: videoModelPriceTestPointer(math.NaN())}},
		},
		{
			name:   "infinite",
			prices: VideoModelPrices{"doubao-seedance-2-0-pro": {Price1080P: videoModelPriceTestPointer(math.Inf(-1))}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &groupRepoStubForAdmin{}
			svc := &adminServiceImpl{groupRepo: repo}
			group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
				Name:             "invalid",
				Platform:         PlatformSeedance,
				RateMultiplier:   1,
				VideoModelPrices: test.prices,
			})

			require.Error(t, err)
			require.Nil(t, group)
			require.Nil(t, repo.created)
		})
	}
}

func TestAdminServiceUpdateGroupNormalizesAndClearsSeedanceVideoModelPrices(t *testing.T) {
	t.Run("normalize submitted matrix", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{ID: 1, Platform: PlatformSeedance, Status: StatusActive}}
		svc := &adminServiceImpl{groupRepo: repo}
		price720P := 0.16
		prices := VideoModelPrices{
			" Doubao-Seedance-2-0-Pro ": {Price720P: &price720P},
		}

		group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{VideoModelPrices: &prices})

		require.NoError(t, err)
		require.Contains(t, group.VideoModelPrices, "doubao-seedance-2-0-pro")
		require.NotContains(t, group.VideoModelPrices, " Doubao-Seedance-2-0-Pro ")
	})

	t.Run("omitted matrix is preserved", func(t *testing.T) {
		price480P := 0.12
		existingPrices := VideoModelPrices{
			"doubao-seedance-2-0-pro": {Price480P: &price480P},
		}
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID:               1,
			Platform:         PlatformSeedance,
			Status:           StatusActive,
			VideoModelPrices: existingPrices,
		}}
		svc := &adminServiceImpl{groupRepo: repo}

		group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{})

		require.NoError(t, err)
		require.Equal(t, existingPrices, group.VideoModelPrices)
	})

	t.Run("explicit empty object clears matrix", func(t *testing.T) {
		existingPrice := 0.12
		emptyPrices := VideoModelPrices{}
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID:       1,
			Platform: PlatformSeedance,
			Status:   StatusActive,
			VideoModelPrices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price480P: &existingPrice},
			},
		}}
		svc := &adminServiceImpl{groupRepo: repo}

		group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{VideoModelPrices: &emptyPrices})

		require.NoError(t, err)
		require.NotNil(t, group.VideoModelPrices)
		require.Empty(t, group.VideoModelPrices)
	})

	t.Run("changing platform away from Seedance clears matrix", func(t *testing.T) {
		existingPrice := 0.12
		legacy480P := 0.08
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID:             1,
			Platform:       PlatformSeedance,
			Status:         StatusActive,
			VideoPrice480P: &legacy480P,
			VideoModelPrices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price480P: &existingPrice},
			},
		}}
		svc := &adminServiceImpl{groupRepo: repo}

		group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{Platform: PlatformGrok})

		require.NoError(t, err)
		require.Equal(t, PlatformGrok, group.Platform)
		require.Empty(t, group.VideoModelPrices)
		require.Same(t, group.VideoPrice480P, group.GetVideoPriceForModel("any-grok-model", "480p"))
	})
}

func TestAdminServiceUpdateGroupRejectsInvalidSeedanceVideoModelPrices(t *testing.T) {
	repo := &groupRepoStubForAdmin{getByID: &Group{ID: 1, Platform: PlatformSeedance, Status: StatusActive}}
	svc := &adminServiceImpl{groupRepo: repo}
	prices := VideoModelPrices{
		"doubao-seedance-2-0-pro": {Price480P: videoModelPriceTestPointer(-1)},
	}

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{VideoModelPrices: &prices})

	require.Error(t, err)
	require.Nil(t, group)
	require.Nil(t, repo.updated)
}
