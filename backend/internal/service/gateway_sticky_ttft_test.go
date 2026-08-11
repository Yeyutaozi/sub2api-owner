package service

import "testing"

func TestShouldEscapeStickyByRuntime_RequiresMuchFasterPeer(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	slowID, fastID := int64(91001), int64(91002)
	slow := 12000
	fast := 2000
	stats.report(slowID, true, &slow)
	stats.report(fastID, true, &fast)

	cfg := gatewayStickyEscapeConfig()
	if cfg.relativeRatio != 1.3 {
		t.Fatalf("gateway sticky relativeRatio = %v, want 1.3", cfg.relativeRatio)
	}
	// Alone / no peers: even multi-second TTFT must NOT unstick (preserve cache).
	reason, _, ttft, escape := shouldEscapeStickyByRuntime(slowID, cfg, nil)
	if escape {
		t.Fatalf("solo slow sticky must not escape without faster peer, ttft=%v reason=%s", ttft, reason)
	}
	// Peer list often includes self from listSchedulableAccounts — must still ignore self.
	reason, _, ttft, escape = shouldEscapeStickyByRuntime(slowID, cfg, []int64{slowID})
	if escape {
		t.Fatalf("self-only peer list must not escape, ttft=%v reason=%s", ttft, reason)
	}
	// With much faster peer: escape.
	reason, _, ttft, escape = shouldEscapeStickyByRuntime(slowID, cfg, []int64{slowID, fastID})
	if !escape {
		t.Fatalf("expected slow sticky to escape vs fast peer, ttft=%v reason=%s", ttft, reason)
	}
	if reason != "ttft_relative" {
		t.Fatalf("unexpected reason %q", reason)
	}

	reason, _, _, escape = shouldEscapeStickyByRuntime(fastID, cfg, []int64{fastID, slowID})
	if escape {
		t.Fatalf("fast account should not escape, reason=%s", reason)
	}
}

func TestShouldEscapeStickyByRuntime_RelativeUnderAbsolute(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })
	slowID, fastID := int64(93001), int64(93002)
	// Similar-ish peers: 4s vs 3.5s should NOT escape (not "much faster").
	closeSlow, closeFast := 4000, 3500
	stats.report(slowID, true, &closeSlow)
	stats.report(fastID, true, &closeFast)

	cfg := gatewayStickyEscapeConfig()
	reason, _, _, escape := shouldEscapeStickyByRuntime(slowID, cfg, []int64{slowID, fastID})
	if escape {
		t.Fatalf("near peers must keep sticky for cache, reason=%s", reason)
	}

	// Clearly faster peer under any absolute floor.
	stats2 := newOpenAIAccountRuntimeStats()
	sharedOpenAIAccountRuntimeStats.Store(stats2)
	slowID2, fastID2 := int64(93011), int64(93012)
	slow, fast := 4000, 1500
	stats2.report(slowID2, true, &slow)
	stats2.report(fastID2, true, &fast)
	reason, _, _, escape = shouldEscapeStickyByRuntime(slowID2, cfg, []int64{slowID2, fastID2})
	if !escape {
		t.Fatalf("expected relative escape when peer is much faster, reason=%s", reason)
	}
	if reason != "ttft_relative" {
		t.Fatalf("want ttft_relative, got %q", reason)
	}
}

func TestShouldEscapeStickyByRuntime_ExploreWhenPeersUnmeasured(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })

	stickyID, peerID := int64(94001), int64(94002)
	// Sticky monopoly at 14s+ (matches production complaint). Peer exists but never sampled.
	slow := 14540
	stats.report(stickyID, true, &slow)

	cfg := gatewayStickyEscapeConfig()
	// listSchedulableAccounts includes self + unmeasured peer.
	reason, _, ttft, escape := shouldEscapeStickyByRuntime(stickyID, cfg, []int64{stickyID, peerID})
	if !escape {
		t.Fatalf("expected explore escape when peers lack samples and sticky is severe, ttft=%v", ttft)
	}
	if reason != "ttft_explore" {
		t.Fatalf("want ttft_explore, got %q", reason)
	}

	// Mild TTFT should still stick to preserve cache even if peers unmeasured.
	mildID, mildPeer := int64(94011), int64(94012)
	mild := 2200
	stats.report(mildID, true, &mild)
	reason, _, _, escape = shouldEscapeStickyByRuntime(mildID, cfg, []int64{mildID, mildPeer})
	if escape {
		t.Fatalf("mild TTFT must not explore-escape, reason=%s", reason)
	}
}

func TestAccountTTFTForSortPrefersFaster(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })
	a, b := int64(92001), int64(92002)
	fast, slow := 800, 7000
	stats.report(a, true, &fast)
	stats.report(b, true, &slow)
	if accountTTFTForSort(a) >= accountTTFTForSort(b) {
		t.Fatalf("expected a faster than b: a=%v b=%v", accountTTFTForSort(a), accountTTFTForSort(b))
	}
}
