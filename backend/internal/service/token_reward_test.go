package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func TestNormalizeTokenRewardConfigSortsAndDefaults(t *testing.T) {
	cfg, err := normalizeTokenRewardConfig(TokenRewardConfig{
		Enabled: true,
		Tiers: []TokenRewardTier{
			{ID: "large", Tokens: 5000, RewardBalance: 5},
			{ID: "small", Tokens: 1000, RewardBalance: 1},
		},
	})
	if err != nil {
		t.Fatalf("normalizeTokenRewardConfig() error = %v", err)
	}
	if cfg.Cycle != TokenRewardCycleWeekly {
		t.Fatalf("cycle = %q, want %q", cfg.Cycle, TokenRewardCycleWeekly)
	}
	if cfg.Tiers[0].TokenUnit != TokenRewardTokenUnitRaw {
		t.Fatalf("default token unit = %q, want %q", cfg.Tiers[0].TokenUnit, TokenRewardTokenUnitRaw)
	}
	if cfg.Tiers[0].ID != "small" || cfg.Tiers[1].ID != "large" {
		t.Fatalf("tiers not sorted by token threshold: %+v", cfg.Tiers)
	}
}

func TestNormalizeTokenRewardConfigKeepsTierUnits(t *testing.T) {
	cfg, err := normalizeTokenRewardConfig(TokenRewardConfig{
		Cycle: TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "million", Tokens: 5, TokenUnit: "m", RewardBalance: 5},
			{ID: "trillion", Tokens: 1, TokenUnit: "T", RewardBalance: 10},
		},
	})
	if err != nil {
		t.Fatalf("normalizeTokenRewardConfig() error = %v", err)
	}
	if cfg.Tiers[0].TokenUnit != TokenRewardTokenUnitM {
		t.Fatalf("first tier token unit = %q, want %q", cfg.Tiers[0].TokenUnit, TokenRewardTokenUnitM)
	}
	if cfg.Tiers[1].TokenUnit != TokenRewardTokenUnitT {
		t.Fatalf("second tier token unit = %q, want %q", cfg.Tiers[1].TokenUnit, TokenRewardTokenUnitT)
	}
	required, err := tokenRewardRequiredTokens(cfg.Tiers[0])
	if err != nil {
		t.Fatalf("tokenRewardRequiredTokens() error = %v", err)
	}
	if required != 5_000_000 {
		t.Fatalf("required tokens = %d, want %d", required, int64(5_000_000))
	}
}

func TestNormalizeTokenRewardConfigSortsByResolvedTokenThreshold(t *testing.T) {
	cfg, err := normalizeTokenRewardConfig(TokenRewardConfig{
		Cycle: TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "one_b", Tokens: 1, TokenUnit: "B", RewardBalance: 10},
			{ID: "two_m", Tokens: 2, TokenUnit: "M", RewardBalance: 2},
			{ID: "five_k", Tokens: 5, TokenUnit: "K", RewardBalance: 1},
		},
	})
	if err != nil {
		t.Fatalf("normalizeTokenRewardConfig() error = %v", err)
	}
	if cfg.Tiers[0].ID != "five_k" || cfg.Tiers[1].ID != "two_m" || cfg.Tiers[2].ID != "one_b" {
		t.Fatalf("tiers not sorted by resolved token threshold: %+v", cfg.Tiers)
	}
}

func TestNormalizeTokenRewardConfigRejectsDuplicateTier(t *testing.T) {
	_, err := normalizeTokenRewardConfig(TokenRewardConfig{
		Cycle: TokenRewardCycleMonthly,
		Tiers: []TokenRewardTier{
			{ID: "same", Tokens: 1000, RewardBalance: 1},
			{ID: "same", Tokens: 2000, RewardBalance: 2},
		},
	})
	if err == nil {
		t.Fatal("normalizeTokenRewardConfig() expected duplicate tier error")
	}
}

func TestNormalizeTokenRewardConfigRejectsDuplicateThreshold(t *testing.T) {
	_, err := normalizeTokenRewardConfig(TokenRewardConfig{
		Cycle: TokenRewardCycleMonthly,
		Tiers: []TokenRewardTier{
			{ID: "raw", Tokens: 1000, TokenUnit: TokenRewardTokenUnitRaw, RewardBalance: 1},
			{ID: "k", Tokens: 1, TokenUnit: TokenRewardTokenUnitK, RewardBalance: 2},
		},
	})
	if err == nil {
		t.Fatal("normalizeTokenRewardConfig() expected duplicate threshold error")
	}
}

func TestResolveCurrentCycleConfigAutoSyncsEnabledCycle(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("timezone.Init() error = %v", err)
	}
	repo := newMemoryTokenRewardSettingRepo()
	service := NewTokenRewardService(nil, repo, nil)
	ctx := context.Background()
	now := time.Date(2026, 7, 3, 15, 30, 0, 0, timezone.Location())

	initial := TokenRewardConfig{
		Enabled: true,
		Cycle:   TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "old", Tokens: 5, TokenUnit: TokenRewardTokenUnitRaw, RewardBalance: 1},
		},
	}
	cfg, cycle, err := service.resolveCurrentCycleConfig(ctx, initial, now)
	if err != nil {
		t.Fatalf("resolveCurrentCycleConfig() error = %v", err)
	}
	if cycle.Type != TokenRewardCycleWeekly || cfg.Tiers[0].ID != "old" {
		t.Fatalf("initial snapshot mismatch cfg=%+v cycle=%+v", cfg, cycle)
	}

	changed := TokenRewardConfig{
		Enabled: true,
		Cycle:   TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "new", Tokens: 1, TokenUnit: TokenRewardTokenUnitM, RewardBalance: 10},
		},
	}
	cfg, cycle, err = service.resolveCurrentCycleConfig(ctx, changed, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("resolveCurrentCycleConfig() after change error = %v", err)
	}
	if cycle.Type != TokenRewardCycleWeekly || cfg.Tiers[0].ID != "new" || cfg.Tiers[0].Tokens != 1 || cfg.Tiers[0].TokenUnit != TokenRewardTokenUnitM {
		t.Fatalf("current cycle should sync changed tier, got cfg=%+v cycle=%+v", cfg, cycle)
	}
}

func TestUpdateConfigSyncsCurrentCycleSnapshot(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("timezone.Init() error = %v", err)
	}
	repo := newMemoryTokenRewardSettingRepo()
	service := NewTokenRewardService(nil, repo, nil)
	ctx := context.Background()

	initial := TokenRewardConfig{
		Enabled: true,
		Cycle:   TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "first", Tokens: 100, TokenUnit: TokenRewardTokenUnitRaw, RewardBalance: 1},
		},
	}
	if _, err := service.UpdateConfig(ctx, initial); err != nil {
		t.Fatalf("UpdateConfig(initial) error = %v", err)
	}
	changed := TokenRewardConfig{
		Enabled: true,
		Cycle:   TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "first", Tokens: 100, TokenUnit: TokenRewardTokenUnitRaw, RewardBalance: 1},
			{ID: "second", Tokens: 2, TokenUnit: TokenRewardTokenUnitM, RewardBalance: 2},
		},
	}
	if _, err := service.UpdateConfig(ctx, changed); err != nil {
		t.Fatalf("UpdateConfig(changed) error = %v", err)
	}
	cfg, cycle, err := service.resolveCurrentCycleConfig(ctx, changed, timezone.Now())
	if err != nil {
		t.Fatalf("resolveCurrentCycleConfig() error = %v", err)
	}
	if cycle.Type != TokenRewardCycleWeekly || len(cfg.Tiers) != 2 || cfg.Tiers[1].ID != "second" {
		t.Fatalf("current cycle snapshot was not synced, cfg=%+v cycle=%+v", cfg, cycle)
	}
}

func TestResolveCurrentCycleConfigKeepsLiveEnabledSwitch(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("timezone.Init() error = %v", err)
	}
	repo := newMemoryTokenRewardSettingRepo()
	service := NewTokenRewardService(nil, repo, nil)
	ctx := context.Background()
	now := time.Date(2026, 7, 3, 15, 30, 0, 0, timezone.Location())

	enabled := TokenRewardConfig{
		Enabled: true,
		Cycle:   TokenRewardCycleWeekly,
		Tiers: []TokenRewardTier{
			{ID: "weekly", Tokens: 5, TokenUnit: TokenRewardTokenUnitRaw, RewardBalance: 1},
		},
	}
	if _, _, err := service.resolveCurrentCycleConfig(ctx, enabled, now); err != nil {
		t.Fatalf("resolveCurrentCycleConfig() error = %v", err)
	}
	disabled := enabled
	disabled.Enabled = false
	cfg, _, err := service.resolveCurrentCycleConfig(ctx, disabled, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("resolveCurrentCycleConfig() disabled error = %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("enabled switch should remain live, got cfg=%+v", cfg)
	}
	if cfg.Tiers[0].ID != "weekly" {
		t.Fatalf("disabled current cycle should keep frozen tiers, got %+v", cfg.Tiers)
	}
}

func TestCurrentTokenRewardCycleWeeklyStartsMonday(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("timezone.Init() error = %v", err)
	}
	now := time.Date(2026, 7, 3, 15, 30, 0, 0, timezone.Location())
	cycle := CurrentTokenRewardCycle(TokenRewardCycleWeekly, now)
	wantStart := time.Date(2026, 6, 29, 0, 0, 0, 0, timezone.Location())
	wantEnd := time.Date(2026, 7, 6, 0, 0, 0, 0, timezone.Location())
	if !cycle.Start.Equal(wantStart) || !cycle.End.Equal(wantEnd) {
		t.Fatalf("weekly cycle = [%s, %s), want [%s, %s)", cycle.Start, cycle.End, wantStart, wantEnd)
	}
}

func TestCurrentTokenRewardCycleMonthly(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("timezone.Init() error = %v", err)
	}
	now := time.Date(2026, 7, 3, 15, 30, 0, 0, timezone.Location())
	cycle := CurrentTokenRewardCycle(TokenRewardCycleMonthly, now)
	wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, timezone.Location())
	wantEnd := time.Date(2026, 8, 1, 0, 0, 0, 0, timezone.Location())
	if !cycle.Start.Equal(wantStart) || !cycle.End.Equal(wantEnd) {
		t.Fatalf("monthly cycle = [%s, %s), want [%s, %s)", cycle.Start, cycle.End, wantStart, wantEnd)
	}
}

type memoryTokenRewardSettingRepo struct {
	values map[string]string
}

func newMemoryTokenRewardSettingRepo() *memoryTokenRewardSettingRepo {
	return &memoryTokenRewardSettingRepo{values: make(map[string]string)}
}

func (r *memoryTokenRewardSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *memoryTokenRewardSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *memoryTokenRewardSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		return errors.New("nil settings")
	}
	r.values[key] = value
	return nil
}

func (r *memoryTokenRewardSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *memoryTokenRewardSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *memoryTokenRewardSettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *memoryTokenRewardSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}
