package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceDefaultModelsUseFFLinkIDs(t *testing.T) {
	require.Equal(t, []string{
		"seedance-2.0",
		"seedance-2.0-fast",
		"seedance-2.0-mini",
		"sd2-mx933-720-1s",
		"sd2-mx933-720-fast-1s",
	}, defaultModelsListCandidateIDs(PlatformSeedance))
}

type seedanceHTTPUpstreamStub struct {
	request    *http.Request
	body       string
	statusCode int
	header     http.Header
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

type seedanceTaskBindingRepoStub struct {
	UsageLogRepository
	mu           sync.Mutex
	saved        *SeedanceTaskBinding
	bindings     []SeedanceTaskBinding
	listUserID   int64
	listAPIKeyID int64
	listGroupID  int64
	listLimit    int
	getCalls     int
}

func (s *seedanceTaskBindingRepoStub) SaveSeedanceTaskBinding(_ context.Context, binding *SeedanceTaskBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *binding
	s.saved = &copy
	return nil
}

func (s *seedanceTaskBindingRepoStub) GetSeedanceTaskBinding(
	_ context.Context,
	userID, apiKeyID, groupID int64,
	jobID string,
) (*SeedanceTaskBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	for i := range s.bindings {
		binding := s.bindings[i]
		if binding.UserID == userID && binding.APIKeyID == apiKeyID && binding.GroupID == groupID && binding.JobID == jobID {
			return &binding, nil
		}
	}
	return nil, errors.New("binding not found")
}

func (s *seedanceTaskBindingRepoStub) ListSeedanceTaskBindings(
	_ context.Context,
	userID, apiKeyID, groupID int64,
	limit int,
) ([]SeedanceTaskBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listUserID = userID
	s.listAPIKeyID = apiKeyID
	s.listGroupID = groupID
	s.listLimit = limit
	bindings := append([]SeedanceTaskBinding(nil), s.bindings...)
	if len(bindings) > limit {
		bindings = bindings[:limit]
	}
	return bindings, nil
}

type seedanceBindingCacheStub struct {
	GatewayCache
	mu       sync.Mutex
	bindings map[string]int64
}

func (s *seedanceBindingCacheStub) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if accountID, ok := s.bindings[sessionHash]; ok {
		return accountID, nil
	}
	return 0, errors.New("cache miss")
}

func (s *seedanceBindingCacheStub) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bindings == nil {
		s.bindings = make(map[string]int64)
	}
	s.bindings[sessionHash] = accountID
	return nil
}

type seedanceAccountRepoStub struct {
	AccountRepository
	accounts map[int64]*Account
}

func (s *seedanceAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, errors.New("account not found")
	}
	return account, nil
}

type seedanceIndexedHTTPUpstreamStub struct {
	mu         sync.Mutex
	bodies     map[int64]string
	accountIDs []int64
}

func (s *seedanceIndexedHTTPUpstreamStub) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	s.accountIDs = append(s.accountIDs, accountID)
	body, ok := s.bodies[accountID]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("upstream unavailable")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (s *seedanceIndexedHTTPUpstreamStub) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
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
	statusCode := s.statusCode
	if statusCode == 0 {
		statusCode = http.StatusAccepted
	}
	header := s.header
	if header == nil {
		header = http.Header{"Content-Type": []string{"application/json"}}
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
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

func TestParseSeedanceVideoGenerationRequestPublicGuidances(t *testing.T) {
	request, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"seedance-2.0",
		"prompt":"Animate the product",
		"resolution":"720p",
		"duration":6,
		"aspect_ratio":"9:16",
		"audio":true,
		"prompt_enhance":true,
		"image_url":"https://media.example/start.png",
		"guidances":{
			"video_reference_base":[{"video":{"url":"https://media.example/ref.mp4","type":"UPLOADED"},"order":2}],
			"audio_reference":[{"audio":{"url":"https://media.example/ref.mp3","type":"UPLOADED"},"order":3}]
		}
	}`))
	require.NoError(t, err)
	require.Equal(t, "seedance-2.0", request.Model)
	require.Equal(t, "Animate the product", request.Prompt)
	require.True(t, request.GenerateAudio)
	require.Equal(t, true, request.PromptEnhance)
	require.Equal(t, "https://media.example/start.png", request.StartFrameURL)
	require.Len(t, request.References, 0)
	require.Len(t, request.VideoReferences, 1)
	require.Len(t, request.AudioReferences, 1)

	body, err := request.UpstreamBody("seedance-2.0-fast")
	require.NoError(t, err)
	var upstream map[string]any
	require.NoError(t, json.Unmarshal(body, &upstream))
	require.Equal(t, true, upstream["audio"])
	require.Equal(t, true, upstream["prompt_enhance"])
	require.Equal(t, "https://media.example/start.png", upstream["image_url"])
	guidances := upstream["guidances"].(map[string]any)
	require.Contains(t, guidances, "video_reference_base")
	require.Contains(t, guidances, "audio_reference")
	videoRefs := guidances["video_reference_base"].([]any)
	audioRefs := guidances["audio_reference"].([]any)
	require.NotContains(t, videoRefs[0].(map[string]any), "order")
	require.NotContains(t, audioRefs[0].(map[string]any), "order")
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
		[]byte(`{"job_id":"vidjob_123","status":"completed","model":"seedance-2.0","duration":8,"seconds":9,"aspect_ratio":"9:16"}`),
		"https://gateway.example/api/v3/contents/generations/tasks/vidjob_123/content",
	)
	require.NoError(t, err)
	require.Equal(t, "vidjob_123", response["id"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, "seedance-2.0", response["model"])
	require.EqualValues(t, 8, response["duration"])
	require.NotContains(t, response, "ratio")
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
	require.EqualError(t, err, "FFLink video forwarding requires a compatible API key account")
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

func TestForwardSeedanceContentUsesExplicitRangeOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &seedanceHTTPUpstreamStub{
		body:       "\x00\x00\x00\x0cftypisom",
		statusCode: http.StatusOK,
		header: http.Header{
			"Content-Type":   []string{"video/mp4"},
			"Content-Length": []string{"12"},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.fflink.top", "api_key": "upstream-secret"},
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, SeedanceOfficialTasksEndpoint+"/vidjob_123/content", nil)
	ctx.Request.Header.Set("Range", "bytes=0-0")

	response, err := service.ForwardSeedanceContent(context.Background(), ctx, account, "vidjob_123", "")
	require.NoError(t, err)
	require.NotNil(t, response.BodyStream)
	require.NoError(t, response.BodyStream.Close())
	require.Empty(t, upstream.request.Header.Get("Range"))

	response, err = service.ForwardSeedanceContent(context.Background(), ctx, account, "vidjob_123", "bytes=2-4")
	require.NoError(t, err)
	require.NotNil(t, response.BodyStream)
	require.NoError(t, response.BodyStream.Close())
	require.Equal(t, "bytes=2-4", upstream.request.Header.Get("Range"))
}

func TestForwardSeedanceJobsListPreservesQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &seedanceHTTPUpstreamStub{body: `{"data":[{"job_id":"vidjob_123"}]}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       42,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://api.fflink.top",
			"api_key":  "upstream-secret",
		},
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, SeedancePublicJobsEndpoint+"?limit=50&status=running", nil)

	response, err := service.ForwardSeedanceJobsList(context.Background(), ctx, account)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "https://api.fflink.top/v1/videos/jobs?limit=50&status=running", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-secret", upstream.request.Header.Get("Authorization"))
	require.Equal(t, `{"data":[{"job_id":"vidjob_123"}]}`, string(response.Body))
}

func TestFilterSeedanceJobsListKeepsOnlyBoundTasks(t *testing.T) {
	body := []byte(`{"data":[{"job_id":"vidjob_owned","status":"completed","status_url":"https://upstream.example/jobs/vidjob_owned","result":{"data":[{"mp4_url":"https://upstream.example/video.mp4","url":"https://upstream.example/video.mp4","local_url":"https://upstream.example/video.mp4"}]}},{"job_id":"vidjob_other","status":"completed"}]}`)
	filtered, err := FilterSeedanceJobsList(body, func(taskID string) bool {
		return taskID == "vidjob_owned"
	})
	require.NoError(t, err)

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(filtered, &payload))
	require.Len(t, payload.Data, 1)
	require.Equal(t, "vidjob_owned", payload.Data[0]["job_id"])
	require.Equal(t, "/v1/videos/jobs/vidjob_owned", payload.Data[0]["status_url"])
	result := payload.Data[0]["result"].(map[string]any)
	file := result["data"].([]any)[0].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/vidjob_owned/content", file["mp4_url"])
	require.Equal(t, file["mp4_url"], file["url"])
	require.Equal(t, file["mp4_url"], file["local_url"])
}

func TestFilterSeedanceJobsListRejectsMalformedPayload(t *testing.T) {
	_, err := FilterSeedanceJobsList([]byte(`{"jobs":[]}`), func(string) bool { return true })
	require.ErrorContains(t, err, "data array")
}

func TestNormalizeSeedanceJobRewritesUpstreamURLs(t *testing.T) {
	body := []byte(`{"job_id":"vidjob_example","status":"completed","status_url":"https://upstream.example/jobs/vidjob_example","video_url":"https://upstream.example/top-level.mp4","download_url":"https://upstream.example/download.mp4","result":{"data":[{"mp4_url":"https://upstream.example/video.mp4","url":"https://upstream.example/video.mp4","local_url":"https://upstream.example/video.mp4","thumbnail_url":"https://cdn.example/thumb.jpg"}]}}`)
	normalized, err := NormalizeSeedanceJob(body, "vidjob_example")
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(normalized, &payload))
	require.Equal(t, "/v1/videos/jobs/vidjob_example", payload["status_url"])
	require.Equal(t, "vidjob_example", payload["job_id"])
	require.NotContains(t, payload, "id")
	require.NotContains(t, payload, "task_id")
	require.Equal(t, "https://upstream.example/top-level.mp4", payload["video_url"])
	require.Equal(t, "https://upstream.example/download.mp4", payload["download_url"])
	result := payload["result"].(map[string]any)
	file := result["data"].([]any)[0].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/vidjob_example/content", file["mp4_url"])
	require.Equal(t, "https://cdn.example/thumb.jpg", file["thumbnail_url"])
}

func TestNormalizeSeedanceJobDoesNotSynthesizeLegacyFFLinkResult(t *testing.T) {
	normalized, err := NormalizeSeedanceJob(
		[]byte(`{"job_id":"vidjob_example","status":"completed","video_url":"https://upstream.example/video.mp4"}`),
		"vidjob_example",
	)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(normalized, &payload))
	require.NotContains(t, payload, "result")
	require.Equal(t, "https://upstream.example/video.mp4", payload["video_url"])
}

func TestSeedanceTaskBindingPersistsAndBackfillsCache(t *testing.T) {
	groupID := int64(3)
	repo := &seedanceTaskBindingRepoStub{}
	cache := &seedanceBindingCacheStub{}
	service := &OpenAIGatewayService{usageLogRepo: repo, cache: cache}

	err := service.BindSeedanceTaskAccount(
		context.Background(), &groupID, "vidjob_bound", 1, 2, 4, "seedance-2.0",
	)
	require.NoError(t, err)
	require.NotNil(t, repo.saved)
	require.Equal(t, int64(1), repo.saved.UserID)
	require.Equal(t, int64(2), repo.saved.APIKeyID)
	require.Equal(t, int64(3), repo.saved.GroupID)
	require.Equal(t, int64(4), repo.saved.AccountID)
	require.Equal(t, "seedance-2.0", repo.saved.Model)

	repo.bindings = []SeedanceTaskBinding{*repo.saved}
	cache = &seedanceBindingCacheStub{}
	service.cache = cache
	accountID, err := service.ResolveSeedanceTaskAccount(context.Background(), &groupID, "vidjob_bound", 1, 2)
	require.NoError(t, err)
	require.Equal(t, int64(4), accountID)
	require.Equal(t, 1, repo.getCalls)

	cacheKey := service.openAISessionCacheKey(SeedanceTaskSessionHash("vidjob_bound", 1, 2))
	cachedAccountID, err := cache.GetSessionAccountID(context.Background(), groupID, cacheKey)
	require.NoError(t, err)
	require.Equal(t, int64(4), cachedAccountID)

	legacyCache := &seedanceBindingCacheStub{bindings: map[string]int64{cacheKey: 4}}
	legacyService := &OpenAIGatewayService{cache: legacyCache}
	legacyAccountID, err := legacyService.ResolveSeedanceTaskAccount(context.Background(), &groupID, "vidjob_bound", 1, 2)
	require.NoError(t, err)
	require.Equal(t, int64(4), legacyAccountID)
}

func TestListOwnedSeedanceJobsAggregatesBoundAccounts(t *testing.T) {
	groupID := int64(30)
	createdAt := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	repo := &seedanceTaskBindingRepoStub{bindings: []SeedanceTaskBinding{
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 101, JobID: "vidjob_completed", Model: "seedance-2.0", CreatedAt: createdAt.Add(2 * time.Minute)},
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 202, JobID: "vidjob_running", Model: "ltx-2.0", CreatedAt: createdAt.Add(time.Minute)},
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 303, JobID: "vidjob_unavailable", Model: "happy-horse-1.1", CreatedAt: createdAt},
		{UserID: 99, APIKeyID: 20, GroupID: groupID, AccountID: 404, JobID: "vidjob_other_user", Model: "seedance-2.0", CreatedAt: createdAt},
		{UserID: 10, APIKeyID: 99, GroupID: groupID, AccountID: 505, JobID: "vidjob_other_key", Model: "seedance-2.0", CreatedAt: createdAt},
	}}
	account := func(id int64) *Account {
		return &Account{
			ID: id, Platform: PlatformSeedance, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"base_url": "https://video-upstream.example", "api_key": "secret"},
		}
	}
	accounts := &seedanceAccountRepoStub{accounts: map[int64]*Account{
		101: account(101),
		202: account(202),
	}}
	upstream := &seedanceIndexedHTTPUpstreamStub{bodies: map[int64]string{
		101: `{"job_id":"vidjob_completed","status":"completed","model":"internal-model","result":{"data":[{"mp4_url":"https://upstream.example/one.mp4"}]}}`,
		202: `{"job_id":"vidjob_running","status":"running"}`,
	}}
	service := &OpenAIGatewayService{
		accountRepo: accounts, usageLogRepo: repo, cfg: &config.Config{}, httpUpstream: upstream,
	}

	jobs, err := service.ListOwnedSeedanceJobs(context.Background(), &groupID, 10, 20, 10, "")
	require.NoError(t, err)
	require.Len(t, jobs, 3)
	require.Equal(t, int64(10), repo.listUserID)
	require.Equal(t, int64(20), repo.listAPIKeyID)
	require.Equal(t, groupID, repo.listGroupID)
	require.Equal(t, 10, repo.listLimit)
	require.Equal(t, "vidjob_completed", jobs[0]["job_id"])
	require.Equal(t, "seedance-2.0", jobs[0]["model"])
	require.Equal(t, "completed", jobs[0]["status"])
	result := jobs[0]["result"].(map[string]any)
	file := result["data"].([]any)[0].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/vidjob_completed/content", file["mp4_url"])
	require.Equal(t, "running", jobs[1]["status"])
	require.Equal(t, "unknown", jobs[2]["status"])
	require.Equal(t, "/v1/videos/jobs/vidjob_unavailable", jobs[2]["status_url"])
	for _, job := range jobs {
		require.NotEqual(t, "vidjob_other_user", job["job_id"])
		require.NotEqual(t, "vidjob_other_key", job["job_id"])
	}

	completed, err := service.ListOwnedSeedanceJobs(context.Background(), &groupID, 10, 20, 1, "completed")
	require.NoError(t, err)
	require.Len(t, completed, 1)
	require.Equal(t, "vidjob_completed", completed[0]["job_id"])
	require.Equal(t, MaxSeedanceJobsLimit, repo.listLimit)
}

func TestListOwnedSeedanceJobsFailsClosedForEditedHuiquAccounts(t *testing.T) {
	groupID := int64(31)
	repo := &seedanceTaskBindingRepoStub{bindings: []SeedanceTaskBinding{
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 101, JobID: "hqv1_task_provider_edited", Model: "sd2-mx933-720-1s"},
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 202, JobID: "hqv1_task_group_removed", Model: "sd2-mx933-720-fast-1s"},
		{UserID: 10, APIKeyID: 20, GroupID: groupID, AccountID: 303, JobID: "hqv1_task_valid", Model: "sd2-mx933-720-1s"},
	}}
	account := func(id int64, provider string, groups ...int64) *Account {
		return &Account{
			ID: id, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{
				"api_key":        "secret",
				"video_provider": provider,
			},
			GroupIDs: groups,
		}
	}
	accounts := &seedanceAccountRepoStub{accounts: map[int64]*Account{
		101: account(101, VideoProviderFFLink, groupID),
		202: account(202, VideoProviderHuiqu, groupID+1),
		303: account(303, VideoProviderHuiqu, groupID),
	}}
	upstream := &seedanceIndexedHTTPUpstreamStub{bodies: map[int64]string{
		101: `{"id":"task_provider_edited","status":"completed"}`,
		202: `{"id":"task_group_removed","status":"completed"}`,
		303: `{"id":"task_valid","status":"running"}`,
	}}
	gateway := &OpenAIGatewayService{
		accountRepo: accounts, usageLogRepo: repo, cfg: &config.Config{}, httpUpstream: upstream,
	}

	jobs, err := gateway.ListOwnedSeedanceJobs(context.Background(), &groupID, 10, 20, 10, "")
	require.NoError(t, err)
	require.Len(t, jobs, 3)
	require.Equal(t, "unknown", jobs[0]["status"])
	require.Equal(t, "unknown", jobs[1]["status"])
	require.Equal(t, "running", jobs[2]["status"])
	require.Equal(t, []int64{303}, upstream.accountIDs)
}
