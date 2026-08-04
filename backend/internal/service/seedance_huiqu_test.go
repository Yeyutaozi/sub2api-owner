package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type huiquCapturingUpstream struct {
	request *http.Request
	body    []byte
	status  int
	reply   string
}

func (s *huiquCapturingUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = request
	if request.Body != nil {
		s.body, _ = io.ReadAll(request.Body)
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(s.reply)),
	}, nil
}

func (s *huiquCapturingUpstream) DoWithTLS(request *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, concurrency)
}

func TestVideoProviderRoutesSeedanceAccountsByRequestedModel(t *testing.T) {
	fflink := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ff"}}
	huiqu := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "hq", "video_provider": VideoProviderHuiqu}}

	require.True(t, fflink.IsModelSupported("seedance-2.0"))
	require.False(t, fflink.IsModelSupported("sd2-mx933-720-1s"))
	require.True(t, huiqu.IsModelSupported("sd2-mx933-720-1s"))
	require.True(t, huiqu.IsModelSupported("sd2-mx933-720-fast-1s"))
	require.False(t, huiqu.IsModelSupported("seedance-2.0"))
	require.True(t, IsHuiquSeedanceTaskID("hqv1_task_abc123"))
	require.False(t, IsHuiquSeedanceTaskID("vidjob_existing_fflink_task"))
}

func TestValidateHuiquVideoAccountConfiguration(t *testing.T) {
	require.NoError(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "hq-secret",
		"video_provider": VideoProviderHuiqu,
		"model_mapping": map[string]any{
			"sd2-mx933-720-1s": "sd2-mx933-720-1s",
		},
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformLTX, AccountTypeAPIKey, map[string]any{
		"api_key":        "hq-secret",
		"video_provider": VideoProviderHuiqu,
	}))
	require.Error(t, ValidateSeedanceAccountConfiguration(PlatformSeedance, AccountTypeAPIKey, map[string]any{
		"api_key":        "hq-secret",
		"video_provider": VideoProviderHuiqu,
		"model_mapping": map[string]any{
			"seedance-2.0": "seedance-2.0",
		},
	}))
	require.Equal(t, DefaultHuiquVideoBaseURL, (&Account{
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}).GetSeedanceBaseURL())
}

func TestUpstreamSeedanceTaskIDRejectsProviderMismatch(t *testing.T) {
	upstreamID, err := upstreamSeedanceTaskID(VideoProviderHuiqu, "hqv1_task_abc123")
	require.NoError(t, err)
	require.Equal(t, "task_abc123", upstreamID)

	upstreamID, err = upstreamSeedanceTaskID(VideoProviderFFLink, "vidjob_existing_fflink_task")
	require.NoError(t, err)
	require.Equal(t, "vidjob_existing_fflink_task", upstreamID)

	_, err = upstreamSeedanceTaskID(VideoProviderFFLink, "hqv1_task_abc123")
	require.ErrorContains(t, err, "cannot be forwarded through another provider")

	_, err = upstreamSeedanceTaskID(VideoProviderHuiqu, "vidjob_existing_fflink_task")
	require.ErrorContains(t, err, "does not belong to the Huiqu provider")
}

func TestSeedanceTaskAccountSelectionRequiresCurrentHuiquAccountInOriginalGroup(t *testing.T) {
	groupID := int64(70)
	newAccount := func() *Account {
		return &Account{
			ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 2,
			Credentials: map[string]any{
				"api_key":        "hq-secret",
				"video_provider": VideoProviderHuiqu,
			},
			GroupIDs: []int64{groupID},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Account)
		wantErr bool
	}{
		{name: "current huiqu account", wantErr: false},
		{name: "provider edited to fflink", mutate: func(account *Account) {
			account.Credentials["video_provider"] = VideoProviderFFLink
		}, wantErr: true},
		{name: "removed from original group", mutate: func(account *Account) {
			account.GroupIDs = []int64{groupID + 1}
		}, wantErr: true},
		{name: "inactive", mutate: func(account *Account) {
			account.Status = StatusDisabled
		}, wantErr: true},
		{name: "not schedulable", mutate: func(account *Account) {
			account.Schedulable = false
		}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account := newAccount()
			if test.mutate != nil {
				test.mutate(account)
			}
			gateway := &OpenAIGatewayService{
				accountRepo: &seedanceAccountRepoStub{accounts: map[int64]*Account{account.ID: account}},
			}
			selection, err := gateway.SeedanceTaskAccountSelection(context.Background(), account.ID, &groupID)
			if test.wantErr {
				require.Error(t, err)
				require.Nil(t, selection)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, account.ID, selection.Account.ID)
			require.NotNil(t, selection.WaitPlan)
			require.Equal(t, account.Concurrency, selection.WaitPlan.MaxConcurrency)
		})
	}
}

func TestParseHuiquRequestAllowsMixedReferenceMedia(t *testing.T) {
	request, err := ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"sd2-mx933-720-1s",
		"prompt":"Keep the subject consistent and move the camera forward",
		"duration":10,
		"resolution":"720p",
		"aspect_ratio":"3:2",
		"audio":true,
		"start_frame_url":"https://media.example/first.png",
		"end_frame_url":"https://media.example/last.png",
		"guidances":{
			"image_reference":[{"image":{"url":"https://media.example/ref.png"}}],
			"video_reference_base":[{"video":{"url":"https://media.example/motion.mp4"}}],
			"audio_reference":[{"audio":{"url":"https://media.example/dialogue.wav"}}]
		}
	}`))
	require.NoError(t, err)
	require.Len(t, request.References, 1)
	require.Len(t, request.VideoReferences, 1)
	require.Len(t, request.AudioReferences, 1)
	require.True(t, request.GenerateAudio)

	_, err = ParseSeedanceVideoGenerationRequest([]byte(`{
		"model":"sd2-mx933-720-fast-1s",
		"prompt":"Animate",
		"prompt_enhance":true
	}`))
	require.ErrorContains(t, err, "does not support prompt_enhance")
}

func TestHuiquTextRequestUsesSeconds(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           "sd2-mx933-720-1s",
		Prompt:          "A city at night",
		Resolution:      "480p",
		DurationSeconds: 10,
		AspectRatio:     "2:3",
		GenerateAudio:   true,
	}
	body, err := request.HuiquUpstreamBody(request.Model)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"sd2-mx933-720-1s",
		"prompt":"A city at night",
		"seconds":10,
		"aspect_ratio":"2:3",
		"resolution":"480p",
		"generate_audio":true
	}`, string(body))
	require.NotContains(t, string(body), `"duration"`)
	require.NotContains(t, string(body), `"audio"`)
}

func TestBuildHuiquMultipartBodyUsesDocumentedFields(t *testing.T) {
	image := huiquTestMediaFile(t, "reference.png", "image/png", []byte("image-bytes"))
	video := huiquTestMediaFile(t, "motion.mp4", "video/mp4", []byte("video-bytes"))
	audio := huiquTestMediaFile(t, "voice.wav", "audio/wav", []byte("audio-bytes"))
	request := &SeedanceRequestInfo{
		Model:           "sd2-mx933-720-fast-1s",
		Prompt:          "Use the reference voice exactly",
		Resolution:      "720p",
		DurationSeconds: 10,
		AspectRatio:     "9:16",
		GenerateAudio:   true,
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			Images: []SeedanceHuiquMediaFile{image},
			Videos: []SeedanceHuiquMediaFile{video},
			Audios: []SeedanceHuiquMediaFile{audio},
		},
	}
	body, err := buildHuiquMultipartBody(request, request.Model)
	require.NoError(t, err)
	defer body.Close()

	mediaType, params, err := mime.ParseMediaType(body.ContentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(body.File, params["boundary"])
	values := map[string][]string{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		payload, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		values[part.FormName()] = append(values[part.FormName()], string(payload))
	}
	require.Equal(t, []string{"sd2-mx933-720-fast-1s"}, values["model"])
	require.Equal(t, []string{"10"}, values["seconds"])
	require.NotContains(t, values, "duration")
	require.Equal(t, []string{"true"}, values["generate_audio"])
	require.Equal(t, []string{"image-bytes"}, values["images"])
	require.Equal(t, []string{"video-bytes"}, values["videos"])
	require.Equal(t, []string{"audio-bytes"}, values["audios"])
}

func TestInspectHuiquMediaNormalizesCommonWAVMIMETypes(t *testing.T) {
	for _, declaredType := range []string{"audio/wave", "audio/vnd.wave", "audio/x-wav"} {
		t.Run(declaredType, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "reference-*.wav")
			require.NoError(t, err)
			defer file.Close()
			_, err = file.Write([]byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x44\xac\x00\x00\x88\x58\x01\x00\x02\x00\x10\x00data\x00\x00\x00\x00"))
			require.NoError(t, err)

			contentType, extension, err := inspectHuiquMedia(file, declaredType, "reference.wav", "audio")
			require.NoError(t, err)
			require.Equal(t, "audio/wav", contentType)
			require.Equal(t, "wav", extension)
		})
	}
}

func TestForwardHuiquUsesProviderPathsAndOpaquePublicTaskID(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_abc123","status":"queued"}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}
	request := &SeedanceRequestInfo{
		Model: "sd2-mx933-720-1s", Prompt: "A coastal sunrise",
		Resolution: "720p", DurationSeconds: 5, AspectRatio: "16:9",
	}
	response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", request)
	require.NoError(t, err)
	require.Equal(t, DefaultHuiquVideoBaseURL+"/v1/videos/generations", upstream.request.URL.String())
	require.Empty(t, upstream.request.Header.Get("Prefer"))
	require.JSONEq(t, `{"model":"sd2-mx933-720-1s","prompt":"A coastal sunrise","seconds":5,"aspect_ratio":"16:9","resolution":"720p","generate_audio":false}`, string(upstream.body))
	require.NotContains(t, string(upstream.body), `"duration"`)
	require.Equal(t, "hqv1_task_abc123", response.Result.ResponseID)

	upstream.reply = `{"id":"task_abc123","status":"completed","video_url":"https://upstream.example/video.mp4"}`
	_, err = service.ForwardSeedance(context.Background(), nil, account, http.MethodGet, response.Result.ResponseID, nil)
	require.NoError(t, err)
	require.Equal(t, DefaultHuiquVideoBaseURL+"/v1/videos/task_abc123", upstream.request.URL.String())
}

func TestForwardHuiquMultipartUsesSecondsAndRepeatedMediaFields(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_multipart123","status":"queued"}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}
	request := &SeedanceRequestInfo{
		Model: "sd2-mx933-720-fast-1s", Prompt: "Keep all reference media consistent",
		Resolution: "720p", DurationSeconds: 10, AspectRatio: "9:16", GenerateAudio: true,
		References: []SeedanceReferenceImage{
			{URL: "https://media.example/reference-1.png"},
			{URL: "https://media.example/reference-2.png"},
		},
		VideoReferences: []SeedanceReferenceVideo{
			{URL: "https://media.example/motion-1.mp4"},
			{URL: "https://media.example/motion-2.mp4"},
		},
		AudioReferences: []SeedanceReferenceAudio{
			{URL: "https://media.example/voice-1.wav"},
			{URL: "https://media.example/voice-2.wav"},
		},
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			Images: []SeedanceHuiquMediaFile{
				huiquTestMediaFile(t, "reference-1.png", "image/png", []byte("image-one")),
				huiquTestMediaFile(t, "reference-2.png", "image/png", []byte("image-two")),
			},
			Videos: []SeedanceHuiquMediaFile{
				huiquTestMediaFile(t, "motion-1.mp4", "video/mp4", []byte("video-one")),
				huiquTestMediaFile(t, "motion-2.mp4", "video/mp4", []byte("video-two")),
			},
			Audios: []SeedanceHuiquMediaFile{
				huiquTestMediaFile(t, "voice-1.wav", "audio/wav", []byte("audio-one")),
				huiquTestMediaFile(t, "voice-2.wav", "audio/wav", []byte("audio-two")),
			},
		},
	}

	response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", request)
	require.NoError(t, err)
	require.Equal(t, DefaultHuiquVideoBaseURL+"/v1/videos/generations", upstream.request.URL.String())
	require.Equal(t, "multipart/form-data", strings.SplitN(upstream.request.Header.Get("Content-Type"), ";", 2)[0])

	mediaType, params, err := mime.ParseMediaType(upstream.request.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(upstream.body), params["boundary"])
	values := map[string][]string{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		payload, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		values[part.FormName()] = append(values[part.FormName()], string(payload))
	}
	require.Equal(t, []string{"sd2-mx933-720-fast-1s"}, values["model"])
	require.Equal(t, []string{"10"}, values["seconds"])
	require.NotContains(t, values, "duration")
	require.Equal(t, []string{"true"}, values["generate_audio"])
	require.Equal(t, []string{"image-one", "image-two"}, values["images"])
	require.Equal(t, []string{"video-one", "video-two"}, values["videos"])
	require.Equal(t, []string{"audio-one", "audio-two"}, values["audios"])
	require.Equal(t, "hqv1_task_multipart123", response.Result.ResponseID)
}

func TestForwardHuiquSanitizesUpstreamErrorBody(t *testing.T) {
	upstream := &huiquCapturingUpstream{
		status: http.StatusBadRequest,
		reply:  `{"error":{"message":"download https://api.bjhuiqu.net/internal.mp4?X-Amz-Signature=secret token=secret-token","accessToken":"secret-access-token","details":{"videoUrl":"https://cdn.huiqu.example/output.mp4"}}}`,
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}

	_, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodGet, "hqv1_task_abc123", nil)
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.NotContains(t, string(upstreamErr.Body), "bjhuiqu")
	require.NotContains(t, string(upstreamErr.Body), "huiqu.example")
	require.NotContains(t, string(upstreamErr.Body), "secret")
	require.NotContains(t, string(upstreamErr.Body), "accessToken")
	require.Contains(t, string(upstreamErr.Body), "[redacted-url]")
}

func TestForwardFFLinkKeepsExistingUpstreamErrorBehavior(t *testing.T) {
	upstream := &huiquCapturingUpstream{
		status: http.StatusBadRequest,
		reply:  `{"error":{"message":"download https://upstream.example/internal.mp4 token=legacy-value"}}`,
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 43, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "ff-secret", "base_url": "https://api.fflink.top"},
	}

	_, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodGet, "vidjob_abc123", nil)
	var upstreamErr *SeedanceUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Contains(t, string(upstreamErr.Body), "https://upstream.example/internal.mp4")
	require.Contains(t, string(upstreamErr.Body), "token=legacy-value")
}

func TestNormalizeSeedanceJobCreatesPublicResultForHuiquResponse(t *testing.T) {
	normalized, err := NormalizeSeedanceJob(
		[]byte(`{"id":"task_abc123","status":"completed","video_url":"https://api.bjhuiqu.net/video_cache/internal.mp4","download_url":"https://ycyapi.cn/video/internal.mp4","result":{"data":[{"download_url":"https://ycyapi.cn/video/internal.mp4"}]}}`),
		"hqv1_task_abc123",
	)
	require.NoError(t, err)
	require.NotContains(t, string(normalized), "api.bjhuiqu.net")
	require.NotContains(t, string(normalized), "ycyapi.cn")
	require.JSONEq(t, `{
		"id":"hqv1_task_abc123",
		"job_id":"hqv1_task_abc123",
		"task_id":"hqv1_task_abc123",
		"status":"completed",
		"status_url":"/v1/videos/jobs/hqv1_task_abc123",
		"video_url":"/v1/videos/jobs/hqv1_task_abc123/content",
		"download_url":"/v1/videos/jobs/hqv1_task_abc123/content",
		"result":{"data":[{
			"mp4_url":"/v1/videos/jobs/hqv1_task_abc123/content",
			"url":"/v1/videos/jobs/hqv1_task_abc123/content",
			"local_url":"/v1/videos/jobs/hqv1_task_abc123/content",
			"download_url":"/v1/videos/jobs/hqv1_task_abc123/content"
		}]}
	}`, string(normalized))
}

func TestNormalizeSeedanceJobRecursivelyRemovesHuiquURLsAndSigningFields(t *testing.T) {
	normalized, err := NormalizeSeedanceJob(
		[]byte(`{
			"id":"task_abc123",
			"status":"completed",
			"callback_url":"https://api.bjhuiqu.net/callback/task_abc123",
			"content":{
				"video_url":"https://cdn.huiqu.example/output.mp4?X-Amz-Signature=secret",
				"thumbnail_url":"https://cdn.huiqu.example/thumb.jpg",
				"metadata":{
					"task_url":"https://api.bjhuiqu.net/v1/videos/task_abc123",
					"signature":"secret-signature",
					"nested":[{"content_url":"https://cdn.huiqu.example/output.mp4"},"https://cdn.huiqu.example/leak.mp4"]
				}
			},
			"result":{"data":[{
				"url":"https://cdn.huiqu.example/output.mp4",
				"cover_url":"https://cdn.huiqu.example/cover.jpg",
				"token":"secret-token",
				"signing":{"X-Amz-Credential":"secret-credential","download_url":"https://cdn.huiqu.example/output.mp4"}
			}]}
		}`),
		"hqv1_task_abc123",
	)
	require.NoError(t, err)
	require.NotContains(t, string(normalized), "bjhuiqu")
	require.NotContains(t, string(normalized), "huiqu.example")
	require.NotContains(t, string(normalized), "secret")
	require.NotContains(t, string(normalized), "X-Amz")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(normalized, &payload))
	require.NotContains(t, payload, "callback_url")
	content := payload["content"].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123/content", content["video_url"])
	require.NotContains(t, content, "thumbnail_url")
	metadata := content["metadata"].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123", metadata["task_url"])
	require.NotContains(t, metadata, "signature")
	nested := metadata["nested"].([]any)
	require.Len(t, nested, 1)
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123/content", nested[0].(map[string]any)["content_url"])

	result := payload["result"].(map[string]any)
	file := result["data"].([]any)[0].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123/content", file["mp4_url"])
	require.Equal(t, file["mp4_url"], file["url"])
	require.Equal(t, file["mp4_url"], file["local_url"])
	require.NotContains(t, file, "cover_url")
	require.NotContains(t, file, "token")
	signing := file["signing"].(map[string]any)
	require.NotContains(t, signing, "X-Amz-Credential")
	require.Equal(t, file["mp4_url"], signing["download_url"])
}

func TestNormalizeSeedanceJobHandlesHuiquCamelCaseFields(t *testing.T) {
	normalized, err := NormalizeSeedanceJob(
		[]byte(`{"status":"completed","statusUrl":"https://api.bjhuiqu.net/v1/videos/task_abc123","videoUrl":"https://cdn.huiqu.example/output.mp4","accessToken":"secret-token","signatureValue":"secret-signature","result":{"data":[{"videoUrl":"https://cdn.huiqu.example/output.mp4"}]}}`),
		"hqv1_task_abc123",
	)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(normalized, &payload))
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123", payload["statusUrl"])
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123/content", payload["videoUrl"])
	require.NotContains(t, payload, "accessToken")
	require.NotContains(t, payload, "signatureValue")
	result := payload["result"].(map[string]any)
	file := result["data"].([]any)[0].(map[string]any)
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123/content", file["videoUrl"])
}

func TestBuildSeedanceOfficialTaskResponseSanitizesHuiquFailure(t *testing.T) {
	response, err := BuildSeedanceOfficialTaskResponse(
		"hqv1_task_abc123",
		[]byte(`{"status":"failed","model":"https://api.bjhuiqu.net/internal-model","error":{"message":"upstream failed","statusUrl":"https://api.bjhuiqu.net/task_abc123?X-Amz-Signature=secret","token":"secret-token","details":["https://cdn.huiqu.example/internal",{"signature":"secret-signature"}]}}`),
		"/v1/videos/jobs/hqv1_task_abc123/content",
	)
	require.NoError(t, err)
	require.NotContains(t, string(mustMarshalJSON(t, response)), "bjhuiqu")
	require.NotContains(t, string(mustMarshalJSON(t, response)), "secret")
	require.NotContains(t, response, "model")
	errorValue := response["error"].(map[string]any)
	require.Equal(t, "upstream failed", errorValue["message"])
	require.Equal(t, "/v1/videos/jobs/hqv1_task_abc123", errorValue["statusUrl"])
	require.NotContains(t, errorValue, "token")
	require.NotContains(t, errorValue, "details")
}

func TestBuildSeedanceOfficialTaskResponseMapsHuiquFields(t *testing.T) {
	response, err := BuildSeedanceOfficialTaskResponse(
		"hqv1_task_abc123",
		[]byte(`{"id":"task_abc123","status":"completed","model":"sd2-mx933-720-1s","seconds":15,"aspect_ratio":"3:2"}`),
		"https://gateway.example/v1/videos/jobs/hqv1_task_abc123/content",
	)
	require.NoError(t, err)
	require.EqualValues(t, 15, response["duration"])
	require.Equal(t, "3:2", response["ratio"])
}

func huiquTestMediaFile(t *testing.T, filename, contentType string, payload []byte) SeedanceHuiquMediaFile {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "huiqu-test-*")
	require.NoError(t, err)
	_, err = file.Write(payload)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	return SeedanceHuiquMediaFile{Path: file.Name(), Filename: filename, ContentType: contentType, SizeBytes: int64(len(payload))}
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}
