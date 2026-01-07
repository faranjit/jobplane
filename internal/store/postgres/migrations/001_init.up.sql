-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 2. Jobs (Definitions)
CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID REFERENCES tenants(id),
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    default_command JSONB,
    default_timeout INT DEFAULT 300, -- 5 mins
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 3. Executions (State/History)
CREATE TABLE executions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID REFERENCES jobs(id),
    tenant_id UUID REFERENCES tenants(id),
    status TEXT CHECK (status IN ('PENDING', 'SCHEDULED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED')),
    priority INT NOT NULL DEFAULT 0,
    attempt INT DEFAULT 0,
    exit_code INT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    scheduled_at TIMESTAMP,
    started_at TIMESTAMP,
    finished_at TIMESTAMP
);

-- 4. Execution Queue (The Mechanism)
CREATE TABLE execution_queue (
    id BIGSERIAL PRIMARY KEY,
    execution_id UUID UNIQUE REFERENCES executions(id),
    tenant_id UUID REFERENCES tenants(id),
    payload JSONB NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    attempt INT DEFAULT 0,
    visible_after TIMESTAMP DEFAULT NOW(), -- For retry/backoff
    created_at TIMESTAMP DEFAULT NOW()
);

-- 5. ExecutionLogs
CREATE TABLE execution_logs (
    id BIGSERIAL PRIMARY KEY,
    execution_id UUID REFERENCES executions(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Primary dequeue index: covers WHERE visible_after <= NOW() ORDER BY priority DESC, created_at ASC
-- This index allows PostgreSQL to:
-- 1. Use an index scan to filter visible_after <= NOW()
-- 2. Return rows already sorted by created_at (no extra sort step)
CREATE INDEX idx_queue_dequeue ON execution_queue (priority DESC, created_at ASC, visible_after)
WHERE visible_after IS NOT NULL;

-- Tenant-filtered dequeue index: for when workers are assigned to specific tenants
-- Covers: WHERE tenant_id = ? AND visible_after <= NOW() ORDER BY priority DESC, created_at ASC
CREATE INDEX idx_queue_tenant_dequeue ON execution_queue (tenant_id, priority DESC, created_at ASC, visible_after)
WHERE visible_after IS NOT NULL;

-- Index by execution time
CREATE INDEX idx_logs_execution_time ON execution_logs (execution_id, created_at);
