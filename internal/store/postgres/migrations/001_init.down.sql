-- Down migration: drop all tables
DROP INDEX IF EXISTS idx_logs_execution_time;
DROP INDEX IF EXISTS idx_queue_tenant_dequeue;
DROP INDEX IF EXISTS idx_queue_dequeue;
DROP TABLE IF EXISTS execution_logs;
DROP TABLE IF EXISTS execution_queue;
DROP TABLE IF EXISTS executions;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS tenants;
DROP EXTENSION IF EXISTS "uuid-ossp";
