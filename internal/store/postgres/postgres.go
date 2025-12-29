// Package postgres implements the store interfaces using PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
)

// Store provides PostgreSQL-backed implementations of all repositories.
type Store struct {
	db *sql.DB
}

// New creates a new PostgreSQL store.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	// TODO: Connect to PostgreSQL and run migrations
	return &Store{}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
