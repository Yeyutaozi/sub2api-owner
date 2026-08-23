package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotPlatformsIncludesAllCanonicalPlatformsExactlyOnce(t *testing.T) {
	platforms := schedulerSnapshotPlatforms()
	require.Len(t, platforms, 14)
	require.ElementsMatch(t, []string{
		PlatformAnthropic,
		PlatformGemini,
		PlatformOpenAI,
		PlatformAntigravity,
		PlatformGrok,
		PlatformKimi,
		PlatformZhipu,
		PlatformDeepseek,
		PlatformGLM,
		PlatformSeedance,
		PlatformLTX,
		PlatformHappyHorse,
		PlatformMiniMax,
		PlatformGrokImagine,
	}, platforms[:])
}
