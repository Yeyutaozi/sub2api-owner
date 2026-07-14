package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type failingAgentArtifactSecretEncryptor struct{}

func (failingAgentArtifactSecretEncryptor) Encrypt(string) (string, error) {
	return "", errors.New("encrypt failed")
}

func (failingAgentArtifactSecretEncryptor) Decrypt(string) (string, error) {
	return "", errors.New("decrypt failed")
}

func TestAgentArtifactStorageValidateConfigRejectsDisabledEmptyDraft(t *testing.T) {
	svc := &AgentArtifactStorageConfigService{
		cfg: &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}},
	}

	_, err := svc.ValidateConfig(context.Background(), AgentArtifactStorageConfigView{
		Enabled:  false,
		Provider: "cos",
	})

	require.Error(t, err)
	require.Equal(t, "AGENT_ARTIFACT_STORAGE_BUCKET_REQUIRED", infraerrors.Reason(err))
}

func TestValidateAgentArtifactStorageRecordRejectsRelativePublicBaseURL(t *testing.T) {
	err := validateAgentArtifactStorageRecord(agentArtifactStorageConfigRecord{
		Enabled:                  true,
		Provider:                 "cos",
		Region:                   "ap-hongkong",
		Bucket:                   "bucket-1234567890",
		AccessKeyID:              "access-key",
		SecretAccessKeyEncrypted: "ciphertext",
		PublicBaseURL:            "image",
		MaxUploadBytes:           1024,
		DownloadURLTTLSeconds:    60,
	}, true)

	require.Error(t, err)
	require.Equal(t, "AGENT_ARTIFACT_STORAGE_PUBLIC_BASE_URL_INVALID", infraerrors.Reason(err))
}

func TestAgentArtifactStorageConfigViewRequiresSecretReentryAfterDecryptFailure(t *testing.T) {
	svc := &AgentArtifactStorageConfigService{
		encryptor: failingAgentArtifactSecretEncryptor{},
		cfg:       &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}},
	}

	view, err := svc.recordToView(&agentArtifactStorageConfigRecord{
		Enabled:                  true,
		Provider:                 "cos",
		Region:                   "ap-hongkong",
		Bucket:                   "bucket",
		AccessKeyID:              "access-key",
		SecretAccessKeyEncrypted: "old-ciphertext",
		SecretEncryptedAtRest:    true,
	}, "database")

	require.NoError(t, err)
	require.False(t, view.SecretAccessKeyConfigured)
	require.Contains(t, view.RuntimeError, "AGENT_ARTIFACT_STORAGE_SECRET_DECRYPT_FAILED")
}
