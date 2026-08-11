package service

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// SafeRateOverReasonPrefix marks accounts filtered/cut because upstream cost rate
	// exceeds the group's independent safe-rate ceiling.
	SafeRateOverReasonPrefix = "RATE_OVER_SAFE"

	// AccountExtraUpstreamDeclaredRate is the manual fallback when remote probe is unsupported.
	// Account-level only (not per model).
	AccountExtraUpstreamDeclaredRate = "upstream_declared_rate_multiplier"

	// NewAPI management Access Token path (official sk /api/usage/token has quota only).
	// Requires both token and user id per NewAPI docs (Authorization + New-Api-User).
	AccountExtraNewAPIAccessToken = "newapi_access_token"
	AccountExtraNewAPIUserID      = "newapi_user_id"
	// Optional preferred group name when the token user belongs to multiple groups.
	AccountExtraNewAPIGroup       = "newapi_group"

	// AccountExtraSafeRateStatus stores admin-visible safe-rate evaluation.
	AccountExtraSafeRateStatus = "safe_rate_status"

	// SafeRateSource* identify where the upstream rate came from.
	SafeRateSourceProbe   = "probe"
	SafeRateSourceManual  = "manual"
	SafeRateSourceUnknown = "unknown"
)

// SafeRateStatus is persisted under accounts.extra for admin visibility.
// Scheduling filters by comparing live values; this snapshot is diagnostic.
type SafeRateStatus struct {
	Status           string    `json:"status"` // ok | over_safe | unknown
	UpstreamRate     *float64  `json:"upstream_rate,omitempty"`
	Source           string    `json:"source,omitempty"`
	SafeRateBaseline *float64  `json:"safe_rate_baseline,omitempty"` // min group safe_rate_multiplier among bound groups when known
	OverGroupIDs     []int64   `json:"over_group_ids,omitempty"`
	CheckedAt        time.Time `json:"checked_at"`
	Message          string    `json:"message,omitempty"`
}

const (
	SafeRateStatusOK       = "ok"
	SafeRateStatusOverSafe = "over_safe"
	SafeRateStatusUnknown  = "unknown"
)

// GroupSafeRateBaseline returns the independent safe-rate ceiling for a group.
// Product rule: use groups.safe_rate_multiplier only (not sell rate_multiplier).
// Invalid/zero values fall back to 1.0 so misconfiguration never disables the guard silently.
func GroupSafeRateBaseline(group *Group) float64 {
	if group == nil {
		return 1
	}
	v := group.SafeRateMultiplier
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	return v
}

// ResolveAccountUpstreamCostRate returns the best-known upstream cost multiplier
// for an account. One account probe covers all models on that key.
//
// Priority:
//  1. upstream billing probe effective/resolved rate (token scope)
//  2. manual extra upstream_declared_rate_multiplier
//  3. unknown (ok=false) — never treat as over-safe
func ResolveAccountUpstreamCostRate(account *Account, now time.Time) (rate float64, source string, ok bool) {
	if account == nil {
		return 0, SafeRateSourceUnknown, false
	}
	if snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra); snapshot != nil &&
		snapshot.Status == UpstreamBillingProbeStatusOK && snapshot.Data != nil {
		if r, found := upstreamBillingRateAt(snapshot.Data, now); found {
			return r, SafeRateSourceProbe, true
		}
		if r, found := resolveAccountExtraNumber(snapshot.Data, "effective_rate_multiplier"); found && r >= 0 {
			return r, SafeRateSourceProbe, true
		}
	}
	if account.Extra != nil {
		if r, found := resolveAccountExtraNumber(account.Extra, AccountExtraUpstreamDeclaredRate); found && r >= 0 {
			return r, SafeRateSourceManual, true
		}
	}
	return 0, SafeRateSourceUnknown, false
}

// ExceedsGroupSafeRate reports whether upstream cost is strictly above the group safe-rate ceiling.
// Equal rates are allowed (break-even). Unknown rates never exceed.
func ExceedsGroupSafeRate(upstreamRate float64, hasUpstream bool, safeBaseline float64) bool {
	if !hasUpstream {
		return false
	}
	if math.IsNaN(upstreamRate) || math.IsInf(upstreamRate, 0) || upstreamRate < 0 {
		return false
	}
	if math.IsNaN(safeBaseline) || math.IsInf(safeBaseline, 0) || safeBaseline < 0 {
		return false
	}
	return upstreamRate > safeBaseline
}

// IsAccountOverGroupSafeRate is the schedule-time check for a single group.
func IsAccountOverGroupSafeRate(account *Account, group *Group, now time.Time) bool {
	if account == nil || group == nil {
		return false
	}
	rate, _, ok := ResolveAccountUpstreamCostRate(account, now)
	if !ok {
		return false
	}
	return ExceedsGroupSafeRate(rate, true, GroupSafeRateBaseline(group))
}

// FilterAccountsByGroupSafeRate drops accounts whose known upstream cost exceeds
// the group's safe-rate ceiling. Unknown rates are kept.
func FilterAccountsByGroupSafeRate(accounts []Account, group *Group, now time.Time) []Account {
	if group == nil || len(accounts) == 0 {
		return accounts
	}
	baseline := GroupSafeRateBaseline(group)
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		rate, _, ok := ResolveAccountUpstreamCostRate(&accounts[i], now)
		if ExceedsGroupSafeRate(rate, ok, baseline) {
			continue
		}
		out = append(out, accounts[i])
	}
	return out
}

// FilterAccountPointersByGroupSafeRate is the pointer variant used by schedulers.
func FilterAccountPointersByGroupSafeRate(accounts []*Account, group *Group, now time.Time) []*Account {
	if group == nil || len(accounts) == 0 {
		return accounts
	}
	baseline := GroupSafeRateBaseline(group)
	out := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		rate, _, ok := ResolveAccountUpstreamCostRate(account, now)
		if ExceedsGroupSafeRate(rate, ok, baseline) {
			continue
		}
		out = append(out, account)
	}
	return out
}

// BuildSafeRateStatus evaluates an account against provided group baselines.
// groups may be empty; then status is ok/unknown based only on whether a rate exists.
func BuildSafeRateStatus(account *Account, groups []*Group, now time.Time) SafeRateStatus {
	status := SafeRateStatus{
		Status:    SafeRateStatusUnknown,
		Source:    SafeRateSourceUnknown,
		CheckedAt: now.UTC(),
	}
	rate, source, ok := ResolveAccountUpstreamCostRate(account, now)
	status.Source = source
	if !ok {
		status.Message = "upstream rate unknown; not auto-cut"
		return status
	}
	status.UpstreamRate = &rate
	status.Source = source

	if len(groups) == 0 {
		status.Status = SafeRateStatusOK
		status.Message = "upstream rate known; no bound groups to compare"
		return status
	}

	var minBaseline float64
	hasBaseline := false
	overIDs := make([]int64, 0)
	for _, g := range groups {
		if g == nil {
			continue
		}
		baseline := GroupSafeRateBaseline(g)
		if !hasBaseline || baseline < minBaseline {
			minBaseline = baseline
			hasBaseline = true
		}
		if ExceedsGroupSafeRate(rate, true, baseline) {
			overIDs = append(overIDs, g.ID)
		}
	}
	if hasBaseline {
		status.SafeRateBaseline = &minBaseline
	}
	if len(overIDs) > 0 {
		status.Status = SafeRateStatusOverSafe
		status.OverGroupIDs = overIDs
		status.Message = fmt.Sprintf(
			"%s: upstream=%.4f exceeds safe rate on %d group(s)",
			SafeRateOverReasonPrefix, rate, len(overIDs),
		)
		return status
	}
	status.Status = SafeRateStatusOK
	status.Message = "upstream rate within group safe-rate ceilings"
	return status
}

// SafeRateFilterReason is used in scheduler filter stats.
func SafeRateFilterReason() string {
	return "rate_over_safe"
}

// IsSafeRateOverReason reports whether a temp-unschedulable reason is from this guard.
func IsSafeRateOverReason(reason string) bool {
	return strings.HasPrefix(strings.TrimSpace(reason), SafeRateOverReasonPrefix)
}

// FormatSafeRateOverReason builds a stable temp-unschedulable reason string.
func FormatSafeRateOverReason(upstreamRate, safeBaseline float64, groupID int64) string {
	return fmt.Sprintf(
		"%s: upstream=%.4f safe=%.4f group_id=%d",
		SafeRateOverReasonPrefix, upstreamRate, safeBaseline, groupID,
	)
}
