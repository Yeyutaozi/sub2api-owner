package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFFLinkVideoJobBindingsMigrationDefinesOwnerScopedIndex(t *testing.T) {
	content, err := FS.ReadFile("195_fflink_video_job_bindings.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS FFLINK_VIDEO_JOB_BINDINGS")
	require.Contains(t, sql, "UNIQUE (USER_ID, API_KEY_ID, GROUP_ID, JOB_ID)")
	require.Contains(t, sql, "ACCOUNT_ID BIGINT NOT NULL")
	require.Contains(t, sql, "MODEL VARCHAR(100) NOT NULL")
	require.Contains(t, sql, "ON FFLINK_VIDEO_JOB_BINDINGS (USER_ID, API_KEY_ID, GROUP_ID, CREATED_AT DESC, ID DESC)")
}

func TestSeedanceProviderFallbackMigrationDefinesLeaseColumns(t *testing.T) {
	content, err := FS.ReadFile("196_seedance_provider_fallback.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS FALLBACK_CLAIM_TOKEN VARCHAR(64)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS FALLBACK_LEASE_UNTIL TIMESTAMPTZ")
	require.Contains(t, sql, "FALLBACK_STATUS IN ('READY', 'STARTING')")
}

func TestSeedanceTaskSettlementMigrationDefinesDurableReconciliationState(t *testing.T) {
	content, err := FS.ReadFile("198_seedance_task_settlement.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS TASK_STATUS VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS NEXT_POLL_AT TIMESTAMPTZ NOT NULL DEFAULT NOW()")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS SETTLED_AT TIMESTAMPTZ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS REFUNDED_AT TIMESTAMPTZ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS REFUND_STATUS VARCHAR(20) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS REFUND_ATTEMPTS INTEGER NOT NULL DEFAULT 0")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS SETTLEMENT_ATTEMPTS INTEGER NOT NULL DEFAULT 0")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS SETTLEMENT_CLAIMED_AT TIMESTAMPTZ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS SETTLEMENT_CLAIMED_BY VARCHAR(64)")
	require.Contains(t, sql, "WHERE SETTLED_AT IS NULL")
}
