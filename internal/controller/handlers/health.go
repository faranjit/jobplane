package handlers

import "net/http"

// Healthz is a liveness probe.
// It returns 200 OK if the server is running.
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	h.respondJson(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// Readyz is a readiness probe.
// It checks if the service is ready to accept traffic (e.g., DB is connected).
func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		h.httpError(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}
	h.respondJson(w, http.StatusOK, map[string]string{"status": "ready"})
}
