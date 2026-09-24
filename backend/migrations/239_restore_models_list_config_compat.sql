-- 239: restore the legacy models_list_config column for the application-center
-- compatibility path. Migration 235 moved the old value to model_allowlist,
-- while the owner edition still persists and reads both independent settings.
-- Add the column only when it is missing; never overwrite either setting.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE groups
   SET models_list_config = '{}'::jsonb
 WHERE models_list_config IS NULL;

ALTER TABLE groups
    ALTER COLUMN models_list_config SET DEFAULT '{}'::jsonb,
    ALTER COLUMN models_list_config SET NOT NULL;

COMMENT ON COLUMN groups.models_list_config IS
    'Legacy application-center model list configuration; independent from model_allowlist';
