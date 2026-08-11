package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestAccountSwitchAuditRetentionAndList(t *testing.T) {
	store := &accountSwitchAuditStore{events: make([]AccountSwitchAuditEvent, 0, 8)}

	// fresh event
	store.record(AccountSwitchAuditEvent{
		UserID:        7,
		Reason:        "ttft",
		FromAccountID: 11,
		ToAccountID:   22,
		EventType:     "sticky_escape",
	})
	// expired event
	store.record(AccountSwitchAuditEvent{
		At:            time.Now().UTC().Add(-25 * time.Hour),
		UserID:        7,
		Reason:        "ttft",
		FromAccountID: 33,
		ToAccountID:   44,
	})

	items := store.list(50, 0, 0, 0, "")
	if len(items) != 1 {
		t.Fatalf("expected 1 retained event, got %d", len(items))
	}
	if items[0].FromAccountID != 11 {
		t.Fatalf("unexpected retained event from=%d", items[0].FromAccountID)
	}

	filtered := store.list(50, 7, 0, 22, "ttft")
	if len(filtered) != 1 {
		t.Fatalf("expected filter match, got %d", len(filtered))
	}
	noMatch := store.list(50, 8, 0, 0, "")
	if len(noMatch) != 0 {
		t.Fatalf("expected no match for other user, got %d", len(noMatch))
	}
}

func TestBuildAccountSwitchCandidatesKeepsEscapedAndSelected(t *testing.T) {
	accounts := make([]*Account, 0, 15)
	scores := map[int64]OpenAIAccountSchedulerScoreSnapshot{}
	for i := int64(1); i <= 15; i++ {
		accounts = append(accounts, &Account{ID: i, Name: "a", Priority: int(i)})
		scores[i] = OpenAIAccountSchedulerScoreSnapshot{
			BaseScore:       float64(100 - i),
			AvgFirstTokenMs: float64(i * 100),
			HasTTFT:         true,
			ErrorRate:       0.01 * float64(i),
		}
	}
	// from=15 is lowest score, to=1 highest
	cands := buildAccountSwitchCandidatesFromScores(accounts, scores, 15, 1, 5)
	if len(cands) < 5 {
		t.Fatalf("expected at least topN candidates, got %d", len(cands))
	}
	var hasFrom, hasTo bool
	for _, c := range cands {
		if c.AccountID == 15 && c.Escaped {
			hasFrom = true
		}
		if c.AccountID == 1 && c.Selected {
			hasTo = true
		}
	}
	if !hasFrom || !hasTo {
		t.Fatalf("expected escaped+selected kept, hasFrom=%v hasTo=%v len=%d", hasFrom, hasTo, len(cands))
	}
}

func TestBuildAccountSwitchAuditFromEscapeContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(99))
	ctx = context.WithValue(ctx, ctxkey.RequestID, "req-1")
	ctx = context.WithValue(ctx, ctxkey.Model, "gpt-test")
	gid := int64(3)
	meta := &accountSwitchEscapeMeta{
		fromAccountID: 10,
		fromName:      "slow",
		reason:        "ttft_relative",
		errorRate:     0.2,
		ttft:          3200,
		hasTTFT:       true,
		layer:         "session_hash",
		cfg: openAIStickyEscapeConfig{
			enabled: true, ttftMs: 1500, errorRate: 0.5, relativeRatio: 1.15, relativeMinDelta: 250,
		},
		contextOK: true,
	}
	selected := &Account{ID: 20, Name: "fast"}
	ev := buildAccountSwitchAuditFromEscape(ctx, OpenAIAccountScheduleRequest{
		GroupID:        &gid,
		Platform:       "openai",
		SessionHash:    "abcdefghijklmnop",
		RequestedModel: "gpt-test",
	}, meta, selected, nil, &AccountSwitchScoreWeights{TTFT: 2.5})

	if ev.UserID != 99 || ev.ToAccountID != 20 || ev.FromAccountID != 10 {
		t.Fatalf("unexpected event user/to/from: %+v", ev)
	}
	if ev.Reason != "ttft_relative" || !ev.ContextPreserved {
		t.Fatalf("unexpected reason/context: %+v", ev)
	}
	if ev.ThresholdTTFTMs != 1500 {
		t.Fatalf("threshold not recorded: %v", ev.ThresholdTTFTMs)
	}
}
