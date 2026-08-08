//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3ModelProfileAndCatalog(t *testing.T) {
	models := FFLinkVideoModelIDsForPlatform(PlatformSeedance)
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

func TestMiniMaxH3MultipartUsesDocumentedFieldNames(t *testing.T) {
	image := huiquTestMediaFile(t, "reference.png", "image/png", []byte("image-bytes"))
	audio := huiquTestMediaFile(t, "voice.wav", "audio/wav", []byte("audio-bytes"))
	request := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "Match the reference character voice",
		Resolution:      VideoBillingResolution1440P,
		DurationSeconds: 7,
		AspectRatio:     "9:16",
		GenerateAudio:   true,
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			Images: []SeedanceHuiquMediaFile{image},
			Audios: []SeedanceHuiquMediaFile{audio},
		},
	}
	upstreamModel, err := huiquUpstreamModelFor(request.Model, request.DurationSeconds)
	require.NoError(t, err)
	body, err := buildHuiquMultipartBody(request, upstreamModel)
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
	require.Equal(t, []string{SeedanceMiniMaxH3UpstreamModel}, values["model"])
	require.Equal(t, []string{"7"}, values["seconds"])
	require.Equal(t, []string{"1440P"}, values["resolution"])
	require.Equal(t, []string{"1440x2560"}, values["size"])
	require.Equal(t, []string{"true"}, values["audio"])
	require.Equal(t, []string{"image-bytes"}, values["reference_images"])
	require.Equal(t, []string{"audio-bytes"}, values["audio_reference"])
	require.NotContains(t, values, "generate_audio")
	require.NotContains(t, values, "images")
	require.NotContains(t, values, "audios")
	require.NotContains(t, values, "videos")
}

func TestMiniMaxH3MultipartStartEndFrames(t *testing.T) {
	start := huiquTestMediaFile(t, "start.png", "image/png", []byte("start-bytes"))
	end := huiquTestMediaFile(t, "end.png", "image/png", []byte("end-bytes"))
	request := &SeedanceRequestInfo{
		Model:           SeedanceMiniMaxH3Model,
		Prompt:          "Interpolate between frames",
		Resolution:      VideoBillingResolution1440P,
		DurationSeconds: 6,
		AspectRatio:     "16:9",
		GenerateAudio:   false,
		HuiquMedia: &SeedanceHuiquPreparedMedia{
			FirstFrame: &start,
			LastFrame:  &end,
		},
	}
	body, err := buildHuiquMultipartBody(request, SeedanceMiniMaxH3UpstreamModel)
	require.NoError(t, err)
	defer body.Close()

	_, params, err := mime.ParseMediaType(body.ContentType)
	require.NoError(t, err)
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
	require.Equal(t, []string{"start-bytes"}, values["start_frame"])
	require.Equal(t, []string{"end-bytes"}, values["end_frame"])
	require.NotContains(t, values, "first_frame")
	require.NotContains(t, values, "last_frame")
	require.Equal(t, []string{"true"}, values["audio"])
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
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(endOnly), "requires a first frame")

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
