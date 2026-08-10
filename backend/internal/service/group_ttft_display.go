package service

import (
	"math"
	"strings"
	"sync"
	"time"
)

// User-facing TTFT display for model plaza / group cards.
//
// Design rules:
//   - Never expose "no traffic / sample count = 0" to end users
//   - Prefer recent real request EWMA when available
//   - Fall back to last stable display or platform baseline
//   - Always attach a neutral disclaimer

const (
	GroupTTFTDisclaimerZH = "根据近期请求统计，仅供参考"
	GroupTTFTDisclaimerEN = "Based on recent requests; for reference only"

	groupTTFTDefaultBaselineMs = 900.0
	groupTTFTAlpha             = 0.2
	groupTTFTMaxStepRatio      = 0.2 // display cannot jump more than 20% per update
)

// GroupTTFTDisplay is the user-visible latency card payload.
type GroupTTFTDisplay struct {
	// AvgFirstTokenMs is always populated for user surfaces (baseline if needed).
	AvgFirstTokenMs int `json:"avg_first_token_ms"`
	// Disclaimer must be shown next to the metric.
	Disclaimer string `json:"disclaimer"`
	// Source is internal/admin oriented: live | baseline | mixed
	Source string `json:"-"`
}

type groupTTFTState struct {
	ewma        float64
	hasEWMA     bool
	display     float64
	hasDisplay  bool
	sampleCount int64
	lastAt      time.Time
}

// GroupTTFTDisplayStore keeps process-local group TTFT EWMA for showcase.
// It is intentionally not a hard scheduler gate.
type GroupTTFTDisplayStore struct {
	mu    sync.RWMutex
	byID  map[int64]*groupTTFTState
	nowFn func() time.Time
}

// DefaultGroupTTFTDisplay is the process-wide store used by gateways and plaza.
var DefaultGroupTTFTDisplay = NewGroupTTFTDisplayStore()

func NewGroupTTFTDisplayStore() *GroupTTFTDisplayStore {
	return &GroupTTFTDisplayStore{
		byID:  make(map[int64]*groupTTFTState),
		nowFn: time.Now,
	}
}

func (s *GroupTTFTDisplayStore) currentTime() time.Time {
	if s != nil && s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

// Report records a successful first-token observation for a group.
func (s *GroupTTFTDisplayStore) Report(groupID int64, firstTokenMs int) {
	if s == nil || groupID <= 0 || firstTokenMs <= 0 {
		return
	}
	sample := float64(firstTokenMs)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.byID[groupID]
	if st == nil {
		st = &groupTTFTState{}
		s.byID[groupID] = st
	}
	if !st.hasEWMA {
		st.ewma = sample
		st.hasEWMA = true
	} else {
		st.ewma = groupTTFTAlpha*sample + (1-groupTTFTAlpha)*st.ewma
	}
	st.sampleCount++
	st.lastAt = s.currentTime().UTC()
	st.display = smoothTTFTDisplay(st.display, st.hasDisplay, st.ewma)
	st.hasDisplay = true
}

// GetDisplay returns a user-safe TTFT figure for the group.
// platform is used only for baseline when live data is missing.
func (s *GroupTTFTDisplayStore) GetDisplay(groupID int64, platform string) GroupTTFTDisplay {
	baseline := PlatformTTFTBaselineMs(platform)
	disclaimer := GroupTTFTDisclaimerZH

	if s == nil || groupID <= 0 {
		return GroupTTFTDisplay{
			AvgFirstTokenMs: int(math.Round(baseline)),
			Disclaimer:      disclaimer,
			Source:          "baseline",
		}
	}

	s.mu.RLock()
	st := s.byID[groupID]
	var display float64
	hasDisplay := false
	hasEWMA := false
	samples := int64(0)
	if st != nil {
		display = st.display
		hasDisplay = st.hasDisplay
		hasEWMA = st.hasEWMA
		samples = st.sampleCount
	}
	s.mu.RUnlock()

	source := "baseline"
	value := baseline
	if hasDisplay && display > 0 {
		value = display
		if samples >= 20 {
			source = "live"
		} else if hasEWMA {
			// blend toward baseline when samples are sparse (still no user-facing empty state)
			value = 0.6*display + 0.4*baseline
			source = "mixed"
		}
	}

	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		value = baseline
		source = "baseline"
	}

	return GroupTTFTDisplay{
		AvgFirstTokenMs: int(math.Round(value)),
		Disclaimer:      disclaimer,
		Source:          source,
	}
}

// GetDisplayBatch resolves many groups at once.
func (s *GroupTTFTDisplayStore) GetDisplayBatch(groupPlatforms map[int64]string) map[int64]GroupTTFTDisplay {
	out := make(map[int64]GroupTTFTDisplay, len(groupPlatforms))
	for id, platform := range groupPlatforms {
		out[id] = s.GetDisplay(id, platform)
	}
	return out
}

func smoothTTFTDisplay(prev float64, hasPrev bool, target float64) float64 {
	if !hasPrev || prev <= 0 {
		return target
	}
	maxDelta := prev * groupTTFTMaxStepRatio
	delta := target - prev
	if delta > maxDelta {
		return prev + maxDelta
	}
	if delta < -maxDelta {
		return prev - maxDelta
	}
	return target
}

// PlatformTTFTBaselineMs returns a neutral showcase baseline so empty traffic
// never surfaces as "unused".
func PlatformTTFTBaselineMs(platform string) float64 {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI, "codex":
		return 750
	case PlatformAnthropic:
		return 850
	case PlatformGemini:
		return 700
	case PlatformAntigravity:
		return 900
	case PlatformGrok:
		return 1000
	case PlatformGLM:
		return 900
	case "seedance", "minimax", "ltx", "happyhorse", "grok_imagine", "fflink":
		// media workloads are not classic TTFT; keep a calm placeholder
		return 1200
	default:
		return groupTTFTDefaultBaselineMs
	}
}

// ReportGroupFirstToken is a package helper for handlers/gateways.
func ReportGroupFirstToken(groupID int64, firstTokenMs int) {
	DefaultGroupTTFTDisplay.Report(groupID, firstTokenMs)
}
