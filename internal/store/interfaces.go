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

// TenantStore defines the methods for tenant operations.
type TenantStore interface {
	// GetTenantByID returns a tenant by its ID.
	GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)

	// GetTenantByAPIKeyHash returns a tenant by its API key hash.
	GetTenantByAPIKeyHash(ctx context.Context, hash string) (*Tenant, error)
}
