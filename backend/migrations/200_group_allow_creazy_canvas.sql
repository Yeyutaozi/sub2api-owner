-- Creazy 画布分组准入：默认开放，管理员可关闭。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_creazy_canvas BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN groups.allow_creazy_canvas IS
    'Whether keys in this group can be used in Creazy Canvas web UI. Default true; does not affect /v1 API access.';
