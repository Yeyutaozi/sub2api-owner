-- Creazy Canvas works metadata.
-- V1 stores job metadata + optional COS object_key; gateway content transfer is optional.

CREATE TABLE IF NOT EXISTS creazy_canvas_works (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id          BIGINT NOT NULL,
    group_id            BIGINT,
    kind                VARCHAR(16) NOT NULL,
    public_model        VARCHAR(128) NOT NULL DEFAULT '',
    status              VARCHAR(32) NOT NULL DEFAULT 'created',
    prompt              TEXT NOT NULL DEFAULT '',
    params_json         JSONB NOT NULL DEFAULT '{}',
    gateway_type        VARCHAR(64) NOT NULL DEFAULT '',
    gateway_remote_id   VARCHAR(255) NOT NULL DEFAULT '',
    object_key          TEXT NOT NULL DEFAULT '',
    storage_provider    VARCHAR(64) NOT NULL DEFAULT '',
    bucket              VARCHAR(255) NOT NULL DEFAULT '',
    object_url          TEXT NOT NULL DEFAULT '',
    preview_url         TEXT NOT NULL DEFAULT '',
    mime_type           VARCHAR(128) NOT NULL DEFAULT '',
    size_bytes          BIGINT NOT NULL DEFAULT 0,
    error_message       TEXT NOT NULL DEFAULT '',
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT creazy_canvas_works_kind_check CHECK (kind IN ('image', 'video')),
    CONSTRAINT creazy_canvas_works_status_check CHECK (
        status IN ('created', 'queued', 'running', 'succeeded', 'failed', 'canceled', 'expired')
    )
);

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_user_created
    ON creazy_canvas_works(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_user_kind
    ON creazy_canvas_works(user_id, kind)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_user_status
    ON creazy_canvas_works(user_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_api_key
    ON creazy_canvas_works(api_key_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_gateway_remote
    ON creazy_canvas_works(gateway_type, gateway_remote_id)
    WHERE deleted_at IS NULL AND gateway_remote_id <> '';

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_expires_at
    ON creazy_canvas_works(expires_at)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE creazy_canvas_works IS
    'Creazy Canvas user works metadata (image/video). Content may live in gateway or COS under creazy-canvas/ prefix.';
