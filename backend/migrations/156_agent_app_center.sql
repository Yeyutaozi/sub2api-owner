-- Agent app center core tables.
-- P0-P2 focuses on Worker Host management and app version binding.

CREATE TABLE IF NOT EXISTS agent_worker_hosts (
    id                      BIGSERIAL PRIMARY KEY,
    name                    VARCHAR(100) NOT NULL,
    base_url                TEXT NOT NULL,
    protocol                VARCHAR(50) NOT NULL DEFAULT 'sub2api-worker-v1',
    auth_type               VARCHAR(32) NOT NULL DEFAULT 'hmac_run_token',
    secret_ref              VARCHAR(255),
    health_path             VARCHAR(255) NOT NULL DEFAULT '/health',
    run_path                VARCHAR(255) NOT NULL DEFAULT '/runs',
    cancel_path             VARCHAR(255),
    max_concurrency         INT NOT NULL DEFAULT 1,
    timeout_seconds         INT NOT NULL DEFAULT 600,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active',
    last_health_status      VARCHAR(20) NOT NULL DEFAULT 'unknown',
    last_health_message     TEXT,
    last_health_latency_ms  INT,
    last_checked_at         TIMESTAMPTZ,
    metadata                JSONB NOT NULL DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ,
    CONSTRAINT agent_worker_hosts_auth_type_check CHECK (auth_type IN ('none', 'hmac_run_token', 'bearer')),
    CONSTRAINT agent_worker_hosts_status_check CHECK (status IN ('active', 'disabled', 'unhealthy')),
    CONSTRAINT agent_worker_hosts_health_status_check CHECK (last_health_status IN ('healthy', 'unhealthy', 'unknown')),
    CONSTRAINT agent_worker_hosts_max_concurrency_check CHECK (max_concurrency > 0),
    CONSTRAINT agent_worker_hosts_timeout_seconds_check CHECK (timeout_seconds > 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_worker_hosts_status
    ON agent_worker_hosts(status)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_worker_hosts_name_active
    ON agent_worker_hosts(name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_worker_hosts_deleted_at
    ON agent_worker_hosts(deleted_at);

CREATE TABLE IF NOT EXISTS agent_apps (
    id                    BIGSERIAL PRIMARY KEY,
    name                  VARCHAR(120) NOT NULL,
    slug                  VARCHAR(140) NOT NULL,
    description           TEXT,
    icon_url              TEXT,
    category              VARCHAR(80),
    app_type              VARCHAR(32) NOT NULL DEFAULT 'external',
    visibility            VARCHAR(20) NOT NULL DEFAULT 'private',
    status                VARCHAR(20) NOT NULL DEFAULT 'draft',
    published_version_id  BIGINT,
    created_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ,
    CONSTRAINT agent_apps_app_type_check CHECK (app_type IN ('prompt', 'workflow', 'agent', 'external')),
    CONSTRAINT agent_apps_visibility_check CHECK (visibility IN ('public', 'private')),
    CONSTRAINT agent_apps_status_check CHECK (status IN ('draft', 'published', 'disabled', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_agent_apps_status
    ON agent_apps(status)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_apps_slug_active
    ON agent_apps(slug)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_apps_type
    ON agent_apps(app_type)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_apps_deleted_at
    ON agent_apps(deleted_at);

CREATE TABLE IF NOT EXISTS agent_app_versions (
    id                         BIGSERIAL PRIMARY KEY,
    app_id                     BIGINT NOT NULL REFERENCES agent_apps(id) ON DELETE CASCADE,
    version                    VARCHAR(64) NOT NULL,
    status                     VARCHAR(20) NOT NULL DEFAULT 'draft',
    runtime_type               VARCHAR(32) NOT NULL DEFAULT 'worker',
    worker_host_id             BIGINT REFERENCES agent_worker_hosts(id) ON DELETE RESTRICT,
    worker_route               VARCHAR(255),
    worker_health_route        VARCHAR(255),
    image_ref                  TEXT,
    source_ref                 TEXT,
    input_schema_json          JSONB NOT NULL DEFAULT '{}',
    output_schema_json         JSONB NOT NULL DEFAULT '{}',
    capabilities_json          JSONB NOT NULL DEFAULT '{}',
    default_model_config_json  JSONB NOT NULL DEFAULT '{}',
    node_model_policy_json     JSONB NOT NULL DEFAULT '{}',
    artifact_policy_json       JSONB NOT NULL DEFAULT '{}',
    changelog                  TEXT,
    created_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    published_at               TIMESTAMPTZ,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                 TIMESTAMPTZ,
    CONSTRAINT agent_app_versions_status_check CHECK (status IN ('draft', 'published', 'disabled', 'archived')),
    CONSTRAINT agent_app_versions_runtime_type_check CHECK (runtime_type IN ('worker', 'prompt', 'internal')),
    CONSTRAINT agent_app_versions_worker_binding_check CHECK (
        runtime_type <> 'worker'
        OR (worker_host_id IS NOT NULL AND worker_route IS NOT NULL AND worker_route <> '')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_app_versions_app_version
    ON agent_app_versions(app_id, version)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_app_versions_app_id
    ON agent_app_versions(app_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_app_versions_worker_host_id
    ON agent_app_versions(worker_host_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_app_versions_deleted_at
    ON agent_app_versions(deleted_at);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'agent_apps_published_version_id_fkey'
    ) THEN
        ALTER TABLE agent_apps
            ADD CONSTRAINT agent_apps_published_version_id_fkey
            FOREIGN KEY (published_version_id) REFERENCES agent_app_versions(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS agent_runs (
    id                   BIGSERIAL PRIMARY KEY,
    app_id               BIGINT NOT NULL REFERENCES agent_apps(id) ON DELETE RESTRICT,
    app_version_id       BIGINT NOT NULL REFERENCES agent_app_versions(id) ON DELETE RESTRICT,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id           BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    worker_host_id       BIGINT REFERENCES agent_worker_hosts(id) ON DELETE SET NULL,
    run_token_hash       VARCHAR(128) NOT NULL,
    status               VARCHAR(20) NOT NULL DEFAULT 'queued',
    input_ref_url        TEXT,
    input_summary_json   JSONB NOT NULL DEFAULT '{}',
    output_ref_url       TEXT,
    output_summary_json  JSONB NOT NULL DEFAULT '{}',
    error_code           VARCHAR(80),
    error_message        TEXT,
    usage_json           JSONB NOT NULL DEFAULT '{}',
    started_at           TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    expires_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_runs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'timeout'))
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_user_created
    ON agent_runs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_runs_app_created
    ON agent_runs(app_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_runs_status
    ON agent_runs(status);

CREATE INDEX IF NOT EXISTS idx_agent_runs_expires_at
    ON agent_runs(expires_at);

CREATE TABLE IF NOT EXISTS agent_artifacts (
    id                BIGSERIAL PRIMARY KEY,
    run_id            BIGINT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    artifact_type     VARCHAR(32) NOT NULL,
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
    CONSTRAINT agent_artifacts_type_check CHECK (artifact_type IN ('input', 'output', 'log', 'preview')),
    CONSTRAINT agent_artifacts_size_check CHECK (size_bytes >= 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_artifacts_run_id
    ON agent_artifacts(run_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_artifacts_user_created
    ON agent_artifacts(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_artifacts_expires_at
    ON agent_artifacts(expires_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_artifacts_deleted_at
    ON agent_artifacts(deleted_at);
