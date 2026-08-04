package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type seedanceCancellationRepositoryStub struct {
	UsageLogRepository
	claimed    bool
	token      string
	completed  bool
	released   bool
	userID     int64
	apiKeyID   int64
	groupID    int64
	jobID      string
	claimToken string
}

func (s *seedanceCancellationRepositoryStub) ClaimSeedanceTaskCancellation(_ context.Context, userID, apiKeyID, groupID int64, jobID string) (bool, string, error) {
	s.userID, s.apiKeyID, s.groupID, s.jobID = userID, apiKeyID, groupID, jobID
	return s.claimed, s.token, nil
}

func (s *seedanceCancellationRepositoryStub) CompleteSeedanceTaskCancellation(_ context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error) {
	s.userID, s.apiKeyID, s.groupID, s.jobID, s.claimToken = userID, apiKeyID, groupID, jobID, claimToken
	return s.completed, nil
}

func (s *seedanceCancellationRepositoryStub) ReleaseSeedanceTaskCancellation(_ context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error) {
	s.userID, s.apiKeyID, s.groupID, s.jobID, s.claimToken = userID, apiKeyID, groupID, jobID, claimToken
	return s.released, nil
}

func TestSeedanceTaskCancellationServiceDelegatesWithOwnerScope(t *testing.T) {
	groupID := int64(33)
	repo := &seedanceCancellationRepositoryStub{claimed: true, token: "cancel-token", completed: true, released: true}
	svc := &OpenAIGatewayService{usageLogRepo: repo}

	claimed, token, err := svc.ClaimSeedanceTaskCancellation(context.Background(), &groupID, "job-1", 11, 22)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "cancel-token", token)
	require.Equal(t, []int64{11, 22, 33}, []int64{repo.userID, repo.apiKeyID, repo.groupID})
	require.Equal(t, "job-1", repo.jobID)
	completed, err := svc.CompleteSeedanceTaskCancellation(context.Background(), &groupID, "job-1", 11, 22, token)
	require.NoError(t, err)
	require.True(t, completed)
	require.Equal(t, token, repo.claimToken)
	released, err := svc.ReleaseSeedanceTaskCancellation(context.Background(), &groupID, "job-1", 11, 22, token)
	require.NoError(t, err)
	require.True(t, released)
}

func TestSeedanceTaskCancellationServiceRejectsInvalidScope(t *testing.T) {
	repo := &seedanceCancellationRepositoryStub{claimed: true, token: "cancel-token"}
	svc := &OpenAIGatewayService{usageLogRepo: repo}
	claimed, token, err := svc.ClaimSeedanceTaskCancellation(context.Background(), nil, "job-1", 11, 22)
	require.Error(t, err)
	require.False(t, claimed)
	require.Empty(t, token)
}

func TestSeedanceIndexedJobDoesNotPollPrimaryDuringCancellation(t *testing.T) {
	svc := &OpenAIGatewayService{}
	binding := SeedanceTaskBinding{JobID: "job-1", Model: "seedance-2.0", FallbackStatus: SeedanceFallbackStatusCancelling}
	job := svc.loadSeedanceIndexedJob(context.Background(), binding)
	require.Equal(t, "running", job["status"])
	require.Equal(t, "seedance-2.0", job["model"])

	binding.FallbackStatus = SeedanceFallbackStatusCancelled
	job = svc.loadSeedanceIndexedJob(context.Background(), binding)
	require.Equal(t, "cancelled", job["status"])
}

func TestSeedanceFallbackModelForOnlyMapsSupportedFFLink720pModels(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		wantModel  string
		wantOK     bool
	}{
		{model: "seedance-2.0", resolution: "720p", wantModel: "sd2-mx933-720-1s", wantOK: true},
		{model: "seedance-2.0-fast", resolution: "720p", wantModel: "sd2-mx933-720-fast-1s", wantOK: true},
		{model: "seedance-2.0-mini", resolution: "720p", wantOK: false},
		{model: "seedance-2.0", resolution: "1080p", wantOK: false},
	}
	for _, test := range tests {
		got, ok := SeedanceFallbackModelFor(test.model, test.resolution)
		require.Equal(t, test.wantOK, ok, test.model+"/"+test.resolution)
		require.Equal(t, test.wantModel, got, test.model+"/"+test.resolution)
	}
}

func TestSeedanceFallbackSnapshotPreservesRequestShape(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "keep the subject and motion",
		Resolution:      "720p",
		DurationSeconds: 7,
		AspectRatio:     "9:16",
		GenerateAudio:   true,
		PromptEnhance:   "AUTO",
		StartFrameURL:   "https://media.example/start.png",
		References:      []SeedanceReferenceImage{{URL: "https://media.example/ref.png", Strength: "strong"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://media.example/ref.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://media.example/ref.wav"}},
	}
	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(snapshot, &raw))
	require.Equal(t, "AUTO", raw["prompt_enhance"])

	restored, err := RestoreSeedanceFallbackRequest(snapshot, "sd2-mx933-720-1s")
	require.NoError(t, err)
	require.Equal(t, "sd2-mx933-720-1s", restored.Model)
	require.Equal(t, info.Prompt, restored.Prompt)
	require.Equal(t, info.DurationSeconds, restored.DurationSeconds)
	require.Equal(t, info.PromptEnhance, restored.PromptEnhance)
	require.Len(t, restored.References, 1)
	require.Len(t, restored.VideoReferences, 1)
	require.Len(t, restored.AudioReferences, 1)
}
