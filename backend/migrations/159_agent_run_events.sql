-- App Center run event timeline.
-- Events are lightweight operational records; large outputs stay in object storage.

CREATE TABLE IF NOT EXISTS agent_run_events (
    id             BIGSERIAL PRIMARY KEY,
    run_id         BIGINT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type     VARCHAR(40) NOT NULL,
    status         VARCHAR(32),
    node_id        VARCHAR(120),
    node_role      VARCHAR(120),
    message        TEXT,
    progress       DOUBLE PRECISION,
    metadata_json  JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_run_events_progress_check CHECK (progress IS NULL OR (progress >= 0 AND progress <= 1))
);

CREATE INDEX IF NOT EXISTS idx_agent_run_events_run_created
    ON agent_run_events(run_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_agent_run_events_user_created
    ON agent_run_events(user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_agent_run_events_type_created
    ON agent_run_events(event_type, created_at DESC);
