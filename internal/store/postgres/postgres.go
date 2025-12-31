// Package postgres implements the store interfaces using PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
)

// Store is the PostgreSQL implementation of the application's storage interfaces.
// It holds the database connection pool.
type Store struct {
	db *sql.DB
}

// New creates a connected Store instance.
// It verifies the connection with a Ping before returning.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("postgress", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// BeginTx starts a new transaction.
// The caller is responsible for calling Commit() or Rollback().
func (s *Store) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}
