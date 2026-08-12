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

func (s *OpenAIGatewayService) AdminListSeedanceVideoJobs(
	ctx context.Context,
	filters SeedanceTaskAdminFilters,
	page, pageSize int,
) ([]SeedanceTaskAdminItem, int64, error) {
	repo, err := s.seedanceAdminRepo()
	if err != nil {
		return nil, 0, err
	}
	return repo.ListAdminSeedanceTaskBindings(ctx, filters, page, pageSize)
}

func (s *OpenAIGatewayService) AdminGetSeedanceVideoJob(ctx context.Context, jobID string) (*SeedanceTaskAdminItem, error) {
	repo, err := s.seedanceAdminRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAdminSeedanceTaskBindingByJobID(ctx, jobID)
}

func (s *OpenAIGatewayService) AdminSyncSeedanceVideoJob(ctx context.Context, jobID string) (*SeedanceTaskAdminItem, error) {
	item, err := s.AdminGetSeedanceVideoJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("seedance task binding not found")
	}
	if !item.SettledAt.IsZero() {
		return item, nil
	}

	binding := item.SeedanceTaskBinding
	inspection, inspectErr := s.InspectSeedanceTask(ctx, &binding)
	if inspectErr != nil {
		if seedanceBindingExceedsSettlementMaxAge(&binding, time.Now().UTC()) {
			if _, killErr := s.AdminKillSeedanceVideoJob(ctx, jobID, joinSeedanceSettlementErrors(inspectErr.Error(), "admin sync timed out; forced failed settlement")); killErr != nil {
				return nil, killErr
			}
			return s.AdminGetSeedanceVideoJob(ctx, jobID)
		}
		_, _ = s.ForceCompleteSeedanceTaskSettlement(ctx, &binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         firstNonEmptyString(binding.TaskStatus, SeedanceTaskStatusUnknown),
			NextPollAt:         timePointer(time.Now().UTC().Add(seedancePollDelay(binding.SettlementAttempts+1, true))),
			RefundStatus:       binding.RefundStatus,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          inspectErr.Error(),
		})
		return s.AdminGetSeedanceVideoJob(ctx, jobID)
	}

	switch inspection.Status {
	case SeedanceTaskStatusSucceeded:
		now := time.Now().UTC()
		_, err = s.ForceCompleteSeedanceTaskSettlement(ctx, &binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         SeedanceTaskStatusSucceeded,
			SettledAt:          &now,
			RefundStatus:       SeedanceRefundStatusNotRequired,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
		})
	case SeedanceTaskStatusCancelled:
		_, err = s.adminRefundAndSettle(ctx, &binding, SeedanceTaskStatusCancelled, inspection.Error)
	case SeedanceTaskStatusFailed:
		_, err = s.adminRefundAndSettle(ctx, &binding, SeedanceTaskStatusFailed, inspection.Error)
	case SeedanceTaskStatusQueued, SeedanceTaskStatusRunning:
		if seedanceBindingExceedsSettlementMaxAge(&binding, time.Now().UTC()) {
			_, err = s.adminRefundAndSettle(ctx, &binding, SeedanceTaskStatusFailed, firstNonEmptyString(inspection.Error, "task exceeded maximum generation window; marked failed for settlement"))
			break
		}
		_, err = s.ForceCompleteSeedanceTaskSettlement(ctx, &binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         inspection.Status,
			NextPollAt:         timePointer(time.Now().UTC().Add(seedancePollDelay(binding.SettlementAttempts+1, false))),
			RefundStatus:       binding.RefundStatus,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          "",
		})
	default:
		_, err = s.ForceCompleteSeedanceTaskSettlement(ctx, &binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         SeedanceTaskStatusUnknown,
			NextPollAt:         timePointer(time.Now().UTC().Add(seedancePollDelay(binding.SettlementAttempts+1, true))),
			RefundStatus:       binding.RefundStatus,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          "unrecognized upstream task status: " + inspection.Status,
		})
	}
	if err != nil {
		return nil, err
	}
	return s.AdminGetSeedanceVideoJob(ctx, jobID)
}

func (s *OpenAIGatewayService) AdminKillSeedanceVideoJob(ctx context.Context, jobID, reason string) (*SeedanceTaskAdminItem, error) {
	item, err := s.AdminGetSeedanceVideoJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("seedance task binding not found")
	}
	if !item.SettledAt.IsZero() {
		return item, nil
	}

	binding := item.SeedanceTaskBinding
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "admin killed task"
	}
	s.adminBestEffortCancelUpstream(ctx, &binding)
	if _, err := s.adminRefundAndSettle(ctx, &binding, SeedanceTaskStatusCancelled, reason); err != nil {
		return nil, err
	}
	return s.AdminGetSeedanceVideoJob(ctx, jobID)
}

func (s *OpenAIGatewayService) AdminForceFailSeedanceVideoJob(ctx context.Context, jobID, reason string) (*SeedanceTaskAdminItem, error) {
	item, err := s.AdminGetSeedanceVideoJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("seedance task binding not found")
	}
	if !item.SettledAt.IsZero() {
		return item, nil
	}

	binding := item.SeedanceTaskBinding
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "admin forced failure"
	}
	s.adminBestEffortCancelUpstream(ctx, &binding)
	if _, err := s.adminRefundAndSettle(ctx, &binding, SeedanceTaskStatusFailed, reason); err != nil {
		return nil, err
	}
	return s.AdminGetSeedanceVideoJob(ctx, jobID)
}

func (s *OpenAIGatewayService) adminBestEffortCancelUpstream(ctx context.Context, binding *SeedanceTaskBinding) {
	if s == nil || s.accountRepo == nil || binding == nil || binding.AccountID <= 0 {
		return
	}
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil || account == nil || !account.IsFFLinkVideo() {
		return
	}
	upstreamJobID := strings.TrimSpace(binding.UpstreamJobID)
	if upstreamJobID == "" {
		upstreamJobID = strings.TrimSpace(binding.JobID)
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, _ = s.ForwardSeedance(cancelCtx, nil, account, http.MethodDelete, upstreamJobID, nil)
}

func (s *OpenAIGatewayService) adminRefundAndSettle(
	ctx context.Context,
	binding *SeedanceTaskBinding,
	status, taskError string,
) (bool, error) {
	if binding == nil {
		return false, errors.New("seedance task binding is required")
	}
	status = firstNonEmptyString(status, SeedanceTaskStatusFailed)
	taskError = strings.TrimSpace(taskError)

	if binding.RefundStatus == SeedanceRefundStatusApplied || binding.RefundStatus == SeedanceRefundStatusNotRequired {
		now := time.Now().UTC()
		return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         status,
			SettledAt:          &now,
			RefundStatus:       binding.RefundStatus,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          taskError,
		})
	}

	result, err := s.RefundSeedanceUsage(ctx, binding.JobID, binding.UserID, binding.APIKeyID)
	if err != nil {
		return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         status,
			NextPollAt:         timePointer(time.Now().UTC().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus:       SeedanceRefundStatusError,
			RefundAttempts:     binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          joinSeedanceSettlementErrors(taskError, "refund failed: "+err.Error()),
		})
	}
	if result == nil {
		return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         status,
			NextPollAt:         timePointer(time.Now().UTC().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus:       SeedanceRefundStatusError,
			RefundAttempts:     binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          joinSeedanceSettlementErrors(taskError, "refund returned no result"),
		})
	}
	if result.NotRequired {
		now := time.Now().UTC()
		return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         status,
			SettledAt:          &now,
			RefundStatus:       SeedanceRefundStatusNotRequired,
			RefundAttempts:     binding.RefundAttempts,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          taskError,
		})
	}
	if !result.Found || result.UsageLogID <= 0 {
		return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
			TaskStatus:         status,
			NextPollAt:         timePointer(time.Now().UTC().Add(seedanceRefundRetryDelay(binding.RefundAttempts + 1))),
			RefundStatus:       SeedanceRefundStatusPending,
			RefundAttempts:     binding.RefundAttempts + 1,
			SettlementAttempts: binding.SettlementAttempts + 1,
			LastError:          joinSeedanceSettlementErrors(taskError, "usage record is not yet available for refund"),
		})
	}
	now := time.Now().UTC()
	return s.ForceCompleteSeedanceTaskSettlement(ctx, binding, SeedanceTaskSettlementUpdate{
		TaskStatus:         status,
		SettledAt:          &now,
		RefundedAt:         &now,
		RefundStatus:       SeedanceRefundStatusApplied,
		RefundAttempts:     binding.RefundAttempts + 1,
		SettlementAttempts: binding.SettlementAttempts + 1,
		LastError:          taskError,
	})
}

func (s *OpenAIGatewayService) seedanceAdminRepo() (SeedanceTaskAdminRepository, error) {
	if s == nil || s.usageLogRepo == nil {
		return nil, errors.New("seedance task admin repository is unavailable")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskAdminRepository)
	if !ok || repo == nil {
		return nil, errors.New("seedance task admin repository is unavailable")
	}
	return repo, nil
}

func ParseSeedanceRequestSnapshot(snapshot []byte) map[string]any {
	out := map[string]any{}
	if len(snapshot) == 0 {
		return out
	}
	var raw map[string]any
	if err := json.Unmarshal(snapshot, &raw); err != nil {
		out["raw"] = string(snapshot)
		return out
	}
	return raw
}

func SeedancePublicResultPath(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ""
	}
	return fmt.Sprintf("/v1/videos/jobs/%s/content", jobID)
}
