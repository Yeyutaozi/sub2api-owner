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
	require.Contains(t, models, "sd2-mx933-720-1s")
	require.Contains(t, models, "sd2-mx933-720-fast-1s")

	for _, model := range []string{"sd2-mx933-720-1s", "sd2-mx933-720-fast-1s"} {
		require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, model))
		require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformLTX, model))
	}
}

func TestMX933RequestValidation(t *testing.T) {
	for _, model := range []string{"sd2-mx933-720-1s", "sd2-mx933-720-fast-1s"} {
		t.Run(model, func(t *testing.T) {
			for duration := 1; duration <= 15; duration++ {
				valid := mx933RequestInfo(model)
				valid.Resolution = VideoBillingResolution720P
				valid.DurationSeconds = duration
				valid.AspectRatio = "2:3"
				require.NoError(t, validateFFLinkVideoRequestInfo(valid))
			}

			for _, duration := range []int{-1, 16} {
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

			audioWithoutVisualReference := mx933RequestInfo(model)
			audioWithoutVisualReference.References = nil
			audioWithoutVisualReference.StartFrameURL = ""
			audioWithoutVisualReference.EndFrameURL = ""
			audioWithoutVisualReference.VideoReferences = nil
			require.ErrorContains(t, validateFFLinkVideoRequestInfo(audioWithoutVisualReference), "reference audio requires")
		})
	}
}

func TestMX933TotalImageLimitIncludesFirstAndLastFrames(t *testing.T) {
	valid := mx933RequestInfo("sd2-mx933-720-1s")
	require.Len(t, valid.References, 4)
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooMany := mx933RequestInfo("sd2-mx933-720-1s")
	tooMany.References = make([]SeedanceReferenceImage, 8)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooMany), "at most 9 total images")

	nineReferences := mx933RequestInfo("sd2-mx933-720-fast-1s")
	nineReferences.References = make([]SeedanceReferenceImage, 9)
	nineReferences.StartFrameURL = ""
	nineReferences.EndFrameURL = ""
	nineReferences.VideoReferences = nil
	nineReferences.AudioReferences = nil
	require.NoError(t, validateFFLinkVideoRequestInfo(nineReferences))
}

func TestMX933TotalMediaLimit(t *testing.T) {
	valid := mx933RequestInfo("sd2-mx933-720-1s")
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooMany := mx933RequestInfo("sd2-mx933-720-fast-1s")
	tooMany.References = append(tooMany.References, SeedanceReferenceImage{})
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooMany), "at most 12 total reference media files")
}

func TestMX933AudioReferenceKeepsExistingSeedanceVisualRequirement(t *testing.T) {
	mx933 := mx933RequestInfo("sd2-mx933-720-1s")
	mx933.References = nil
	mx933.VideoReferences = nil
	mx933.EndFrameURL = ""
	mx933.AudioReferences = make([]SeedanceReferenceAudio, 1)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(mx933), "reference audio requires")

	existing := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "Keep the existing FFLink validation behavior",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 8,
		AspectRatio:     "16:9",
		StartFrameURL:   "https://media.example/start.png",
		AudioReferences: make([]SeedanceReferenceAudio, 1),
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(existing), "reference audio requires")
}

func TestMX933PromptLimitAndDefaults(t *testing.T) {
	validPrompt := mx933RequestInfo("sd2-mx933-720-1s")
	validPrompt.Prompt = strings.Repeat("x", 5000)
	require.NoError(t, validateFFLinkVideoRequestInfo(validPrompt))

	tooLong := mx933RequestInfo("sd2-mx933-720-1s")
	tooLong.Prompt = strings.Repeat("x", 5001)
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(tooLong), "prompt exceeds the 5000 character limit")

	defaults := &SeedanceRequestInfo{Model: "sd2-mx933-720-fast-1s", Prompt: "A city street"}
	require.NoError(t, validateFFLinkVideoRequestInfo(defaults))
	require.Equal(t, VideoBillingResolution720P, defaults.Resolution)
	require.Equal(t, 5, defaults.DurationSeconds)
	require.Equal(t, "16:9", defaults.AspectRatio)
}

func TestMX933RejectsPromptEnhance(t *testing.T) {
	for _, model := range []string{"sd2-mx933-720-1s", "sd2-mx933-720-fast-1s"} {
		_, err := normalizeFFLinkPromptEnhance(true, model)
		require.ErrorContains(t, err, "does not support prompt_enhance")
	}
}

func TestMX933TotalImageLimitDoesNotChangeExistingModels(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "Existing Seedance behavior",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 8,
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
		References:      make([]SeedanceReferenceImage, 4),
		StartFrameURL:   "https://media.example/start.png",
		EndFrameURL:     "https://media.example/end.png",
		VideoReferences: make([]SeedanceReferenceVideo, 3),
		AudioReferences: make([]SeedanceReferenceAudio, 3),
	}
}
