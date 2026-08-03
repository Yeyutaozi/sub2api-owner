ALTER TABLE user_group_rate_multipliers
    ADD COLUMN IF NOT EXISTS video_model_prices JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN user_group_rate_multipliers.video_model_prices IS
    'Per-user video price overrides by model and resolution in USD per second.';
