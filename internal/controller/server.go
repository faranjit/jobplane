// Package controller contains the controller-specific logic for the HTTP API.
package controller

import (
	"context"
	"jobplane/internal/controller/handlers"
	"jobplane/internal/controller/middleware"
	"net/http"
	"time"
)

// Server is the HTTP server for the controller API.
type Server struct {
	httpServer *http.Server
}

// New creates a new controller server.
func New(addr string, store handlers.StoreFactory) *Server {
	h := handlers.New(store)
	authMW := middleware.AuthMiddleware(store)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /tenants", h.CreateTenant)

	// Public authenticated apis
	mux.Handle("POST /jobs", authMW(http.HandlerFunc(h.CreateJob)))
	mux.Handle("POST /jobs/{id}/run", authMW(http.HandlerFunc(h.RunJob)))
	mux.Handle("GET /executions/{id}", authMW(http.HandlerFunc(h.GetExecution)))

	// Internal endpoints
	// These are called by the Worker Agent.
	// these should run on a separate port or strict network rules.
	mux.HandleFunc("PUT /internal/executions/{id}/heartbeat", h.InternalHeartbeat)
	mux.HandleFunc("PUT /internal/executions/{id}/result", h.InternalUpdateResult)

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

// Run starts the HTTP server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		shutDownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		return s.Shutdown(shutDownCtx)
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
