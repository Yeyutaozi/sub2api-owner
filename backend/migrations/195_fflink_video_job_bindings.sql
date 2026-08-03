CREATE TABLE IF NOT EXISTS fflink_video_job_bindings (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    job_id VARCHAR(256) NOT NULL,
    model VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fflink_video_job_bindings_owner_job_unique
        UNIQUE (user_id, api_key_id, group_id, job_id)
);

CREATE INDEX IF NOT EXISTS idx_fflink_video_job_bindings_owner_created
    ON fflink_video_job_bindings (user_id, api_key_id, group_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_fflink_video_job_bindings_account
    ON fflink_video_job_bindings (account_id);
