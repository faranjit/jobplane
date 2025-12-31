package store

import (
	"context"
	"database/sql"

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
}
