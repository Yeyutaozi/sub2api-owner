package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseXimeiRequestRejectsMediaContractErrorsBeforeDispatch(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{
			name: "video duration is required",
			body: `{
				"model":"sd-2.0-mx933",
				"prompt":"人物展示产品",
				"resolution":"480p",
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}],
					"video_reference_base":[{"video":{"url":"https://media.example/motion.mp4"}}]
				}
			}`,
			message: "video.duration_seconds is required",
		},
		{
			name: "audio duration is required",
			body: `{
				"model":"sd-2.0-mx933",
				"prompt":"人物展示产品并保留参考音色",
				"resolution":"480p",
				"audio":true,
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}],
					"audio_reference":[{"audio":{"url":"https://media.example/voice.wav"}}]
				}
			}`,
			message: "audio.duration_seconds is required",
		},
		{
			name: "cumulative video duration is bounded",
			body: `{
				"model":"sd-2.0-mx933",
				"prompt":"人物展示产品",
				"resolution":"720p",
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}],
					"video_reference_base":[
						{"video":{"url":"https://media.example/a.mp4","duration_seconds":8}},
						{"video":{"url":"https://media.example/b.mp4","duration_seconds":8}}
					]
				}
			}`,
			message: "reference video duration must not exceed 15 seconds",
		},
		{
			name: "cumulative audio duration is bounded",
			body: `{
				"model":"sd-2.0-mx933",
				"prompt":"人物展示产品并保留参考音色",
				"resolution":"480p",
				"audio":true,
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}],
					"audio_reference":[
						{"audio":{"url":"https://media.example/a.wav","duration_seconds":8}},
						{"audio":{"url":"https://media.example/b.wav","duration_seconds":8}}
					]
				}
			}`,
			message: "reference audio duration must not exceed 15 seconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseSeedanceVideoGenerationRequest([]byte(test.body))
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestParseSeedanceRequestRequiresAudioFlagForAudioReference(t *testing.T) {
	for _, model := range []string{"seedance-2.0", SeedanceMX933Model, SeedanceXimeiSD20Model} {
		t.Run(model, func(t *testing.T) {
			body := strings.NewReplacer("MODEL", model).Replace(`{
				"model":"MODEL",
				"prompt":"人物展示产品并保留参考音色",
				"resolution":"720p",
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}],
					"audio_reference":[{"audio":{"url":"https://media.example/voice.wav","duration_seconds":5}}]
				}
			}`)
			_, err := ParseSeedanceVideoGenerationRequest([]byte(body))
			require.EqualError(t, err, "audio=true is required when guidances.audio_reference is provided")
		})
	}
}

func TestParseXimeiRequestRejectsReservedPromptMediaReferences(t *testing.T) {
	for _, token := range []string{"@Image1", "@audio2", "@VIDEO3"} {
		t.Run(token, func(t *testing.T) {
			body := strings.NewReplacer("TOKEN", token).Replace(`{
				"model":"sd-2.0-mx933",
				"prompt":"让 TOKEN 中的主体自然转身",
				"resolution":"480p",
				"guidances":{
					"image_reference":[{"image":{"url":"https://media.example/product.png"}}]
				}
			}`)
			_, err := ParseSeedanceVideoGenerationRequest([]byte(body))
			require.ErrorContains(t, err, "platform-reserved media references")
		})
	}
}

func TestParseXimeiRequestRejectsPromptThatOnlyExceedsLimitAfterCompilation(t *testing.T) {
	prompt := strings.Repeat("x", 4900)
	body := strings.NewReplacer("PROMPT", prompt).Replace(`{
		"model":"sd-2.0-mx933",
		"prompt":"PROMPT",
		"resolution":"480p",
		"guidances":{
			"image_reference":[
				{"image":{"url":"https://media.example/a.png"}},
				{"image":{"url":"https://media.example/b.png"}},
				{"image":{"url":"https://media.example/c.png"}}
			]
		}
	}`)
	_, err := ParseSeedanceVideoGenerationRequest([]byte(body))
	require.ErrorContains(t, err, "compiled prompt exceeds the 5000 character limit")
}
