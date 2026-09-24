package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelsListConfigCompatibilityMigration(t *testing.T) {
	content, err := FS.ReadFile("239_restore_models_list_config_compat.sql")
	require.NoError(t, err)

	sql := strings.ToLower(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "alter table groups")
	require.Contains(t, sql, "add column if not exists models_list_config jsonb")
	require.Contains(t, sql, "model_allowlist")
	require.Contains(t, sql, "never overwrite")
}
