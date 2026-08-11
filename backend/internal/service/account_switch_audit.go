package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	accountSwitchAuditRetention = 24 * time.Hour
	accountSwitchAuditMaxEvents = 5000
	accountSwitchAuditTopN      = 12
)

// AccountSwitchAuditEvent is an admin-auditable record of automatic sticky/account switches.
// Retention is 24h (hard-capped by max events). When Redis is wired, events survive process
// restarts and are shared across instances; otherwise they stay process-local memory.
type AccountSwitchAuditEvent struct {
	ID                 string                        `json:"id"`
	At                 time.Time                     `json:"at"`
	EventType          string                        `json:"event_type"` // sticky_escape | failover_switch
	Reason             string                        `json:"reason"`
	UserID             int64                         `json:"user_id,omitempty"`
	GroupID            *int64                        `json:"group_id,omitempty"`
	Platform           string                        `json:"platform,omitempty"`
	Model              string                        `json:"model,omitempty"`
	RequestID          string                        `json:"request_id,omitempty"`
	SessionHashShort   string                        `json:"session_hash_short,omitempty"`
	Layer              string                        `json:"layer,omitempty"`
	FromAccountID      int64                         `json:"from_account_id,omitempty"`
	FromAccountName    string                        `json:"from_account_name,omitempty"`
	ToAccountID        int64                         `json:"to_account_id,omitempty"`
	ToAccountName      string                        `json:"to_account_name,omitempty"`
	FromTTFTMs         float64                       `json:"from_ttft_ms,omitempty"`
	FromErrorRate      float64                       `json:"from_error_rate,omitempty"`
	HasFromTTFT        bool                          `json:"has_from_ttft"`
	ContextPreserved   bool                          `json:"context_preserved"`
	ThresholdTTFTMs    float64                       `json:"threshold_ttft_ms,omitempty"`
	ThresholdErrorRate float64                       `json:"threshold_error_rate,omitempty"`
	RelativeRatio      float64                       `json:"relative_ratio,omitempty"`
	RelativeMinDeltaMs float64                       `json:"relative_min_delta_ms,omitempty"`
	ScoreWeights       *AccountSwitchScoreWeights    `json:"score_weights,omitempty"`
	Candidates         []AccountSwitchAuditCandidate `json:"candidates,omitempty"`
	Note               string                        `json:"note,omitempty"`
}

// AccountSwitchScoreWeights mirrors active scheduler score weights for audit readability.
type AccountSwitchScoreWeights struct {
	Priority      float64 `json:"priority"`
	Load          float64 `json:"load"`
	Queue         float64 `json:"queue"`
	ErrorRate     float64 `json:"error_rate"`
	TTFT          float64 `json:"ttft"`
	Reset         float64 `json:"reset"`
	QuotaHeadroom float64 `json:"quota_headroom"`
	UpstreamCost  float64 `json:"upstream_cost"`
}

// AccountSwitchAuditCandidate is one scored account considered during reselection.
type AccountSwitchAuditCandidate struct {
	AccountID      int64   `json:"account_id"`
	AccountName    string  `json:"account_name,omitempty"`
	Score          float64 `json:"score"`
	Priority       int     `json:"priority"`
	TTFTMs         float64 `json:"ttft_ms,omitempty"`
	HasTTFT        bool    `json:"has_ttft"`
	ErrorRate      float64 `json:"error_rate"`
	LoadRate       float64 `json:"load_rate"`
	WaitingCount   int     `json:"waiting_count"`
	RateMultiplier float64 `json:"rate_multiplier,omitempty"`
	Selected       bool    `json:"selected"`
	Escaped        bool    `json:"escaped,omitempty"`
}

type accountSwitchEscapeMeta struct {
	fromAccountID int64
	fromName      string
	reason        string
	errorRate     float64
	ttft          float64
	hasTTFT       bool
	layer         string
	cfg           openAIStickyEscapeConfig
	contextOK     bool
	note          string
}

type accountSwitchAuditStore struct {
	mu     sync.RWMutex
	events []AccountSwitchAuditEvent // newest first
}

var defaultAccountSwitchAuditStore = &accountSwitchAuditStore{
	events: make([]AccountSwitchAuditEvent, 0, 256),
}

const accountSwitchAuditRedisKey = "sub2api:account_switch_audit:v1"

var accountSwitchAuditRedis atomic.Pointer[redis.Client]

// SetAccountSwitchAuditRedis enables cross-instance 24h persistence for switch audits.
// Safe to call with nil to disable.
func SetAccountSwitchAuditRedis(client *redis.Client) {
	accountSwitchAuditRedis.Store(client)
}

func accountSwitchAuditRedisClient() *redis.Client {
	return accountSwitchAuditRedis.Load()
}

// RecordAccountSwitchAudit appends an automatic switch audit event (best-effort, never blocks hot path hard).
func RecordAccountSwitchAudit(event AccountSwitchAuditEvent) {
	defaultAccountSwitchAuditStore.record(event)
	persistAccountSwitchAuditToRedis(event)
}

// ListAccountSwitchAudit returns newest-first events within retention.
// Prefers Redis (shared) when configured; falls back to process memory.
func ListAccountSwitchAudit(limit int, userID, groupID, accountID int64, reason string) []AccountSwitchAuditEvent {
	if items, ok := listAccountSwitchAuditFromRedis(limit * 2); ok {
		return filterAccountSwitchAuditEvents(items, limit, userID, groupID, accountID, reason)
	}
	return defaultAccountSwitchAuditStore.list(limit, userID, groupID, accountID, reason)
}

// AccountSwitchAuditRetentionHours exposes retention for admin UI/API.
func AccountSwitchAuditRetentionHours() int {
	return int(accountSwitchAuditRetention / time.Hour)
}

func (s *accountSwitchAuditStore) record(event AccountSwitchAuditEvent) {
	if s == nil {
		return
	}
	event = normalizeAccountSwitchAuditEvent(event)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append([]AccountSwitchAuditEvent{event}, s.events...)
	s.pruneLocked(time.Now().UTC())
}

func (s *accountSwitchAuditStore) list(limit int, userID, groupID, accountID int64, reason string) []AccountSwitchAuditEvent {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC()

	s.mu.Lock()
	s.pruneLocked(now)
	src := make([]AccountSwitchAuditEvent, len(s.events))
	copy(src, s.events)
	s.mu.Unlock()

	out := make([]AccountSwitchAuditEvent, 0, limit)
	for i := range src {
		ev := src[i]
		if userID > 0 && ev.UserID != userID {
			continue
		}
		if groupID > 0 {
			if ev.GroupID == nil || *ev.GroupID != groupID {
				continue
			}
		}
		if accountID > 0 && ev.FromAccountID != accountID && ev.ToAccountID != accountID {
			continue
		}
		if reason != "" && !strings.EqualFold(ev.Reason, reason) {
			continue
		}
		out = append(out, ev)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *accountSwitchAuditStore) pruneLocked(now time.Time) {
	cutoff := now.Add(-accountSwitchAuditRetention)
	// Filter expired regardless of insertion order; keep newest-first order.
	kept := s.events[:0]
	for i := range s.events {
		if s.events[i].At.Before(cutoff) {
			continue
		}
		kept = append(kept, s.events[i])
	}
	if len(kept) > accountSwitchAuditMaxEvents {
		kept = kept[:accountSwitchAuditMaxEvents]
	}
	// Ensure no aliasing surprises if capacity was reused.
	s.events = append([]AccountSwitchAuditEvent(nil), kept...)
}

func filterAccountSwitchAuditEvents(src []AccountSwitchAuditEvent, limit int, userID, groupID, accountID int64, reason string) []AccountSwitchAuditEvent {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	reason = strings.TrimSpace(reason)
	cutoff := time.Now().UTC().Add(-accountSwitchAuditRetention)
	out := make([]AccountSwitchAuditEvent, 0, limit)
	for i := range src {
		ev := src[i]
		if ev.At.IsZero() || ev.At.Before(cutoff) {
			continue
		}
		if userID > 0 && ev.UserID != userID {
			continue
		}
		if groupID > 0 {
			if ev.GroupID == nil || *ev.GroupID != groupID {
				continue
			}
		}
		if accountID > 0 && ev.FromAccountID != accountID && ev.ToAccountID != accountID {
			continue
		}
		if reason != "" && !strings.EqualFold(ev.Reason, reason) {
			continue
		}
		out = append(out, ev)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeAccountSwitchAuditEvent(event AccountSwitchAuditEvent) AccountSwitchAuditEvent {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	if event.EventType == "" {
		event.EventType = "sticky_escape"
	}
	return event
}

func persistAccountSwitchAuditToRedis(event AccountSwitchAuditEvent) {
	rdb := accountSwitchAuditRedisClient()
	if rdb == nil {
		return
	}
	event = normalizeAccountSwitchAuditEvent(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	pipe := rdb.Pipeline()
	pipe.LPush(ctx, accountSwitchAuditRedisKey, payload)
	pipe.LTrim(ctx, accountSwitchAuditRedisKey, 0, int64(accountSwitchAuditMaxEvents-1))
	pipe.Expire(ctx, accountSwitchAuditRedisKey, accountSwitchAuditRetention)
	_, _ = pipe.Exec(ctx)
}

func listAccountSwitchAuditFromRedis(limit int) ([]AccountSwitchAuditEvent, bool) {
	rdb := accountSwitchAuditRedisClient()
	if rdb == nil {
		return nil, false
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > accountSwitchAuditMaxEvents {
		limit = accountSwitchAuditMaxEvents
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	raw, err := rdb.LRange(ctx, accountSwitchAuditRedisKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, false
	}
	out := make([]AccountSwitchAuditEvent, 0, len(raw))
	for _, item := range raw {
		var ev AccountSwitchAuditEvent
		if json.Unmarshal([]byte(item), &ev) != nil {
			continue
		}
		out = append(out, ev)
	}
	return out, true
}

// RecordFailoverSwitchAudit records upstream-error driven account switches (not sticky TTFT escape).
func RecordFailoverSwitchAudit(
	ctx context.Context,
	fromAccountID int64,
	fromAccountName string,
	toAccountID int64,
	toAccountName string,
	platform string,
	statusCode int,
	switchCount int,
	maxSwitches int,
	failoverReason string,
) {
	userID, requestID, model, ctxPlatform := accountSwitchAuditContext(ctx)
	if platform == "" {
		platform = ctxPlatform
	}
	reason := strings.TrimSpace(failoverReason)
	if reason == "" {
		if statusCode > 0 {
			reason = "upstream_" + strconv.Itoa(statusCode)
		} else {
			reason = "upstream_failover"
		}
	}
	note := fmt.Sprintf("上游失败触发切号 switch=%d/%d status=%d；同账号临时重试不记入", switchCount, maxSwitches, statusCode)
	RecordAccountSwitchAudit(AccountSwitchAuditEvent{
		EventType:        "failover_switch",
		Reason:           reason,
		UserID:           userID,
		Platform:         platform,
		Model:            model,
		RequestID:        requestID,
		Layer:            "failover",
		FromAccountID:    fromAccountID,
		FromAccountName:  fromAccountName,
		ToAccountID:      toAccountID,
		ToAccountName:    toAccountName,
		ContextPreserved: true,
		Note:             note,
	})
}

func accountSwitchAuditContext(ctx context.Context) (userID int64, requestID, model, platform string) {
	if ctx == nil {
		return 0, "", "", ""
	}
	if v, ok := ctx.Value(ctxkey.UserID).(int64); ok {
		userID = v
	}
	if v, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		requestID = strings.TrimSpace(v)
	}
	if v, ok := ctx.Value(ctxkey.Model).(string); ok {
		model = strings.TrimSpace(v)
	}
	if v, ok := ctx.Value(ctxkey.Platform).(string); ok {
		platform = strings.TrimSpace(v)
	}
	return userID, requestID, model, platform
}

func buildAccountSwitchAuditFromEscape(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	meta *accountSwitchEscapeMeta,
	selected *Account,
	candidates []AccountSwitchAuditCandidate,
	weights *AccountSwitchScoreWeights,
) AccountSwitchAuditEvent {
	userID, requestID, model, platform := accountSwitchAuditContext(ctx)
	if model == "" {
		model = strings.TrimSpace(req.RequestedModel)
	}
	if platform == "" {
		platform = strings.TrimSpace(req.Platform)
	}

	ev := AccountSwitchAuditEvent{
		EventType:          "sticky_escape",
		Reason:             "",
		UserID:             userID,
		GroupID:            req.GroupID,
		Platform:           platform,
		Model:              model,
		RequestID:          requestID,
		SessionHashShort:   shortSessionHash(req.SessionHash),
		ContextPreserved:   true,
		Candidates:         candidates,
		ScoreWeights:       weights,
		Note:               "自动切号审计（进程内保留 24h，基于真实请求样本，非主动探测）",
	}
	if meta != nil {
		ev.Reason = meta.reason
		ev.Layer = meta.layer
		ev.FromAccountID = meta.fromAccountID
		ev.FromAccountName = meta.fromName
		ev.FromTTFTMs = meta.ttft
		ev.FromErrorRate = meta.errorRate
		ev.HasFromTTFT = meta.hasTTFT
		ev.ContextPreserved = meta.contextOK
		ev.ThresholdTTFTMs = meta.cfg.ttftMs
		ev.ThresholdErrorRate = meta.cfg.errorRate
		ev.RelativeRatio = meta.cfg.relativeRatio
		ev.RelativeMinDeltaMs = meta.cfg.relativeMinDelta
		if meta.note != "" {
			ev.Note = meta.note
		}
	}
	if selected != nil {
		ev.ToAccountID = selected.ID
		ev.ToAccountName = selected.Name
	}
	return ev
}

func buildAccountSwitchCandidatesFromScores(
	accounts []*Account,
	scores map[int64]OpenAIAccountSchedulerScoreSnapshot,
	fromID, toID int64,
	topN int,
) []AccountSwitchAuditCandidate {
	if topN <= 0 {
		topN = accountSwitchAuditTopN
	}
	nameByID := make(map[int64]string, len(accounts))
	priorityByID := make(map[int64]int, len(accounts))
	for _, acc := range accounts {
		if acc == nil {
			continue
		}
		nameByID[acc.ID] = acc.Name
		priorityByID[acc.ID] = acc.Priority
	}

	list := make([]AccountSwitchAuditCandidate, 0, len(scores))
	for id, sc := range scores {
		list = append(list, AccountSwitchAuditCandidate{
			AccountID:      id,
			AccountName:    nameByID[id],
			Score:          sc.BaseScore,
			Priority:       priorityByID[id],
			TTFTMs:         sc.AvgFirstTokenMs,
			HasTTFT:        sc.HasTTFT,
			ErrorRate:      sc.ErrorRate,
			LoadRate:       sc.LoadRate,
			WaitingCount:   sc.WaitingCount,
			RateMultiplier: sc.RateMultiplier,
			Selected:       id == toID,
			Escaped:        id == fromID,
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return list[i].AccountID < list[j].AccountID
	})
	if len(list) > topN {
		// Keep selected/escaped even if outside topN.
		kept := list[:topN]
		need := map[int64]struct{}{}
		if fromID > 0 {
			need[fromID] = struct{}{}
		}
		if toID > 0 {
			need[toID] = struct{}{}
		}
		have := make(map[int64]struct{}, len(kept))
		for _, c := range kept {
			have[c.AccountID] = struct{}{}
		}
		for _, c := range list[topN:] {
			if _, ok := need[c.AccountID]; !ok {
				continue
			}
			if _, ok := have[c.AccountID]; ok {
				continue
			}
			kept = append(kept, c)
		}
		list = kept
	}
	return list
}

func currentOpenAIScoreWeightsView(s *OpenAIGatewayService) *AccountSwitchScoreWeights {
	if s == nil {
		return nil
	}
	w := s.openAIWSSchedulerWeights()
	return &AccountSwitchScoreWeights{
		Priority:      w.Priority,
		Load:          w.Load,
		Queue:         w.Queue,
		ErrorRate:     w.ErrorRate,
		TTFT:          w.TTFT,
		Reset:         w.Reset,
		QuotaHeadroom: w.QuotaHeadroom,
		UpstreamCost:  w.UpstreamCost,
	}
}

func (s *defaultOpenAIAccountScheduler) enrichAndRecordStickyEscapeAudit(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	meta *accountSwitchEscapeMeta,
	selected *Account,
) {
	if meta == nil || s == nil || s.service == nil {
		return
	}
	var (
		accounts []*Account
		loadMap  map[int64]*AccountLoadInfo
	)
	if raw, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform); err == nil && len(raw) > 0 {
		accounts = make([]*Account, 0, len(raw))
		loadReq := make([]AccountWithConcurrency, 0, len(raw))
		for i := range raw {
			acc := &raw[i]
			accounts = append(accounts, acc)
			loadReq = append(loadReq, AccountWithConcurrency{
				ID:             acc.ID,
				MaxConcurrency: acc.EffectiveLoadFactor(),
			})
		}
		if s.service.concurrencyService != nil {
			if batch, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
				loadMap = batch
			}
		}
	}

	var candidates []AccountSwitchAuditCandidate
	if len(accounts) > 0 {
		scores := BuildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap)
		toID := int64(0)
		if selected != nil {
			toID = selected.ID
		}
		candidates = buildAccountSwitchCandidatesFromScores(accounts, scores, meta.fromAccountID, toID, accountSwitchAuditTopN)
	}

	ev := buildAccountSwitchAuditFromEscape(ctx, req, meta, selected, candidates, currentOpenAIScoreWeightsView(s.service))
	RecordAccountSwitchAudit(ev)
}

// RecordGatewayStickyEscapeAudit records Claude/Gemini sticky escape with peer TTFT ranking.
func RecordGatewayStickyEscapeAudit(
	ctx context.Context,
	groupID *int64,
	platform, model, sessionHash, reason string,
	fromAccount *Account,
	toAccount *Account,
	peers []*Account,
	ttft, errorRate float64,
	cfg openAIStickyEscapeConfig,
) {
	userID, requestID, ctxModel, ctxPlatform := accountSwitchAuditContext(ctx)
	if model == "" {
		model = ctxModel
	}
	if platform == "" {
		platform = ctxPlatform
	}

	fromID := int64(0)
	fromName := ""
	if fromAccount != nil {
		fromID = fromAccount.ID
		fromName = fromAccount.Name
	}
	toID := int64(0)
	toName := ""
	if toAccount != nil {
		toID = toAccount.ID
		toName = toAccount.Name
	}

	candidates := make([]AccountSwitchAuditCandidate, 0, len(peers)+1)
	addPeer := func(acc *Account, escaped, selected bool) {
		if acc == nil {
			return
		}
		errRate, peerTTFT, has := SnapshotOpenAIAccountRuntime(acc.ID)
		candidates = append(candidates, AccountSwitchAuditCandidate{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Priority:    acc.Priority,
			TTFTMs:      peerTTFT,
			HasTTFT:     has,
			ErrorRate:   errRate,
			// Gateway sticky path prioritizes TTFT then priority; score = inverse TTFT heuristic.
			Score:    gatewayAuditScore(peerTTFT, has, acc.Priority),
			Escaped:  escaped,
			Selected: selected,
		})
	}
	seen := map[int64]struct{}{}
	if fromAccount != nil {
		addPeer(fromAccount, true, fromID == toID)
		seen[fromAccount.ID] = struct{}{}
	}
	for _, p := range peers {
		if p == nil {
			continue
		}
		if _, ok := seen[p.ID]; ok {
			continue
		}
		seen[p.ID] = struct{}{}
		addPeer(p, false, p.ID == toID)
	}
	if toAccount != nil {
		if _, ok := seen[toAccount.ID]; !ok {
			addPeer(toAccount, false, true)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].AccountID < candidates[j].AccountID
	})
	if len(candidates) > accountSwitchAuditTopN {
		candidates = candidates[:accountSwitchAuditTopN]
	}

	ev := AccountSwitchAuditEvent{
		EventType:          "sticky_escape",
		Reason:             reason,
		UserID:             userID,
		GroupID:            groupID,
		Platform:           platform,
		Model:              model,
		RequestID:          requestID,
		SessionHashShort:   shortSessionHash(sessionHash),
		Layer:              "gateway_sticky",
		FromAccountID:      fromID,
		FromAccountName:    fromName,
		ToAccountID:        toID,
		ToAccountName:      toName,
		FromTTFTMs:         ttft,
		FromErrorRate:      errorRate,
		HasFromTTFT:        ttft > 0,
		ContextPreserved:   true,
		ThresholdTTFTMs:    cfg.ttftMs,
		ThresholdErrorRate: cfg.errorRate,
		RelativeRatio:      cfg.relativeRatio,
		RelativeMinDeltaMs: cfg.relativeMinDelta,
		Candidates:         candidates,
		Note:               "网关粘性逃逸：请求正文上下文原样转发；仅更换上游账号（24h 审计）",
	}
	RecordAccountSwitchAudit(ev)
}

func gatewayAuditScore(ttft float64, hasTTFT bool, priority int) float64 {
	// Higher is better. Prefer lower TTFT; unknown TTFT gets mid score; lower priority value is better.
	score := 1000.0
	if hasTTFT && ttft > 0 {
		score = 1_000_000.0 / (ttft + 1)
	} else {
		score = 100.0
	}
	score -= float64(priority) * 0.01
	return score
}

