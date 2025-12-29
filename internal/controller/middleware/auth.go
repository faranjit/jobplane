// Package middleware contains HTTP middleware for the controller.
package middleware

import (
	"context"
	"net/http"
)

// tenantIDKey is the context key for the tenant ID.
type tenantIDKey struct{}

// Auth is middleware that extracts and validates the tenant from the request.
// Every operation must be scoped by tenant_id.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Extract tenant from header/token
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			http.Error(w, "missing tenant ID", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), tenantIDKey{}, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantIDFromContext extracts the tenant ID from the context.
func TenantIDFromContext(ctx context.Context) string {
	if v := ctx.Value(tenantIDKey{}); v != nil {
		return v.(string)
	}
	return ""
}
