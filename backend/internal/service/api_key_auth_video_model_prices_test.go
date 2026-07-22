package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_SnapshotRoundTrip_PreservesSeedanceVideoModelPrices(t *testing.T) {
	groupID := int64(801)
	pro720P := 0.16
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "seedance-round-trip",
		Status:  StatusActive,
		User: &User{
			ID:     2,
			Status: StatusActive,
		},
		Group: &Group{
			ID:       groupID,
			Platform: PlatformSeedance,
			Status:   StatusActive,
			VideoModelPrices: VideoModelPrices{
				"doubao-seedance-2-0-pro": {Price720P: &pro720P},
			},
		},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
	require.NotNil(t, snapshot.Group)
	require.Equal(t, apiKey.Group.VideoModelPrices, snapshot.Group.VideoModelPrices)

	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	require.NotNil(t, roundTrip.Group)
	require.Equal(t, apiKey.Group.VideoModelPrices, roundTrip.Group.VideoModelPrices)

	snapshotCard := snapshot.Group.VideoModelPrices["doubao-seedance-2-0-pro"]
	*snapshotCard.Price720P = 9.99
	require.InDelta(t, 0.16, *apiKey.Group.VideoModelPrices["doubao-seedance-2-0-pro"].Price720P, 1e-12)
	require.InDelta(t, 0.16, *roundTrip.Group.VideoModelPrices["doubao-seedance-2-0-pro"].Price720P, 1e-12)
}

func TestAPIKeyService_SnapshotIgnoresVideoModelPricesForOtherPlatforms(t *testing.T) {
	groupID := int64(802)
	dirtyPrice := 0.001
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		User:    &User{ID: 2},
		Group: &Group{
			ID:       groupID,
			Platform: PlatformGrok,
			VideoModelPrices: VideoModelPrices{
				"grok-imagine-video": {Price720P: &dirtyPrice},
			},
		},
	})

	require.NotNil(t, snapshot.Group)
	require.Empty(t, snapshot.Group.VideoModelPrices)

	snapshot.Group.VideoModelPrices = VideoModelPrices{
		"grok-imagine-video": {Price720P: &dirtyPrice},
	}
	roundTrip := svc.snapshotToAPIKey("grok-round-trip", snapshot)
	require.Empty(t, roundTrip.Group.VideoModelPrices)
}

func TestAPIKeyService_AcceptsV15AuthSnapshotForExistingPlatforms(t *testing.T) {
	groupID := int64(803)
	svc := &APIKeyService{}
	require.Equal(t, 16, apiKeyAuthSnapshotVersion)

	apiKey, ok, err := svc.applyAuthCacheEntry("existing-grok-key", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  15,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User:     APIKeyAuthUserSnapshot{ID: 2, Status: StatusActive},
			Group: &APIKeyAuthGroupSnapshot{
				ID:       groupID,
				Platform: PlatformGrok,
				Status:   StatusActive,
			},
		},
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, apiKey)
	require.NotNil(t, apiKey.Group)
	require.Equal(t, PlatformGrok, apiKey.Group.Platform)
	require.Empty(t, apiKey.Group.VideoModelPrices)
}

func TestAPIKeyService_RejectsV15AuthSnapshotForSeedance(t *testing.T) {
	groupID := int64(804)
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("stale-seedance-key", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  apiKeyAuthSnapshotVersionBeforeSeedance,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User:     APIKeyAuthUserSnapshot{ID: 2, Status: StatusActive},
			Group: &APIKeyAuthGroupSnapshot{
				ID:       groupID,
				Platform: PlatformSeedance,
				Status:   StatusActive,
			},
		},
	})

	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, apiKey)
}
