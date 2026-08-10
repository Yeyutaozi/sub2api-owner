package service

import (
	"testing"
	"time"
)

func TestGroupSafeRateBaselineAlignsWithSellRate(t *testing.T) {
	g := &Group{RateMultiplier: 1.2}
	if got := GroupSafeRateBaseline(g); got != 1.2 {
		t.Fatalf("baseline=%v want 1.2", got)
	}
	if got := GroupSafeRateBaseline(nil); got != 1 {
		t.Fatalf("nil baseline=%v want 1", got)
	}
}

func TestExceedsGroupSafeRateStrictGreater(t *testing.T) {
	if ExceedsGroupSafeRate(1.0, true, 1.0) {
		t.Fatal("equal should not cut")
	}
	if !ExceedsGroupSafeRate(1.01, true, 1.0) {
		t.Fatal("greater should cut")
	}
	if ExceedsGroupSafeRate(0.9, true, 1.0) {
		t.Fatal("lower should not cut")
	}
	if ExceedsGroupSafeRate(2, false, 1.0) {
		t.Fatal("unknown should not cut")
	}
}

func TestResolveAccountUpstreamCostRateProbeAndManual(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	probeAccount := &Account{
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: map[string]any{
				"status": UpstreamBillingProbeStatusOK,
				"data": map[string]any{
					"billing_scope":             "token",
					"resolved_rate_multiplier":  1.25,
					"peak_rate_enabled":         false,
					"effective_rate_multiplier": 1.25,
				},
			},
		},
	}
	rate, source, ok := ResolveAccountUpstreamCostRate(probeAccount, now)
	if !ok || source != SafeRateSourceProbe || rate != 1.25 {
		t.Fatalf("probe resolve rate=%v source=%s ok=%v", rate, source, ok)
	}

	manualAccount := &Account{
		Extra: map[string]any{
			AccountExtraUpstreamDeclaredRate: 0.8,
		},
	}
	rate, source, ok = ResolveAccountUpstreamCostRate(manualAccount, now)
	if !ok || source != SafeRateSourceManual || rate != 0.8 {
		t.Fatalf("manual resolve rate=%v source=%s ok=%v", rate, source, ok)
	}

	unknown := &Account{}
	_, source, ok = ResolveAccountUpstreamCostRate(unknown, now)
	if ok || source != SafeRateSourceUnknown {
		t.Fatalf("unknown should fail closed, source=%s ok=%v", source, ok)
	}
}

func TestFilterAccountsByGroupSafeRate(t *testing.T) {
	now := time.Now().UTC()
	group := &Group{ID: 1, RateMultiplier: 1.0}
	cheap := Account{ID: 1, Extra: map[string]any{AccountExtraUpstreamDeclaredRate: 0.8}}
	equal := Account{ID: 2, Extra: map[string]any{AccountExtraUpstreamDeclaredRate: 1.0}}
	expensive := Account{ID: 3, Extra: map[string]any{AccountExtraUpstreamDeclaredRate: 1.2}}
	unknown := Account{ID: 4}

	out := FilterAccountsByGroupSafeRate([]Account{cheap, equal, expensive, unknown}, group, now)
	if len(out) != 3 {
		t.Fatalf("len=%d want 3", len(out))
	}
	for _, a := range out {
		if a.ID == 3 {
			t.Fatal("expensive account should be filtered")
		}
	}
}

func TestBuildSafeRateStatusOverGroups(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Extra: map[string]any{AccountExtraUpstreamDeclaredRate: 1.2}}
	groups := []*Group{
		{ID: 10, RateMultiplier: 1.0},
		{ID: 11, RateMultiplier: 1.5},
	}
	status := BuildSafeRateStatus(account, groups, now)
	if status.Status != SafeRateStatusOverSafe {
		t.Fatalf("status=%s want over_safe", status.Status)
	}
	if len(status.OverGroupIDs) != 1 || status.OverGroupIDs[0] != 10 {
		t.Fatalf("over groups=%v", status.OverGroupIDs)
	}
}

func TestGroupTTFTDisplayNeverEmpty(t *testing.T) {
	store := NewGroupTTFTDisplayStore()
	d := store.GetDisplay(99, PlatformOpenAI)
	if d.AvgFirstTokenMs <= 0 {
		t.Fatal("expected baseline display")
	}
	if d.Disclaimer != GroupTTFTDisclaimerZH {
		t.Fatalf("disclaimer=%q", d.Disclaimer)
	}
	if d.Source != "baseline" {
		t.Fatalf("source=%s", d.Source)
	}

	store.Report(99, 400)
	d2 := store.GetDisplay(99, PlatformOpenAI)
	if d2.AvgFirstTokenMs <= 0 {
		t.Fatal("expected positive display after report")
	}
}
