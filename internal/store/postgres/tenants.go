package postgres

import (
	"context"
	"jobplane/internal/store"

	"github.com/google/uuid"
)

func (s *Store) CreateTenant(ctx context.Context, tenant *store.Tenant, hashedKey string) error {
	query := `
		INSERT INTO tenants (id, name, api_key_hash, created_at, rate_limit, rate_limit_burst, max_concurrent_executions)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.db.Exec(query,
		tenant.ID,
		tenant.Name,
		hashedKey,
		tenant.CreatedAt,
		tenant.RateLimit,
		tenant.RateLimitBurst,
		tenant.MaxConcurrentExecutions,
	)
	return err
}

func (s *Store) UpdateTenant(ctx context.Context, tenant *store.Tenant) error {
	query := `
		UPDATE tenants
		SET name = $2,
		rate_limit = $3,
		rate_limit_burst = $4,
		max_concurrent_executions = $5
		WHERE id = $1
	`

	_, err := s.db.ExecContext(ctx,
		query,
		tenant.ID,
		tenant.Name,
		tenant.RateLimit,
		tenant.RateLimitBurst,
		tenant.MaxConcurrentExecutions,
	)
	return err
}

func (s *Store) GetTenantByID(ctx context.Context, id uuid.UUID) (*store.Tenant, error) {
	query := "SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE id = $1"

	var t store.Tenant

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID,
		&t.Name,
		&t.RateLimit,
		&t.RateLimitBurst,
		&t.MaxConcurrentExecutions,
		&t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func (s *Store) GetTenantByAPIKeyHash(ctx context.Context, hash string) (*store.Tenant, error) {
	query := "SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE api_key_hash = $1"

	var t store.Tenant

	err := s.db.QueryRowContext(ctx, query, hash).Scan(
		&t.ID,
		&t.Name,
		&t.RateLimit,
		&t.RateLimitBurst,
		&t.MaxConcurrentExecutions,
		&t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &t, nil
}
