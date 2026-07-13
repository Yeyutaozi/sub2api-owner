-- User-owned input assets for App Center runs.
-- Binary content lives in object storage; the database stores references only.

CREATE TABLE IF NOT EXISTS agent_input_assets (
    id                BIGSERIAL PRIMARY KEY,
    run_id            BIGINT REFERENCES agent_runs(id) ON DELETE SET NULL,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id            BIGINT REFERENCES agent_apps(id) ON DELETE SET NULL,
    field_name        VARCHAR(120),
    asset_type        VARCHAR(32) NOT NULL DEFAULT 'file',
    asset_role        VARCHAR(120),
    name              VARCHAR(255) NOT NULL,
    mime_type         VARCHAR(120),
    storage_provider  VARCHAR(40) NOT NULL DEFAULT 's3',
    bucket            VARCHAR(255),
    object_key        TEXT NOT NULL,
    object_url        TEXT NOT NULL,
    size_bytes        BIGINT NOT NULL DEFAULT 0,
    sha256            VARCHAR(64),
    metadata_json     JSONB NOT NULL DEFAULT '{}',
    expires_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    CONSTRAINT agent_input_assets_type_check CHECK (asset_type IN ('image', 'file', 'audio', 'video')),
    CONSTRAINT agent_input_assets_size_check CHECK (size_bytes >= 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_input_assets_user_created
    ON agent_input_assets(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_input_assets_user_app_created
    ON agent_input_assets(user_id, app_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_input_assets_run_id
    ON agent_input_assets(run_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_input_assets_expires_at
    ON agent_input_assets(expires_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_input_assets_deleted_at
    ON agent_input_assets(deleted_at);
