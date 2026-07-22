ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_model_prices JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.video_model_prices IS
    'Requested video model -> 480p/720p/1080p per-second prices (USD/s)';
