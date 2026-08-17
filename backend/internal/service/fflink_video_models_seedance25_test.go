//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedance25UsesResolutionDurationCaps(t *testing.T) {
	for _, duration := range []int{4, 5, 6, 8, 10, 12, 15} {
		info := &SeedanceRequestInfo{
			Model: "seedance-2.5", Prompt: "duration tier", Resolution: VideoBillingResolution720P,
			DurationSeconds: duration, AspectRatio: "16:9",
		}
		require.NoError(t, validateFFLinkVideoRequestInfo(info), fmt.Sprint(duration))
	}

	for _, duration := range []int{16, 20, 25, 30} {
		info := &SeedanceRequestInfo{
			Model: "seedance-2.5", Prompt: "duration tier", Resolution: VideoBillingResolution720P,
			DurationSeconds: duration, AspectRatio: "16:9",
		}
		require.ErrorContains(t, validateFFLinkVideoRequestInfo(info), fmt.Sprintf("duration %d is not supported", duration))
	}

	for _, duration := range []int{20, 25, 30} {
		info := &SeedanceRequestInfo{
			Model: "seedance-2.5", Prompt: "long 480p tier", Resolution: VideoBillingResolution480P,
			DurationSeconds: duration, AspectRatio: "16:9",
		}
		require.NoError(t, validateFFLinkVideoRequestInfo(info), fmt.Sprint(duration))
	}
}
