package service

import (
	"testing"
	"time"
)

func TestAccountSwitchAuditPublicAPI(t *testing.T) {
	id := "test-audit-" + time.Now().UTC().Format("150405.000")
	RecordAccountSwitchAudit(AccountSwitchAuditEvent{
		ID:               id,
		UserID:           12345,
		Reason:           "ttft_relative",
		FromAccountID:    101,
		FromAccountName:  "slow-acc",
		ToAccountID:      202,
		ToAccountName:    "fast-acc",
		EventType:        "sticky_escape",
		ContextPreserved: true,
		HasFromTTFT:      true,
		FromTTFTMs:       4200,
		ThresholdTTFTMs:  2500,
		Note:             "unit public api",
		Candidates: []AccountSwitchAuditCandidate{
			{AccountID: 202, AccountName: "fast-acc", Score: 90, Selected: true, HasTTFT: true, TTFTMs: 400},
			{AccountID: 101, AccountName: "slow-acc", Score: 10, Escaped: true, HasTTFT: true, TTFTMs: 4200},
		},
	})

	items := ListAccountSwitchAudit(50, 12345, 0, 0, "ttft_relative")
	found := false
	for _, it := range items {
		if it.ID == id {
			found = true
			if it.ToAccountID != 202 || !it.ContextPreserved {
				t.Fatalf("unexpected event: %+v", it)
			}
			if len(it.Candidates) < 2 {
				t.Fatalf("expected candidates, got %d", len(it.Candidates))
			}
			break
		}
	}
	if !found {
		t.Fatalf("recorded event not found, got %d items", len(items))
	}
	if AccountSwitchAuditRetentionHours() != 24 {
		t.Fatalf("retention hours=%d", AccountSwitchAuditRetentionHours())
	}
}
