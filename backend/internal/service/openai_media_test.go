package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIMediaHTTPUpstreamStub struct {
	response    *http.Response
	request     *http.Request
	requestBody []byte
}

func (s *openAIMediaHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	if req != nil && req.Body != nil {
		s.requestBody, _ = io.ReadAll(req.Body)
	}
	return s.response, nil
}

func (s *openAIMediaHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func newOpenAIMediaTestContext(method string, target string, body []byte, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c, recorder
}

func buildOpenAIMediaMultipart(t *testing.T, model string, file []byte) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", model))
	part, err := writer.CreateFormFile("file", "sample.wav")
	require.NoError(t, err)
	_, err = part.Write(file)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func TestParseOpenAIMediaRequest(t *testing.T) {
	t.Run("audio speech json", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o-mini-tts","input":"hello"}`)
		c, _ := newOpenAIMediaTestContext(http.MethodPost, openAIAudioSpeechEndpoint, body, "application/json; charset=utf-8")
		parsed, err := (&OpenAIGatewayService{}).ParseOpenAIMediaRequest(c, body)
		require.NoError(t, err)
		require.Equal(t, OpenAIEndpointCapabilityAudioSpeech, parsed.RequiredCapability)
		require.Equal(t, "gpt-4o-mini-tts", parsed.Model)
		require.True(t, parsed.Billable)
	})

	t.Run("audio transcription multipart", func(t *testing.T) {
		body, contentType := buildOpenAIMediaMultipart(t, "gpt-4o-transcribe", []byte("wave"))
		c, _ := newOpenAIMediaTestContext(http.MethodPost, openAIAudioTranscriptionsEndpoint, body, contentType)
		parsed, err := (&OpenAIGatewayService{}).ParseOpenAIMediaRequest(c, body)
		require.NoError(t, err)
		require.True(t, parsed.Multipart)
		require.Equal(t, "gpt-4o-transcribe", parsed.Model)
	})

	t.Run("video content get", func(t *testing.T) {
		c, _ := newOpenAIMediaTestContext(http.MethodGet, "/v1/videos/video_123/content?download=1", nil, "")
		parsed, err := (&OpenAIGatewayService{}).ParseOpenAIMediaRequest(c, nil)
		require.NoError(t, err)
		require.Equal(t, "/v1/videos/video_123/content", parsed.Endpoint)
		require.Equal(t, "video_123", parsed.ResourceID)
		require.False(t, parsed.Billable)
	})

	t.Run("rejects non allowlisted subpath", func(t *testing.T) {
		c, _ := newOpenAIMediaTestContext(http.MethodGet, "/v1/videos/video_123/metadata/raw", nil, "")
		_, err := (&OpenAIGatewayService{}).ParseOpenAIMediaRequest(c, nil)
		require.ErrorContains(t, err, "unsupported video get endpoint")
	})

	t.Run("rejects transcription json", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o-transcribe"}`)
		c, _ := newOpenAIMediaTestContext(http.MethodPost, openAIAudioTranscriptionsEndpoint, body, "application/json")
		_, err := (&OpenAIGatewayService{}).ParseOpenAIMediaRequest(c, body)
		require.ErrorContains(t, err, "multipart/form-data")
	})
}

func TestOpenAIAccountSupportsMediaCapabilities(t *testing.T) {
	apiKeyAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, capability := range []OpenAIEndpointCapability{
		OpenAIEndpointCapabilityAudioSpeech,
		OpenAIEndpointCapabilityTranscriptions,
		OpenAIEndpointCapabilityTranslations,
		OpenAIEndpointCapabilityVideos,
	} {
		require.True(t, apiKeyAccount.SupportsOpenAIEndpointCapability(capability))
	}

	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, oauthAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAudioSpeech))
	require.False(t, oauthAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))

	restrictedAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			openAIEndpointCapabilitiesCredentialKey: []any{string(OpenAIEndpointCapabilityAudioSpeech)},
		},
	}
	require.True(t, restrictedAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAudioSpeech))
	require.False(t, restrictedAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
}

func TestForwardOpenAIMediaPreservesVideoBinaryRequestAndResponse(t *testing.T) {
	upstream := &openAIMediaHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":        []string{"video/mp4"},
			"Content-Length":      []string{"5"},
			"Content-Range":       []string{"bytes 0-4/20"},
			"Content-Disposition": []string{`attachment; filename="clip.mp4"`},
		},
		Body: io.NopCloser(strings.NewReader("video")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	c, recorder := newOpenAIMediaTestContext(http.MethodGet, "/v1/videos/video_123/content?download=1", nil, "")
	c.Request.Header.Set("Range", "bytes=0-4")
	c.Request.Header.Set("Authorization", "Bearer client-secret")
	parsed := &OpenAIMediaRequest{
		Endpoint:           "/v1/videos/video_123/content",
		Method:             http.MethodGet,
		ResourceID:         "video_123",
		RequiredCapability: OpenAIEndpointCapabilityVideos,
	}
	account := &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://api.openai.com",
		},
	}

	result, err := svc.ForwardMedia(context.Background(), c, account, nil, parsed, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video", recorder.Body.String())
	require.Equal(t, "bytes 0-4/20", recorder.Header().Get("Content-Range"))
	require.Equal(t, `attachment; filename="clip.mp4"`, recorder.Header().Get("Content-Disposition"))
	require.Equal(t, http.MethodGet, upstream.request.Method)
	require.Equal(t, "download=1", upstream.request.URL.RawQuery)
	require.Equal(t, "bytes=0-4", upstream.request.Header.Get("Range"))
	require.Equal(t, "Bearer upstream-key", upstream.request.Header.Get("Authorization"))
}

func TestForwardOpenAIMediaRewritesVideoModelAndExtractsResourceID(t *testing.T) {
	upstream := &openAIMediaHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"req_media"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"video_456","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"sora-2","prompt":"waves","seconds":"8","size":"1280x720"}`)
	c, recorder := newOpenAIMediaTestContext(http.MethodPost, "/v1/videos?project=demo", body, "application/json")
	c.Request.Header.Set("Idempotency-Key", "idem-1")
	parsed, err := svc.ParseOpenAIMediaRequest(c, body)
	require.NoError(t, err)
	account := &Account{
		ID:       9,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://api.openai.com",
		},
	}

	result, err := svc.ForwardMedia(context.Background(), c, account, body, parsed, "sora-2-pro")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "video_456", result.ResponseID)
	require.Equal(t, "sora-2", result.Model)
	require.Equal(t, "sora-2-pro", result.BillingModel)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, 8, result.VideoDurationSeconds)
	require.Equal(t, "/v1/videos", result.UpstreamEndpoint)
	require.Equal(t, "sora-2-pro", gjson.GetBytes(upstream.requestBody, "model").String())
	require.Equal(t, "project=demo", upstream.request.URL.RawQuery)
	require.Equal(t, "idem-1", upstream.request.Header.Get("Idempotency-Key"))
}

func TestRewriteOpenAIMediaMultipartModelPreservesFile(t *testing.T) {
	body, contentType := buildOpenAIMediaMultipart(t, "old-model", []byte("audio-data"))
	rewritten, rewrittenType, err := rewriteOpenAIMediaModel(body, contentType, "new-model")
	require.NoError(t, err)
	model, err := parseOpenAIMediaMultipartModel(rewritten, rewrittenType)
	require.NoError(t, err)
	require.Equal(t, "new-model", model)
	require.Contains(t, string(rewritten), "audio-data")
}

func TestOpenAIMediaResourceSessionHashIsScopedByAPIKey(t *testing.T) {
	require.Equal(t, OpenAIMediaResourceSessionHash(1, "video_1"), OpenAIMediaResourceSessionHash(1, "video_1"))
	require.NotEqual(t, OpenAIMediaResourceSessionHash(1, "video_1"), OpenAIMediaResourceSessionHash(2, "video_1"))
}
