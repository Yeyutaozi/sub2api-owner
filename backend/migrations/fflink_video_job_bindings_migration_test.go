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
