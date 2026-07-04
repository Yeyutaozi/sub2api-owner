package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	SettingKeyTokenRewardConfig = "token_reward_config"

	settingKeyTokenRewardCycleSnapshotPrefix = "token_reward_cycle_snapshot:"

	TokenRewardCycleWeekly  = "weekly"
	TokenRewardCycleMonthly = "monthly"

	TokenRewardTokenUnitRaw = "raw"
	TokenRewardTokenUnitK   = "K"
	TokenRewardTokenUnitM   = "M"
	TokenRewardTokenUnitB   = "B"
	TokenRewardTokenUnitT   = "T"

	TokenRewardTierStatusLocked    = "locked"
	TokenRewardTierStatusClaimable = "claimable"
	TokenRewardTierStatusClaimed   = "claimed"

	maxTokenRewardTokens = int64(1<<63 - 1)
)

var (
	ErrTokenRewardDisabled       = infraerrors.Forbidden("TOKEN_REWARD_DISABLED", "token reward plan is disabled")
	ErrTokenRewardTierNotFound   = infraerrors.NotFound("TOKEN_REWARD_TIER_NOT_FOUND", "token reward tier not found")
	ErrTokenRewardNotEligible    = infraerrors.BadRequest("TOKEN_REWARD_NOT_ELIGIBLE", "token requirement not reached")
	ErrTokenRewardAlreadyClaimed = infraerrors.Conflict("TOKEN_REWARD_ALREADY_CLAIMED", "reward already claimed in this cycle")
	ErrTokenRewardUnavailable    = infraerrors.ServiceUnavailable("TOKEN_REWARD_SERVICE_UNAVAILABLE", "token reward service unavailable")

	tokenRewardTierIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type TokenRewardRepository interface {
	GetCycleTokens(ctx context.Context, userID int64, start, end time.Time) (int64, error)
	ListClaims(ctx context.Context, userID int64, cycleType string, cycleStart time.Time) ([]TokenRewardClaim, error)
	ListClaimHistory(ctx context.Context, userID int64, page, pageSize int) ([]TokenRewardClaim, int64, error)
	ListAllClaimHistory(ctx context.Context, page, pageSize int) ([]TokenRewardAdminClaim, int64, error)
	Claim(ctx context.Context, input TokenRewardClaimInput) (*TokenRewardClaimResult, error)
}

type TokenRewardConfig struct {
	Enabled bool              `json:"enabled"`
	Cycle   string            `json:"cycle"`
	Tiers   []TokenRewardTier `json:"tiers"`
}

type TokenRewardTier struct {
	ID            string  `json:"id"`
	Tokens        int64   `json:"tokens"`
	TokenUnit     string  `json:"token_unit"`
	RewardBalance float64 `json:"reward_balance"`
}

type TokenRewardCycle struct {
	Type  string    `json:"type"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type TokenRewardClaim struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	TierID         string    `json:"tier_id"`
	CycleType      string    `json:"cycle_type"`
	CycleStart     time.Time `json:"cycle_start"`
	CycleEnd       time.Time `json:"cycle_end"`
	RequiredTokens int64     `json:"required_tokens"`
	TokenUnit      string    `json:"token_unit"`
	RewardBalance  float64   `json:"reward_balance"`
	TokenSnapshot  int64     `json:"token_snapshot"`
	ClaimedAt      time.Time `json:"claimed_at"`
}

type TokenRewardAdminClaim struct {
	TokenRewardClaim
	UserEmail string `json:"user_email"`
}

type TokenRewardClaimInput struct {
	UserID         int64
	TierID         string
	CycleType      string
	CycleStart     time.Time
	CycleEnd       time.Time
	RequiredTokens int64
	TokenUnit      string
	RewardBalance  float64
}

type TokenRewardClaimResult struct {
	Claim      TokenRewardClaim `json:"claim"`
	NewBalance float64          `json:"new_balance"`
}

type TokenRewardTierStatus struct {
	Tier      TokenRewardTier `json:"tier"`
	Status    string          `json:"status"`
	ClaimedAt *time.Time      `json:"claimed_at,omitempty"`
}

type TokenRewardStatus struct {
	Config        TokenRewardConfig       `json:"config"`
	Cycle         TokenRewardCycle        `json:"cycle"`
	CurrentTokens int64                   `json:"current_tokens"`
	Tiers         []TokenRewardTierStatus `json:"tiers"`
}

type tokenRewardCycleSnapshot struct {
	Config    TokenRewardConfig `json:"config"`
	Cycle     TokenRewardCycle  `json:"cycle"`
	CreatedAt time.Time         `json:"created_at"`
}

type TokenRewardService struct {
	repo                TokenRewardRepository
	settingRepo         SettingRepository
	billingCacheService *BillingCacheService
}

func NewTokenRewardService(repo TokenRewardRepository, settingRepo SettingRepository, billingCacheService *BillingCacheService) *TokenRewardService {
	return &TokenRewardService{
		repo:                repo,
		settingRepo:         settingRepo,
		billingCacheService: billingCacheService,
	}
}

func (s *TokenRewardService) GetConfig(ctx context.Context) (TokenRewardConfig, error) {
	if s == nil || s.settingRepo == nil {
		return defaultTokenRewardConfig(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyTokenRewardConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaultTokenRewardConfig(), nil
		}
		return TokenRewardConfig{}, err
	}
	return parseTokenRewardConfig(raw)
}

func (s *TokenRewardService) UpdateConfig(ctx context.Context, cfg TokenRewardConfig) (TokenRewardConfig, error) {
	if s == nil || s.settingRepo == nil {
		return TokenRewardConfig{}, ErrTokenRewardUnavailable
	}
	normalized, err := normalizeTokenRewardConfig(cfg)
	if err != nil {
		return TokenRewardConfig{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return TokenRewardConfig{}, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyTokenRewardConfig, string(payload)); err != nil {
		return TokenRewardConfig{}, err
	}
	if normalized.Enabled {
		if err := s.syncCurrentCycleSnapshot(ctx, normalized, timezone.Now()); err != nil {
			return TokenRewardConfig{}, err
		}
	}
	return normalized, nil
}

func (s *TokenRewardService) GetStatus(ctx context.Context, userID int64) (*TokenRewardStatus, error) {
	if s == nil || s.repo == nil {
		return nil, ErrTokenRewardUnavailable
	}
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg, cycle, err := s.resolveCurrentCycleConfig(ctx, cfg, timezone.Now())
	if err != nil {
		return nil, err
	}
	tokens, err := s.repo.GetCycleTokens(ctx, userID, cycle.Start, cycle.End)
	if err != nil {
		return nil, err
	}
	claims, err := s.repo.ListClaims(ctx, userID, cycle.Type, cycle.Start)
	if err != nil {
		return nil, err
	}
	claimed := make(map[string]time.Time, len(claims))
	for _, claim := range claims {
		claimed[claim.TierID] = claim.ClaimedAt
	}
	statuses := make([]TokenRewardTierStatus, 0, len(cfg.Tiers))
	for _, tier := range cfg.Tiers {
		status := TokenRewardTierStatus{Tier: tier, Status: TokenRewardTierStatusLocked}
		requiredTokens, err := tokenRewardRequiredTokens(tier)
		if err != nil {
			return nil, err
		}
		if claimedAt, ok := claimed[tier.ID]; ok {
			status.Status = TokenRewardTierStatusClaimed
			t := claimedAt
			status.ClaimedAt = &t
		} else if cfg.Enabled && tokens >= requiredTokens {
			status.Status = TokenRewardTierStatusClaimable
		}
		statuses = append(statuses, status)
	}
	return &TokenRewardStatus{
		Config:        cfg,
		Cycle:         cycle,
		CurrentTokens: tokens,
		Tiers:         statuses,
	}, nil
}

func (s *TokenRewardService) Claim(ctx context.Context, userID int64, tierID string) (*TokenRewardClaimResult, error) {
	if s == nil || s.repo == nil {
		return nil, ErrTokenRewardUnavailable
	}
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg, cycle, err := s.resolveCurrentCycleConfig(ctx, cfg, timezone.Now())
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrTokenRewardDisabled
	}
	tier, ok := findTokenRewardTier(cfg.Tiers, tierID)
	if !ok {
		return nil, ErrTokenRewardTierNotFound
	}
	requiredTokens, err := tokenRewardRequiredTokens(tier)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.Claim(ctx, TokenRewardClaimInput{
		UserID:         userID,
		TierID:         tier.ID,
		CycleType:      cycle.Type,
		CycleStart:     cycle.Start,
		CycleEnd:       cycle.End,
		RequiredTokens: requiredTokens,
		TokenUnit:      tier.TokenUnit,
		RewardBalance:  tier.RewardBalance,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateBalanceCache(userID)
	return result, nil
}

func (s *TokenRewardService) ListClaimHistory(ctx context.Context, userID int64, page, pageSize int) ([]TokenRewardClaim, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrTokenRewardUnavailable
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return s.repo.ListClaimHistory(ctx, userID, page, pageSize)
}

func (s *TokenRewardService) ListAllClaimHistory(ctx context.Context, page, pageSize int) ([]TokenRewardAdminClaim, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrTokenRewardUnavailable
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return s.repo.ListAllClaimHistory(ctx, page, pageSize)
}

func (s *TokenRewardService) resolveCurrentCycleConfig(ctx context.Context, liveCfg TokenRewardConfig, now time.Time) (TokenRewardConfig, TokenRewardCycle, error) {
	liveCfg, err := normalizeTokenRewardConfig(liveCfg)
	if err != nil {
		return TokenRewardConfig{}, TokenRewardCycle{}, err
	}
	if s == nil || s.settingRepo == nil {
		cycle := CurrentTokenRewardCycle(liveCfg.Cycle, now)
		return liveCfg, cycle, nil
	}
	if snap, ok, err := s.findCurrentCycleSnapshot(ctx, liveCfg.Cycle, now); err != nil {
		return TokenRewardConfig{}, TokenRewardCycle{}, err
	} else if ok {
		snap.Config.Enabled = liveCfg.Enabled
		if liveCfg.Enabled && snap.Cycle.Type == liveCfg.Cycle && !reflect.DeepEqual(snap.Config, liveCfg) {
			snap.Config = liveCfg
			snap.CreatedAt = now
			if err := s.saveCycleSnapshot(ctx, snap); err != nil {
				return TokenRewardConfig{}, TokenRewardCycle{}, err
			}
		}
		return snap.Config, snap.Cycle, nil
	}
	cycle := CurrentTokenRewardCycle(liveCfg.Cycle, now)
	if !liveCfg.Enabled {
		return liveCfg, cycle, nil
	}
	snap := tokenRewardCycleSnapshot{
		Config:    liveCfg,
		Cycle:     cycle,
		CreatedAt: now,
	}
	if err := s.saveCycleSnapshot(ctx, snap); err != nil {
		return TokenRewardConfig{}, TokenRewardCycle{}, err
	}
	return liveCfg, cycle, nil
}

func (s *TokenRewardService) syncCurrentCycleSnapshot(ctx context.Context, cfg TokenRewardConfig, now time.Time) error {
	if s == nil || s.settingRepo == nil || !cfg.Enabled {
		return nil
	}
	cfg, err := normalizeTokenRewardConfig(cfg)
	if err != nil {
		return err
	}
	cycle := CurrentTokenRewardCycle(cfg.Cycle, now)
	return s.saveCycleSnapshot(ctx, tokenRewardCycleSnapshot{
		Config:    cfg,
		Cycle:     cycle,
		CreatedAt: now,
	})
}

func (s *TokenRewardService) findCurrentCycleSnapshot(ctx context.Context, preferredCycle string, now time.Time) (tokenRewardCycleSnapshot, bool, error) {
	cycles := []string{normalizeTokenRewardCycle(preferredCycle), TokenRewardCycleWeekly, TokenRewardCycleMonthly}
	seen := make(map[string]struct{}, len(cycles))
	for _, cycleType := range cycles {
		if _, ok := seen[cycleType]; ok {
			continue
		}
		seen[cycleType] = struct{}{}
		cycle := CurrentTokenRewardCycle(cycleType, now)
		snap, ok, err := s.getCycleSnapshot(ctx, cycle)
		if err != nil || ok {
			return snap, ok, err
		}
	}
	return tokenRewardCycleSnapshot{}, false, nil
}

func (s *TokenRewardService) getCycleSnapshot(ctx context.Context, cycle TokenRewardCycle) (tokenRewardCycleSnapshot, bool, error) {
	raw, err := s.settingRepo.GetValue(ctx, tokenRewardCycleSnapshotKey(cycle))
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return tokenRewardCycleSnapshot{}, false, nil
		}
		return tokenRewardCycleSnapshot{}, false, err
	}
	var snap tokenRewardCycleSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return tokenRewardCycleSnapshot{}, false, infraerrors.BadRequest("TOKEN_REWARD_SNAPSHOT_INVALID", "invalid token reward cycle snapshot").WithCause(err)
	}
	cfg, err := normalizeTokenRewardConfig(snap.Config)
	if err != nil {
		return tokenRewardCycleSnapshot{}, false, err
	}
	snap.Config = cfg
	if snap.Cycle.Type == "" || snap.Cycle.Start.IsZero() || snap.Cycle.End.IsZero() {
		snap.Cycle = cycle
	}
	return snap, true, nil
}

func (s *TokenRewardService) saveCycleSnapshot(ctx context.Context, snap tokenRewardCycleSnapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, tokenRewardCycleSnapshotKey(snap.Cycle), string(payload))
}

func tokenRewardCycleSnapshotKey(cycle TokenRewardCycle) string {
	return fmt.Sprintf("%s%s:%s", settingKeyTokenRewardCycleSnapshotPrefix, cycle.Type, cycle.Start.Format("20060102"))
}

func (s *TokenRewardService) invalidateBalanceCache(userID int64) {
	if s == nil || s.billingCacheService == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.token_reward", "invalidate user balance cache failed user_id=%d err=%v", userID, err)
		}
	}()
}

func defaultTokenRewardConfig() TokenRewardConfig {
	return TokenRewardConfig{
		Enabled: false,
		Cycle:   TokenRewardCycleWeekly,
		Tiers:   []TokenRewardTier{},
	}
}

func parseTokenRewardConfig(raw string) (TokenRewardConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultTokenRewardConfig(), nil
	}
	var cfg TokenRewardConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_CONFIG_INVALID", "invalid token reward config").WithCause(err)
	}
	return normalizeTokenRewardConfig(cfg)
}

func normalizeTokenRewardConfig(cfg TokenRewardConfig) (TokenRewardConfig, error) {
	cycle := normalizeTokenRewardCycle(cfg.Cycle)
	if cycle == "" {
		return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_CYCLE_INVALID", "cycle must be weekly or monthly")
	}
	seen := make(map[string]struct{}, len(cfg.Tiers))
	seenRequiredTokens := make(map[int64]string, len(cfg.Tiers))
	tiers := make([]TokenRewardTier, 0, len(cfg.Tiers))
	for i, tier := range cfg.Tiers {
		tier.ID = strings.TrimSpace(tier.ID)
		if tier.ID == "" {
			tier.ID = fmt.Sprintf("tier_%d", i+1)
		}
		if !tokenRewardTierIDPattern.MatchString(tier.ID) {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_TIER_ID_INVALID", "tier id must contain only letters, numbers, underscores or hyphens")
		}
		if _, ok := seen[tier.ID]; ok {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_TIER_DUPLICATE", "tier id must be unique")
		}
		seen[tier.ID] = struct{}{}
		if tier.Tokens <= 0 {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_TOKENS_INVALID", "tier tokens must be greater than zero")
		}
		tier.TokenUnit = normalizeTokenRewardUnit(tier.TokenUnit)
		if tier.TokenUnit == "" {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_TOKEN_UNIT_INVALID", "token unit must be raw, K, M, B or T")
		}
		requiredTokens, err := tokenRewardRequiredTokens(tier)
		if err != nil {
			return TokenRewardConfig{}, err
		}
		if existingID, ok := seenRequiredTokens[requiredTokens]; ok {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_TIER_THRESHOLD_DUPLICATE", fmt.Sprintf("tier token thresholds must be unique: %s and %s", existingID, tier.ID))
		}
		seenRequiredTokens[requiredTokens] = tier.ID
		if tier.RewardBalance <= 0 || math.IsNaN(tier.RewardBalance) || math.IsInf(tier.RewardBalance, 0) {
			return TokenRewardConfig{}, infraerrors.BadRequest("TOKEN_REWARD_BALANCE_INVALID", "reward balance must be greater than zero")
		}
		tiers = append(tiers, tier)
	}
	sort.SliceStable(tiers, func(i, j int) bool {
		left, _ := tokenRewardRequiredTokens(tiers[i])
		right, _ := tokenRewardRequiredTokens(tiers[j])
		if left == right {
			return tiers[i].ID < tiers[j].ID
		}
		return left < right
	})
	return TokenRewardConfig{
		Enabled: cfg.Enabled,
		Cycle:   cycle,
		Tiers:   tiers,
	}, nil
}

func normalizeTokenRewardCycle(cycle string) string {
	cycle = strings.ToLower(strings.TrimSpace(cycle))
	if cycle == "" {
		return TokenRewardCycleWeekly
	}
	if cycle != TokenRewardCycleWeekly && cycle != TokenRewardCycleMonthly {
		return ""
	}
	return cycle
}

func tokenRewardRequiredTokens(tier TokenRewardTier) (int64, error) {
	factor := tokenRewardUnitFactor(tier.TokenUnit)
	if factor <= 0 {
		return 0, infraerrors.BadRequest("TOKEN_REWARD_TOKEN_UNIT_INVALID", "token unit must be raw, K, M, B or T")
	}
	if tier.Tokens > maxTokenRewardTokens/factor {
		return 0, infraerrors.BadRequest("TOKEN_REWARD_TOKENS_INVALID", "tier tokens are too large")
	}
	return tier.Tokens * factor, nil
}

func tokenRewardUnitFactor(unit string) int64 {
	switch normalizeTokenRewardUnit(unit) {
	case TokenRewardTokenUnitRaw:
		return 1
	case TokenRewardTokenUnitK:
		return 1_000
	case TokenRewardTokenUnitM:
		return 1_000_000
	case TokenRewardTokenUnitB:
		return 1_000_000_000
	case TokenRewardTokenUnitT:
		return 1_000_000_000_000
	default:
		return 0
	}
}

func normalizeTokenRewardUnit(unit string) string {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "", "RAW":
		return TokenRewardTokenUnitRaw
	case TokenRewardTokenUnitK:
		return TokenRewardTokenUnitK
	case TokenRewardTokenUnitM:
		return TokenRewardTokenUnitM
	case TokenRewardTokenUnitB:
		return TokenRewardTokenUnitB
	case TokenRewardTokenUnitT:
		return TokenRewardTokenUnitT
	default:
		return ""
	}
}

func findTokenRewardTier(tiers []TokenRewardTier, id string) (TokenRewardTier, bool) {
	id = strings.TrimSpace(id)
	for _, tier := range tiers {
		if tier.ID == id {
			return tier, true
		}
	}
	return TokenRewardTier{}, false
}

func CurrentTokenRewardCycle(cycle string, now time.Time) TokenRewardCycle {
	cycle = strings.ToLower(strings.TrimSpace(cycle))
	if cycle != TokenRewardCycleMonthly {
		cycle = TokenRewardCycleWeekly
	}
	switch cycle {
	case TokenRewardCycleMonthly:
		start := timezone.StartOfMonth(now)
		return TokenRewardCycle{
			Type:  TokenRewardCycleMonthly,
			Start: start,
			End:   start.AddDate(0, 1, 0),
		}
	default:
		start := timezone.StartOfWeek(now)
		return TokenRewardCycle{
			Type:  TokenRewardCycleWeekly,
			Start: start,
			End:   start.AddDate(0, 0, 7),
		}
	}
}
