//go:build unit

package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestCreazyCanvasPlaybackStreamsRangeWithoutArchiveWait(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/videos/jobs/job-41/content", r.URL.Path)
		require.Equal(t, "1", r.URL.Query().Get("canvas_playback"))
		require.Equal(t, "Bearer sk-canvas", r.Header.Get("Authorization"))
		require.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("test"))
	}))
	defer upstream.Close()

	svc := newCreazyCanvasPlaybackTestService(t, upstream.URL)
	playback, err := svc.GetPlaybackURL(context.Background(), 7, 41)
	require.NoError(t, err)
	require.Equal(t, "playback", playback.Source)
	parsed, err := url.Parse(playback.URL)
	require.NoError(t, err)
	require.Equal(t, "/creazy-canvas/works/41/playback", parsed.Path)
	require.NotEmpty(t, parsed.Query().Get("token"))

	content, err := svc.OpenPlayback(context.Background(), 41, parsed.Query().Get("token"), "bytes=0-3")
	require.NoError(t, err)
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.NoError(t, content.Body.Close())
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "video/mp4", content.ContentType)
	require.Equal(t, int64(4), content.ContentLength)
	require.Equal(t, "bytes 0-3/10", content.Header.Get("Content-Range"))
	require.Equal(t, []byte("test"), body)
	require.Empty(t, svc.playbackStreams)
}

func TestCreazyCanvasPlaybackArchivesSucceededVideoBeforeServing(t *testing.T) {
	payload := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/videos/jobs/job-41/content", r.URL.Path)
		require.Empty(t, r.URL.Query().Get("canvas_playback"))
		require.Empty(t, r.Header.Get("Range"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	store := &creazyCanvasArchiveStore{}
	svc := newCreazyCanvasPlaybackTestService(t, upstream.URL)
	svc.artifactStore = store
	content, err := svc.openWorkContent(context.Background(), 7, 41, "bytes=0-3", true)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, content.StatusCode)
	require.Equal(t, "https://signed.example.com/result.png", content.RedirectURL)
	require.Equal(t, 1, store.putCalls)
	require.Equal(t, "video/mp4", store.putContentType)
	require.Equal(t, payload, store.putBody)
	require.Equal(t, "tenant/creazy-canvas/7/video/1/result.mp4", svc.workRepo.(*creazyCanvasWorkRepoStub).works[41].ObjectKey)
}

func TestCreazyCanvasPlaybackRejectsTamperingExpiryAndInvalidRange(t *testing.T) {
	svc := newCreazyCanvasPlaybackTestService(t, "http://127.0.0.1:1")
	valid, err := svc.signCreazyCanvasPlaybackToken(41, 7, time.Now().Add(time.Minute))
	require.NoError(t, err)

	_, err = svc.OpenPlayback(context.Background(), 41, valid+"x", "")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))

	expired, err := svc.signCreazyCanvasPlaybackToken(41, 7, time.Now().Add(-time.Second))
	require.NoError(t, err)
	_, err = svc.OpenPlayback(context.Background(), 41, expired, "")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))

	_, err = svc.OpenPlayback(context.Background(), 41, valid, "bytes=0-1,4-5")
	require.Error(t, err)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, infraerrors.Code(err))
}

func TestCreazyCanvasPlaybackRejectsDifferentWorkAndNonVideo(t *testing.T) {
	svc := newCreazyCanvasPlaybackTestService(t, "http://127.0.0.1:1")
	token, err := svc.signCreazyCanvasPlaybackToken(41, 7, time.Now().Add(time.Minute))
	require.NoError(t, err)

	_, err = svc.OpenPlayback(context.Background(), 42, token, "")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))

	work := svc.workRepo.(*creazyCanvasWorkRepoStub).works[41]
	work.Kind = CreazyCanvasWorkKindImage
	_, err = svc.GetPlaybackURL(context.Background(), 7, 41)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
}

func TestCreazyCanvasPlaybackAllowsAcceptedAPIVideoWithoutCanvasFlag(t *testing.T) {
	svc := newCreazyCanvasPlaybackTestService(t, "http://127.0.0.1:1")
	work := svc.workRepo.(*creazyCanvasWorkRepoStub).works[41]
	work.ParamsJSON = map[string]any{"source": "api"}
	key := svc.apiKeyService.(*creazyCanvasAPIKeyStub).keys[9]
	key.Group.AllowCreazyCanvas = false

	loaded, err := svc.loadCanvasAPIKeyForContent(context.Background(), 7, work)
	require.NoError(t, err)
	require.Equal(t, key.ID, loaded.ID)
	require.False(t, loaded.Group.AllowCreazyCanvas)
}

func newCreazyCanvasPlaybackTestService(t *testing.T, gatewayURL string) *CreazyCanvasService {
	t.Helper()
	parsed, err := url.Parse(gatewayURL)
	require.NoError(t, err)
	host, portRaw, err := net.SplitHostPort(parsed.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portRaw)
	require.NoError(t, err)

	groupID := int64(5)
	repo := newCreazyCanvasWorkRepoStub()
	repo.works[41] = &CreazyCanvasWork{
		ID:              41,
		UserID:          7,
		APIKeyID:        9,
		GroupID:         &groupID,
		Kind:            CreazyCanvasWorkKindVideo,
		Status:          CreazyCanvasWorkStatusSucceeded,
		GatewayType:     CreazyCanvasGatewayVideoJob,
		GatewayRemoteID: "job-41",
		ObjectURL:       "https://private.example.test/video.mp4",
		MimeType:        "video/mp4",
		ExpiresAt:       time.Now().Add(time.Hour),
	}
	keys := &creazyCanvasAPIKeyStub{keys: map[int64]*APIKey{
		9: {
			ID:      9,
			UserID:  7,
			Key:     "sk-canvas",
			Status:  StatusAPIKeyActive,
			GroupID: &groupID,
			Group: &Group{
				ID:                groupID,
				Platform:          PlatformSeedance,
				AllowCreazyCanvas: true,
			},
		},
	}}
	return NewCreazyCanvasService(repo, keys, disabledAgentArtifactStore{}, &config.Config{
		Server: config.ServerConfig{Host: strings.Trim(host, "[]"), Port: port},
		JWT:    config.JWTConfig{Secret: strings.Repeat("s", 32)},
	})
}
