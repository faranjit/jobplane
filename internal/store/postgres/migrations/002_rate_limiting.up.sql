ALTER TABLE tenants
ADD COLUMN rate_limit INT DEFAULT 100,
ADD COLUMN rate_limit_burst INT DEFAULT 200,
ADD COLUMN max_concurrent_executions INT DEFAULT 10; --0 = unlimited