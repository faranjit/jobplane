package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"jobplane/internal/auth"
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
		h.httpError(w, "Invalid JSON", http.StatusBadRequest)
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

	tenant := &store.Tenant{
		ID:        uuid.New(),
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	if err := h.store.CreateTenant(ctx, tenant, hashedKey); err != nil {
		h.httpError(w, "Failed to create execution", http.StatusInternalServerError)
	}

	// Return the Raw Key (This is the only time the user sees it)
	resp := api.CreateTenantResponse{
		ID:     tenant.ID.String(),
		Name:   tenant.Name,
		ApiKey: apiKey,
	}
	h.respondJson(w, http.StatusCreated, resp)
}
