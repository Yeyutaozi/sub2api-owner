-- Run-scoped API key bindings for app center model proxy calls.
-- Worker receives only node/role/model metadata; Sub2API selects the user's
-- run-bound API key before entering the existing gateway.

CREATE TABLE IF NOT EXISTS agent_run_key_bindings (
    id              BIGSERIAL PRIMARY KEY,
    run_id          BIGINT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id      BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    policy_key      VARCHAR(255) NOT NULL,
    node_id         VARCHAR(120),
    node_role       VARCHAR(120),
    model_group_id  BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    capability      VARCHAR(50),
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_run_key_bindings_policy_key_check CHECK (policy_key <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_run_key_bindings_run_policy
    ON agent_run_key_bindings(run_id, policy_key);

CREATE INDEX IF NOT EXISTS idx_agent_run_key_bindings_run_id
    ON agent_run_key_bindings(run_id);

CREATE INDEX IF NOT EXISTS idx_agent_run_key_bindings_api_key_id
    ON agent_run_key_bindings(api_key_id);

CREATE INDEX IF NOT EXISTS idx_agent_run_key_bindings_model_group_id
    ON agent_run_key_bindings(model_group_id);
