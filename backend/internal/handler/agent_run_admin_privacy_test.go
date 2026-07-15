package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAgentRunToAdminAuditResponseOmitsBusinessData(t *testing.T) {
	startedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(1500 * time.Millisecond)
	run := &service.AgentRun{
		ID:                101,
		AppID:             11,
		AppVersionID:      12,
		UserID:            21,
		APIKeyID:          22,
		Status:            service.AgentRunStatusFailed,
		InputRefURL:       "cos://private/input.png",
		InputSummaryJSON:  map[string]any{"prompt": "private customer prompt", "input_assets": []any{"private image"}},
		OutputRefURL:      "cos://private/output.png",
		OutputSummaryJSON: map[string]any{"result": "private generated content"},
		ErrorCode:         "MODEL_PROXY_FAILED",
		ErrorMessage:      "upstream echoed private customer prompt",
		UsageJSON:         map[string]any{"raw_response": "private generated content"},
		StartedAt:         &startedAt,
		CompletedAt:       &completedAt,
		CreatedAt:         startedAt,
		UpdatedAt:         completedAt,
	}

	payload, err := json.Marshal(agentRunToAdminAuditResponse(run))
	require.NoError(t, err)
	serialized := string(payload)

	require.Contains(t, serialized, `"id":101`)
	require.Contains(t, serialized, `"duration_ms":1500`)
	for _, forbidden := range []string{
		"input_summary_json",
		"output_summary_json",
		"input_ref_url",
		"output_ref_url",
		"error_message",
		"error_code",
		"usage_json",
		"artifact",
		"object_key",
		"private customer prompt",
		"private generated content",
		"MODEL_PROXY_FAILED",
		"cos://private",
	} {
		require.NotContains(t, serialized, forbidden)
	}
}
