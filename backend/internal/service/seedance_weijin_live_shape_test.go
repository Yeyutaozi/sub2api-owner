package service

import "testing"

func TestWeijinAccountModelSupportLiveShape(t *testing.T) {
	a := &Account{
		ID:       70,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"base_url":       "https://www.weijinapi.top",
			"video_provider": "weijin",
			"api_key":        "sk-test",
			"model_mapping": map[string]any{
				"seedance2.0-one-face-reference-480p": "seedance2.0-one-face-reference-480p",
				"seedance2.0-one-face-reference-720p": "seedance2.0-one-face-reference-720p",
			},
		},
	}
	if !a.IsWeijinVideo() {
		t.Fatalf("expected weijin video account, provider=%q", a.GetVideoProvider())
	}
	for _, model := range []string{
		"seedance2.0-one-face-reference-720p",
		"seedance2.0-one-face-reference-480p",
	} {
		if !a.IsModelSupported(model) {
			t.Fatalf("expected model supported: %s provider=%s", model, a.GetVideoProvider())
		}
		if !videoProviderSupportsModelForPlatform(PlatformSeedance, a.GetVideoProvider(), model) {
			t.Fatalf("provider support false: %s", model)
		}
	}
	if a.IsModelSupported("seedance-2.0") {
		t.Fatalf("fflink model should not be supported by weijin account")
	}
}
