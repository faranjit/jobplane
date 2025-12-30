// Package middleware contains HTTP middleware for the controller.
package middleware

import (
	"context"
	"jobplane/internal/auth"
	"jobplane/internal/store"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

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
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			if tenant == nil {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tenantIDKey{}, tenant.ID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantIDFromContext extracts the tenant ID from the context.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(tenantIDKey{})
	if v == nil {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}
