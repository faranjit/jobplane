package middleware

import (
	"encoding/json"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RateLimiter implements rate limiting based on tenant configuration.
// It uses a sync.Map to cache rate limiters for each tenant.
// The cache expires after a configurable TTL.
type RateLimiter struct {
	ttl      time.Duration
	limiters sync.Map // tenantID -> rate.Limiter
}

// Option is a function that configures a RateLimiter.
type Option func(*RateLimiter)

func WithTTL(ttl time.Duration) Option {
	return func(rl *RateLimiter) {
		rl.ttl = ttl
	}
}

type cachedLimiter struct {
	limiter   *rate.Limiter
	expiresAt time.Time
}

// NewRateLimiter creates a new rate limiter.
// It uses a sync.Map to cache rate limiters for each tenant.
// The cache expires after a configurable TTL.
func NewRateLimiter(opts ...Option) *RateLimiter {
	rl := &RateLimiter{
		ttl:      5 * time.Minute,
		limiters: sync.Map{},
	}

	for _, opt := range opts {
		opt(rl)
	}
	return rl
}

// Middleware implements rate limiting based on tenant configuration.
// It uses a sync.Map to cache rate limiters for each tenant.
// The cache expires after a configurable TTL.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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
				limiter := rl.getOrCreateLimiter(tenant)
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

// Invalidates a tenant's rate limiter cache.
// This is used when a tenant's rate limit changes.
// It affects immediately without waiting for the TTL to expire.
func (rl *RateLimiter) InvalidateTenant(tenantID uuid.UUID) {
	rl.limiters.Delete(tenantID)
}

func (rl *RateLimiter) getOrCreateLimiter(tenant *store.Tenant) *rate.Limiter {
	if limiter, ok := rl.limiters.Load(tenant.ID); ok {
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
	rl.limiters.Store(tenant.ID, &cachedLimiter{
		limiter:   limiter,
		expiresAt: time.Now().Add(rl.ttl),
	})
	return limiter
}
