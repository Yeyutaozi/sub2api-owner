package service

import "testing"

func TestShouldEscapeStickyByRuntime_AbsoluteAndRelative(t *testing.T) {
	// isolate shared stats via register return value
	stats := registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())

	slowID, fastID := int64(91001), int64(91002)
	slow := 12000
	fast := 2000
	stats.report(slowID, true, &slow)
	stats.report(fastID, true, &fast)

	cfg := gatewayStickyEscapeConfig()
	if cfg.ttftMs != 5000 {
		t.Fatalf("gateway sticky absolute threshold = %v, want 5000", cfg.ttftMs)
	}
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

func TestShouldEscapeStickyByRuntime_RelativeUnderAbsolute(t *testing.T) {
	stats := registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())
	slowID, fastID := int64(93001), int64(93002)
	// Both under absolute 5s, but sticky is >1.15x peer with delta >300ms.
	slow, fast := 4000, 2000
	stats.report(slowID, true, &slow)
	stats.report(fastID, true, &fast)

	cfg := gatewayStickyEscapeConfig()
	reason, _, _, escape := shouldEscapeStickyByRuntime(slowID, cfg, []int64{fastID})
	if !escape {
		t.Fatalf("expected relative escape under absolute threshold, reason=%s", reason)
	}
	if reason != "ttft_relative" {
		t.Fatalf("want ttft_relative, got %q", reason)
	}
}

func TestAccountTTFTForSortPrefersFaster(t *testing.T) {
	stats := registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())
	a, b := int64(92001), int64(92002)
	fast, slow := 800, 7000
	stats.report(a, true, &fast)
	stats.report(b, true, &slow)
	if accountTTFTForSort(a) >= accountTTFTForSort(b) {
		t.Fatalf("expected a faster than b: a=%v b=%v", accountTTFTForSort(a), accountTTFTForSort(b))
	}
}
