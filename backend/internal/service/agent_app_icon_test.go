package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type agentAppIconTestStore struct {
	location AgentArtifactObjectLocation
}

func (s *agentAppIconTestStore) IsConfigured() bool { return true }
func (s *agentAppIconTestStore) Provider() string   { return "cos" }
func (s *agentAppIconTestStore) Bucket() string     { return "icons-1234567890" }
func (s *agentAppIconTestStore) Put(context.Context, AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	return &AgentArtifactStorePutResult{
		Provider:  "cos",
		Bucket:    "icons-1234567890",
		ObjectKey: "agent-artifacts/app-icons/icon.png",
		ObjectURL: "https://cdn.example.com/agent-artifacts/app-icons/icon.png",
		SizeBytes: 3,
	}, nil
}
func (s *agentAppIconTestStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "https://signed.example.com/icon.png", nil
}
func (s *agentAppIconTestStore) PresignGetObject(_ context.Context, location AgentArtifactObjectLocation, _ time.Duration) (string, error) {
	s.location = location
	return "https://signed.example.com/icon.png", nil
}
func (s *agentAppIconTestStore) Delete(context.Context, string) error { return nil }
func (s *agentAppIconTestStore) DeleteObject(context.Context, AgentArtifactObjectLocation) error {
	return nil
}

func TestResolveAgentAppIconURLUsesStoredProviderAndBucket(t *testing.T) {
	store := &agentAppIconTestStore{}
	result, err := resolveAgentAppIconURL(context.Background(), store, &AgentApp{
		ID:      12,
		IconURL: "cos://icons-1234567890/agent-artifacts/app-icons/icon.png",
	}, time.Hour)

	require.NoError(t, err)
	require.Equal(t, int64(12), result.AppID)
	require.Equal(t, "https://signed.example.com/icon.png", result.URL)
	require.Equal(t, AgentArtifactObjectLocation{
		StorageProvider: "cos",
		Bucket:          "icons-1234567890",
		ObjectKey:       "agent-artifacts/app-icons/icon.png",
	}, store.location)
}

func TestUploadAgentAppIconReturnsCanonicalStorageReference(t *testing.T) {
	store := &agentAppIconTestStore{}
	service := NewAgentAppService(nil, nil, nil, store)
	result, err := service.UploadIcon(context.Background(), UploadAgentAppIconInput{
		FileName:    "icon.png",
		ContentType: "image/png",
		SizeBytes:   3,
		Body:        bytes.NewReader([]byte("png")),
	})

	require.NoError(t, err)
	require.Equal(t, "cos://icons-1234567890/agent-artifacts/app-icons/icon.png", result.URL)
	require.Equal(t, "https://signed.example.com/icon.png", result.PreviewURL)
}
