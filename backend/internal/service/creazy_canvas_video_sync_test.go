//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSyncAcceptedVideoWorkCreatesDirectAPIWorkWithoutCanvasGroupGate(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	groupID := int64(12)
	apiKey := &APIKey{
		ID:      9,
		UserID:  7,
		GroupID: &groupID,
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, AllowCreazyCanvas: false},
	}
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)

	work, err := svc.SyncAcceptedVideoWork(context.Background(), SyncAcceptedCreazyCanvasVideoInput{
		UserID:          7,
		APIKey:          apiKey,
		PublicModel:     "sd-2.0-900-720p",
		Prompt:          "direct api prompt",
		ParamsJSON:      map[string]any{"resolution": "720p", "duration": 5},
		GatewayRemoteID: "vidjob_direct_1",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), work.ID)
	require.Equal(t, CreazyCanvasWorkKindVideo, work.Kind)
	require.Equal(t, CreazyCanvasWorkStatusRunning, work.Status)
	require.Equal(t, CreazyCanvasGatewayVideoJob, work.GatewayType)
	require.Equal(t, "vidjob_direct_1", work.GatewayRemoteID)
	require.Equal(t, "sd-2.0-900-720p", work.PublicModel)
	require.Equal(t, "direct api prompt", work.Prompt)
	require.Equal(t, "api", work.ParamsJSON["source"])
	require.Len(t, repo.works, 1)
}

func TestSyncAcceptedVideoWorkReusesOwnedCanvasAssociation(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	repo.nextID = 45
	repo.works[44] = &CreazyCanvasWork{
		ID:         44,
		UserID:     7,
		APIKeyID:   9,
		Kind:       CreazyCanvasWorkKindVideo,
		Status:     CreazyCanvasWorkStatusRunning,
		ParamsJSON: map[string]any{"canvas_document_id": float64(3), "canvas_node_id": "node-video"},
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)

	work, err := svc.SyncAcceptedVideoWork(context.Background(), SyncAcceptedCreazyCanvasVideoInput{
		UserID:           7,
		APIKey:           &APIKey{ID: 9, UserID: 7},
		AssociatedWorkID: 44,
		PublicModel:      "seedance-2.0",
		Prompt:           "canvas prompt",
		ParamsJSON:       map[string]any{"resolution": "1080p", "duration": 10},
		GatewayRemoteID:  "vidjob_canvas_1",
	})

	require.NoError(t, err)
	require.Equal(t, int64(44), work.ID)
	require.Len(t, repo.works, 1)
	require.Equal(t, "vidjob_canvas_1", repo.works[44].GatewayRemoteID)
	require.Equal(t, "canvas", repo.works[44].ParamsJSON["source"])
	require.Equal(t, float64(3), repo.works[44].ParamsJSON["canvas_document_id"])
	require.Equal(t, "1080p", repo.works[44].ParamsJSON["resolution"])
}

func TestSyncAcceptedVideoWorkRejectsUntrustedCanvasAssociations(t *testing.T) {
	tests := []struct {
		name string
		work *CreazyCanvasWork
	}{
		{name: "different user", work: &CreazyCanvasWork{ID: 1, UserID: 8, APIKeyID: 9, Kind: CreazyCanvasWorkKindVideo, Status: CreazyCanvasWorkStatusRunning}},
		{name: "different key", work: &CreazyCanvasWork{ID: 1, UserID: 7, APIKeyID: 10, Kind: CreazyCanvasWorkKindVideo, Status: CreazyCanvasWorkStatusRunning}},
		{name: "different kind", work: &CreazyCanvasWork{ID: 1, UserID: 7, APIKeyID: 9, Kind: CreazyCanvasWorkKindImage, Status: CreazyCanvasWorkStatusRunning}},
		{name: "already bound elsewhere", work: &CreazyCanvasWork{ID: 1, UserID: 7, APIKeyID: 9, Kind: CreazyCanvasWorkKindVideo, Status: CreazyCanvasWorkStatusRunning, GatewayRemoteID: "vidjob_other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCreazyCanvasWorkRepoStub()
			repo.nextID = 10
			repo.works[1] = tt.work
			svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)

			created, err := svc.SyncAcceptedVideoWork(context.Background(), SyncAcceptedCreazyCanvasVideoInput{
				UserID:           7,
				APIKey:           &APIKey{ID: 9, UserID: 7},
				AssociatedWorkID: 1,
				PublicModel:      "seedance-2.0",
				Prompt:           "new prompt",
				GatewayRemoteID:  "vidjob_new",
			})

			require.NoError(t, err)
			require.Equal(t, int64(10), created.ID)
			require.Equal(t, "api", created.ParamsJSON["source"])
			require.Empty(t, tt.work.Prompt)
			if tt.name == "already bound elsewhere" {
				require.Equal(t, "vidjob_other", tt.work.GatewayRemoteID)
			}
			require.Len(t, repo.works, 2)
		})
	}
}

func TestSyncAcceptedVideoWorkIsIdempotentByRemoteIDAndPreservesTerminalStatus(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)
	input := SyncAcceptedCreazyCanvasVideoInput{
		UserID: 7, APIKey: &APIKey{ID: 9, UserID: 7}, PublicModel: "seedance-2.0", Prompt: "first", GatewayRemoteID: "vidjob_same",
	}
	first, err := svc.SyncAcceptedVideoWork(context.Background(), input)
	require.NoError(t, err)
	repo.works[first.ID].Status = CreazyCanvasWorkStatusSucceeded

	input.Prompt = "idempotent retry"
	second, err := svc.SyncAcceptedVideoWork(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, CreazyCanvasWorkStatusSucceeded, second.Status)
	require.Equal(t, "idempotent retry", second.Prompt)
	require.Len(t, repo.works, 1)
}
