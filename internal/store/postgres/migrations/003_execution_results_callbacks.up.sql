-- Execution Results & Integration Hooks
ALTER TABLE executions ADD COLUMN result JSONB;
ALTER TABLE executions ADD COLUMN callback_url TEXT;
ALTER TABLE executions ADD COLUMN callback_headers JSONB;
ALTER TABLE executions ADD COLUMN callback_status TEXT CHECK (callback_status IN ('pending', 'delivered', 'failed'));