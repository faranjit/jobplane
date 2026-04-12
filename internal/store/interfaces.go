package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// DBTransaction defines the methods shared by *sql.DB and *sql.Tx
// This allows us to pass either a connection pool or an active transaction to the repository methods.
type DBTransaction interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type Tx interface {
	DBTransaction
	Commit() error
	Rollback() error
}

// TenantStore handles retrieving tenant information for authentication.
type TenantStore interface {
	// CreateTenant inserts a new tenant to the database
	CreateTenant(ctx context.Context, tenant *Tenant, hashedKey string) error

	// GetTenantByID returns a tenant by its ID.
	GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)

	// GetTenantByAPIKeyHash returns a tenant by its API key hash.
	GetTenantByAPIKeyHash(ctx context.Context, hash string) (*Tenant, error)

	// UpdateTenant updates an existing tenant.
	UpdateTenant(ctx context.Context, tenant *Tenant) error
}

// JobStore handles the persistence of Job definitions and Execution history.
type JobStore interface {
	// CreateJob inserts a new job definition to the database
	CreateJob(ctx context.Context, tx DBTransaction, job *Job) error

	// GetJobByID returns a job by its ID.
	GetJobByID(ctx context.Context, id uuid.UUID) (*Job, error)

	// CreateExecution inserts the initial state of a new execution to the database.
	CreateExecution(ctx context.Context, tx DBTransaction, execution *Execution) error

	// GetExecutionByID returns an execution by its ID.
	GetExecutionByID(ctx context.Context, id uuid.UUID) (*Execution, error)

	// UpdateCallbackStatus updates the callback status of an execution.
	UpdateCallbackStatus(ctx context.Context, executionID uuid.UUID, status string) error

	// AddLogEntry inserts a chunk of logs for an execution.
	AddLogEntry(ctx context.Context, executionID uuid.UUID, content string) error

	// GetExecutionLogs retrieves all logs for an execution, ordered by time.
	GetExecutionLogs(ctx context.Context, executionID uuid.UUID, afterID int64, limit int) ([]LogEntry, error)

	// ListDLQ lists the DLQ entries for a tenant.
	ListDLQ(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]DLQEntry, error)

	// RetryFromDLQ retries an execution from the DLQ.
	RetryFromDLQ(ctx context.Context, executionID uuid.UUID) (newExecutionID uuid.UUID, err error)
}

// ArtifactStore handles persistence of artifact metadata in PostgreSQL.
// This is separate from the StorageBackend (which handles blob storage).
// Metadata lives in the database so that listing is fast and strongly consistent,
// regardless of the underlying blob storage backend.
type ArtifactStore interface {
	// CreateArtifact inserts a new artifact record with status "pending".
	// Called at authorize time, before the file is uploaded.
	CreateArtifact(ctx context.Context, artifact *ExecutionArtifact) error

	// ConfirmArtifact transitions an artifact from "pending" to "confirmed"
	// and records the backend-specific storage path where the file was written.
	ConfirmArtifact(ctx context.Context, artifactID uuid.UUID, storagePath string) error

	// GetArtifact retrieves a single artifact by its ID.
	GetArtifact(ctx context.Context, artifactID uuid.UUID) (*ExecutionArtifact, error)

	// ListArtifactsByExecution returns all confirmed artifacts for an execution.
	ListArtifactsByExecution(ctx context.Context, executionID uuid.UUID) ([]ExecutionArtifact, error)

	// GetPendingArtifacts returns artifacts still in "pending" status older than the given time.
	// Used by the cleanup goroutine to sweep orphaned uploads.
	GetPendingArtifacts(ctx context.Context, olderThan time.Time) ([]ExecutionArtifact, error)

	// DeleteArtifact removes an artifact metadata record.
	// Called by the cleanup goroutine after deleting the backing file.
	DeleteArtifact(ctx context.Context, artifactID uuid.UUID) error

	// GetTenantStorageUsage returns the total size in bytes of all artifacts
	// (both pending and confirmed) for a tenant. Used for quota enforcement.
	GetTenantStorageUsage(ctx context.Context, tenantID uuid.UUID) (int64, error)
}
