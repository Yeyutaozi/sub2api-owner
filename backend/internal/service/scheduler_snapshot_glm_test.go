package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotPlatformsIncludesGLMExactlyOnce(t *testing.T) {
	platforms := schedulerSnapshotPlatforms()
	require.Len(t, platforms, 7)
	require.ElementsMatch(t, []string{
		PlatformAnthropic,
		PlatformGemini,
		PlatformOpenAI,
		PlatformAntigravity,
		PlatformGrok,
		PlatformGLM,
		PlatformSeedance,
	}, platforms[:])
}
