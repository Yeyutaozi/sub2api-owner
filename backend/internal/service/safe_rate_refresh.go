package service

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// accountGroupsForSafeRateLoader is implemented by accountRepository.GetGroups.
type accountGroupsForSafeRateLoader interface {
	GetGroups(ctx context.Context, accountID int64) ([]Group, error)
}

// LoadAccountGroupsForSafeRate returns groups used for safe-rate evaluation.
// Prefer already-hydrated account.Groups; otherwise load via GetGroups when available.
func LoadAccountGroupsForSafeRate(ctx context.Context, accountRepo any, account *Account) []*Group {
	if account == nil {
		return nil
	}
	out := make([]*Group, 0)
	seen := make(map[int64]struct{})
	for _, g := range account.Groups {
		if g == nil || g.ID <= 0 {
			continue
		}
		if _, ok := seen[g.ID]; ok {
			continue
		}
		seen[g.ID] = struct{}{}
		out = append(out, g)
	}
	if len(out) > 0 {
		return out
	}
	loader, ok := accountRepo.(accountGroupsForSafeRateLoader)
	if !ok {
		return out
	}
	groups, err := loader.GetGroups(ctx, account.ID)
	if err != nil {
		return out
	}
	for i := range groups {
		g := groups[i]
		if g.ID <= 0 {
			continue
		}
		if _, exists := seen[g.ID]; exists {
			continue
		}
		seen[g.ID] = struct{}{}
		cp := g
		out = append(out, &cp)
	}
	return out
}

// OverlayGroupForSafeRate replaces or appends group by ID so freshly updated
// sell rates are visible without waiting for a repo reload.
func OverlayGroupForSafeRate(groups []*Group, group *Group) []*Group {
	if group == nil || group.ID <= 0 {
		return groups
	}
	out := make([]*Group, 0, len(groups)+1)
	replaced := false
	for _, g := range groups {
		if g == nil {
			continue
		}
		if g.ID == group.ID {
			cp := *group
			out = append(out, &cp)
			replaced = true
			continue
		}
		out = append(out, g)
	}
	if !replaced {
		cp := *group
		out = append(out, &cp)
	}
	return out
}

// PersistAccountSafeRateStatus builds and stores admin-visible safe_rate_status.
func PersistAccountSafeRateStatus(
	ctx context.Context,
	accountRepo interface {
		UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	},
	account *Account,
	groups []*Group,
	now time.Time,
) error {
	if accountRepo == nil || account == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	status := BuildSafeRateStatus(account, groups, now)
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[AccountExtraSafeRateStatus] = status
	return accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		AccountExtraSafeRateStatus: status,
	})
}

// RefreshAccountSafeRateStatus loads bound groups and persists safe_rate_status.
func RefreshAccountSafeRateStatus(
	ctx context.Context,
	accountRepo interface {
		UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	},
	account *Account,
	now time.Time,
) error {
	if account == nil {
		return nil
	}
	groups := LoadAccountGroupsForSafeRate(ctx, accountRepo, account)
	return PersistAccountSafeRateStatus(ctx, accountRepo, account, groups, now)
}

// RefreshGroupBoundAccountsSafeRateStatus recomputes safe_rate_status for every
// account bound to the group. Used after sell-rate (rate_multiplier) changes.
// Failures are best-effort: scheduling still uses live baselines at select time.
func RefreshGroupBoundAccountsSafeRateStatus(
	ctx context.Context,
	groupRepo interface {
		GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error)
	},
	accountRepo interface {
		GetByIDs(ctx context.Context, ids []int64) ([]*Account, error)
		UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	},
	group *Group,
	now time.Time,
) {
	if group == nil || group.ID <= 0 || groupRepo == nil || accountRepo == nil {
		return
	}
	ids, err := groupRepo.GetAccountIDsByGroupIDs(ctx, []int64{group.ID})
	if err != nil || len(ids) == 0 {
		return
	}
	accounts, err := accountRepo.GetByIDs(ctx, ids)
	if err != nil || len(accounts) == 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		groups := LoadAccountGroupsForSafeRate(ctx, accountRepo, account)
		groups = OverlayGroupForSafeRate(groups, group)
		_ = PersistAccountSafeRateStatus(ctx, accountRepo, account, groups, now)
	}
}

// normalizeProbeRateMultiplier validates a raw upstream rate value.
func normalizeProbeRateMultiplier(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	return value, true
}

// buildNormalizedRateProbeData builds a billing-like map so ResolveAccountUpstreamCostRate
// can reuse the existing probe path for non-Sub2API providers.
func buildNormalizedRateProbeData(provider string, rate float64, now time.Time, extras map[string]any) map[string]any {
	data := map[string]any{
		"object":                    "upstream.rate_probe",
		"schema_version":            1,
		"billing_scope":             "token",
		"provider":                  provider,
		"group_rate_multiplier":     rate,
		"resolved_rate_multiplier":  rate,
		"peak_rate_enabled":         false,
		"effective_rate_multiplier": rate,
		"observed_at":               now.UTC().Format(time.RFC3339Nano),
	}
	for k, v := range extras {
		if strings.TrimSpace(k) == "" || v == nil {
			continue
		}
		data[k] = v
	}
	return data
}

// extractRateMultiplierFromMap searches common rate field names.
func extractRateMultiplierFromMap(data map[string]any) (float64, bool) {
	if data == nil {
		return 0, false
	}
	keys := []string{
		"effective_rate_multiplier",
		"resolved_rate_multiplier",
		"group_rate_multiplier",
		"group_ratio",
		"group_rate",
		"rate_multiplier",
		"ratio",
	}
	if rate, ok := resolveAccountExtraNumber(data, keys...); ok {
		return normalizeProbeRateMultiplier(rate)
	}
	// Nested data / group objects used by NewAPI-style envelopes.
	for _, nestKey := range []string{"data", "group", "token", "usage"} {
		nested, ok := data[nestKey].(map[string]any)
		if !ok || nested == nil {
			continue
		}
		if rate, ok := extractRateMultiplierFromMap(nested); ok {
			return rate, true
		}
	}
	return 0, false
}

// parseNewAPIRateProbeResponse parses official NewAPI / forks of token usage and
// optional rate fields. Official NewAPI only returns quota (token_usage) without
// group ratio — that is detected but not "found".
//
// Returns:
//   - data: normalized probe data when a rate is found
//   - detected: response looks like NewAPI / NewAPI-compatible
//   - found: a usable rate multiplier was extracted
func parseNewAPIRateProbeResponse(body []byte, now time.Time) (data map[string]any, detected bool, found bool) {
	if len(body) == 0 {
		return nil, false, false
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, false, false
	}

	detected = isNewAPIStyleRateProbePayload(root)
	if rate, ok := extractRateMultiplierFromMap(root); ok {
		return buildNormalizedRateProbeData("newapi", rate, now, map[string]any{
			"source": "newapi_usage",
		}), true, true
	}
	return nil, detected, false
}

func isNewAPIStyleRateProbePayload(root map[string]any) bool {
	if root == nil {
		return false
	}
	if obj, _ := root["object"].(string); strings.EqualFold(strings.TrimSpace(obj), "token_usage") {
		return true
	}
	// NewAPI envelope: {"code":true|1|0,"data":{...}}
	if data, ok := root["data"].(map[string]any); ok && data != nil {
		switch code := root["code"].(type) {
		case bool:
			if code {
				return true
			}
		case float64:
			// 0/1 style success codes
			if code == 0 || code == 1 {
				return true
			}
		case int:
			if code == 0 || code == 1 {
				return true
			}
		case json.Number:
			if n, err := code.Int64(); err == nil && (n == 0 || n == 1) {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(code)) {
			case "true", "0", "1", "success", "ok":
				return true
			}
		}
		if success, ok := root["success"].(bool); ok && success {
			return true
		}
		if obj, _ := data["object"].(string); strings.EqualFold(strings.TrimSpace(obj), "token_usage") {
			return true
		}
	}
	// Some forks expose ratio keys at the top level without token_usage object.
	if _, ok := extractRateMultiplierFromMap(root); ok {
		// Only treat as NewAPI-like when at least one NewAPI-ish key exists.
		for _, key := range []string{"total_granted", "total_used", "total_available", "unlimited_quota", "model_limits"} {
			if _, exists := root[key]; exists {
				return true
			}
		}
	}
	return false
}


// parseNewAPIUserSelfGroup extracts the user's default group from /api/user/self.
func parseNewAPIUserSelfGroup(body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return "", false
	}
	// Envelope: {code, data:{group:...}} or flat {group:...}
	candidates := []map[string]any{root}
	if data, ok := root["data"].(map[string]any); ok && data != nil {
		candidates = append(candidates, data)
	}
	for _, m := range candidates {
		for _, key := range []string{"group", "Group", "user_group", "group_name"} {
			if g, ok := m[key].(string); ok {
				g = strings.TrimSpace(g)
				if g != "" {
					return g, true
				}
			}
		}
	}
	return "", false
}

// parseNewAPIUserGroupsRate parses /api/user/self/groups into a rate multiplier.
// preferred selects a named group when present; otherwise tries "default" then first numeric ratio.
func parseNewAPIUserGroupsRate(body []byte, preferred string) (rate float64, groupName string, ok bool) {
	if len(body) == 0 {
		return 0, "", false
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return 0, "", false
	}
	groups := extractNewAPIGroupsMap(root)
	if len(groups) == 0 {
		return 0, "", false
	}
	preferred = strings.TrimSpace(preferred)
	order := make([]string, 0, len(groups)+2)
	if preferred != "" {
		order = append(order, preferred)
	}
	order = append(order, "default")
	// Stable fallback: sorted keys so probes are deterministic.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == preferred || k == "default" {
			continue
		}
		order = append(order, k)
	}
	seen := map[string]struct{}{}
	for _, name := range order {
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		raw, exists := groups[name]
		if !exists {
			continue
		}
		if r, found := extractRatioFromGroupEntry(raw); found {
			return r, name, true
		}
	}
	return 0, "", false
}

func extractNewAPIGroupsMap(root map[string]any) map[string]any {
	if root == nil {
		return nil
	}
	if obj, _ := root["object"].(string); strings.EqualFold(strings.TrimSpace(obj), "token_usage") {
		return nil
	}
	// Common envelope: {code:true, data:{groupName:{ratio:1}}}
	if data, ok := root["data"].(map[string]any); ok && data != nil {
		// data itself may be the groups map
		if looksLikeGroupsMap(data) {
			return data
		}
		if nested, ok := data["groups"].(map[string]any); ok && nested != nil {
			return nested
		}
	}
	// Array envelope: {data:[{name:default,ratio:1}, ...]} or {groups:[...]}
	if arr, ok := root["data"].([]any); ok {
		if m := groupsMapFromSlice(arr); len(m) > 0 {
			return m
		}
	}
	if groups, ok := root["groups"].(map[string]any); ok && groups != nil {
		return groups
	}
	if arr, ok := root["groups"].([]any); ok {
		if m := groupsMapFromSlice(arr); len(m) > 0 {
			return m
		}
	}
	if looksLikeGroupsMap(root) {
		return root
	}
	return nil
}

// groupsMapFromSlice normalizes NewAPI / fork group list payloads into name->entry map.
func groupsMapFromSlice(items []any) map[string]any {
	out := make(map[string]any)
	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			name := ""
			for _, key := range []string{"name", "group", "group_name", "Group", "id"} {
				if s, ok := v[key].(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						name = s
						break
					}
				}
			}
			if name == "" {
				// fall back to nested name fields
				if nested, ok := v["group"].(map[string]any); ok {
					if s, ok := nested["name"].(string); ok {
						name = strings.TrimSpace(s)
					}
				}
			}
			if name == "" {
				continue
			}
			out[name] = v
		}
	}
	return out
}

func looksLikeGroupsMap(m map[string]any) bool {
	if m == nil || len(m) == 0 {
		return false
	}
	// Never treat official NewAPI token_usage quota objects as group maps.
	if obj, _ := m["object"].(string); strings.EqualFold(strings.TrimSpace(obj), "token_usage") {
		return false
	}
	quotaKeys := 0
	for _, key := range []string{"total_granted", "total_used", "total_available", "unlimited_quota", "model_limits"} {
		if _, ok := m[key]; ok {
			quotaKeys++
		}
	}
	if quotaKeys >= 2 {
		return false
	}

	// Prefer entries that look like group -> {ratio: ...}.
	groupish := 0
	for k, v := range m {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "object" || key == "code" || key == "success" || key == "message" || key == "msg" {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			if _, ok := extractRatioFromGroupEntry(val); ok {
				groupish++
			}
		case float64, int, int64, json.Number:
			// Bare number group ratios only when key is not a known quota field.
			if key == "total_granted" || key == "total_used" || key == "total_available" ||
				key == "unlimited_quota" || key == "model_limits" || key == "id" || key == "user_id" {
				continue
			}
			if r, ok := extractRatioFromGroupEntry(val); ok && r >= 0 && r <= 100 {
				groupish++
			}
		}
	}
	return groupish > 0
}

func extractRatioFromGroupEntry(raw any) (float64, bool) {
	switch v := raw.(type) {
	case map[string]any:
		if r, ok := extractRateMultiplierFromMap(v); ok {
			return r, true
		}
		// NewAPI docs use "ratio"
		if r, ok := resolveAccountExtraNumber(v, "ratio", "group_ratio", "Rate", "Ratio"); ok {
			return normalizeProbeRateMultiplier(r)
		}
	case float64:
		return normalizeProbeRateMultiplier(v)
	case int:
		return normalizeProbeRateMultiplier(float64(v))
	case int64:
		return normalizeProbeRateMultiplier(float64(v))
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return normalizeProbeRateMultiplier(f)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return normalizeProbeRateMultiplier(f)
	}
	return 0, false
}
