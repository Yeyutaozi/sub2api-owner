package service

import "testing"

func TestBeebleSD20720ModelProfile(t *testing.T) {
	if err := ValidateFFLinkVideoModelPlatform(PlatformSeedance, SeedanceBeebleSD20720Model); err != nil {
		t.Fatalf("expected beeble alias to be a Seedance model: %v", err)
	}
	profile, ok := ffLinkVideoModelProfileFor(SeedanceBeebleSD20720Model)
	if !ok {
		t.Fatal("expected beeble alias profile")
	}
	if profile.DefaultResolution != VideoBillingResolution720P {
		t.Fatalf("default resolution = %q, want 720p", profile.DefaultResolution)
	}
	if _, ok := profile.AllowedResolutions[VideoBillingResolution720P]; !ok {
		t.Fatal("expected 720p to be supported")
	}
	if len(profile.AllowedResolutions) != 1 {
		t.Fatalf("allowed resolutions = %v, want only 720p", profile.AllowedResolutions)
	}
	for _, duration := range []int{4, 5, 10, 15} {
		if !profile.ValidateDuration(duration, VideoBillingResolution720P) {
			t.Fatalf("duration %d should be supported", duration)
		}
	}
	if profile.ValidateDuration(3, VideoBillingResolution720P) || profile.ValidateDuration(16, VideoBillingResolution720P) {
		t.Fatal("durations outside 4-15 seconds should be rejected")
	}
}
