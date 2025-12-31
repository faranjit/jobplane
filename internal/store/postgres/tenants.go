package postgres

import (
	"context"
	"jobplane/internal/store"

	"github.com/google/uuid"
)

func (s *Store) CreateTenant(ctx context.Context, tenant *store.Tenant, hashedKey string) error {
	query := `
		INSERT INTO jobs (id, name, api_key_hash, created_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := s.db.Exec(query,
		tenant.ID,
		tenant.Name,
		hashedKey,
		tenant.CreatedAt,
	)
	return err
}

func (s *Store) GetTenantByID(ctx context.Context, id uuid.UUID) (*store.Tenant, error) {
	query := "SELECT id, name, created_at FROM tenants WHERE id = $1"

	var t store.Tenant

	err := s.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func (s *Store) GetTenantByAPIKeyHash(ctx context.Context, hash string) (*store.Tenant, error) {
	query := "SELECT id, name, created_at FROM tenants WHERE api_key_hash = $1"

	var t store.Tenant

	err := s.db.QueryRowContext(ctx, query, hash).Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &t, nil
}
