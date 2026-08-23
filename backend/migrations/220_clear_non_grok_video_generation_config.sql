-- The owner edition supports video generation on multiple platforms, including
-- Seedance, LTX, HappyHorse, MiniMax, Grok Imagine, and composite routes.
-- Clearing prices solely because a group is not Grok would remove valid billing
-- and model-exposure configuration. Keep this migration as a no-op marker so
-- upgrades preserve every existing video platform and remain forward-compatible
-- with additional providers.
SELECT 1;
