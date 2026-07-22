package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type seedanceHTTPUpstreamStub struct {
	request *http.Request
	body    string
}

type seedanceUsageRefundRepoStub struct {
	UsageBillingRepository
	result   *SeedanceUsageRefundResult
	err      error
	calls    int
	taskID   string
	userID   int64
	apiKeyID int64
}

func (s *seedanceUsageRefundRepoStub) RefundSeedanceUsage(
	_ context.Context,
	taskID string,
	userID int64,
	apiKeyID int64,
) (*SeedanceUsageRefundResult, error) {
	s.calls++
	s.taskID = taskID
	s.userID = userID
	s.apiKeyID = apiKeyID
	return s.result, s.err
}

func (s *seedanceHTTPUpstreamStub) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = request
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
	}, nil
}

func (s *seedanceHTTPUpstreamStub) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
}

func TestParseSeedanceCreateRequestTextOnly(t *testing.T) {
	request, err := ParseSeedanceCreateRequest([]byte(`{
		"model":"seedance-2.0",
		"content":[{"type":"text","text":"A slow aerial shot"}],
		"ratio":"16:9",
		"duration":8,
		"resolution":"720p",
		"generate_audio":true,
		"watermark":false
	}`))
	require.NoError(t, err)
	require.Equal(t, "seedance-2.0", request.Model)
	require.Equal(t, "A slow aerial shot", request.Prompt)
	require.Equal(t, "16:9", request.AspectRatio)
	require.Equal(t, 8, request.DurationSeconds)
	require.Equal(t, "720p", request.Resolution)
	require.True(t, request.GenerateAudio)

	body, err := request.UpstreamBody("seedance-2.0-fast")
	require.NoError(t, err)
	var upstream map[string]any
	require.NoError(t, json.Unmarshal(body, &upstream))
	require.Equal(t, "seedance-2.0-fast", upstream["model"])
	require.Equal(t, "A slow aerial shot", upstream["prompt"])
	require.Equal(t, "16:9", upstream["aspect_ratio"])
	require.Equal(t, float64(8), upstream["duration"])
	require.Equal(t, true, upstream["audio"])
}

func TestParseSeedanceCreateRequestFirstAndLastFrames(t *testing.T) {
	request, err := ParseSeedanceCreateRequest([]byte(`{
		"model":"seedance-2.0",
		"content":[
			{"type":"text","text":"Transition smoothly"},
			{"type":"image_url","image_url":{"url":"https://example.com/start.png","role":"first_frame"}},
			{"type":"image_url","image_url":{"url":"https://example.com/end.png","role":"last_frame"}}
		]
	}`))
	require.NoError(t, err)
	body, err := request.UpstreamBody(request.Model)
	require.NoError(t, err)
	var upstream map[string]any
	require.NoError(t, json.Unmarshal(body, &upstream))
	require.Equal(t, "https://example.com/start.png", upstream["start_frame_url"])
	require.Equal(t, "https://example.com/end.png", upstream["end_frame_url"])
	require.NotContains(t, upstream, "image_url")
}

func TestParseSeedanceCreateRequestReferenceImages(t *testing.T) {
	request, err := ParseSeedanceCreateRequest([]byte(`{
		"model":"seedance-2.0",
		"content":[
			{"type":"text","text":"Keep the product consistent"},
			{"type":"image_url","image_url":{"url":"https://example.com/a.png","role":"reference_image","strength":"HIGH"}},
			{"type":"image_url","image_url":{"url":"https://example.com/b.png","role":"reference_image"}}
		]
	}`))
	require.NoError(t, err)
	body, err := request.UpstreamBody(request.Model)
	require.NoError(t, err)
	var upstream struct {
		Guidances struct {
			References []struct {
				Strength string `json:"strength"`
				Order    int    `json:"order"`
			} `json:"image_reference"`
		} `json:"guidances"`
	}
	require.NoError(t, json.Unmarshal(body, &upstream))
	require.Len(t, upstream.Guidances.References, 2)
	require.Equal(t, "HIGH", upstream.Guidances.References[0].Strength)
	require.Equal(t, 0, upstream.Guidances.References[0].Order)
	require.Equal(t, "MID", upstream.Guidances.References[1].Strength)
	require.Equal(t, 1, upstream.Guidances.References[1].Order)
}

func TestParseSeedanceCreateRequestRejectsMixedImageModes(t *testing.T) {
	_, err := ParseSeedanceCreateRequest([]byte(`{
		"model":"seedance-2.0",
		"content":[
			{"type":"text","text":"Animate"},
			{"type":"image_url","image_url":{"url":"https://example.com/start.png","role":"first_frame"}},
			{"type":"image_url","image_url":{"url":"https://example.com/ref.png","role":"reference_image"}}
		]
	}`))
	require.EqualError(t, err, "reference images cannot be combined with first/last frames")
}

func TestBuildSeedanceOfficialTaskResponse(t *testing.T) {
	response, err := BuildSeedanceOfficialTaskResponse(
		"vidjob_123",
		[]byte(`{"job_id":"vidjob_123","status":"completed","model":"seedance-2.0","duration":8}`),
		"https://gateway.example/api/v3/contents/generations/tasks/vidjob_123/content",
	)
	require.NoError(t, err)
	require.Equal(t, "vidjob_123", response["id"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, "seedance-2.0", response["model"])
	content, ok := response["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://gateway.example/api/v3/contents/generations/tasks/vidjob_123/content", content["video_url"])
}

func TestMapSeedanceTaskStatus(t *testing.T) {
	require.Equal(t, "queued", MapSeedanceTaskStatus("pending"))
	require.Equal(t, "running", MapSeedanceTaskStatus("settling"))
	require.Equal(t, "succeeded", MapSeedanceTaskStatus("completed"))
	require.Equal(t, "failed", MapSeedanceTaskStatus("failed"))
	require.Equal(t, "cancelled", MapSeedanceTaskStatus("canceled"))
}

func TestSeedanceUsageRequestID(t *testing.T) {
	require.Equal(t, "seedance:vidjob_123", SeedanceUsageRequestID(" vidjob_123 "))
	require.Empty(t, SeedanceUsageRequestID(" "))
}

func TestRefundSeedanceUsageUsesOptionalBillingCapability(t *testing.T) {
	repo := &seedanceUsageRefundRepoStub{result: &SeedanceUsageRefundResult{
		Applied:      true,
		UsageLogID:   91,
		UserID:       42,
		APIKeyID:     7,
		RefundedCost: 1.2,
	}}
	svc := &OpenAIGatewayService{usageBillingRepo: repo}

	result, err := svc.RefundSeedanceUsage(context.Background(), "vidjob_123", 42, 7)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.InDelta(t, 1.2, result.RefundedCost, 1e-12)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, "vidjob_123", repo.taskID)
	require.Equal(t, int64(42), repo.userID)
	require.Equal(t, int64(7), repo.apiKeyID)
}

func TestRefundSeedanceUsageRejectsRepositoryWithoutCapability(t *testing.T) {
	svc := &OpenAIGatewayService{usageBillingRepo: &openAIRecordUsageBillingRepoStub{}}
	_, err := svc.RefundSeedanceUsage(context.Background(), "vidjob_123", 42, 7)
	require.ErrorIs(t, err, ErrSeedanceUsageRefundUnavailable)
}

func TestSeedancePlatformIsolation(t *testing.T) {
	require.Equal(t, PlatformSeedance, normalizeOpenAICompatiblePlatform(PlatformSeedance))
	require.Equal(t, PlatformOpenAI, normalizeOpenAICompatiblePlatform(PlatformOpenAI))
	require.Equal(t, PlatformGrok, normalizeOpenAICompatiblePlatform(PlatformGrok))
	require.Equal(t, PlatformOpenAI, normalizeOpenAICompatiblePlatform(PlatformAnthropic))

	openAI := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grok := &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}
	require.True(t, accountMatchesOpenAICompatiblePlatform(openAI, PlatformOpenAI))
	require.False(t, accountMatchesOpenAICompatiblePlatform(openAI, PlatformGrok))
	require.True(t, accountMatchesOpenAICompatiblePlatform(grok, PlatformGrok))
	require.False(t, accountMatchesOpenAICompatiblePlatform(grok, PlatformOpenAI))

	seedance := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "upstream-secret",
		},
	}
	require.True(t, seedance.IsSeedance())
	require.False(t, seedance.IsOpenAICompatible())
	require.True(t, accountMatchesOpenAICompatiblePlatform(seedance, PlatformSeedance))
	require.False(t, accountMatchesOpenAICompatiblePlatform(seedance, PlatformOpenAI))
	require.Equal(t, DefaultSeedanceBaseURL, seedance.GetSeedanceBaseURL())
	require.Equal(t, "upstream-secret", seedance.GetSeedanceAPIKey())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).IsSeedance())
}

func TestValidateSeedanceAccountConfiguration(t *testing.T) {
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{"api_key": "key"}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeOAuth, map[string]any{"api_key": "key"}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{}))
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformOpenAI, AccountTypeOAuth, nil))
}

func TestForwardSeedanceRejectsOpenAIAccount(t *testing.T) {
	service := &OpenAIGatewayService{}
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "openai-secret"},
	}
	_, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodGet, "vidjob_123", nil)
	require.EqualError(t, err, "Seedance forwarding requires a Seedance API key account")
}

func TestForwardSeedanceUsesFYLinkContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &seedanceHTTPUpstreamStub{body: `{"job_id":"vidjob_123","status":"pending"}`}
	service := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:       42,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://api.fflink.top",
			"api_key":  "upstream-secret",
			"model_mapping": map[string]any{
				"doubao-seedance-2-0-pro": "seedance-2.0",
			},
		},
	}
	requestInfo := &SeedanceRequestInfo{
		Model:           "doubao-seedance-2-0-pro",
		Prompt:          "A coastal sunrise",
		Resolution:      "720p",
		DurationSeconds: 8,
		AspectRatio:     "16:9",
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, SeedanceOfficialTasksEndpoint, nil)
	ctx.Request.Header.Set("Idempotency-Key", "client-request-1")

	response, err := service.ForwardSeedance(context.Background(), ctx, account, http.MethodPost, "", requestInfo)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	require.Equal(t, "vidjob_123", response.Result.ResponseID)
	require.Equal(t, "doubao-seedance-2-0-pro", response.Result.Model)
	require.Equal(t, "seedance-2.0", response.Result.UpstreamModel)
	require.Equal(t, 8, response.Result.VideoDurationSeconds)
	require.Equal(t, "720p", response.Result.VideoResolution)

	require.NotNil(t, upstream.request)
	require.Equal(t, "https://api.fflink.top/v1/videos/generations", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-secret", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "respond-async", upstream.request.Header.Get("Prefer"))
	require.Equal(t, "client-request-1", upstream.request.Header.Get("Idempotency-Key"))
	forwardedBody, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	var forwarded map[string]any
	require.NoError(t, json.Unmarshal(forwardedBody, &forwarded))
	require.Equal(t, "seedance-2.0", forwarded["model"])
	require.Equal(t, "A coastal sunrise", forwarded["prompt"])
	require.Equal(t, float64(8), forwarded["duration"])
}
