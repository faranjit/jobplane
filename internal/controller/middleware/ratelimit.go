package middleware

import (
	"encoding/json"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitMiddleware is middleware that extracts and validates the tenant from the request.
// Every operation must be scoped by tenant_id.
func RateLimitMiddleware(s store.TenantStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limiters := sync.Map{} // tenantID -> rate.Limiter

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, ok := TenantFromContext(r.Context())
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(api.ErrorResponse{
					Error: "Unauthorized",
					Code:  "401",
				})
				return
			}

			// RateLimit=0 means unlimited
			if tenant.RateLimit > 0 {
				limiter := getOrCreateLimiter(&limiters, tenant, 5*time.Minute)
				if !limiter.Allow() {
					w.Header().Set("Retry-After", "1")
					http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type cachedLimiter struct {
	limiter   *rate.Limiter
	expiresAt time.Time
}

func getOrCreateLimiter(limiters *sync.Map, tenant *store.Tenant, ttl time.Duration) *rate.Limiter {
	if limiter, ok := limiters.Load(tenant.ID); ok {
		cached := limiter.(*cachedLimiter)
		if time.Now().Before(cached.expiresAt) {
			return cached.limiter
		}
		// expired, need to create new
	}

	limiter := rate.NewLimiter(
		rate.Limit(tenant.RateLimit),
		tenant.RateLimitBurst,
	)
	limiters.Store(tenant.ID, &cachedLimiter{
		limiter:   limiter,
		expiresAt: time.Now().Add(ttl),
	})
	return limiter
}
