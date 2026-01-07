// Package handlers contains HTTP handlers for the controller API.
package handlers

import (
	"context"
	"encoding/json"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"strconv"
)

// StoreFactory combines the interfaces needed for the controller to function.
type StoreFactory interface {
	BeginTx(ctx context.Context) (store.Tx, error)
	Ping(ctx context.Context) error
	store.JobStore
	store.TenantStore
	store.Queue
	store.UserStore
}

// Handlers holds all HTTP handlers and their dependencies.
type Handlers struct {
	store StoreFactory
}

// New creates a new Handlers instance with the given store dependency.
func New(s StoreFactory) *Handlers {
	return &Handlers{store: s}
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
