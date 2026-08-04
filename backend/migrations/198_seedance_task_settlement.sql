ALTER TABLE fflink_video_job_bindings
    ADD COLUMN IF NOT EXISTS task_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS next_poll_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS last_polled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS settled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refunded_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refund_status VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS refund_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS settlement_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS settlement_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS settlement_claimed_by VARCHAR(64),
    ADD COLUMN IF NOT EXISTS last_error TEXT;

ALTER TABLE fflink_video_job_bindings
    ALTER COLUMN task_status SET DEFAULT 'unknown';

UPDATE fflink_video_job_bindings
SET task_status = 'unknown',
    next_poll_at = LEAST(next_poll_at, NOW())
WHERE settled_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fflink_video_job_bindings_settlement_due
    ON fflink_video_job_bindings (next_poll_at, id)
    WHERE settled_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fflink_video_job_bindings_refund_status
    ON fflink_video_job_bindings (refund_status, next_poll_at, id)
    WHERE settled_at IS NULL AND refund_status IN ('pending', 'error');
