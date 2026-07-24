package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSeedanceUploadImageAcceptsJSONBase64(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	store := &seedanceHandlerMediaStore{configured: true}
	handler := &OpenAIGatewayHandler{
		seedanceMediaService: service.NewSeedanceMediaService(store, nil, redisClient),
	}

	body := `{"image_base64":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z5YQAAAAASUVORK5CYII=","content_type":"image/png","filename":"reference.png"}`
	c, recorder := newSeedanceMediaHandlerContext(t, http.MethodPost, service.SeedanceOfficialUploadsEndpoint, "application/json", strings.NewReader(body), service.PlatformSeedance)
	handler.SeedanceUploadImage(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		UploadID    string `json:"upload_id"`
		ImageURL    string `json:"image_url"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		SHA256      string `json:"sha256"`
		ExpiresAt   string `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, strings.HasPrefix(response.UploadID, "sdupl_"))
	require.Equal(t, "image/png", response.ContentType)
	require.Positive(t, response.Size)
	require.Len(t, response.SHA256, 64)
	require.NotEmpty(t, response.ExpiresAt)
	require.Equal(t, "https://gateway.example.com"+service.SeedanceOfficialUploadsEndpoint+"/"+response.UploadID, response.ImageURL)
	require.Equal(t, 1, store.putCount())
	require.True(t, strings.HasPrefix(store.lastKey(), "seedance/inputs/staged/"))
}

func TestSeedanceUploadImageRequiresMultipartFieldImage(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	store := &seedanceHandlerMediaStore{configured: true}
	handler := &OpenAIGatewayHandler{
		seedanceMediaService: service.NewSeedanceMediaService(store, nil, redisClient),
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("not read because the field name is wrong"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, recorder := newSeedanceMediaHandlerContext(t, http.MethodPost, service.SeedanceOfficialUploadsEndpoint, writer.FormDataContentType(), &body, service.PlatformSeedance)
	handler.SeedanceUploadImage(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "image_required")
	require.Equal(t, 0, store.putCount())
}

func TestSeedanceUploadRejectsOtherPlatformsBeforeStorageAccess(t *testing.T) {
	store := &seedanceHandlerMediaStore{configured: true}
	handler := &OpenAIGatewayHandler{
		seedanceMediaService: service.NewSeedanceMediaService(store, nil, nil),
	}
	c, recorder := newSeedanceMediaHandlerContext(
		t,
		http.MethodPost,
		service.SeedanceOfficialUploadsEndpoint,
		"application/json",
		strings.NewReader(`{"image_base64":"unused","content_type":"image/png"}`),
		service.PlatformOpenAI,
	)

	handler.SeedanceUploadImage(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "permission_denied")
	require.Equal(t, 0, store.configuredChecks())
	require.Equal(t, 0, store.putCount())
}

func TestServeSeedanceCapturedVideoHonorsRange(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "seedance-range-*.mp4")
	require.NoError(t, err)
	video := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	_, err = file.Write(video)
	require.NoError(t, err)
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, service.SeedanceOfficialTasksEndpoint+"/task/content", nil)
	c.Request.Header.Set("Range", "bytes=4-7")
	handler := &OpenAIGatewayHandler{}
	handler.serveSeedanceCapturedVideo(c, &service.SeedanceCapturedVideo{
		File:        file,
		SizeBytes:   int64(len(video)),
		ContentType: "video/mp4",
	})

	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "bytes 4-7/12", recorder.Header().Get("Content-Range"))
	require.Equal(t, "ftyp", recorder.Body.String())
}

func newSeedanceMediaHandlerContext(
	t *testing.T,
	method string,
	path string,
	contentType string,
	body io.Reader,
	platform string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "https://gateway.example.com"+path, body)
	c.Request.Host = "gateway.example.com"
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	groupID := int64(303)
	group := &service.Group{ID: groupID, Platform: platform, AllowImageGeneration: true}
	apiKey := &service.APIKey{ID: 202, UserID: 101, GroupID: &groupID, Group: group}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 101})
	return c, recorder
}

type seedanceHandlerMediaStore struct {
	mu                sync.Mutex
	configured        bool
	isConfiguredCalls int
	puts              []service.AgentArtifactStorePutInput
}

func (s *seedanceHandlerMediaStore) IsConfigured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isConfiguredCalls++
	return s.configured
}

func (s *seedanceHandlerMediaStore) Put(_ context.Context, input service.AgentArtifactStorePutInput) (*service.AgentArtifactStorePutResult, error) {
	_, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.puts = append(s.puts, input)
	s.mu.Unlock()
	return &service.AgentArtifactStorePutResult{Provider: "cos", Bucket: "seedance-test", ObjectKey: input.Key, SizeBytes: input.SizeBytes}, nil
}

func (s *seedanceHandlerMediaStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "https://cos.example.com/object", nil
}

func (s *seedanceHandlerMediaStore) PresignGetObject(context.Context, service.AgentArtifactObjectLocation, time.Duration) (string, error) {
	return "https://cos.example.com/object", nil
}

func (s *seedanceHandlerMediaStore) Delete(context.Context, string) error { return nil }

func (s *seedanceHandlerMediaStore) DeleteObject(context.Context, service.AgentArtifactObjectLocation) error {
	return nil
}

func (s *seedanceHandlerMediaStore) Provider() string { return "cos" }
func (s *seedanceHandlerMediaStore) Bucket() string   { return "seedance-test" }

func (s *seedanceHandlerMediaStore) putCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.puts)
}

func (s *seedanceHandlerMediaStore) lastKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.puts) == 0 {
		return ""
	}
	return s.puts[len(s.puts)-1].Key
}

func (s *seedanceHandlerMediaStore) configuredChecks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isConfiguredCalls
}
