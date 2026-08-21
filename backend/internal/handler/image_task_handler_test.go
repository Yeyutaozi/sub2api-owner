package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageMultipartFlagIsRemovedBeforeExecution(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "edit"))
	require.NoError(t, writer.WriteField("quality", "medium"))
	require.NoError(t, writer.WriteField("async", "true"))
	for _, name := range []string{"first.png", "second.png"} {
		part, err := writer.CreateFormFile("image", name)
		require.NoError(t, err)
		_, err = part.Write([]byte("image-data"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	requested, err := asyncImageRequested(writer.FormDataContentType(), body.Bytes())
	require.NoError(t, err)
	require.True(t, requested)

	cleaned, contentType, err := stripAsyncImageFlag(writer.FormDataContentType(), body.Bytes())
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(cleaned))
	req.Header.Set("Content-Type", contentType)
	require.NoError(t, req.ParseMultipartForm(1<<20))
	require.Empty(t, req.MultipartForm.Value["async"])
	require.Empty(t, req.MultipartForm.Value["quality"])
	require.Equal(t, []string{"gpt-image-2"}, req.MultipartForm.Value["model"])
	require.Len(t, req.MultipartForm.File["image"], 2)
}

type asyncImageMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*service.ImageTaskRecord
}

func (s *asyncImageMemoryStore) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	s.tasks[task.ID] = &copy
	return nil
}

func (s *asyncImageMemoryStore) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[id]
	if task == nil {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	return &copy, nil
}

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	release := make(chan struct{})
	executedBody := make(chan string, 1)
	canvasWorks := &creazyCanvasGatewayWorkServiceStub{}
	h := &AsyncImageHandler{
		tasks:  tasks,
		openAI: &OpenAIGatewayHandler{creazyCanvasService: canvasWorks},
	}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		body, _ := io.ReadAll(c.Request.Body)
		executedBody <- string(body)
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7, Concurrency: 1})
		c.Next()
	})
	router.POST("/v1/images/generations", h.Submit)
	router.GET("/v1/image/tasks/:task_id", h.Get)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat","quality":"medium","async":true}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var accepted struct {
		TaskID    string `json:"task_id"`
		Status    string `json:"status"`
		QueryPath string `json:"query_path"`
		PollURL   string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	require.Equal(t, service.ImageTaskStatusProcessing, accepted.Status)
	require.Equal(t, "/v1/image/tasks/"+accepted.TaskID, accepted.QueryPath)
	require.Equal(t, accepted.QueryPath, accepted.PollURL)
	require.Equal(t, accepted.PollURL, w.Header().Get("Location"))
	require.Len(t, canvasWorks.createCalls, 1)
	require.Equal(t, service.CreazyCanvasWorkKindImage, canvasWorks.createCalls[0].Kind)
	require.Equal(t, service.CreazyCanvasGatewayImageTask, canvasWorks.createCalls[0].GatewayType)
	require.Equal(t, accepted.TaskID, canvasWorks.createCalls[0].GatewayRemoteID)

	// The detached background request must survive completion of/cancellation
	// from the short submission request.
	cancelRequest()
	close(release)
	forwardedBody := <-executedBody
	require.NotContains(t, forwardedBody, `"async"`)
	require.NotContains(t, forwardedBody, `"quality"`)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return canvasWorks.hasSucceededImageURL("https://example.test/image.png")
	}, time.Second, 10*time.Millisecond)

	pollReq := httptest.NewRequest(http.MethodGet, accepted.PollURL, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Equal(t, "no-store", pollWriter.Header().Get("Cache-Control"))
	require.Empty(t, pollWriter.Header().Get("Retry-After"))
	require.Contains(t, pollWriter.Body.String(), "https://example.test/image.png")
}

// When object storage is not configured the feature is fully disabled: the
// endpoints must return 404 without creating a task or writing to Redis.
func TestAsyncImageHandlerDisabledReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithOptions(store, time.Hour, time.Minute) // enabled == false
	h := &AsyncImageHandler{tasks: tasks}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not enabled")

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_missing", nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusNotFound, pollWriter.Code)

	// No task was created / persisted.
	require.Empty(t, store.tasks)
}
