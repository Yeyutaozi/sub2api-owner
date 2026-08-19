package handler

import (
	"sync"
	"time"
)

var seedanceRequestLimiter = struct {
	sync.Mutex
	last map[int64]time.Time
}{last: make(map[int64]time.Time)}

// Prevents tight client retry loops from consuming gateway/database capacity.
// The interval is deliberately short so normal users are not affected.
func allowSeedanceRequest(apiKeyID int64) bool {
	now := time.Now()
	seedanceRequestLimiter.Lock()
	defer seedanceRequestLimiter.Unlock()
	if previous, ok := seedanceRequestLimiter.last[apiKeyID]; ok && now.Sub(previous) < 900*time.Millisecond {
		return false
	}
	seedanceRequestLimiter.last[apiKeyID] = now
	if len(seedanceRequestLimiter.last) > 10000 {
		for key, value := range seedanceRequestLimiter.last {
			if now.Sub(value) > 10*time.Minute {
				delete(seedanceRequestLimiter.last, key)
			}
		}
	}
	return true
}
