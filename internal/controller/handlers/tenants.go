package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"jobplane/internal/auth"
	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// CreateTenant handles POST /tenants (Admin Only).
// It generates a new API Key, hashes it for storage, and returns the raw key ONCE.
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req api.CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate a secure random API key (32 bytes)
	rawKeyBytes := make([]byte, 32)
	if _, err := rand.Read(rawKeyBytes); err != nil {
		h.httpError(w, "Entropy failure", http.StatusInternalServerError)
		return
	}
	apiKey := "jp_" + hex.EncodeToString(rawKeyBytes) // e.g. "jp_a1b2..."

	hashedKey := auth.HashKey(apiKey)

	rateLimit := req.RateLimit
	if rateLimit == 0 {
		rateLimit = 100
	}

	rateLimitBurst := req.RateLimitBurst
	if rateLimitBurst == 0 {
		rateLimitBurst = 100
	}

	tenant := &store.Tenant{
		ID:                      uuid.New(),
		Name:                    req.Name,
		RateLimit:               rateLimit,
		RateLimitBurst:          rateLimitBurst,
		MaxConcurrentExecutions: req.MaxConcurrentExecutions, // 0 = unlimited, so no need to check if empty
		CreatedAt:               time.Now(),
	}

	if err := h.store.CreateTenant(ctx, tenant, hashedKey); err != nil {
		h.httpError(w, "Failed to create tenant", http.StatusInternalServerError)
		return
	}

	// Return the Raw Key (This is the only time the user sees it)
	resp := api.CreateTenantResponse{
		ID:     tenant.ID.String(),
		Name:   tenant.Name,
		ApiKey: apiKey,
	}
	h.respondJson(w, http.StatusCreated, resp)
}

// UpdateTenant handles PATCH /tenants/{id} (Admin Only).
// It updates an existing tenant.
func (h *Handlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req api.UpdateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant, ok := middleware.TenantFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if req.Name != "" {
		tenant.Name = req.Name
	}

	if req.RateLimit != 0 {
		tenant.RateLimit = req.RateLimit
	}

	if req.RateLimitBurst != 0 {
		tenant.RateLimitBurst = req.RateLimitBurst
	}

	if req.MaxConcurrentExecutions != 0 {
		tenant.MaxConcurrentExecutions = req.MaxConcurrentExecutions
	}

	if err := h.store.UpdateTenant(ctx, tenant); err != nil {
		h.httpError(w, "Failed to update tenant", http.StatusInternalServerError)
		return
	}

	if h.callbacks.OnTenantUpdated != nil {
		h.callbacks.OnTenantUpdated(tenant.ID)
	}

	h.respondJson(w, http.StatusNoContent, nil)
}
