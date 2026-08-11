package service

import "testing"

func TestShouldEscapeStickyByRuntime_AbsoluteAndRelative(t *testing.T) {
	// isolate shared stats via register return value
	stats := registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())

	slowID, fastID := int64(91001), int64(91002)
	slow := 50000
	fast := 6000
	stats.report(slowID, true, &slow)
	stats.report(fastID, true, &fast)

	cfg := gatewayStickyEscapeConfig()
	reason, _, ttft, escape := shouldEscapeStickyByRuntime(slowID, cfg, []int64{fastID})
	if !escape {
		t.Fatalf("expected slow sticky to escape, ttft=%v reason=%s", ttft, reason)
	}
	if reason != "ttft" && reason != "ttft_relative" {
		t.Fatalf("unexpected reason %q", reason)
	}

	reason, _, _, escape = shouldEscapeStickyByRuntime(fastID, cfg, []int64{slowID})
	if escape {
		t.Fatalf("fast account should not escape, reason=%s", reason)
	}
}

func TestAccountTTFTForSortPrefersFaster(t *testing.T) {
	stats := registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())
	a, b := int64(92001), int64(92002)
	fast, slow := 1200, 9000
	stats.report(a, true, &fast)
	stats.report(b, true, &slow)
	if accountTTFTForSort(a) >= accountTTFTForSort(b) {
		t.Fatalf("expected a faster than b: a=%v b=%v", accountTTFTForSort(a), accountTTFTForSort(b))
	}
}
