package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestRecordFailoverSwitchAuditAppearsInList(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(9001))
	ctx = context.WithValue(ctx, ctxkey.RequestID, "req-failover-audit")
	ctx = context.WithValue(ctx, ctxkey.Model, "gpt-test")
	ctx = context.WithValue(ctx, ctxkey.Platform, "openai")

	RecordFailoverSwitchAudit(ctx, 11, "from-acc", 0, "", "openai", 429, 1, 10, "rate_limited")
	items := ListAccountSwitchAudit(50, 9001, 0, 11, "")
	found := false
	for _, it := range items {
		if it.EventType == "failover_switch" && it.FromAccountID == 11 && it.RequestID == "req-failover-audit" {
			found = true
			if it.Reason != "rate_limited" {
				t.Fatalf("reason=%s", it.Reason)
			}
			break
		}
	}
	if !found {
		t.Fatalf("failover audit not found, got %d items", len(items))
	}
}
