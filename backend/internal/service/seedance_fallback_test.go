package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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
		duration   int
		wantModel  string
		wantOK     bool
	}{
		{model: "seedance-2.0", resolution: "720p", duration: 5, wantModel: SeedanceMX933Model, wantOK: true},
		{model: "seedance-2.0-fast", resolution: "720p", duration: 15, wantModel: SeedanceMX933FastModel, wantOK: true},
		{model: "seedance-2.0-mini", resolution: "720p", duration: 10, wantOK: false},
		{model: "seedance-2.0", resolution: "1080p", duration: 10, wantOK: false},
		{model: "seedance-2.0", resolution: "720p", duration: 8, wantOK: false},
	}
	for _, test := range tests {
		got, ok := SeedanceFallbackModelFor(test.model, test.resolution, test.duration)
		require.Equal(t, test.wantOK, ok, test.model+"/"+test.resolution)
		require.Equal(t, test.wantModel, got, test.model+"/"+test.resolution)
	}
}

func TestSeedanceFallbackSnapshotPreservesRequestShape(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "keep the subject and motion",
		Resolution:      "720p",
		DurationSeconds: 10,
		AspectRatio:     "9:16",
		GenerateAudio:   true,
		PromptEnhance:   "AUTO",
		StartFrameURL:   "https://media.example/start.png",
		References:      []SeedanceReferenceImage{{URL: "https://media.example/ref.png", Strength: "strong"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://media.example/ref.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://media.example/ref.wav"}},
		StoredMedia: []SeedanceStoredMediaReference{{
			Slot: seedanceStoredMediaVideo, StorageProvider: "cos", Bucket: "media-bucket",
			ObjectKey: "agent-artifacts/seedance/inputs/task/1/2/video.mp4",
		}},
	}
	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(snapshot, &raw))
	require.Equal(t, "AUTO", raw["prompt_enhance"])

	restored, err := RestoreSeedanceFallbackRequest(snapshot, SeedanceMX933Model)
	require.NoError(t, err)
	require.Equal(t, SeedanceMX933Model, restored.Model)
	require.Equal(t, info.Prompt, restored.Prompt)
	require.Equal(t, info.DurationSeconds, restored.DurationSeconds)
	require.Equal(t, info.PromptEnhance, restored.PromptEnhance)
	require.Len(t, restored.References, 1)
	require.Len(t, restored.VideoReferences, 1)
	require.Len(t, restored.AudioReferences, 1)
	require.Equal(t, info.StoredMedia, restored.StoredMedia)
}

func TestSeedanceFallbackAudioReferenceEnablesHuiquGeneratedAudio(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "preserve the reference sound",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 10,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://media.example/reference.png"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://media.example/reference.wav"}},
	}
	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)

	var raw seedanceFallbackSnapshot
	require.NoError(t, json.Unmarshal(snapshot, &raw))
	require.True(t, raw.GenerateAudio)

	restored, err := RestoreSeedanceFallbackRequest(snapshot, SeedanceMX933Model)
	require.NoError(t, err)
	require.True(t, restored.GenerateAudio)
	restored.HuiquMedia = &SeedanceHuiquPreparedMedia{
		Images: []SeedanceHuiquMediaFile{huiquTestMediaFile(t, "reference.png", "image/png", []byte("image"))},
		Audios: []SeedanceHuiquMediaFile{huiquTestMediaFile(t, "reference.wav", "audio/wav", []byte("audio"))},
	}

	body, err := buildHuiquMultipartBody(restored, "sd2-mx933-720-10s")
	require.NoError(t, err)
	defer body.Close()
	mediaType, params, err := mime.ParseMediaType(body.ContentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	payload, err := io.ReadAll(body.File)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(payload), params["boundary"])
	fields := map[string][]string{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		value, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		fields[part.FormName()] = append(fields[part.FormName()], string(value))
	}
	require.Equal(t, []string{"true"}, fields["generate_audio"])
}

func TestRestoreLegacyFallbackSnapshotAudioReferenceEnablesGeneratedAudio(t *testing.T) {
	snapshot, err := json.Marshal(seedanceFallbackSnapshot{
		Prompt: "legacy audio fallback", Resolution: VideoBillingResolution720P,
		DurationSeconds: 10, AspectRatio: "16:9",
		References:      []SeedanceReferenceImage{{URL: "https://media.example/reference.png"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://media.example/reference.wav"}},
	})
	require.NoError(t, err)

	restored, err := RestoreSeedanceFallbackRequest(snapshot, SeedanceMX933LegacyModel)
	require.NoError(t, err)
	require.True(t, restored.GenerateAudio)
}

func TestRestoreSeedanceFallbackRequestAllowsLegacyVariableDurationSnapshot(t *testing.T) {
	snapshot, err := json.Marshal(seedanceFallbackSnapshot{
		Prompt: "legacy in-flight task", Resolution: VideoBillingResolution720P,
		DurationSeconds: 8, AspectRatio: "16:9",
	})
	require.NoError(t, err)

	restored, err := RestoreSeedanceFallbackRequest(snapshot, SeedanceMX933LegacyModel)
	require.NoError(t, err)
	require.Equal(t, 8, restored.DurationSeconds)
	require.Equal(t, SeedanceMX933LegacyModel, restored.Model)

	upstreamModel, err := huiquUpstreamModelFor(restored.Model, restored.DurationSeconds)
	require.NoError(t, err)
	require.Equal(t, SeedanceMX933LegacyModel, upstreamModel)
}

func TestSnapshotSeedanceRequestAlwaysStoresMaterials(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "测试卡人脸中文提示词",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		AspectRatio:     "16:9",
		StartFrameURL:   "https://media.example/face-start.png",
		References: []SeedanceReferenceImage{
			{URL: "https://media.example/face-ref.png", Strength: "high"},
		},
		StoredMedia: []SeedanceStoredMediaReference{{
			Slot: seedanceStoredMediaImage, Index: 0,
			StorageProvider: "cos", Bucket: "media",
			ObjectKey: "agent-artifacts/seedance/inputs/task/9/8/face.png",
			DeleteAfterSettlement: true,
		}},
	}
	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)
	require.NotEmpty(t, snapshot)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(snapshot, &raw))
	require.Equal(t, "测试卡人脸中文提示词", raw["prompt"])
	require.Equal(t, "https://media.example/face-start.png", raw["start_frame_url"])

	refs, ok := raw["image_references"].([]any)
	require.True(t, ok)
	require.Len(t, refs, 1)
	ref0, ok := refs[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://media.example/face-ref.png", ref0["url"])
	require.Equal(t, "high", ref0["strength"])
	_, hasLegacyURL := ref0["URL"]
	require.False(t, hasLegacyURL, "reference json must use lowercase url")
}

func TestParseSeedanceRequestSnapshotNormalizesLegacyCapitalURL(t *testing.T) {
	// Simulate historical snapshots marshalled without json tags.
	legacy := []byte(`{
		"prompt":"legacy face prompt",
		"resolution":"720p",
		"duration_seconds":8,
		"image_references":[{"URL":"https://media.example/legacy.png","Strength":"medium"}],
		"stored_media":[{"slot":"image_reference","index":0,"storage_provider":"cos","bucket":"b","object_key":"k","delete_after_settlement":true}]
	}`)
	parsed := ParseSeedanceRequestSnapshot(legacy)
	require.Equal(t, "legacy face prompt", parsed["prompt"])
	refs, ok := parsed["image_references"].([]any)
	require.True(t, ok)
	require.Len(t, refs, 1)
	ref0 := refs[0].(map[string]any)
	require.Equal(t, "https://media.example/legacy.png", ref0["url"])
	require.Equal(t, "medium", ref0["strength"])
}

