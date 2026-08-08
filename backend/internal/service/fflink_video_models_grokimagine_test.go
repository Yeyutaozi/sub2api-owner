//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGrokImagine15ProfileMatchesFFLinkUpstream documents the constraints observed
// against api.fflink.top for model grok-imagine-1.5 (2026-08-08 probe):
//   - prompt required
//   - start frame required ("start frame is required by grok-imagine-1.5")
//   - duration whole number 3..15
//   - resolution only 720p (480p/1080p rejected)
//   - aspect_ratio supports 16:9 and 9:16 (1:1/4:3 rejected)
//   - reference image/video/audio not offered in local profile
func TestGrokImagine15ProfileMatchesFFLinkUpstream(t *testing.T) {
	models := FFLinkVideoModelIDsForPlatform(PlatformGrokImagine)
	require.Equal(t, []string{"grok-imagine-1.5"}, models)
	require.NoError(t, ValidateFFLinkVideoModelPlatform(PlatformGrokImagine, "grok-imagine-1.5"))
	require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformGrok, "grok-imagine-1.5"))
	require.Error(t, ValidateFFLinkVideoModelPlatform(PlatformSeedance, "grok-imagine-1.5"))

	profile, ok := ffLinkVideoModelProfileFor("grok-imagine-1.5")
	require.True(t, ok)
	require.Equal(t, PlatformGrokImagine, profile.Platform)
	require.True(t, profile.AllowStartFrame)
	require.True(t, profile.RequireStartFrame)
	require.False(t, profile.AllowEndFrame)
	require.True(t, profile.AllowGeneratedAudio)
	require.Equal(t, 0, profile.MaxImageReferences)
	require.Equal(t, 0, profile.MaxVideoReferences)
	require.Equal(t, 0, profile.MaxAudioReferences)
	require.Equal(t, VideoBillingResolution720P, profile.DefaultResolution)
	_, ok720 := profile.AllowedResolutions[VideoBillingResolution720P]
	require.True(t, ok720)
	_, ok480 := profile.AllowedResolutions[VideoBillingResolution480P]
	require.False(t, ok480)
	_, ok1080 := profile.AllowedResolutions[VideoBillingResolution1080P]
	require.False(t, ok1080)
	_, ok169 := profile.AllowedAspectRatios["16:9"]
	require.True(t, ok169)
	_, ok916 := profile.AllowedAspectRatios["9:16"]
	require.True(t, ok916)
	_, ok11 := profile.AllowedAspectRatios["1:1"]
	require.False(t, ok11)
	require.True(t, profile.ValidateDuration(3, VideoBillingResolution720P))
	require.True(t, profile.ValidateDuration(15, VideoBillingResolution720P))
	require.False(t, profile.ValidateDuration(1, VideoBillingResolution720P))
	require.False(t, profile.ValidateDuration(2, VideoBillingResolution720P))
	require.False(t, profile.ValidateDuration(16, VideoBillingResolution720P))
}

func TestGrokImagine15ValidateRejectsPromptOnly(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           "grok-imagine-1.5",
		Prompt:          "A calm ocean wave at sunset",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 6,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
	}
	err := validateFFLinkVideoRequestInfo(info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start frame is required by grok-imagine-1.5")
}

func TestGrokImagine15ValidateAcceptsStartFrame(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           "grok-imagine-1.5",
		Prompt:          "Animate this image with gentle motion",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 6,
		AspectRatio:     "16:9",
		GenerateAudio:   true,
		StartFrameURL:   "https://cdn.example.com/frame.jpg",
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(info))
}

func TestGrokImagine15ValidateRejectsUnsupportedResolutionAndAspect(t *testing.T) {
	base := SeedanceRequestInfo{
		Model:           "grok-imagine-1.5",
		Prompt:          "Animate this image",
		DurationSeconds: 6,
		GenerateAudio:   true,
		StartFrameURL:   "https://cdn.example.com/frame.jpg",
	}

	info480 := base
	info480.Resolution = VideoBillingResolution480P
	info480.AspectRatio = "16:9"
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&info480), "resolution")

	info1080 := base
	info1080.Resolution = VideoBillingResolution1080P
	info1080.AspectRatio = "16:9"
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&info1080), "resolution")

	info11 := base
	info11.Resolution = VideoBillingResolution720P
	info11.AspectRatio = "1:1"
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&info11), "aspect_ratio")

	infoDur1 := base
	infoDur1.Resolution = VideoBillingResolution720P
	infoDur1.AspectRatio = "16:9"
	infoDur1.DurationSeconds = 1
	require.ErrorContains(t, validateFFLinkVideoRequestInfo(&infoDur1), "duration")
}

func TestCreazyCanvasCatalogExposesGrokImagineRequireStartFrame(t *testing.T) {
	groupID := int64(23)
	price720 := 0.14
	group := &Group{
		ID:                groupID,
		Name:              "GrokImagine FF",
		Platform:          PlatformGrokImagine,
		AllowCreazyCanvas: true,
		VideoPrice720P:    &price720,
		VideoBillingUnit:  VideoBillingUnitPerSecond,
	}
	keys := map[int64]*APIKey{
		25: {ID: 25, UserID: 1, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)
	catalog, err := svc.Catalog(context.Background(), 1, 25)
	require.NoError(t, err)
	require.Equal(t, PlatformGrokImagine, catalog.Platform)
	require.Len(t, catalog.VideoModels, 1)
	model := catalog.VideoModels[0]
	require.Equal(t, "grok-imagine-1.5", model.ID)
	require.True(t, model.AllowStartFrame)
	require.True(t, model.RequireStartFrame)
	require.False(t, model.AllowEndFrame)
	require.Equal(t, []string{VideoBillingResolution720P}, model.AllowedResolutions)
	require.Contains(t, model.AllowedAspectRatios, "16:9")
	require.Contains(t, model.AllowedAspectRatios, "9:16")
	require.Equal(t, 0, model.MaxImageReferences)
	require.Equal(t, 0, model.MaxVideoReferences)
	require.Equal(t, 0, model.MaxAudioReferences)
}
