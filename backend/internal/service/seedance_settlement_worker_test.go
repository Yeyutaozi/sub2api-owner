package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type seedanceSettlementHarness struct {
	worker        *SeedanceSettlementWorker
	latest        SeedanceTaskBinding
	inspection    *SeedanceTaskInspection
	inspectionErr error
	refundResult  *SeedanceUsageRefundResult
	refundErr     error
	refundCalls   int
	fallbackCalls int
	fallback      seedanceSettlementFallbackOutcome
	fallbackErr   error
	updates       []SeedanceTaskSettlementUpdate
}

func newSeedanceSettlementHarness(binding SeedanceTaskBinding) *seedanceSettlementHarness {
	h := &seedanceSettlementHarness{
		latest:       binding,
		inspection:   &SeedanceTaskInspection{Status: SeedanceTaskStatusRunning},
		refundResult: &SeedanceUsageRefundResult{Found: true, Applied: true, UsageLogID: 91},
		fallback:     seedanceSettlementFallbackActive,
	}
	h.worker = &SeedanceSettlementWorker{
		inspectTask: func(context.Context, *SeedanceTaskBinding) (*SeedanceTaskInspection, error) {
			return h.inspection, h.inspectionErr
		},
		loadBinding: func(context.Context, *SeedanceTaskBinding) (*SeedanceTaskBinding, error) {
			copy := h.latest
			return &copy, nil
		},
		refundUsage: func(context.Context, string, int64, int64) (*SeedanceUsageRefundResult, error) {
			h.refundCalls++
			return h.refundResult, h.refundErr
		},
		completeSettlement: func(_ context.Context, _ *SeedanceTaskBinding, update SeedanceTaskSettlementUpdate) (bool, error) {
			h.updates = append(h.updates, update)
			return true, nil
		},
	}
	h.worker.startFallbackTask = func(context.Context, *SeedanceTaskBinding) (seedanceSettlementFallbackOutcome, error) {
		h.fallbackCalls++
		return h.fallback, h.fallbackErr
	}
	return h
}

func seedanceSettlementBinding() SeedanceTaskBinding {
	return SeedanceTaskBinding{
		ID: 1, UserID: 2, APIKeyID: 3, GroupID: 4, AccountID: 5,
		JobID: "public-job", UpstreamJobID: "upstream-job", Model: "sd2-mx933-720-1s",
		TaskStatus: SeedanceTaskStatusRunning,
	}
}

func TestSeedanceSettlementDirect933FailureRefundsWithoutClientPoll(t *testing.T) {
	binding := seedanceSettlementBinding()
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed, Error: "provider failed"}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusApplied, h.updates[0].RefundStatus)
	require.Equal(t, SeedanceTaskStatusFailed, h.updates[0].TaskStatus)
}

func TestSeedanceSettlementRunOnceClaimsDirect933FailureWithoutClientPoll(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.SettlementClaimedBy = "worker-one"
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed, Error: "provider failed"}
	h.worker.ctx = context.Background()
	h.worker.workerID = "worker-one"
	h.worker.claimSettlements = func(context.Context, string, int, time.Duration) ([]SeedanceTaskBinding, error) {
		return []SeedanceTaskBinding{binding}, nil
	}

	h.worker.runOnce()

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusApplied, h.updates[0].RefundStatus)
}

func TestSeedanceSettlementRunOnceRenewsClaimsWaitingForConcurrency(t *testing.T) {
	bindings := make([]SeedanceTaskBinding, seedanceSettlementConcurrency+1)
	for index := range bindings {
		bindings[index] = seedanceSettlementBinding()
		bindings[index].ID = int64(index + 1)
		bindings[index].JobID = fmt.Sprintf("public-job-%d", index+1)
		bindings[index].UpstreamJobID = fmt.Sprintf("upstream-job-%d", index+1)
		bindings[index].SettlementClaimedBy = "worker-one"
	}

	blockInspect := make(chan struct{})
	runDone := make(chan struct{})
	var renewMu sync.Mutex
	renewed := make(map[string]int)
	worker := &SeedanceSettlementWorker{
		ctx:           context.Background(),
		workerID:      "worker-one",
		leaseDuration: 30 * time.Millisecond,
		claimSettlements: func(context.Context, string, int, time.Duration) ([]SeedanceTaskBinding, error) {
			return bindings, nil
		},
		renewSettlement: func(_ context.Context, binding *SeedanceTaskBinding) (bool, error) {
			renewMu.Lock()
			renewed[binding.JobID]++
			renewMu.Unlock()
			return true, nil
		},
		inspectTask: func(context.Context, *SeedanceTaskBinding) (*SeedanceTaskInspection, error) {
			<-blockInspect
			return &SeedanceTaskInspection{Status: SeedanceTaskStatusRunning}, nil
		},
		completeSettlement: func(context.Context, *SeedanceTaskBinding, SeedanceTaskSettlementUpdate) (bool, error) {
			return true, nil
		},
	}

	go func() {
		defer close(runDone)
		worker.runOnce()
	}()

	waitingJobID := bindings[len(bindings)-1].JobID
	require.Eventually(t, func() bool {
		renewMu.Lock()
		defer renewMu.Unlock()
		return renewed[waitingJobID] > 0
	}, time.Second, 10*time.Millisecond)

	close(blockInspect)
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("settlement run did not finish")
	}
}

func TestSeedanceSettlementPrimaryFailureStartsFallbackWithoutRefund(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.Model = "seedance-2.0"
	binding.FallbackModel = "sd2-mx933-720-1s"
	binding.FallbackStatus = SeedanceFallbackStatusReady
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.fallbackCalls)
	require.Zero(t, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceTaskStatusQueued, h.updates[0].TaskStatus)
}

func TestSeedanceSettlementFallbackFailureRefundsExactlyOnce(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.Model = "seedance-2.0"
	binding.FallbackModel = "sd2-mx933-720-1s"
	binding.FallbackStatus = SeedanceFallbackStatusActive
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Zero(t, h.fallbackCalls)
	require.Len(t, h.updates, 1)
	require.NotNil(t, h.updates[0].SettledAt)
}

func TestSeedanceSettlementExplicitFallbackRejectionRefundsAfterStateRecheck(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.Model = "seedance-2.0"
	binding.FallbackModel = "sd2-mx933-720-1s"
	binding.FallbackStatus = SeedanceFallbackStatusReady
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed}
	h.fallback = seedanceSettlementFallbackRejected
	h.worker.startFallbackTask = func(context.Context, *SeedanceTaskBinding) (seedanceSettlementFallbackOutcome, error) {
		h.fallbackCalls++
		h.latest.FallbackStatus = SeedanceFallbackStatusFailed
		return seedanceSettlementFallbackRejected, nil
	}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.fallbackCalls)
	require.Equal(t, 1, h.refundCalls)
	require.NotNil(t, h.updates[0].SettledAt)
}

func TestSeedanceSettlementUnknownPollingErrorNeverRefunds(t *testing.T) {
	binding := seedanceSettlementBinding()
	h := newSeedanceSettlementHarness(binding)
	h.inspectionErr = errors.New("status timeout")

	h.worker.process(context.Background(), &binding)

	require.Zero(t, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.Contains(t, h.updates[0].LastError, "status timeout")
}

func TestSeedanceSettlementMissingUsageLogRetriesIndefinitely(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.RefundStatus = SeedanceRefundStatusPending
	binding.RefundAttempts = 100
	h := newSeedanceSettlementHarness(binding)
	h.refundResult = &SeedanceUsageRefundResult{}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusPending, h.updates[0].RefundStatus)
	require.Equal(t, 101, h.updates[0].RefundAttempts)
}

func TestSeedanceSettlementRefundFailureRetainsBothErrorsAndRetries(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.RefundStatus = SeedanceRefundStatusPending
	h := newSeedanceSettlementHarness(binding)
	h.refundErr = errors.New("database temporarily unavailable")

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusError, h.updates[0].RefundStatus)
	require.Contains(t, h.updates[0].LastError, "database temporarily unavailable")
	require.NotNil(t, h.updates[0].NextPollAt)
}

func TestSeedanceSettlementAlreadyRefundedUsageIsSettledIdempotently(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.RefundStatus = SeedanceRefundStatusPending
	h := newSeedanceSettlementHarness(binding)
	h.refundResult = &SeedanceUsageRefundResult{Found: true, Applied: false, UsageLogID: 91}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusApplied, h.updates[0].RefundStatus)
}

func TestSeedanceSettlementUnbilledSimpleModeFailureNeedsNoRefund(t *testing.T) {
	binding := seedanceSettlementBinding()
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed, Error: "provider failed"}
	h.refundResult = &SeedanceUsageRefundResult{NotRequired: true}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Equal(t, SeedanceTaskStatusFailed, h.updates[0].TaskStatus)
	require.Equal(t, SeedanceRefundStatusNotRequired, h.updates[0].RefundStatus)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Nil(t, h.updates[0].RefundedAt)
}

func TestSeedanceSettlementProviderChangePreventsStaleRefund(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.FallbackModel = "sd2-mx933-720-1s"
	binding.FallbackStatus = SeedanceFallbackStatusActive
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusFailed}
	h.latest.AccountID = 99
	h.latest.UpstreamJobID = "new-upstream-job"

	h.worker.process(context.Background(), &binding)

	require.Zero(t, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceTaskStatusQueued, h.updates[0].TaskStatus)
}

func TestSeedanceSettlementSuccessNeverRefunds(t *testing.T) {
	binding := seedanceSettlementBinding()
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusSucceeded}

	h.worker.process(context.Background(), &binding)

	require.Zero(t, h.refundCalls)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceRefundStatusNotRequired, h.updates[0].RefundStatus)
}

func TestSeedanceSettlementRetriesTerminalUpdateWhenFallbackMediaCleanupFails(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.Model = "seedance-2.0"
	binding.FallbackModel = "sd2-mx933-720-1s"
	snapshot, err := SnapshotSeedanceFallbackRequest(&SeedanceRequestInfo{
		Model: "seedance-2.0", Resolution: "720p",
		StoredMedia: []SeedanceStoredMediaReference{{
			Slot: seedanceStoredMediaVideo, StorageProvider: "cos", Bucket: "seedance-test",
			ObjectKey: "agent-artifacts/seedance/inputs/task/2/3/reference.mp4", DeleteAfterSettlement: true,
		}},
	})
	require.NoError(t, err)
	binding.RequestSnapshot = snapshot
	h := newSeedanceSettlementHarness(binding)
	h.inspection = &SeedanceTaskInspection{Status: SeedanceTaskStatusSucceeded}
	store := newSeedanceMediaMemoryStore()
	store.deleteErr = errors.New("temporary object storage failure")
	h.worker.media = NewSeedanceMediaService(store, nil, nil)

	h.worker.process(context.Background(), &binding)

	require.Len(t, h.updates, 1)
	require.Nil(t, h.updates[0].SettledAt)
	require.NotNil(t, h.updates[0].NextPollAt)
	require.Contains(t, h.updates[0].LastError, "fallback media cleanup failed")
}
