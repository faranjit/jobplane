package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jobplane/internal/store"

	"github.com/google/uuid"
)

func rateMw() func(http.Handler) http.Handler {
	return NewRateLimiter(WithTTL(5 * time.Minute)).Middleware()
}

func TestRateLimitMiddleware_NoTenantInContext(t *testing.T) {
	middleware := rateMw()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when no tenant in context")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRateLimitMiddleware_AllowsRequestUnderLimit(t *testing.T) {
	middleware := rateMw()

	handlerCalled := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	tenant := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Test Tenant",
		RateLimit:      100, // 100 requests per second
		RateLimitBurst: 200,
	}
	ctx := NewContextWithTenant(context.Background(), tenant)

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestRateLimitMiddleware_RejectsRequestOverLimit(t *testing.T) {
	middleware := rateMw()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tenant := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Test Tenant",
		RateLimit:      1, // 1 request per second
		RateLimitBurst: 1, // burst of 1
	}
	ctx := NewContextWithTenant(context.Background(), tenant)

	// First request should succeed (uses the burst)
	req1 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("first request: got status %d, want %d", rr1.Code, http.StatusOK)
	}

	// Second request should be rate limited (burst exhausted)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: got status %d, want %d", rr2.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_RetryAfterHeader(t *testing.T) {
	middleware := rateMw()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tenant := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Test Tenant",
		RateLimit:      1,
		RateLimitBurst: 1,
	}
	ctx := NewContextWithTenant(context.Background(), tenant)

	// Exhaust the burst
	req1 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	// This request should be rate limited and have Retry-After header
	req2 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	retryAfter := rr2.Header().Get("Retry-After")
	if retryAfter != "1" {
		t.Errorf("got Retry-After %q, want %q", retryAfter, "1")
	}
}

func TestRateLimitMiddleware_IndependentLimitsPerTenant(t *testing.T) {
	middleware := rateMw()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Tenant A with very low limit
	tenantA := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Tenant A",
		RateLimit:      1,
		RateLimitBurst: 1,
	}
	ctxA := NewContextWithTenant(context.Background(), tenantA)

	// Tenant B with high limit
	tenantB := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Tenant B",
		RateLimit:      100,
		RateLimitBurst: 100,
	}
	ctxB := NewContextWithTenant(context.Background(), tenantB)

	// Exhaust Tenant A's limit
	reqA1 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxA)
	rrA1 := httptest.NewRecorder()
	handler.ServeHTTP(rrA1, reqA1)

	reqA2 := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxA)
	rrA2 := httptest.NewRecorder()
	handler.ServeHTTP(rrA2, reqA2)

	// Tenant A should be rate limited
	if rrA2.Code != http.StatusTooManyRequests {
		t.Errorf("Tenant A second request: got status %d, want %d", rrA2.Code, http.StatusTooManyRequests)
	}

	// Tenant B should still be able to make requests
	reqB := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxB)
	rrB := httptest.NewRecorder()
	handler.ServeHTTP(rrB, reqB)

	if rrB.Code != http.StatusOK {
		t.Errorf("Tenant B request: got status %d, want %d", rrB.Code, http.StatusOK)
	}
}

func TestRateLimitMiddleware_UnlimitedWhenRateLimitZero(t *testing.T) {
	middleware := rateMw()

	handlerCallCount := 0
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		w.WriteHeader(http.StatusOK)
	}))

	tenant := &store.Tenant{
		ID:             uuid.New(),
		Name:           "Unlimited Tenant",
		RateLimit:      0, // 0 = unlimited
		RateLimitBurst: 0,
	}
	ctx := NewContextWithTenant(context.Background(), tenant)

	// Make many requests - all should succeed
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: got status %d, want %d", i, rr.Code, http.StatusOK)
		}
	}

	if handlerCallCount != 10 {
		t.Errorf("expected 10 handler calls, got %d", handlerCallCount)
	}
}
