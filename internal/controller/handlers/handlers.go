// Package handlers contains HTTP handlers for the controller API.
package handlers

import (
	"context"
	"encoding/json"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// StoreFactory combines the interfaces needed for the controller to function.
type StoreFactory interface {
	BeginTx(ctx context.Context) (store.Tx, error)
	Ping(ctx context.Context) error
	store.JobStore
	store.TenantStore
	store.Queue
}

// HandlerConfig holds configuration related to handlers.
type HandlerConfig struct {
	VisibilityExtension time.Duration // How long to extend visibility on heartbeat (default: 5m)
}

// Handlers holds all HTTP handlers and their dependencies.
type Handlers struct {
	store     StoreFactory
	config    HandlerConfig
	callbacks Callbacks
}

// Callbacks holds optional callbacks for handlers.
type Callbacks struct {
	// Called after a tenant is updated.
	// This is useful for invalidating caches or triggering other actions.
	OnTenantUpdated func(uuid.UUID)
}

// New creates a new Handlers instance with the given store dependency.
func New(s StoreFactory, c HandlerConfig) *Handlers {
	return &Handlers{store: s, config: c}
}

// WithCallbacks returns a new Handlers instance with the given callbacks.
func (h *Handlers) WithCallbacks(c Callbacks) *Handlers {
	h.callbacks = c
	return h
}

// A helper function to write standard JSON responses.
func (h *Handlers) respondJson(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

// A helper function to return consistent error messages.
func (h *Handlers) httpError(w http.ResponseWriter, message string, code int) {
	h.respondJson(w, code, api.ErrorResponse{
		Error: message,
		Code:  strconv.Itoa(code),
	})
}
