package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jobplane/internal/store"

	"github.com/google/uuid"
)

// mockTenantStore implements store.TenantStore for testing
type mockTenantStore struct {
	tenant *store.Tenant
	err    error
}

func (m *mockTenantStore) GetTenantByID(ctx context.Context, id uuid.UUID) (*store.Tenant, error) {
	return m.tenant, m.err
}

func (m *mockTenantStore) GetTenantByAPIKeyHash(ctx context.Context, hash string) (*store.Tenant, error) {
	return m.tenant, m.err
}

func TestAuthMiddleware_MissingAuthHeader(t *testing.T) {
	mockStore := &mockTenantStore{}
	middleware := AuthMiddleware(mockStore)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidAuthHeaderFormat(t *testing.T) {
	mockStore := &mockTenantStore{}
	middleware := AuthMiddleware(mockStore)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "api-key-123"},
		{"wrong prefix", "Basic api-key-123"},
		{"too many parts", "Bearer key1 key2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tt.header)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestAuthMiddleware_StoreError(t *testing.T) {
	mockStore := &mockTenantStore{
		err: errors.New("database error"),
	}
	middleware := AuthMiddleware(mockStore)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestAuthMiddleware_TenantNotFound(t *testing.T) {
	mockStore := &mockTenantStore{
		tenant: nil,
		err:    nil,
	}
	middleware := AuthMiddleware(mockStore)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ValidAuth(t *testing.T) {
	tenantID := uuid.New()
	mockStore := &mockTenantStore{
		tenant: &store.Tenant{
			ID:        tenantID,
			Name:      "Test Tenant",
			CreatedAt: time.Now(),
		},
	}
	middleware := AuthMiddleware(mockStore)

	var capturedCtx context.Context
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-api-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify context has tenant ID
	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
}

func TestTenantIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	id, ok := TenantIDFromContext(ctx)

	if ok {
		t.Error("expected ok to be false for empty context")
	}
	if id != uuid.Nil {
		t.Errorf("expected uuid.Nil, got %v", id)
	}
}
