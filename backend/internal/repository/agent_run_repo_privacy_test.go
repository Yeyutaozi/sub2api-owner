package repository

import (
	"strings"
	"testing"
)

func TestAgentRunAuditSelectSQLExcludesBusinessData(t *testing.T) {
	query := strings.ToLower(agentRunAuditSelectSQL())
	for _, required := range []string{"user_id", "api_key_id", "worker_host_id", "status", "started_at", "completed_at"} {
		if !strings.Contains(query, required) {
			t.Fatalf("audit query is missing required field %q", required)
		}
	}
	for _, forbidden := range []string{
		"run_token_hash",
		"input_ref_url",
		"input_summary_json",
		"output_ref_url",
		"output_summary_json",
		"error_code",
		"error_message",
		"usage_json",
		"expires_at",
	} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("audit query must not select business field %q", forbidden)
		}
	}
}
