package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAgentArtifactPolicyVersionOverrides(t *testing.T) {
	retentionDays := 7
	policy := mergeAgentArtifactPolicy(agentArtifactPolicy{
		RetentionDays: &retentionDays,
		MaxBytes:      512,
		AllowedTypes:  map[string]bool{},
	}, map[string]any{
		"retention_days": int64(0),
		"max_file_mb":    int64(2),
		"allowed_types":  []any{"image", "json"},
	})

	require.Nil(t, policy.expiresAt())
	require.Equal(t, int64(2*1024*1024), policy.MaxBytes)
	require.True(t, policy.HasAllowedTypes)
	require.True(t, policy.AllowedTypes["image"])
	require.True(t, policy.AllowedTypes["json"])
}

func TestEnforceAgentArtifactPolicyAllowedTypeAndSize(t *testing.T) {
	policy := agentArtifactPolicy{
		MaxBytes:        1024,
		AllowedTypes:    map[string]bool{"image": true, "json": true},
		HasAllowedTypes: true,
	}

	require.NoError(t, enforceAgentArtifactPolicy(policy, AgentArtifactTypeOutput, "result.png", "image/png", 1024))
	require.NoError(t, enforceAgentArtifactPolicy(policy, AgentArtifactTypeOutput, "result.json", "", 64))
	require.Error(t, enforceAgentArtifactPolicy(policy, AgentArtifactTypeOutput, "result.mp4", "video/mp4", 64))
	require.Error(t, enforceAgentArtifactPolicy(policy, AgentArtifactTypeOutput, "result.png", "image/png", 1025))
}
