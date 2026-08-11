package service

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestShouldEscapeStickyUsesLastSampleNotOnlyEWMA(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	prev := sharedOpenAIAccountRuntimeStats.Swap(stats)
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prev) })
	id := int64(94001)
	peerID := int64(94002)
	// Warm up with moderate TTFT then one multi-second hang last sample.
	for i := 0; i < 8; i++ {
		v := 2000
		stats.report(id, true, &v)
	}
	spike := 20000
	stats.report(id, true, &spike)
	// Peer is much faster; last-sample-sensitive escape should fire only vs peer.
	fast := 900
	stats.report(peerID, true, &fast)

	cfg := gatewayStickyEscapeConfig()
	// Solo still must not unstick (cache-preserving).
	reason, _, ttft, escape := shouldEscapeStickyByRuntime(id, cfg, nil)
	require.False(t, escape, "solo spike must not unstick without faster peer, ttft=%v reason=%s", ttft, reason)

	reason, _, ttft, escape = shouldEscapeStickyByRuntime(id, cfg, []int64{peerID})
	require.True(t, escape, "last sample spike should escape vs faster peer, ttft=%v reason=%s", ttft, reason)
	require.Equal(t, "ttft_relative", reason)
	require.GreaterOrEqual(t, ttft, float64(spike))
}

func TestAccountRuntimeRedisSharedAcrossProcessLocalMiss(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	SetAccountRuntimeRedis(rdb)
	t.Cleanup(func() { SetAccountRuntimeRedis(nil) })

	// Isolate shared stats for this test, restore after.
	prevStats := sharedOpenAIAccountRuntimeStats.Swap(newOpenAIAccountRuntimeStats())
	t.Cleanup(func() { sharedOpenAIAccountRuntimeStats.Store(prevStats) })

	// Simulate instance A reporting into Redis + local.
	slowID := int64(95011)
	fastID := int64(95012)
	slow, fast := 18000, 900
	ReportAccountRuntimeResult(slowID, true, &slow)
	ReportAccountRuntimeResult(fastID, true, &fast)

	// Wipe process-local stats to simulate another instance reading only Redis.
	sharedOpenAIAccountRuntimeStats.Store(newOpenAIAccountRuntimeStats())

	// Instance B must still see Redis TTFT and escape sticky when a much faster peer exists.
	cfg := openAIStickyEscapeConfig{
		enabled:          true,
		ttftMs:           0,
		errorRate:        0.5,
		relativeRatio:    1.3,
		relativeMinDelta: 600,
	}
	reason, _, ttft, escape := shouldEscapeStickyByRuntime(slowID, cfg, []int64{fastID})
	require.True(t, escape, "redis-backed TTFT should escape on cold local cache, ttft=%v reason=%s", ttft, reason)
	require.Equal(t, "ttft_relative", reason)
	require.Greater(t, ttft, 1500.0)

	// Fast account should not escape.
	_, _, _, escapeFast := shouldEscapeStickyByRuntime(fastID, cfg, []int64{slowID})
	require.False(t, escapeFast)
}

func TestEffectiveTTFTForEscapePrefersLast(t *testing.T) {
	v, ok := effectiveTTFTForEscape(2000, 30000, true)
	require.True(t, ok)
	require.Equal(t, 30000.0, v)
	_, ok = effectiveTTFTForEscape(0, 0, false)
	require.False(t, ok)
}

func TestAccountRuntimeRedisTTLRefreshed(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	SetAccountRuntimeRedis(rdb)
	t.Cleanup(func() { SetAccountRuntimeRedis(nil) })

	id := int64(96001)
	tt := 2500
	ReportAccountRuntimeResult(id, true, &tt)
	require.True(t, mr.Exists(accountRuntimeRedisKey(id)))
	// miniredis TTL should be set
	ttl := mr.TTL(accountRuntimeRedisKey(id))
	require.Greater(t, ttl, time.Hour)
}
