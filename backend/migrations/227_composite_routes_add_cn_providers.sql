-- Keep the constraint as a superset of every previously supported concrete
-- platform. In particular, migration 193 already allowed GLM and must not be
-- narrowed when the CN and owner-edition video providers are added.
ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'glm', 'seedance', 'ltx',
                               'happyhorse', 'minimax', 'grokimagine'));
