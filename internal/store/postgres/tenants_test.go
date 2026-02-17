package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	storeTypes "jobplane/internal/store"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetTenantByID_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	tenantName := "Acme Corp"
	callbackUrl := "https://hooks.example.com/jobs"
	callbackHeaders := json.RawMessage(`{"Authorization": "Bearer abc123"}`)
	createdAt := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, callback_url, callback_headers, created_at FROM tenants WHERE id = \$1`).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_limit", "rate_limit_burst", "max_concurrent_executions", "callback_url", "callback_headers", "created_at"}).
			AddRow(tenantID, tenantName, 61, 61, 6, callbackUrl, callbackHeaders, createdAt))

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
	if *tenant.CallbackURL != callbackUrl {
		t.Errorf("got CallbackURL %v, want %v", tenant.CallbackURL, callbackUrl)
	}
	if tenant.CallbackHeaders != nil {
		headers := string(tenant.CallbackHeaders)
		if headers != string(callbackHeaders) {
			t.Errorf("got CallbackHeaders %s, want %s", headers, callbackHeaders)
		}
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

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, callback_url, callback_headers, created_at FROM tenants WHERE id = \$1`).
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

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, callback_url, callback_headers, created_at FROM tenants WHERE api_key_hash = \$1`).
		WithArgs(apiKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_limit", "rate_limit_burst", "max_concurrent_executions", "callback_url", "callback_headers", "created_at"}).
			AddRow(tenantID, tenantName, 61, 61, 6, nil, []byte(nil), createdAt))

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
	if tenant.CallbackURL != nil {
		t.Errorf("got CallbackURL %s, want nil", *tenant.CallbackURL)
	}
	if tenant.CallbackHeaders != nil {
		t.Errorf("got CallbackHeaders %s, want nil", tenant.CallbackHeaders)
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

	mock.ExpectQuery(`SELECT id, name, rate_limit, rate_limit_burst, max_concurrent_executions, callback_url, callback_headers, created_at FROM tenants WHERE api_key_hash = \$1`).
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

func TestUpdateTenant_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	callbackURL := "https://hooks.example.com/jobs"
	callbackHeaders := json.RawMessage(`{"Authorization": "Bearer abc123"}`)

	tenant := &storeTypes.Tenant{
		ID:                      tenantID,
		Name:                    "Updated Corp",
		RateLimit:               50,
		RateLimitBurst:          100,
		MaxConcurrentExecutions: 10,
		CallbackURL:             &callbackURL,
		CallbackHeaders:         callbackHeaders,
	}

	mock.ExpectExec(`UPDATE tenants SET name = \$2, rate_limit = \$3, rate_limit_burst = \$4, max_concurrent_executions = \$5, callback_url = \$6, callback_headers = \$7 WHERE id = \$1`).
		WithArgs(tenantID, "Updated Corp", 50, 100, 10, callbackURL, callbackHeaders).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpdateTenant(ctx, tenant)
	if err != nil {
		t.Fatalf("UpdateTenant failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUpdateTenant_DatabaseError(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	tenant := &storeTypes.Tenant{
		ID:                      tenantID,
		Name:                    "Error Corp",
		RateLimit:               50,
		RateLimitBurst:          100,
		MaxConcurrentExecutions: 10,
	}

	mock.ExpectExec(`UPDATE tenants SET name = \$2, rate_limit = \$3, rate_limit_burst = \$4, max_concurrent_executions = \$5, callback_url = \$6, callback_headers = \$7 WHERE id = \$1`).
		WithArgs(tenantID, "Error Corp", 50, 100, 10, nil, []byte(nil)).
		WillReturnError(sql.ErrConnDone)

	err := store.UpdateTenant(ctx, tenant)
	if err != sql.ErrConnDone {
		t.Errorf("expected sql.ErrConnDone, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
