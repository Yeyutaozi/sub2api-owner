package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteSeedanceForwardErrorKeepsProviderDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"code":"adapter_error","message":"Xmanway HTTP 400: 参考视频分辨率必须在 480p 和 720p 之间"}}`),
	}
	(&OpenAIGatewayHandler{}).writeSeedanceForwardError(ctx, err)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"invalid_request","message":"参考视频分辨率必须在 480p 和 720p 之间","type":"invalid_request_error"}}`, recorder.Body.String())
}
