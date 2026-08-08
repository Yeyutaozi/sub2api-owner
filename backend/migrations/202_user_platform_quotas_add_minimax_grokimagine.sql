-- Keep the database constraint aligned with service.AllowedQuotaPlatforms
-- and ent/schema/user_platform_quota.go.
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN (
        'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
        'glm', 'seedance', 'ltx', 'happyhorse', 'minimax', 'grokimagine'
    ));
