ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_billing_unit VARCHAR(20) NOT NULL DEFAULT 'per_second';

COMMENT ON COLUMN groups.video_billing_unit IS
    'Video price unit: per_second or per_request. Existing prices remain per_second until explicitly switched.';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_video_billing_unit_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_video_billing_unit_check
            CHECK (video_billing_unit IN ('per_second', 'per_request'));
    END IF;
END
$$;
