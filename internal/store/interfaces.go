package store

import (
	"context"
	"database/sql"
)

// DBTransaction defines the methods shared by *sql.DB and *sql.Tx
// This allows us to pass either a connection pool or an active transaction to the repository methods.
type DBTransaction interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}
