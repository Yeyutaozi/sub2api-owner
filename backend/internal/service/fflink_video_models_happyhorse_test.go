//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHappyHorseModelProfile(t *testing.T) {
	require.Equal(t, []string{"happy-horse-1.1"}, FFLinkVideoModelIDsForPlatform(PlatformHappyHorse))
	require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformHappyHorse, "happy-horse-1.1"))
	require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformLTX, "happy-horse-1.1"))
}

func TestHappyHorseRequestValidation(t *testing.T) {
	references := make([]SeedanceReferenceImage, 9)
	for index := range references {
		references[index] = SeedanceReferenceImage{
			URL:      fmt.Sprintf("https://media.example/reference-%d.png", index+1),
			Strength: "HIGH",
		}
	}

	valid := &SeedanceRequestInfo{
		Model:           "happy-horse-1.1",
		Prompt:          "A product rotates on a clean studio table",
		Resolution:      "720p",
		DurationSeconds: 15,
		AspectRatio:     "9:16",
		References:      references,
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(valid))

	tooMany := *valid
	tooMany.References = append(append([]SeedanceReferenceImage{}, references...), SeedanceReferenceImage{URL: "https://media.example/reference-10.png"})
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&tooMany), "at most 9 reference images")

	withEndFrame := *valid
	withEndFrame.References = nil
	withEndFrame.StartFrameURL = "https://media.example/start.png"
	withEndFrame.EndFrameURL = "https://media.example/end.png"
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&withEndFrame), "does not support a last frame")

	withAudio := *valid
	withAudio.GenerateAudio = true
	require.NoError(t, validateFFLinkVideoRequestInfo(&withAudio))
}

func TestHappyHorseFirstFrameUsesStartFrameURL(t *testing.T) {
	request := &SeedanceRequestInfo{
		Model:           "happy-horse-1.1",
		Prompt:          "A horse runs through a sunlit meadow",
		Resolution:      "1080p",
		DurationSeconds: 5,
		AspectRatio:     "16:9",
		StartFrameURL:   "https://media.example/start.png",
		PromptEnhance:   "OFF",
		GenerateAudio:   true,
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(request))

	body, err := request.UpstreamBody("happy-horse-1.1")
	require.NoError(t, err)
	var upstream map[string]any
	require.NoError(t, json.Unmarshal(body, &upstream))
	require.Equal(t, "https://media.example/start.png", upstream["start_frame_url"])
	require.NotContains(t, upstream, "image_url")
	require.Equal(t, true, upstream["audio"])
	require.Equal(t, "OFF", upstream["prompt_enhance"])
}
