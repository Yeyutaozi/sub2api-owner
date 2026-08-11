package service

import "testing"

func TestCompareAccountsBySignificantTTFT_PrefersMuchFasterPeer(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	fastID, slowID := int64(96001), int64(96002)
	fast, slow := 1500, 22000
	stats.report(fastID, true, &fast)
	stats.report(slowID, true, &slow)

	if got := compareAccountsBySignificantTTFT(fastID, slowID); got != -1 {
		t.Fatalf("fast vs slow prefer fast, got %d", got)
	}
	if got := compareAccountsBySignificantTTFT(slowID, fastID); got != 1 {
		t.Fatalf("slow vs fast prefer fast, got %d", got)
	}
}

func TestCompareAccountsBySignificantTTFT_AbsoluteGapExtremeHanger(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	// 16s vs 20s fails 1.3x relative (20 < 16*1.3=20.8) but abs gap 4s > 1.5s
	betterID, worseID := int64(96011), int64(96012)
	better, worse := 16000, 20000
	stats.report(betterID, true, &better)
	stats.report(worseID, true, &worse)

	if got := compareAccountsBySignificantTTFT(betterID, worseID); got != -1 {
		t.Fatalf("expected abs-gap prefer better, got %d", got)
	}
}

func TestCompareAccountsBySignificantTTFT_NearPeersKeepOrder(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	aID, bID := int64(96021), int64(96022)
	a, b := 1800, 2100 // close — must not reorder on TTFT alone
	stats.report(aID, true, &a)
	stats.report(bID, true, &b)

	if got := compareAccountsBySignificantTTFT(aID, bID); got != 0 {
		t.Fatalf("near peers must return 0, got %d", got)
	}
}

func TestCompareAccountsBySignificantTTFT_ExploreUnmeasuredOverSevereHanger(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	slowID, unmeasuredID := int64(96031), int64(96032)
	slow := 14500
	stats.report(slowID, true, &slow)

	if got := compareAccountsBySignificantTTFT(unmeasuredID, slowID); got != -1 {
		t.Fatalf("unmeasured should rank above severe hanger, got %d", got)
	}
	if got := compareAccountsBySignificantTTFT(slowID, unmeasuredID); got != 1 {
		t.Fatalf("severe hanger should rank below unmeasured, got %d", got)
	}
}

func TestCompareAccountsBySignificantTTFT_MeasuredHealthyPreferredOverUnmeasured(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	healthyID, unmeasuredID := int64(96041), int64(96042)
	healthy := 900
	stats.report(healthyID, true, &healthy)

	if got := compareAccountsBySignificantTTFT(healthyID, unmeasuredID); got != -1 {
		t.Fatalf("healthy measured should beat unmeasured, got %d", got)
	}
}

func TestStickyEscapeReasonByFasterPeer_AbsoluteGap(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	slowID, peerID := int64(96101), int64(96102)
	// ratio fails: 20000 < 16000*1.3=20800, but abs gap 4s should escape
	slow, peer := 20000, 16000
	stats.report(slowID, true, &slow)
	stats.report(peerID, true, &peer)

	cfg := gatewayStickyEscapeConfig()
	reason, _, _, escape := shouldEscapeStickyByRuntime(slowID, cfg, []int64{slowID, peerID})
	if !escape {
		t.Fatalf("expected abs-gap sticky escape for 20s vs 16s peer")
	}
	if reason != "ttft_relative" {
		t.Fatalf("want ttft_relative, got %q", reason)
	}
}
