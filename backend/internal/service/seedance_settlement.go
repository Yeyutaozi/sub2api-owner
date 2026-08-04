package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	SeedanceTaskStatusQueued    = "queued"
	SeedanceTaskStatusRunning   = "running"
	SeedanceTaskStatusSucceeded = "succeeded"
	SeedanceTaskStatusFailed    = "failed"
	SeedanceTaskStatusCancelled = "cancelled"
	SeedanceTaskStatusUnknown   = "unknown"

	SeedanceRefundStatusPending     = "pending"
	SeedanceRefundStatusApplied     = "applied"
	SeedanceRefundStatusError       = "error"
	SeedanceRefundStatusNotRequired = "not_required"
)

type SeedanceTaskInspection struct {
	Status string
	Error  string
}

func (s *OpenAIGatewayService) ClaimSeedanceTaskSettlements(
	ctx context.Context,
	workerID string,
	limit int,
	leaseDuration time.Duration,
) ([]SeedanceTaskBinding, error) {
	if s == nil || s.usageLogRepo == nil {
		return nil, errors.New("seedance task settlement repository is unavailable")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskSettlementRepository)
	if !ok || repo == nil {
		return nil, errors.New("seedance task settlement repository is unavailable")
	}
	return repo.ClaimSeedanceTaskSettlements(ctx, workerID, limit, leaseDuration)
}

func (s *OpenAIGatewayService) CompleteSeedanceTaskSettlement(
	ctx context.Context,
	binding *SeedanceTaskBinding,
	update SeedanceTaskSettlementUpdate,
) (bool, error) {
	if s == nil || s.usageLogRepo == nil || binding == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskSettlementRepository)
	if !ok || repo == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	return repo.CompleteSeedanceTaskSettlement(ctx, binding.ID, binding.SettlementClaimedBy, update)
}

func (s *OpenAIGatewayService) RenewSeedanceTaskSettlement(
	ctx context.Context,
	binding *SeedanceTaskBinding,
) (bool, error) {
	if s == nil || s.usageLogRepo == nil || binding == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskSettlementRepository)
	if !ok || repo == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	return repo.RenewSeedanceTaskSettlement(ctx, binding.ID, binding.SettlementClaimedBy)
}

// InspectSeedanceTask reads the current bound upstream task even when the
// account was disabled after creation. Existing jobs must remain observable so
// settlement cannot be stranded by an administrative scheduling change.
func (s *OpenAIGatewayService) InspectSeedanceTask(ctx context.Context, binding *SeedanceTaskBinding) (*SeedanceTaskInspection, error) {
	if s == nil || s.accountRepo == nil || binding == nil || binding.AccountID <= 0 {
		return nil, errors.New("seedance task inspection is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil {
		return nil, fmt.Errorf("load seedance task account: %w", err)
	}
	if account == nil || !account.IsFFLinkVideo() || account.GetVideoProvider() == "" {
		return nil, errors.New("seedance task account is unavailable")
	}
	var release func()
	if s.concurrencyService != nil {
		maxConcurrency := account.Concurrency
		if maxConcurrency <= 0 {
			maxConcurrency = 1
		}
		slot, slotErr := s.concurrencyService.AcquireAccountSlot(ctx, account.ID, maxConcurrency)
		if slotErr != nil {
			return nil, fmt.Errorf("acquire seedance task polling slot: %w", slotErr)
		}
		if slot == nil || !slot.Acquired {
			return nil, errors.New("seedance task account is at capacity")
		}
		release = slot.ReleaseFunc
	}
	if release != nil {
		defer release()
	}
	upstreamJobID := strings.TrimSpace(binding.UpstreamJobID)
	if upstreamJobID == "" {
		upstreamJobID = strings.TrimSpace(binding.JobID)
	}
	response, err := s.ForwardSeedance(ctx, nil, account, http.MethodGet, upstreamJobID, nil)
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.Body) == 0 {
		return nil, errors.New("seedance upstream task response is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, errors.New("seedance upstream task response is invalid")
	}
	status, _ := payload["status"].(string)
	status = MapSeedanceTaskStatus(status)
	if status == "" {
		return nil, errors.New("seedance upstream task status is missing")
	}
	inspection := &SeedanceTaskInspection{Status: status}
	if value, ok := payload["error"]; ok && value != nil {
		switch typed := value.(type) {
		case string:
			inspection.Error = sanitizeUpstreamErrorMessage(typed)
		default:
			encoded, _ := json.Marshal(typed)
			inspection.Error = sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(encoded))
		}
	}
	return inspection, nil
}
