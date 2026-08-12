package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSoftExcludeStickyAccount(t *testing.T) {
	var nilMap map[int64]struct{}
	out := softExcludeStickyAccount(nilMap, 42)
	require.NotNil(t, out)
	_, ok := out[42]
	require.True(t, ok)

	out2 := softExcludeStickyAccount(out, 99)
	_, ok = out2[99]
	require.True(t, ok)
	_, ok = out2[42]
	require.True(t, ok)

	// invalid id is no-op
	same := softExcludeStickyAccount(out2, 0)
	require.Equal(t, len(out2), len(same))

	// Copy-on-write: soft-exclude must not poison handler failedAccountIDs used by failover.
	failed := map[int64]struct{}{100: {}}
	soft := softExcludeStickyAccount(failed, 200)
	_, ok = soft[100]
	require.True(t, ok)
	_, ok = soft[200]
	require.True(t, ok)
	_, ok = failed[200]
	require.False(t, ok, "caller failedAccountIDs must stay free of soft sticky-escape excludes")
	require.Equal(t, 1, len(failed))

	// Already present: may return same map reference (no need to copy)
	failed2 := map[int64]struct{}{200: {}}
	soft2 := softExcludeStickyAccount(failed2, 200)
	_, ok = soft2[200]
	require.True(t, ok)
}

func TestSoftExcludeDoesNotPoisonFailoverAfterPeer502(t *testing.T) {
	// Simulates: sticky escape soft-excludes slow (A), free-selects peer (B).
	// Peer B returns 502 and is hard-failed. Next selection must still allow A.
	failedAccountIDs := make(map[int64]struct{})
	slowID, fastID := int64(30), int64(31)

	// Escape reselection uses a local soft-excluded map.
	reselectExcluded := softExcludeStickyAccount(failedAccountIDs, slowID)
	_, ok := reselectExcluded[slowID]
	require.True(t, ok)
	// Handler map still empty after soft-exclude.
	require.Equal(t, 0, len(failedAccountIDs))

	// Peer forward fails -> hard exclude only the peer.
	failedAccountIDs[fastID] = struct{}{}

	// Next failover selection: only hard excludes apply; slow sticky is still eligible.
	_, hardSlow := failedAccountIDs[slowID]
	_, hardFast := failedAccountIDs[fastID]
	require.False(t, hardSlow, "slow sticky must remain eligible after peer 502")
	require.True(t, hardFast)
	require.Equal(t, 1, len(failedAccountIDs))
}

func TestApplyGatewayStickyEscapeSoftExcludesAndCarriesContextNote(t *testing.T) {
	// Isolate process-local runtime stats so other tests don't interfere.
	prev := sharedOpenAIAccountRuntimeStats.Swap(newOpenAIAccountRuntimeStats())
	t.Cleanup(func() {
		sharedOpenAIAccountRuntimeStats.Store(prev)
		SetAccountRuntimeRedis(nil)
	})
	SetAccountRuntimeRedis(nil)

	slowID := int64(91001)
	fastID := int64(91002)
	slowTTFT := 32000
	fastTTFT := 400
	ReportAccountRuntimeResult(slowID, true, &slowTTFT)
	ReportAccountRuntimeResult(fastID, true, &fastTTFT)

	svc := &GatewayService{}
	slow := &Account{ID: slowID, Name: "slow"}
	escaped, excluded := svc.applyGatewayStickyEscapeIfNeeded(
		context.Background(),
		nil,
		PlatformAnthropic,
		"claude-sonnet-4",
		"sess-hash-test",
		slow,
		[]int64{slowID, fastID},
		[]*Account{slow, &Account{ID: fastID, Name: "fast"}},
		"unit_test",
		nil,
	)
	require.True(t, escaped, "slow sticky must escape when peer is much faster / absolute hang")
	require.NotNil(t, excluded)
	_, ok := excluded[slowID]
	require.True(t, ok, "escaped account must be soft-excluded so LB cannot re-stick immediately")

	// Fast account must not escape
	fast := &Account{ID: fastID, Name: "fast"}
	escapedFast, excludedFast := svc.applyGatewayStickyEscapeIfNeeded(
		context.Background(),
		nil,
		PlatformAnthropic,
		"claude-sonnet-4",
		"sess-hash-test-2",
		fast,
		[]int64{slowID, fastID},
		nil,
		"unit_test",
		nil,
	)
	require.False(t, escapedFast)
	require.Nil(t, excludedFast)
}

func TestRemovePreviousResponseIDPreservesFullInputContext(t *testing.T) {
	// OpenAI sticky escape strips previous_response_id only; full input must remain so
	// conversation context is rebuilt on the new account instead of being dropped.
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"previous_response_id":"resp_abc123",
		"input":[
			{"role":"user","content":"hello history turn 1"},
			{"role":"assistant","content":"hi"},
			{"role":"user","content":"continue with tools"}
		],
		"stream":true
	}`)
	out := RemovePreviousResponseIDFromBody(body)
	require.NotContains(t, string(out), "previous_response_id")
	require.Contains(t, string(out), "hello history turn 1")
	require.Contains(t, string(out), "continue with tools")
	require.Contains(t, string(out), "gpt-5.6-sol")
}

func TestEffectiveTTFTUsesLastSampleForSensitiveEscape(t *testing.T) {
	// EWMA may still be warm while last sample is a multi-second hang.
	v, ok := effectiveTTFTForEscape(1200, 30000, true)
	require.True(t, ok)
	require.Equal(t, 30000.0, v)

	_, ok = effectiveTTFTForEscape(0, 0, false)
	require.False(t, ok)
}

