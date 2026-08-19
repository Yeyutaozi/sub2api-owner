package service

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAgentLocalArtifactStorePersistsSignsAndRanges(t *testing.T) {
	store := newAgentLocalArtifactStore(config.AgentArtifactStorageConfig{
		Provider: "local", Endpoint: t.TempDir(), Prefix: "media",
		PublicBaseURL: "https://example.com/api/v1/local-media", SecretAccessKey: "test-signing-secret",
	})
	require.True(t, store.IsConfigured())
	put, err := store.Put(context.Background(), AgentArtifactStorePutInput{Key: "canvas/result.mp4", Body: bytes.NewReader([]byte("0123456789")), SizeBytes: 10})
	require.NoError(t, err)
	require.Equal(t, "local", put.Provider)
	signed, err := store.PresignGetObject(context.Background(), AgentArtifactObjectLocation{ObjectKey: put.ObjectKey}, time.Hour)
	require.NoError(t, err)
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	require.NoError(t, err)
	reader := store.(AgentArtifactSignedReader)
	result, err := reader.OpenSignedObject(context.Background(), parsed.Query().Get("key"), expires, parsed.Query().Get("signature"), "bytes=2-5")
	require.NoError(t, err)
	defer result.Body.Close()
	require.Equal(t, 206, result.StatusCode)
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.Equal(t, "2345", string(body))
}
