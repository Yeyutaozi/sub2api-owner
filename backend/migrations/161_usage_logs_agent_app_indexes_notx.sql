CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_agent_run_created
	ON usage_logs (agent_run_id, created_at)
	WHERE agent_run_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_agent_app_created
	ON usage_logs (agent_app_id, created_at)
	WHERE agent_app_id IS NOT NULL;
