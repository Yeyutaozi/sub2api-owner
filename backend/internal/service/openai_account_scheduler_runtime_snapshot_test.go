package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIAccountSchedulerScoreSnapshotUsesRuntimeTTFT(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	registerOpenAIAccountRuntimeStats(stats)

	fast := 180
	slow := 2400
	stats.report(9101, true, &fast)
	stats.report(9102, true, &slow)

	rate1 := 1.0
	rate2 := 1.5
	accounts := []*Account{
		{ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 1, RateMultiplier: &rate1},
		{ID: 9102, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 1, RateMultiplier: &rate2},
	}
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:  0.2,
		Load:      0.1,
		Queue:     0.1,
		ErrorRate: 0.1,
		TTFT:      0.5,
	}

	scores := buildOpenAIAccountSchedulerScoreSnapshot(accounts, nil, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.Contains(t, scores, int64(9101))
	require.Contains(t, scores, int64(9102))
	require.True(t, scores[9101].HasTTFT)
	require.True(t, scores[9102].HasTTFT)
	require.InDelta(t, 180, scores[9101].AvgFirstTokenMs, 1)
	require.InDelta(t, 2400, scores[9102].AvgFirstTokenMs, 1)
	require.Equal(t, 1.0, scores[9101].RateMultiplier)
	require.Equal(t, 1.5, scores[9102].RateMultiplier)
	// Faster first-token should score higher when TTFT weight dominates.
	require.Greater(t, scores[9101].BaseScore, scores[9102].BaseScore)
}

func TestSnapshotOpenAIAccountRuntimeEmpty(t *testing.T) {
	registerOpenAIAccountRuntimeStats(newOpenAIAccountRuntimeStats())
	errRate, ttft, has := SnapshotOpenAIAccountRuntime(42)
	require.Equal(t, 0.0, errRate)
	require.Equal(t, 0.0, ttft)
	require.False(t, has)
}
