package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const publicVideoContentBindingMinTTL = 24 * time.Hour

var ErrPublicVideoContentBindingNotFound = errors.New("public video content binding not found")

const (
	PublicVideoProviderGrok     = "grok"
	PublicVideoProviderOpenAI   = "openai"
	PublicVideoProviderSeedance = "seedance"
)

// PublicVideoContentBinding restores the original request owner from an opaque
// provider task ID. The task ID acts as the bearer credential for content only.
type PublicVideoContentBinding struct {
	RequestID string `json:"request_id"`
	UserID    int64  `json:"user_id"`
	APIKeyID  int64  `json:"api_key_id"`
	GroupID   int64  `json:"group_id"`
	AccountID int64  `json:"account_id"`
	Provider  string `json:"provider"`
}

type PublicVideoContentBindingCache interface {
	SetPublicVideoContentBinding(ctx context.Context, requestID string, payload []byte, ttl time.Duration) error
	GetPublicVideoContentBinding(ctx context.Context, requestID string) ([]byte, error)
}

// SeedancePublicTaskBindingRepository is deliberately separate from
// SeedanceTaskBindingRepository so existing owner-scoped repository stubs do
// not gain a cross-owner lookup method.
type SeedancePublicTaskBindingRepository interface {
	GetSeedanceTaskBindingByJobID(ctx context.Context, jobID string) (*SeedanceTaskBinding, error)
}

func (s *OpenAIGatewayService) BindPublicVideoContent(
	ctx context.Context,
	binding PublicVideoContentBinding,
) error {
	if s == nil || s.cache == nil {
		return errors.New("public video content binding cache is unavailable")
	}
	binding.RequestID = strings.TrimSpace(binding.RequestID)
	binding.Provider = strings.ToLower(strings.TrimSpace(binding.Provider))
	if !validPublicVideoContentBinding(&binding) {
		return errors.New("public video content binding is invalid")
	}
	cache, ok := s.cache.(PublicVideoContentBindingCache)
	if !ok || cache == nil {
		return errors.New("public video content binding cache is unavailable")
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		return fmt.Errorf("marshal public video content binding: %w", err)
	}
	return cache.SetPublicVideoContentBinding(ctx, binding.RequestID, payload, s.publicVideoContentBindingTTL())
}

func (s *OpenAIGatewayService) ResolvePublicVideoContentBinding(
	ctx context.Context,
	requestID string,
) (*PublicVideoContentBinding, error) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || s.cache == nil || requestID == "" {
		return nil, ErrPublicVideoContentBindingNotFound
	}
	cache, ok := s.cache.(PublicVideoContentBindingCache)
	if !ok || cache == nil {
		return nil, errors.New("public video content binding cache is unavailable")
	}
	payload, err := cache.GetPublicVideoContentBinding(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get public video content binding: %w", err)
	}
	if len(payload) == 0 {
		return nil, ErrPublicVideoContentBindingNotFound
	}
	var binding PublicVideoContentBinding
	if err := json.Unmarshal(payload, &binding); err != nil {
		return nil, fmt.Errorf("decode public video content binding: %w", err)
	}
	binding.RequestID = strings.TrimSpace(binding.RequestID)
	binding.Provider = strings.ToLower(strings.TrimSpace(binding.Provider))
	if binding.RequestID != requestID || !validPublicVideoContentBinding(&binding) {
		return nil, errors.New("public video content binding is invalid")
	}
	return &binding, nil
}

func (s *OpenAIGatewayService) ResolvePublicSeedanceTaskBinding(
	ctx context.Context,
	jobID string,
) (*PublicVideoContentBinding, error) {
	jobID = strings.TrimSpace(jobID)
	if s == nil || jobID == "" {
		return nil, ErrPublicVideoContentBindingNotFound
	}
	repo, ok := s.usageLogRepo.(SeedancePublicTaskBindingRepository)
	if !ok || repo == nil {
		return nil, errors.New("public seedance task binding repository is unavailable")
	}
	binding, err := repo.GetSeedanceTaskBindingByJobID(ctx, jobID)
	if err != nil || binding == nil {
		if err != nil && !errors.Is(err, ErrPublicVideoContentBindingNotFound) {
			return nil, fmt.Errorf("get public seedance task binding: %w", err)
		}
		return nil, ErrPublicVideoContentBindingNotFound
	}
	publicBinding := &PublicVideoContentBinding{
		RequestID: binding.JobID,
		UserID:    binding.UserID,
		APIKeyID:  binding.APIKeyID,
		GroupID:   binding.GroupID,
		AccountID: binding.AccountID,
		Provider:  PublicVideoProviderSeedance,
	}
	if publicBinding.RequestID != jobID || !validPublicVideoContentBinding(publicBinding) {
		return nil, errors.New("public seedance task binding is invalid")
	}
	return publicBinding, nil
}

// ResolvePublicVideoContentAccount loads the exact account that created an
// existing Grok/OpenAI task. Downloading must not re-schedule onto another
// account or require that the creator account still admits new work.
func (s *OpenAIGatewayService) ResolvePublicVideoContentAccount(
	ctx context.Context,
	binding *PublicVideoContentBinding,
) (*Account, error) {
	if s == nil || s.accountRepo == nil || !validPublicVideoContentBinding(binding) {
		return nil, errors.New("public video content account is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get public video content account: %w", err)
	}
	groupID := binding.GroupID
	if account == nil || !s.openAIAccountMatchesSchedulingGroup(account, &groupID) {
		return nil, errors.New("public video content account is unavailable")
	}
	switch binding.Provider {
	case PublicVideoProviderGrok:
		if account.Platform != PlatformGrok {
			return nil, errors.New("public video content account provider mismatch")
		}
	case PublicVideoProviderOpenAI:
		if account.Platform != PlatformOpenAI {
			return nil, errors.New("public video content account provider mismatch")
		}
	default:
		return nil, errors.New("public video content account provider mismatch")
	}
	return account, nil
}

func validPublicVideoContentBinding(binding *PublicVideoContentBinding) bool {
	if binding == nil || binding.RequestID == "" || len(binding.RequestID) > 512 ||
		binding.UserID <= 0 || binding.APIKeyID <= 0 || binding.GroupID <= 0 || binding.AccountID <= 0 {
		return false
	}
	switch binding.Provider {
	case PublicVideoProviderGrok, PublicVideoProviderOpenAI, PublicVideoProviderSeedance:
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) publicVideoContentBindingTTL() time.Duration {
	ttl := publicVideoContentBindingMinTTL
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		stickyTTL := time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
		if stickyTTL > ttl {
			ttl = stickyTTL
		}
	}
	return ttl
}
