package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultGLMModelIDsContainOnlySupportedTextCapabilities(t *testing.T) {
	models := DefaultGLMModelIDs()

	require.Contains(t, models, "glm-5.2")
	require.Contains(t, models, "embedding-3")
	require.NotContains(t, models, "cogview-4")
	require.NotContains(t, models, "glm-4v")
	require.NotContains(t, models, "cogvideo")
}
