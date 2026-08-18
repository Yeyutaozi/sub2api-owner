//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMX933ModelProfiles(t *testing.T) {
	models := FFLinkVideoModelIDsForPlatform(PlatformSeedance)
	require.Contains(t, models, SeedanceMX933Model)
	require.Contains(t, models, SeedanceMX933FastModel)
	require.NotContains(t, models, SeedanceMX933LegacyModel)
	require.NotContains(t, models, SeedanceMX933LegacyFastModel)

	for _, model := range []string{SeedanceMX933Model, SeedanceMX933FastModel, SeedanceMX933LegacyModel, SeedanceMX933LegacyFastModel} {
		require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, model))
		require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformLTX, model))
	}
}

func TestPublicSeedanceModelIDHidesMX933ProviderTiers(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{SeedanceMX933Model, SeedanceMX933Model},
		{" SD2-MX933-720-1S ", SeedanceMX933Model},
		{"sd2-mx933-720-10s", SeedanceMX933Model},
		{SeedanceMX933FastModel, SeedanceMX933FastModel},
		{" SD2-MX933-720-FAST-1S ", SeedanceMX933FastModel},
		{"sd2-mx933-720-fast-15s", SeedanceMX933FastModel},
		{"seedance-2.0", "seedance-2.0"},
	} {
		require.Equal(t, test.want, PublicSeedanceModelID(test.input))
	}
}

func TestNewSeedanceRequestsRejectLegacyMX933ModelIDs(t *testing.T) {
	for _, model := range []string{" SD2-MX933-720-1S ", "sd2-mx933-720-fast-1s"} {
		_, err := ParseSeedanceVideoGenerationRequest([]byte(fmt.Sprintf(`{
			"model":%q,
			"prompt":"legacy public request",
			"duration":5,
			"resolution":"720p",
			"aspect_ratio":"16:9"
		}`, model)))
		require.ErrorContains(t, err, "unsupported video model")

		_, err = ParseSeedanceCreateRequest([]byte(fmt.Sprintf(`{
			"model":%q,
			"content":[{"type":"text","text":"legacy task request"}],
			"duration":5,
			"resolution":"720p",
			"ratio":"16:9"
		}`, model)))
		require.ErrorContains(t, err, "unsupported video model")
	}
}

func TestMX933RequestValidation(t *testing.T) {
	for _, model := range []string{SeedanceMX933Model, SeedanceMX933FastModel} {
		t.Run(model, func(t *testing.T) {
			for _, duration := range []int{5, 10, 15} {
				valid := mx933RequestInfo(model)
				valid.Resolution = VideoBillingResolution720P
				valid.DurationSeconds = duration
				valid.AspectRatio = "2:3"
				require.NoError(t, validateFFLinkVideoRequestInfo(valid))
			}

			for _, duration := range []int{-1, 1, 4, 6, 8, 14, 16} {
				invalid := mx933RequestInfo(model)
				invalid.DurationSeconds = duration
				require.ErrorContains(t, validateFFLinkVideoRequestInfo(invalid), fmt.Sprintf("duration %d is not supported", duration))
			}

			invalidResolution := mx933RequestInfo(model)
			invalidResolution.Resolution = VideoBillingResolution1080P
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(invalidResolution), "resolution 1080p is not supported")

			invalidRatio := mx933RequestInfo(model)
			invalidRatio.AspectRatio = "21:9"
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(invalidRatio), "aspect_ratio 21:9 is not supported")

			generatedAudio := mx933RequestInfo(model)
			generatedAudio.GenerateAudio = true
			require.NoError(t, validateFFLinkVideoRequestInfo(generatedAudio))

			tooManyVideoReferences := mx933RequestInfo(model)
			tooManyVideoReferences.VideoReferences = append(tooManyVideoReferences.VideoReferences, SeedanceReferenceVideo{})
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooManyVideoReferences), "at most 3 reference videos")

			tooManyAudioReferences := mx933RequestInfo(model)
			tooManyAudioReferences.AudioReferences = append(tooManyAudioReferences.AudioReferences, SeedanceReferenceAudio{})
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooManyAudioReferences), "at most 3 reference audio files")

			// 参考音频可单独上传
			audioOnly := mx933RequestInfo(model)
			audioOnly.References = nil
			audioOnly.StartFrameURL = ""
			audioOnly.EndFrameURL = ""
			audioOnly.VideoReferences = nil
			audioOnly.GenerateAudio = true
			audioOnly.AudioReferences = make([]SeedanceReferenceAudio, 1)
			require.NoError(t, validateFFLinkVideoRequestInfo(audioOnly))
		})
	}
}

func TestSeedanceModelsOnlyAcceptFixedDurationTiers(t *testing.T) {
	for _, model := range []string{"seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini", SeedanceMX933Model, SeedanceMX933FastModel} {
		for _, duration := range []int{5, 10, 15} {
			info := &SeedanceRequestInfo{Model: model, Prompt: "duration tier", Resolution: VideoBillingResolution720P, DurationSeconds: duration, AspectRatio: "16:9"}
			require.NoError(t, validateFFLinkVideoRequestInfo(info), model+"/"+fmt.Sprint(duration))
		}
		for _, duration := range []int{4, 6, 8, 12, 14} {
			info := &SeedanceRequestInfo{Model: model, Prompt: "duration tier", Resolution: VideoBillingResolution720P, DurationSeconds: duration, AspectRatio: "16:9"}
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), fmt.Sprintf("duration %d is not supported", duration), model)
		}
	}

	unsupported1080 := &SeedanceRequestInfo{Model: "seedance-2.0", Prompt: "duration tier", Resolution: VideoBillingResolution1080P, DurationSeconds: 15, AspectRatio: "16:9"}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(unsupported1080), "duration 15 is not supported")

	for _, duration := range []int{5, 10} {
		supported1080 := &SeedanceRequestInfo{Model: "seedance-2.0", Prompt: "duration tier", Resolution: VideoBillingResolution1080P, DurationSeconds: duration, AspectRatio: "16:9"}
		require.NoError(t, validateFFLinkVideoRequestInfo(supported1080), fmt.Sprintf("seedance-2.0/1080p/%d", duration))
	}
}

func TestPublicSeedance933ProfilesAllowFullMixedReferenceCapacity(t *testing.T) {
	for _, model := range []string{"seedance-2.0", "seedance-2.0-fast"} {
		profile, ok := ffLinkVideoModelProfileFor(model)
		require.True(t, ok, model)
		require.Equal(t, 9, profile.MaxImageReferences, model)
		require.Equal(t, 3, profile.MaxVideoReferences, model)
		require.Equal(t, 3, profile.MaxAudioReferences, model)
		require.Equal(t, 15, profile.MaxTotalMedia, model)
	}
}

func TestHuiquUpstreamModelForFixedDurationTiers(t *testing.T) {
	for _, test := range []struct {
		model    string
		duration int
		want     string
	}{
		{SeedanceMX933Model, 5, "sd2-mx933-720-5s"},
		{SeedanceMX933Model, 10, "sd2-mx933-720-10s"},
		{SeedanceMX933Model, 15, "sd2-mx933-720-15s"},
		{SeedanceMX933FastModel, 5, "sd2-mx933-720-fast-5s"},
		{SeedanceMX933FastModel, 10, "sd2-mx933-720-fast-10s"},
		{SeedanceMX933FastModel, 15, "sd2-mx933-720-fast-15s"},
		{SeedanceMX933LegacyModel, 10, "sd2-mx933-720-10s"},
	} {
		got, err := huiquUpstreamModelFor(test.model, test.duration)
		require.NoError(t, err)
		require.Equal(t, test.want, got)
	}

	_, err := huiquUpstreamModelFor(SeedanceMX933Model, 8)
	require.ErrorContains(t, err, "duration 8 is not supported")
}

func TestMX933TotalImageLimitIncludesFirstAndLastFrames(t *testing.T) {
	valid := mx933RequestInfo(SeedanceMX933Model)
	require.Len(t, valid.References, 4)
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooMany := mx933RequestInfo(SeedanceMX933Model)
	tooMany.References = make([]SeedanceReferenceImage, 8)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooMany), "at most 9 total images")

	nineReferences := mx933RequestInfo(SeedanceMX933FastModel)
	nineReferences.References = make([]SeedanceReferenceImage, 9)
	nineReferences.StartFrameURL = ""
	nineReferences.EndFrameURL = ""
	nineReferences.VideoReferences = nil
	nineReferences.AudioReferences = nil
	require.NoError(t, validateFFLinkVideoRequestInfo(nineReferences))
}

func TestMX933TotalMediaLimit(t *testing.T) {
	valid := mx933RequestInfo(SeedanceMX933Model)
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooMany := mx933RequestInfo(SeedanceMX933FastModel)
	tooMany.References = append(tooMany.References, SeedanceReferenceImage{})
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooMany), "at most 12 total reference media files")
}

func TestPublicSeedance20ModelsAllowFull933ReferenceSet(t *testing.T) {
	for _, model := range []string{"seedance-2.0", "seedance-2.0-fast"} {
		info := mx933RequestInfo(model)
		info.References = make([]SeedanceReferenceImage, 9)
		info.VideoReferences = make([]SeedanceReferenceVideo, 3)
		info.AudioReferences = make([]SeedanceReferenceAudio, 3)
		info.GenerateAudio = true
		info.AspectRatio = "16:9"
		info.StartFrameURL = ""
		info.EndFrameURL = ""
		require.NoError(t, validateFFLinkVideoRequestInfo(info), model)

		tooManyImages := *info
		tooManyImages.References = append(tooManyImages.References, SeedanceReferenceImage{})
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyImages), "at most 9 reference images")

		tooManyVideos := *info
		tooManyVideos.VideoReferences = append(tooManyVideos.VideoReferences, SeedanceReferenceVideo{})
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyVideos), "at most 3 reference videos")

		tooManyAudio := *info
		tooManyAudio.AudioReferences = append(tooManyAudio.AudioReferences, SeedanceReferenceAudio{})
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyAudio), "at most 3 reference audio files")

		tooManyTotal := *info
		tooManyTotal.References = append(tooManyTotal.References, SeedanceReferenceImage{})
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooManyTotal), "at most 9 reference images")
	}
}

func TestAudioReferenceCanStandAlone(t *testing.T) {
	mx933 := mx933RequestInfo(SeedanceMX933Model)
	mx933.References = nil
	mx933.VideoReferences = nil
	mx933.StartFrameURL = ""
	mx933.EndFrameURL = ""
	mx933.GenerateAudio = true
	mx933.AudioReferences = make([]SeedanceReferenceAudio, 1)
	require.NoError(t, validateFFLinkVideoRequestInfo(mx933))

	existing := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "audio only is allowed",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 10,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
		AudioReferences: make([]SeedanceReferenceAudio, 1),
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(existing))
}

func TestMX933PromptLimitAndDefaults(t *testing.T) {
	validPrompt := mx933RequestInfo(SeedanceMX933Model)
	validPrompt.Prompt = strings.Repeat("x", 5000)
	require.NoError(t, validateFFLinkVideoRequestInfo(validPrompt))

	tooLong := mx933RequestInfo(SeedanceMX933Model)
	tooLong.Prompt = strings.Repeat("x", 5001)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooLong), "prompt exceeds the 5000 character limit")

	defaults := &SeedanceRequestInfo{Model: SeedanceMX933FastModel, Prompt: "A city street"}
	require.NoError(t, validateFFLinkVideoRequestInfo(defaults))
	require.Equal(t, VideoBillingResolution720P, defaults.Resolution)
	require.Equal(t, 5, defaults.DurationSeconds)
	require.Equal(t, "16:9", defaults.AspectRatio)
}

func TestMX933RejectsPromptEnhance(t *testing.T) {
	for _, model := range []string{SeedanceMX933Model, SeedanceMX933FastModel} {
		_, err := normalizeFFLinkPromptEnhance(true, model)
		require.ErrorContains(t, err, "does not support prompt_enhance")
	}
}

func TestMX933TotalImageLimitDoesNotChangeExistingModels(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "Existing Seedance behavior",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 10,
		AspectRatio:     "16:9",
		References:      make([]SeedanceReferenceImage, 4),
		StartFrameURL:   "https://media.example/start.png",
		EndFrameURL:     "https://media.example/end.png",
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(request))
}

func mx933RequestInfo(model string) *SeedanceRequestInfo {
	return &SeedanceRequestInfo{
		Model:           model,
		Prompt:          "A character walks through a city",
		Resolution:      VideoBillingResolution480P,
		DurationSeconds: 5,
		AspectRatio:     "3:2",
		GenerateAudio:   true,
		References:      make([]SeedanceReferenceImage, 4),
		StartFrameURL:   "https://media.example/start.png",
		EndFrameURL:     "https://media.example/end.png",
		VideoReferences: make([]SeedanceReferenceVideo, 3),
		AudioReferences: make([]SeedanceReferenceAudio, 3),
	}
}
