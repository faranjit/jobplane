CREATE TABLE execution_artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES executions(id),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    filename        VARCHAR(255) NOT NULL,
    content_type    VARCHAR(127) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes      BIGINT NOT NULL,
    storage_backend VARCHAR(16) NOT NULL DEFAULT 'local',
    storage_path    TEXT,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_execution_filename UNIQUE (execution_id, filename)
);

CREATE INDEX idx_artifacts_tenant_status ON execution_artifacts (tenant_id, status);
CREATE INDEX idx_artifacts_execution ON execution_artifacts (execution_id);
