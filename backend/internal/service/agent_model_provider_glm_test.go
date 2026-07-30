package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentModelProviderSupportsGLM(t *testing.T) {
	require.Equal(t, PlatformGLM, normalizeAgentModelProvider("glm"))
	require.Equal(t, PlatformGLM, normalizeAgentModelProvider("zhipuai"))
	require.Equal(t, PlatformGLM, inferAgentModelProvider("glm-5"))
	require.Equal(t, PlatformGLM, modelPolicyProvider(ModelPolicy{Model: "glm-4.5"}))
}
