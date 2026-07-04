-- Token reward claims.
-- Rewards are configured in settings and claimed once per user/tier/cycle.

CREATE TABLE IF NOT EXISTS token_reward_claims (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier_id         VARCHAR(64) NOT NULL,
    cycle_type      VARCHAR(16) NOT NULL,
    cycle_start     TIMESTAMPTZ NOT NULL,
    cycle_end       TIMESTAMPTZ NOT NULL,
    required_tokens BIGINT NOT NULL,
    reward_balance  DECIMAL(20, 8) NOT NULL,
    token_snapshot  BIGINT NOT NULL,
    claimed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT token_reward_claims_cycle_type_check CHECK (cycle_type IN ('weekly', 'monthly')),
    CONSTRAINT token_reward_claims_required_tokens_check CHECK (required_tokens > 0),
    CONSTRAINT token_reward_claims_reward_balance_check CHECK (reward_balance > 0),
    CONSTRAINT token_reward_claims_token_snapshot_check CHECK (token_snapshot >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_token_reward_claims_user_tier_cycle
    ON token_reward_claims(user_id, tier_id, cycle_type, cycle_start);

CREATE INDEX IF NOT EXISTS idx_token_reward_claims_user_cycle
    ON token_reward_claims(user_id, cycle_type, cycle_start);
