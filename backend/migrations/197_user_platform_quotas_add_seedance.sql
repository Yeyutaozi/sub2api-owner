-- Keep the database constraint aligned with service.AllowedQuotaPlatforms
-- and ent/schema/user_platform_quota.go. These platforms are already valid
-- in application code but were missing from the PostgreSQL CHECK constraint.
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN (
        'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
        'glm', 'seedance', 'ltx', 'happyhorse'
    ));
