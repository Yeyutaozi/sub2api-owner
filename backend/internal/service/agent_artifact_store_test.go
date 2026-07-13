package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveAgentArtifactStorageSettingsProviderPresets(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.AgentArtifactStorageConfig
		wantProvider   string
		wantEndpoint   string
		wantRegion     string
		wantPathStyle  bool
		wantErrContain string
	}{
		{
			name: "cloudflare r2",
			cfg: config.AgentArtifactStorageConfig{
				Provider:        "cloudflare-r2",
				AccountID:       "acc123",
				Bucket:          "bucket",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
			wantProvider:  "r2",
			wantEndpoint:  "https://acc123.r2.cloudflarestorage.com",
			wantRegion:    "auto",
			wantPathStyle: true,
		},
		{
			name: "tencent cos",
			cfg: config.AgentArtifactStorageConfig{
				Provider:        "tencent-cos",
				Region:          "ap-hongkong",
				Bucket:          "bucket-123",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
			wantProvider: "cos",
			wantEndpoint: "https://cos.ap-hongkong.myqcloud.com",
			wantRegion:   "ap-hongkong",
		},
		{
			name: "minio requires endpoint",
			cfg: config.AgentArtifactStorageConfig{
				Provider:        "minio",
				Bucket:          "bucket",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
			wantErrContain: "endpoint is required",
		},
		{
			name: "minio custom endpoint",
			cfg: config.AgentArtifactStorageConfig{
				Provider:        "minio",
				Endpoint:        "https://minio.example.com/",
				Bucket:          "bucket",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
			wantProvider:  "minio",
			wantEndpoint:  "https://minio.example.com",
			wantRegion:    "auto",
			wantPathStyle: true,
		},
		{
			name: "unknown provider requires endpoint",
			cfg: config.AgentArtifactStorageConfig{
				Provider:        "future-store",
				Bucket:          "bucket",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
			wantErrContain: "endpoint is required",
		},
		{
			name: "unknown provider with endpoint",
			cfg: config.AgentArtifactStorageConfig{
				Provider:         "future-store",
				Endpoint:         "https://object.future.example",
				Bucket:           "bucket",
				AccessKeyID:      "ak",
				SecretAccessKey:  "sk",
				VirtualHostStyle: true,
			},
			wantProvider: "future-store",
			wantEndpoint: "https://object.future.example",
			wantRegion:   "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := resolveAgentArtifactStorageSettings(tt.cfg)
			if tt.wantErrContain != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContain)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantProvider, settings.provider)
			require.Equal(t, tt.wantEndpoint, settings.endpoint)
			require.Equal(t, tt.wantRegion, settings.region)
			require.Equal(t, tt.wantPathStyle, settings.forcePathStyle)
		})
	}
}
