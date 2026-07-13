package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAgentRunServiceCancelRunPropagatesToWorker(t *testing.T) {
	runToken := "ar_cancel_test"
	var received WorkerCancelRequest
	var receivedSignature string
	var expectedSignature string

	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/cancel", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &received))
		receivedSignature = r.Header.Get("X-Sub2API-Signature")
		expectedSignature = signWorkerPayload(runToken, r.Header.Get("X-Sub2API-Timestamp"), body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true,"status":"cancel_requested"}`))
	}))
	defer worker.Close()

	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	repo := &testAgentRunRepo{run: &AgentRun{
		ID:           1001,
		UserID:       42,
		WorkerHostID: ptrInt64(7),
		RunTokenHash: hashAgentRunTokenForTest(runToken),
		Status:       AgentRunStatusRunning,
	}}
	svc := NewAgentRunService(nil, &testCancelWorkerHostRepo{host: &AgentWorkerHost{
		ID:         7,
		BaseURL:    worker.URL,
		Protocol:   AgentWorkerProtocolV1,
		CancelPath: "/cancel",
	}}, repo, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	require.NoError(t, svc.rememberRunDispatchToken(context.Background(), repo.run.ID, runToken))

	run, err := svc.CancelRun(context.Background(), repo.run.ID, repo.run.UserID, "test cancel")
	require.NoError(t, err)
	require.Equal(t, AgentRunStatusCanceled, run.Status)
	require.Equal(t, AgentRunStatusCanceled, repo.run.Status)
	require.Equal(t, repo.run.ID, received.RunID)
	require.Equal(t, runToken, received.RunToken)
	require.Equal(t, "test cancel", received.Reason)
	require.Equal(t, expectedSignature, receivedSignature)
	cancelFlag, err := rdb.Get(context.Background(), agentRunCancelKey(repo.run.ID)).Result()
	require.NoError(t, err)
	require.Equal(t, "1", cancelFlag)
}

func TestAgentRunServiceCancelQueuedRunDoesNotCallWorkerAndForgetsToken(t *testing.T) {
	runToken := "ar_cancel_queued_test"
	workerCalled := false
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer worker.Close()

	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	repo := &testAgentRunRepo{run: &AgentRun{
		ID:           1002,
		UserID:       42,
		WorkerHostID: ptrInt64(8),
		RunTokenHash: hashAgentRunTokenForTest(runToken),
		Status:       AgentRunStatusQueued,
	}}
	svc := NewAgentRunService(nil, &testCancelWorkerHostRepo{host: &AgentWorkerHost{
		ID:         8,
		BaseURL:    worker.URL,
		Protocol:   AgentWorkerProtocolV1,
		CancelPath: "/cancel",
	}}, repo, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	require.NoError(t, svc.rememberRunDispatchToken(context.Background(), repo.run.ID, runToken))

	run, err := svc.CancelRun(context.Background(), repo.run.ID, repo.run.UserID, "")
	require.NoError(t, err)
	require.Equal(t, AgentRunStatusCanceled, run.Status)
	require.False(t, workerCalled)
	cancelFlag, err := rdb.Get(context.Background(), agentRunCancelKey(repo.run.ID)).Result()
	require.NoError(t, err)
	require.Equal(t, "1", cancelFlag)

	_, err = rdb.Get(context.Background(), agentRunDispatchTokenKey(repo.run.ID)).Result()
	require.ErrorIs(t, err, redis.Nil)
}

type testCancelWorkerHostRepo struct {
	host *AgentWorkerHost
}

func (r *testCancelWorkerHostRepo) Create(context.Context, *AgentWorkerHost) error { return nil }

func (r *testCancelWorkerHostRepo) GetByID(_ context.Context, id int64) (*AgentWorkerHost, error) {
	if r.host == nil || r.host.ID != id {
		return nil, ErrAgentWorkerHostNotFound
	}
	cp := *r.host
	return &cp, nil
}

func (r *testCancelWorkerHostRepo) List(context.Context, pagination.PaginationParams, AgentWorkerHostListFilters) ([]AgentWorkerHost, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *testCancelWorkerHostRepo) ListAll(context.Context, string) ([]AgentWorkerHost, error) {
	return nil, nil
}

func (r *testCancelWorkerHostRepo) Update(context.Context, *AgentWorkerHost) error { return nil }

func (r *testCancelWorkerHostRepo) UpdateHealth(context.Context, int64, string, string, string, *int, time.Time) error {
	return nil
}

func (r *testCancelWorkerHostRepo) Delete(context.Context, int64) error { return nil }
