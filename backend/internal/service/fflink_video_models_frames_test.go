//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoEndFrameRequiresStartFrameForAllModels(t *testing.T) {
	endOnly := &SeedanceRequestInfo{
		Model:         "seedance-2.0",
		Prompt:        "scene",
		DurationSeconds: 5,
		Resolution:    VideoBillingResolution720P,
		AspectRatio:   "16:9",
		EndFrameURL:   "https://media.example/end.png",
	}
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(endOnly), "first frame is required when a last frame is provided")

	paired := &SeedanceRequestInfo{
		Model:           "seedance-2.0",
		Prompt:          "scene",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		StartFrameURL:   "https://media.example/start.png",
		EndFrameURL:     "https://media.example/end.png",
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(paired))
}
