package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupVideoBillingUnitMigrationPreservesExistingPerSecondPrices(t *testing.T) {
	content, err := FS.ReadFile("199_group_video_billing_unit.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS VIDEO_BILLING_UNIT VARCHAR(20) NOT NULL DEFAULT 'PER_SECOND'")
	require.Contains(t, sql, "CHECK (VIDEO_BILLING_UNIT IN ('PER_SECOND', 'PER_REQUEST'))")
	require.NotContains(t, sql, "UPDATE GROUPS")
	require.NotContains(t, sql, "VIDEO_MODEL_PRICES =")
}
