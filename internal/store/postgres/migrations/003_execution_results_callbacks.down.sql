ALTER TABLE executions DROP COLUMN IF EXISTS result;
ALTER TABLE executions DROP COLUMN IF EXISTS callback_url;
ALTER TABLE executions DROP COLUMN IF EXISTS callback_headers;
ALTER TABLE executions DROP COLUMN IF EXISTS callback_status;

ALTER TABLE tenants DROP COLUMN IF EXISTS callback_url;
ALTER TABLE tenants DROP COLUMN IF EXISTS callback_headers;

ALTER TABLE jobs DROP COLUMN IF EXISTS callback_url;
ALTER TABLE jobs DROP COLUMN IF EXISTS callback_headers;
