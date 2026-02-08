ALTER TABLE tenants
DROP COLUMN rate_limit,
DROP COLUMN rate_limit_burst,
DROP COLUMN max_concurrent_executions;