package handler

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type creazyCanvasGatewayWorkServiceStub struct {
	mu          sync.Mutex
	work        *service.CreazyCanvasWork
	createCalls []service.CreateCreazyCanvasWorkInput
	syncCalls   []service.SyncAcceptedCreazyCanvasVideoInput
	updateCalls []service.UpdateCreazyCanvasWorkInput
}

func (s *creazyCanvasGatewayWorkServiceStub) CreateWork(_ context.Context, input service.CreateCreazyCanvasWorkInput) (*service.CreazyCanvasWork, error) {
	s.createCalls = append(s.createCalls, input)
	work := &service.CreazyCanvasWork{
		ID:          9001,
		UserID:      input.UserID,
		APIKeyID:    input.APIKeyID,
		Kind:        input.Kind,
		Status:      input.Status,
		PublicModel: input.PublicModel,
		Prompt:      input.Prompt,
		ParamsJSON:  input.ParamsJSON,
	}
	s.work = work
	return work, nil
}

func (s *creazyCanvasGatewayWorkServiceStub) GetWork(_ context.Context, _, _ int64) (*service.CreazyCanvasWork, error) {
	if s.work == nil {
		return nil, service.ErrCreazyCanvasWorkNotFound
	}
	copy := *s.work
	return &copy, nil
}

func (s *creazyCanvasGatewayWorkServiceStub) SyncAcceptedVideoWork(_ context.Context, input service.SyncAcceptedCreazyCanvasVideoInput) (*service.CreazyCanvasWork, error) {
	s.syncCalls = append(s.syncCalls, input)
	work := &service.CreazyCanvasWork{
		ID:              input.AssociatedWorkID,
		UserID:          input.UserID,
		APIKeyID:        input.APIKey.ID,
		Kind:            service.CreazyCanvasWorkKindVideo,
		PublicModel:     input.PublicModel,
		Status:          service.CreazyCanvasWorkStatusRunning,
		Prompt:          input.Prompt,
		ParamsJSON:      input.ParamsJSON,
		GatewayType:     service.CreazyCanvasGatewayVideoJob,
		GatewayRemoteID: input.GatewayRemoteID,
	}
	s.work = work
	return work, nil
}

func (s *creazyCanvasGatewayWorkServiceStub) UpdateWork(_ context.Context, input service.UpdateCreazyCanvasWorkInput) (*service.CreazyCanvasWork, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls = append(s.updateCalls, input)
	if s.work == nil {
		s.work = &service.CreazyCanvasWork{ID: input.WorkID, UserID: input.UserID}
	}
	if input.Status != nil {
		s.work.Status = *input.Status
	}
	return s.work, nil
}

func (s *creazyCanvasGatewayWorkServiceStub) hasSucceededImageURL(mediaURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, update := range s.updateCalls {
		if update.Status != nil && *update.Status == service.CreazyCanvasWorkStatusSucceeded &&
			update.ObjectURL != nil && *update.ObjectURL == mediaURL {
			return true
		}
	}
	return false
}

func newCreazyCanvasGatewayTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	return c
}

func TestBeginCreazyCanvasGatewayWorkReusesOwnedActiveCorrelation(t *testing.T) {
	stub := &creazyCanvasGatewayWorkServiceStub{work: &service.CreazyCanvasWork{
		ID:       17,
		UserID:   3,
		APIKeyID: 9,
		Kind:     service.CreazyCanvasWorkKindImage,
		Status:   service.CreazyCanvasWorkStatusRunning,
	}}
	h := &OpenAIGatewayHandler{creazyCanvasService: stub}
	c := newCreazyCanvasGatewayTestContext()
	c.Request.Header.Set(creazyCanvasWorkIDHeader, "17")

	tracker := h.beginCreazyCanvasGatewayWork(c, creazyCanvasGatewayWorkInput{
		UserID:      3,
		APIKeyID:    9,
		Kind:        service.CreazyCanvasWorkKindImage,
		PublicModel: "gpt-image-2",
		GatewayType: service.CreazyCanvasGatewayImageSync,
		Status:      service.CreazyCanvasWorkStatusRunning,
	})

	require.NotNil(t, tracker)
	require.Equal(t, int64(17), tracker.workID)
	require.Empty(t, stub.createCalls)
	require.Len(t, stub.updateCalls, 1)

	h.succeedCreazyCanvasGatewayWork(c, tracker, "", "https://cdn.example/result.png", "image/png")
	h.failCreazyCanvasGatewayWork(c, tracker, "must not overwrite success")
	require.Len(t, stub.updateCalls, 2)
	require.NotNil(t, stub.updateCalls[1].Status)
	require.Equal(t, service.CreazyCanvasWorkStatusSucceeded, *stub.updateCalls[1].Status)
	require.NotNil(t, stub.updateCalls[1].ObjectURL)
	require.Equal(t, "https://cdn.example/result.png", *stub.updateCalls[1].ObjectURL)
}

func TestBeginCreazyCanvasGatewayWorkDoesNotReuseWrongKey(t *testing.T) {
	stub := &creazyCanvasGatewayWorkServiceStub{work: &service.CreazyCanvasWork{
		ID:       17,
		UserID:   3,
		APIKeyID: 99,
		Kind:     service.CreazyCanvasWorkKindImage,
		Status:   service.CreazyCanvasWorkStatusRunning,
	}}
	h := &OpenAIGatewayHandler{creazyCanvasService: stub}
	c := newCreazyCanvasGatewayTestContext()
	c.Request.Header.Set(creazyCanvasWorkIDHeader, "17")

	tracker := h.beginCreazyCanvasGatewayWork(c, creazyCanvasGatewayWorkInput{
		UserID:      3,
		APIKeyID:    9,
		Kind:        service.CreazyCanvasWorkKindImage,
		PublicModel: "gpt-image-2",
		ParamsJSON:  map[string]any{"size": "1024x1024"},
		GatewayType: service.CreazyCanvasGatewayImageSync,
		Status:      service.CreazyCanvasWorkStatusRunning,
	})

	require.NotNil(t, tracker)
	require.Equal(t, int64(9001), tracker.workID)
	require.Len(t, stub.createCalls, 1)
	require.Equal(t, "api", stub.createCalls[0].ParamsJSON["source"])
}

func TestCreazyCanvasGatewayHeaderAndImageURLParsing(t *testing.T) {
	require.Equal(t, int64(42), creazyCanvasCorrelationWorkID(" 42 "))
	require.Zero(t, creazyCanvasCorrelationWorkID("0"))
	require.Zero(t, creazyCanvasCorrelationWorkID("1.5"))
	require.Equal(t, "https://cdn.example/result.webp", creazyCanvasImageURLFromResponse([]byte(`{"data":[{"url":"https://cdn.example/result.webp"}]}`)))
	require.Empty(t, creazyCanvasImageURLFromResponse([]byte(`{"data":[{"url":"javascript:alert(1)"}]}`)))
}
