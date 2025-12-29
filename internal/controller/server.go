// Package controller contains the controller-specific logic for the HTTP API.
package controller

import (
	"context"
	"net/http"
)

// Server is the HTTP server for the controller API.
type Server struct {
	httpServer *http.Server
}

// New creates a new controller server.
func New(addr string) *Server {
	mux := http.NewServeMux()
	// TODO: Register handlers

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

// Run starts the HTTP server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// TODO: Implement graceful shutdown
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
