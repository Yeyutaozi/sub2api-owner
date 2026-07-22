package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupVideoModelPricesMigrationIsIdempotentAndDataPreserving(t *testing.T) {
	content, err := FS.ReadFile("185_group_video_model_prices.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "ALTER TABLE GROUPS ADD COLUMN IF NOT EXISTS VIDEO_MODEL_PRICES JSONB NOT NULL DEFAULT '{}'::JSONB")
	require.Contains(t, sql, "COMMENT ON COLUMN GROUPS.VIDEO_MODEL_PRICES IS")
	require.Equal(t, 1, strings.Count(sql, "ALTER TABLE GROUPS"))
	require.NotContains(t, sql, "UPDATE GROUPS")
	require.NotContains(t, sql, "DROP COLUMN")
}
