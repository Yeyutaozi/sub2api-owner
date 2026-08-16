package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSyncAcceptedSeedanceVideoWorkForwardsTrustedIdentityAndAssociationHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &creazyCanvasGatewayWorkServiceStub{}
	h := &OpenAIGatewayHandler{creazyCanvasService: stub}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	c.Request.Header.Set(creazyCanvasWorkIDHeader, "44")
	apiKey := &service.APIKey{ID: 9, UserID: 7}
	info := &service.SeedanceRequestInfo{
		Model:           "sd-2.0-900-720p",
		Prompt:          "nine reference images",
		Resolution:      "720p",
		DurationSeconds: 5,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
		StartFrameURL:   "data:image/png;base64,inline-must-not-persist",
		References: []service.SeedanceReferenceImage{
			{URL: "https://cdn.example/ref-1.png"},
			{URL: "data:image/png;base64,inline-must-not-persist"},
		},
	}

	work, err := h.syncAcceptedSeedanceVideoWork(c, apiKey, 7, info, "vidjob_123")

	require.NoError(t, err)
	require.NotNil(t, work)
	require.Len(t, stub.syncCalls, 1)
	input := stub.syncCalls[0]
	require.Same(t, apiKey, input.APIKey)
	require.Equal(t, int64(7), input.UserID)
	require.Equal(t, int64(44), input.AssociatedWorkID)
	require.Equal(t, "sd-2.0-900-720p", input.PublicModel)
	require.Equal(t, "nine reference images", input.Prompt)
	require.Equal(t, "vidjob_123", input.GatewayRemoteID)
	require.Equal(t, "720p", input.ParamsJSON["resolution"])
	require.Equal(t, 5, input.ParamsJSON["duration"])
	require.Equal(t, true, input.ParamsJSON["generate_audio"])
	require.Equal(t, 2, input.ParamsJSON["image_reference_count"])
	require.Equal(t, []string{"https://cdn.example/ref-1.png"}, input.ParamsJSON["ref_images"])
	require.NotContains(t, input.ParamsJSON, "start_frame")
}

func TestSyncAcceptedSeedanceVideoWorkTreatsMalformedAssociationAsDirectAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &creazyCanvasGatewayWorkServiceStub{}
	h := &OpenAIGatewayHandler{creazyCanvasService: stub}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	c.Request.Header.Set(creazyCanvasWorkIDHeader, "not-an-id")

	_, err := h.syncAcceptedSeedanceVideoWork(c, &service.APIKey{ID: 9, UserID: 7}, 7, &service.SeedanceRequestInfo{}, "vidjob_456")

	require.NoError(t, err)
	require.Len(t, stub.syncCalls, 1)
	require.Zero(t, stub.syncCalls[0].AssociatedWorkID)
}
