package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClearNonGrokVideoGenerationConfigMigrationPreservesOwnerPlatforms(t *testing.T) {
	content, err := FS.ReadFile("220_clear_non_grok_video_generation_config.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "SELECT 1")
	require.NotContains(t, sql, "UPDATE GROUPS")
	require.NotContains(t, sql, "VIDEO_MODEL_PRICES = NULL")
	require.NotContains(t, sql, "ALTER TABLE GROUPS")
}
