package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	defaultAgentRunTTL          = 7 * 24 * time.Hour
	defaultAgentWorkerHTTPGrace = 5 * time.Second
	defaultAgentRunQueueWorkers = 4
	agentRunQueueBlockTimeout   = 5 * time.Second
	agentRunQueueClaimIdle      = 30 * time.Minute
	agentRunQueueClaimBatchSize = int64(10)
	agentHostSlotPollInterval   = 250 * time.Millisecond
	defaultAgentCleanupInterval = time.Hour
	agentRunLeaseSafetyWindow   = 2 * time.Minute
	agentRunLeaseReleaseTimeout = 2 * time.Second
)

const (
	agentRunQueueStream = "agent:runs:stream"
	agentRunQueueGroup  = "agent-runners"
)

var (
	agentHostSlotAcquireScript = redis.NewScript(`
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
local current = redis.call("ZCARD", KEYS[1])
if current < tonumber(ARGV[2]) then
  redis.call("ZADD", KEYS[1], ARGV[3], ARGV[4])
  redis.call("PEXPIRE", KEYS[1], ARGV[5])
  return 1
end
return 0
`)
	agentHostSlotReleaseScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
return 1
`)
	agentRunDispatchLeaseReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)
)

type AgentRunService struct {
	appRepo        AgentAppRepository
	workerHostRepo AgentWorkerHostRepository
	runRepo        AgentRunRepository
	apiKeyService  AgentRunAPIKeyService
	modelProxy     ModelProxyGatewayCaller
	artifactStore  AgentArtifactStore
	artifactConfig *AgentArtifactStorageConfigService
	cfg            *config.Config
	redisClient    *redis.Client
	httpClient     *http.Client
	previewClient  *http.Client

	mu                   sync.Mutex
	hostSemaphores       map[int64]chan struct{}
	previewMu            sync.Mutex
	previewActiveStreams map[int64]int
	runnerOnce           sync.Once
	cleanupOnce          sync.Once
	consumerID           string
}

func NewAgentRunService(
	appRepo AgentAppRepository,
	workerHostRepo AgentWorkerHostRepository,
	runRepo AgentRunRepository,
	apiKeyService AgentRunAPIKeyService,
	modelProxy ModelProxyGatewayCaller,
	artifactStore AgentArtifactStore,
	cfg *config.Config,
	redisClients ...*redis.Client,
) *AgentRunService {
	if artifactStore == nil {
		artifactStore = disabledAgentArtifactStore{}
	}
	var redisClient *redis.Client
	if len(redisClients) > 0 {
		redisClient = redisClients[0]
	}
	svc := &AgentRunService{
		appRepo:        appRepo,
		workerHostRepo: workerHostRepo,
		runRepo:        runRepo,
		apiKeyService:  apiKeyService,
		modelProxy:     modelProxy,
		artifactStore:  artifactStore,
		cfg:            cfg,
		redisClient:    redisClient,
		consumerID:     newAgentRunQueueConsumerID(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		previewClient:        newArtifactPreviewHTTPClient(),
		hostSemaphores:       make(map[int64]chan struct{}),
		previewActiveStreams: make(map[int64]int),
	}
	if dynamic, ok := artifactStore.(*dynamicAgentArtifactStore); ok {
		svc.artifactConfig = dynamic.configService
	}
	svc.startRedisRunner()
	svc.startAgentCleanupRunner()
	return svc
}

type CreateAgentRunInput struct {
	AppID           int64
	VersionID       int64
	UserID          int64
	APIKeyID        int64
	APIKeyBindings  []CreateAgentRunKeyBindingInput
	InputAssetIDs   []int64
	Input           map[string]any
	InputRefURL     string
	CallbackBaseURL string
}

func (s *AgentRunService) ListPublishedApps(ctx context.Context, params pagination.PaginationParams, filters AgentAppListFilters) ([]AgentApp, *pagination.PaginationResult, error) {
	filters.Status = AgentAppStatusPublished
	filters.Visibility = AgentAppVisibilityPublic
	filters.RequirePublishedVersion = true
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}
	return s.appRepo.ListApps(ctx, params, filters)
}

func (s *AgentRunService) GetPublishedApp(ctx context.Context, appID int64) (*AgentApp, *AgentAppVersion, error) {
	return s.appRepo.GetPublishedVersionForApp(ctx, appID, 0)
}

func (s *AgentRunService) GetPublishedAppIconURL(ctx context.Context, appID int64) (*AgentAppIconURL, error) {
	app, _, err := s.appRepo.GetPublishedVersionForApp(ctx, appID, 0)
	if err != nil {
		return nil, err
	}
	return resolveAgentAppIconURL(ctx, s.artifactStore, app, s.artifactDownloadTTL())
}

func (s *AgentRunService) CreateRun(ctx context.Context, input CreateAgentRunInput) (*AgentRun, error) {
	if input.AppID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_RUN_APP_ID_INVALID", "app id is invalid")
	}
	if input.UserID <= 0 {
		return nil, infraerrors.Unauthorized("AGENT_RUN_USER_REQUIRED", "user is required")
	}
	if input.APIKeyID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_RUN_API_KEY_REQUIRED", "please select a platform api key")
	}

	app, version, err := s.appRepo.GetPublishedVersionForApp(ctx, input.AppID, input.VersionID)
	if err != nil {
		return nil, err
	}
	if version.RuntimeType != AgentRuntimeTypeWorker {
		return nil, infraerrors.BadRequest("AGENT_RUN_RUNTIME_UNSUPPORTED", "only worker runtime is supported")
	}
	if version.WorkerHostID == nil || *version.WorkerHostID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_RUN_WORKER_HOST_REQUIRED", "worker host is required")
	}
	host, err := s.workerHostRepo.GetByID(ctx, *version.WorkerHostID)
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_RUN_WORKER_HOST_UNAVAILABLE", "应用执行服务暂时不可用，请稍后重试")
	}
	if host.Status != AgentWorkerHostStatusActive {
		return nil, infraerrors.BadRequest("AGENT_RUN_WORKER_HOST_INACTIVE", "应用执行服务当前不可用，请联系管理员")
	}

	inputAssets, err := s.loadInputAssetsForRun(ctx, input.UserID, app.ID, input.InputAssetIDs)
	if err != nil {
		return nil, err
	}
	keyBindings, err := s.prepareRunKeyBindings(ctx, input, version)
	if err != nil {
		return nil, err
	}

	runToken, tokenHash, err := newAgentRunToken()
	if err != nil {
		return nil, fmt.Errorf("generate agent run token: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(defaultAgentRunTTL)
	workerHostID := *version.WorkerHostID
	inputSummary := copyAgentMap(input.Input)
	attachInputAssetsToSummary(inputSummary, inputAssets)
	run := &AgentRun{
		AppID:             app.ID,
		AppVersionID:      version.ID,
		UserID:            input.UserID,
		APIKeyID:          input.APIKeyID,
		WorkerHostID:      &workerHostID,
		RunTokenHash:      tokenHash,
		Status:            AgentRunStatusQueued,
		InputRefURL:       strings.TrimSpace(input.InputRefURL),
		InputSummaryJSON:  inputSummary,
		OutputSummaryJSON: map[string]any{},
		UsageJSON:         map[string]any{},
		ExpiresAt:         &expiresAt,
	}
	if err := s.runRepo.CreateRunWithKeyBindings(ctx, run, keyBindings); err != nil {
		return nil, fmt.Errorf("create agent run: %w", err)
	}
	s.recordRunEvent(ctx, run, AgentRunEventQueued, AgentRunStatusQueued, "", "", "run queued", nil, map[string]any{
		"app_id":             app.ID,
		"app_version_id":     version.ID,
		"worker_host_id":     workerHostID,
		"input_asset_count":  len(inputAssets),
		"model_policy_count": len(modelPolicyMap(version.NodeModelPolicyJSON)),
	})

	callbackBaseURL := strings.TrimRight(strings.TrimSpace(input.CallbackBaseURL), "/")
	if err := s.enqueueRun(ctx, run.ID, runToken, callbackBaseURL); err != nil {
		log.Printf("[AgentRunRunner] enqueue failed, falling back to in-process dispatch: run_id=%d err=%v", run.ID, err)
		s.recordRunEvent(ctx, run, AgentRunEventDispatching, AgentRunStatusQueued, "", "", "queue unavailable, dispatching in process", nil, map[string]any{
			"error": err.Error(),
		})
		go s.dispatchRunFallback(run.ID, runToken, callbackBaseURL)
	}

	return run, nil
}

func (s *AgentRunService) ListRuns(ctx context.Context, userID int64, params pagination.PaginationParams, filters AgentRunListFilters) ([]AgentRun, *pagination.PaginationResult, error) {
	return s.runRepo.ListRunsByUser(ctx, userID, params, filters)
}

func (s *AgentRunService) ListRunsForAdmin(ctx context.Context, params pagination.PaginationParams, filters AgentRunListFilters) ([]AgentRun, *pagination.PaginationResult, error) {
	return s.runRepo.ListRuns(ctx, params, filters)
}

func (s *AgentRunService) GetRunForUser(ctx context.Context, runID, userID int64) (*AgentRun, []AgentArtifact, error) {
	run, err := s.runRepo.GetRunByIDForUser(ctx, runID, userID)
	if err != nil {
		return nil, nil, err
	}
	artifacts, err := s.runRepo.ListArtifactsByRun(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	return run, artifacts, nil
}

func (s *AgentRunService) ListRunEventsForUser(ctx context.Context, runID, userID int64, params pagination.PaginationParams) ([]AgentRunEvent, *pagination.PaginationResult, error) {
	if runID <= 0 {
		return nil, nil, infraerrors.BadRequest("AGENT_RUN_ID_INVALID", "run id is invalid")
	}
	if userID <= 0 {
		return nil, nil, infraerrors.Unauthorized("AGENT_RUN_USER_REQUIRED", "user is required")
	}
	if _, err := s.runRepo.GetRunByIDForUser(ctx, runID, userID); err != nil {
		return nil, nil, err
	}
	if params.PageSize <= 0 || params.PageSize > 200 {
		params.PageSize = 100
	}
	return s.runRepo.ListRunEventsByRunForUser(ctx, runID, userID, params)
}

func (s *AgentRunService) CancelRun(ctx context.Context, runID, userID int64, reason string) (*AgentRun, error) {
	if runID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_RUN_ID_INVALID", "run id is invalid")
	}
	if userID <= 0 {
		return nil, infraerrors.Unauthorized("AGENT_RUN_USER_REQUIRED", "user is required")
	}
	run, err := s.runRepo.GetRunByIDForUser(ctx, runID, userID)
	if err != nil {
		return nil, err
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "user canceled run"
	}
	if isAgentRunTerminal(run.Status) {
		if run.Status == AgentRunStatusCanceled {
			s.requestRunCancel(ctx, run.ID)
			runToken := s.getRunDispatchToken(ctx, run.ID)
			s.recordRunEvent(ctx, run, AgentRunEventCanceled, run.Status, "", "", reason, nil, map[string]any{
				"already_canceled": true,
			})
			s.propagateRunCancel(ctx, run, runToken, reason)
			return run, nil
		}
		return nil, infraerrors.Conflict("AGENT_RUN_CANCEL_CONFLICT", "run is already completed")
	}

	previousStatus := run.Status
	s.requestRunCancel(ctx, run.ID)
	runToken := s.getRunDispatchToken(ctx, run.ID)
	now := time.Now().UTC()
	if err := s.runRepo.MarkCanceled(ctx, run.ID, userID, "USER_CANCELED", reason, now); err != nil {
		current, getErr := s.runRepo.GetRunByIDForUser(ctx, runID, userID)
		if getErr == nil && current.Status == AgentRunStatusCanceled {
			s.propagateRunCancel(ctx, current, runToken, reason)
			return current, nil
		}
		if getErr == nil && isAgentRunTerminal(current.Status) {
			return nil, infraerrors.Conflict("AGENT_RUN_CANCEL_CONFLICT", "run is already completed")
		}
		return nil, err
	}

	run.Status = AgentRunStatusCanceled
	run.ErrorCode = "USER_CANCELED"
	run.ErrorMessage = reason
	run.CompletedAt = &now
	s.recordRunEvent(ctx, run, AgentRunEventCanceled, AgentRunStatusCanceled, "", "", reason, nil, map[string]any{
		"previous_status": previousStatus,
	})
	if previousStatus == AgentRunStatusRunning {
		s.propagateRunCancel(ctx, run, runToken, reason)
	} else {
		s.forgetRunDispatchToken(context.Background(), run.ID)
	}

	refreshed, err := s.runRepo.GetRunByIDForUser(ctx, runID, userID)
	if err != nil {
		return run, nil
	}
	return refreshed, nil
}

type agentRunQueuePayload struct {
	RunID           int64  `json:"run_id"`
	RunToken        string `json:"run_token"`
	CallbackBaseURL string `json:"callback_base_url"`
	EnqueuedAt      string `json:"enqueued_at"`
}

func (s *AgentRunService) enqueueRun(ctx context.Context, runID int64, runToken, callbackBaseURL string) error {
	if s == nil || s.redisClient == nil {
		return errors.New("agent run redis queue is not configured")
	}
	if runID <= 0 || strings.TrimSpace(runToken) == "" {
		return errors.New("agent run queue payload is invalid")
	}
	payload := agentRunQueuePayload{
		RunID:           runID,
		RunToken:        strings.TrimSpace(runToken),
		CallbackBaseURL: strings.TrimRight(strings.TrimSpace(callbackBaseURL), "/"),
		EnqueuedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := s.ensureAgentRunQueueGroup(ctx); err != nil {
		return err
	}
	if err := s.rememberRunDispatchToken(ctx, runID, runToken); err != nil {
		return err
	}
	return s.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: agentRunQueueStream,
		Values: map[string]any{
			"run_id":  fmt.Sprintf("%d", runID),
			"payload": string(raw),
		},
	}).Err()
}

func (s *AgentRunService) startRedisRunner() {
	if s == nil || s.redisClient == nil {
		return
	}
	s.runnerOnce.Do(func() {
		for i := 0; i < defaultAgentRunQueueWorkers; i++ {
			consumerID := fmt.Sprintf("%s-%d", s.consumerID, i+1)
			go s.agentRunQueueLoop(consumerID)
		}
		go s.agentRunPendingClaimLoop(s.consumerID + "-claim")
	})
}

func (s *AgentRunService) runAgentCleanupOnce(ctx context.Context, now time.Time) AgentRunCleanupResult {
	result := AgentRunCleanupResult{}
	if s == nil || s.runRepo == nil {
		return result
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	batchSize := opsCleanupBatchSize

	if s.cleanupExpiredArtifactsEnabled() {
		artifactRefs, err := s.runRepo.ListExpiredArtifacts(ctx, now, batchSize)
		if err != nil {
			log.Printf("[AgentCleanup] list expired artifacts failed: err=%v", err)
		} else {
			objectsDeleted, objectErrors := s.deleteAgentCleanupObjects(ctx, artifactRefs)
			result.ObjectsDeleted += objectsDeleted
			result.ObjectDeleteErrors += objectErrors
			deleted, err := s.runRepo.MarkArtifactsDeleted(ctx, agentCleanupObjectIDs(artifactRefs), now)
			if err != nil {
				log.Printf("[AgentCleanup] mark expired artifacts deleted failed: err=%v", err)
			} else {
				result.ArtifactsDeleted = deleted
			}
		}

		inputRefs, err := s.runRepo.ListExpiredInputAssets(ctx, now, batchSize)
		if err != nil {
			log.Printf("[AgentCleanup] list expired input assets failed: err=%v", err)
		} else {
			objectsDeleted, objectErrors := s.deleteAgentCleanupObjects(ctx, inputRefs)
			result.ObjectsDeleted += objectsDeleted
			result.ObjectDeleteErrors += objectErrors
			deleted, err := s.runRepo.MarkInputAssetsDeleted(ctx, agentCleanupObjectIDs(inputRefs), now)
			if err != nil {
				log.Printf("[AgentCleanup] mark expired input assets deleted failed: err=%v", err)
			} else {
				result.InputAssetsDeleted = deleted
			}
		}
	}

	if result.ArtifactsDeleted > 0 || result.InputAssetsDeleted > 0 || result.ObjectDeleteErrors > 0 {
		log.Printf("[AgentCleanup] complete: artifacts=%d input_assets=%d objects=%d object_errors=%d",
			result.ArtifactsDeleted,
			result.InputAssetsDeleted,
			result.ObjectsDeleted,
			result.ObjectDeleteErrors,
		)
	}
	return result
}

func (s *AgentRunService) startAgentCleanupRunner() {
	if s == nil || s.runRepo == nil {
		return
	}
	if s.artifactConfig == nil && (s.cfg == nil || !s.cfg.AgentArtifacts.CleanupExpiredArtifactsEnabled) {
		return
	}
	s.cleanupOnce.Do(func() {
		go s.agentCleanupLoop(context.Background(), defaultAgentCleanupInterval)
	})
}

func (s *AgentRunService) agentCleanupLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultAgentCleanupInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.runAgentCleanupOnce(ctx, now.UTC())
		}
	}
}

func (s *AgentRunService) agentRunQueueLoop(consumerID string) {
	ctx := context.Background()
	if err := s.ensureAgentRunQueueGroup(ctx); err != nil {
		log.Printf("[AgentRunRunner] create queue group failed: consumer=%s err=%v", consumerID, err)
	}
	for {
		streams, err := s.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    agentRunQueueGroup,
			Consumer: consumerID,
			Streams:  []string{agentRunQueueStream, ">"},
			Count:    1,
			Block:    agentRunQueueBlockTimeout,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			if isAgentRunQueueNoGroupError(err) {
				_ = s.ensureAgentRunQueueGroup(ctx)
				time.Sleep(time.Second)
				continue
			}
			log.Printf("[AgentRunRunner] read queue failed: consumer=%s err=%v", consumerID, err)
			time.Sleep(time.Second)
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				s.handleAgentRunQueueMessage(ctx, consumerID, message)
			}
		}
	}
}

func (s *AgentRunService) agentRunPendingClaimLoop(consumerID string) {
	ctx := context.Background()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		start := "0-0"
		for {
			messages, nextStart, err := s.claimAgentRunPendingMessages(ctx, consumerID, start)
			if err != nil {
				if !errors.Is(err, redis.Nil) {
					log.Printf("[AgentRunRunner] claim pending failed: consumer=%s err=%v", consumerID, err)
				}
				break
			}
			for _, message := range messages {
				s.handleAgentRunQueueMessage(ctx, consumerID, message)
			}
			if nextStart == "" || nextStart == "0-0" || nextStart == start {
				break
			}
			start = nextStart
		}
	}
}

func (s *AgentRunService) claimAgentRunPendingMessages(ctx context.Context, consumerID, start string) ([]redis.XMessage, string, error) {
	queryStart := strings.TrimSpace(start)
	skipID := ""
	if queryStart == "" || queryStart == "0-0" {
		queryStart = "-"
	} else {
		skipID = queryStart
	}

	scanCount := agentRunQueueClaimBatchSize * 10
	pending, err := s.redisClient.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: agentRunQueueStream,
		Group:  agentRunQueueGroup,
		Start:  queryStart,
		End:    "+",
		Count:  scanCount,
	}).Result()
	if err != nil {
		return nil, "", err
	}
	if len(pending) == 0 {
		return nil, "0-0", nil
	}

	messageIDs := make([]string, 0, agentRunQueueClaimBatchSize)
	lastScannedID := ""
	for _, entry := range pending {
		lastScannedID = entry.ID
		if entry.ID == skipID {
			continue
		}
		if entry.Idle >= agentRunQueueClaimIdle {
			messageIDs = append(messageIDs, entry.ID)
			if int64(len(messageIDs)) >= agentRunQueueClaimBatchSize {
				break
			}
		}
	}

	nextStart := "0-0"
	if lastScannedID != "" && (int64(len(pending)) >= scanCount || int64(len(messageIDs)) >= agentRunQueueClaimBatchSize) {
		nextStart = lastScannedID
	}
	if len(messageIDs) == 0 {
		return nil, nextStart, nil
	}

	messages, err := s.redisClient.XClaim(ctx, &redis.XClaimArgs{
		Stream:   agentRunQueueStream,
		Group:    agentRunQueueGroup,
		Consumer: consumerID,
		MinIdle:  agentRunQueueClaimIdle,
		Messages: messageIDs,
	}).Result()
	if err != nil {
		return nil, "", err
	}
	return messages, nextStart, nil
}

func (s *AgentRunService) handleAgentRunQueueMessage(ctx context.Context, consumerID string, message redis.XMessage) {
	payload, err := decodeAgentRunQueuePayload(message.Values)
	if err != nil {
		log.Printf("[AgentRunRunner] invalid queue message: consumer=%s id=%s err=%v", consumerID, message.ID, err)
		s.ackAgentRunQueueMessage(ctx, message.ID)
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[AgentRunRunner] dispatch panic: consumer=%s id=%s run_id=%d panic=%v", consumerID, message.ID, payload.RunID, recovered)
		}
	}()
	if s.dispatchRun(payload.RunID, payload.RunToken, payload.CallbackBaseURL) {
		s.ackAgentRunQueueMessage(ctx, message.ID)
	}
}

func (s *AgentRunService) ackAgentRunQueueMessage(ctx context.Context, messageID string) {
	if strings.TrimSpace(messageID) == "" || s == nil || s.redisClient == nil {
		return
	}
	if err := s.redisClient.XAck(ctx, agentRunQueueStream, agentRunQueueGroup, messageID).Err(); err != nil {
		log.Printf("[AgentRunRunner] ack message failed: id=%s err=%v", messageID, err)
		return
	}
	if err := s.redisClient.XDel(ctx, agentRunQueueStream, messageID).Err(); err != nil {
		log.Printf("[AgentRunRunner] delete message failed: id=%s err=%v", messageID, err)
	}
}

func (s *AgentRunService) ensureAgentRunQueueGroup(ctx context.Context) error {
	if s == nil || s.redisClient == nil {
		return errors.New("agent run redis queue is not configured")
	}
	err := s.redisClient.XGroupCreateMkStream(ctx, agentRunQueueStream, agentRunQueueGroup, "0").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
		return nil
	}
	return err
}

func isAgentRunQueueNoGroupError(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOGROUP")
}

func decodeAgentRunQueuePayload(values map[string]any) (agentRunQueuePayload, error) {
	var payload agentRunQueuePayload
	raw := agentStringFromAny(values["payload"])
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return payload, err
		}
	}
	if payload.RunID <= 0 {
		payload.RunID = int64FromAny(values["run_id"])
	}
	payload.RunToken = strings.TrimSpace(payload.RunToken)
	payload.CallbackBaseURL = strings.TrimRight(strings.TrimSpace(payload.CallbackBaseURL), "/")
	if payload.RunID <= 0 || payload.RunToken == "" {
		return payload, errors.New("run_id or run_token is empty")
	}
	return payload, nil
}

func (s *AgentRunService) rememberRunDispatchToken(ctx context.Context, runID int64, runToken string) error {
	if s == nil || s.redisClient == nil {
		return errors.New("agent run redis token index is not configured")
	}
	runToken = strings.TrimSpace(runToken)
	if runID <= 0 || runToken == "" {
		return errors.New("agent run token index payload is invalid")
	}
	return s.redisClient.Set(ctx, agentRunDispatchTokenKey(runID), runToken, defaultAgentRunTTL).Err()
}

func (s *AgentRunService) getRunDispatchToken(ctx context.Context, runID int64) string {
	if s == nil || s.redisClient == nil || runID <= 0 {
		return ""
	}
	token, err := s.redisClient.Get(ctx, agentRunDispatchTokenKey(runID)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("[AgentRunRunner] read run token index failed: run_id=%d err=%v", runID, err)
		}
		return ""
	}
	return strings.TrimSpace(token)
}

func (s *AgentRunService) forgetRunDispatchToken(ctx context.Context, runID int64) {
	if s == nil || s.redisClient == nil || runID <= 0 {
		return
	}
	if err := s.redisClient.Del(ctx, agentRunDispatchTokenKey(runID)).Err(); err != nil {
		log.Printf("[AgentRunRunner] delete run token index failed: run_id=%d err=%v", runID, err)
	}
}

func agentRunDispatchTokenKey(runID int64) string {
	return fmt.Sprintf("agent:run:%d:dispatch-token", runID)
}

func (s *AgentRunService) requestRunCancel(_ context.Context, runID int64) {
	if runID <= 0 {
		return
	}
	if s == nil || s.redisClient == nil {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.redisClient.Set(cancelCtx, agentRunCancelKey(runID), "1", defaultAgentRunTTL).Err(); err != nil {
		log.Printf("[AgentRunRunner] set run cancel flag failed: run_id=%d err=%v", runID, err)
	}
}

func (s *AgentRunService) isRunCancelRequested(ctx context.Context, runID int64) bool {
	if s == nil || s.redisClient == nil || runID <= 0 {
		return false
	}
	value, err := s.redisClient.Get(ctx, agentRunCancelKey(runID)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("[AgentRunRunner] read run cancel flag failed: run_id=%d err=%v", runID, err)
		}
		return false
	}
	return strings.TrimSpace(value) != ""
}

func agentRunCancelKey(runID int64) string {
	return fmt.Sprintf("agent:run:%d:cancel-requested", runID)
}

func (s *AgentRunService) propagateRunCancel(ctx context.Context, run *AgentRun, runToken, reason string) {
	if s == nil || run == nil || run.WorkerHostID == nil || *run.WorkerHostID <= 0 {
		return
	}
	runToken = strings.TrimSpace(runToken)
	if runToken == "" {
		log.Printf("[AgentRunRunner] cannot propagate cancel without run token: run_id=%d", run.ID)
		return
	}
	if s.workerHostRepo == nil {
		return
	}
	host, err := s.workerHostRepo.GetByID(ctx, *run.WorkerHostID)
	if err != nil {
		log.Printf("[AgentRunRunner] load worker host for cancel failed: run_id=%d host_id=%d err=%v", run.ID, *run.WorkerHostID, err)
		return
	}
	cancelPath := strings.TrimSpace(host.CancelPath)
	if cancelPath == "" {
		cancelPath = "/cancel"
	}
	cancelURL, err := joinWorkerURL(host.BaseURL, cancelPath)
	if err != nil {
		log.Printf("[AgentRunRunner] build worker cancel url failed: run_id=%d err=%v", run.ID, err)
		return
	}
	body, err := json.Marshal(WorkerCancelRequest{
		RunID:    run.ID,
		RunToken: runToken,
		Reason:   reason,
	})
	if err != nil {
		log.Printf("[AgentRunRunner] marshal worker cancel payload failed: run_id=%d err=%v", run.ID, err)
		return
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(cancelCtx, http.MethodPost, cancelURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("[AgentRunRunner] create worker cancel request failed: run_id=%d err=%v", run.ID, err)
		return
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Sub2API-Worker-Protocol", host.Protocol)
	httpReq.Header.Set("X-Sub2API-Run-ID", fmt.Sprintf("%d", run.ID))
	httpReq.Header.Set("X-Sub2API-Run-Token", runToken)
	httpReq.Header.Set("X-Sub2API-Timestamp", timestamp)
	httpReq.Header.Set("X-Sub2API-Signature", signWorkerPayload(runToken, timestamp, body))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[AgentRunRunner] worker cancel request failed: run_id=%d err=%v", run.ID, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		log.Printf("[AgentRunRunner] worker cancel rejected: run_id=%d status=%d body=%s", run.ID, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
}

func newAgentRunQueueConsumerID() string {
	host, _ := os.Hostname()
	host = artifactNameUnsafePattern.ReplaceAllString(strings.TrimSpace(host), "-")
	if host == "" {
		host = "sub2api"
	}
	return fmt.Sprintf("%s-%s", host, uuid.NewString())
}

func (s *AgentRunService) prepareRunKeyBindings(ctx context.Context, input CreateAgentRunInput, version *AgentAppVersion) ([]AgentRunKeyBinding, error) {
	if s.apiKeyService == nil {
		return nil, infraerrors.InternalServer("AGENT_RUN_API_KEY_SERVICE_UNAVAILABLE", "api key service is unavailable")
	}
	defaultKey, err := s.loadUsableRunAPIKey(ctx, input.UserID, input.APIKeyID, "AGENT_RUN")
	if err != nil {
		return nil, err
	}

	policies := modelPolicyMap(nil)
	if version != nil {
		policies = modelPolicyMap(version.NodeModelPolicyJSON)
	}
	explicit, err := normalizeRunKeyBindingInputs(input.APIKeyBindings)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		if len(explicit) > 0 {
			return nil, infraerrors.BadRequest("AGENT_RUN_KEY_BINDING_POLICY_UNKNOWN", "api key binding policy is not defined by this app version")
		}
		return nil, nil
	}
	for policyKey := range explicit {
		if _, ok := policies[policyKey]; !ok {
			return nil, infraerrors.BadRequest("AGENT_RUN_KEY_BINDING_POLICY_UNKNOWN", "api key binding policy is not defined by this app version")
		}
	}

	keyCache := map[int64]*APIKey{defaultKey.ID: defaultKey}
	policyKeys := make([]string, 0, len(policies))
	for policyKey := range policies {
		policyKeys = append(policyKeys, policyKey)
	}
	sort.Strings(policyKeys)

	bindings := make([]AgentRunKeyBinding, 0, len(policyKeys))
	for _, policyKey := range policyKeys {
		policy := policies[policyKey]
		bindingInput, hasExplicitBinding := explicit[policyKey]
		if policy.Optional && !hasExplicitBinding {
			continue
		}
		apiKeyID := input.APIKeyID
		isDefault := true
		if hasExplicitBinding {
			apiKeyID = bindingInput.APIKeyID
			isDefault = false
		}

		apiKey := keyCache[apiKeyID]
		if apiKey == nil {
			apiKey, err = s.loadUsableRunAPIKey(ctx, input.UserID, apiKeyID, "AGENT_RUN")
			if err != nil {
				return nil, err
			}
			keyCache[apiKeyID] = apiKey
		}
		if err := validateRunAPIKeyMatchesPolicy(apiKey, policy, "AGENT_RUN_API_KEY_GROUP_MISMATCH"); err != nil {
			return nil, err
		}

		nodeID, role := modelPolicyNodeRole(policyKey, policy)
		bindings = append(bindings, AgentRunKeyBinding{
			UserID:       input.UserID,
			APIKeyID:     apiKey.ID,
			PolicyKey:    policyKey,
			NodeID:       nodeID,
			Role:         role,
			ModelGroupID: cloneAgentInt64Ptr(policy.ModelGroupID),
			Capability:   strings.TrimSpace(policy.Capability),
			IsDefault:    isDefault,
		})
	}
	return bindings, nil
}

func (s *AgentRunService) loadUsableRunAPIKey(ctx context.Context, userID, apiKeyID int64, reasonPrefix string) (*APIKey, error) {
	if apiKeyID <= 0 {
		return nil, infraerrors.BadRequest(reasonPrefix+"_API_KEY_REQUIRED", "api key is required")
	}
	apiKey, err := s.apiKeyService.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}
	if apiKey.UserID != userID {
		return nil, infraerrors.NotFound(reasonPrefix+"_API_KEY_NOT_FOUND", "api key not found")
	}
	if !apiKey.IsActive() {
		return nil, infraerrors.Forbidden(reasonPrefix+"_API_KEY_INACTIVE", "api key is not active")
	}
	if err := s.apiKeyService.CheckAPIKeyQuotaAndExpiry(apiKey); err != nil {
		return nil, err
	}
	return apiKey, nil
}

func normalizeRunKeyBindingInputs(inputs []CreateAgentRunKeyBindingInput) (map[string]CreateAgentRunKeyBindingInput, error) {
	out := make(map[string]CreateAgentRunKeyBindingInput, len(inputs))
	for _, input := range inputs {
		policyKey := normalizePolicyKey(input.PolicyKey, input.NodeID, input.Role)
		if policyKey == "" {
			return nil, infraerrors.BadRequest("AGENT_RUN_KEY_BINDING_POLICY_REQUIRED", "api key binding policy is required")
		}
		if input.APIKeyID <= 0 {
			return nil, infraerrors.BadRequest("AGENT_RUN_KEY_BINDING_API_KEY_REQUIRED", "api key binding api_key_id is required")
		}
		input.PolicyKey = policyKey
		input.NodeID = strings.TrimSpace(input.NodeID)
		input.Role = strings.TrimSpace(input.Role)
		out[policyKey] = input
	}
	return out, nil
}

func validateRunAPIKeyMatchesPolicy(apiKey *APIKey, policy ModelPolicy, reason string) error {
	if apiKey == nil {
		return nil
	}
	provider := modelPolicyProvider(policy)
	if provider != "" {
		if apiKey.Group == nil || !strings.EqualFold(strings.TrimSpace(apiKey.Group.Platform), provider) {
			return infraerrors.Forbidden(modelPolicyProviderMismatchReason(reason), "selected api key does not match required model provider")
		}
	}
	if policy.ModelGroupID == nil {
		return nil
	}
	if apiKey.GroupID == nil || *apiKey.GroupID != *policy.ModelGroupID {
		return infraerrors.Forbidden(reason, "selected api key does not match required model group")
	}
	return nil
}

func modelPolicyProviderMismatchReason(reason string) string {
	if strings.Contains(reason, "GROUP_MISMATCH") {
		return strings.Replace(reason, "GROUP_MISMATCH", "PROVIDER_MISMATCH", 1)
	}
	return reason
}

func (s *AgentRunService) loadInputAssetsForRun(ctx context.Context, userID, appID int64, ids []int64) ([]AgentInputAsset, error) {
	ids = uniquePositiveInt64s(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	if s.runRepo == nil {
		return nil, infraerrors.InternalServer("AGENT_INPUT_ASSET_REPO_UNAVAILABLE", "input asset repository is unavailable")
	}
	assets, err := s.runRepo.ListInputAssetsByIDsForUser(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]AgentInputAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	now := time.Now().UTC()
	ordered := make([]AgentInputAsset, 0, len(ids))
	for _, id := range ids {
		asset, ok := byID[id]
		if !ok || asset.DeletedAt != nil {
			return nil, ErrAgentInputAssetNotFound
		}
		if asset.ExpiresAt != nil && asset.ExpiresAt.Before(now) {
			return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_EXPIRED", "input asset has expired")
		}
		if asset.AppID != nil && appID > 0 && *asset.AppID != appID {
			return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_APP_MISMATCH", "input asset does not belong to this app")
		}
		ordered = append(ordered, asset)
	}
	return ordered, nil
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func attachInputAssetsToSummary(summary map[string]any, assets []AgentInputAsset) {
	if summary == nil || len(assets) == 0 {
		return
	}
	ids := make([]int64, 0, len(assets))
	items := make([]map[string]any, 0, len(assets))
	for i := range assets {
		asset := assets[i]
		ids = append(ids, asset.ID)
		items = append(items, inputAssetSummary(&asset))
	}
	summary["input_asset_ids"] = ids
	summary["input_assets"] = items
}

func inputAssetSummary(asset *AgentInputAsset) map[string]any {
	if asset == nil {
		return map[string]any{}
	}
	item := map[string]any{
		"id":               asset.ID,
		"file_id":          asset.ID,
		"asset_type":       asset.AssetType,
		"asset_role":       asset.AssetRole,
		"field_name":       asset.FieldName,
		"name":             asset.Name,
		"mime_type":        asset.MimeType,
		"storage_provider": asset.StorageProvider,
		"bucket":           asset.Bucket,
		"object_key":       asset.ObjectKey,
		"object_url":       asset.ObjectURL,
		"size_bytes":       asset.SizeBytes,
		"sha256":           asset.SHA256,
		"metadata_json":    ensureMap(asset.MetadataJSON),
	}
	if asset.AppID != nil {
		item["app_id"] = *asset.AppID
	}
	if asset.RunID != nil {
		item["run_id"] = *asset.RunID
	}
	if asset.ExpiresAt != nil {
		item["expires_at"] = asset.ExpiresAt.Format(time.RFC3339)
	}
	if asset.CreatedAt.IsZero() {
		return item
	}
	item["created_at"] = asset.CreatedAt.Format(time.RFC3339)
	return item
}

func (s *AgentRunService) HandleCallback(ctx context.Context, req WorkerCallbackRequest) (*AgentRun, error) {
	run, err := s.runRepo.GetRunByID(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if !constantTimeRunTokenEqual(run.RunTokenHash, req.RunToken) {
		return nil, ErrAgentRunTokenInvalid
	}

	now := time.Now().UTC()
	nextStatus := strings.TrimSpace(req.Status)
	if nextStatus == "" {
		nextStatus = callbackEventStatus(req.EventType, run.Status)
	}
	switch nextStatus {
	case AgentRunStatusQueued, AgentRunStatusRunning, AgentRunStatusSucceeded, AgentRunStatusFailed, AgentRunStatusCanceled, AgentRunStatusTimeout:
	default:
		return nil, infraerrors.BadRequest("AGENT_CALLBACK_STATUS_INVALID", "run status is invalid")
	}
	if isAgentRunTerminal(run.Status) && run.Status != nextStatus {
		s.forgetRunDispatchToken(context.Background(), run.ID)
		return run, nil
	}

	req.NodeID = strings.TrimSpace(firstNonEmpty(req.NodeID, agentStringFromAny(req.Metadata["node_id"]), agentStringFromAny(req.Metadata["agent_node_id"])))
	req.Role = strings.TrimSpace(firstNonEmpty(req.Role, agentStringFromAny(req.Metadata["role"]), agentStringFromAny(req.Metadata["node_role"]), agentStringFromAny(req.Metadata["agent_node_role"])))
	output := ensureMap(run.OutputSummaryJSON)
	if req.Output != nil {
		output["output"] = req.Output
	}
	if req.Progress != nil {
		output["progress"] = *req.Progress
	}
	if strings.TrimSpace(req.Message) != "" {
		output["message"] = strings.TrimSpace(req.Message)
	}
	if req.Metadata != nil {
		output["metadata"] = req.Metadata
	}

	run.Status = nextStatus
	run.OutputSummaryJSON = output
	run.StartedAt = coalesceTime(run.StartedAt, &now)
	if isAgentRunTerminal(nextStatus) {
		run.CompletedAt = &now
	}
	if req.Error != nil {
		run.ErrorCode = strings.TrimSpace(req.Error.Code)
		run.ErrorMessage = strings.TrimSpace(req.Error.Message)
		if run.Status == AgentRunStatusRunning || run.Status == AgentRunStatusQueued {
			run.Status = AgentRunStatusFailed
			run.CompletedAt = &now
		}
	}
	if usage, ok := req.Metadata["usage"].(map[string]any); ok {
		run.UsageJSON = usage
	}
	if err := s.runRepo.UpdateFromCallback(ctx, run); err != nil {
		current, getErr := s.runRepo.GetRunByID(ctx, run.ID)
		if getErr == nil && isAgentRunTerminal(current.Status) {
			s.forgetRunDispatchToken(context.Background(), current.ID)
			return current, nil
		}
		return nil, fmt.Errorf("update agent run from callback: %w", err)
	}
	if isAgentRunTerminal(run.Status) {
		s.forgetRunDispatchToken(context.Background(), run.ID)
	}
	callbackMetadata := ensureMap(req.Metadata)
	if req.Output != nil {
		callbackMetadata["has_output"] = true
	}
	if req.Error != nil {
		callbackMetadata["error_code"] = strings.TrimSpace(req.Error.Code)
		callbackMetadata["error_message"] = strings.TrimSpace(req.Error.Message)
	}
	if len(req.Artifacts) > 0 {
		callbackMetadata["artifact_count"] = len(req.Artifacts)
	}
	s.recordRunEvent(ctx, run, callbackRunEventType(req.EventType, run.Status), run.Status, req.NodeID, req.Role, strings.TrimSpace(req.Message), req.Progress, callbackMetadata)

	artifactPolicy := s.artifactPolicyForRun(ctx, run)
	for _, ref := range req.Artifacts {
		if ref.ArtifactID > 0 {
			continue
		}
		if err := enforceAgentArtifactPolicy(artifactPolicy, normalizeAgentArtifactType(ref.Type), ref.Name, ref.MimeType, ref.SizeBytes); err != nil {
			log.Printf("[AgentRunEvent] callback artifact rejected: run_id=%d name=%s err=%v", run.ID, ref.Name, err)
			s.recordRunEvent(ctx, run, AgentRunEventArtifact, run.Status, req.NodeID, req.Role, "artifact rejected by policy", nil, map[string]any{
				"name":       ref.Name,
				"mime_type":  ref.MimeType,
				"size_bytes": ref.SizeBytes,
				"error":      err.Error(),
			})
			continue
		}
		artifact := artifactFromWorkerRef(run, ref, artifactPolicy.expiresAt())
		if artifact == nil {
			continue
		}
		if err := s.runRepo.CreateArtifact(ctx, artifact); err != nil {
			log.Printf("[AgentRunEvent] create callback artifact failed: run_id=%d err=%v", run.ID, err)
			continue
		}
		s.recordRunEvent(ctx, run, AgentRunEventArtifact, run.Status, req.NodeID, req.Role, artifact.Name, nil, map[string]any{
			"artifact_id":      artifact.ID,
			"artifact_type":    artifact.ArtifactType,
			"mime_type":        artifact.MimeType,
			"storage_provider": artifact.StorageProvider,
			"object_key":       artifact.ObjectKey,
			"size_bytes":       artifact.SizeBytes,
		})
	}

	return s.runRepo.GetRunByID(ctx, run.ID)
}

type preparedAgentModelProxy struct {
	run           *AgentRun
	request       ModelProxyRequest
	apiKey        *APIKey
	eventMetadata map[string]any
}

func (s *AgentRunService) prepareModelProxy(ctx context.Context, runID int64, req ModelProxyRequest, runToken string) (*preparedAgentModelProxy, error) {
	if runID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_RUN_ID_INVALID", "run id invalid")
	}
	if req.RunID == 0 {
		req.RunID = runID
	}
	if req.RunID != runID {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_RUN_ID_MISMATCH", "run id mismatch")
	}
	if strings.TrimSpace(runToken) == "" {
		return nil, ErrAgentRunTokenInvalid
	}
	if s.modelProxy == nil {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_UNAVAILABLE", "model proxy is not configured")
	}

	run, err := s.runRepo.GetRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if !constantTimeRunTokenEqual(run.RunTokenHash, runToken) {
		return nil, ErrAgentRunTokenInvalid
	}
	if isAgentRunTerminal(run.Status) {
		s.recordRunEvent(ctx, run, AgentRunEventModelProxy, run.Status, req.NodeID, req.Role, "model proxy rejected: run is terminal", nil, map[string]any{
			"model":    req.Model,
			"endpoint": req.Endpoint,
		})
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_RUN_TERMINAL", "run is already terminal")
	}
	if s.isRunCancelRequested(ctx, run.ID) {
		s.recordRunEvent(ctx, run, AgentRunEventModelProxy, AgentRunStatusCanceled, req.NodeID, req.Role, "model proxy rejected: run canceled", nil, map[string]any{
			"model":    req.Model,
			"endpoint": req.Endpoint,
		})
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_RUN_CANCELED", "run has been canceled")
	}

	req.NodeID = strings.TrimSpace(req.NodeID)
	req.Role = strings.TrimSpace(req.Role)
	req.Model = strings.TrimSpace(req.Model)
	req.Endpoint = normalizeModelProxyEndpoint(req.Endpoint)
	if req.Model == "" {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_MODEL_REQUIRED", "model is required")
	}
	if req.Endpoint == "" {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_ENDPOINT_REQUIRED", "endpoint is required")
	}
	if req.Request == nil {
		req.Request = map[string]any{}
	}
	if _, ok := req.Request["model"]; !ok {
		req.Request["model"] = req.Model
	}
	resolvedPolicy, err := s.resolveModelProxyPolicy(ctx, run, &req)
	if err != nil {
		return nil, err
	}
	apiKeyID, err := s.selectRunAPIKeyIDForPolicy(ctx, run, resolvedPolicy)
	if err != nil {
		return nil, err
	}
	apiKey, err := s.loadUsableRunAPIKey(ctx, run.UserID, apiKeyID, "AGENT_MODEL_PROXY")
	if err != nil {
		return nil, err
	}
	if resolvedPolicy != nil {
		if err := validateRunAPIKeyMatchesPolicy(apiKey, resolvedPolicy.Policy, "AGENT_MODEL_PROXY_API_KEY_GROUP_MISMATCH"); err != nil {
			return nil, err
		}
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	req.Metadata["agent_run_id"] = run.ID
	req.Metadata["agent_app_id"] = run.AppID
	req.Metadata["agent_app_version_id"] = run.AppVersionID
	req.Metadata["agent_node_id"] = req.NodeID
	req.Metadata["agent_node_role"] = req.Role
	if resolvedPolicy != nil && resolvedPolicy.PolicyKey != "" {
		req.Metadata["agent_policy_key"] = resolvedPolicy.PolicyKey
	}
	if req.GroupID != nil {
		req.Metadata["agent_model_group_id"] = *req.GroupID
	}
	req.Metadata["agent_api_key_id"] = apiKey.ID
	eventMetadata := map[string]any{
		"model":      req.Model,
		"endpoint":   req.Endpoint,
		"platform":   req.Platform,
		"api_key_id": apiKey.ID,
	}
	if req.GroupID != nil {
		eventMetadata["model_group_id"] = *req.GroupID
	}
	if resolvedPolicy != nil && resolvedPolicy.PolicyKey != "" {
		eventMetadata["policy_key"] = resolvedPolicy.PolicyKey
	}
	return &preparedAgentModelProxy{
		run:           run,
		request:       req,
		apiKey:        apiKey,
		eventMetadata: eventMetadata,
	}, nil
}

func (s *AgentRunService) HandleModelProxy(ctx context.Context, runID int64, req ModelProxyRequest, runToken string) (*ModelProxyResponse, error) {
	prepared, err := s.prepareModelProxy(ctx, runID, req, runToken)
	if err != nil {
		return nil, err
	}
	if isModelProxyStreamingRequest(prepared.request) {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_STREAM_HANDLER_REQUIRED", "streaming model proxy must use the streaming handler")
	}

	resp, err := s.modelProxy.CallModelProxy(ctx, prepared.request, prepared.apiKey)
	if resp != nil && resp.Usage != nil {
		prepared.eventMetadata["usage"] = resp.Usage
	}
	if err != nil {
		s.recordRunEvent(ctx, prepared.run, AgentRunEventModelProxy, AgentRunStatusFailed, prepared.request.NodeID, prepared.request.Role, err.Error(), nil, prepared.eventMetadata)
		return nil, err
	}
	s.recordRunEvent(ctx, prepared.run, AgentRunEventModelProxy, AgentRunStatusSucceeded, prepared.request.NodeID, prepared.request.Role, "model proxy completed", nil, prepared.eventMetadata)
	return resp, nil
}

func (s *AgentRunService) OpenModelProxyStream(ctx context.Context, runID int64, req ModelProxyRequest, runToken string) (*AgentModelProxyStream, error) {
	prepared, err := s.prepareModelProxy(ctx, runID, req, runToken)
	if err != nil {
		return nil, err
	}
	if !isModelProxyStreamingRequest(prepared.request) {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_STREAM_REQUIRED", "streaming model proxy requires stream=true")
	}
	streamGateway, ok := s.modelProxy.(ModelProxyStreamGatewayCaller)
	if !ok {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_STREAM_UNAVAILABLE", "streaming model proxy is not configured")
	}
	startedAt := time.Now()
	resp, err := streamGateway.CallModelProxyStream(ctx, prepared.request, prepared.apiKey)
	if err != nil {
		metadata := cloneModelProxyEventMetadata(prepared.eventMetadata)
		metadata["stream"] = true
		s.recordRunEvent(ctx, prepared.run, AgentRunEventModelProxy, AgentRunStatusFailed, prepared.request.NodeID, prepared.request.Role, err.Error(), nil, metadata)
		return nil, err
	}
	return &AgentModelProxyStream{
		Response:      resp,
		run:           prepared.run,
		request:       prepared.request,
		eventMetadata: prepared.eventMetadata,
		startedAt:     startedAt,
	}, nil
}

func (s *AgentRunService) FinishModelProxyStream(ctx context.Context, stream *AgentModelProxyStream, streamErr error) {
	if stream == nil || stream.run == nil {
		return
	}
	metadata := cloneModelProxyEventMetadata(stream.eventMetadata)
	metadata["stream"] = true
	metadata["duration_ms"] = time.Since(stream.startedAt).Milliseconds()
	if stream.Response != nil {
		metadata["status_code"] = stream.Response.Status
	}
	if streamErr != nil {
		metadata["error"] = streamErr.Error()
		s.recordRunEvent(ctx, stream.run, AgentRunEventModelProxy, AgentRunStatusFailed, stream.request.NodeID, stream.request.Role, "model proxy stream interrupted", nil, metadata)
		return
	}
	s.recordRunEvent(ctx, stream.run, AgentRunEventModelProxy, AgentRunStatusSucceeded, stream.request.NodeID, stream.request.Role, "model proxy stream completed", nil, metadata)
}

func isModelProxyStreamingRequest(req ModelProxyRequest) bool {
	stream, _ := req.Request["stream"].(bool)
	return stream
}

func cloneModelProxyEventMetadata(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+3)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (s *AgentRunService) RegisterArtifact(ctx context.Context, runID int64, req ArtifactCreateRequest, runToken string) (*ArtifactCreateResponse, error) {
	if strings.TrimSpace(runToken) == "" {
		runToken = strings.TrimSpace(req.RunToken)
	}
	run, err := s.validateArtifactRun(ctx, runID, req.RunID, runToken)
	if err != nil {
		return nil, err
	}
	artifactPolicy := s.artifactPolicyForRun(ctx, run)

	artifactType := normalizeAgentArtifactType(req.Type)
	name := normalizeArtifactName(req.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_NAME_REQUIRED", "artifact name is required")
	}
	objectURL := strings.TrimSpace(req.ObjectURL)
	objectKey := sanitizeArtifactObjectKey(req.ObjectKey)
	if objectURL == "" {
		objectURL = objectKey
	}
	if objectURL == "" {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_URL_REQUIRED", "artifact object url is required")
	}
	if objectKey == "" {
		objectKey = objectURL
	}
	sizeBytes := req.SizeBytes
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	if err := enforceAgentArtifactPolicy(artifactPolicy, artifactType, name, req.MimeType, sizeBytes); err != nil {
		return nil, err
	}
	storageProvider := strings.TrimSpace(req.StorageProvider)
	if storageProvider == "" {
		storageProvider = "external"
	}
	artifact := &AgentArtifact{
		RunID:           run.ID,
		UserID:          run.UserID,
		ArtifactType:    artifactType,
		Name:            name,
		MimeType:        strings.TrimSpace(req.MimeType),
		StorageProvider: storageProvider,
		ObjectKey:       objectKey,
		ObjectURL:       objectURL,
		SizeBytes:       sizeBytes,
		SHA256:          strings.TrimSpace(req.SHA256),
		MetadataJSON:    ensureMap(req.Metadata),
		ExpiresAt:       artifactPolicy.expiresAt(),
	}
	if err := s.runRepo.CreateArtifact(ctx, artifact); err != nil {
		return nil, fmt.Errorf("create agent artifact: %w", err)
	}
	s.recordRunEvent(ctx, run, AgentRunEventArtifact, run.Status, "", "", artifact.Name, nil, map[string]any{
		"artifact_id":      artifact.ID,
		"artifact_type":    artifact.ArtifactType,
		"mime_type":        artifact.MimeType,
		"storage_provider": artifact.StorageProvider,
		"object_key":       artifact.ObjectKey,
		"size_bytes":       artifact.SizeBytes,
	})
	return artifactCreateResponse(artifact), nil
}

func (s *AgentRunService) UploadArtifact(ctx context.Context, input ArtifactUploadInput) (*ArtifactCreateResponse, error) {
	run, err := s.validateArtifactRun(ctx, input.RunID, input.RunID, input.RunToken)
	if err != nil {
		return nil, err
	}
	if s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	if input.Body == nil {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_FILE_REQUIRED", "artifact file is required")
	}
	artifactPolicy := s.artifactPolicyForRun(ctx, run)
	maxBytes := artifactPolicy.MaxBytes
	if input.SizeBytes > maxBytes && maxBytes > 0 {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_FILE_TOO_LARGE", "artifact file is too large")
	}

	tmp, err := os.CreateTemp("", "sub2api-agent-artifact-*")
	if err != nil {
		return nil, fmt.Errorf("create artifact temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	hasher := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(limitedArtifactReader(input.Body, maxBytes), hasher))
	closeErr := tmp.Close()
	if err != nil {
		return nil, fmt.Errorf("read artifact file: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close artifact temp file: %w", closeErr)
	}
	if maxBytes > 0 && written > maxBytes {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_FILE_TOO_LARGE", "artifact file is too large")
	}
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if expected := strings.TrimSpace(input.SHA256); expected != "" && !strings.EqualFold(expected, actualSHA256) {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_SHA256_MISMATCH", "artifact sha256 does not match")
	}

	reader, err := os.Open(tmpName)
	if err != nil {
		return nil, fmt.Errorf("open artifact temp file: %w", err)
	}
	defer func() { _ = reader.Close() }()

	artifactType := normalizeAgentArtifactType(input.Type)
	name := normalizeArtifactName(input.Name)
	if name == "" {
		name = "artifact"
	}
	contentType := strings.TrimSpace(input.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := enforceAgentArtifactPolicy(artifactPolicy, artifactType, name, contentType, written); err != nil {
		return nil, err
	}
	objectKey := buildArtifactObjectKey(run, artifactType, name)
	putResult, err := s.artifactStore.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        reader,
		ContentType: contentType,
		SizeBytes:   written,
		Metadata: map[string]string{
			"agent-run-id":  fmt.Sprintf("%d", run.ID),
			"agent-user-id": fmt.Sprintf("%d", run.UserID),
			"artifact-type": artifactType,
		},
	})
	if err != nil {
		return nil, err
	}

	artifact := &AgentArtifact{
		RunID:           run.ID,
		UserID:          run.UserID,
		ArtifactType:    artifactType,
		Name:            name,
		MimeType:        contentType,
		StorageProvider: putResult.Provider,
		Bucket:          putResult.Bucket,
		ObjectKey:       putResult.ObjectKey,
		ObjectURL:       putResult.ObjectURL,
		SizeBytes:       written,
		SHA256:          actualSHA256,
		MetadataJSON:    ensureMap(input.Metadata),
		ExpiresAt:       artifactPolicy.expiresAt(),
	}
	if artifact.ObjectURL == "" {
		artifact.ObjectURL = fmt.Sprintf("%s://%s/%s", artifact.StorageProvider, artifact.Bucket, artifact.ObjectKey)
	}
	if err := s.runRepo.CreateArtifact(ctx, artifact); err != nil {
		return nil, fmt.Errorf("create agent artifact: %w", err)
	}
	s.recordRunEvent(ctx, run, AgentRunEventArtifact, run.Status, "", "", artifact.Name, nil, map[string]any{
		"artifact_id":      artifact.ID,
		"artifact_type":    artifact.ArtifactType,
		"mime_type":        artifact.MimeType,
		"storage_provider": artifact.StorageProvider,
		"bucket":           artifact.Bucket,
		"object_key":       artifact.ObjectKey,
		"size_bytes":       artifact.SizeBytes,
	})
	return artifactCreateResponse(artifact), nil
}

func (s *AgentRunService) UploadInputAsset(ctx context.Context, input InputAssetUploadInput) (*AgentInputAsset, error) {
	if input.UserID <= 0 {
		return nil, infraerrors.Unauthorized("AGENT_INPUT_ASSET_USER_REQUIRED", "user is required")
	}
	if s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	if input.Body == nil {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_FILE_REQUIRED", "input asset file is required")
	}
	maxBytes := s.artifactMaxUploadBytes()
	if input.SizeBytes > maxBytes && maxBytes > 0 {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_FILE_TOO_LARGE", "input asset file is too large")
	}

	tmp, err := os.CreateTemp("", "sub2api-agent-input-*")
	if err != nil {
		return nil, fmt.Errorf("create input asset temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	hasher := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(limitedArtifactReader(input.Body, maxBytes), hasher))
	closeErr := tmp.Close()
	if err != nil {
		return nil, fmt.Errorf("read input asset file: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close input asset temp file: %w", closeErr)
	}
	if maxBytes > 0 && written > maxBytes {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_FILE_TOO_LARGE", "input asset file is too large")
	}
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if expected := strings.TrimSpace(input.SHA256); expected != "" && !strings.EqualFold(expected, actualSHA256) {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_SHA256_MISMATCH", "input asset sha256 does not match")
	}

	reader, err := os.Open(tmpName)
	if err != nil {
		return nil, fmt.Errorf("open input asset temp file: %w", err)
	}
	defer func() { _ = reader.Close() }()

	contentType := strings.TrimSpace(input.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	assetType := normalizeAgentInputAssetType(input.AssetType, contentType)
	name := normalizeArtifactName(input.Name)
	if name == "" {
		name = "input"
	}
	objectKey := buildInputAssetObjectKey(input.UserID, input.AppID, assetType, name)
	metadata := ensureMap(input.Metadata)
	metadata["asset_type"] = assetType
	if fieldName := strings.TrimSpace(input.FieldName); fieldName != "" {
		metadata["field_name"] = fieldName
	}
	if assetRole := strings.TrimSpace(input.AssetRole); assetRole != "" {
		metadata["asset_role"] = assetRole
	}
	putResult, err := s.artifactStore.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        reader,
		ContentType: contentType,
		SizeBytes:   written,
		Metadata: map[string]string{
			"agent-user-id": fmt.Sprintf("%d", input.UserID),
			"asset-kind":    "agent-input-asset",
			"asset-type":    assetType,
		},
	})
	if err != nil {
		return nil, err
	}

	asset := &AgentInputAsset{
		UserID:          input.UserID,
		AppID:           cloneAgentInt64Ptr(input.AppID),
		FieldName:       strings.TrimSpace(input.FieldName),
		AssetType:       assetType,
		AssetRole:       strings.TrimSpace(input.AssetRole),
		Name:            name,
		MimeType:        contentType,
		StorageProvider: putResult.Provider,
		Bucket:          putResult.Bucket,
		ObjectKey:       putResult.ObjectKey,
		ObjectURL:       putResult.ObjectURL,
		SizeBytes:       written,
		SHA256:          actualSHA256,
		MetadataJSON:    metadata,
		ExpiresAt:       s.inputAssetExpiresAt(),
	}
	if asset.ObjectURL == "" {
		asset.ObjectURL = fmt.Sprintf("%s://%s/%s", asset.StorageProvider, asset.Bucket, asset.ObjectKey)
	}
	if err := s.runRepo.CreateInputAsset(ctx, asset); err != nil {
		return nil, fmt.Errorf("create agent input asset: %w", err)
	}
	return asset, nil
}

func (s *AgentRunService) ListInputAssets(ctx context.Context, userID int64, params pagination.PaginationParams, filters AgentInputAssetListFilters) ([]AgentInputAsset, *pagination.PaginationResult, error) {
	if userID <= 0 {
		return nil, nil, infraerrors.Unauthorized("AGENT_INPUT_ASSET_USER_REQUIRED", "user is required")
	}
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}
	return s.runRepo.ListInputAssetsByUser(ctx, userID, params, filters)
}

func (s *AgentRunService) GetInputAssetDownloadURL(ctx context.Context, assetID, userID int64) (*InputAssetDownloadURL, error) {
	if assetID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_ID_INVALID", "input asset id invalid")
	}
	asset, err := s.runRepo.GetInputAssetByID(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.UserID != userID || asset.DeletedAt != nil {
		return nil, ErrAgentInputAssetNotFound
	}
	if asset.ExpiresAt != nil && asset.ExpiresAt.Before(time.Now().UTC()) {
		return nil, infraerrors.BadRequest("AGENT_INPUT_ASSET_EXPIRED", "input asset has expired")
	}
	ttl := s.artifactDownloadTTL()
	expiresAt := time.Now().UTC().Add(ttl)
	if isExternalInputAssetURL(asset) {
		return &InputAssetDownloadURL{InputAssetID: asset.ID, URL: asset.ObjectURL, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
	}
	if s.artifactStore != nil && s.artifactStore.IsConfigured() && strings.TrimSpace(asset.ObjectKey) != "" {
		url, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{StorageProvider: asset.StorageProvider, Bucket: asset.Bucket, ObjectKey: asset.ObjectKey}, ttl)
		if err == nil && strings.TrimSpace(url) != "" {
			return &InputAssetDownloadURL{InputAssetID: asset.ID, URL: url, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
		}
		if !isArtifactStorageNotConfigured(err) {
			return nil, err
		}
	}
	return nil, ErrAgentArtifactStorageNotConfigured
}

func (s *AgentRunService) ListArtifactsForUserRun(ctx context.Context, runID, userID int64) ([]AgentArtifact, error) {
	run, err := s.runRepo.GetRunByIDForUser(ctx, runID, userID)
	if err != nil {
		return nil, err
	}
	return s.runRepo.ListArtifactsByRun(ctx, run.ID)
}

func (s *AgentRunService) GetArtifactDownloadURL(ctx context.Context, artifactID, userID int64) (*ArtifactDownloadURL, error) {
	if artifactID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_ID_INVALID", "artifact id invalid")
	}
	artifact, err := s.runRepo.GetArtifactByID(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.UserID != userID {
		return nil, ErrAgentArtifactNotFound
	}
	if artifact.DeletedAt != nil {
		return nil, ErrAgentArtifactNotFound
	}
	if artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(time.Now().UTC()) {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_EXPIRED", "artifact has expired")
	}
	ttl := s.artifactDownloadTTL()
	expiresAt := time.Now().UTC().Add(ttl)
	if isExternalArtifactURL(artifact) {
		return &ArtifactDownloadURL{ArtifactID: artifact.ID, URL: artifact.ObjectURL, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
	}
	if s.artifactStore != nil && s.artifactStore.IsConfigured() && strings.TrimSpace(artifact.ObjectKey) != "" {
		url, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{StorageProvider: artifact.StorageProvider, Bucket: artifact.Bucket, ObjectKey: artifact.ObjectKey}, ttl)
		if err == nil && strings.TrimSpace(url) != "" {
			return &ArtifactDownloadURL{ArtifactID: artifact.ID, URL: url, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
		}
		if !isArtifactStorageNotConfigured(err) {
			return nil, err
		}
	}
	return nil, ErrAgentArtifactStorageNotConfigured
}

func (s *AgentRunService) validateArtifactRun(ctx context.Context, runID, bodyRunID int64, runToken string) (*AgentRun, error) {
	if runID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_RUN_ID_INVALID", "run id invalid")
	}
	if bodyRunID != 0 && bodyRunID != runID {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_RUN_ID_MISMATCH", "run id mismatch")
	}
	if strings.TrimSpace(runToken) == "" {
		return nil, ErrAgentRunTokenInvalid
	}
	run, err := s.runRepo.GetRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if !constantTimeRunTokenEqual(run.RunTokenHash, runToken) {
		return nil, ErrAgentRunTokenInvalid
	}
	if isAgentRunTerminal(run.Status) {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_RUN_TERMINAL", "run is already terminal")
	}
	return run, nil
}

type resolvedAgentModelPolicy struct {
	PolicyKey string
	Policy    ModelPolicy
}

func (s *AgentRunService) validateModelProxyPolicy(ctx context.Context, run *AgentRun, req *ModelProxyRequest) error {
	_, err := s.resolveModelProxyPolicy(ctx, run, req)
	return err
}

func (s *AgentRunService) resolveModelProxyPolicy(ctx context.Context, run *AgentRun, req *ModelProxyRequest) (*resolvedAgentModelPolicy, error) {
	if s == nil || run == nil || req == nil {
		return nil, nil
	}
	nodeID := strings.TrimSpace(req.NodeID)
	role := strings.TrimSpace(req.Role)
	version, err := s.appRepo.GetVersionByID(ctx, run.AppVersionID)
	if err != nil || version == nil {
		return nil, err
	}
	policies := modelPolicyMap(version.NodeModelPolicyJSON)
	if len(policies) == 0 {
		return nil, nil
	}
	if nodeID == "" && role == "" {
		return nil, infraerrors.Forbidden("AGENT_MODEL_PROXY_NODE_REQUIRED", "model proxy node is required")
	}
	policyKey, policy, ok := lookupModelPolicyWithKey(policies, nodeID, role)
	if !ok {
		return nil, infraerrors.Forbidden("AGENT_MODEL_PROXY_NODE_NOT_ALLOWED", "model proxy node is not allowed")
	}
	if strings.TrimSpace(policy.Model) != "" && strings.TrimSpace(policy.Model) != req.Model {
		return nil, infraerrors.Forbidden("AGENT_MODEL_PROXY_MODEL_NOT_ALLOWED", "model is not allowed for this node")
	}
	if provider := modelPolicyProvider(policy); provider != "" {
		requestPlatform := normalizeAgentModelProvider(req.Platform)
		if requestPlatform != "" && requestPlatform != provider {
			return nil, infraerrors.Forbidden("AGENT_MODEL_PROXY_PROVIDER_NOT_ALLOWED", "model provider is not allowed for this node")
		}
		req.Platform = provider
	}
	if policy.ModelGroupID != nil {
		if req.GroupID != nil && *policy.ModelGroupID != *req.GroupID {
			return nil, infraerrors.Forbidden("AGENT_MODEL_PROXY_GROUP_NOT_ALLOWED", "model group is not allowed for this node")
		}
		if req.GroupID == nil {
			groupID := *policy.ModelGroupID
			req.GroupID = &groupID
		}
	}
	return &resolvedAgentModelPolicy{PolicyKey: policyKey, Policy: policy}, nil
}

func (s *AgentRunService) selectRunAPIKeyIDForPolicy(ctx context.Context, run *AgentRun, resolved *resolvedAgentModelPolicy) (int64, error) {
	if run == nil {
		return 0, ErrAgentRunNotFound
	}
	if resolved == nil || strings.TrimSpace(resolved.PolicyKey) == "" {
		return run.APIKeyID, nil
	}
	bindings, err := s.runRepo.ListRunKeyBindings(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	if len(bindings) == 0 {
		return run.APIKeyID, nil
	}
	policyKey := strings.TrimSpace(resolved.PolicyKey)
	for _, binding := range bindings {
		if strings.TrimSpace(binding.PolicyKey) == policyKey {
			return binding.APIKeyID, nil
		}
	}
	if resolved.Policy.ModelGroupID != nil {
		for _, binding := range bindings {
			if binding.ModelGroupID != nil && *binding.ModelGroupID == *resolved.Policy.ModelGroupID {
				return binding.APIKeyID, nil
			}
		}
	}
	return 0, infraerrors.Forbidden("AGENT_MODEL_PROXY_API_KEY_BINDING_MISSING", "api key binding is missing for this model policy")
}

func (s *AgentRunService) dispatchRun(runID int64, runToken, callbackBaseURL string) bool {
	return s.dispatchRunWithLeasePolicy(runID, runToken, callbackBaseURL, false)
}

func (s *AgentRunService) dispatchRunFallback(runID int64, runToken, callbackBaseURL string) bool {
	return s.dispatchRunWithLeasePolicy(runID, runToken, callbackBaseURL, true)
}

func (s *AgentRunService) dispatchRunWithLeasePolicy(runID int64, runToken, callbackBaseURL string, allowLeaseFailure bool) (acknowledge bool) {
	ctx := context.Background()
	keepDispatchToken := false
	defer func() {
		if acknowledge && !keepDispatchToken {
			s.forgetRunDispatchToken(context.Background(), runID)
		}
	}()
	run, err := s.runRepo.GetRunByID(ctx, runID)
	if err != nil {
		return false
	}
	if isAgentRunTerminal(run.Status) {
		return true
	}
	app, err := s.appRepo.GetAppByID(ctx, run.AppID)
	if err != nil {
		return s.markRunFailed(ctx, run, "APP_UNAVAILABLE", err.Error()) == nil
	}
	version, err := s.appRepo.GetVersionByID(ctx, run.AppVersionID)
	if err != nil || version.AppID != run.AppID {
		if err == nil {
			err = ErrAgentAppVersionNotFound
		}
		return s.markRunFailed(ctx, run, "APP_VERSION_UNAVAILABLE", err.Error()) == nil
	}
	if version.WorkerHostID == nil {
		return s.markRunFailed(ctx, run, "WORKER_HOST_MISSING", "worker host missing") == nil
	}
	host, err := s.workerHostRepo.GetByID(ctx, *version.WorkerHostID)
	if err != nil {
		return s.markRunFailed(ctx, run, "WORKER_HOST_UNAVAILABLE", err.Error()) == nil
	}
	if host.Status != AgentWorkerHostStatusActive {
		return s.markRunFailed(ctx, run, "WORKER_HOST_INACTIVE", "worker host is not active") == nil
	}
	if _, err := supportedAgentWorkerAuthType(host.AuthType); err != nil {
		return s.markRunFailed(ctx, run, "WORKER_AUTH_TYPE_UNSUPPORTED", err.Error()) == nil
	}

	timeout := agentWorkerDispatchTimeout(host)
	leaseToken, leaseAcquired, leaseErr := s.acquireAgentRunDispatchLease(ctx, run.ID, agentRunDispatchLeaseTTL(timeout))
	if leaseErr != nil {
		if !allowLeaseFailure {
			log.Printf("[AgentRunRunner] acquire dispatch lease failed: run_id=%d err=%v", run.ID, leaseErr)
			return false
		}
		log.Printf("[AgentRunRunner] dispatch lease unavailable during in-process fallback: run_id=%d err=%v", run.ID, leaseErr)
	} else if !leaseAcquired {
		log.Printf("[AgentRunRunner] dispatch already leased by another runner: run_id=%d", run.ID)
		return false
	}
	if leaseToken != "" {
		defer s.releaseAgentRunDispatchLease(run.ID, leaseToken)
	}

	s.recordRunEvent(ctx, run, AgentRunEventDispatching, run.Status, "", "", "dispatching run to worker", nil, nil)
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	release, err := s.acquireHostSlot(runCtx, host)
	if err != nil {
		return s.markRunTimeout(ctx, run, "worker slot timeout") == nil
	}
	defer release()

	startedAt := time.Now().UTC()
	if err := s.runRepo.MarkRunning(ctx, run.ID, startedAt); err != nil {
		return false
	}
	run.Status = AgentRunStatusRunning
	run.StartedAt = &startedAt
	s.recordRunEvent(ctx, run, AgentRunEventRunning, AgentRunStatusRunning, "", "", "worker dispatch started", nil, map[string]any{
		"worker_host_id": host.ID,
		"worker_route":   version.WorkerRoute,
	})

	workerURL, err := joinWorkerURL(host.BaseURL, version.WorkerRoute)
	if err != nil {
		return s.markRunFailed(ctx, run, "WORKER_URL_INVALID", err.Error()) == nil
	}

	workerPolicies := modelPolicyMap(version.NodeModelPolicyJSON)
	if bindings, bindingErr := s.runRepo.ListRunKeyBindings(runCtx, run.ID); bindingErr == nil {
		bound := make(map[string]bool, len(bindings))
		for _, binding := range bindings {
			bound[binding.PolicyKey] = true
		}
		for key, policy := range workerPolicies {
			if policy.Optional && !bound[key] {
				delete(workerPolicies, key)
			}
		}
	}
	payload := WorkerRunRequest{
		RunID:           run.ID,
		AppID:           app.ID,
		AppVersionID:    version.ID,
		RunToken:        runToken,
		CallbackURL:     callbackBaseURL + fmt.Sprintf("/api/v1/agent-runs/%d/callback", run.ID),
		ModelProxyURL:   callbackBaseURL + fmt.Sprintf("/api/v1/agent-runs/%d/model-proxy", run.ID),
		ArtifactURL:     callbackBaseURL + fmt.Sprintf("/api/v1/agent-runs/%d/artifacts", run.ID),
		TimeoutSeconds:  host.TimeoutSeconds,
		User:            WorkerRunUserContext{UserID: run.UserID, APIKeyID: run.APIKeyID},
		Input:           ensureMap(run.InputSummaryJSON),
		InputArtifacts:  s.workerInputArtifactRefs(runCtx, run, host.TimeoutSeconds),
		NodeModelPolicy: workerPolicies,
		Metadata: map[string]any{
			"app_slug":        app.Slug,
			"app_version":     version.Version,
			"default_model":   version.DefaultModelConfigJSON,
			"artifact_policy": version.ArtifactPolicyJSON,
			"capabilities":    version.CapabilitiesJSON,
			"worker_protocol": host.Protocol,
			"sub2api_runtime": "agent-runner-go",
			"worker_route":    version.WorkerRoute,
			"worker_host_id":  host.ID,
		},
	}
	payload.InputAssets = payload.InputArtifacts

	body, err := json.Marshal(payload)
	if err != nil {
		return s.markRunFailed(ctx, run, "WORKER_PAYLOAD_INVALID", err.Error()) == nil
	}
	httpReq, err := http.NewRequestWithContext(runCtx, http.MethodPost, workerURL, bytes.NewReader(body))
	if err != nil {
		return s.markRunFailed(ctx, run, "WORKER_REQUEST_INVALID", err.Error()) == nil
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Sub2API-Worker-Protocol", host.Protocol)
	httpReq.Header.Set("X-Sub2API-Run-ID", fmt.Sprintf("%d", run.ID))
	httpReq.Header.Set("X-Sub2API-Run-Token", runToken)
	httpReq.Header.Set("X-Sub2API-Timestamp", timestamp)
	httpReq.Header.Set("X-Sub2API-Signature", signWorkerPayload(runToken, timestamp, body))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		if runCtx.Err() != nil {
			return s.markRunTimeout(ctx, run, "worker request timeout") == nil
		}
		return s.markRunFailed(ctx, run, "WORKER_REQUEST_FAILED", err.Error()) == nil
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.markRunFailed(ctx, run, "WORKER_REJECTED", fmt.Sprintf("worker HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))) == nil
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		return s.markRunFailed(ctx, run, "WORKER_RESPONSE_INVALID", "worker returned an empty response") == nil
	}
	var workerResp WorkerRunResponse
	if err := json.Unmarshal(raw, &workerResp); err != nil {
		return s.markRunFailed(ctx, run, "WORKER_RESPONSE_INVALID", fmt.Sprintf("worker returned invalid JSON: %v", err)) == nil
	}
	workerStatus := strings.ToLower(strings.TrimSpace(workerResp.Status))
	if workerStatus == AgentRunStatusSucceeded {
		now := time.Now().UTC()
		run.Status = AgentRunStatusSucceeded
		run.StartedAt = &startedAt
		run.CompletedAt = &now
		run.OutputSummaryJSON = ensureMap(workerResp.Metadata)
		if err := s.runRepo.UpdateFromCallback(ctx, run); err != nil {
			return false
		}
		s.recordRunEvent(ctx, run, AgentRunEventSucceeded, AgentRunStatusSucceeded, "", "", "worker completed synchronously", nil, workerResp.Metadata)
		return true
	}
	if workerStatus != AgentRunStatusRunning {
		return s.markRunFailed(ctx, run, "WORKER_RESPONSE_INVALID", fmt.Sprintf("worker returned invalid status %q", strings.TrimSpace(workerResp.Status))) == nil
	}
	if !workerResp.Accepted {
		message := strings.TrimSpace(workerResp.Message)
		if message == "" {
			message = "worker rejected run"
		}
		return s.markRunFailed(ctx, run, "WORKER_REJECTED", message) == nil
	}
	s.recordRunEvent(ctx, run, AgentRunEventWorkerAccepted, AgentRunStatusRunning, "", "", strings.TrimSpace(workerResp.Message), nil, map[string]any{
		"accepted":                 workerResp.Accepted,
		"worker_run_id":            workerResp.WorkerRunID,
		"worker_status":            workerResp.Status,
		"poll_url":                 workerResp.PollURL,
		"estimated_time_seconds":   workerResp.EstimatedTime,
		"worker_response_metadata": workerResp.Metadata,
	})
	keepDispatchToken = true
	return true
}

func agentWorkerDispatchTimeout(host *AgentWorkerHost) time.Duration {
	if host == nil || host.TimeoutSeconds <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(host.TimeoutSeconds)*time.Second + defaultAgentWorkerHTTPGrace
}

func agentRunDispatchLeaseTTL(dispatchTimeout time.Duration) time.Duration {
	if dispatchTimeout <= 0 {
		dispatchTimeout = 10 * time.Minute
	}
	return dispatchTimeout + agentRunLeaseSafetyWindow
}

func (s *AgentRunService) acquireAgentRunDispatchLease(ctx context.Context, runID int64, ttl time.Duration) (string, bool, error) {
	if s == nil || s.redisClient == nil {
		return "", true, nil
	}
	if runID <= 0 {
		return "", false, errors.New("agent run dispatch lease run id is invalid")
	}
	if ttl <= 0 {
		ttl = agentRunDispatchLeaseTTL(0)
	}
	token := uuid.NewString()
	acquired, err := s.redisClient.SetNX(ctx, agentRunDispatchLeaseKey(runID), token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	if !acquired {
		return "", false, nil
	}
	return token, true, nil
}

func (s *AgentRunService) releaseAgentRunDispatchLease(runID int64, token string) {
	if s == nil || s.redisClient == nil || runID <= 0 || strings.TrimSpace(token) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentRunLeaseReleaseTimeout)
	defer cancel()
	if err := agentRunDispatchLeaseReleaseScript.Run(ctx, s.redisClient, []string{agentRunDispatchLeaseKey(runID)}, token).Err(); err != nil {
		log.Printf("[AgentRunRunner] release dispatch lease failed: run_id=%d err=%v", runID, err)
	}
}

func agentRunDispatchLeaseKey(runID int64) string {
	return fmt.Sprintf("agent:run:%d:dispatch-lease", runID)
}

func (s *AgentRunService) markRunFailed(ctx context.Context, run *AgentRun, code, message string) error {
	if run == nil {
		return ErrAgentRunNotFound
	}
	now := time.Now().UTC()
	err := s.runRepo.MarkFailed(ctx, run.ID, code, message, now)
	if err == nil {
		run.Status = AgentRunStatusFailed
		run.ErrorCode = strings.TrimSpace(code)
		run.ErrorMessage = strings.TrimSpace(message)
		run.CompletedAt = &now
		s.recordRunEvent(ctx, run, AgentRunEventFailed, AgentRunStatusFailed, "", "", message, nil, map[string]any{
			"error_code": strings.TrimSpace(code),
		})
	}
	return err
}

func (s *AgentRunService) markRunTimeout(ctx context.Context, run *AgentRun, message string) error {
	if run == nil {
		return ErrAgentRunNotFound
	}
	now := time.Now().UTC()
	err := s.runRepo.MarkTimeout(ctx, run.ID, now)
	if err == nil {
		run.Status = AgentRunStatusTimeout
		run.ErrorCode = "WORKER_TIMEOUT"
		run.ErrorMessage = strings.TrimSpace(message)
		run.CompletedAt = &now
		s.recordRunEvent(ctx, run, AgentRunEventTimeout, AgentRunStatusTimeout, "", "", message, nil, nil)
	}
	return err
}

func (s *AgentRunService) acquireHostSlot(ctx context.Context, host *AgentWorkerHost) (func(), error) {
	if host == nil || host.ID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_WORKER_HOST_INVALID", "Worker Host 无效")
	}
	limit := host.MaxConcurrency
	if limit <= 0 {
		limit = 1
	}
	if s.redisClient != nil {
		release, err := s.acquireRedisHostSlot(ctx, host, limit)
		if err == nil {
			return release, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		log.Printf("[AgentRunRunner] redis host slot unavailable, falling back to local slot: host_id=%d err=%v", host.ID, err)
	}
	return s.acquireLocalHostSlot(ctx, host.ID, limit)
}

func (s *AgentRunService) acquireLocalHostSlot(ctx context.Context, hostID int64, limit int) (func(), error) {
	s.mu.Lock()
	sem := s.hostSemaphores[hostID]
	if sem == nil || cap(sem) != limit {
		sem = make(chan struct{}, limit)
		s.hostSemaphores[hostID] = sem
	}
	s.mu.Unlock()

	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *AgentRunService) acquireRedisHostSlot(ctx context.Context, host *AgentWorkerHost, limit int) (func(), error) {
	if s == nil || s.redisClient == nil || host == nil {
		return nil, errors.New("redis host slot is not configured")
	}
	key := agentHostSlotKey(host.ID)
	token := uuid.NewString()
	ttl := agentHostSlotTTL(host)
	ttlMillis := int64(ttl / time.Millisecond)
	if ttlMillis <= 0 {
		ttlMillis = int64(time.Minute / time.Millisecond)
	}
	for {
		nowMillis := time.Now().UTC().UnixMilli()
		expiresAtMillis := nowMillis + ttlMillis
		acquired, err := agentHostSlotAcquireScript.Run(ctx, s.redisClient, []string{key},
			nowMillis,
			limit,
			expiresAtMillis,
			token,
			ttlMillis,
		).Int()
		if err != nil {
			return nil, err
		}
		if acquired == 1 {
			return func() {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = agentHostSlotReleaseScript.Run(releaseCtx, s.redisClient, []string{key}, token).Err()
			}, nil
		}
		timer := time.NewTimer(agentHostSlotPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func agentHostSlotKey(hostID int64) string {
	return fmt.Sprintf("agent:worker-host:%d:slots", hostID)
}

func agentHostSlotTTL(host *AgentWorkerHost) time.Duration {
	timeout := 60 * time.Second
	if host != nil && host.TimeoutSeconds > 0 {
		timeout = time.Duration(host.TimeoutSeconds)*time.Second + defaultAgentWorkerHTTPGrace + 30*time.Second
	}
	if timeout < time.Minute {
		return time.Minute
	}
	return timeout
}

func newAgentRunToken() (plain, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plain = "ar_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}

func constantTimeRunTokenEqual(hash, token string) bool {
	sum := sha256.Sum256([]byte(token))
	actual := []byte(strings.TrimSpace(hash))
	expected := []byte(hex.EncodeToString(sum[:]))
	return hmac.Equal(actual, expected)
}

func signWorkerPayload(runToken, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(runToken))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func callbackEventStatus(eventType, fallback string) string {
	switch strings.TrimSpace(eventType) {
	case "started", "progress":
		return AgentRunStatusRunning
	case "succeeded", "completed":
		return AgentRunStatusSucceeded
	case "failed", "error":
		return AgentRunStatusFailed
	case "canceled":
		return AgentRunStatusCanceled
	case "timeout":
		return AgentRunStatusTimeout
	default:
		if strings.TrimSpace(fallback) == "" {
			return AgentRunStatusRunning
		}
		return fallback
	}
}

func isAgentRunTerminal(status string) bool {
	switch status {
	case AgentRunStatusSucceeded, AgentRunStatusFailed, AgentRunStatusCanceled, AgentRunStatusTimeout:
		return true
	default:
		return false
	}
}

func (s *AgentRunService) recordRunEvent(ctx context.Context, run *AgentRun, eventType, status, nodeID, role, message string, progress *float64, metadata map[string]any) {
	if s == nil || s.runRepo == nil || run == nil || run.ID <= 0 || run.UserID <= 0 {
		return
	}
	event := &AgentRunEvent{
		RunID:        run.ID,
		UserID:       run.UserID,
		EventType:    normalizeAgentRunEventType(eventType, status),
		Status:       strings.TrimSpace(status),
		NodeID:       strings.TrimSpace(nodeID),
		Role:         strings.TrimSpace(role),
		Message:      strings.TrimSpace(message),
		Progress:     normalizeAgentRunProgress(progress),
		MetadataJSON: ensureMap(metadata),
	}
	if event.Status == "" {
		event.Status = run.Status
	}
	if err := s.runRepo.CreateRunEvent(ctx, event); err != nil {
		log.Printf("[AgentRunEvent] create event failed: run_id=%d type=%s err=%v", run.ID, event.EventType, err)
	}
}

func normalizeAgentRunEventType(eventType, status string) string {
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case "started":
		return AgentRunEventRunning
	case "completed":
		return AgentRunEventSucceeded
	case "error":
		return AgentRunEventFailed
	case "":
	default:
		return eventType
	}
	switch strings.TrimSpace(status) {
	case AgentRunStatusQueued:
		return AgentRunEventQueued
	case AgentRunStatusRunning:
		return AgentRunEventProgress
	case AgentRunStatusSucceeded:
		return AgentRunEventSucceeded
	case AgentRunStatusFailed:
		return AgentRunEventFailed
	case AgentRunStatusCanceled:
		return AgentRunEventCanceled
	case AgentRunStatusTimeout:
		return AgentRunEventTimeout
	default:
		return AgentRunEventLog
	}
}

func callbackRunEventType(eventType, status string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType != "" {
		return normalizeAgentRunEventType(eventType, status)
	}
	return normalizeAgentRunEventType("", status)
}

func normalizeAgentRunProgress(progress *float64) *float64 {
	if progress == nil {
		return nil
	}
	value := *progress
	if value > 1 && value <= 100 {
		value = value / 100
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return &value
}

func coalesceTime(value, fallback *time.Time) *time.Time {
	if value != nil {
		return value
	}
	return fallback
}

func modelPolicyMap(raw map[string]any) map[string]ModelPolicy {
	out := make(map[string]ModelPolicy)
	for key, value := range raw {
		b, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var policy ModelPolicy
		if err := json.Unmarshal(b, &policy); err == nil {
			policy.Provider = modelPolicyProvider(policy)
			policy.Platform = policy.Provider
			out[key] = policy
		}
	}
	return out
}

func modelPolicyProvider(policy ModelPolicy) string {
	provider := normalizeAgentModelProvider(firstNonEmpty(policy.Provider, policy.Platform))
	if provider != "" {
		return provider
	}
	return inferAgentModelProvider(policy.Model)
}

func normalizeAgentModelProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "", "auto":
		return ""
	case "openai", "open-ai":
		return "openai"
	case "anthropic", "claude":
		return "anthropic"
	case "gemini", "google", "google-gemini":
		return "gemini"
	case "antigravity", "anti-gravity":
		return "antigravity"
	default:
		return value
	}
}

func inferAgentModelProvider(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return ""
	}
	if strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") || strings.Contains(model, "openai") {
		return "openai"
	}
	if strings.HasPrefix(model, "claude") || strings.Contains(model, "anthropic") {
		return "anthropic"
	}
	if strings.HasPrefix(model, "gemini") || strings.Contains(model, "gemini") {
		return "gemini"
	}
	if strings.Contains(model, "antigravity") {
		return "antigravity"
	}
	return ""
}

func lookupModelPolicy(policies map[string]ModelPolicy, nodeID, role string) (ModelPolicy, bool) {
	_, policy, ok := lookupModelPolicyWithKey(policies, nodeID, role)
	return policy, ok
}

func lookupModelPolicyWithKey(policies map[string]ModelPolicy, nodeID, role string) (string, ModelPolicy, bool) {
	nodeID = strings.TrimSpace(nodeID)
	role = strings.TrimSpace(role)
	if nodeID != "" && role != "" {
		key := nodeID + "." + role
		if policy, ok := policies[key]; ok {
			return key, policy, true
		}
	}
	if nodeID != "" {
		if policy, ok := policies[nodeID]; ok {
			return nodeID, policy, true
		}
	}
	if role != "" {
		if policy, ok := policies[role]; ok {
			return role, policy, true
		}
	}
	return "", ModelPolicy{}, false
}

func normalizePolicyKey(policyKey, nodeID, role string) string {
	policyKey = strings.TrimSpace(policyKey)
	if policyKey != "" {
		return policyKey
	}
	nodeID = strings.TrimSpace(nodeID)
	role = strings.TrimSpace(role)
	if nodeID != "" && role != "" {
		return nodeID + "." + role
	}
	if nodeID != "" {
		return nodeID
	}
	return role
}

func modelPolicyNodeRole(policyKey string, policy ModelPolicy) (string, string) {
	nodeID := strings.TrimSpace(policy.NodeID)
	role := strings.TrimSpace(policy.Role)
	if nodeID != "" || role != "" {
		return nodeID, role
	}
	parts := strings.SplitN(strings.TrimSpace(policyKey), ".", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(policyKey), ""
}

func cloneAgentInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

var artifactNameUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func normalizeAgentArtifactType(value string) string {
	switch strings.TrimSpace(value) {
	case AgentArtifactTypeInput:
		return AgentArtifactTypeInput
	case AgentArtifactTypeLog:
		return AgentArtifactTypeLog
	case AgentArtifactTypePreview:
		return AgentArtifactTypePreview
	default:
		return AgentArtifactTypeOutput
	}
}

func normalizeAgentInputAssetType(value, mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AgentInputAssetTypeImage:
		return AgentInputAssetTypeImage
	case AgentInputAssetTypeAudio:
		return AgentInputAssetTypeAudio
	case AgentInputAssetTypeVideo:
		return AgentInputAssetTypeVideo
	case AgentInputAssetTypeFile:
		return AgentInputAssetTypeFile
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return AgentInputAssetTypeImage
	case strings.HasPrefix(mimeType, "audio/"):
		return AgentInputAssetTypeAudio
	case strings.HasPrefix(mimeType, "video/"):
		return AgentInputAssetTypeVideo
	default:
		return AgentInputAssetTypeFile
	}
}

func normalizeArtifactName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSpace(name)
	if len(name) > 180 {
		ext := pathExt(name)
		base := strings.TrimSuffix(name, ext)
		if len(ext) > 32 {
			ext = ""
		}
		limit := 180 - len(ext)
		if limit < 1 {
			limit = 180
		}
		name = base[:minInt(len(base), limit)] + ext
	}
	return name
}

func buildArtifactObjectKey(run *AgentRun, artifactType, name string) string {
	if run == nil {
		return ""
	}
	safeName := artifactNameUnsafePattern.ReplaceAllString(normalizeArtifactName(name), "_")
	if safeName == "" {
		safeName = "artifact"
	}
	return fmt.Sprintf("%d/%d/%s/%s-%s", run.UserID, run.ID, artifactType, uuid.NewString(), safeName)
}

func buildInputAssetObjectKey(userID int64, appID *int64, assetType, name string) string {
	safeName := artifactNameUnsafePattern.ReplaceAllString(normalizeArtifactName(name), "_")
	if safeName == "" {
		safeName = "input"
	}
	appPart := "shared"
	if appID != nil && *appID > 0 {
		appPart = fmt.Sprintf("app-%d", *appID)
	}
	return fmt.Sprintf("%d/input-assets/%s/%s/%s-%s", userID, appPart, assetType, uuid.NewString(), safeName)
}

func artifactCreateResponse(artifact *AgentArtifact) *ArtifactCreateResponse {
	if artifact == nil {
		return nil
	}
	return &ArtifactCreateResponse{
		ArtifactID:      artifact.ID,
		URL:             artifact.ObjectURL,
		ObjectKey:       artifact.ObjectKey,
		StorageProvider: artifact.StorageProvider,
		Bucket:          artifact.Bucket,
		SizeBytes:       artifact.SizeBytes,
		SHA256:          artifact.SHA256,
		Metadata:        ensureMap(artifact.MetadataJSON),
	}
}

func isExternalArtifactURL(artifact *AgentArtifact) bool {
	if artifact == nil {
		return false
	}
	objectURL := strings.TrimSpace(artifact.ObjectURL)
	if objectURL == "" {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(artifact.StorageProvider))
	return provider == "external" || strings.HasPrefix(objectURL, "http://") || strings.HasPrefix(objectURL, "https://")
}

func isExternalInputAssetURL(asset *AgentInputAsset) bool {
	if asset == nil {
		return false
	}
	objectURL := strings.TrimSpace(asset.ObjectURL)
	if objectURL == "" {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(asset.StorageProvider))
	return provider == "external" || strings.HasPrefix(objectURL, "http://") || strings.HasPrefix(objectURL, "https://")
}

func (s *AgentRunService) artifactMaxUploadBytes() int64 {
	cfg := s.agentArtifactRuntimeConfig()
	if cfg.MaxUploadBytes <= 0 {
		return 512 * 1024 * 1024
	}
	return cfg.MaxUploadBytes
}

func (s *AgentRunService) artifactDownloadTTL() time.Duration {
	cfg := s.agentArtifactRuntimeConfig()
	if cfg.DownloadURLTTLSeconds <= 0 {
		return time.Hour
	}
	return time.Duration(cfg.DownloadURLTTLSeconds) * time.Second
}

func (s *AgentRunService) inputAssetExpiresAt() *time.Time {
	cfg := s.agentArtifactRuntimeConfig()
	if cfg.RetentionDays <= 0 {
		return nil
	}
	expiresAt := time.Now().UTC().Add(time.Duration(cfg.RetentionDays) * 24 * time.Hour)
	return &expiresAt
}

func (s *AgentRunService) artifactExpiresAt() *time.Time {
	cfg := s.agentArtifactRuntimeConfig()
	if cfg.RetentionDays <= 0 {
		return nil
	}
	expiresAt := time.Now().UTC().Add(time.Duration(cfg.RetentionDays) * 24 * time.Hour)
	return &expiresAt
}

type agentArtifactPolicy struct {
	RetentionDays   *int
	MaxBytes        int64
	AllowedTypes    map[string]bool
	HasAllowedTypes bool
}

func (p agentArtifactPolicy) expiresAt() *time.Time {
	if p.RetentionDays == nil || *p.RetentionDays <= 0 {
		return nil
	}
	expiresAt := time.Now().UTC().Add(time.Duration(*p.RetentionDays) * 24 * time.Hour)
	return &expiresAt
}

func (s *AgentRunService) defaultAgentArtifactPolicy() agentArtifactPolicy {
	policy := agentArtifactPolicy{
		MaxBytes:     s.artifactMaxUploadBytes(),
		AllowedTypes: map[string]bool{},
	}
	cfg := s.agentArtifactRuntimeConfig()
	if cfg.RetentionDays > 0 {
		retentionDays := cfg.RetentionDays
		policy.RetentionDays = &retentionDays
	}
	return policy
}

func (s *AgentRunService) artifactPolicyForRun(ctx context.Context, run *AgentRun) agentArtifactPolicy {
	policy := s.defaultAgentArtifactPolicy()
	if s == nil || s.appRepo == nil || run == nil || run.AppVersionID <= 0 {
		return policy
	}
	version, err := s.appRepo.GetVersionByID(ctx, run.AppVersionID)
	if err != nil || version == nil {
		if err != nil {
			log.Printf("[AgentArtifactPolicy] load app version failed: run_id=%d version_id=%d err=%v", run.ID, run.AppVersionID, err)
		}
		return policy
	}
	return mergeAgentArtifactPolicy(policy, version.ArtifactPolicyJSON)
}

func mergeAgentArtifactPolicy(policy agentArtifactPolicy, raw map[string]any) agentArtifactPolicy {
	if raw == nil {
		return policy
	}
	if value, ok := raw["retention_days"]; ok {
		retentionDays := int(int64FromAny(value))
		policy.RetentionDays = &retentionDays
	}
	if value, ok := raw["max_file_mb"]; ok {
		maxFileMB := int64FromAny(value)
		if maxFileMB <= 0 {
			policy.MaxBytes = 0
		} else {
			policy.MaxBytes = maxFileMB * 1024 * 1024
		}
	}
	if value, ok := raw["allowed_types"]; ok {
		policy.AllowedTypes = normalizeAgentArtifactAllowedTypes(value)
		policy.HasAllowedTypes = true
	}
	return policy
}

func normalizeAgentArtifactAllowedTypes(value any) map[string]bool {
	out := make(map[string]bool)
	add := func(item string) {
		if normalized := normalizeAgentArtifactPolicyType(item); normalized != "" {
			out[normalized] = true
		}
	}
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			add(agentStringFromAny(item))
		}
	case []string:
		for _, item := range v {
			add(item)
		}
	case map[string]any:
		for key, enabled := range v {
			if boolFromAny(enabled) {
				add(key)
			}
		}
	case map[string]bool:
		for key, enabled := range v {
			if enabled {
				add(key)
			}
		}
	case string:
		for _, item := range strings.Split(v, ",") {
			add(item)
		}
	}
	return out
}

func normalizeAgentArtifactPolicyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json", "application/json":
		return "json"
	case "image", "images":
		return "image"
	case "video", "videos":
		return "video"
	case "audio", "audios":
		return "audio"
	case "file", "files", "output", "artifact":
		return "file"
	case "log", "logs":
		return "log"
	default:
		return ""
	}
}

func enforceAgentArtifactPolicy(policy agentArtifactPolicy, artifactType, name, mimeType string, sizeBytes int64) error {
	if policy.MaxBytes > 0 && sizeBytes > policy.MaxBytes {
		return infraerrors.BadRequest("AGENT_ARTIFACT_FILE_TOO_LARGE", "artifact file is too large")
	}
	if !policy.HasAllowedTypes {
		return nil
	}
	classifiedType := classifyAgentArtifactPolicyType(artifactType, name, mimeType)
	if policy.AllowedTypes[classifiedType] {
		return nil
	}
	return infraerrors.BadRequest("AGENT_ARTIFACT_TYPE_NOT_ALLOWED", "artifact type is not allowed")
}

func classifyAgentArtifactPolicyType(artifactType, name, mimeType string) string {
	if normalizeAgentArtifactType(artifactType) == AgentArtifactTypeLog {
		return "log"
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case mimeType == "application/json" || strings.HasSuffix(mimeType, "+json"):
		return "json"
	}
	if strings.EqualFold(pathExt(name), ".json") {
		return "json"
	}
	return "file"
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on", "enabled":
			return true
		default:
			return false
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func (s *AgentRunService) cleanupExpiredArtifactsEnabled() bool {
	return s.agentArtifactRuntimeConfig().CleanupExpiredArtifactsEnabled
}

func (s *AgentRunService) agentArtifactRuntimeConfig() config.AgentArtifactStorageConfig {
	if s == nil {
		return config.AgentArtifactStorageConfig{}
	}
	if s.artifactConfig != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if cfg, ok, err := s.artifactConfig.CurrentRuntimeConfig(ctx); err == nil && ok {
			return cfg
		}
	}
	if s.cfg != nil {
		return s.cfg.AgentArtifacts
	}
	return config.AgentArtifactStorageConfig{}
}

func (s *AgentRunService) deleteAgentCleanupObjects(ctx context.Context, refs []AgentCleanupObjectRef) (int64, int64) {
	if s == nil || s.artifactStore == nil || !s.artifactStore.IsConfigured() || len(refs) == 0 {
		return 0, 0
	}
	var deleted int64
	var failed int64
	for _, ref := range refs {
		if !s.shouldDeleteAgentCleanupObject(ref) {
			continue
		}
		if err := s.artifactStore.DeleteObject(ctx, AgentArtifactObjectLocation{StorageProvider: ref.StorageProvider, Bucket: ref.Bucket, ObjectKey: ref.ObjectKey}); err != nil {
			failed++
			log.Printf("[AgentCleanup] delete object failed: id=%d provider=%s bucket=%s object_key=%s err=%v",
				ref.ID,
				ref.StorageProvider,
				ref.Bucket,
				ref.ObjectKey,
				err,
			)
			continue
		}
		deleted++
	}
	return deleted, failed
}

func (s *AgentRunService) shouldDeleteAgentCleanupObject(ref AgentCleanupObjectRef) bool {
	if s == nil || s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return false
	}
	provider := normalizeAgentArtifactProvider(ref.StorageProvider)
	return provider != "" && provider != "external" && strings.TrimSpace(ref.Bucket) != "" && strings.TrimSpace(ref.ObjectKey) != ""
}

func agentCleanupObjectIDs(refs []AgentCleanupObjectRef) []int64 {
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		if ref.ID > 0 {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func (s *AgentRunService) workerInputArtifactRefs(ctx context.Context, run *AgentRun, timeoutSeconds int) []WorkerArtifactRef {
	assets := inputAssetsFromRunSummary(run)
	if len(assets) == 0 {
		return nil
	}
	ttl := s.workerInputAssetURLTTL(timeoutSeconds)
	refs := make([]WorkerArtifactRef, 0, len(assets))
	for i := range assets {
		asset := assets[i]
		url := strings.TrimSpace(asset.ObjectURL)
		if s.artifactStore != nil && s.artifactStore.IsConfigured() && strings.TrimSpace(asset.ObjectKey) != "" && !isExternalInputAssetURL(&asset) {
			if signed, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{StorageProvider: asset.StorageProvider, Bucket: asset.Bucket, ObjectKey: asset.ObjectKey}, ttl); err == nil && strings.TrimSpace(signed) != "" {
				url = signed
			}
		}
		metadata := copyAgentMap(asset.MetadataJSON)
		metadata["input_asset_id"] = asset.ID
		metadata["file_id"] = asset.ID
		metadata["asset_type"] = asset.AssetType
		metadata["asset_role"] = asset.AssetRole
		metadata["field_name"] = asset.FieldName
		refs = append(refs, WorkerArtifactRef{
			ArtifactID: asset.ID,
			Type:       AgentArtifactTypeInput,
			Name:       asset.Name,
			MimeType:   asset.MimeType,
			URL:        url,
			ObjectKey:  asset.ObjectKey,
			SizeBytes:  asset.SizeBytes,
			SHA256:     asset.SHA256,
			Metadata:   metadata,
		})
	}
	return refs
}

func (s *AgentRunService) workerInputAssetURLTTL(timeoutSeconds int) time.Duration {
	ttl := s.artifactDownloadTTL()
	if timeoutSeconds > 0 {
		runTTL := time.Duration(timeoutSeconds)*time.Second + 5*time.Minute
		if runTTL > ttl {
			ttl = runTTL
		}
	}
	if ttl < 10*time.Minute {
		return 10 * time.Minute
	}
	return ttl
}

func inputAssetsFromRunSummary(run *AgentRun) []AgentInputAsset {
	if run == nil || run.InputSummaryJSON == nil {
		return nil
	}
	raw, ok := run.InputSummaryJSON["input_assets"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	assets := make([]AgentInputAsset, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		asset := AgentInputAsset{
			ID:              int64FromAny(record["id"]),
			FieldName:       agentStringFromAny(record["field_name"]),
			AssetType:       agentStringFromAny(record["asset_type"]),
			AssetRole:       agentStringFromAny(record["asset_role"]),
			Name:            agentStringFromAny(record["name"]),
			MimeType:        agentStringFromAny(record["mime_type"]),
			StorageProvider: agentStringFromAny(record["storage_provider"]),
			Bucket:          agentStringFromAny(record["bucket"]),
			ObjectKey:       agentStringFromAny(record["object_key"]),
			ObjectURL:       agentStringFromAny(record["object_url"]),
			SizeBytes:       int64FromAny(record["size_bytes"]),
			SHA256:          agentStringFromAny(record["sha256"]),
			MetadataJSON:    mapFromAny(record["metadata_json"]),
		}
		if asset.ID <= 0 || strings.TrimSpace(asset.ObjectKey) == "" {
			continue
		}
		if asset.AssetType == "" {
			asset.AssetType = AgentInputAssetTypeFile
		}
		assets = append(assets, asset)
	}
	return assets
}

func copyAgentMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func agentStringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return copyAgentMap(m)
	}
	return map[string]any{}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pathExt(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx <= 0 || idx == len(name)-1 {
		return ""
	}
	return name[idx:]
}

func artifactFromWorkerRef(run *AgentRun, ref WorkerArtifactRef, expiresAt *time.Time) *AgentArtifact {
	if run == nil || strings.TrimSpace(ref.URL) == "" {
		return nil
	}
	artifactType := normalizeAgentArtifactType(ref.Type)
	name := normalizeArtifactName(ref.Name)
	if name == "" {
		name = "artifact"
	}
	objectKey := strings.TrimSpace(ref.ObjectKey)
	if objectKey == "" {
		objectKey = strings.TrimSpace(ref.URL)
	}
	return &AgentArtifact{
		RunID:           run.ID,
		UserID:          run.UserID,
		ArtifactType:    artifactType,
		Name:            name,
		MimeType:        strings.TrimSpace(ref.MimeType),
		StorageProvider: "external",
		ObjectKey:       objectKey,
		ObjectURL:       strings.TrimSpace(ref.URL),
		SizeBytes:       ref.SizeBytes,
		SHA256:          strings.TrimSpace(ref.SHA256),
		MetadataJSON:    ensureMap(ref.Metadata),
		ExpiresAt:       expiresAt,
	}
}
