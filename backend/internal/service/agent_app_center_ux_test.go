package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkerRunRoutesNormalizesManifestRoutes(t *testing.T) {
	routes := workerRunRoutes(map[string]any{
		"runs": []any{"/runs", "/workflow/runs", "/workflow/runs", 123},
	})
	require.Equal(t, []string{"/runs", "/workflow/runs"}, routes)
}

func TestBuildWorkerHostFromInputOnlySupportsHMACRunToken(t *testing.T) {
	base := CreateAgentWorkerHostInput{
		Name:    "worker-a",
		BaseURL: "https://worker.example.com",
	}

	host, err := buildWorkerHostFromInput(base, nil)
	require.NoError(t, err)
	require.Equal(t, AgentWorkerAuthHMACRunToken, host.AuthType)

	for _, authType := range []string{AgentWorkerAuthNone, AgentWorkerAuthBearer} {
		input := base
		input.AuthType = authType
		host, err := buildWorkerHostFromInput(input, nil)
		require.Nil(t, host)
		require.Error(t, err)
		require.Contains(t, err.Error(), "HMAC")
	}
}

func TestValidateAgentAppModelPoliciesAllowsOptionalBranches(t *testing.T) {
	err := validateAgentAppModelPolicies(map[string]any{
		"text.generate": map[string]any{
			"provider": "openai",
			"model":    "gpt-test",
		},
		"image.fallback": map[string]any{
			"provider": "openai",
			"model":    "image-test",
			"optional": true,
		},
	})
	require.NoError(t, err)
}

func TestValidateAgentAppModelPoliciesRejectsAllOptional(t *testing.T) {
	err := validateAgentAppModelPolicies(map[string]any{
		"image.fallback": map[string]any{
			"provider": "openai",
			"model":    "image-test",
			"optional": true,
		},
	})
	require.Error(t, err)
}

func TestStorageCredentialIdentityChanged(t *testing.T) {
	base := agentArtifactStorageConfigRecord{
		Provider:    "cos",
		Region:      "ap-hongkong",
		Bucket:      "bucket-a",
		AccessKeyID: "key-a",
	}
	sameCredentialsNewBucket := base
	sameCredentialsNewBucket.Bucket = "bucket-b"
	require.False(t, storageCredentialIdentityChanged(base, sameCredentialsNewBucket))

	newProvider := base
	newProvider.Provider = "oss"
	require.True(t, storageCredentialIdentityChanged(base, newProvider))

	newAccessKey := base
	newAccessKey.AccessKeyID = "key-b"
	require.True(t, storageCredentialIdentityChanged(base, newAccessKey))
}
