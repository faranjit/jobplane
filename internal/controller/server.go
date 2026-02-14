// Package controller contains the controller-specific logic for the HTTP API.
package controller

import (
	"context"
	"jobplane/internal/config"
	"jobplane/internal/controller/handlers"
	"jobplane/internal/controller/middleware"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server is the HTTP server for the controller API.
type Server struct {
	httpServer *http.Server
}

// New creates a new controller server.
func New(addr string, store handlers.StoreFactory, config *config.Config, metricsHandler http.Handler) *Server {
	rateLimiter := middleware.NewRateLimiter()
	authMW := middleware.AuthMiddleware(store)
	rateMW := rateLimiter.Middleware()

	h := handlers.New(store, handlers.HandlerConfig{
		VisibilityExtension: config.HeartVisibilityExtension,
	}).WithCallbacks(handlers.Callbacks{
		OnTenantUpdated: rateLimiter.InvalidateTenant,
	})

	mux := http.NewServeMux()

	// Probes (No Auth)
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)

	mux.HandleFunc("POST /tenants", h.CreateTenant)

	// Public authenticated apis
	mux.Handle("PATCH /tenants/{id}", authMW(rateMW(http.HandlerFunc(h.UpdateTenant))))
	mux.Handle("POST /jobs", authMW(rateMW(http.HandlerFunc(h.CreateJob))))
	mux.Handle("POST /jobs/{id}/run", authMW(rateMW(http.HandlerFunc(h.RunJob))))
	mux.Handle("GET /executions/{id}", authMW(rateMW(http.HandlerFunc(h.GetExecution))))
	mux.Handle("GET /executions/{id}/logs", authMW(rateMW(http.HandlerFunc(h.GetExecutionLogs))))
	mux.Handle("GET /executions/dlq", authMW(rateMW(http.HandlerFunc(h.GetDQLExecutions))))
	mux.Handle("POST /executions/dlq/{id}/retry", authMW(rateMW(http.HandlerFunc(h.RetryDQLExecution))))

	// Internal endpoints
	// These are called by the Worker Agent.
	// these should run on a separate port or strict network rules.
	// Internal API (System Secret Auth)
	internalAuth := middleware.RequireInternalAuth(config.SystemSecret)

	mux.Handle("POST /internal/executions/dequeue", internalAuth(http.HandlerFunc(h.InternalDequeue)))
	mux.Handle("PUT /internal/executions/{id}/heartbeat", internalAuth(http.HandlerFunc(h.InternalHeartbeat)))
	mux.Handle("PUT /internal/executions/{id}/result", internalAuth(http.HandlerFunc(h.InternalUpdateResult)))
	mux.Handle("POST /internal/executions/{id}/logs", internalAuth(http.HandlerFunc(h.InternalAddLogs)))

	// Metrics Endpoint
	if metricsHandler != nil {
		mux.Handle("GET /metrics", metricsHandler)
	}

	handler := otelhttp.NewHandler(mux, "controller-server")

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
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
