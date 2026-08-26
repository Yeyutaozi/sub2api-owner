package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestTianyueAccountSupportsCustomPublicModelMapping(t *testing.T) {
	credentials := map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping": map[string]any{
			"my-standard-video": SeedanceTianyueSD20Model,
			"my-fast-video":     SeedanceTianyueSD20FastModel,
		},
	}
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, credentials))

	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: credentials}
	require.True(t, account.IsTianyueVideo())
	require.True(t, account.IsModelSupported("my-standard-video"))
	require.True(t, account.IsModelSupported("my-fast-video"))
	require.True(t, account.IsModelSupported("MY-STANDARD-VIDEO"))
	require.False(t, account.IsModelSupported(SeedanceTianyueSD20Model), "Tianyue requires an explicit public mapping")
	require.False(t, account.IsModelSupported("unmapped-video"))
	require.Equal(t, DefaultTianyueVideoBaseURL, account.GetSeedanceBaseURL())
}

func TestParseTianyueCanonicalModelPreservesPublicCasing(t *testing.T) {
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"l-stable-seedance-2-0-933-720p",
		"prompt":"cinematic portrait",
		"resolution":"720p",
		"duration":15
	}`))
	require.NoError(t, err)
	require.Equal(t, SeedanceTianyueSD20FastModel, info.Model)

	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{SeedanceTianyueSD20FastModel: SeedanceTianyueSD20FastModel},
	}}
	require.True(t, account.IsModelSupported(info.Model))
	require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
	require.Equal(t, SeedanceTianyueSD20FastModel, account.GetMappedModel(info.Model))
}

func TestTianyueAccountRejectsUnsupportedMappingTarget(t *testing.T) {
	err := ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": "other-model"},
	})
	require.ErrorContains(t, err, "must map to a supported Tianyue model")
}

func TestValidateTianyueAliasUsesMappedModelProfile(t *testing.T) {
	account := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "test-key",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": SeedanceTianyueSD20Model},
	}}
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"my-video",
		"prompt":"cinematic portrait"
	}`))
	require.NoError(t, err)
	require.Zero(t, info.DurationSeconds)

	require.NoError(t, ValidateSeedanceRequestForAccount(account, info))
	require.Equal(t, "my-video", info.Model)
	require.Equal(t, 15, info.DurationSeconds)
	require.Equal(t, VideoBillingResolution720P, info.Resolution)
	require.Equal(t, "16:9", info.AspectRatio)
}

func TestBuildTianyueVideoCreateRequestUsesPerRequestBillingDuration(t *testing.T) {
	body, err := buildTianyueVideoCreateRequest(&SeedanceRequestInfo{
		Prompt: "cinematic portrait", DurationSeconds: 15, AspectRatio: "9:16", Resolution: VideoBillingResolution720P,
		StartFrameURL:   "https://example.com/start.jpg",
		References:      []SeedanceReferenceImage{{URL: "https://example.com/reference.jpg"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://example.com/reference.mp3"}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://example.com/reference.mp4"}},
	}, "l-stable-seedance-2-0-933-720p")
	require.NoError(t, err)

	var request tianyueVideoCreateRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Equal(t, SeedanceTianyueSD20FastModel, request.Model)
	require.Equal(t, 1, request.Duration, "per-request Tianyue models must always send duration=1")
	require.Equal(t, 15, request.VideoDuration)
	require.Equal(t, []string{"https://example.com/reference.jpg", "https://example.com/start.jpg"}, request.ImageURLs)
	require.Equal(t, []string{"https://example.com/reference.mp3"}, request.AudioURLs)
	require.Equal(t, []string{"https://example.com/reference.mp4"}, request.VideoURLs)
}

func TestBuildTianyueVideoCreateRequestPreservesSignedCanvasMediaURL(t *testing.T) {
	mediaURL := "https://gateway.example.com/api/v1/local-media?key=canvas-image&expires=1893456000&signature=test-signature"
	body, err := buildTianyueVideoCreateRequest(&SeedanceRequestInfo{
		Prompt: "animate the reference", DurationSeconds: 15, AspectRatio: "16:9", Resolution: VideoBillingResolution720P,
		References: []SeedanceReferenceImage{{URL: mediaURL}},
	}, SeedanceTianyueSD20FastModel)
	require.NoError(t, err)

	var request tianyueVideoCreateRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Equal(t, []string{mediaURL}, request.ImageURLs)
}

func TestForwardTianyueSeedanceCreatesOpaqueBoundTask(t *testing.T) {
	upstream := &seedanceHTTPUpstreamStub{body: `{"id":"task_123","task_id":"task_123","status":"queued"}`}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	gateway := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "secret",
		"base_url":       "http://tianyue.example",
		"video_provider": VideoProviderTianyue,
		"model_mapping":  map[string]any{"my-video": SeedanceTianyueSD20Model},
	}}
	info := &SeedanceRequestInfo{Model: "my-video", Prompt: "cinematic portrait", DurationSeconds: 15, AspectRatio: "16:9", Resolution: VideoBillingResolution720P}

	response, err := gateway.ForwardSeedance(t.Context(), nil, account, "POST", "", info)
	require.NoError(t, err)
	require.NotNil(t, response.Result)
	require.Equal(t, "task_123", response.Result.UpstreamResponseID)
	require.Equal(t, SeedanceTianyueSD20UpstreamModel, response.Result.UpstreamModel)
	require.NotEqual(t, "task_123", response.Result.ResponseID)
	require.Contains(t, response.Result.ResponseID, "vidjob_")
	require.Equal(t, "Bearer secret", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "http://tianyue.example/v1/videos", upstream.request.URL.String())

	requestBody, readErr := io.ReadAll(upstream.request.Body)
	require.NoError(t, readErr)
	var request tianyueVideoCreateRequest
	require.NoError(t, json.Unmarshal(requestBody, &request))
	require.Equal(t, SeedanceTianyueSD20UpstreamModel, request.Model)
	require.Equal(t, 1, request.Duration)
	require.Equal(t, 15, request.VideoDuration)
}

func TestForwardTianyueSeedanceContentUsesFinalVideoRedirect(t *testing.T) {
	upstream := &tianyueHTTPUpstreamSequenceStub{responses: []tianyueHTTPUpstreamResponse{
		{
			body:   `{"status":"completed","url":"http://216.36.118.192:15036/wrong/content","video_url":"http://216.36.118.192:15036/right/content?signature=one"}`,
			header: http.Header{"Content-Type": []string{"application/json"}},
		},
		{
			statusCode: http.StatusFound,
			header:     http.Header{"Location": []string{"https://104.18.0.1/video.mp4?signature=two"}},
		},
		{
			body:       "video-bytes",
			statusCode: http.StatusPartialContent,
			header: http.Header{
				"Content-Type":  []string{"video/mp4"},
				"Content-Range": []string{"bytes 0-10/11"},
			},
		},
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	gateway := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":        "secret",
		"base_url":       "http://tianyue.example",
		"video_provider": VideoProviderTianyue,
	}}

	response, err := gateway.ForwardSeedanceContent(t.Context(), nil, account, "task_123", "bytes=0-10")
	require.NoError(t, err)
	require.NotNil(t, response.BodyStream)
	defer func() { _ = response.BodyStream.Close() }()
	content, readErr := io.ReadAll(response.BodyStream)
	require.NoError(t, readErr)
	require.Equal(t, "video-bytes", string(content))
	require.Equal(t, http.StatusPartialContent, response.StatusCode)
	require.Equal(t, "bytes 0-10/11", response.Header.Get("Content-Range"))
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "http://tianyue.example/v1/videos/task_123", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer secret", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "http://216.36.118.192:15036/right/content?signature=one", upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "https://104.18.0.1/video.mp4?signature=two", upstream.requests[2].URL.String())
	require.Empty(t, upstream.requests[2].Header.Get("Authorization"))
	require.Equal(t, "bytes=0-10", upstream.requests[2].Header.Get("Range"))
	require.Equal(t, "*/*", upstream.requests[2].Header.Get("Accept"))
}

func TestParseTianyueTaskResultPrefersVideoURL(t *testing.T) {
	status, resultURL, err := parseTianyueTaskResult([]byte(`{
		"status":"completed",
		"url":"http://216.36.118.192:15036/wrong/content",
		"video_url":"https://104.18.0.1/correct.mp4",
		"result_url":"https://104.18.0.1/fallback.mp4"
	}`))
	require.NoError(t, err)
	require.Equal(t, "completed", status)
	require.Equal(t, "https://104.18.0.1/correct.mp4", resultURL)
}

func TestValidateTianyueVideoResultURLRestrictsBootstrapAndFinalPorts(t *testing.T) {
	_, secure, err := validateTianyueVideoResultURL("http://216.36.118.192:15036/content?signature=one", true)
	require.NoError(t, err)
	require.False(t, secure)

	_, secure, err = validateTianyueVideoResultURL("https://104.18.0.1/video.mp4?signature=two", false)
	require.NoError(t, err)
	require.True(t, secure)

	_, _, err = validateTianyueVideoResultURL("http://216.36.118.192:8080/content", true)
	require.Error(t, err)
	_, _, err = validateTianyueVideoResultURL("https://104.18.0.1:15036/video.mp4", false)
	require.Error(t, err)
}

type tianyueHTTPUpstreamResponse struct {
	body       string
	statusCode int
	header     http.Header
}

type tianyueHTTPUpstreamSequenceStub struct {
	requests  []*http.Request
	responses []tianyueHTTPUpstreamResponse
}

func (s *tianyueHTTPUpstreamSequenceStub) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.requests = append(s.requests, request.Clone(request.Context()))
	response := s.responses[len(s.requests)-1]
	statusCode := response.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	header := response.header
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header.Clone(),
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}

func (s *tianyueHTTPUpstreamSequenceStub) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
}

func TestNormalizeTianyueTaskHidesUpstreamDetails(t *testing.T) {
	body, err := NormalizeSeedanceJobForRoute(
		[]byte(`{"id":"upstream-task","status":"completed","model":"L-SD2-F-720-933","video_url":"https://cdn.example.com/video.mp4"}`),
		"vidjob_public", VideoProviderTianyue, "my-video",
	)
	require.NoError(t, err)
	require.NotContains(t, string(body), "upstream-task")
	require.NotContains(t, string(body), "cdn.example.com")
	require.NotContains(t, string(body), SeedanceTianyueSD20Model)
	require.Contains(t, string(body), `"model":"my-video"`)
	require.Contains(t, string(body), `/v1/videos/jobs/vidjob_public/content`)
}
