package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	seedanceSettlementInterval       = 3 * time.Second
	seedanceSettlementLeaseDuration  = 3 * time.Minute
	seedanceSettlementProcessTimeout = 25 * time.Minute
	seedanceSettlementBatchSize      = 25
	seedanceSettlementConcurrency    = 4
	seedanceSettlementMaxSwitches    = 3
)

type SeedanceSettlementWorker struct {
	gateway       *OpenAIGatewayService
	media         *SeedanceMediaService
	concurrency   *ConcurrencyService
	apiKeys       *APIKeyService
	workerID      string
	interval      time.Duration
	leaseDuration time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	startOnce     sync.Once
	stopOnce      sync.Once
	wg            sync.WaitGroup

	claimSettlements   func(context.Context, string, int, time.Duration) ([]SeedanceTaskBinding, error)
	renewSettlement    func(context.Context, *SeedanceTaskBinding) (bool, error)
	completeSettlement func(context.Context, *SeedanceTaskBinding, SeedanceTaskSettlementUpdate) (bool, error)
	inspectTask        func(context.Context, *SeedanceTaskBinding) (*SeedanceTaskInspection, error)
	loadBinding        func(context.Context, *SeedanceTaskBinding) (*SeedanceTaskBinding, error)
	refundUsage        func(context.Context, string, int64, int64) (*SeedanceUsageRefundResult, error)
	startFallbackTask  func(context.Context, *SeedanceTaskBinding) (seedanceSettlementFallbackOutcome, error)
}

func NewSeedanceSettlementWorker(
	gateway *OpenAIGatewayService,
	media *SeedanceMediaService,
	concurrency *ConcurrencyService,
	apiKeys *APIKeyService,
) *SeedanceSettlementWorker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &SeedanceSettlementWorker{
		gateway:       gateway,
		media:         media,
		concurrency:   concurrency,
		apiKeys:       apiKeys,
		workerID:      uuid.NewString(),
		interval:      seedanceSettlementInterval,
		leaseDuration: seedanceSettlementLeaseDuration,
		ctx:           ctx,
		cancel:        cancel,
	}
	if gateway != nil {
		worker.claimSettlements = gateway.ClaimSeedanceTaskSettlements
		worker.renewSettlement = gateway.RenewSeedanceTaskSettlement
		worker.completeSettlement = gateway.CompleteSeedanceTaskSettlement
		worker.inspectTask = gateway.InspectSeedanceTask
		worker.loadBinding = func(ctx context.Context, binding *SeedanceTaskBinding) (*SeedanceTaskBinding, error) {
			if binding == nil {
				return nil, errors.New("seedance task binding is required")
			}
			groupID := binding.GroupID
			return gateway.GetSeedanceTaskBinding(ctx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID)
		}
		worker.refundUsage = gateway.RefundSeedanceUsage
	}
	worker.startFallbackTask = worker.startFallback
	return worker
}

func ProvideSeedanceSettlementWorker(
	gateway *OpenAIGatewayService,
	media *SeedanceMediaService,
	concurrency *ConcurrencyService,
	apiKeys *APIKeyService,
) *SeedanceSettlementWorker {
	worker := NewSeedanceSettlementWorker(gateway, media, concurrency, apiKeys)
	worker.Start()
	return worker
}

func (w *SeedanceSettlementWorker) Start() {
	if w == nil || w.claimSettlements == nil || w.interval <= 0 {
		return
	}
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(w.interval)
			defer ticker.Stop()
			w.runOnce()
			for {
				select {
				case <-ticker.C:
					w.runOnce()
				case <-w.ctx.Done():
					return
				}
			}
		}()
	})
}

func (w *SeedanceSettlementWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	w.wg.Wait()
}

func (w *SeedanceSettlementWorker) runOnce() {
	if w == nil || w.claimSettlements == nil {
		return
	}
	base := w.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 20*time.Second)
	leaseDuration := w.leaseDuration
	if leaseDuration <= 0 {
		leaseDuration = seedanceSettlementLeaseDuration
	}
	bindings, err := w.claimSettlements(ctx, w.workerID, seedanceSettlementBatchSize, leaseDuration)
	cancel()
	if err != nil {
		logger.L().Warn("seedance.settlement_claim_failed", zap.Error(err))
		return
	}
	if len(bindings) == 0 {
		return
	}
	semaphore := make(chan struct{}, seedanceSettlementConcurrency)
	var wg sync.WaitGroup
	for i := range bindings {
		binding := bindings[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimCtx, stopRenewal := w.maintainSettlementClaim(base, &binding)
			defer func() {
				if renewErr := stopRenewal(); renewErr != nil && base.Err() == nil {
					logger.L().Warn("seedance.settlement_lease_lost", zap.String("task_id", binding.JobID), zap.Error(renewErr))
				}
			}()
			select {
			case semaphore <- struct{}{}:
			case <-claimCtx.Done():
				return
			}
			defer func() { <-semaphore }()
			processCtx, cancel := context.WithTimeout(claimCtx, seedanceSettlementProcessTimeout)
			defer cancel()
			w.process(processCtx, &binding)
		}()
	}
	wg.Wait()
}

func (w *SeedanceSettlementWorker) maintainSettlementClaim(parent context.Context, binding *SeedanceTaskBinding) (context.Context, func() error) {
	ctx, cancel := context.WithCancel(parent)
	if w == nil || w.renewSettlement == nil || binding == nil {
		return ctx, func() error { cancel(); return nil }
	}
	done := make(chan error, 1)
	leaseDuration := w.leaseDuration
	if leaseDuration <= 0 {
		leaseDuration = seedanceSettlementLeaseDuration
	}
	go func() {
		ticker := time.NewTicker(leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				renewed, err := w.renewSettlement(renewCtx, binding)
				renewCancel()
				if err != nil || !renewed {
					if err == nil {
						err = errors.New("seedance settlement claim is no longer owned")
					}
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return ctx, func() error {
		cancel()
		return <-done
	}
}

func (w *SeedanceSettlementWorker) process(ctx context.Context, binding *SeedanceTaskBinding) {
	if binding == nil || w == nil {
		return
	}
	log := logger.L().With(zap.String("component", "seedance.settlement"), zap.String("task_id", binding.JobID))
	if binding.RefundStatus == SeedanceRefundStatusPending || binding.RefundStatus == SeedanceRefundStatusError {
		w.refundTerminal(ctx, binding, firstNonEmptyString(binding.TaskStatus, SeedanceTaskStatusFailed), binding.LastError)
		return
	}
	if binding.FallbackStatus == SeedanceFallbackStatusCancelled {
		w.refundTerminal(ctx, binding, SeedanceTaskStatusCancelled, binding.LastError)
		return
	}
	if binding.FallbackStatus == SeedanceFallbackStatusStarting && seedanceBindingLeaseActive(binding, time.Now()) {
		w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "", false)
		return
	}
	if w.inspectTask == nil {
		w.reschedule(ctx, binding, SeedanceTaskStatusUnknown, "seedance task inspection is unavailable", true)
		return
	}
	inspection, err := w.inspectTask(ctx, binding)
	if err != nil {
		log.Warn("seedance.settlement_poll_failed", zap.Error(err), zap.Int64("account_id", binding.AccountID))
		w.reschedule(ctx, binding, firstNonEmptyString(binding.TaskStatus, SeedanceTaskStatusUnknown), err.Error(), true)
		return
	}
	switch inspection.Status {
	case SeedanceTaskStatusQueued, SeedanceTaskStatusRunning:
		w.reschedule(ctx, binding, inspection.Status, "", false)
	case SeedanceTaskStatusSucceeded:
		w.settleWithoutRefund(ctx, binding, SeedanceTaskStatusSucceeded)
	case SeedanceTaskStatusCancelled:
		w.refundTerminal(ctx, binding, SeedanceTaskStatusCancelled, inspection.Error)
	case SeedanceTaskStatusFailed:
		w.processFailed(ctx, binding, inspection.Error)
	default:
		w.reschedule(ctx, binding, SeedanceTaskStatusUnknown, "unrecognized upstream task status: "+inspection.Status, true)
	}
}

func (w *SeedanceSettlementWorker) processFailed(ctx context.Context, binding *SeedanceTaskBinding, upstreamError string) {
	latest, err := w.currentBinding(ctx, binding)
	if err != nil {
		w.reschedule(ctx, binding, SeedanceTaskStatusUnknown, err.Error(), true)
		return
	}
	if !sameSeedanceUpstreamTask(binding, latest) {
		w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "seedance task provider changed while status was being reconciled", false)
		return
	}
	switch latest.FallbackStatus {
	case SeedanceFallbackStatusReady, SeedanceFallbackStatusStarting:
		if w.startFallbackTask == nil {
			w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "seedance fallback lifecycle is unavailable", true)
			return
		}
		outcome, err := w.startFallbackTask(ctx, latest)
		if err != nil {
			w.reschedule(ctx, binding, SeedanceTaskStatusQueued, err.Error(), true)
			return
		}
		switch outcome {
		case seedanceSettlementFallbackActive, seedanceSettlementFallbackUnknown:
			w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "", false)
		case seedanceSettlementFallbackRejected:
			w.refundTerminal(ctx, binding, SeedanceTaskStatusFailed, upstreamError)
		default:
			w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "fallback state changed", true)
		}
	case SeedanceFallbackStatusCancelling:
		w.reschedule(ctx, binding, SeedanceTaskStatusRunning, "", false)
	case SeedanceFallbackStatusActive, SeedanceFallbackStatusFailed, "":
		w.refundTerminal(ctx, binding, SeedanceTaskStatusFailed, upstreamError)
	case SeedanceFallbackStatusCancelled:
		w.refundTerminal(ctx, binding, SeedanceTaskStatusCancelled, upstreamError)
	default:
		w.reschedule(ctx, binding, SeedanceTaskStatusUnknown, "unrecognized fallback state: "+latest.FallbackStatus, true)
	}
}

func (w *SeedanceSettlementWorker) currentBinding(ctx context.Context, binding *SeedanceTaskBinding) (*SeedanceTaskBinding, error) {
	if binding == nil {
		return nil, errors.New("seedance task binding is required")
	}
	if w == nil || w.loadBinding == nil {
		return nil, errors.New("seedance task binding lookup is unavailable")
	}
	latest, err := w.loadBinding(ctx, binding)
	if err != nil {
		return nil, fmt.Errorf("reload seedance task binding: %w", err)
	}
	if latest == nil || latest.ID != binding.ID {
		return nil, errors.New("seedance task binding changed while being reconciled")
	}
	return latest, nil
}

func sameSeedanceUpstreamTask(left, right *SeedanceTaskBinding) bool {
	if left == nil || right == nil {
		return false
	}
	return left.AccountID == right.AccountID &&
		strings.TrimSpace(left.UpstreamJobID) == strings.TrimSpace(right.UpstreamJobID)
}

type seedanceSettlementFallbackOutcome uint8

const (
	seedanceSettlementFallbackRetry seedanceSettlementFallbackOutcome = iota
	seedanceSettlementFallbackActive
	seedanceSettlementFallbackUnknown
	seedanceSettlementFallbackRejected
)

func (w *SeedanceSettlementWorker) startFallback(ctx context.Context, binding *SeedanceTaskBinding) (seedanceSettlementFallbackOutcome, error) {
	groupID := binding.GroupID
	claimed, claimToken, err := w.gateway.ClaimSeedanceTaskFallback(ctx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID)
	if err != nil {
		return seedanceSettlementFallbackRetry, err
	}
	if !claimed {
		latest, loadErr := w.gateway.GetSeedanceTaskBinding(ctx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID)
		if loadErr != nil || latest == nil {
			return seedanceSettlementFallbackRetry, firstNonNilError(loadErr, errors.New("fallback state is unavailable"))
		}
		switch latest.FallbackStatus {
		case SeedanceFallbackStatusActive:
			return seedanceSettlementFallbackActive, nil
		case SeedanceFallbackStatusStarting:
			return seedanceSettlementFallbackUnknown, nil
		case SeedanceFallbackStatusFailed:
			return seedanceSettlementFallbackRejected, nil
		default:
			return seedanceSettlementFallbackRetry, errors.New("fallback state changed")
		}
	}
	fallbackCtx, stopFallbackRenewal := w.maintainFallbackClaim(
		ctx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID, claimToken,
	)
	defer func() {
		if renewErr := stopFallbackRenewal(); renewErr != nil && ctx.Err() == nil {
			logger.L().Warn("seedance.fallback_lease_lost", zap.String("task_id", binding.JobID), zap.Error(renewErr))
		}
	}()

	finalized := false
	releaseClaim := func() {
		if finalized {
			return
		}
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer releaseCancel()
		_, _ = w.gateway.ReleaseSeedanceTaskFallback(releaseCtx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID, claimToken)
	}
	requestInfo, err := RestoreSeedanceFallbackRequest(binding.RequestSnapshot, binding.FallbackModel)
	if err != nil {
		releaseClaim()
		return seedanceSettlementFallbackRetry, err
	}
	if w.media == nil {
		releaseClaim()
		return seedanceSettlementFallbackRetry, errors.New("seedance media service is unavailable")
	}
	mediaRelease, err := w.media.AcquireMediaIO(fallbackCtx, SeedanceMediaOwner{UserID: binding.UserID, APIKeyID: binding.APIKeyID, GroupID: binding.GroupID}, 1)
	if err != nil {
		releaseClaim()
		return seedanceSettlementFallbackRetry, err
	}
	defer mediaRelease()
	if err := w.media.RefreshSeedanceFallbackMediaURLs(fallbackCtx, SeedanceMediaOwner{
		UserID: binding.UserID, APIKeyID: binding.APIKeyID, GroupID: binding.GroupID,
	}, requestInfo); err != nil {
		releaseClaim()
		return seedanceSettlementFallbackRetry, err
	}
	if requestInfo.HasReferenceMedia() {
		requestInfo.HuiquMedia, err = w.media.PrepareHuiquMedia(fallbackCtx, requestInfo)
		if err != nil {
			releaseClaim()
			return seedanceSettlementFallbackRetry, err
		}
		defer requestInfo.HuiquMedia.Cleanup()
	}

	failedAccountIDs := make(map[int64]struct{})
	attemptedAccounts := 0
	explicitRejections := 0
	sessionHash := SeedanceTaskSessionHash(binding.JobID+":fallback", binding.UserID, binding.APIKeyID)
	for switchCount := 0; switchCount <= seedanceSettlementMaxSwitches; switchCount++ {
		selection, _, selectErr := w.gateway.SelectAccountWithSchedulerForCapability(
			fallbackCtx, &groupID, "", sessionHash, binding.FallbackModel, failedAccountIDs,
			OpenAIUpstreamTransportHTTPSSE, "", false, false, false, PlatformSeedance,
		)
		if selectErr != nil || selection == nil || selection.Account == nil || !selection.Account.IsHuiquVideo() {
			if selection != nil && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if allSeedanceFallbackAttemptsExplicitlyRejected(attemptedAccounts, explicitRejections) {
				outcome, finalizeErr := w.finalizeFallbackRejection(fallbackCtx, binding, claimToken)
				if finalizeErr == nil {
					finalized = true
				}
				return outcome, finalizeErr
			}
			releaseClaim()
			return seedanceSettlementFallbackRetry, firstNonNilError(selectErr, errors.New("no compatible fallback account is available"))
		}
		account := selection.Account
		release, acquired, acquireErr := w.acquireFallbackAccount(fallbackCtx, &groupID, sessionHash, selection)
		if acquireErr != nil || !acquired {
			releaseClaim()
			return seedanceSettlementFallbackRetry, firstNonNilError(acquireErr, errors.New("fallback account is at capacity"))
		}
		forwardCtx := WithSeedanceIdempotencyKey(fallbackCtx, "seedance-fallback-"+binding.JobID)
		attemptedAccounts++
		forwarded, forwardErr := func() (*SeedanceUpstreamResponse, error) {
			if release != nil {
				defer release()
			}
			return w.gateway.ForwardSeedance(forwardCtx, nil, account, http.MethodPost, "", requestInfo)
		}()
		if forwardErr != nil {
			var unknown *SeedanceUpstreamAcceptanceUnknownError
			if errors.As(forwardErr, &unknown) {
				return seedanceSettlementFallbackUnknown, nil
			}
			var failover *UpstreamFailoverError
			if errors.As(forwardErr, &failover) {
				failedAccountIDs[account.ID] = struct{}{}
				if isSeedanceFallbackExplicitRejection(forwardErr) {
					explicitRejections++
				}
				w.gateway.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(binding.FallbackModel), false, nil)
				continue
			}
			if isSeedanceFallbackExplicitRejection(forwardErr) {
				explicitRejections++
				failedAccountIDs[account.ID] = struct{}{}
				continue
			}
			releaseClaim()
			return seedanceSettlementFallbackRetry, forwardErr
		}
		if forwarded == nil || forwarded.Result == nil || strings.TrimSpace(forwarded.Result.ResponseID) == "" {
			return seedanceSettlementFallbackUnknown, nil
		}
		activated, activateErr := w.gateway.ActivateSeedanceTaskFallback(
			fallbackCtx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID,
			claimToken, account.ID, forwarded.Result.ResponseID,
		)
		if activateErr != nil || !activated {
			return seedanceSettlementFallbackUnknown, nil
		}
		finalized = true
		w.gateway.ReportOpenAIAccountScheduleResult(account.ID, forwarded.Result.UpstreamModel, true, nil)
		w.gateway.RecordOpenAIAccountSwitch()
		return seedanceSettlementFallbackActive, nil
	}
	if allSeedanceFallbackAttemptsExplicitlyRejected(attemptedAccounts, explicitRejections) {
		outcome, finalizeErr := w.finalizeFallbackRejection(fallbackCtx, binding, claimToken)
		if finalizeErr == nil {
			finalized = true
		}
		return outcome, finalizeErr
	}
	releaseClaim()
	return seedanceSettlementFallbackRetry, errors.New("fallback account attempts exhausted")
}

func isSeedanceFallbackExplicitRejection(err error) bool {
	statusCode := 0
	var failover *UpstreamFailoverError
	if errors.As(err, &failover) {
		statusCode = failover.StatusCode
	} else {
		var upstream *SeedanceUpstreamError
		if !errors.As(err, &upstream) {
			return false
		}
		statusCode = upstream.StatusCode
	}
	return statusCode >= http.StatusBadRequest &&
		statusCode < http.StatusInternalServerError &&
		statusCode != http.StatusRequestTimeout &&
		statusCode != http.StatusTooManyRequests
}

func allSeedanceFallbackAttemptsExplicitlyRejected(attemptedAccounts, explicitRejections int) bool {
	return attemptedAccounts > 0 && explicitRejections == attemptedAccounts
}

func (w *SeedanceSettlementWorker) finalizeFallbackRejection(
	ctx context.Context,
	binding *SeedanceTaskBinding,
	claimToken string,
) (seedanceSettlementFallbackOutcome, error) {
	if w == nil || w.gateway == nil || binding == nil {
		return seedanceSettlementFallbackRetry, errors.New("fallback rejection could not be finalized")
	}
	groupID := binding.GroupID
	failed, err := w.gateway.FailSeedanceTaskFallback(
		ctx, &groupID, binding.JobID, binding.UserID, binding.APIKeyID, claimToken,
	)
	if err != nil || !failed {
		return seedanceSettlementFallbackRetry, firstNonNilError(err, errors.New("fallback rejection could not be finalized"))
	}
	return seedanceSettlementFallbackRejected, nil
}

func (w *SeedanceSettlementWorker) maintainFallbackClaim(
	parent context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (context.Context, func() error) {
	if w == nil || w.gateway == nil {
		return MaintainSeedanceFallbackLease(parent, nil)
	}
	return MaintainSeedanceFallbackLease(parent, func(renewCtx context.Context) (bool, error) {
		return w.gateway.RenewSeedanceTaskFallback(renewCtx, groupID, jobID, userID, apiKeyID, claimToken)
	})
}

func (w *SeedanceSettlementWorker) acquireFallbackAccount(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	selection *AccountSelectionResult,
) (func(), bool, error) {
	if selection == nil || selection.Account == nil {
		return nil, false, errors.New("fallback account selection is invalid")
	}
	if selection.Acquired {
		return selection.ReleaseFunc, true, nil
	}
	if selection.WaitPlan == nil || w.concurrency == nil {
		return nil, false, nil
	}
	result, err := w.concurrency.AcquireAccountSlot(ctx, selection.Account.ID, selection.WaitPlan.MaxConcurrency)
	if err != nil || result == nil || !result.Acquired {
		return nil, false, err
	}
	if err := w.gateway.BindStickySession(ctx, groupID, sessionHash, selection.Account.ID); err != nil {
		logger.L().Warn("seedance.settlement_sticky_bind_failed", zap.Int64("account_id", selection.Account.ID), zap.Error(err))
	}
	return result.ReleaseFunc, true, nil
}

func (w *SeedanceSettlementWorker) refundTerminal(ctx context.Context, binding *SeedanceTaskBinding, status, taskError string) {
	latest, loadErr := w.currentBinding(ctx, binding)
	if loadErr != nil {
		w.reschedule(ctx, binding, status, loadErr.Error(), true)
		return
	}
	if !sameSeedanceUpstreamTask(binding, latest) || seedanceRefundMustWait(latest, status) {
		w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "seedance task state changed before refund", false)
		return
	}
	if w.refundUsage == nil {
		w.reschedule(ctx, binding, status, "seedance usage refund is unavailable", true)
		return
	}
	result, err := w.refundUsage(ctx, binding.JobID, binding.UserID, binding.APIKeyID)
	if err != nil {
		w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus: status, NextPollAt: timePointer(time.Now().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus: SeedanceRefundStatusError, RefundAttempts: binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1, LastError: joinSeedanceSettlementErrors(taskError, "refund failed: "+err.Error()),
		})
		return
	}
	if result == nil {
		w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus: status, NextPollAt: timePointer(time.Now().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus: SeedanceRefundStatusError, RefundAttempts: binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1, LastError: joinSeedanceSettlementErrors(taskError, "refund returned no result"),
		})
		return
	}
	if result.NotRequired {
		now := time.Now().UTC()
		w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus: status, SettledAt: &now, RefundStatus: SeedanceRefundStatusNotRequired,
			RefundAttempts: binding.RefundAttempts, SettlementAttempts: binding.SettlementAttempts + 1, LastError: taskError,
		})
		return
	}
	if !result.Found || result.UsageLogID <= 0 {
		w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus: status, NextPollAt: timePointer(time.Now().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus: SeedanceRefundStatusPending, RefundAttempts: binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1, LastError: joinSeedanceSettlementErrors(taskError, "usage record is not yet available for refund"),
		})
		return
	}
	now := time.Now().UTC()
	w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
		TaskStatus: status, SettledAt: &now, RefundedAt: &now, RefundStatus: SeedanceRefundStatusApplied,
		RefundAttempts: binding.RefundAttempts + 1, SettlementAttempts: binding.SettlementAttempts + 1, LastError: taskError,
	})
	w.invalidateAPIKeyAuth(ctx, binding.APIKeyID)
}

func (w *SeedanceSettlementWorker) settleWithoutRefund(ctx context.Context, binding *SeedanceTaskBinding, status string) {
	latest, err := w.currentBinding(ctx, binding)
	if err != nil {
		w.reschedule(ctx, binding, SeedanceTaskStatusUnknown, err.Error(), true)
		return
	}
	if !sameSeedanceUpstreamTask(binding, latest) {
		w.reschedule(ctx, binding, SeedanceTaskStatusQueued, "seedance task provider changed while success was being reconciled", false)
		return
	}
	now := time.Now().UTC()
	w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
		TaskStatus: status, SettledAt: &now, RefundStatus: SeedanceRefundStatusNotRequired,
		RefundAttempts: binding.RefundAttempts, SettlementAttempts: binding.SettlementAttempts + 1,
	})
}

func (w *SeedanceSettlementWorker) reschedule(ctx context.Context, binding *SeedanceTaskBinding, status, lastError string, failedPoll bool) {
	attempts := binding.SettlementAttempts + 1
	delay := seedancePollDelay(attempts, failedPoll)
	w.complete(ctx, binding, SeedanceTaskSettlementUpdate{
		TaskStatus: status, NextPollAt: timePointer(time.Now().Add(delay)), RefundStatus: binding.RefundStatus,
		RefundAttempts: binding.RefundAttempts, SettlementAttempts: attempts, LastError: lastError,
	})
}

func (w *SeedanceSettlementWorker) complete(ctx context.Context, binding *SeedanceTaskBinding, update SeedanceTaskSettlementUpdate) {
	if w == nil || w.completeSettlement == nil || binding == nil {
		return
	}
	if update.SettledAt != nil && w.media != nil && len(binding.RequestSnapshot) > 0 {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		cleanupErr := w.media.DeleteSeedanceFallbackMedia(cleanupCtx, SeedanceMediaOwner{
			UserID: binding.UserID, APIKeyID: binding.APIKeyID, GroupID: binding.GroupID,
		}, binding.RequestSnapshot)
		cleanupCancel()
		if cleanupErr != nil {
			update.SettledAt = nil
			update.NextPollAt = timePointer(time.Now().Add(seedancePollDelay(update.SettlementAttempts, true)))
			update.LastError = joinSeedanceSettlementErrors(update.LastError, "fallback media cleanup failed: "+cleanupErr.Error())
			logger.L().Warn("seedance.settlement_media_cleanup_failed", zap.String("task_id", binding.JobID), zap.Error(cleanupErr))
		}
	}
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	updated, err := w.completeSettlement(updateCtx, binding, update)
	if err != nil || !updated {
		logger.L().Error("seedance.settlement_update_failed", zap.String("task_id", binding.JobID), zap.Error(err))
	}
}

func seedanceRefundMustWait(binding *SeedanceTaskBinding, terminalStatus string) bool {
	if binding == nil || !binding.SettledAt.IsZero() {
		return true
	}
	if strings.TrimSpace(binding.FallbackModel) == "" {
		return false
	}
	if terminalStatus == SeedanceTaskStatusCancelled {
		return binding.FallbackStatus == SeedanceFallbackStatusStarting ||
			binding.FallbackStatus == SeedanceFallbackStatusCancelling
	}
	switch binding.FallbackStatus {
	case SeedanceFallbackStatusActive, SeedanceFallbackStatusFailed, SeedanceFallbackStatusCancelled:
		return false
	case SeedanceFallbackStatusReady, SeedanceFallbackStatusStarting, SeedanceFallbackStatusCancelling, "":
		return true
	default:
		return true
	}
}

func (w *SeedanceSettlementWorker) invalidateAPIKeyAuth(ctx context.Context, apiKeyID int64) {
	if w.apiKeys == nil || apiKeyID <= 0 {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	apiKey, err := w.apiKeys.GetByID(cacheCtx, apiKeyID)
	if err == nil && apiKey != nil && strings.TrimSpace(apiKey.Key) != "" {
		w.apiKeys.InvalidateAuthCacheByKey(cacheCtx, apiKey.Key)
	}
}

func seedanceBindingLeaseActive(binding *SeedanceTaskBinding, now time.Time) bool {
	return binding != nil && !binding.FallbackLeaseUntil.IsZero() && !now.After(binding.FallbackLeaseUntil)
}

func seedancePollDelay(attempts int, failed bool) time.Duration {
	var delay time.Duration
	if failed {
		switch {
		case attempts <= 3:
			delay = 5 * time.Second
		case attempts <= 8:
			delay = 15 * time.Second
		default:
			delay = time.Minute
		}
	} else {
		switch {
		case attempts <= 12:
			delay = 5 * time.Second
		case attempts <= 40:
			delay = 15 * time.Second
		default:
			delay = 30 * time.Second
		}
	}
	return jitterSeedanceDelay(delay)
}

func seedanceRefundRetryDelay(attempts int) time.Duration {
	delay := time.Duration(1<<min(attempts, 8)) * time.Second
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	return jitterSeedanceDelay(delay)
}

func jitterSeedanceDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return time.Second
	}
	return time.Duration(float64(delay) * (0.8 + rand.Float64()*0.4))
}

func timePointer(value time.Time) *time.Time { return &value }

func firstNonNilError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return fmt.Errorf("unknown seedance settlement error")
}

func joinSeedanceSettlementErrors(values ...string) string {
	parts := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
	}
	return strings.Join(parts, "; ")
}
