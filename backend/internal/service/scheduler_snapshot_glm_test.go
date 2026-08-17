package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotPlatformsIncludesAllCanonicalPlatformsExactlyOnce(t *testing.T) {
	platforms := schedulerSnapshotPlatforms()
	require.Len(t, platforms, 11)
	require.ElementsMatch(t, []string{
		PlatformAnthropic,
		PlatformGemini,
		PlatformOpenAI,
		PlatformAntigravity,
		PlatformGrok,
		PlatformGLM,
		PlatformSeedance,
		PlatformLTX,
		PlatformHappyHorse,
		PlatformMiniMax,
		PlatformGrokImagine,
	}, platforms[:])
}
