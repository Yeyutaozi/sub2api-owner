-- Preserve the configured token unit on each reward claim.

ALTER TABLE token_reward_claims
    ADD COLUMN IF NOT EXISTS token_unit VARCHAR(8) NOT NULL DEFAULT 'raw';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'token_reward_claims_token_unit_check'
    ) THEN
        ALTER TABLE token_reward_claims
            ADD CONSTRAINT token_reward_claims_token_unit_check
            CHECK (token_unit IN ('raw', 'K', 'M', 'B', 'T'));
    END IF;
END $$;
