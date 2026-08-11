-- Independent per-group safe cost multiplier ceiling for upstream accounts.
-- Not tied to selling rate_multiplier (user-specific sell rates may differ).
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS safe_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

-- Seed existing rows from current sell rate so behavior stays continuous until admins adjust.
UPDATE groups
SET safe_rate_multiplier = rate_multiplier;

COMMENT ON COLUMN groups.safe_rate_multiplier IS
    'Independent per-group safe upstream cost multiplier. Accounts with known upstream cost strictly above this are cut from scheduling for this group. Independent of sell rate_multiplier.';
