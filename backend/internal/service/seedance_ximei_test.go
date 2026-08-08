package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestXimeiVideoProviderRoutesOnlySupportedModels(t *testing.T) {
	account := &Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "sk_live_test",
			"video_provider": VideoProviderXimei,
		},
	}

	require.True(t, account.IsXimeiVideo())
	require.Equal(t, DefaultXimeiVideoBaseURL, account.GetSeedanceBaseURL())
	require.True(t, account.IsModelSupported(SeedanceXimeiSD20Model))
	require.True(t, account.IsModelSupported(SeedanceXimeiSD25Model))
	require.False(t, account.IsModelSupported("seedance-2.0"))
	require.False(t, account.IsModelSupported(SeedanceMX933Model))

	fflink := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ff"}}
	huiqu := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "hq", "video_provider": VideoProviderHuiqu}}
	require.False(t, fflink.IsModelSupported(SeedanceXimeiSD20Model))
	require.False(t, huiqu.IsModelSupported(SeedanceXimeiSD20Model))
}

func TestValidateXimeiVideoAccountConfiguration(t *testing.T) {
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderXimei,
		"model_mapping": map[string]any{
			SeedanceXimeiSD20Model: SeedanceXimeiSD20Model,
			SeedanceXimeiSD25Model: SeedanceXimeiSD25Model,
		},
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeOAuth, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderXimei,
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "sk_live_test",
		"video_provider": VideoProviderXimei,
		"model_mapping":  map[string]any{SeedanceXimeiSD20Model: "kele_pool"},
	}))
}

func TestXimeiProductMappingUsesModelAndResolution(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		route      string
		mode       ximeiDurationMode
	}{
		{SeedanceXimeiSD20Model, VideoBillingResolution480P, "kele_pool", ximeiDurationParameter},
		{SeedanceXimeiSD20Model, VideoBillingResolution720P, "tc_pool", ximeiDurationPrompt},
		{SeedanceXimeiSD25Model, VideoBillingResolution720P, "nangua_pool", ximeiDurationParameter},
	}
	for _, test := range tests {
		product, err := ximeiVideoProductFor(test.model, test.resolution)
		require.NoError(t, err)
		require.Equal(t, test.route, product.Route)
		require.Equal(t, test.resolution, product.Resolution)
		require.Equal(t, test.mode, product.DurationMode)
	}
	for _, test := range []struct{ model, resolution string }{
		{SeedanceXimeiSD20Model, VideoBillingResolution1080P},
		{SeedanceXimeiSD25Model, VideoBillingResolution480P},
		{SeedanceXimeiSD25Model, VideoBillingResolution1080P},
	} {
		_, err := ximeiVideoProductFor(test.model, test.resolution)
		require.Error(t, err)
	}
}

func TestBuildXimeiRequestCompilesRouteFramesDurationAndMedia(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model: SeedanceXimeiSD20Model, Prompt: "人物拿起产品并走向窗边",
		Resolution: VideoBillingResolution720P, DurationSeconds: 10, AspectRatio: "9:16", GenerateAudio: true,
		StartFrameURL: "https://media.example/start.png",
		EndFrameURL:   "https://media.example/end.png",
		References:    []SeedanceReferenceImage{{URL: "https://media.example/product.png"}},
		VideoReferences: []SeedanceReferenceVideo{
			{URL: "https://media.example/motion.mp4", DurationSeconds: 6.25},
		},
		AudioReferences: []SeedanceReferenceAudio{
			{URL: "https://media.example/voice.wav", DurationSeconds: 8.5},
		},
	}

	body, route, err := buildXimeiVideoCreateRequest(request)
	require.NoError(t, err)
	require.Equal(t, "tc_pool", route)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "video", payload["model"])
	require.Equal(t, "tc_pool", payload["provider_route"])
	require.Equal(t, "auto", payload["duration"])
	require.Equal(t, "9:16", payload["aspect_ratio"])
	require.Equal(t, true, payload["generate_audio"])
	// 参考图在前，首尾帧追加到末尾
	require.Equal(t, []any{
		"https://media.example/product.png",
		"https://media.example/start.png",
		"https://media.example/end.png",
	}, payload["image_urls"])
	prompt := payload["prompt"].(string)
	require.Contains(t, prompt, "严格为 10 秒")
	require.Contains(t, prompt, "@Image1 是普通图片参考")
	require.Contains(t, prompt, "@Image2 是首帧")
	require.Contains(t, prompt, "@Image3 是尾帧")
	require.Contains(t, prompt, "@Audio1")
	require.Contains(t, prompt, "@Video1")
	require.EqualValues(t, 6.25, payload["video_urls"].([]any)[0].(map[string]any)["durationSeconds"])
	require.EqualValues(t, 8.5, payload["audio_urls"].([]any)[0].(map[string]any)["durationSeconds"])
}

func TestParseOfficialXimeiRequestAllowsFramesWithReferenceImages(t *testing.T) {
	info, err := ParseSeedanceCreateRequest([]byte(`{
		"model":"sd-2.0-mx933",
		"content":[
			{"type":"text","text":"人物从首帧连续运动到尾帧"},
			{"type":"image_url","image_url":"https://media.example/first.png","role":"first_frame"},
			{"type":"image_url","image_url":"https://media.example/last.png","role":"last_frame"},
			{"type":"image_url","image_url":"https://media.example/product.png","role":"reference_image"}
		],
		"duration":10,
		"resolution":"720p",
		"ratio":"16:9"
	}`))
	require.NoError(t, err)
	require.Equal(t, "https://media.example/first.png", info.StartFrameURL)
	require.Equal(t, "https://media.example/last.png", info.EndFrameURL)
	require.Len(t, info.References, 1)
	require.Equal(t, "https://media.example/product.png", info.References[0].URL)
}

func TestParsePublicXimeiRequestRequiresFirstFrameWithLastFrame(t *testing.T) {
	_, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"sd-2.0-mx933",
		"prompt":"自然收束到尾帧",
		"end_frame_url":"https://media.example/last.png",
		"duration":5,
		"resolution":"720p"
	}`))
	require.ErrorContains(t, err, "last-frame image requires a first-frame image")
}

func TestParseXimeiPublicRequestPreservesReferenceDurations(t *testing.T) {
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"sd-2.0-mx933",
		"prompt":"人物展示产品",
		"resolution":"480p",
		"duration":5,
		"audio":true,
		"guidances":{
			"video_reference_base":[{"video":{"url":"https://media.example/motion.mp4","duration_seconds":6.25}}],
			"audio_reference":[{"audio":{"url":"https://media.example/voice.wav","duration_seconds":8.5}}]
		}
	}`))
	require.NoError(t, err)
	require.Len(t, info.VideoReferences, 1)
	require.Len(t, info.AudioReferences, 1)
	require.Equal(t, 6.25, info.VideoReferences[0].DurationSeconds)
	require.Equal(t, 8.5, info.AudioReferences[0].DurationSeconds)
}

func TestBuildXimeiParameterizedRequestWritesExactDurationString(t *testing.T) {
	for _, test := range []struct {
		name       string
		model      string
		resolution string
		duration   int
		route      string
	}{
		{"SD 2.0 480p 5 seconds", SeedanceXimeiSD20Model, VideoBillingResolution480P, 5, "kele_pool"},
		{"SD 2.5 720p 5 seconds", SeedanceXimeiSD25Model, VideoBillingResolution720P, 5, "nangua_pool"},
		{"SD 2.5 720p 10 seconds", SeedanceXimeiSD25Model, VideoBillingResolution720P, 10, "nangua_pool"},
		{"SD 2.5 720p 15 seconds", SeedanceXimeiSD25Model, VideoBillingResolution720P, 15, "nangua_pool"},
		{"SD 2.5 720p 30 seconds", SeedanceXimeiSD25Model, VideoBillingResolution720P, 30, "nangua_pool"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, route, err := buildXimeiVideoCreateRequest(&SeedanceRequestInfo{
				Model: test.model, Prompt: "safe product demonstration", Resolution: test.resolution,
				DurationSeconds: test.duration, AspectRatio: "16:9",
			})
			require.NoError(t, err)
			require.Equal(t, test.route, route)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			require.Equal(t, fmt.Sprintf("%d", test.duration), payload["duration"])
		})
	}
}

func TestBuildXimeiSD25RequestRejectsUnsupportedDurations(t *testing.T) {
	for _, duration := range []int{1, 20, 25, 31} {
		duration := duration
		t.Run(fmt.Sprintf("%d_seconds", duration), func(t *testing.T) {
			_, _, err := buildXimeiVideoCreateRequest(&SeedanceRequestInfo{
				Model: SeedanceXimeiSD25Model, Prompt: "safe product demonstration",
				Resolution: VideoBillingResolution720P, DurationSeconds: duration, AspectRatio: "16:9",
			})
			require.ErrorContains(t, err, fmt.Sprintf("duration %d is not supported", duration))
		})
	}
}

func TestParseXimeiSD25RequestDefaultsToFiveSecondsAndAllowsSupportedDurations(t *testing.T) {
	info, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"sd-2.5-mx",
		"prompt":"A safe cinematic product demonstration",
		"resolution":"720p",
		"aspect_ratio":"16:9"
	}`))
	require.NoError(t, err)
	require.Equal(t, 5, info.DurationSeconds)

	for _, duration := range []int{5, 10, 15, 30} {
		duration := duration
		t.Run(fmt.Sprintf("%d_seconds", duration), func(t *testing.T) {
			parsed, parseErr := ParseSeedanceVideoGenerationRequest([]byte(fmt.Sprintf(`{
			"model":"sd-2.5-mx",
			"prompt":"A safe cinematic product demonstration",
			"resolution":"720p",
			"duration":%d,
			"aspect_ratio":"16:9"
		}`, duration)))
			require.NoError(t, parseErr)
			require.Equal(t, duration, parsed.DurationSeconds)
			require.Equal(t, VideoBillingResolution720P, parsed.Resolution)
		})
	}
}

func TestParseXimeiSD25RequestRejectsUnsupportedDurationAndResolution(t *testing.T) {
	for _, duration := range []int{1, 20, 25, 31} {
		_, err := ParseSeedanceVideoGenerationRequest([]byte(fmt.Sprintf(`{
			"model":"sd-2.5-mx",
			"prompt":"A safe cinematic product demonstration",
			"resolution":"720p",
			"duration":%d,
			"aspect_ratio":"16:9"
		}`, duration)))
		require.ErrorContains(t, err, "duration")
		require.ErrorContains(t, err, "is not supported")
	}

	for _, resolution := range []string{VideoBillingResolution480P, VideoBillingResolution1080P} {
		_, err := ParseSeedanceVideoGenerationRequest([]byte(fmt.Sprintf(`{
			"model":"sd-2.5-mx",
			"prompt":"A safe cinematic product demonstration",
			"resolution":%q,
			"duration":5,
			"aspect_ratio":"16:9"
		}`, resolution)))
		require.ErrorContains(t, err, "resolution")
		require.ErrorContains(t, err, "is not supported")
	}
}

func TestXimeiSD25BillingUsesActualDurationForPerSecondAndOneUnitForPerRequest(t *testing.T) {
	price := 0.10
	groupID := int64(7250)
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}

	for _, duration := range []int{5, 10, 15, 30} {
		duration := duration
		t.Run(fmt.Sprintf("%d_seconds", duration), func(t *testing.T) {
			result := &OpenAIForwardResult{
				Model: SeedanceXimeiSD25Model, VideoCount: 1,
				VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: duration,
			}
			perSecondKey := &APIKey{GroupID: &groupID, Group: &Group{
				ID: groupID, Platform: PlatformSeedance, VideoBillingUnit: VideoBillingUnitPerSecond,
				VideoModelPrices: VideoModelPrices{
					SeedanceXimeiSD25Model: {Price720P: &price},
				},
			}}
			perSecondCost := svc.calculateOpenAIVideoCost(
				context.Background(), SeedanceXimeiSD25Model, perSecondKey, result, 1,
			)
			require.InDelta(t, price*float64(duration), perSecondCost.TotalCost, 1e-12)

			perRequestKey := &APIKey{GroupID: &groupID, Group: &Group{
				ID: groupID, Platform: PlatformSeedance, VideoBillingUnit: VideoBillingUnitPerRequest,
				VideoModelPrices: VideoModelPrices{
					SeedanceXimeiSD25Model: {Price720P: &price},
				},
			}}
			perRequestCost := svc.calculateOpenAIVideoCost(
				context.Background(), SeedanceXimeiSD25Model, perRequestKey, result, 1,
			)
			require.InDelta(t, price, perRequestCost.TotalCost, 1e-12)
		})
	}
}

func TestBuildXimeiRequestRequiresAndBoundsMediaDurations(t *testing.T) {
	base := &SeedanceRequestInfo{
		Model: SeedanceXimeiSD20Model, Prompt: "安全提示词", Resolution: VideoBillingResolution480P,
		DurationSeconds: 5, AspectRatio: "16:9",
	}
	missing := *base
	missing.VideoReferences = []SeedanceReferenceVideo{{URL: "https://media.example/video.mp4"}}
	_, _, err := buildXimeiVideoCreateRequest(&missing)
	require.ErrorContains(t, err, "duration_seconds is required")

	over := *base
	over.AudioReferences = []SeedanceReferenceAudio{
		{URL: "https://media.example/a.wav", DurationSeconds: 8},
		{URL: "https://media.example/b.wav", DurationSeconds: 8},
	}
	_, _, err = buildXimeiVideoCreateRequest(&over)
	require.ErrorContains(t, err, "must not exceed 15 seconds")
}

func TestForwardXimeiCreateKeepsUpstreamTaskPrivate(t *testing.T) {
	upstream := &huiquCapturingUpstream{status: http.StatusAccepted, reply: `{"task_id":"cstask_test_123","status":"queued","provider_route":"tc_pool"}`}
	gateway := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk_live_secret", "video_provider": VideoProviderXimei},
	}
	request := &SeedanceRequestInfo{
		Model: SeedanceXimeiSD20Model, Prompt: "安全提示词", Resolution: VideoBillingResolution720P,
		DurationSeconds: 10, AspectRatio: "16:9",
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	ginContext.Request.Header.Set("Idempotency-Key", "client-operation-123")
	forwardContext := WithSeedanceIdempotencyKey(context.Background(), "seedance-create:v1:user:7:api-key:9:client:client-operation-123")

	response, err := gateway.ForwardSeedance(forwardContext, ginContext, account, http.MethodPost, "", request)
	require.NoError(t, err)
	require.NotNil(t, response.Result)
	require.True(t, strings.HasPrefix(response.Result.ResponseID, "vidjob_"))
	require.NotContains(t, response.Result.ResponseID, "cstask")
	require.Equal(t, "cstask_test_123", response.Result.UpstreamResponseID)
	require.Equal(t, "tc_pool", response.Result.UpstreamModel)
	require.Equal(t, DefaultXimeiVideoBaseURL+ximeiVideoCreatePath, upstream.request.URL.String())
	require.Equal(t, "Bearer sk_live_secret", upstream.request.Header.Get("Authorization"))
	platformKey := upstream.request.Header.Get("Idempotency-Key")
	require.True(t, strings.HasPrefix(platformKey, ximeiPlatformIdempotencyKeyPrefix))
	require.NotEqual(t, "client-operation-123", platformKey)
	require.Equal(t, ximeiPlatformIdempotencyKey(forwardContext, upstream.body), platformKey)
	require.NotContains(t, platformKey, "sk_live_secret")
	var body map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &body))
	require.Equal(t, "tc_pool", body["provider_route"])
	require.Equal(t, "auto", body["duration"])
}

func TestSeedanceBoundTaskAccountSelectionAllowsPausedXimeiAccount(t *testing.T) {
	groupID := int64(70)
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Status: StatusDisabled, Schedulable: false, Concurrency: 2,
		Credentials: map[string]any{
			"api_key":        "secret",
			"video_provider": VideoProviderXimei,
		},
		GroupIDs: []int64{groupID},
	}
	gateway := &OpenAIGatewayService{
		accountRepo: &seedanceAccountRepoStub{accounts: map[int64]*Account{account.ID: account}},
	}

	selection, err := gateway.SeedanceBoundTaskAccountSelection(context.Background(), account.ID, &groupID)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Same(t, account, selection.Account)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, account.Concurrency, selection.WaitPlan.MaxConcurrency)

	account.GroupIDs = []int64{groupID + 1}
	selection, err = gateway.SeedanceBoundTaskAccountSelection(context.Background(), account.ID, &groupID)
	require.Error(t, err)
	require.Nil(t, selection)
}

func TestXimeiPlatformIdempotencyKeyIsStableAndOpaque(t *testing.T) {
	body := []byte(`{"prompt":"same logical request","image_urls":["https://media.example/ref.png?token=private-media-token"]}`)
	publicTaskCtx := WithSeedanceIdempotencyKey(context.Background(), "seedance-fallback-vidjob_public_123")

	first := ximeiPlatformIdempotencyKey(publicTaskCtx, body)
	second := ximeiPlatformIdempotencyKey(publicTaskCtx, append([]byte(nil), body...))
	require.Equal(t, first, second)
	require.Equal(t, first, ximeiPlatformIdempotencyKey(publicTaskCtx, []byte(`{"prompt":"same task with refreshed signed URLs"}`)))
	require.True(t, strings.HasPrefix(first, ximeiPlatformIdempotencyKeyPrefix))
	require.NotContains(t, first, "vidjob_public_123")
	require.NotContains(t, first, "private-media-token")

	otherTask := ximeiPlatformIdempotencyKey(
		WithSeedanceIdempotencyKey(context.Background(), "seedance-fallback-vidjob_public_456"),
		body,
	)
	require.NotEqual(t, first, otherTask)
}

func TestXimeiPlatformIdempotencyKeyUsesRequestContextWithoutClientKey(t *testing.T) {
	body := []byte(`{"prompt":"same request"}`)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "server-request-123")
	require.Equal(t, ximeiPlatformIdempotencyKey(ctx, body), ximeiPlatformIdempotencyKey(ctx, body))

	otherCtx := context.WithValue(context.Background(), ctxkey.RequestID, "server-request-456")
	require.NotEqual(t, ximeiPlatformIdempotencyKey(ctx, body), ximeiPlatformIdempotencyKey(otherCtx, body))
}

type ximeiSequenceUpstream struct {
	requests []*http.Request
	bodies   [][]byte
}

func (s *ximeiSequenceUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.requests = append(s.requests, request)
	if request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		s.bodies = append(s.bodies, body)
	}
	if len(s.requests) == 1 {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"status":"succeeded","content":{"video_url":"https://tdown2.ximeiedu.org/result.mp4?token=private"}}`,
			)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":  []string{"video/mp4"},
			"Content-Range": []string{"bytes 0-3/4"},
		},
		Body: io.NopCloser(bytes.NewReader([]byte("mp4!"))),
	}, nil
}

func (s *ximeiSequenceUpstream) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
}

func TestForwardXimeiContentQueriesTaskThenStreamsPrivateResult(t *testing.T) {
	stubXimeiResultDNSValidation(t, nil)
	upstream := &ximeiSequenceUpstream{}
	gateway := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk_live_secret", "video_provider": VideoProviderXimei},
	}

	response, err := gateway.ForwardSeedanceContent(context.Background(), nil, account, "cstask_test_123", "bytes=0-3")
	require.NoError(t, err)
	require.NotNil(t, response.BodyStream)
	defer func() { _ = response.BodyStream.Close() }()
	content, err := io.ReadAll(response.BodyStream)
	require.NoError(t, err)
	require.Equal(t, []byte("mp4!"), content)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, DefaultXimeiVideoBaseURL+ximeiVideoCreatePath+"/cstask_test_123", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk_live_secret", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "https://tdown2.ximeiedu.org/result.mp4?token=private", upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "bytes=0-3", upstream.requests[1].Header.Get("Range"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[1].Context()))
}

func TestValidateXimeiVideoResultURLUsesDedicatedHTTPSAllowlist(t *testing.T) {
	stubXimeiResultDNSValidation(t, nil)

	valid, err := validateXimeiVideoResultURL("https://tdown1.ximeiedu.org/results/video.mp4?token=signed")
	require.NoError(t, err)
	require.Equal(t, "https://tdown1.ximeiedu.org/results/video.mp4?token=signed", valid)

	for _, raw := range []string{
		"http://tdown1.ximeiedu.org/results/video.mp4",
		"https://evil.example/results/video.mp4",
		"https://tdown1.ximeiedu.org.evil.example/results/video.mp4",
		"https://user:password@tdown1.ximeiedu.org/results/video.mp4",
		"https://tdown1.ximeiedu.org:8443/results/video.mp4",
		"https://tdown1.ximeiedu.org/results/video.mp4#fragment",
		"https://127.0.0.1/results/video.mp4",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := validateXimeiVideoResultURL(raw)
			require.Error(t, err)
		})
	}
}

func TestValidateXimeiVideoResultURLRejectsUnsafeDNSResolution(t *testing.T) {
	stubXimeiResultDNSValidation(t, errors.New("resolved ip 127.0.0.1 is not allowed"))

	_, err := validateXimeiVideoResultURL("https://tdown2.ximeiedu.org/results/video.mp4")
	require.ErrorContains(t, err, "resolution is unsafe")
}

type ximeiRedirectUpstream struct {
	requests []*http.Request
}

func (s *ximeiRedirectUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.requests = append(s.requests, request)
	if len(s.requests) == 1 {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"status":"succeeded","content":{"video_url":"https://tdown1.ximeiedu.org/result.mp4"}}`,
			)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusFound,
		Header: http.Header{
			"Content-Type": []string{"text/html"},
			"Location":     []string{"http://127.0.0.1/private"},
		},
		Body: io.NopCloser(strings.NewReader("redirect")),
	}, nil
}

func (s *ximeiRedirectUpstream) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
}

func TestForwardXimeiContentRejectsResultRedirect(t *testing.T) {
	stubXimeiResultDNSValidation(t, nil)
	upstream := &ximeiRedirectUpstream{}
	gateway := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-secret", "video_provider": VideoProviderXimei},
	}

	_, err := gateway.ForwardSeedanceContent(context.Background(), nil, account, "cstask_test_123", "")
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadGateway, upstreamErr.StatusCode)
	require.Equal(t, []byte("video provider result redirect is not allowed"), upstreamErr.Body)
	require.Len(t, upstream.requests, 2)
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[1].Context()))
}

func stubXimeiResultDNSValidation(t *testing.T, result error) {
	t.Helper()
	previous := validateXimeiResultResolvedIP
	validateXimeiResultResolvedIP = func(string) error { return result }
	t.Cleanup(func() { validateXimeiResultResolvedIP = previous })
}

func TestNormalizeXimeiResponseRemovesProviderDetails(t *testing.T) {
	body := []byte(`{
		"task_id":"cstask_top_secret",
		"status":"succeeded",
		"provider_route":"tc_pool",
		"content":{"video_url":"https://private.example/result.mp4"},
		"data":{"id":"cstask_nested_id","job_id":"cstask_nested_job","task_id":"cstask_nested_task","product":"nangua_pool","legacy_product":"fenda_pool","note":"ximei tc_pool internal"}
	}`)
	normalized, err := NormalizeSeedanceJobForRoute(body, "vidjob_public", VideoProviderXimei, SeedanceXimeiSD20Model)
	require.NoError(t, err)
	require.Contains(t, string(normalized), `"status":"completed"`)
	require.NotContains(t, string(normalized), "tc_pool")
	require.NotContains(t, string(normalized), "fenda_pool")
	require.NotContains(t, string(normalized), "nangua_pool")
	require.NotContains(t, string(normalized), "ximei")
	require.NotContains(t, string(normalized), "cstask_")
	require.NotContains(t, string(normalized), "private.example")
	require.Contains(t, string(normalized), `/v1/videos/jobs/vidjob_public/content`)
	require.Contains(t, string(normalized), SeedanceXimeiSD20Model)
}

func TestXimeiFailedStatusUsesStablePublicError(t *testing.T) {
	body := []byte(`{
		"status":"failed",
		"task_id":"cstask_failed_secret",
		"provider_route":"kele_pool",
		"error":{"message":"ximei kele_pool rejected cstask_failed_secret","details":{"job_id":"cstask_nested"}},
		"status_message":"internal moderation rule 932",
		"diagnostic":{"code":"provider-only-code"}
	}`)

	normalized, err := NormalizeSeedanceJobForRoute(body, "vidjob_public", VideoProviderXimei, SeedanceXimeiSD20Model)
	require.NoError(t, err)
	require.Contains(t, string(normalized), `"status":"failed"`)
	require.Contains(t, string(normalized), "Video generation failed")
	require.NotContains(t, string(normalized), "cstask_")
	require.NotContains(t, string(normalized), "kele_pool")
	require.NotContains(t, string(normalized), "ximei")
	require.NotContains(t, string(normalized), "moderation rule 932")
	require.NotContains(t, string(normalized), "provider-only-code")

	official, err := BuildSeedanceOfficialTaskResponseForRoute(
		"vidjob_public", body, "https://gateway.example/v1/videos/jobs/vidjob_public/content",
		VideoProviderXimei, SeedanceXimeiSD20Model,
	)
	require.NoError(t, err)
	encoded, err := json.Marshal(official)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "Video generation failed")
	require.NotContains(t, string(encoded), "cstask_")
	require.NotContains(t, string(encoded), "kele_pool")
	require.NotContains(t, string(encoded), "ximei")
	require.NotContains(t, string(encoded), "moderation rule 932")
	require.NotContains(t, string(encoded), "provider-only-code")
}

func TestBuildXimeiEndpointURLAcceptsOriginOrAPIV3Base(t *testing.T) {
	require.Equal(t,
		"https://provider.example/api/v3/contents/generations/tasks",
		buildXimeiEndpointURL("https://provider.example", ximeiVideoCreatePath),
	)
	require.Equal(t,
		"https://provider.example/api/v3/contents/generations/tasks",
		buildXimeiEndpointURL("https://provider.example/api/v3", ximeiVideoCreatePath),
	)
}


func TestXimeiNanguaPoolSupportsThirtyTenTenMediaCaps(t *testing.T) {
	product, err := ximeiVideoProductFor(SeedanceXimeiSD25Model, VideoBillingResolution720P)
	require.NoError(t, err)
	require.Equal(t, "nangua_pool", product.Route)
	require.Equal(t, 30, product.MaxImages)
	require.Equal(t, 10, product.MaxVideos)
	require.Equal(t, 10, product.MaxAudios)

	refs := make([]SeedanceReferenceImage, 0, 30)
	for i := 0; i < 30; i++ {
		refs = append(refs, SeedanceReferenceImage{URL: fmt.Sprintf("https://media.example/ref-%02d.png", i)})
	}
	videos := make([]SeedanceReferenceVideo, 0, 10)
	for i := 0; i < 10; i++ {
		videos = append(videos, SeedanceReferenceVideo{URL: fmt.Sprintf("https://media.example/v-%02d.mp4", i), DurationSeconds: 1})
	}
	audios := make([]SeedanceReferenceAudio, 0, 10)
	for i := 0; i < 10; i++ {
		audios = append(audios, SeedanceReferenceAudio{URL: fmt.Sprintf("https://media.example/a-%02d.mp3", i), DurationSeconds: 1})
	}

	valid := &SeedanceRequestInfo{
		Model: SeedanceXimeiSD25Model, Prompt: "safe product demonstration",
		Resolution: VideoBillingResolution720P, DurationSeconds: 5, AspectRatio: "16:9",
		GenerateAudio: true, References: refs, VideoReferences: videos, AudioReferences: audios,
	}
	require.NoError(t, validateXimeiReferenceDurations(valid, product))
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooManyImages := *valid
	tooManyImages.References = append(append([]SeedanceReferenceImage{}, refs...), SeedanceReferenceImage{URL: "https://media.example/overflow.png"})
	require.ErrorContains(t, validateXimeiReferenceDurations(&tooManyImages, product), "at most 30 images")
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyImages), "at most 30 reference images")

	tooManyVideos := *valid
	tooManyVideos.VideoReferences = append(append([]SeedanceReferenceVideo{}, videos...), SeedanceReferenceVideo{URL: "https://media.example/overflow.mp4", DurationSeconds: 1})
	require.ErrorContains(t, validateXimeiReferenceDurations(&tooManyVideos, product), "at most 10 reference videos")
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyVideos), "at most 10 reference videos")

	tooManyAudios := *valid
	tooManyAudios.AudioReferences = append(append([]SeedanceReferenceAudio{}, audios...), SeedanceReferenceAudio{URL: "https://media.example/overflow.mp3", DurationSeconds: 1})
	require.ErrorContains(t, validateXimeiReferenceDurations(&tooManyAudios, product), "at most 10 reference audio files")
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyAudios), "at most 10 reference audio files")
}
