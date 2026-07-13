package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAgentRunServiceEnqueueRunWritesRedisStreamPayload(t *testing.T) {
	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()
	hook := &agentRunQueueXAddHook{}
	rdb.AddHook(hook)

	svc := NewAgentRunService(nil, nil, nil, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb

	err := svc.enqueueRun(context.Background(), 123, "ar_test", "https://sub2.example.com/")
	require.NoError(t, err)

	streams, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    agentRunQueueGroup,
		Consumer: "test-consumer",
		Streams:  []string{agentRunQueueStream, ">"},
		Count:    1,
		Block:    time.Millisecond,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	payload, err := decodeAgentRunQueuePayload(streams[0].Messages[0].Values)
	require.NoError(t, err)
	require.Equal(t, int64(123), payload.RunID)
	require.Equal(t, "ar_test", payload.RunToken)
	require.Equal(t, "https://sub2.example.com", payload.CallbackBaseURL)
	require.NotEmpty(t, payload.EnqueuedAt)
	require.NotEmpty(t, hook.args)
	require.NotContains(t, hook.args, "maxlen")
}

func TestAgentRunServiceRedisHostSlotRespectsLimitAndRelease(t *testing.T) {
	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	svc := NewAgentRunService(nil, nil, nil, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	host := &AgentWorkerHost{ID: 77, TimeoutSeconds: 1}

	release, err := svc.acquireRedisHostSlot(context.Background(), host, 1)
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = svc.acquireRedisHostSlot(waitCtx, host, 1)
	require.Error(t, err)

	release()
	reacquired, err := svc.acquireRedisHostSlot(context.Background(), host, 1)
	require.NoError(t, err)
	reacquired()
}

func TestAgentRunServiceClaimsPendingWithCompatibleCommands(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
		mr.Close()
	}()

	messageID := addIdleAgentRunQueueMessage(t, mr, rdb)

	svc := NewAgentRunService(nil, nil, nil, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	messages, nextStart, err := svc.claimAgentRunPendingMessages(context.Background(), "claim-consumer", "0-0")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, messageID, messages[0].ID)
	require.Equal(t, "0-0", nextStart)

	pending, err := rdb.XPendingExt(context.Background(), &redis.XPendingExtArgs{
		Stream: agentRunQueueStream,
		Group:  agentRunQueueGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "claim-consumer", pending[0].Consumer)
}

func TestAgentRunServiceHandleQueueMessageKeepsPendingOnRetryableFailure(t *testing.T) {
	tests := []struct {
		name       string
		getRunErr  error
		panicOnGet bool
	}{
		{name: "database read error", getRunErr: errors.New("database unavailable")},
		{name: "repository panic", panicOnGet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdb, cleanup := newAgentRunQueueTestRedis(t)
			defer cleanup()

			repo := &agentRunQueueDispatchFailureRepo{
				testAgentRunRepo: &testAgentRunRepo{},
				getRunErr:        tt.getRunErr,
				panicOnGet:       tt.panicOnGet,
			}
			svc := NewAgentRunService(nil, nil, repo, nil, nil, disabledAgentArtifactStore{}, nil)
			svc.redisClient = rdb
			message := enqueuePendingAgentRunQueueMessage(t, svc, rdb, 101, "ar_retry", "https://sub2.example.com")

			svc.handleAgentRunQueueMessage(context.Background(), "retry-consumer", message)

			requireAgentRunQueueMessagePending(t, rdb, message.ID, true)
			token, err := rdb.Get(context.Background(), agentRunDispatchTokenKey(101)).Result()
			require.NoError(t, err)
			require.Equal(t, "ar_retry", token)
		})
	}
}

func TestAgentRunServiceHandleQueueMessageAcknowledgesTerminalRun(t *testing.T) {
	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	repo := &testAgentRunRepo{run: &AgentRun{
		ID:     102,
		UserID: 202,
		Status: AgentRunStatusFailed,
	}}
	svc := NewAgentRunService(nil, nil, repo, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	message := enqueuePendingAgentRunQueueMessage(t, svc, rdb, repo.run.ID, "ar_terminal", "https://sub2.example.com")

	svc.handleAgentRunQueueMessage(context.Background(), "terminal-consumer", message)

	requireAgentRunQueueMessagePending(t, rdb, message.ID, false)
	_, err := rdb.Get(context.Background(), agentRunDispatchTokenKey(repo.run.ID)).Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestAgentRunServiceHandleQueueMessageAcknowledgesAcceptedRun(t *testing.T) {
	workerCalled := make(chan struct{}, 1)
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		workerCalled <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true,"status":"running"}`))
	}))
	defer workerServer.Close()

	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	hostID := int64(303)
	repo := &testAgentRunRepo{run: &AgentRun{
		ID:                103,
		AppID:             203,
		AppVersionID:      204,
		UserID:            205,
		APIKeyID:          206,
		WorkerHostID:      &hostID,
		Status:            AgentRunStatusQueued,
		InputSummaryJSON:  map[string]any{},
		OutputSummaryJSON: map[string]any{},
	}}
	appRepo := &testAgentAppRepo{version: &AgentAppVersion{
		ID:           repo.run.AppVersionID,
		AppID:        repo.run.AppID,
		WorkerHostID: &hostID,
		WorkerRoute:  "/run",
	}}
	hostRepo := &testCancelWorkerHostRepo{host: &AgentWorkerHost{
		ID:             hostID,
		BaseURL:        workerServer.URL,
		Protocol:       AgentWorkerProtocolV1,
		MaxConcurrency: 1,
		TimeoutSeconds: 5,
		Status:         AgentWorkerHostStatusActive,
	}}
	svc := NewAgentRunService(appRepo, hostRepo, repo, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	message := enqueuePendingAgentRunQueueMessage(t, svc, rdb, repo.run.ID, "ar_accepted", workerServer.URL)

	svc.handleAgentRunQueueMessage(context.Background(), "accepted-consumer", message)

	select {
	case <-workerCalled:
	default:
		t.Fatal("worker was not called")
	}
	requireAgentRunQueueMessagePending(t, rdb, message.ID, false)
	token, err := rdb.Get(context.Background(), agentRunDispatchTokenKey(repo.run.ID)).Result()
	require.NoError(t, err)
	require.Equal(t, "ar_accepted", token)
}

func TestAgentRunServiceHandleQueueMessageAcknowledgesSynchronousSuccess(t *testing.T) {
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"succeeded","metadata":{"result":"ok"}}`))
	}))
	defer workerServer.Close()

	rdb, cleanup := newAgentRunQueueTestRedis(t)
	defer cleanup()

	hostID := int64(304)
	run := newAgentRunQueueDispatchTestRun(104, hostID)
	repo := &testAgentRunRepo{run: run}
	svc := newAgentRunQueueDispatchTestService(rdb, repo, run, workerServer.URL)
	message := enqueuePendingAgentRunQueueMessage(t, svc, rdb, run.ID, "ar_succeeded", workerServer.URL)

	svc.handleAgentRunQueueMessage(context.Background(), "succeeded-consumer", message)

	requireAgentRunQueueMessagePending(t, rdb, message.ID, false)
	_, err := rdb.Get(context.Background(), agentRunDispatchTokenKey(run.ID)).Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestAgentRunServiceHandleQueueMessageRejectsInvalidWorkerResponse(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		expectedCode string
	}{
		{name: "empty body", expectedCode: "WORKER_RESPONSE_INVALID"},
		{name: "invalid json", body: "not-json", expectedCode: "WORKER_RESPONSE_INVALID"},
		{name: "missing status", body: `{}`, expectedCode: "WORKER_RESPONSE_INVALID"},
		{name: "not accepted", body: `{"accepted":false,"status":"running","message":"capacity full"}`, expectedCode: "WORKER_REJECTED"},
		{name: "unsupported async status", body: `{"accepted":true,"status":"queued"}`, expectedCode: "WORKER_RESPONSE_INVALID"},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer workerServer.Close()

			rdb, cleanup := newAgentRunQueueTestRedis(t)
			defer cleanup()

			hostID := int64(400 + index)
			run := newAgentRunQueueDispatchTestRun(int64(200+index), hostID)
			repo := &agentRunQueueResponseRepo{testAgentRunRepo: &testAgentRunRepo{run: run}}
			svc := newAgentRunQueueDispatchTestService(rdb, repo, run, workerServer.URL)
			message := enqueuePendingAgentRunQueueMessage(t, svc, rdb, run.ID, "ar_invalid", workerServer.URL)

			svc.handleAgentRunQueueMessage(context.Background(), "invalid-response-consumer", message)

			requireAgentRunQueueMessagePending(t, rdb, message.ID, false)
			require.Equal(t, tt.expectedCode, repo.failedCode)
			require.Equal(t, AgentRunStatusFailed, repo.run.Status)
			_, err := rdb.Get(context.Background(), agentRunDispatchTokenKey(run.ID)).Result()
			require.ErrorIs(t, err, redis.Nil)
		})
	}
}

func addIdleAgentRunQueueMessage(t *testing.T, mr *miniredis.Miniredis, rdb *redis.Client) string {
	t.Helper()
	baseTime := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	mr.SetTime(baseTime)
	require.NoError(t, rdb.XGroupCreateMkStream(context.Background(), agentRunQueueStream, agentRunQueueGroup, "0").Err())
	messageID, err := rdb.XAdd(context.Background(), &redis.XAddArgs{
		Stream: agentRunQueueStream,
		Values: map[string]any{"payload": "test"},
	}).Result()
	require.NoError(t, err)
	streams, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    agentRunQueueGroup,
		Consumer: "original-consumer",
		Streams:  []string{agentRunQueueStream, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)
	mr.SetTime(baseTime.Add(agentRunQueueClaimIdle + time.Second))
	return messageID
}

func enqueuePendingAgentRunQueueMessage(t *testing.T, svc *AgentRunService, rdb *redis.Client, runID int64, runToken, callbackBaseURL string) redis.XMessage {
	t.Helper()
	require.NoError(t, svc.enqueueRun(context.Background(), runID, runToken, callbackBaseURL))
	streams, err := rdb.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    agentRunQueueGroup,
		Consumer: "original-consumer",
		Streams:  []string{agentRunQueueStream, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)
	return streams[0].Messages[0]
}

func requireAgentRunQueueMessagePending(t *testing.T, rdb *redis.Client, messageID string, expected bool) {
	t.Helper()
	pending, err := rdb.XPendingExt(context.Background(), &redis.XPendingExtArgs{
		Stream: agentRunQueueStream,
		Group:  agentRunQueueGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	require.NoError(t, err)
	if expected {
		require.Len(t, pending, 1)
		require.Equal(t, messageID, pending[0].ID)
		require.Equal(t, int64(1), rdb.XLen(context.Background(), agentRunQueueStream).Val())
		return
	}
	require.Empty(t, pending)
	require.Zero(t, rdb.XLen(context.Background(), agentRunQueueStream).Val())
}

func newAgentRunQueueDispatchTestRun(runID, hostID int64) *AgentRun {
	return &AgentRun{
		ID:                runID,
		AppID:             runID + 1000,
		AppVersionID:      runID + 2000,
		UserID:            runID + 3000,
		APIKeyID:          runID + 4000,
		WorkerHostID:      &hostID,
		Status:            AgentRunStatusQueued,
		InputSummaryJSON:  map[string]any{},
		OutputSummaryJSON: map[string]any{},
	}
}

func newAgentRunQueueDispatchTestService(rdb *redis.Client, runRepo AgentRunRepository, run *AgentRun, workerURL string) *AgentRunService {
	hostID := *run.WorkerHostID
	appRepo := &testAgentAppRepo{version: &AgentAppVersion{
		ID:           run.AppVersionID,
		AppID:        run.AppID,
		WorkerHostID: &hostID,
		WorkerRoute:  "/run",
	}}
	hostRepo := &testCancelWorkerHostRepo{host: &AgentWorkerHost{
		ID:             hostID,
		BaseURL:        workerURL,
		Protocol:       AgentWorkerProtocolV1,
		MaxConcurrency: 1,
		TimeoutSeconds: 5,
		Status:         AgentWorkerHostStatusActive,
	}}
	svc := NewAgentRunService(appRepo, hostRepo, runRepo, nil, nil, disabledAgentArtifactStore{}, nil)
	svc.redisClient = rdb
	return svc
}

type agentRunQueueDispatchFailureRepo struct {
	*testAgentRunRepo
	getRunErr  error
	panicOnGet bool
}

type agentRunQueueResponseRepo struct {
	*testAgentRunRepo
	failedCode    string
	failedMessage string
}

func (r *agentRunQueueResponseRepo) MarkFailed(_ context.Context, id int64, code, message string, _ time.Time) error {
	r.failedCode = code
	r.failedMessage = message
	if r.run != nil && r.run.ID == id {
		r.run.Status = AgentRunStatusFailed
		r.run.ErrorCode = code
		r.run.ErrorMessage = message
	}
	return nil
}

type agentRunQueueXAddHook struct {
	args []string
}

func (h *agentRunQueueXAddHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *agentRunQueueXAddHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "xadd" {
			for _, arg := range cmd.Args() {
				h.args = append(h.args, strings.ToLower(agentStringFromAny(arg)))
			}
		}
		return next(ctx, cmd)
	}
}

func (h *agentRunQueueXAddHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (r *agentRunQueueDispatchFailureRepo) GetRunByID(ctx context.Context, id int64) (*AgentRun, error) {
	if r.panicOnGet {
		panic("repository panic")
	}
	if r.getRunErr != nil {
		return nil, r.getRunErr
	}
	return r.testAgentRunRepo.GetRunByID(ctx, id)
}

func newAgentRunQueueTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, func() {
		require.NoError(t, rdb.Close())
		mr.Close()
	}
}
