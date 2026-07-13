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
