ALTER TABLE fflink_video_job_bindings
    ADD COLUMN IF NOT EXISTS upstream_job_id VARCHAR(256),
    ADD COLUMN IF NOT EXISTS fallback_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS fallback_status VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fallback_claim_token VARCHAR(64),
    ADD COLUMN IF NOT EXISTS fallback_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS request_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE fflink_video_job_bindings
SET upstream_job_id = job_id
WHERE upstream_job_id IS NULL OR upstream_job_id = '';

ALTER TABLE fflink_video_job_bindings
    ALTER COLUMN upstream_job_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fflink_video_job_bindings_fallback_status
    ON fflink_video_job_bindings (fallback_status)
    WHERE fallback_status IN ('ready', 'starting');
