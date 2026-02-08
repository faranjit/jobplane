package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetTenantByID_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	tenantName := "Acme Corp"
	createdAt := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE id = \$1`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_limit", "rate_limit_burst", "max_concurrent_executions", "created_at"}).
			AddRow(tenantID, tenantName, 61, 61, 6, createdAt))

	tenant, err := store.GetTenantByID(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetTenantByID failed: %v", err)
	}
	if tenant.ID != tenantID {
		t.Errorf("got ID %v, want %v", tenant.ID, tenantID)
	}
	if tenant.Name != tenantName {
		t.Errorf("got Name %s, want %s", tenant.Name, tenantName)
	}
	if !tenant.CreatedAt.Equal(createdAt) {
		t.Errorf("got CreatedAt %v, want %v", tenant.CreatedAt, createdAt)
	}
	if tenant.RateLimit != 61 {
		t.Errorf("got RateLimit %d, want %d", tenant.RateLimit, 61)
	}
	if tenant.RateLimitBurst != 61 {
		t.Errorf("got RateLimitBurst %d, want %d", tenant.RateLimitBurst, 61)
	}
	if tenant.MaxConcurrentExecutions != 6 {
		t.Errorf("got MaxConcurrentExecutions %d, want %d", tenant.MaxConcurrentExecutions, 6)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetTenantByID_NotFound(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE id = \$1`).
		WithArgs(tenantID).
		WillReturnError(sql.ErrNoRows)

	tenant, err := store.GetTenantByID(ctx, tenantID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	if tenant != nil {
		t.Error("expected nil tenant")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetTenantByAPIKeyHash_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	tenantName := "Test Tenant"
	createdAt := time.Now().Truncate(time.Second)
	apiKeyHash := "abc123hash"

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE api_key_hash = \$1`).
		WithArgs(apiKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_limit", "rate_limit_burst", "max_concurrent_executions", "created_at"}).
			AddRow(tenantID, tenantName, 61, 61, 6, createdAt))

	tenant, err := store.GetTenantByAPIKeyHash(ctx, apiKeyHash)
	if err != nil {
		t.Fatalf("GetTenantByAPIKeyHash failed: %v", err)
	}
	if tenant.ID != tenantID {
		t.Errorf("got ID %v, want %v", tenant.ID, tenantID)
	}
	if tenant.Name != tenantName {
		t.Errorf("got Name %s, want %s", tenant.Name, tenantName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetTenantByAPIKeyHash_NotFound(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	apiKeyHash := "invalid-hash"

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, created_at FROM tenants WHERE api_key_hash = \$1`).
		WithArgs(apiKeyHash).
		WillReturnError(sql.ErrNoRows)

	tenant, err := store.GetTenantByAPIKeyHash(ctx, apiKeyHash)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	if tenant != nil {
		t.Error("expected nil tenant")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
