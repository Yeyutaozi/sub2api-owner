package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAgentRunServiceHandleModelProxyRejectsWrongRunToken(t *testing.T) {
	svc, gateway := newTestAgentRunService()

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		Endpoint: "/v1/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, "wrong-token")

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrAgentRunTokenInvalid)
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyRejectsUnsupportedEndpoint(t *testing.T) {
	svc, gateway := newTestAgentRunService()

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		Endpoint: "https://example.com/v1/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, "AGENT_MODEL_PROXY_ENDPOINT_REQUIRED", errorReason(err))
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyRejectsTerminalRun(t *testing.T) {
	svc, gateway := newTestAgentRunService()
	svc.runRepo.(*testAgentRunRepo).run.Status = AgentRunStatusSucceeded

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		Endpoint: "/v1/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, "AGENT_MODEL_PROXY_RUN_TERMINAL", errorReason(err))
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyRejectsRedisCancelFlag(t *testing.T) {
	svc, gateway := newTestAgentRunService()
	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()
	svc.redisClient = rdb
	require.NoError(t, rdb.Set(context.Background(), agentRunCancelKey(1001), "1", time.Minute).Err())

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		Endpoint: "/v1/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, "AGENT_MODEL_PROXY_RUN_CANCELED", errorReason(err))
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyRejectsModelOutsideNodePolicy(t *testing.T) {
	svc, gateway := newTestAgentRunService()

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-large",
		Endpoint: "/v1/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, "AGENT_MODEL_PROXY_MODEL_NOT_ALLOWED", errorReason(err))
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyUsesNodeRolePolicyAndFillsGroup(t *testing.T) {
	svc, gateway := newTestAgentRunService()
	svc.appRepo.(*testAgentAppRepo).version.NodeModelPolicyJSON = map[string]any{
		"prompt_rewrite.rewrite": map[string]any{
			"model":          "gpt-5-mini",
			"model_group_id": float64(88),
		},
	}

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Role:     "rewrite",
		Model:    "gpt-5-mini",
		Endpoint: "/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, gateway.calls)
	require.NotNil(t, gateway.req.GroupID)
	require.Equal(t, int64(88), *gateway.req.GroupID)
	require.Equal(t, "prompt_rewrite", gateway.req.Metadata["agent_node_id"])
	require.Equal(t, "rewrite", gateway.req.Metadata["agent_node_role"])
	require.Equal(t, int64(88), gateway.req.Metadata["agent_model_group_id"])
}

func TestAgentRunServiceHandleModelProxyRejectsWrongNodeRoleGroup(t *testing.T) {
	svc, gateway := newTestAgentRunService()

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Role:     "rewrite",
		Model:    "gpt-5-mini",
		GroupID:  ptrInt64(89),
		Endpoint: "/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, "AGENT_MODEL_PROXY_GROUP_NOT_ALLOWED", errorReason(err))
	require.Equal(t, 0, gateway.calls)
}

func TestAgentRunServiceHandleModelProxyCallsGatewayWithRunBoundAPIKey(t *testing.T) {
	svc, gateway := newTestAgentRunService()
	reqBody := map[string]any{"messages": []any{}}

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		GroupID:  ptrInt64(88),
		Endpoint: "/chat/completions",
		Request:  reqBody,
	}, testAgentRunPlainToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, gateway.calls)
	require.Equal(t, "sk-test-run-bound", gateway.apiKey.Key)
	require.Equal(t, int64(42), gateway.apiKey.ID)
	require.Equal(t, "/v1/chat/completions", gateway.req.Endpoint)
	require.Equal(t, "gpt-5-mini", gateway.req.Request["model"])
	require.Equal(t, int64(1001), gateway.req.Metadata["agent_run_id"])
}

func TestAgentRunServiceHandleModelProxyRecordsRunEvent(t *testing.T) {
	svc, _ := newTestAgentRunService()
	repo := svc.runRepo.(*testAgentRunRepo)

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "prompt_rewrite",
		Model:    "gpt-5-mini",
		GroupID:  ptrInt64(88),
		Endpoint: "/chat/completions",
		Request:  map[string]any{"messages": []any{}},
	}, testAgentRunPlainToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, repo.events, 1)
	event := repo.events[0]
	require.Equal(t, AgentRunEventModelProxy, event.EventType)
	require.Equal(t, AgentRunStatusSucceeded, event.Status)
	require.Equal(t, "prompt_rewrite", event.NodeID)
	require.Equal(t, "model proxy completed", event.Message)
	require.Equal(t, "gpt-5-mini", event.MetadataJSON["model"])
	require.Equal(t, "/v1/chat/completions", event.MetadataJSON["endpoint"])
	require.Equal(t, int64(42), event.MetadataJSON["api_key_id"])
	require.NotContains(t, event.MetadataJSON, "api_key")
	require.NotContains(t, event.MetadataJSON, "key")
}

func TestAgentRunServiceHandleModelProxyUsesPolicyBoundAPIKey(t *testing.T) {
	svc, gateway := newTestAgentRunService()
	svc.appRepo.(*testAgentAppRepo).version.NodeModelPolicyJSON = map[string]any{
		"prompt_rewrite.rewrite": map[string]any{
			"model":          "gpt-5-mini",
			"model_group_id": float64(88),
		},
		"image_generation.generate": map[string]any{
			"model":          "gpt-image-1",
			"model_group_id": float64(77),
		},
	}
	svc.runRepo.(*testAgentRunRepo).bindings = []AgentRunKeyBinding{
		{
			RunID:        1001,
			UserID:       7,
			APIKeyID:     99,
			PolicyKey:    "image_generation.generate",
			NodeID:       "image_generation",
			Role:         "generate",
			ModelGroupID: ptrInt64(77),
		},
	}
	svc.apiKeyService.(*testAgentRunAPIKeyService).keys[99] = &APIKey{
		ID:      99,
		UserID:  7,
		Key:     "sk-test-image",
		GroupID: ptrInt64(77),
		Group:   &Group{ID: 77, Platform: PlatformOpenAI},
		Status:  StatusActive,
	}

	resp, err := svc.HandleModelProxy(context.Background(), 1001, ModelProxyRequest{
		RunID:    1001,
		NodeID:   "image_generation",
		Role:     "generate",
		Model:    "gpt-image-1",
		Endpoint: "/images/generations",
		Request:  map[string]any{"prompt": "test"},
	}, testAgentRunPlainToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, gateway.calls)
	require.Equal(t, int64(99), gateway.apiKey.ID)
	require.Equal(t, "sk-test-image", gateway.apiKey.Key)
	require.NotNil(t, gateway.req.GroupID)
	require.Equal(t, int64(77), *gateway.req.GroupID)
	require.Equal(t, "image_generation.generate", gateway.req.Metadata["agent_policy_key"])
	require.Equal(t, int64(99), gateway.req.Metadata["agent_api_key_id"])
}

func TestAgentRunServicePrepareRunKeyBindingsRejectsGroupMismatch(t *testing.T) {
	svc, _ := newTestAgentRunService()
	version := svc.appRepo.(*testAgentAppRepo).version
	version.NodeModelPolicyJSON = map[string]any{
		"image_generation.generate": map[string]any{
			"model":          "gpt-image-1",
			"model_group_id": float64(77),
		},
	}

	bindings, err := svc.prepareRunKeyBindings(context.Background(), CreateAgentRunInput{
		UserID:   7,
		APIKeyID: 42,
	}, version)

	require.Nil(t, bindings)
	require.Error(t, err)
	require.Equal(t, "AGENT_RUN_API_KEY_GROUP_MISMATCH", errorReason(err))
}

func TestAgentRunServicePrepareRunKeyBindingsRejectsProviderMismatch(t *testing.T) {
	svc, _ := newTestAgentRunService()
	version := svc.appRepo.(*testAgentAppRepo).version
	version.NodeModelPolicyJSON = map[string]any{
		"prompt_rewrite.rewrite": map[string]any{
			"provider": "openai",
			"model":    "gpt-5-mini",
		},
	}
	key := svc.apiKeyService.(*testAgentRunAPIKeyService).keys[42]
	key.GroupID = ptrInt64(66)
	key.Group = &Group{ID: 66, Platform: PlatformAnthropic}

	bindings, err := svc.prepareRunKeyBindings(context.Background(), CreateAgentRunInput{
		UserID:   7,
		APIKeyID: 42,
	}, version)

	require.Nil(t, bindings)
	require.Error(t, err)
	require.Equal(t, "AGENT_RUN_API_KEY_PROVIDER_MISMATCH", errorReason(err))
}

func TestAgentRunServicePrepareRunKeyBindingsCreatesDefaultPolicyBindings(t *testing.T) {
	svc, _ := newTestAgentRunService()
	version := svc.appRepo.(*testAgentAppRepo).version

	bindings, err := svc.prepareRunKeyBindings(context.Background(), CreateAgentRunInput{
		UserID:   7,
		APIKeyID: 42,
	}, version)

	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "prompt_rewrite", bindings[0].PolicyKey)
	require.Equal(t, int64(42), bindings[0].APIKeyID)
	require.True(t, bindings[0].IsDefault)
	require.NotNil(t, bindings[0].ModelGroupID)
	require.Equal(t, int64(88), *bindings[0].ModelGroupID)
}

func TestAgentRunServiceCleanupDeletesExpiredObjectsWhenExplicitlyEnabled(t *testing.T) {
	repo := &testAgentRunRepo{
		events: []AgentRunEvent{
			{ID: 1, RunID: 1001, UserID: 7, EventType: AgentRunEventQueued},
			{ID: 2, RunID: 1001, UserID: 7, EventType: AgentRunEventRunning},
		},
		expiredArtifacts: []AgentCleanupObjectRef{
			{ID: 11, StorageProvider: "s3", Bucket: "agent-artifacts", ObjectKey: "agent-artifacts/runs/1/out.txt"},
			{ID: 12, StorageProvider: "external", Bucket: "", ObjectKey: "external.txt"},
		},
		expiredInputs: []AgentCleanupObjectRef{
			{ID: 21, StorageProvider: "s3", Bucket: "agent-artifacts", ObjectKey: "agent-artifacts/inputs/1/image.png"},
		},
	}
	store := &testAgentArtifactStore{provider: "s3", bucket: "agent-artifacts"}
	svc := NewAgentRunService(nil, nil, repo, nil, nil, store, &config.Config{
		AgentArtifacts: config.AgentArtifactStorageConfig{
			CleanupExpiredArtifactsEnabled: true,
		},
	})

	result := svc.runAgentCleanupOnce(context.Background(), time.Now().UTC())

	require.Len(t, repo.events, 2)
	require.Equal(t, int64(2), result.ArtifactsDeleted)
	require.Equal(t, int64(1), result.InputAssetsDeleted)
	require.Equal(t, int64(2), result.ObjectsDeleted)
	require.Equal(t, int64(0), result.ObjectDeleteErrors)
	require.Equal(t, []string{"agent-artifacts/runs/1/out.txt", "agent-artifacts/inputs/1/image.png"}, store.deletedKeys)
	require.ElementsMatch(t, []int64{11, 12}, repo.deletedArtifacts)
	require.ElementsMatch(t, []int64{21}, repo.deletedInputs)
}

func TestAgentRunServiceCleanupDoesNotDeleteRunEventsOrBusinessArtifactsByDefault(t *testing.T) {
	repo := &testAgentRunRepo{
		events: []AgentRunEvent{
			{ID: 1, RunID: 1001, UserID: 7, EventType: AgentRunEventQueued},
		},
		expiredArtifacts: []AgentCleanupObjectRef{
			{ID: 11, StorageProvider: "s3", Bucket: "agent-artifacts", ObjectKey: "agent-artifacts/runs/1/out.txt"},
		},
		expiredInputs: []AgentCleanupObjectRef{
			{ID: 21, StorageProvider: "s3", Bucket: "agent-artifacts", ObjectKey: "agent-artifacts/inputs/1/image.png"},
		},
	}
	store := &testAgentArtifactStore{provider: "s3", bucket: "agent-artifacts"}
	svc := NewAgentRunService(nil, nil, repo, nil, nil, store, &config.Config{})

	result := svc.runAgentCleanupOnce(context.Background(), time.Now().UTC())

	require.Len(t, repo.events, 1)
	require.Zero(t, result.ArtifactsDeleted)
	require.Zero(t, result.InputAssetsDeleted)
	require.Zero(t, result.ObjectsDeleted)
	require.Empty(t, store.deletedKeys)
	require.Empty(t, repo.deletedArtifacts)
	require.Empty(t, repo.deletedInputs)
}

func TestAgentRunServiceCleanupLoopRunsWhenExplicitlyEnabled(t *testing.T) {
	repo := &testAgentRunRepo{cleanupCalls: make(chan struct{}, 1)}
	svc := &AgentRunService{
		runRepo:       repo,
		artifactStore: &testAgentArtifactStore{provider: "s3", bucket: "agent-artifacts"},
		cfg: &config.Config{AgentArtifacts: config.AgentArtifactStorageConfig{
			CleanupExpiredArtifactsEnabled: true,
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.agentCleanupLoop(ctx, 5*time.Millisecond)
	}()

	select {
	case <-repo.cleanupCalls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("scheduled agent artifact cleanup did not run")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("agent artifact cleanup loop did not stop after context cancellation")
	}
}

const testAgentRunPlainToken = "ar_test_model_proxy_token"

func newTestAgentRunService() (*AgentRunService, *testModelProxyGateway) {
	tokenHash := hashAgentRunTokenForTest(testAgentRunPlainToken)
	runRepo := &testAgentRunRepo{
		run: &AgentRun{
			ID:                1001,
			AppID:             11,
			AppVersionID:      22,
			UserID:            7,
			APIKeyID:          42,
			RunTokenHash:      tokenHash,
			Status:            AgentRunStatusRunning,
			InputSummaryJSON:  map[string]any{},
			OutputSummaryJSON: map[string]any{},
			UsageJSON:         map[string]any{},
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		},
	}
	appRepo := &testAgentAppRepo{
		version: &AgentAppVersion{
			ID:    22,
			AppID: 11,
			NodeModelPolicyJSON: map[string]any{
				"prompt_rewrite": map[string]any{
					"model":          "gpt-5-mini",
					"model_group_id": float64(88),
				},
			},
		},
	}
	gateway := &testModelProxyGateway{}
	apiKeyService := &testAgentRunAPIKeyService{
		keys: map[int64]*APIKey{
			42: {
				ID:      42,
				UserID:  7,
				Key:     "sk-test-run-bound",
				GroupID: ptrInt64(88),
				Group:   &Group{ID: 88, Platform: PlatformOpenAI},
				Status:  StatusActive,
			},
		},
	}
	return NewAgentRunService(appRepo, nil, runRepo, apiKeyService, gateway, disabledAgentArtifactStore{}, nil), gateway
}

func hashAgentRunTokenForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ptrInt64(v int64) *int64 {
	return &v
}

func errorReason(err error) string {
	return infraerrors.Reason(err)
}

type testModelProxyGateway struct {
	calls  int
	req    ModelProxyRequest
	apiKey *APIKey
}

func (g *testModelProxyGateway) CallModelProxy(_ context.Context, req ModelProxyRequest, apiKey *APIKey) (*ModelProxyResponse, error) {
	g.calls++
	g.req = req
	g.apiKey = apiKey
	return &ModelProxyResponse{
		Response: map[string]any{"ok": true},
		Usage:    map[string]any{"total_tokens": 1},
	}, nil
}

type testAgentArtifactStore struct {
	provider    string
	bucket      string
	deletedKeys []string
}

func (s *testAgentArtifactStore) IsConfigured() bool { return true }
func (s *testAgentArtifactStore) Provider() string   { return s.provider }
func (s *testAgentArtifactStore) Bucket() string     { return s.bucket }
func (s *testAgentArtifactStore) Put(context.Context, AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	return nil, ErrAgentArtifactStorageNotConfigured
}
func (s *testAgentArtifactStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrAgentArtifactStorageNotConfigured
}
func (s *testAgentArtifactStore) PresignGetObject(_ context.Context, location AgentArtifactObjectLocation, ttl time.Duration) (string, error) {
	return s.PresignGet(context.Background(), location.ObjectKey, ttl)
}
func (s *testAgentArtifactStore) Delete(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}
func (s *testAgentArtifactStore) DeleteObject(_ context.Context, location AgentArtifactObjectLocation) error {
	return s.Delete(context.Background(), location.ObjectKey)
}

type testAgentRunRepo struct {
	run              *AgentRun
	bindings         []AgentRunKeyBinding
	inputAssets      []AgentInputAsset
	events           []AgentRunEvent
	expiredArtifacts []AgentCleanupObjectRef
	expiredInputs    []AgentCleanupObjectRef
	deletedArtifacts []int64
	deletedInputs    []int64
	cleanupCalls     chan struct{}
}

func (r *testAgentRunRepo) CreateRun(context.Context, *AgentRun) error { return nil }
func (r *testAgentRunRepo) CreateRunWithKeyBindings(_ context.Context, run *AgentRun, bindings []AgentRunKeyBinding) error {
	r.run = run
	r.bindings = append([]AgentRunKeyBinding(nil), bindings...)
	return nil
}
func (r *testAgentRunRepo) GetRunByID(_ context.Context, id int64) (*AgentRun, error) {
	if r.run == nil || r.run.ID != id {
		return nil, ErrAgentRunNotFound
	}
	cp := *r.run
	return &cp, nil
}
func (r *testAgentRunRepo) GetRunByIDForUser(_ context.Context, id, userID int64) (*AgentRun, error) {
	if r.run == nil || r.run.ID != id || r.run.UserID != userID {
		return nil, ErrAgentRunNotFound
	}
	cp := *r.run
	return &cp, nil
}
func (r *testAgentRunRepo) ListRunsByUser(context.Context, int64, pagination.PaginationParams, AgentRunListFilters) ([]AgentRun, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *testAgentRunRepo) ListRunKeyBindings(_ context.Context, runID int64) ([]AgentRunKeyBinding, error) {
	items := make([]AgentRunKeyBinding, 0)
	for _, binding := range r.bindings {
		if binding.RunID == runID {
			items = append(items, binding)
		}
	}
	return items, nil
}
func (r *testAgentRunRepo) CreateInputAsset(_ context.Context, asset *AgentInputAsset) error {
	if asset.ID == 0 {
		asset.ID = int64(len(r.inputAssets) + 1)
	}
	r.inputAssets = append(r.inputAssets, *asset)
	return nil
}
func (r *testAgentRunRepo) ListInputAssetsByUser(_ context.Context, userID int64, _ pagination.PaginationParams, _ AgentInputAssetListFilters) ([]AgentInputAsset, *pagination.PaginationResult, error) {
	items := make([]AgentInputAsset, 0)
	for _, asset := range r.inputAssets {
		if asset.UserID == userID {
			items = append(items, asset)
		}
	}
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: 1, PageSize: len(items), Pages: 1}, nil
}
func (r *testAgentRunRepo) ListInputAssetsByIDsForUser(_ context.Context, userID int64, ids []int64) ([]AgentInputAsset, error) {
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	items := make([]AgentInputAsset, 0)
	for _, asset := range r.inputAssets {
		if asset.UserID == userID {
			if _, ok := wanted[asset.ID]; ok {
				items = append(items, asset)
			}
		}
	}
	return items, nil
}
func (r *testAgentRunRepo) GetInputAssetByID(_ context.Context, assetID int64) (*AgentInputAsset, error) {
	for _, asset := range r.inputAssets {
		if asset.ID == assetID {
			cp := asset
			return &cp, nil
		}
	}
	return nil, ErrAgentInputAssetNotFound
}
func (r *testAgentRunRepo) MarkRunning(context.Context, int64, time.Time) error { return nil }
func (r *testAgentRunRepo) MarkFailed(context.Context, int64, string, string, time.Time) error {
	return nil
}
func (r *testAgentRunRepo) MarkTimeout(context.Context, int64, time.Time) error { return nil }
func (r *testAgentRunRepo) MarkCanceled(context.Context, int64, int64, string, string, time.Time) error {
	if r.run != nil {
		r.run.Status = AgentRunStatusCanceled
	}
	return nil
}
func (r *testAgentRunRepo) UpdateFromCallback(context.Context, *AgentRun) error { return nil }
func (r *testAgentRunRepo) CreateRunEvent(_ context.Context, event *AgentRunEvent) error {
	if event == nil {
		return nil
	}
	if event.ID == 0 {
		event.ID = int64(len(r.events) + 1)
	}
	event.CreatedAt = time.Now().UTC()
	r.events = append(r.events, *event)
	return nil
}
func (r *testAgentRunRepo) ListRunEventsByRunForUser(_ context.Context, runID, userID int64, _ pagination.PaginationParams) ([]AgentRunEvent, *pagination.PaginationResult, error) {
	items := make([]AgentRunEvent, 0)
	for _, event := range r.events {
		if event.RunID == runID && event.UserID == userID {
			items = append(items, event)
		}
	}
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: 1, PageSize: len(items), Pages: 1}, nil
}
func (r *testAgentRunRepo) CreateArtifact(context.Context, *AgentArtifact) error { return nil }
func (r *testAgentRunRepo) ListArtifactsByRun(context.Context, int64) ([]AgentArtifact, error) {
	return nil, nil
}
func (r *testAgentRunRepo) GetArtifactByID(context.Context, int64) (*AgentArtifact, error) {
	return nil, ErrAgentArtifactNotFound
}
func (r *testAgentRunRepo) ListExpiredArtifacts(context.Context, time.Time, int) ([]AgentCleanupObjectRef, error) {
	if r.cleanupCalls != nil {
		select {
		case r.cleanupCalls <- struct{}{}:
		default:
		}
	}
	return append([]AgentCleanupObjectRef(nil), r.expiredArtifacts...), nil
}
func (r *testAgentRunRepo) MarkArtifactsDeleted(_ context.Context, ids []int64, _ time.Time) (int64, error) {
	r.deletedArtifacts = append(r.deletedArtifacts, ids...)
	return int64(len(ids)), nil
}
func (r *testAgentRunRepo) ListExpiredInputAssets(context.Context, time.Time, int) ([]AgentCleanupObjectRef, error) {
	return append([]AgentCleanupObjectRef(nil), r.expiredInputs...), nil
}
func (r *testAgentRunRepo) MarkInputAssetsDeleted(_ context.Context, ids []int64, _ time.Time) (int64, error) {
	r.deletedInputs = append(r.deletedInputs, ids...)
	return int64(len(ids)), nil
}

type testAgentAppRepo struct {
	version *AgentAppVersion
}

func (r *testAgentAppRepo) CreateApp(context.Context, *AgentApp) error { return nil }
func (r *testAgentAppRepo) CreateAppWithVersion(context.Context, *AgentApp, *AgentAppVersion) error {
	return nil
}
func (r *testAgentAppRepo) UpdateApp(context.Context, *AgentApp) error     { return nil }
func (r *testAgentAppRepo) DeleteApp(context.Context, int64, *int64) error { return nil }
func (r *testAgentAppRepo) GetAppByID(context.Context, int64) (*AgentApp, error) {
	return &AgentApp{ID: 11}, nil
}
func (r *testAgentAppRepo) ListApps(context.Context, pagination.PaginationParams, AgentAppListFilters) ([]AgentApp, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *testAgentAppRepo) CreateVersion(context.Context, *AgentAppVersion) error { return nil }
func (r *testAgentAppRepo) GetVersionByID(_ context.Context, id int64) (*AgentAppVersion, error) {
	if r.version == nil || r.version.ID != id {
		return nil, ErrAgentAppVersionNotFound
	}
	cp := *r.version
	return &cp, nil
}
func (r *testAgentAppRepo) GetPublishedVersionForApp(context.Context, int64, int64) (*AgentApp, *AgentAppVersion, error) {
	return nil, nil, nil
}
func (r *testAgentAppRepo) ListVersions(context.Context, int64) ([]AgentAppVersion, error) {
	return nil, nil
}
func (r *testAgentAppRepo) PublishVersion(context.Context, int64, int64, *int64) error {
	return nil
}
func (r *testAgentAppRepo) SetVersionStatus(context.Context, int64, int64, string, *int64) error {
	return nil
}

type testAgentRunAPIKeyService struct {
	keys map[int64]*APIKey
}

func (s *testAgentRunAPIKeyService) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if s.keys == nil || s.keys[id] == nil {
		return nil, ErrAPIKeyNotFound
	}
	cp := *s.keys[id]
	return &cp, nil
}

func (s *testAgentRunAPIKeyService) CheckAPIKeyQuotaAndExpiry(apiKey *APIKey) error {
	return nil
}
