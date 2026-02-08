// Package middleware contains HTTP middleware for the controller.
package middleware

import (
	"context"
	"database/sql"
	"errors"
	"jobplane/internal/auth"
	"jobplane/internal/store"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// tenantKey is the context key for the tenant.
type tenantKey struct{}

// tenantIDKey is the context key for the tenant ID.
type tenantIDKey struct{}

// AuthMiddleware is middleware that extracts and validates the tenant from the request.
// Every operation must be scoped by tenant_id.
func AuthMiddleware(s store.TenantStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}

			apiKey := parts[1]
			apiKeyHash := auth.HashKey(apiKey)
			tenant, err := s.GetTenantByAPIKeyHash(r.Context(), apiKeyHash)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "Invalid API key", http.StatusUnauthorized)
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey{}, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFromContext extracts the tenant from the context.
func TenantFromContext(ctx context.Context) (*store.Tenant, bool) {
	v := ctx.Value(tenantKey{})
	if v == nil {
		return nil, false
	}
	tenant, ok := v.(*store.Tenant)
	return tenant, ok
}

// TenantIDFromContext extracts the tenant ID from the context.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	tenant, ok := TenantFromContext(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return tenant.ID, true
}

// NewContextWithTenant returns a new context with the given tenant.
// This is used ONLY for testing handlers in isolation.
func NewContextWithTenant(ctx context.Context, tenant *store.Tenant) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}
