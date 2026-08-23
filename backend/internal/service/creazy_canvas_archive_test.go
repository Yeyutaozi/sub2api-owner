//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type creazyCanvasArchiveRoundTripFunc func(*http.Request) (*http.Response, error)

func (f creazyCanvasArchiveRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type creazyCanvasArchiveStore struct {
	provider       string
	putErr         error
	putCalls       int
	putKey         string
	putContentType string
	putSizeBytes   int64
	putMetadata    map[string]string
	putBody        []byte
	deleteCalls    int
}

func (s *creazyCanvasArchiveStore) IsConfigured() bool { return true }
func (s *creazyCanvasArchiveStore) Provider() string {
	if s.provider != "" {
		return s.provider
	}
	return "cos"
}
func (s *creazyCanvasArchiveStore) Bucket() string {
	if s.Provider() == "local" {
		return "local-media"
	}
	return "canvas-bucket"
}

func (s *creazyCanvasArchiveStore) Put(_ context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	s.putCalls++
	s.putKey = input.Key
	s.putContentType = input.ContentType
	s.putSizeBytes = input.SizeBytes
	s.putMetadata = input.Metadata
	payload, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	s.putBody = payload
	if s.putErr != nil {
		return nil, s.putErr
	}
	objectKey := "tenant/creazy-canvas/7/image/1/result.png"
	objectURL := "https://canvas-bucket.example.com/tenant/creazy-canvas/7/image/1/result.png"
	if input.ContentType == "video/mp4" {
		objectKey = "tenant/creazy-canvas/7/video/1/result.mp4"
		objectURL = "https://canvas-bucket.example.com/tenant/creazy-canvas/7/video/1/result.mp4"
	}
	return &AgentArtifactStorePutResult{
		Provider:  s.Provider(),
		Bucket:    s.Bucket(),
		ObjectKey: objectKey,
		ObjectURL: objectURL,
		SizeBytes: int64(len(payload)),
	}, nil
}

func TestCreazyCanvasVideoArchiveFollowsRedirectedDownload(t *testing.T) {
	payload := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 'd', 'a', 't', 'a'}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://cdn.example.com/result.mp4")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	store := &creazyCanvasArchiveStore{provider: "local"}
	svc := newCreazyCanvasPlaybackTestService(t, upstream.URL)
	svc.artifactStore = store
	svc.mediaHTTPClient.Transport = creazyCanvasArchiveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "cdn.example.com", req.URL.Hostname())
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"video/mp4"}},
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Request:       req,
		}, nil
	})

	playback, err := svc.GetPlaybackURL(context.Background(), 7, 41)
	require.NoError(t, err)
	require.Equal(t, "object", playback.Source)
	require.Equal(t, payload, store.putBody)
	require.Equal(t, 1, store.putCalls)
}

func TestCreazyCanvasVideoArchiveRetriesTransientGatewayFailure(t *testing.T) {
	payload := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	store := &creazyCanvasArchiveStore{provider: "local"}
	svc := newCreazyCanvasPlaybackTestService(t, upstream.URL)
	svc.artifactStore = store
	playback, err := svc.GetPlaybackURL(context.Background(), 7, 41)
	require.NoError(t, err)
	require.Equal(t, "object", playback.Source)
	require.Equal(t, 2, requests)
	require.Equal(t, 1, store.putCalls)
}

func TestCreazyCanvasPlaybackFallsBackWhenLocalArchiveFails(t *testing.T) {
	payload := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	svc := newCreazyCanvasPlaybackTestService(t, upstream.URL)
	svc.artifactStore = &creazyCanvasArchiveStore{provider: "local", putErr: errors.New("no space left on device")}
	playback, err := svc.GetPlaybackURL(context.Background(), 7, 41)
	require.NoError(t, err)
	require.Equal(t, "playback", playback.Source)
	require.Contains(t, playback.URL, "/creazy-canvas/works/41/playback")
}

func TestCreazyCanvasCleanupExpiredLocalContent(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	repo.works[41] = &CreazyCanvasWork{
		ID: 41, UserID: 7, Status: CreazyCanvasWorkStatusSucceeded,
		ObjectKey: "media/creazy-canvas/7/video/41/result.mp4", StorageProvider: "local", Bucket: "local-media",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	store := &creazyCanvasArchiveStore{provider: "local"}
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, store, nil)
	svc.cleanupExpiredLocalContent(context.Background())

	require.Equal(t, 1, store.deleteCalls)
	require.Equal(t, CreazyCanvasWorkStatusExpired, repo.works[41].Status)
	require.Empty(t, repo.works[41].ObjectKey)
	require.Zero(t, repo.works[41].SizeBytes)
}

func (s *creazyCanvasArchiveStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "https://signed.example.com/result.png", nil
}

func (s *creazyCanvasArchiveStore) PresignGetObject(_ context.Context, _ AgentArtifactObjectLocation, _ time.Duration) (string, error) {
	return "https://signed.example.com/result.png", nil
}

func (s *creazyCanvasArchiveStore) Delete(context.Context, string) error {
	s.deleteCalls++
	return nil
}

func (s *creazyCanvasArchiveStore) DeleteObject(context.Context, AgentArtifactObjectLocation) error {
	s.deleteCalls++
	return nil
}

func creazyCanvasArchiveTestService(store *creazyCanvasArchiveStore) (*CreazyCanvasService, *creazyCanvasWorkRepoStub) {
	groupID := int64(9)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformGrok,
		AllowCreazyCanvas:    true,
		AllowImageGeneration: true,
	}
	keys := map[int64]*APIKey{
		31: {ID: 31, UserID: 7, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	repo := newCreazyCanvasWorkRepoStub()
	return NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{keys: keys}, store, nil), repo
}

func creazyCanvasArchivePNG(t *testing.T) []byte {
	t.Helper()
	var payload bytes.Buffer
	require.NoError(t, png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return payload.Bytes()
}

func TestCreazyCanvasArchivesSucceededImageOnCreate(t *testing.T) {
	payload := creazyCanvasArchivePNG(t)
	store := &creazyCanvasArchiveStore{}
	service, repo := creazyCanvasArchiveTestService(store)
	service.mediaHTTPClient.Transport = creazyCanvasArchiveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "images.example.com", req.URL.Hostname())
		require.Equal(t, "identity", req.Header.Get("Accept-Encoding"))
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Request:       req,
		}, nil
	})

	work, err := service.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:      7,
		APIKeyID:    31,
		Kind:        CreazyCanvasWorkKindImage,
		Status:      CreazyCanvasWorkStatusSucceeded,
		PreviewURL:  "https://images.example.com/generated.png",
		PublicModel: "gpt-image-2",
	})
	require.NoError(t, err)
	require.Equal(t, CreazyCanvasWorkStatusSucceeded, work.Status)
	require.Equal(t, 1, store.putCalls)
	require.Equal(t, payload, store.putBody)
	require.Equal(t, "image/png", store.putContentType)
	require.Equal(t, int64(len(payload)), store.putSizeBytes)
	require.Equal(t, "1", store.putMetadata["creazy-canvas-work-id"])
	require.Equal(t, "7", store.putMetadata["creazy-canvas-user-id"])
	require.True(t, strings.HasPrefix(store.putKey, "creazy-canvas/7/image/1/"))
	require.True(t, strings.HasSuffix(store.putKey, "-result.png"))
	require.Equal(t, "tenant/creazy-canvas/7/image/1/result.png", work.ObjectKey)
	require.Equal(t, "cos", work.StorageProvider)
	require.Equal(t, "canvas-bucket", work.Bucket)
	require.Equal(t, "image/png", work.MimeType)
	require.Equal(t, int64(len(payload)), work.SizeBytes)
	require.Equal(t, work.ObjectURL, work.PreviewURL)
	require.Equal(t, work.ObjectKey, repo.works[work.ID].ObjectKey)
	require.Equal(t, work.PreviewURL, repo.works[work.ID].PreviewURL)
}

func TestCreazyCanvasArchivesImageWhenUpdateSucceeds(t *testing.T) {
	payload := creazyCanvasArchivePNG(t)
	store := &creazyCanvasArchiveStore{}
	service, _ := creazyCanvasArchiveTestService(store)
	service.mediaHTTPClient.Transport = creazyCanvasArchiveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Request:    req,
		}, nil
	})

	work, err := service.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:   7,
		APIKeyID: 31,
		Kind:     CreazyCanvasWorkKindImage,
		Status:   CreazyCanvasWorkStatusRunning,
	})
	require.NoError(t, err)
	require.Zero(t, store.putCalls)

	status := CreazyCanvasWorkStatusSucceeded
	previewURL := "https://images.example.com/updated.png"
	work, err = service.UpdateWork(context.Background(), UpdateCreazyCanvasWorkInput{
		UserID:     7,
		WorkID:     work.ID,
		Status:     &status,
		PreviewURL: &previewURL,
	})
	require.NoError(t, err)
	require.Equal(t, CreazyCanvasWorkStatusSucceeded, work.Status)
	require.Equal(t, 1, store.putCalls)
	require.NotEmpty(t, work.ObjectKey)
	require.Equal(t, "image/png", work.MimeType)
}

func TestCreazyCanvasImageArchiveFailureDoesNotFailGeneratedWork(t *testing.T) {
	payload := creazyCanvasArchivePNG(t)
	store := &creazyCanvasArchiveStore{putErr: errors.New("storage unavailable")}
	service, repo := creazyCanvasArchiveTestService(store)
	service.mediaHTTPClient.Transport = creazyCanvasArchiveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Request:    req,
		}, nil
	})

	work, err := service.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:     7,
		APIKeyID:   31,
		Kind:       CreazyCanvasWorkKindImage,
		Status:     CreazyCanvasWorkStatusSucceeded,
		PreviewURL: "https://images.example.com/generated.png",
	})
	require.NoError(t, err)
	require.Equal(t, CreazyCanvasWorkStatusSucceeded, work.Status)
	require.Empty(t, work.ObjectKey)
	require.Equal(t, "https://images.example.com/generated.png", work.PreviewURL)
	require.Equal(t, CreazyCanvasWorkStatusSucceeded, repo.works[work.ID].Status)
	require.Empty(t, repo.works[work.ID].ObjectKey)
}

func TestCreazyCanvasImageArchiveRejectsUnsafeOrInvalidSources(t *testing.T) {
	pngPayload := creazyCanvasArchivePNG(t)
	tests := []struct {
		name      string
		sourceURL string
		transport creazyCanvasArchiveRoundTripFunc
	}{
		{
			name:      "loopback source",
			sourceURL: "http://127.0.0.1/private.png",
			transport: func(*http.Request) (*http.Response, error) {
				t.Fatal("blocked source must not reach the transport")
				return nil, nil
			},
		},
		{
			name:      "redirect to private address",
			sourceURL: "https://images.example.com/redirect.png",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"http://127.0.0.1/private.png"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			},
		},
		{
			name:      "declared oversized response",
			sourceURL: "https://images.example.com/large.png",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"image/png"}},
					Body:          io.NopCloser(bytes.NewReader(pngPayload)),
					ContentLength: SeedanceMaxImageBytes + 1,
					Request:       req,
				}, nil
			},
		},
		{
			name:      "non-image body",
			sourceURL: "https://images.example.com/not-image.png",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/png"}},
					Body:       io.NopCloser(strings.NewReader("<html>not an image</html>")),
					Request:    req,
				}, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &creazyCanvasArchiveStore{}
			service, repo := creazyCanvasArchiveTestService(store)
			service.mediaHTTPClient.Transport = test.transport

			work, err := service.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
				UserID:     7,
				APIKeyID:   31,
				Kind:       CreazyCanvasWorkKindImage,
				Status:     CreazyCanvasWorkStatusSucceeded,
				PreviewURL: test.sourceURL,
			})
			require.NoError(t, err)
			require.Equal(t, CreazyCanvasWorkStatusSucceeded, work.Status)
			require.Empty(t, work.ObjectKey)
			require.Zero(t, store.putCalls)
			require.Equal(t, CreazyCanvasWorkStatusSucceeded, repo.works[work.ID].Status)
		})
	}
}
