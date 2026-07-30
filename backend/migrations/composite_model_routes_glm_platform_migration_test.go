package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeModelRoutesGLMPlatformMigration(t *testing.T) {
	content, err := FS.ReadFile("193_composite_model_routes_glm_platform.sql")
	require.NoError(t, err)

	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "composite_model_routes_target_platform_check")
	require.Contains(t, sql, "'glm'")
}
