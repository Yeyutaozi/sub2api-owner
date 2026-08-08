//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3ModelProfileAndCatalog(t *testing.T) {
	models := FFLinkVideoModelIDsForPlatform(PlatformMiniMax)
	require.Contains(t, models, SeedanceMiniMaxH3Model)
	require.True(t, isHuiquVideoModel(SeedanceMiniMaxH3Model))
	require.True(t, isHuiquMiniMaxH3Model(SeedanceMiniMaxH3UpstreamModel))
	require.True(t, videoProviderSupportsModel(VideoProviderHuiqu, SeedanceMiniMaxH3Model))
	require.True(t, videoProviderSupportsModelForPlatform(PlatformMiniMax, VideoProviderHuiqu, SeedanceMiniMaxH3Model))
	require.False(t, videoProviderSupportsModelForPlatform(PlatformSeedance, VideoProviderHuiqu, SeedanceMiniMaxH3Model))
	require.False(t, videoProviderSupportsModel(VideoProviderFFLink, SeedanceMiniMaxH3Model))
	require.False(t, videoProviderSupportsModelForPlatform(PlatformMiniMax, VideoProviderFFLink, SeedanceMiniMaxH3Model))
	require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformMiniMax, SeedanceMiniMaxH3Model))
	require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, SeedanceMiniMaxH3Model))

	profile, ok := ffLinkVideoModelProfileFor(SeedanceMiniMaxH3Model)
	require.True(t, ok)
	require.Equal(t, VideoBillingResolution1440P, profile.DefaultResolution)
	require.Equal(t, 8, profile.DefaultDuration)
	require.Equal(t, 5, profile.MaxImageReferences)
	require.Equal(t, 0, profile.MaxVideoReferences)
	require.Equal(t, 3, profile.MaxAudioReferences)
	require.True(t, profile.AllowGeneratedAudio)
	require.True(t, profile.ValidateDuration(8, VideoBillingResolution1440P))
	require.False(t, profile.ValidateDuration(4, VideoBillingResolution1440P))
	require.False(t, profile.ValidateDuration(16, VideoBillingResolution1440P))
	require.Equal(t, SeedanceMiniMaxH3Model, PublicSeedanceModelID(SeedanceMiniMaxH3UpstreamModel))
	require.Equal(t, SeedanceMiniMaxH3Model, PublicSeedanceModelID(" MiniMax-H3-933-1440P-GF "))
}

func TestMiniMaxH3UpstreamModelMapping(t *testing.T) {
	for _, duration := range []int{5, 8, 12, 15} {
		got, err := huiquUpstreamModelFor(SeedanceMiniMaxH3Model, duration)
		require.NoError(t, err)
		require.Equal(t, SeedanceMiniMaxH3UpstreamModel, got)
	}
	_, err := huiquUpstreamModelFor(SeedanceMiniMaxH3Model, 4)
	require.ErrorContains(t, err, "duration 4 is not supported")
	_, err = huiquUpstreamModelFor(SeedanceMiniMaxH3Model, 16)
	require.ErrorContains(t, err, "duration 16 is not supported")

	got, err := huiquUpstreamModelFor(SeedanceMiniMaxH3UpstreamModel, 9)
	require.NoError(t, err)
	require.Equal(t, SeedanceMiniMaxH3UpstreamModel, got)
}

func TestMiniMaxH3TextRequestBodyUsesAudioAndSize(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "A cinematic tracking shot through a lively night market",
		Resolution:      VideoBillingResolution1440P,
		DurationSeconds: 8,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
	}
	upstreamModel, err := huiquUpstreamModelFor(request.Model, request.DurationSeconds)
	require.NoError(t, err)
	body, err := request.HuiquUpstreamBody(upstreamModel)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"MiniMax-H3-933-1440P-GF",
		"prompt":"A cinematic tracking shot through a lively night market",
		"seconds":8,
		"aspect_ratio":"16:9",
		"resolution":"1440P",
		"size":"2560x1440",
		"audio":true
	}`, string(body))
	require.NotContains(t, string(body), "generate_audio")

	request.AspectRatio = "9:16"
	body, err = request.HuiquUpstreamBody(upstreamModel)
	require.NoError(t, err)
	require.Contains(t, string(body), `"size":"1440x2560"`)
}

func TestMiniMaxH3JSONUsesDocumentedReferenceFieldNames(t *testing.T) {
	image := huiquTestMediaFile(t, "reference.png", "image/png", []byte("image-bytes"))
	audio := huiquTestMediaFile(t, "voice.wav", "audio/wav", []byte("audio-bytes"))
	request := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "Match the reference character voice",
		Resolution:      VideoBillingResolution1440P,
		DurationSeconds: 7,
		AspectRatio:     "9:16",
		GenerateAudio:   true,
		// Mark request as having reference media so body builder includes them.
		References: []SeedanceReferenceImage{{URL: "https://example.invalid/ref.png"}},
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			Images: []SeedanceHuiquMediaFile{image},
			Audios: []SeedanceHuiquMediaFile{audio},
		},
	}
	upstreamModel, err := huiquUpstreamModelFor(request.Model, request.DurationSeconds)
	require.NoError(t, err)
	raw, err := request.HuiquUpstreamBody(upstreamModel)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	require.Equal(t, SeedanceMiniMaxH3UpstreamModel, body["model"])
	require.EqualValues(t, 7, body["seconds"])
	require.Equal(t, "1440P", body["resolution"])
	require.Equal(t, "1440x2560", body["size"])
	require.Equal(t, true, body["audio"])
	require.NotContains(t, body, "generate_audio")
	require.NotContains(t, body, "images")
	require.NotContains(t, body, "audios")
	require.NotContains(t, body, "videos")

	images, ok := body["reference_images"].([]any)
	require.True(t, ok)
	require.Len(t, images, 1)
	require.True(t, strings.HasPrefix(images[0].(string), "data:image/png;base64,"))

	audios, ok := body["audio_reference"].([]any)
	require.True(t, ok)
	require.Len(t, audios, 1)
	require.True(t, strings.HasPrefix(audios[0].(string), "data:audio/wav;base64,"))
}

func TestMiniMaxH3JSONStartEndFrames(t *testing.T) {
	start := huiquTestMediaFile(t, "start.png", "image/png", []byte("start-bytes"))
	endFrame := huiquTestMediaFile(t, "end.png", "image/png", []byte("end-bytes"))
	request := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "Interpolate between frames",
		Resolution:      VideoBillingResolution1440P,
		DurationSeconds: 6,
		AspectRatio:     "16:9",
		GenerateAudio:   false,
		StartFrameURL:   "https://example.invalid/start.png",
		EndFrameURL:     "https://example.invalid/end.png",
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			FirstFrame: &start,
			LastFrame:  &endFrame,
		},
	}
	raw, err := request.HuiquUpstreamBody(SeedanceMiniMaxH3UpstreamModel)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	require.True(t, strings.HasPrefix(body["start_frame"].(string), "data:image/png;base64,"))
	require.True(t, strings.HasPrefix(body["end_frame"].(string), "data:image/png;base64,"))
	require.NotContains(t, body, "first_frame")
	require.NotContains(t, body, "last_frame")
	require.Equal(t, true, body["audio"])
}

func TestForwardHuiquMiniMaxH3ReferenceMediaUsesJSONNotMultipart(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_h3_ref","status":"queued"}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}
	start := huiquTestMediaFile(t, "start.png", "image/png", []byte("start-bytes"))
	request := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "from first frame",
		Resolution: VideoBillingResolution1440P, DurationSeconds: 5, AspectRatio: "16:9",
		StartFrameURL: "https://example.invalid/start.png",
		HuiquMedia:    &SeedanceHuiquPreparedMedia{FirstFrame: &start},
	}
	response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", request)
	require.NoError(t, err)
	require.Equal(t, "hqv1_task_h3_ref", response.Result.ResponseID)
	require.Equal(t, "application/json", upstream.request.Header.Get("Content-Type"))
	require.NotContains(t, upstream.request.Header.Get("Content-Type"), "multipart")

	var body map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &body))
	require.Equal(t, SeedanceMiniMaxH3UpstreamModel, body["model"])
	require.True(t, strings.HasPrefix(body["start_frame"].(string), "data:image/png;base64,"))
}

func TestMiniMaxH3ForcesNativeAudio(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "night market",
		DurationSeconds: 8,
		GenerateAudio:   false,
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(info))
	require.True(t, info.GenerateAudio)
}

func TestMiniMaxH3RequestValidationModes(t *testing.T) {
	textOnly := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "night market",
		DurationSeconds: 8,
		GenerateAudio:   true,
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(textOnly))
	require.Equal(t, VideoBillingResolution1440P, textOnly.Resolution)
	require.Equal(t, "16:9", textOnly.AspectRatio)

	tooLong := &SeedanceRequestInfo{Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 16}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooLong), "duration 16 is not supported")

	badRes := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8, Resolution: VideoBillingResolution720P,
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(badRes), "resolution")

	frames := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8,
		StartFrameURL: "https://media.example/start.png",
		EndFrameURL:   "https://media.example/end.png",
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(frames))

	endOnly := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8,
		EndFrameURL: "https://media.example/end.png",
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(endOnly), "a first frame is required when a last frame is provided")

	mixed := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8,
		StartFrameURL: "https://media.example/start.png",
		References:    []SeedanceReferenceImage{{URL: "https://media.example/ref.png"}},
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(mixed), "cannot be combined")

	audioOnly := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8, GenerateAudio: true,
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://media.example/a.wav"}},
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(audioOnly), "requires reference images")

	modeB := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8, GenerateAudio: true,
		References:      make([]SeedanceReferenceImage, 5),
		AudioReferences: make([]SeedanceReferenceAudio, 3),
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(modeB))

	tooManyImages := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8,
		References: make([]SeedanceReferenceImage, 6),
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooManyImages), "at most 5 reference images")

	withVideo := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "x", DurationSeconds: 8,
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://media.example/v.mp4"}},
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(withVideo), "at most 0 reference videos")
}

func TestMiniMaxH3DoesNotBreakMX933BodyFields(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           SeedanceMX933Model,
		Prompt:          "A city at night",
		Resolution:      "480p",
		DurationSeconds: 10,
		AspectRatio:     "2:3",
		GenerateAudio:   true,
	}
	upstreamModel, err := huiquUpstreamModelFor(request.Model, request.DurationSeconds)
	require.NoError(t, err)
	body, err := request.HuiquUpstreamBody(upstreamModel)
	require.NoError(t, err)
	require.Contains(t, string(body), `"generate_audio":true`)
	require.NotContains(t, string(body), `"audio":`)
	require.NotContains(t, string(body), `"size"`)
	_ = strings.Builder{}
}


func TestForwardHuiquMiniMaxH3UsesVideosCreatePath(t *testing.T) {
	upstream := &huiquCapturingUpstream{reply: `{"id":"task_h3_abc","status":"queued"}`}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":        "hq-secret",
			"video_provider": VideoProviderHuiqu,
		},
	}
	request := &SeedanceRequestInfo{
		Model: SeedanceMiniMaxH3Model, Prompt: "A cinematic tracking shot through a lively night market",
		Resolution: VideoBillingResolution1440P, DurationSeconds: 8, AspectRatio: "16:9", GenerateAudio: true,
	}
	response, err := service.ForwardSeedance(context.Background(), nil, account, http.MethodPost, "", request)
	require.NoError(t, err)
	require.Equal(t, DefaultHuiquVideoBaseURL+"/v1/videos", upstream.request.URL.String())
	require.Equal(t, "hqv1_task_h3_abc", response.Result.ResponseID)

	var body map[string]any
	require.NoError(t, json.Unmarshal(upstream.body, &body))
	require.Equal(t, SeedanceMiniMaxH3UpstreamModel, body["model"])
	require.EqualValues(t, 8, body["seconds"])
	require.Equal(t, "1440P", body["resolution"])
	require.Equal(t, "2560x1440", body["size"])
	require.Equal(t, true, body["audio"])
	require.NotContains(t, body, "generate_audio")
}
