package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cross-instance account runtime TTFT/error EWMA.
// Sticky bindings already live in Redis; runtime stats must too, otherwise multi-instance
// deployments never see peer TTFT and sticky escape never fires on other pods.

const (
	accountRuntimeRedisKeyPrefix = "sub2api:account_runtime:v1:"
	accountRuntimeRedisTTL       = 6 * time.Hour

	// Asymmetric TTFT EWMA: rise fast on slow samples so sticky escape reacts within 1-2 hits.
	accountRuntimeTTFTAlphaUp   = 0.55
	accountRuntimeTTFTAlphaDown = 0.18
	accountRuntimeErrorAlpha    = 0.25
)

var accountRuntimeRedis atomic.Pointer[redis.Client]

// Lua: atomic EWMA update for error rate + TTFT + last sample.
// KEYS[1] = runtime key
// ARGV: success(0/1), ttft_ms(0 if none), alpha_up, alpha_down, alpha_err, ttl_sec
var accountRuntimeReportScript = redis.NewScript(`
local key = KEYS[1]
local success = tonumber(ARGV[1]) or 0
local ttftSample = tonumber(ARGV[2]) or 0
local alphaUp = tonumber(ARGV[3]) or 0.55
local alphaDown = tonumber(ARGV[4]) or 0.18
local alphaErr = tonumber(ARGV[5]) or 0.25
local ttl = tonumber(ARGV[6]) or 21600

local err = tonumber(redis.call('HGET', key, 'e') or '0') or 0
local has = redis.call('HGET', key, 'h')
local ttftRaw = redis.call('HGET', key, 't')
local ttft = tonumber(ttftRaw)

local errSample = 1.0
if success == 1 then
  errSample = 0.0
end
err = alphaErr * errSample + (1.0 - alphaErr) * err

local last = tonumber(redis.call('HGET', key, 'l') or '0') or 0
if ttftSample > 0 then
  last = ttftSample
  if has ~= '1' or ttft == nil then
    ttft = ttftSample
  else
    local alpha = alphaDown
    if ttftSample > ttft then
      alpha = alphaUp
    end
    ttft = alpha * ttftSample + (1.0 - alpha) * ttft
  end
  redis.call('HSET', key, 'h', '1')
  redis.call('HSET', key, 't', string.format('%.6f', ttft))
  redis.call('HSET', key, 'l', string.format('%.6f', last))
end

redis.call('HSET', key, 'e', string.format('%.6f', err))
redis.call('HSET', key, 'u', tostring(redis.call('TIME')[1]))
redis.call('EXPIRE', key, ttl)

local hasOut = '0'
if redis.call('HGET', key, 'h') == '1' then
  hasOut = '1'
end
local ttftOut = redis.call('HGET', key, 't') or '0'
local lastOut = redis.call('HGET', key, 'l') or '0'
return {string.format('%.6f', err), ttftOut, hasOut, lastOut}
`)

// SetAccountRuntimeRedis enables multi-instance shared TTFT/error EWMA for sticky escape.
// Safe to call with nil to disable.
func SetAccountRuntimeRedis(client *redis.Client) {
	accountRuntimeRedis.Store(client)
}

func accountRuntimeRedisClient() *redis.Client {
	return accountRuntimeRedis.Load()
}

func accountRuntimeRedisKey(accountID int64) string {
	return accountRuntimeRedisKeyPrefix + strconv.FormatInt(accountID, 10)
}

type accountRuntimeSnapshot struct {
	errorRate float64
	ttftMs    float64
	lastTTFT  float64
	hasTTFT   bool
}

func persistAccountRuntimeToRedis(accountID int64, success bool, firstTokenMs *int) accountRuntimeSnapshot {
	rdb := accountRuntimeRedisClient()
	if rdb == nil || accountID <= 0 {
		return accountRuntimeSnapshot{}
	}
	ttftSample := 0
	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttftSample = *firstTokenMs
	}
	successFlag := 0
	if success {
		successFlag = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	res, err := accountRuntimeReportScript.Run(ctx, rdb, []string{accountRuntimeRedisKey(accountID)},
		successFlag,
		ttftSample,
		accountRuntimeTTFTAlphaUp,
		accountRuntimeTTFTAlphaDown,
		accountRuntimeErrorAlpha,
		int(accountRuntimeRedisTTL.Seconds()),
	).Result()
	if err != nil {
		return accountRuntimeSnapshot{}
	}
	return parseAccountRuntimeScriptResult(res)
}

func loadAccountRuntimeFromRedis(accountID int64) (accountRuntimeSnapshot, bool) {
	rdb := accountRuntimeRedisClient()
	if rdb == nil || accountID <= 0 {
		return accountRuntimeSnapshot{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	vals, err := rdb.HGetAll(ctx, accountRuntimeRedisKey(accountID)).Result()
	if err != nil || len(vals) == 0 {
		return accountRuntimeSnapshot{}, false
	}
	snap := accountRuntimeSnapshot{}
	if e, ok := vals["e"]; ok {
		if v, err := strconv.ParseFloat(e, 64); err == nil {
			snap.errorRate = clamp01(v)
		}
	}
	if vals["h"] == "1" {
		if t, err := strconv.ParseFloat(vals["t"], 64); err == nil && t > 0 && !math.IsNaN(t) {
			snap.ttftMs = t
			snap.hasTTFT = true
		}
	}
	if l, ok := vals["l"]; ok {
		if v, err := strconv.ParseFloat(l, 64); err == nil && v > 0 {
			snap.lastTTFT = v
		}
	}
	return snap, snap.hasTTFT || snap.errorRate > 0
}

func parseAccountRuntimeScriptResult(res any) accountRuntimeSnapshot {
	arr, ok := res.([]any)
	if !ok || len(arr) < 3 {
		return accountRuntimeSnapshot{}
	}
	snap := accountRuntimeSnapshot{}
	if s, ok := arr[0].(string); ok {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			snap.errorRate = clamp01(v)
		}
	}
	if s, ok := arr[2].(string); ok && s == "1" {
		snap.hasTTFT = true
		if t, ok := arr[1].(string); ok {
			if v, err := strconv.ParseFloat(t, 64); err == nil && v > 0 {
				snap.ttftMs = v
			}
		}
	}
	if len(arr) >= 4 {
		if s, ok := arr[3].(string); ok {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				snap.lastTTFT = v
			}
		}
	}
	return snap
}

func hydrateLocalRuntimeFromSnapshot(accountID int64, snap accountRuntimeSnapshot) {
	if accountID <= 0 || (!snap.hasTTFT && snap.errorRate <= 0) {
		return
	}
	stats := ensureSharedAccountRuntimeStats()
	if stats == nil {
		return
	}
	stat := stats.loadOrCreate(accountID)
	stat.errorRateEWMABits.Store(math.Float64bits(clamp01(snap.errorRate)))
	if snap.hasTTFT && snap.ttftMs > 0 {
		stat.ttftEWMABits.Store(math.Float64bits(snap.ttftMs))
	}
	if snap.lastTTFT > 0 {
		stat.lastTTFTBits.Store(math.Float64bits(snap.lastTTFT))
	}
}

// effectiveTTFTForEscape prefers a sensitive view of latency:
// max(EWMA, last_sample) so a single 30s hang escapes even if EWMA was still warm.
func effectiveTTFTForEscape(ewma float64, last float64, hasTTFT bool) (float64, bool) {
	if !hasTTFT {
		return 0, false
	}
	v := ewma
	if last > v {
		v = last
	}
	if v <= 0 {
		return 0, false
	}
	return v, true
}

// debug helper kept for tests
func accountRuntimeKeyForTest(accountID int64) string {
	return accountRuntimeRedisKey(accountID)
}

func formatAccountRuntimeDebug(accountID int64) string {
	errRate, ttft, has := SnapshotOpenAIAccountRuntime(accountID)
	return fmt.Sprintf("id=%d err=%.3f ttft=%.0f has=%v", accountID, errRate, ttft, has)
}
