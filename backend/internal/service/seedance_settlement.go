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

	// Settlement must not stay open forever when upstream becomes
	// unobservable. After this age, force a terminal failed + refund path.
	seedanceSettlementMaxAge = 2 * time.Hour
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

func (s *OpenAIGatewayService) ForceCompleteSeedanceTaskSettlement(
	ctx context.Context,
	binding *SeedanceTaskBinding,
	update SeedanceTaskSettlementUpdate,
) (bool, error) {
	if s == nil || s.usageLogRepo == nil || binding == nil {
		return false, errors.New("seedance task settlement repository is unavailable")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskAdminRepository)
	if !ok || repo == nil {
		return false, errors.New("seedance task admin repository is unavailable")
	}
	return repo.ForceCompleteSeedanceTaskSettlement(ctx, binding.ID, update)
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
//
// Inspection intentionally does NOT acquire account concurrency slots: the
// settlement worker must not compete with live generation traffic, or failed
// upstream tasks can remain "running" forever and block refunds.
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
	// Prefer public JobID for Lingdong-mapped tasks (ldv1_*). Using the bare
	// UpstreamJobID would route the settlement worker to Weijin instead of
	// Lingdong, leaving local status stuck on queued while upstream already failed.
	forwardTaskID := seedanceForwardTaskID(binding)
	if forwardTaskID == "" {
		return nil, errors.New("seedance task id is missing")
	}
	response, err := s.ForwardSeedance(ctx, nil, account, http.MethodGet, forwardTaskID, nil)
	if err != nil {
		if inspection, ok := seedanceInspectionFromError(err); ok {
			return inspection, nil
		}
		return nil, err
	}
	if response == nil || len(response.Body) == 0 {
		return nil, errors.New("seedance upstream task response is empty")
	}
	return seedanceInspectionFromPayload(response.Body)
}

func seedanceInspectionFromPayload(body []byte) (*SeedanceTaskInspection, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("seedance upstream task response is invalid")
	}
	status, _ := payload["status"].(string)
	if strings.TrimSpace(status) == "" {
		// OpenVideo returns the lifecycle in `state` rather than `status`.
		status, _ = payload["state"].(string)
	}
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
	if inspection.Error == "" {
		inspection.Error = sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))
	}
	return inspection, nil
}

// seedanceInspectionFromError maps permanent upstream inspect failures into a
// terminal inspection so settlement can refund instead of rescheduling forever.
func seedanceInspectionFromError(err error) (*SeedanceTaskInspection, bool) {
	if err == nil {
		return nil, false
	}
	var upstream *SeedanceUpstreamError
	if !errors.As(err, &upstream) || upstream == nil {
		return nil, false
	}
	if len(upstream.Body) > 0 {
		if inspection, parseErr := seedanceInspectionFromPayload(upstream.Body); parseErr == nil && inspection != nil {
			switch inspection.Status {
			case SeedanceTaskStatusSucceeded, SeedanceTaskStatusFailed, SeedanceTaskStatusCancelled:
				return inspection, true
			}
		}
	}
	message := strings.TrimSpace(SeedanceUpstreamErrorMessage(upstream.Body))
	lower := strings.ToLower(message)
	permanentByStatus := upstream.StatusCode == http.StatusNotFound ||
		upstream.StatusCode == http.StatusGone ||
		upstream.StatusCode == http.StatusMethodNotAllowed
	permanentByMessage := strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "task_not_found") ||
		strings.Contains(lower, "job_not_found") ||
		strings.Contains(lower, "no such task") ||
		strings.Contains(lower, "no such job") ||
		strings.Contains(lower, "unknown task") ||
		strings.Contains(lower, "unknown job")
	if !permanentByStatus && !permanentByMessage {
		return nil, false
	}
	if message == "" {
		message = "video task is no longer available upstream"
	}
	return &SeedanceTaskInspection{
		Status: SeedanceTaskStatusFailed,
		Error:  sanitizeUpstreamErrorMessage(message),
	}, true
}

func seedanceBindingExceedsSettlementMaxAge(binding *SeedanceTaskBinding, now time.Time) bool {
	if binding == nil || binding.CreatedAt.IsZero() {
		return false
	}
	return now.Sub(binding.CreatedAt) >= seedanceSettlementMaxAge
}
