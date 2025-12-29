// Package handlers contains HTTP handlers for the controller API.
package handlers

import (
	"net/http"
)

// Handlers holds all HTTP handlers and their dependencies.
type Handlers struct {
	// TODO: Add store dependencies
}

// New creates a new Handlers instance.
func New() *Handlers {
	return &Handlers{}
}

// SubmitJob handles POST /jobs - submits a new job for execution.
func (h *Handlers) SubmitJob(w http.ResponseWriter, r *http.Request) {
	// TODO: Validate request, create job, enqueue execution
}

// GetJobStatus handles GET /jobs/{id} - returns job status.
func (h *Handlers) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch job and return status
}

// ListExecutions handles GET /jobs/{id}/executions - returns execution history.
func (h *Handlers) ListExecutions(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch executions for job
}
