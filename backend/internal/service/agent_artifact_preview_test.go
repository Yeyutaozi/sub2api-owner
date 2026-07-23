package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type artifactPreviewTestRepo struct {
	*testAgentRunRepo
	artifact *AgentArtifact
}

func (r *artifactPreviewTestRepo) GetArtifactByID(_ context.Context, artifactID int64) (*AgentArtifact, error) {
	if r.artifact == nil || r.artifact.ID != artifactID {
		return nil, ErrAgentArtifactNotFound
	}
	copy := *r.artifact
	return &copy, nil
}

type artifactPreviewTestStore struct {
	*testAgentArtifactStore
	url string
}

func (s *artifactPreviewTestStore) PresignGetObject(context.Context, AgentArtifactObjectLocation, time.Duration) (string, error) {
	return s.url, nil
}

func TestArtifactPreviewStreamsRangeInline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		require.Equal(t, "identity", r.Header.Get("Accept-Encoding"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("x-amz-force-download", "true")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.Header().Set("ETag", `"preview-etag"`)
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("test"))
	}))
	defer upstream.Close()

	repo := &artifactPreviewTestRepo{
		testAgentRunRepo: &testAgentRunRepo{},
		artifact: &AgentArtifact{
			ID:              41,
			UserID:          7,
			Name:            "grok-video-41.mp4",
			MimeType:        "video/mp4",
			StorageProvider: "cos",
			Bucket:          "artifact-bucket",
			ObjectKey:       "agent-artifacts/7/41/grok-video-41.mp4",
			ObjectURL:       "cos://artifact-bucket/agent-artifacts/7/41/grok-video-41.mp4",
		},
	}
	store := &artifactPreviewTestStore{
		testAgentArtifactStore: &testAgentArtifactStore{provider: "cos", bucket: "artifact-bucket"},
		url:                    upstream.URL,
	}
	service := &AgentRunService{
		runRepo:       repo,
		artifactStore: store,
		cfg: &config.Config{JWT: config.JWTConfig{
			Secret: strings.Repeat("s", 32),
		}},
		previewClient: upstream.Client(),
	}

	preview, err := service.GetArtifactPreviewURL(context.Background(), 41, 7)
	require.NoError(t, err)
	parsed, err := url.Parse(preview.URL)
	require.NoError(t, err)
	require.Equal(t, "/agent-artifacts/41/content", parsed.Path)
	require.NotEmpty(t, parsed.Query().Get("token"))

	content, err := service.OpenArtifactPreview(context.Background(), 41, parsed.Query().Get("token"), "bytes=0-3")
	require.NoError(t, err)
	defer func() { _ = content.Body.Close() }()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "video/mp4", content.ContentType)
	require.Equal(t, int64(4), content.ContentLength)
	require.Equal(t, "bytes 0-3/10", content.ContentRange)
	require.Equal(t, "bytes", content.AcceptRanges)
	require.Equal(t, `"preview-etag"`, content.ETag)
	require.Equal(t, []byte("test"), body)
}

func TestArtifactPreviewURLUsesShortUniqueTokensForManagedMedia(t *testing.T) {
	repo := &artifactPreviewTestRepo{
		testAgentRunRepo: &testAgentRunRepo{},
		artifact: &AgentArtifact{
			ID:              41,
			UserID:          7,
			Name:            "grok-video-41.mp4",
			MimeType:        "video/mp4",
			StorageProvider: "cos",
			Bucket:          "artifact-bucket",
			ObjectKey:       "agent-artifacts/7/41/grok-video-41.mp4",
			ObjectURL:       "https://cdn.example.com/grok-video-41.mp4",
		},
	}
	service := &AgentRunService{
		runRepo:       repo,
		artifactStore: &artifactPreviewTestStore{testAgentArtifactStore: &testAgentArtifactStore{provider: "cos", bucket: "artifact-bucket"}},
		cfg: &config.Config{JWT: config.JWTConfig{
			Secret: strings.Repeat("s", 32),
		}},
	}

	before := time.Now().UTC()
	first, err := service.GetArtifactPreviewURL(context.Background(), 41, 7)
	require.NoError(t, err)
	second, err := service.GetArtifactPreviewURL(context.Background(), 41, 7)
	require.NoError(t, err)

	firstURL, err := url.Parse(first.URL)
	require.NoError(t, err)
	secondURL, err := url.Parse(second.URL)
	require.NoError(t, err)
	require.Equal(t, "/agent-artifacts/41/content", firstURL.Path)
	require.NotEqual(t, firstURL.Query().Get("token"), secondURL.Query().Get("token"))

	expiresAt, err := time.Parse(time.RFC3339, first.ExpiresAt)
	require.NoError(t, err)
	require.WithinDuration(t, before.Add(artifactPreviewTokenTTL), expiresAt, 2*time.Second)
	require.LessOrEqual(t, expiresAt.Sub(before), artifactPreviewTokenTTL)
}

func TestArtifactPreviewRejectsUnauthorizedExpiredDeletedAndUnsupportedArtifacts(t *testing.T) {
	now := time.Now().UTC()
	base := AgentArtifact{
		ID:              41,
		UserID:          7,
		Name:            "grok-video-41.mp4",
		MimeType:        "video/mp4",
		StorageProvider: "cos",
		Bucket:          "artifact-bucket",
		ObjectKey:       "agent-artifacts/7/41/grok-video-41.mp4",
	}
	tests := []struct {
		name       string
		userID     int64
		mutate     func(*AgentArtifact)
		statusCode int
	}{
		{name: "different owner", userID: 8, statusCode: http.StatusNotFound},
		{name: "deleted", userID: 7, mutate: func(a *AgentArtifact) { a.DeletedAt = &now }, statusCode: http.StatusNotFound},
		{name: "expired", userID: 7, mutate: func(a *AgentArtifact) { expired := now.Add(-time.Minute); a.ExpiresAt = &expired }, statusCode: http.StatusBadRequest},
		{name: "unsupported type", userID: 7, mutate: func(a *AgentArtifact) { a.Name = "report.html"; a.MimeType = "text/html" }, statusCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := base
			if tt.mutate != nil {
				tt.mutate(&artifact)
			}
			service := &AgentRunService{
				runRepo: &artifactPreviewTestRepo{testAgentRunRepo: &testAgentRunRepo{}, artifact: &artifact},
				artifactStore: &artifactPreviewTestStore{
					testAgentArtifactStore: &testAgentArtifactStore{provider: "cos", bucket: "artifact-bucket"},
				},
				cfg: &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("s", 32)}},
			}
			_, err := service.GetArtifactPreviewURL(context.Background(), 41, tt.userID)
			require.Error(t, err)
			require.Equal(t, tt.statusCode, infraerrors.Code(err))
		})
	}
}

func TestArtifactPreviewForwardsUnsatisfiedRange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "bytes=100-", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes */10")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer upstream.Close()

	service := newArtifactPreviewTestService(upstream.URL, upstream.Client())
	token, err := service.signArtifactPreviewToken(41, 7, time.Now().Add(time.Minute))
	require.NoError(t, err)
	content, err := service.OpenArtifactPreview(context.Background(), 41, token, "bytes=100-")
	require.NoError(t, err)
	defer func() { _ = content.Body.Close() }()
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, content.StatusCode)
	require.Equal(t, "bytes */10", content.ContentRange)
}

func TestArtifactPreviewLimitsConcurrentStreamsPerUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "1")
		_, _ = w.Write([]byte("x"))
	}))
	defer upstream.Close()

	service := newArtifactPreviewTestService(upstream.URL, upstream.Client())
	token, err := service.signArtifactPreviewToken(41, 7, time.Now().Add(time.Minute))
	require.NoError(t, err)

	contents := make([]*ArtifactPreviewContent, 0, artifactPreviewMaxStreamsPerUser)
	for range artifactPreviewMaxStreamsPerUser {
		content, openErr := service.OpenArtifactPreview(context.Background(), 41, token, "")
		require.NoError(t, openErr)
		contents = append(contents, content)
	}
	_, err = service.OpenArtifactPreview(context.Background(), 41, token, "")
	require.Error(t, err)
	require.Equal(t, http.StatusTooManyRequests, infraerrors.Code(err))

	require.NoError(t, contents[0].Body.Close())
	replacement, err := service.OpenArtifactPreview(context.Background(), 41, token, "")
	require.NoError(t, err)
	require.NoError(t, replacement.Body.Close())
	for _, content := range contents[1:] {
		require.NoError(t, content.Body.Close())
	}
	require.Empty(t, service.previewActiveStreams)
}

func TestArtifactPreviewReadCloserCancelsIdleStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	released := make(chan struct{})
	closer := newArtifactPreviewReadCloser(
		io.NopCloser(strings.NewReader("")),
		20*time.Millisecond,
		cancel,
		func() { close(released) },
	)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("idle preview stream was not canceled")
	}
	require.NoError(t, closer.Close())
	select {
	case <-released:
	default:
		t.Fatal("preview stream slot was not released")
	}
}

func TestArtifactPreviewTokenRejectsTamperingAndExpiry(t *testing.T) {
	service := &AgentRunService{cfg: &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("s", 32)}}}
	token, err := service.signArtifactPreviewToken(41, 7, time.Now().Add(time.Minute))
	require.NoError(t, err)

	claims, err := service.verifyArtifactPreviewToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(41), claims.ArtifactID)
	require.Equal(t, int64(7), claims.UserID)

	_, err = service.verifyArtifactPreviewToken(token + "x")
	require.Error(t, err)
	expired, err := service.signArtifactPreviewToken(41, 7, time.Now().Add(-time.Second))
	require.NoError(t, err)
	_, err = service.verifyArtifactPreviewToken(expired)
	require.Error(t, err)
}

func TestNormalizeArtifactPreviewRange(t *testing.T) {
	for _, value := range []string{"", "bytes=0-", "bytes=0-1023", "bytes=-1024"} {
		normalized, err := normalizeArtifactPreviewRange(value)
		require.NoError(t, err)
		require.Equal(t, value, normalized)
	}
	for _, value := range []string{"items=0-1", "bytes=-", "bytes=0-1,4-5", "bytes=abc-def"} {
		_, err := normalizeArtifactPreviewRange(value)
		require.Error(t, err)
	}
}

func newArtifactPreviewTestService(upstreamURL string, client *http.Client) *AgentRunService {
	return &AgentRunService{
		runRepo: &artifactPreviewTestRepo{
			testAgentRunRepo: &testAgentRunRepo{},
			artifact: &AgentArtifact{
				ID:              41,
				UserID:          7,
				Name:            "grok-video-41.mp4",
				MimeType:        "video/mp4",
				StorageProvider: "cos",
				Bucket:          "artifact-bucket",
				ObjectKey:       "agent-artifacts/7/41/grok-video-41.mp4",
			},
		},
		artifactStore: &artifactPreviewTestStore{
			testAgentArtifactStore: &testAgentArtifactStore{provider: "cos", bucket: "artifact-bucket"},
			url:                    upstreamURL,
		},
		cfg:           &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("s", 32)}},
		previewClient: client,
	}
}
